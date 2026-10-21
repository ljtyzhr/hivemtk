package bridge

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/service"
)

// ErrAccountOwnedByOther 归属冲突：该渠道账号已归属于其他用户，
// Upsert 仅刷新在线状态、不改动归属字段。供 handler 记录审计日志。
var ErrAccountOwnedByOther = errors.New("bridge account owned by another user")

// BridgeAccountRepository 桥接账号持久化（实现 BridgeAccountRepo 接口）。
//
// 放在 bridge 包内以避免 import cycle（repository 已间接依赖 bridge）。
// 注册即落库：维护 bridge_accounts，并联动 channel_agent_bindings 以保证 AI 路由。
type BridgeAccountRepository struct {
	db *gorm.DB
}

func NewBridgeAccountRepository(db *gorm.DB) *BridgeAccountRepository {
	return &BridgeAccountRepository{db: db}
}

// Upsert 注册/上线：写入 bridge_accounts（owner + 在线状态），并绑定智能体。
//
// 安全约束（防水平越权）：
//   - 记录已存在且归属(user_id)与当前请求不同 → 不覆盖归属/昵称/智能体，仅刷新在线状态，
//     并返回 ErrAccountOwnedByOther（handler 记审计日志，但 hub 收发不受影响）。
//   - 并发首建存在竞争：Create 命中唯一冲突时重试一次（此时记录已存在，走"已存在"分支校验归属）。
func (r *BridgeAccountRepository) Upsert(ctx context.Context, u BridgeAccountUpsert) error {
	// 两次 First→Create 都撞唯一键时，第二次循环退出前什么都没写：
	// 旧实现落到函数末尾 return nil，调用方（ingest 异步注册）把"没写成"当成功，
	// 账号在线位与归属就此静默缺失。记下最后一次冲突，循环耗尽后如实返回。
	var lastUniqueErr error
	for attempt := 0; attempt < 2; attempt++ {
		var acc model.BridgeAccount
		err := r.db.WithContext(ctx).Where("channel = ? AND account_id = ?", u.Channel, u.AccountID).First(&acc).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			acc = model.BridgeAccount{
				Channel:   u.Channel,
				AccountID: u.AccountID,
				UserID:    u.UserID,
				Status:    u.Status,
			}
			if u.AccountName != "" {
				acc.AccountName = u.AccountName
			}
			if u.AgentID != 0 {
				acc.AgentID = u.AgentID
			}
			now := time.Now()
			acc.LastSyncAt = &now
			if cerr := r.db.WithContext(ctx).Create(&acc).Error; cerr != nil {
				if isUniqueViolation(cerr) {
					lastUniqueErr = cerr
					continue
				}
				return cerr
			}
			if u.AgentID != 0 {
				return r.upsertBinding(ctx, u.Channel, u.AccountID, u.AgentID)
			}
			return nil
		} else if err != nil {
			return err
		}

		if acc.UserID != u.UserID {
			now := time.Now()
			if uerr := r.db.WithContext(ctx).Model(&model.BridgeAccount{}).
				Where("channel = ? AND account_id = ?", u.Channel, u.AccountID).
				Updates(map[string]any{"status": u.Status, "last_sync_at": now}).Error; uerr != nil {
				// 归属冲突与刷新失败是两件事：只报后者会让调用方以为"仅归属未改"，
				// 线上却连在线位都没刷新；包在一起交给 handler 一并留痕。
				return fmt.Errorf("%w（刷新在线状态失败：%v）", ErrAccountOwnedByOther, uerr)
			}
			return ErrAccountOwnedByOther
		}

		acc.Status = u.Status
		now := time.Now()
		acc.LastSyncAt = &now
		if u.AccountName != "" {
			acc.AccountName = u.AccountName
		}
		if u.AgentID != 0 {
			acc.AgentID = u.AgentID
		}
		if err := r.db.WithContext(ctx).Save(&acc).Error; err != nil {
			return err
		}
		if u.AgentID != 0 {
			return r.upsertBinding(ctx, u.Channel, u.AccountID, u.AgentID)
		}
		return nil
	}
	if lastUniqueErr != nil {
		return fmt.Errorf("bridge account upsert 重试后仍撞唯一键（channel=%s account_id=%s）: %w",
			u.Channel, u.AccountID, lastUniqueErr)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate key value")
}

func (r *BridgeAccountRepository) upsertBinding(ctx context.Context, channel, accountID string, agentID uint) error {
	r.db.Model(&model.ChannelAgentBinding{}).WithContext(ctx).
		Where("channel_type = ? AND account_id = ? AND agent_id <> ?", channel, accountID, agentID).
		Update("is_primary", false)

	var b model.ChannelAgentBinding
	err := r.db.WithContext(ctx).
		Where("channel_type = ? AND account_id = ? AND agent_id = ?", channel, accountID, agentID).
		First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		b = model.ChannelAgentBinding{ChannelType: channel, AccountID: accountID, AgentID: agentID}
	} else if err != nil {
		return err
	}
	b.IsPrimary = true
	b.Enabled = true
	return r.db.WithContext(ctx).Save(&b).Error
}

// SetOffline 连接断开：置离线 + 记录最后同步时间（G12）。
// ctx 用于透传 trace_id（审计链不断）；用 context.WithoutCancel 解绑 WS 生命周期，
// 防止 readPump defer 触发时 WS ctx 被取消导致 DB 写入失败。
func (r *BridgeAccountRepository) SetOffline(ctx context.Context, channel, accountID string) error {
	now := time.Now()
	dbCtx := context.Background()
	if ctx != nil {
		dbCtx = context.WithoutCancel(ctx)
	}
	return r.db.WithContext(dbCtx).Model(&model.BridgeAccount{}).
		Where("channel = ? AND account_id = ?", channel, accountID).
		Updates(map[string]any{"status": "offline", "last_sync_at": now}).Error
}

