package service

import (
	"context"
	"encoding/json"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// NoticeSyncService 平台消息 → 用户端通知中心的投递服务。
//
// 投递方向是**拉取**：用户端周期调平台 /merchant-api/notices/list，
// 把消息镜像到自己库的 notifications 表。与仓内既有的模块下发同步
// （module-sync）同一模式 —— 生产是单机部署，用户端没有公网回调地址，
// 平台无法主动推。
//
// 游标用**已投递消息的最大平台消息 ID**，不额外存状态：
// 平台消息 ID 单调递增，而本地已落地的消息就是"已投递过"的全部证据，
// 从它们的最大 source_msg_id 就能算出下一轮该从哪继续。少一份状态就少
// 一处可能与实际投递情况漂移的地方 —— 游标与真实数据永远一致。
type NoticeSyncService struct {
	notifRepo *repository.NotificationRepository
	tenantRep repository.TenantRepository
	client    *platform.NoticeClient
}

// NewNoticeSyncService 构造投递服务（使用全局 db）。
func NewNoticeSyncService() *NoticeSyncService {
	return &NoticeSyncService{
		notifRepo: repository.NewNotificationRepository(),
		tenantRep: repository.NewTenantRepository(),
		client:    platform.NewNoticeClient(),
	}
}

// NewNoticeSyncServiceWithDB 构造指定 db 的投递服务（测试注入）。
func NewNoticeSyncServiceWithDB(db *gorm.DB) *NoticeSyncService {
	return &NoticeSyncService{
		notifRepo: repository.NewNotificationRepositoryWithDB(db),
		tenantRep: repository.NewTenantRepository(),
		client:    platform.NewNoticeClient(),
	}
}

// NoticeSyncResult 一轮投递的结果。
type NoticeSyncResult struct {
	// Fetched 从平台取回的消息条数。
	Fetched int `json:"fetched"`
	// Delivered 本轮新落地（首次投递）的条数。
	Delivered int `json:"delivered"`
	// Duplicated 命中幂等键、本轮未重复落地的条数。
	Duplicated int `json:"duplicated"`
	// Tenants 本轮覆盖的租户数。
	Tenants int `json:"tenants"`
	// Skipped 平台联动未启用而整体跳过（true 时其余计数全为 0）。
	Skipped bool `json:"skipped"`
	// Degraded 平台不可达，本轮沿用上次已投递的数据。
	Degraded bool `json:"degraded"`
}

// Sync 执行一轮投递。
//
// 降级口径（照模块同步的同款判定）：
//   - 平台联动未启用 ⇒ 整个跳过，**不发起任何平台请求**；
//   - 平台不可达 ⇒ 保留上次已投递的消息，不清空、不报错中断
//     （清空会让运营方发的公告凭空消失，用户会当成系统故障）；
//   - 单条消息落库失败 ⇒ 记日志继续，不影响同批其它消息。
func (s *NoticeSyncService) Sync(ctx context.Context) (*NoticeSyncResult, error) {
	res := &NoticeSyncResult{}

	if !config.PlatformEnabled() {
		res.Skipped = true
		logger.Info("[notice-sync] 已跳过（平台联动未启用）")
		return res, nil
	}

	// 租户清单是全局表（tenants 无 tenant_id 列），直查不受隔离 callback 影响。
	tenants, err := s.listTenants(ctx)
	if err != nil {
		res.Degraded = true
		logger.Warnf("[notice-sync] 读取租户清单失败（按降级处理）: %v", err)
		return res, nil
	}
	if len(tenants) == 0 {
		return res, nil
	}
	res.Tenants = len(tenants)

	for _, t := range tenants {
		if t.Tenant == nil || t.Tenant.Code == "" {
			continue
		}
		fetched, delivered, duplicated, degraded, err := s.syncTenant(ctx, t.Tenant)
		res.Fetched += fetched
		res.Delivered += delivered
		res.Duplicated += duplicated
		if degraded {
			res.Degraded = true
		}
		if err != nil {
			logger.Warnf("[notice-sync] 租户 %s(code=%s) 投递失败: %v", t.Name, t.Code, err)
		}
	}

	return res, nil
}

// syncTenant 投递单个租户的消息。
func (s *NoticeSyncService) syncTenant(ctx context.Context, t *model.Tenant) (fetched, delivered, duplicated int, degraded bool, err error) {
	// 写面必须钉死在这个租户上：用 tenant.OnlyTenant 而不是继承父 ctx。
	//
	// 理由：隔离 callback 见到 SystemScope 会**跳过 Create 的 tenant_id 填充**
	// ⇒ 行落进列默认值 1（默认租户）⇒ 租户甲的消息出现在租户乙的通知中心。
	// OnlyTenant 构造一个"只带该租户"的干净 ctx，callback 照常填 tenant_id。
	tctx := tenant.OnlyTenant(ctx, t.ID)

	afterID, err := s.lastDeliveredMsgID(tctx)
	if err != nil {
		return 0, 0, 0, false, err
	}

	out, err := s.client.List(ctx, t.Code, afterID)
	if err != nil {
		// 平台不可达：保留上次已投递的数据（不清空），只标降级。
		logger.Warnf("[notice-sync] 拉取平台消息失败(tenant=%s): %v", t.Code, err)
		return 0, 0, 0, true, nil
	}

	fetched = len(out.List)
	for _, msg := range out.List {
		ok, derr := s.deliver(tctx, msg)
		switch {
		case derr != nil:
			logger.Warnf("[notice-sync] 消息落地失败(tenant=%s msg=%d): %v", t.Code, msg.ID, derr)
		case ok:
			delivered++
			s.reportDelivery(ctx, t, msg, "success", "")
		default:
			duplicated++
		}
	}

	return fetched, delivered, duplicated, false, nil
}

// deliver 把一条平台消息落成本租户的通知。
func (s *NoticeSyncService) deliver(ctx context.Context, msg platform.PlatformNotice) (bool, error) {
	level := msg.Level
	if !model.IsValidNotificationLevel(level) {
		level = model.NotificationLevelInfo
	}

	meta, _ := json.Marshal(map[string]string{
		"source": model.NotificationSourcePlatform,
		"level":  level,
	})

	n := &model.Notification{
		UserID:      0, // 广播：本租户全体成员都该看到
		Type:        model.NotificationTypePlatformNotice,
		Title:       msg.Title,
		Content:     msg.Content,
		Level:       level,
		Source:      model.NotificationSourcePlatform,
		SourceMsgID: msg.ID,
		Metadata:    string(meta),
	}
	// created_at 用平台的发布时间而不是本地落库时间：通知中心按 created_at
	// 排序，两端时间线必须一致，否则用户会看到"刚发的公告排在三天前"。
	if !msg.PublishedAt.IsZero() {
		n.CreatedAt = msg.PublishedAt
	}

	return s.notifRepo.UpsertFromPlatform(ctx, n)
}

// reportDelivery 把投递结果回报给平台（写投递台账）。
// 失败只出声：回报失败不影响本地已落地的消息。
func (s *NoticeSyncService) reportDelivery(ctx context.Context, t *model.Tenant, msg platform.PlatformNotice, status, errMsg string) {
	if err := s.client.ReportDelivery(ctx, msg.ID, t.Code, status, errMsg); err != nil {
		logger.Warnf("[notice-sync] 回报投递结果失败(tenant=%s msg=%d): %v", t.Code, msg.ID, err)
	}
}

// listTenants 取本实例的租户清单。
func (s *NoticeSyncService) listTenants(ctx context.Context) ([]repository.TenantWithActivity, error) {
	list, _, err := s.tenantRep.List(ctx, repository.TenantFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}
	return list, nil
}

// lastDeliveredMsgID 取本租户已投递的最大平台消息 ID（= 下一轮的游标）。
//
// 读的是 notifications 表里 source_msg_id 的最大值 —— 它就是"已投递过"的
// 全部证据，不需要额外的游标存储，也不会与实际投递情况漂移。
func (s *NoticeSyncService) lastDeliveredMsgID(ctx context.Context) (int64, error) {
	return s.notifRepo.MaxSourceMsgID(ctx)
}
