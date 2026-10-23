package service

// reach_send_pipeline_compliance_policy_test.go E1 同步发送管线的合规步骤用例。
//
// 这一层要钉的是「接进管线之后的行为」而不是判定逻辑本身（判定逻辑在
// reach_compliance_policy_test.go）：步骤顺序、nil 判定器零影响、拒绝后不再往渠道发、
// 延后走既有延迟队列、以及日上限只在成功后计数。

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingComplianceEvaluator 可编排的判定器替身：把 Evaluate/RecordSent 的调用都记下来。
type recordingComplianceEvaluator struct {
	decision     ComplianceDecision
	mu           sync.Mutex
	evalCount    int
	recordedSent []*ReachSendRequest
}

func (e *recordingComplianceEvaluator) Evaluate(_ context.Context, _ *ReachSendRequest) ComplianceDecision {
	e.mu.Lock()
	e.evalCount++
	e.mu.Unlock()
	return e.decision
}

func (e *recordingComplianceEvaluator) RecordSent(_ context.Context, req *ReachSendRequest) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.recordedSent = append(e.recordedSent, req)
}

func (e *recordingComplianceEvaluator) sentCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.recordedSent)
}

// recordingDeferrer 记录延迟入队（不真的起后台循环）。
type recordingDeferrer struct {
	mu    sync.Mutex
	calls []time.Time
	fail  bool
}

func (d *recordingDeferrer) Defer(_ context.Context, _ *ReachSendRequest, sendAt time.Time) error {
	if d.fail {
		return errors.New("deferrer unavailable")
	}
	d.mu.Lock()
	d.calls = append(d.calls, sendAt)
	d.mu.Unlock()
	return nil
}

func (d *recordingDeferrer) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.calls)
}

func compliancePipeline(adapter ChannelAdapter, ev CompliancePolicyEvaluator, deferrer SendQuietHoursDeferrer) SendPipeline {
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.CompliancePolicy = ev
	cfg.QuietHoursEnabled = true
	if deferrer != nil {
		cfg.QuietHoursDeferrer = deferrer
	}
	// 固定时钟：避免用例在 22:00–8:00 之间跑时被全局静默时段抢先延后
	// （那是 permission 步骤的行为，不属于本用例的判定面）。
	cfg.QuietHoursClock = func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, cstZone) }
	return NewSendPipeline(cfg)
}

func TestSendPipeline_ComplianceStepInDefaultSteps(t *testing.T) {
	// 步骤必须在默认序列里：否则「生产没挂上」会表现为静默不判，
	// 而 DefaultSendPipelineSteps 是装配层唯一的事实源。
	found := false
	for _, s := range DefaultSendPipelineSteps {
		if s == SendStepCompliancePolicy {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("默认步骤必须包含 %s，实际 %v", SendStepCompliancePolicy, DefaultSendPipelineSteps)
	}
	// 位置：permission 之后、rate_limit 之前（见常量注释里的理由）。
	idx := func(step string) int {
		for i, s := range DefaultSendPipelineSteps {
			if s == step {
				return i
			}
		}
		return -1
	}
	if !(idx(SendStepPermission) < idx(SendStepCompliancePolicy) && idx(SendStepCompliancePolicy) < idx(SendStepRateLimit)) {
		t.Fatalf("步骤顺序应为 permission < compliance_policy < rate_limit，实际 %v", DefaultSendPipelineSteps)
	}
}

func TestSendPipeline_NilComplianceEvaluatorIsZeroImpact(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	pipeline := newTestPipeline(adapter) // 默认配置：CompliancePolicy == nil

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "sms", RecipientID: "13800138000", Content: "hello",
	})
	if !resp.Success {
		t.Fatalf("未装配判定器时必须照常发送，实际失败: %s", resp.Error)
	}
	log := findStepResult(resp, SendStepCompliancePolicy)
	if log == nil {
		t.Fatal("步骤应出现在 StepResults 里（记 skipped），而不是整步消失")
	}
	if !log.Skipped || !log.Success {
		t.Errorf("未装配判定器时该步应为 skipped+success，实际 %+v", log)
	}
}

func TestSendPipeline_ComplianceDenyStopsBeforeChannel(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	ev := &recordingComplianceEvaluator{decision: ComplianceDecision{
		Allowed: false, Reason: ComplianceReasonChannelNotAllowed,
		Region: "US", Industry: "medical", PolicyScope: "industry:medical",
	}}
	pipeline := compliancePipeline(adapter, ev, nil)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "sms", RecipientID: "13800138000", CustomerID: "c1", Content: "hello",
	})
	if resp.Success {
		t.Fatal("被策略拒绝时必须判失败")
	}
	if !strings.Contains(resp.Error, ErrSendComplianceDenied.Error()) {
		t.Errorf("错误串应含 %q，实际 %q", ErrSendComplianceDenied.Error(), resp.Error)
	}
	if !strings.Contains(resp.Error, ComplianceReasonChannelNotAllowed) {
		t.Errorf("错误串应含原因码 %q（否则运维只能猜），实际 %q", ComplianceReasonChannelNotAllowed, resp.Error)
	}
	// 拒绝发生在渠道之前：retry/send 不该出现。
	if stepExists(resp, SendStepSend) {
		t.Error("被拒后不得再执行 send 步骤")
	}
	if ev.sentCount() != 0 {
		t.Errorf("被拒不计数，实际记了 %d 次", ev.sentCount())
	}
}

