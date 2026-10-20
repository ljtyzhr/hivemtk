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
		TenantID:   entry.TenantID,
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

// resolveChannelCtx 渠道解析用的作用域选择。
//
// 未绑定租户时走 SystemScope：channel 行本身就是租户线索，公开路由此刻还没有上下文，
// 不加系统作用域的话 on 模式下连查都查不到（隔离 callback 对无绑定 ctx fail-closed）。
//
// 已绑定时**保持绑定**：访客链按 tenant_code 定过租户、或上游已经由别的渠道行自证过，
// 此时再退到 SystemScope 等于在跨租户 union 里重新挑一行——同名的 default/卡片渠道
// 会挑到别的租户，于是"上下文说 A、渠道行是 B"。绑定态的读取天然限定在本租户。
func resolveChannelCtx(ctx context.Context) context.Context {
	if tid, bound := tenant.FromContext(ctx); bound && tid != 0 {
		return ctx
	}
	return tenant.WithSystemScope(ctx)
}

// tenantCodeResolver 装配层适配器：把租户短码解析交给中间件窄接口。
//
// 三处公开入口共用（访客 HTTP 链、访客 WS、MCP 单值凭证）——它们是同一件事：
// 调用方在没有登录态时自己声明"我属于哪个租户"。
func tenantCodeResolver(ctx context.Context, code string) (uint, error) {
	return service.NewTenantService().ResolvePublicTenantID(ctx, code)
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
	if err != nil {
		// ref 槽位可能放的是 app_key（SDK/安装引导以 appKey 作 channel_ref）：
		// 与中间件 AppKeyResolve、service.resolveChannel 同一语义——先按
		// channel_id 列解析，失败再按 app_key 列回退。
		if ch2, err2 := a.svc.GetByAppKey(resolveChannelCtx(ctx), channelID); err2 == nil {
			ch = ch2
		} else {
			return nil, err
		}
	}
	if ch == nil {
		return nil, nil
	}
	return &websocket.VisitorChannelView{ChannelID: ch.ChannelID, TenantID: ch.TenantID}, nil
}

func injectMiddlewarePorts() {
	websocket.SetVisitorChannelResolver(visitorChannelResolverAdapter{svc: service.MustNewChatChannelService(repository.NewChatChannelRepository())})

	middleware.SetPermChecker(service.NewPermissionService())
	middleware.SetAuditSink(operationLogSink{svc: service.NewOperationLogService()})
}
