package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

func (r *MessageHubRepository) CreateWithInboxTx(

	ctx context.Context,

	hub *model.MessageHub,

	inboxRepo *InboxConversationRepository,

	input UpsertFromMessageInput,

) error {
	if r == nil || r.db == nil {
		return nil
	}
	if inboxRepo == nil {
		return r.Create(ctx, hub)
	}

	if hub.MsgID != "" {
		var existing model.MessageHub
		if err := r.db.WithContext(ctx).Where("msg_id = ?", hub.MsgID).First(&existing).Error; err == nil {
			return nil
		}
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(hub).Error; err != nil {
			if isDuplicateKeyErr(err) {
				return nil
			}
			return err
		}
		return inboxRepo.UpsertFromMessageTx(ctx, tx, input)
	})
}

func (r *MessageHubRepository) MarkReadByID(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("id = ?", id).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

// UpdateMsgID 回写平台消息 ID（ChatbotX 模式移植 T2）。
//
// 业务背景：出站消息落库时平台尚未返回消息 ID（自造 wa-out-{UnixNano}
// 占位），发送成功后平台返回 wamid / message_id，必须回写用于：
//  1. echo 精确去重（入站回显按 platform+msg_id 命中 outgoing 行即拦截）
//  2. 状态回执对账（WA statuses 按 wamid 定位消息行）
//  3. 撤回/引用等平台能力的基础
//
// best-effort：回写失败仅影响对账，不阻断发送链路（调用方自行 WARN）。
// 仅更新 direction='outbound' 行，防御性避免误改入站行。
func (r *MessageHubRepository) UpdateMsgID(ctx context.Context, id uint, platformMsgID string) error {
	if r == nil || r.db == nil || platformMsgID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ? AND direction = ?", id, "outbound").
		Update("msg_id", platformMsgID).Error
}

// GetOutgoingByPlatformMsgID 按平台消息 ID 精确查找出站行（echo 拦截用）。
// 范围限定单账号 + 出站方向，避免跨账号/跨方向的偶然 ID 碰撞误判。
// D-9（2026-10-24）：两段式——先查 platform_msg_id 列（新合同），未命中回退
// msg_id 行键（WA 行键=wamid 本体、回填前的存量行——语义与旧口径一致）。
func (r *MessageHubRepository) GetOutgoingByPlatformMsgID(ctx context.Context, platform, accountID, msgID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil || msgID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var existing model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = ? AND platform_msg_id = ?",
			platform, accountID, "outbound", msgID).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	// errors.Is 而非裸 !=：GORM 的驱动层可能把 ErrRecordNotFound 包在
	// *pgconn 错误链里（尤其事务/重试路径），裸比较会把真"没查到"
	// 当成"查库出错"抛出去——echo 拦截与回执对账都要把"没有这一行"
	// 与"读不了库"分开说清。
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = ? AND msg_id = ?",
			platform, accountID, "outbound", msgID).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// GetOutgoingByPlatformMsgIDInConv 在 GetOutgoingByPlatformMsgID 基础上限定
// 会话范围（TG message_id 按聊天独立计数等场景防跨会话误拦）。
// D-9（2026-10-24）：同 GetOutgoingByPlatformMsgID 的两段式口径
// （platform_msg_id 优先，未命中回退 msg_id 行键）。
func (r *MessageHubRepository) GetOutgoingByPlatformMsgIDInConv(ctx context.Context, platform, accountID, conversationID, msgID string) (*model.MessageHub, error) {
	if r == nil || r.db == nil || msgID == "" || conversationID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var existing model.MessageHub
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = ? AND conversation_id = ? AND platform_msg_id = ?",
			platform, accountID, "outbound", conversationID, msgID).
		First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	// 同上：errors.Is 判"没有这一行"，包裹形态的 not found 不许漏进错误分支。
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	err = r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND direction = ? AND conversation_id = ? AND msg_id = ?",
			platform, accountID, "outbound", conversationID, msgID).
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

func (r *MessageHubRepository) MarkOutboundSendFailed(ctx context.Context, id uint, extra model.JSONMap) error {
	if r == nil || r.db == nil || id == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ? AND direction = ?", id, "outbound").
		Updates(map[string]any{
			"status": "send_failed",
			"extra":  extra,
		}).Error
}