// TouchLastSync 心跳/活跃：刷新最后同步时间，并连带翻回在线位。
//
// 只写时间戳不够：status 停在 SetOffline 写下的 "offline" 后再无人改写，
// 于是 isOnlineByLastSync 第一句就判离线，管理面与回扫的在线门读到的都是假值。
// 心跳即在线是 SetOffline 的对偶。
func (r *BridgeAccountRepository) TouchLastSync(ctx context.Context, channel, accountID string) error {
	now := time.Now()
	dbCtx := context.Background()
	if ctx != nil {
		dbCtx = context.WithoutCancel(ctx)
	}
	return r.db.WithContext(dbCtx).Model(&model.BridgeAccount{}).
		Where("channel = ? AND account_id = ?", channel, accountID).
		Updates(map[string]any{"last_sync_at": now, "status": "online"}).Error
}

// OnlineGraceWindow status 为 online 时，last_sync_at 距今多久内仍算在线。
// 必须大于 SSE 心跳间隔——心跳是在线位的唯一续期来源，小于它会让活着的流按心跳周期闪烁成离线
// （由 TestHeartbeatCadenceWithinOnlineGraceWindow 钉住默认值）。
// 值取自 service 包的同名常量：那一层还有渠道总览与主动触达两个读侧要判在线，
// 两处各写一份秒数就会量出两个不同的"在线数"。
const OnlineGraceWindow = service.BridgeOnlineGraceDefault

func runtimeOnlineGraceWindow(ctx context.Context) time.Duration {
	return service.BridgeOnlineGraceWindow(ctx)
}

func isOnlineByLastSync(ctx context.Context, lastSyncAt *time.Time, status string, now time.Time) bool {
	if status == "offline" {
		return false
	}
	if lastSyncAt == nil {
		return false
	}
	return now.Sub(*lastSyncAt) < runtimeOnlineGraceWindow(ctx)
}

// IsAccountRowOnline 判定一条已经读出来的桥接账号行此刻是否收得到下行。
//
// 导出只为一件事：让包外的读侧（触达工具的账号健康查询）与包内的三个读侧走同一份判据。
// 各写一份秒数就会量出两个不同的"在线数"，而运维拿哪一份都对不上另两份。
func IsAccountRowOnline(ctx context.Context, lastSyncAt *time.Time, status string) bool {
	return isOnlineByLastSync(ctx, lastSyncAt, status, time.Now())
}

// ListForUser 列出当前租户内、当前用户可见的桥接账号。
//
// 生产登记入口（HTTP ingest）不携带用户身份，账号以 user_id=0 落库；那是
// 租户资产而不是"无主所以谁都看不见"的孤儿行。只按 user_id=? 过滤会让管理面
// 在单机与 SaaS 两种形态下恒为空。user_id 非零表示显式认领，仍只对本人可见；
// 租户边界由隔离 callback 收窄，本过滤不承担跨租户遮挡。
func (r *BridgeAccountRepository) ListForUser(ctx context.Context, userID uint) ([]BridgeAccountView, error) {
	var accs []model.BridgeAccount
	if err := r.db.WithContext(ctx).
		Where("user_id = 0 OR user_id = ?", userID).
		Order("id desc").Find(&accs).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	views := make([]BridgeAccountView, 0, len(accs))
	for _, a := range accs {
		views = append(views, BridgeAccountView{
			ID:          a.ID,
			UserID:      a.UserID,
			Channel:     a.Channel,
			AccountID:   a.AccountID,
			AccountName: a.AccountName,
			AgentID:     a.AgentID,
			Status:      a.Status,
			LastSyncAt:  a.LastSyncAt,
			Online:      isOnlineByLastSync(ctx, a.LastSyncAt, a.Status, now),
		})
	}
	return views, nil
}

// IsOnline 按 (channel, account_id) 查 DB 判断账号是否在线（供 controller 层使用）。
//
// 性能：单行 SELECT 走 (channel, account_id) 唯一索引，<1ms；为避免 ListByUser 后再单查的二次
// round trip，ListByUser 内部仍走 isOnlineByLastSync 直接判定。
func (r *BridgeAccountRepository) IsOnline(ctx context.Context, channel, accountID string) (bool, error) {
	var acc model.BridgeAccount
	if err := r.db.WithContext(ctx).Where("channel = ? AND account_id = ?", channel, accountID).First(&acc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return isOnlineByLastSync(ctx, acc.LastSyncAt, acc.Status, time.Now()), nil
}

// GetByChannelAccount 按渠道+账号查询（供归属校验使用）。
//
// 查无此行返回 (nil, nil)：对当前上下文而言"不存在"与"归属他人"同属不可见，
// 归属校验据此回 403。把 gorm.ErrRecordNotFound 抛给 router 的校验闭包会把
// 一次越权尝试（或跨租户访问）变成 500，运维看到的是服务故障而非拒绝。
func (r *BridgeAccountRepository) GetByChannelAccount(ctx context.Context, channel, accountID string) (*model.BridgeAccount, error) {
	var acc model.BridgeAccount
	err := r.db.WithContext(ctx).Where("channel = ? AND account_id = ?", channel, accountID).First(&acc).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &acc, nil
}

var _ BridgeAccountRepo = (*BridgeAccountRepository)(nil)
