package agent_runtime

import (
	"context"
	"errors"
	"hivemtk-user/internal/pkg/tenant"
	"runtime/debug"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/utils/logger"
)

type eventBusEventSubscriber struct {
	runtime AgentRuntime
}

// NewEventSubscriber 创建事件订阅者
//
// 调用方:cmd/api/main.go 启动阶段
// 注册方式:
//
//	bus := event.GetGlobalBus()
//	bus.Subscribe(event.TopicCustomerMessageReceived, agent_runtime.NewEventSubscriber(rt))
func NewEventSubscriber(rt AgentRuntime) event.Handler {
	return (&eventBusEventSubscriber{runtime: rt}).Handle
}

func (s *eventBusEventSubscriber) Handle(evt event.Event) error {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[agent_runtime] panic in event handler topic=%s err=%v\n%s",
				evt.Topic, r, debug.Stack())
		}
	}()

	payload, ok := evt.Payload.(event.CustomerMessagePayload)
	if !ok {
		if p, ok := evt.Payload.(*event.CustomerMessagePayload); ok {
			if p == nil {
				return errors.New("event payload is nil")
			}
			return s.handleMessage(tenantCtx(context.Background(), *p), *p)
		}
		return errors.New("invalid event payload type for " + evt.Topic)
	}

	return s.handleMessage(tenantCtx(context.Background(), payload), payload)
}

func (s *eventBusEventSubscriber) handleMessage(ctx context.Context, payload event.CustomerMessagePayload) error {
	if HasReplied(payload.TraceID) {
		logger.Infof("[agent_runtime] event=%s already replied by sync path, skip bus handling", payload.TraceID)
		return nil
	}

	start := time.Now()

	if payload.TraceID == "" {
		payload.TraceID = "agent_runtime_" + start.Format("20060102150405.000000")
	}

	rtPayload := convertFromEventPayload(payload)

	resp, err := s.runtime.HandleCustomerMessage(ctx, rtPayload)
	if err != nil {
		logger.Errorf("[agent_runtime] handle failed channel=%s account=%s customer=%s trace=%s err=%v",
			payload.ChannelType, payload.AccountID, payload.CustomerID, payload.TraceID, err)
		return err
	}

	duration := time.Since(start)
	if resp != nil {
		logger.Infof("[agent_runtime] handled channel=%s account=%s customer=%s trace=%s agent=%s confidence=%.2f tools=%v handoff=%v duration=%s",
			payload.ChannelType, payload.AccountID, payload.CustomerID, payload.TraceID,
			resp.AgentCode, resp.Confidence, resp.ToolsCalled, resp.HandoffToHuman, duration)
	}

	return nil
}

func convertFromEventPayload(ep event.CustomerMessagePayload) CustomerMessagePayload {
	return CustomerMessagePayload{
		ChannelType: ep.ChannelType,
		AccountID:   ep.AccountID,
		CustomerID:  ep.CustomerID,
		SessionID:   ep.SessionID,
		Content:     ep.Content,
		MessageType: ep.MessageType,
		Timestamp:   ep.Timestamp,
		TraceID:     ep.TraceID,
		Raw:         ep.Raw,
	}
}

// tenantCtx 从事件载荷恢复租户上下文（specs/001-saas-multi-tenant T021 行自证）：
// 发布点（入站解析链）已绑定租户的 payload 携带 TenantID，订阅侧据此重建；
// 0 = 历史发布方未填（off 模式等价默认租户语义，由列默认值兜底）。
func tenantCtx(ctx context.Context, payload event.CustomerMessagePayload) context.Context {
	if payload.TenantID == 0 {
		return ctx
	}
	return tenant.WithTenantID(ctx, payload.TenantID)
}