// UpdateDeliveryStatus 按 wamid 更新出站消息的平台回执状态（ChatbotX 模式移植 T3）。
//
// status 语义（仅作用于官方渠道出站行，与桥接 outbox 认领状态机
// pending→inflight→delivered 分离——官方行不进 outbox 认领队列）：
//   - sent/delivered → Status 置对应值（终态 send_failed 不回翻——Meta 乱序回执防护）
//   - read           → IsRead=true + ReadAt
//   - failed         → Status='send_failed'，错误详情写入 Extra
//
// 未命中（旧占位 ID/未回写）返回 ErrRecordNotFound，调用方静默忽略。
// 列级 Updates（非 Save）：避免全列覆盖损毁行数据；条件更新兼做乐观锁，
// 并发回执按 WHERE status 命中与否天然收敛。
func (r *MessageHubRepository) UpdateDeliveryStatus(ctx context.Context, platform, accountID, msgID, status string, failureReason string) error {
	if r == nil || r.db == nil || msgID == "" || status == "" {
		return nil
	}
	row, err := r.GetOutgoingByPlatformMsgID(ctx, platform, accountID, msgID)
	if err != nil {
		return err
	}

	if row.Status == "send_failed" && status != "failed" {
		return nil
	}
	updates := map[string]any{}
	switch status {
	case "read":
		updates["is_read"] = true
		updates["read_at"] = time.Now()
	case "failed":
		updates["status"] = "send_failed"
		if failureReason != "" {
			updates["extra"] = mergeHubExtra(row.Extra, map[string]any{"delivery_error": failureReason})
		}
	case "sent", "delivered":
		updates["status"] = status
	default:
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", row.ID).Updates(updates).Error
}

func mergeHubExtra(existing model.JSONMap, patch map[string]any) model.JSONMap {
	out := model.JSONMap{}
	for k, v := range existing {
		out[k] = v
	}
	for k, v := range patch {
		out[k] = v
	}
	return out
}

func (r *MessageHubRepository) MarkReadByIDs(ctx context.Context, ids []uint) error {
	if r == nil || r.db == nil || len(ids) == 0 {
		return nil
	}
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("id IN ?", ids).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

// UpdateMediaURL 回填媒体消息的转存 URL（渠道媒体 ID 有效期短，转存后把长期 URL 写回）。
func (r *MessageHubRepository) UpdateMediaURL(ctx context.Context, id uint, mediaURL string) error {
	if r == nil || r.db == nil || id == 0 || mediaURL == "" {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).Where("id = ?", id).
		Update("media_url", mediaURL).Error
}

// UpdateMediaURLByMsgID 按 platform+msg_id 回填 media_url（Ingress 去重后行主键未知时的兜底路径）。
func (r *MessageHubRepository) UpdateMediaURLByMsgID(ctx context.Context, platform, msgID, mediaURL string) error {
	if r == nil || r.db == nil || platform == "" || msgID == "" || mediaURL == "" {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("platform = ? AND msg_id = ?", platform, msgID).
		Update("media_url", mediaURL).Error
}

// BackfillMediaByRowKey 按平台+账号+行键定位 hub 行，写 media_url 并把
// media_urls 列表**合并进** Extra（保留行上已有的其他 Extra 键——
// 官方 server_message_id 是报障对号的唯一抓手，不许抹）。
//
// E-4 webhook 域下沉（2026-10-24）：抖音/QQ 的媒体回填原先在 service 层
// 直拿 *gorm.DB 两段式（First + Updates），是本域最后两处 service 层直查。
// 返回 found=false 表示行不存在（调用方按原语义 WARN 后继续，不是错误）。
func (r *MessageHubRepository) BackfillMediaByRowKey(ctx context.Context, platform, accountID, rowKey, mediaURL string, mediaURLs []string) (found bool, err error) {
	if r == nil || r.db == nil || rowKey == "" {
		return false, nil
	}
	var hub model.MessageHub
	if err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND msg_id = ?", platform, accountID, rowKey).
		First(&hub).Error; err != nil {
		return false, nil // 行不存在：媒体回填是 best-effort，找不到就跳过（原语义）
	}
	extra := hub.Extra
	if extra == nil {
		extra = model.JSONMap{}
	}
	extra["media_urls"] = mediaURLs
	fields := map[string]any{"extra": extra}
	if mediaURL != "" {
		fields["media_url"] = mediaURL
	}
	if err := r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", hub.ID).
		Updates(fields).Error; err != nil {
		return false, err
	}
	return true, nil
}

// BackfillMediaByRowKeyExtra BackfillMediaByRowKey 的多键变体：Extra 的合并键集
// 由调用方给（TG 除了 media_urls 还要 media_file_ids——file_id 长期有效，留痕后
// 链接失效或存储迁移时还能重新拉取原件）。已有键被覆盖、其余键保留。
func (r *MessageHubRepository) BackfillMediaByRowKeyExtra(ctx context.Context, platform, accountID, rowKey, mediaURL string, extraPatch model.JSONMap) (found bool, err error) {
	if r == nil || r.db == nil || rowKey == "" {
		return false, nil
	}
	var hub model.MessageHub
	if err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND msg_id = ?", platform, accountID, rowKey).
		First(&hub).Error; err != nil {
		return false, nil // 行不存在：best-effort，跳过（原语义）
	}
	extra := hub.Extra
	if extra == nil {
		extra = model.JSONMap{}
	}
	for k, v := range extraPatch {
		extra[k] = v
	}
	fields := map[string]any{"extra": extra}
	if mediaURL != "" {
		fields["media_url"] = mediaURL
	}
	if err := r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", hub.ID).
		Updates(fields).Error; err != nil {
		return false, err
	}
	return true, nil
}

// UpdateExtra 全量覆盖 hub 行的 extra 字段（用于发送侧回填平台消息 ID 等元数据）。
func (r *MessageHubRepository) UpdateExtra(ctx context.Context, id uint, extra model.JSONMap) error {
	if r == nil || r.db == nil || id == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", id).
		Update("extra", extra).Error
}

// UpdatePlatformMsgID D-9（2026-10-24）：发送成功后把平台侧消息号写入 hub 行。
// platform_msg_id 是一等列（v3.82.0），echo 拦截与 Recall 反查优先走它。
func (r *MessageHubRepository) UpdatePlatformMsgID(ctx context.Context, id uint, platformMsgID string) error {
	if r == nil || r.db == nil || id == 0 || platformMsgID == "" {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.MessageHub{}).
		Where("id = ?", id).
		Update("platform_msg_id", platformMsgID).Error
}