func TestSendPipeline_ComplianceDeferUsesQuietHoursQueue(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	sendAt := time.Date(2026, 10, 24, 10, 0, 0, 0, cstZone)
	ev := &recordingComplianceEvaluator{decision: ComplianceDecision{
		Allowed: true, Deferred: true, SendAt: sendAt, Reason: ComplianceReasonQuietHours, Region: "US",
	}}
	deferrer := &recordingDeferrer{}
	pipeline := compliancePipeline(adapter, ev, deferrer)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "email", RecipientID: "a@b.com", CustomerID: "c1", Content: "hello",
	})
	if !resp.Success {
		t.Fatalf("延后不是失败（消息进了队列），实际: %s", resp.Error)
	}
	if !resp.Deferred {
		t.Fatal("应标记 deferred")
	}
	if !resp.DeferredAt.Equal(sendAt) {
		t.Errorf("deferred_at 应为 %s，实际 %s", sendAt, resp.DeferredAt)
	}
	if deferrer.callCount() != 1 {
		t.Errorf("应入延迟队列 1 次，实际 %d", deferrer.callCount())
	}
	if stepExists(resp, SendStepSend) {
		t.Error("延后后不得再执行 send 步骤")
	}
	if ev.sentCount() != 0 {
		t.Errorf("延后不计数，实际 %d", ev.sentCount())
	}
}

func TestSendPipeline_ComplianceDeferFailureIsDeny(t *testing.T) {
	// 延迟队列不可用时把消息当「已延后」= 静默丢消息。必须判失败。
	adapter := newSuccessAdapter("msg")
	ev := &recordingComplianceEvaluator{decision: ComplianceDecision{
		Allowed: true, Deferred: true, SendAt: time.Now().Add(time.Hour), Reason: ComplianceReasonQuietHours,
	}}
	pipeline := compliancePipeline(adapter, ev, &recordingDeferrer{fail: true})

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "email", RecipientID: "a@b.com", Content: "hello",
	})
	if resp.Success {
		t.Fatal("入队失败必须判失败，不许回执成功")
	}
	if !strings.Contains(resp.Error, ErrSendComplianceDenied.Error()) {
		t.Errorf("错误串应指明是合规步骤失败，实际 %q", resp.Error)
	}
}

func TestSendPipeline_ComplianceAllowRecordsSent(t *testing.T) {
	adapter := newSuccessAdapter("msg")
	ev := &recordingComplianceEvaluator{decision: ComplianceDecision{Allowed: true, Reason: ComplianceReasonAllowed, Region: "CN"}}
	pipeline := compliancePipeline(adapter, ev, nil)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "sms", RecipientID: "13800138000", CustomerID: "c1", Content: "hello",
	})
	if !resp.Success {
		t.Fatalf("放行时应发送成功: %s", resp.Error)
	}
	if ev.sentCount() != 1 {
		t.Fatalf("成功外发后应计数 1 次，实际 %d", ev.sentCount())
	}
	log := findStepResult(resp, SendStepCompliancePolicy)
	if log == nil || !log.Success || log.Skipped {
		t.Fatalf("放行时该步应为 success 且非 skipped，实际 %+v", log)
	}
	// Output 要带区域/原因，D2 可观测性从这里取数。
	if len(log.Output) == 0 {
		t.Fatal("放行也应输出 region/reason，便于回答「这次按哪条规则放行的」")
	}
}

func TestSendPipeline_ComplianceStepRespectsCustomSteps(t *testing.T) {
	// 自定义 Steps 不含该步骤时不该执行（营运侧手工编排的老流水线不受新判据影响）。
	adapter := newSuccessAdapter("msg")
	ev := &recordingComplianceEvaluator{decision: ComplianceDecision{Allowed: false, Reason: ComplianceReasonChannelDenied}}
	cfg := DefaultSendPipelineConfig(adapter)
	cfg.CompliancePolicy = ev
	cfg.Steps = []string{SendStepPermission, SendStepRetry, SendStepSend}
	pipeline := NewSendPipeline(cfg)

	resp := pipeline.Send(context.Background(), &ReachSendRequest{
		Channel: "sms", RecipientID: "13800138000", Content: "hello",
	})
	if !resp.Success {
		t.Fatalf("未编排该步骤时应照常发送: %s", resp.Error)
	}
	if ev.evalCount != 0 {
		t.Errorf("未编排该步骤时不该调用判定器，实际 %d 次", ev.evalCount)
	}
}
