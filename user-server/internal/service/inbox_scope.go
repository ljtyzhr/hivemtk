package service

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
)

// ErrInboxForbidden 非超管访问了未分配给自己的会话（数据权限拦截）
var ErrInboxForbidden = errors.New("无权访问该会话")

// InboxScope 收件箱数据权限上下文（由 controller 从 JWT 注入）
//
// 规则（最小权限）：
//   - Role == "admin" 或 Role 为空（旧调用/内部任务）→ 不隔离
//   - 其余角色（customer_service/staff）→ 只能见到 assigned_to == Username 的会话
type InboxScope struct {
	Role     string
	Username string
}

// restricted 判定是否需要行级隔离
func (sc InboxScope) restricted() bool {
	return sc.Role != "" && sc.Role != model.SystemRoleCodeAdmin
}

// visible 会话归属是否对当前 scope 可见
func (sc InboxScope) visible(conv *model.InboxConversation) bool {
	if !sc.restricted() || conv == nil {
		return true
	}
	return conv.AssignedTo != "" && conv.AssignedTo == sc.Username
}

// applyScope 非超管强制覆盖查询条件为“只看自己”（忽略入参 assigned_to）
func (sc InboxScope) applyScope(q *InboxQuery) {
	if sc.restricted() {
		q.AssignedTo = sc.Username
	}
}

// ensureVisible 取会话并校验归属；不可见返回 ErrInboxForbidden
func (s *InboxService) ensureVisible(ctx context.Context, conversationID uint, sc InboxScope) (*model.InboxConversation, error) {
	conv, err := s.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if !sc.visible(conv) {
		return nil, ErrInboxForbidden
	}
	return conv, nil
}

// ListScoped 带数据权限的会话列表（非超管强制 assigned_to=自己）
func (s *InboxService) ListScoped(ctx context.Context, q InboxQuery, sc InboxScope) ([]*model.InboxConversation, int64, error) {
	sc.applyScope(&q)
	return s.List(ctx, q)
}

// GetByIDScoped 带归属校验的会话详情
func (s *InboxService) GetByIDScoped(ctx context.Context, id uint, sc InboxScope) (*model.InboxConversation, error) {
	return s.ensureVisible(ctx, id, sc)
}

// GetStatsScoped 带数据权限的统计（非超管只统计 assigned_to=自己的会话）
func (s *InboxService) GetStatsScoped(ctx context.Context, sc InboxScope) (*InboxStats, error) {
	if !sc.restricted() || s.inboxRepo == nil {
		return s.GetStats(ctx)
	}
	validStatuses := []string{InboxStatusUnread, InboxStatusOpen, InboxStatusAssigned, InboxStatusClosed}
	activeStatuses := []string{InboxStatusAssigned, InboxStatusOpen}
	threshold := time.Now().Add(-InboxOverdueThreshold)
	res, err := s.inboxRepo.GetStatsByAssignedTo(ctx, validStatuses, activeStatuses, InboxFromCustomer, threshold, sc.Username)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return &InboxStats{ByPlatform: map[string]int64{}, ByAssignedTo: map[string]int64{}}, nil
	}
	return &InboxStats{
		Total:        res.Total,
		Unread:       res.Unread,
		Open:         res.Open,
		Assigned:     res.Assigned,
		Closed:       res.Closed,
		ByPlatform:   res.ByPlatform,
		ByAssignedTo: res.ByAssignedTo,
		OverdueCount: res.OverdueCount,
	}, nil
}

// GetMessagesByConversationScoped 带归属校验的会话消息
func (s *InboxService) GetMessagesByConversationScoped(ctx context.Context, conversationID uint, page, pageSize int, sc InboxScope) ([]map[string]any, int64, error) {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return nil, 0, err
	}
	return s.GetMessagesByConversation(ctx, conversationID, page, pageSize)
}

// MarkReadScoped 带归属校验的已读标记
func (s *InboxService) MarkReadScoped(ctx context.Context, conversationID uint, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.MarkRead(ctx, conversationID)
}

// PinScoped 带归属校验的置顶
func (s *InboxService) PinScoped(ctx context.Context, conversationID uint, pinned bool, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.Pin(ctx, conversationID, pinned)
}

// StarScoped 带归属校验的星标
func (s *InboxService) StarScoped(ctx context.Context, conversationID uint, starred bool, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.Star(ctx, conversationID, starred)
}

// MuteScoped 带归属校验的静音
func (s *InboxService) MuteScoped(ctx context.Context, conversationID uint, muted bool, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.Mute(ctx, conversationID, muted)
}

// AddTagScoped 带归属校验的加标签
func (s *InboxService) AddTagScoped(ctx context.Context, conversationID uint, tag string, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.AddTag(ctx, conversationID, tag)
}

// RemoveTagScoped 带归属校验的删标签
func (s *InboxService) RemoveTagScoped(ctx context.Context, conversationID uint, tag string, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.RemoveTag(ctx, conversationID, tag)
}

// DeleteMessageScoped 带归属校验的删消息
func (s *InboxService) DeleteMessageScoped(ctx context.Context, conversationID, messageID uint, source string, sc InboxScope) error {
	if _, err := s.ensureVisible(ctx, conversationID, sc); err != nil {
		return err
	}
	return s.DeleteMessage(ctx, conversationID, messageID, source)
}
