package agent_runtime

import (
	"context"
	"errors"
	"hivemtk-user/internal/pkg/tenant"
	"runtime/debug"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/event"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ── 内容审查闸（可选装配） ─────────────────
//
// 用「注册极小接口」而非直接 import contentguard：
// agent 运行时不依赖审查域的类型，连接由装配点（internal/app）负责，
// 未装配（nil）时本处是纯 no-op。

// GuardGate 内容审查闸的最小契约（实现方：contentguard 的运行时）。
type GuardGate interface {
	// Blocked 报告该主体当前是否处于生效中的处置（冻结/拉黑）。
	Blocked(ctx context.Context, subjectType, subjectID string) bool
}

// GuardSubjectUser 审查主体类型：终端用户（与 contentguard/model.SubjectUser 同值，
// 本包不 import 那个包，所以显式抄一份常量并在此注明来源）。
const GuardSubjectUser = "user"

// guardGateBox 让 atomic.Value 能承载 nil（直接 Store 一个 nil 接口会 panic）。
type guardGateBox struct{ gate GuardGate }

var guardGateHolder atomic.Value

// SetGuardGate 装配/卸载内容审查闸（传 nil = 卸载）。
func SetGuardGate(g GuardGate) { guardGateHolder.Store(guardGateBox{gate: g}) }

func currentGuardGate() GuardGate {
	if v, ok := guardGateHolder.Load().(guardGateBox); ok {
		return v.gate
	}
	return nil
}

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

	// 内容合规冻结：被处置的主体不再生成回复（这就是"拒收该条用户消息"的落地方式）。
	// 返回 nil 而不是 error：冻结是**预期状态**，不是故障，不该进错误日志、更不该触发重试。
	if gate := currentGuardGate(); gate != nil && payload.CustomerID != "" {
		if gate.Blocked(ctx, GuardSubjectUser, payload.CustomerID) {
			logger.Warnf("[agent_runtime] customer=%s 处于内容合规冻结期，本条消息不生成回复 channel=%s trace=%s",
				payload.CustomerID, payload.ChannelType, payload.TraceID)
			return nil
		}
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

// tenantCtx 从事件载荷恢复租户上下文：
// 发布点（入站解析链）已绑定租户的 payload 携带 TenantID，订阅侧据此重建；
// 0 = 历史发布方未填（等价默认租户语义，由列默认值兜底）。
func tenantCtx(ctx context.Context, payload event.CustomerMessagePayload) context.Context {
	if payload.TenantID == 0 {
		return ctx
	}
	return tenant.WithTenantID(ctx, payload.TenantID)
}
