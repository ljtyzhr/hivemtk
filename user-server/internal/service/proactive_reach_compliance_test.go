package service

// proactive_reach_compliance_test.go E1 主动触达出口（工具/API/cron 共用）的合规用例。
//
// 这一层的价值全在「出口唯一」：`enforceOutboundGates` 是三条非工具路径唯一会经过的判据点。
// 用例因此不启 DB、不装配渠道 registry——只要在**发出去之前**被拦下，
// sendXXX 一条都不会被调到（这也正是断言能成立的原因）。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// installTestCompliancePolicy 装一份策略到全局单例，并在用例结束后复位。
func installTestCompliancePolicy(t *testing.T, raw string) *ConfigCompliancePolicy {
	t.Helper()
	p := NewConfigCompliancePolicy()
	p.SetPolicySourceForTest(func(context.Context) string { return raw })
	p.SetClock(func() time.Time { return time.Date(2026, 10, 23, 21, 30, 0, 0, cstZone) })
	SetGlobalCompliancePolicyForTest(p)
	t.Cleanup(func() { SetGlobalCompliancePolicyForTest(nil) })
	return p
}

func TestEnforceCompliance_DisabledPolicyPasses(t *testing.T) {
	installTestCompliancePolicy(t, "")
	svc := NewProactiveReachService(repository.NewProactiveReachRepository(nil), repository.NewCustomerDoNotContactRepository(nil), nil)

	if err := svc.enforceCompliance(context.Background(), ReachSubject{
		Channel: "sms", Recipient: "13800138000",
	}); err != nil {
		t.Fatalf("未配置策略时必须放行，实际: %v", err)
	}
}

func TestEnforceCompliance_DeniedChannelBlocksProactiveSend(t *testing.T) {
	installTestCompliancePolicy(t, `{"enabled":true,"industries":{"medical":{
		"allowed_channels":["wecom"],"require_opt_in":true}}}`)
	svc := NewProactiveReachService(repository.NewProactiveReachRepository(nil), repository.NewCustomerDoNotContactRepository(nil), nil)

	err := svc.enforceCompliance(context.Background(), ReachSubject{
		Channel: "sms", Recipient: "13800138000", CustomerID: "c1",
		Industry: "medical",
	})
	if err == nil {
		t.Fatal("医美行业只允许企微，短信必须被拒")
	}
	if !errors.Is(err, ErrReachComplianceDenied) {
		t.Fatalf("应是 ErrReachComplianceDenied（调用方据此终止而非退避），实际 %v", err)
	}
	if !strings.Contains(err.Error(), ComplianceReasonChannelNotAllowed) {
		t.Errorf("错误串应带原因码 %q，实际 %q", ComplianceReasonChannelNotAllowed, err.Error())
	}
}

func TestEnforceCompliance_QuietHoursOnSyncPathDeniesWithHint(t *testing.T) {
	// 同步路径没有延迟队列：静默窗口必须拒（并带「本可延后到几点」），
	// 不能假装已延后——那等于静默丢消息。
	installTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{
		"quiet_hours_start":20,"quiet_hours_end":10}}}`)
	svc := NewProactiveReachService(repository.NewProactiveReachRepository(nil), repository.NewCustomerDoNotContactRepository(nil), nil)

	err := svc.enforceCompliance(context.Background(), ReachSubject{
		Channel: "email", Recipient: "a@b.com", Region: "US",
	})
	if err == nil {
		t.Fatal("静默窗口内应拒绝")
	}
	if !errors.Is(err, ErrReachComplianceDenied) {
		t.Fatalf("应是 ErrReachComplianceDenied，实际 %v", err)
	}
	if !strings.Contains(err.Error(), ComplianceReasonQuietHours) {
		t.Errorf("错误串应带 %q，实际 %q", ComplianceReasonQuietHours, err.Error())
	}
	if !strings.Contains(err.Error(), "可延后至") {
		t.Errorf("应告知可延后到的时刻，实际 %q", err.Error())
	}
}

func TestEnforceCompliance_AllowedWithOptIn(t *testing.T) {
	installTestCompliancePolicy(t, `{"enabled":true,"industries":{"medical":{"require_opt_in":true}}}`)
	svc := NewProactiveReachService(repository.NewProactiveReachRepository(nil), repository.NewCustomerDoNotContactRepository(nil), nil)

	if err := svc.enforceCompliance(context.Background(), ReachSubject{
		Channel: "wecom", Recipient: "u1", Industry: "medical", OptIn: true,
	}); err != nil {
		t.Fatalf("已取得同意应放行，实际: %v", err)
	}
	if err := svc.enforceCompliance(context.Background(), ReachSubject{
		Channel: "wecom", Recipient: "u1", Industry: "medical",
	}); err == nil {
		t.Fatal("未取得同意必须拒绝")
	}
}

// stubDoNotContactRepo 永不放行的 DNC 仓库替身：本用例只关心合规判定，
// 不希望为了「DNC 查库」去起一个测试库（默认 nil DB 会在 gorm 里 panic）。
type stubDoNotContactRepo struct{}

func (stubDoNotContactRepo) Create(context.Context, *model.CustomerDoNotContact) (bool, error) {
	return false, nil
}

func (stubDoNotContactRepo) ExistsByOneIDAndChannels(context.Context, string, []string) (bool, error) {
	return false, nil
}

func (stubDoNotContactRepo) DeleteByOneIDAndChannel(context.Context, string, string) error {
	return nil
}

func (stubDoNotContactRepo) ListByOneID(context.Context, string) ([]*model.CustomerDoNotContact, error) {
	return nil, nil
}

func newComplianceOnlyReachService() *ProactiveReachService {
	svc := NewProactiveReachService(repository.NewProactiveReachRepository(nil), repository.NewCustomerDoNotContactRepository(nil), nil)
	svc.SetDoNotContact(NewDoNotContactService(stubDoNotContactRepo{}))
	return svc
}

func TestReachByCustomer_ComplianceDeniedStopsBeforeSend(t *testing.T) {
	// 走 phone 直发分支：判定在 sendSMS 之前，因此这里不需要任何渠道 registry。
	installTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{"denied_channels":["sms"]}}}`)
	svc := newComplianceOnlyReachService()

	_, err := svc.ReachByCustomer(context.Background(), &ProactiveReachRequest{
		Phone:   "13800138000",
		Content: "hello",
		Region:  "US",
	})
	if err == nil {
		t.Fatal("策略禁止 sms 时必须在外发前拦下")
	}
	if !errors.Is(err, ErrReachComplianceDenied) {
		t.Fatalf("应是 ErrReachComplianceDenied，实际 %v", err)
	}
}

func TestReachByCustomer_ApprovalGateRunsBeforeCompliance(t *testing.T) {
	// 授权被拒时不该走到合规判定：否则一条「等白名单」的消息会被报成「合规拒绝」，
	// 运营照着改策略怎么改都不通。
	installTestCompliancePolicy(t, `{"enabled":true,"regions":{"US":{"denied_channels":["sms"]}}}`)
	svc := newComplianceOnlyReachService()
	svc.SetPreSendApprovalChecker(denyAllPreSend{})

	_, err := svc.ReachByCustomer(context.Background(), &ProactiveReachRequest{
		Phone: "13800138000", Content: "hello", Region: "US",
	})
	if !errors.Is(err, ErrReachApprovalDenied) {
		t.Fatalf("应先报授权拒绝（ErrReachApprovalDenied），实际 %v", err)
	}
}

type denyAllPreSend struct{}

func (denyAllPreSend) CheckReachPreSend(context.Context, ReachSubject) (bool, string) {
	return false, "not_whitelisted"
}
