package router

import (
	"context"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/websocket"
)

// operationLogSink 审计 sink：经 OperationLogService 落库（Router 不直连 Repository）
type operationLogSink struct {
	svc *service.OperationLogService
}

func (s operationLogSink) Save(ctx context.Context, entry *middleware.AuditEntry) error {
	return s.svc.Create(ctx, &model.OperationLog{
		UserID:     entry.UserID,
		Username:   entry.Username,
		Action:     entry.Action,
		Module:     entry.Module,
		Resource:   entry.Resource,
		ResourceID: entry.ResourceID,
		Detail:     entry.Detail,
		IP:         entry.IP,
		UserAgent:  entry.UserAgent,
	})
}

type chatChannelResolver struct {
	svc *service.ChatChannelService
}

// resolveChannelCtx 渠道解析专用 ctx：chat_channels 是租户表，但 channel 解析
// 本身是建立租户上下文的引导步骤（channel 行自带 tenant_id），必须用 SystemScope
// 绕过隔离 callback —— 否则 on 模式下公开路由的 channel 查找会 fail-closed 401。
func resolveChannelCtx(ctx context.Context) context.Context {
	return tenant.WithSystemScope(ctx)
}

func (r chatChannelResolver) ResolveByAppKey(ctx context.Context, appKey string) (*middleware.ChatChannelView, error) {
	channel, err := r.svc.GetByAppKey(resolveChannelCtx(ctx), appKey)
	if err != nil {
		return nil, err
	}
	return toChatChannelView(channel), nil
}

func (r chatChannelResolver) ResolveByChannelID(ctx context.Context, channelID string) (*middleware.ChatChannelView, error) {
	channel, err := r.svc.GetByChannelID(resolveChannelCtx(ctx), channelID)
	if err != nil {
		return nil, err
	}
	return toChatChannelView(channel), nil
}

func toChatChannelView(c *model.ChatChannel) *middleware.ChatChannelView {
	return &middleware.ChatChannelView{
		ChannelID:      c.ChannelID,
		ChannelName:    c.ChannelName,
		Status:         string(c.Status),
		Active:         service.ChatChannelIsActive(c),
		AllowedOrigins: service.ChatChannelAllowedOriginsList(c),
		TenantID:       c.TenantID,
	}
}

// visitorChannelResolverAdapter service.ChatChannelService → websocket 窄接口适配
type visitorChannelResolverAdapter struct {
	svc *service.ChatChannelService
}

func (a visitorChannelResolverAdapter) ResolveByChannelID(ctx context.Context, channelID string) (*websocket.VisitorChannelView, error) {
	ch, err := a.svc.GetByChannelID(resolveChannelCtx(ctx), channelID)
	if err != nil || ch == nil {
		return nil, err
	}
	return &websocket.VisitorChannelView{ChannelID: ch.ChannelID, TenantID: ch.TenantID}, nil
}

func injectMiddlewarePorts() {
	websocket.SetVisitorChannelResolver(visitorChannelResolverAdapter{svc: service.MustNewChatChannelService(repository.NewChatChannelRepository())})

	middleware.SetPermChecker(service.NewPermissionService())
	middleware.SetAuditSink(operationLogSink{svc: service.NewOperationLogService()})
}
