// guard_gate_test.go 内容审查闸接入 agent 入站链路的断言。
//
// 这条闸门是"冻结"这件事**唯一能被用户感知到**的地方：没有它，冻结只是库里
// 一行 Active=true，用户照旧收到回复，等于罚了等于没罚。
package agent_runtime

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/event"
)

type fakeGate struct {
	blocked map[string]bool
	calls   []string
	subject []string
}

func (f *fakeGate) Blocked(_ context.Context, subjectType, subjectID string) bool {
	f.calls = append(f.calls, subjectType+"|"+subjectID)
	f.subject = append(f.subject, subjectType)
	return f.blocked[subjectID]
}

func TestEventSubscriber_GuardGateBlocksFrozenCustomer(t *testing.T) {
	defer SetGuardGate(nil)

	mock := &mockRuntime{}
	gate := &fakeGate{blocked: map[string]bool{"cust_bad": true}}
	SetGuardGate(gate)

	handler := NewEventSubscriber(mock)
	err := handler(event.Event{
		Topic: event.TopicCustomerMessageReceived,
		Payload: event.CustomerMessagePayload{
			ChannelType: "telegram", AccountID: "tg_001",
			CustomerID: "cust_bad", Content: "违规内容", TraceID: "trace_gate_1",
			Timestamp: time.Now(),
		},
	})

	// 冻结是预期状态：不报错（否则会被当成故障重试）。
	if err != nil {
		t.Fatalf("冻结跳过不应返回错误：%v", err)
	}
	if mock.callCount() != 0 {
		t.Fatalf("被冻结用户的消息不得生成回复，实际处理 %d 次", mock.callCount())
	}
	if len(gate.calls) != 1 || gate.calls[0] != GuardSubjectUser+"|cust_bad" {
		t.Fatalf("闸门调用 = %v，期望 [user|cust_bad]", gate.calls)
	}
}

func TestEventSubscriber_GuardGateAllowsNormalCustomer(t *testing.T) {
	defer SetGuardGate(nil)

	mock := &mockRuntime{}
	gate := &fakeGate{blocked: map[string]bool{"cust_bad": true}}
	SetGuardGate(gate)

	handler := NewEventSubscriber(mock)
	if err := handler(event.Event{
		Topic: event.TopicCustomerMessageReceived,
		Payload: event.CustomerMessagePayload{
			ChannelType: "telegram", AccountID: "tg_001",
			CustomerID: "cust_ok", Content: "你好", TraceID: "trace_gate_2",
			Timestamp: time.Now(),
		},
	}); err != nil {
		t.Fatalf("Handle 报错：%v", err)
	}
	if mock.callCount() != 1 {
		t.Fatalf("未冻结用户应正常处理，实际 %d 次", mock.callCount())
	}
	if len(gate.calls) != 1 {
		t.Fatalf("闸门应被查询 1 次，实际 %v", gate.calls)
	}
}

func TestEventSubscriber_NoGateIsNoOp(t *testing.T) {
	SetGuardGate(nil)

	mock := &mockRuntime{}
	handler := NewEventSubscriber(mock)
	if err := handler(event.Event{
		Topic: event.TopicCustomerMessageReceived,
		Payload: event.CustomerMessagePayload{
			ChannelType: "wecom", AccountID: "wc_1",
			CustomerID: "cust_1", Content: "hi", TraceID: "trace_gate_3",
		},
	}); err != nil {
		t.Fatalf("Handle 报错：%v", err)
	}
	// 未装配闸门时行为必须与既有完全一致（老部署不受影响）。
	if mock.callCount() != 1 {
		t.Fatalf("未装配闸门时应照常处理，实际 %d 次", mock.callCount())
	}
}

func TestEventSubscriber_GuardGateSkipsEmptyCustomerID(t *testing.T) {
	defer SetGuardGate(nil)

	mock := &mockRuntime{}
	gate := &fakeGate{blocked: map[string]bool{"": true}}
	SetGuardGate(gate)

	handler := NewEventSubscriber(mock)
	if err := handler(event.Event{
		Topic: event.TopicCustomerMessageReceived,
		Payload: event.CustomerMessagePayload{
			ChannelType: "wecom", AccountID: "wc_1",
			Content: "hi", TraceID: "trace_gate_4",
		},
	}); err != nil {
		t.Fatalf("Handle 报错：%v", err)
	}
	// 没有 customer_id 时不能拿空串去查（空主体在处置表里没有语义）。
	if len(gate.calls) != 0 {
		t.Fatalf("空主体不应查闸门，实际 %v", gate.calls)
	}
	if mock.callCount() != 1 {
		t.Fatalf("应正常处理，实际 %d 次", mock.callCount())
	}
}
