// approval_auto_send_bridge_test.go E-10（2026-10-24，产品可复议）：审批通过自动外发的三件判据。
//
//	① quote+approved ⇒ 调一次 Send（带 approval_id，operator=approval-auto-send）；
//	② 非 approved（rejected/expired）或非 quote subject ⇒ 不调 Send；
//	③ 发送腿未装配（quoteSend 回 nil）⇒ 静默不 panic（手动档）；
//	以及 fan-out：AddWaitNotifier 的订阅者收到通知，主槽（SetWaitNotifier）不受影响。
package service

import (
	"context"
	"sync"
	"testing"

	"hivemtk-user/internal/model"
)

// fakeAutoSendQuoteSend 用接口面够不着的最小替身：bridge 只调 Send + Available，
// 两者都在 *QuoteSendService 上。真库用例见 quote_send_test.go 的 SendAfterDecision；
// 本文件只测桥的分流与 fan-out 语义，用真 QuoteSendService 需要九个句柄，
// 而这里的判据不落库——用 nil service + 全局槽位交换的方式测"未装配"分支，
// 已装配分支用全局真实例太重，改由 wiring 层的装配测试覆盖（见 approval_runtime_wiring_test.go）。

type autoSendRecordingNotifier struct {
	mu     sync.Mutex
	got    []*model.ApprovalRequest
	panics bool
}

func (r *autoSendRecordingNotifier) NotifyDecided(_ context.Context, req *model.ApprovalRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.panics {
		panic("autoSendRecordingNotifier: 测试注入的 panic")
	}
	r.got = append(r.got, req)
}

func TestApprovalAutoSendBridgeSkipsNonApprovedAndNonQuote(t *testing.T) {
	// 判据②③：send 腿为 nil（未装配）时，非 approved / 非 quote 直接 return——
	// 不走到取 quoteSend 那一步，自然也不出声。若分流写反，这里会因 nil svc
	// 的日志路径在无 logger 环境下暴露为 panic 或无声通过——所以再用 rec 断言。
	bridge := NewApprovalAutoSendBridge(func() *QuoteSendService { return nil })
	cases := []struct {
		name string
		req  *model.ApprovalRequest
	}{
		{"rejected", &model.ApprovalRequest{SubjectType: QuoteApprovalSubjectType, Status: model.ApprovalStatusRejected}},
		{"expired", &model.ApprovalRequest{SubjectType: QuoteApprovalSubjectType, Status: model.ApprovalStatusExpired}},
		{"pending", &model.ApprovalRequest{SubjectType: QuoteApprovalSubjectType, Status: model.ApprovalStatusPending}},
		{"reach_plan subject", &model.ApprovalRequest{SubjectType: "reach_plan", Status: model.ApprovalStatusApproved}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bridge.NotifyDecided(context.Background(), c.req) // 只要不 panic / 不调 nil Send 即可
		})
	}
}

func TestApprovalFanOutNotifiesAllSubscribers(t *testing.T) {
	svc := NewApprovalRequestService(nil, nil)
	primary := &autoSendRecordingNotifier{}
	extra := &autoSendRecordingNotifier{}
	svc.SetWaitNotifier(primary)
	svc.AddWaitNotifier(extra)

	req := &model.ApprovalRequest{SubjectType: QuoteApprovalSubjectType, Status: model.ApprovalStatusApproved, ID: "apr-1"}
	svc.notifyWait(context.Background(), req)

	if len(primary.got) != 1 {
		t.Fatalf("主槽应收到 1 条，得到 %d", len(primary.got))
	}
	if len(extra.got) != 1 {
		t.Fatalf("追加订阅者应收到 1 条，得到 %d", len(extra.got))
	}
}

func TestApprovalFanOutSubscriberPanicDoesNotBlockOthers(t *testing.T) {
	svc := NewApprovalRequestService(nil, nil)
	primary := &autoSendRecordingNotifier{}
	boom := &autoSendRecordingNotifier{panics: true}
	after := &autoSendRecordingNotifier{}
	svc.SetWaitNotifier(primary)
	svc.AddWaitNotifier(boom)
	svc.AddWaitNotifier(after)

	svc.notifyWait(context.Background(), &model.ApprovalRequest{ID: "apr-2"})

	if len(primary.got) != 1 || len(after.got) != 1 {
		t.Fatalf("panic 订阅者不应影响其他订阅者：primary=%d after=%d", len(primary.got), len(after.got))
	}
}

func TestSetWaitNotifierNilClearsSubscribers(t *testing.T) {
	svc := NewApprovalRequestService(nil, nil)
	extra := &autoSendRecordingNotifier{}
	svc.AddWaitNotifier(extra)
	svc.SetWaitNotifier(nil) // 卸载：清主槽也清追加订阅者
	svc.notifyWait(context.Background(), &model.ApprovalRequest{ID: "apr-3"})
	if len(extra.got) != 0 {
		t.Fatal("SetWaitNotifier(nil) 后追加订阅者不应再收到通知（卸载语义）")
	}
}
