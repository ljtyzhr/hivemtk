package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
)

// 纯函数级数据隔离判定测试（不依赖 DB）。
func TestSessionVisible_Scope(t *testing.T) {
	conv := func(agentID uint) *model.CustomerSession {
		return &model.CustomerSession{AgentID: agentID}
	}
	cases := []struct {
		name  string
		conv  *model.CustomerSession
		scope SessionScope
		want  bool
	}{
		{"admin 不限制", conv(9), SessionScope{Role: "admin", UserID: 1}, true},
		{"空 role 不限制", conv(9), SessionScope{Role: "", UserID: 1}, true},
		{"非 admin 看自己名下", conv(5), SessionScope{Role: "customer_service", UserID: 5}, true},
		{"非 admin 看未分配池", conv(0), SessionScope{Role: "staff", UserID: 5}, true},
		{"非 admin 不可见他人名下", conv(9), SessionScope{Role: "customer_service", UserID: 5}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionVisible(tc.conv, tc.scope); got != tc.want {
				t.Errorf("sessionVisible = %v, want %v", got, tc.want)
			}
		})
	}
}

// Scoped 详情读口：他人名下 → ErrSessionForbidden。
func TestCustomerSessionService_GetSessionByIDScoped_Forbidden(t *testing.T) {
	svc := setupCustomerSessionService(t)

	created, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform:  model.PlatformDouyin,
		AccountID: "acct_scope",
		UserID:    "user_scope",
	})
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	if err := svc.sessionRepo.AssignAgent(context.Background(), created.ID, 9, "other"); err != nil {
		t.Fatalf("AssignAgent failed: %v", err)
	}

	// 归属 9 的会话：身份 5 不可见
	_, err = svc.GetSessionByIDScoped(context.Background(), created.ID, SessionScope{Role: "customer_service", UserID: 5})
	if !errors.Is(err, ErrSessionForbidden) {
		t.Fatalf("expected ErrSessionForbidden, got %v", err)
	}

	// 归属本人可见
	if _, err := svc.GetSessionByIDScoped(context.Background(), created.ID, SessionScope{Role: "customer_service", UserID: 9}); err != nil {
		t.Fatalf("self scope should see own session, got %v", err)
	}

	// admin 不受限
	if _, err := svc.GetSessionByIDScoped(context.Background(), created.ID, SessionScope{Role: "admin", UserID: 1}); err != nil {
		t.Fatalf("admin should see all, got %v", err)
	}
}

// Scoped 列表：非 admin 只回「自己名下 + 未分配池」。
func TestCustomerSessionService_GetSessionsScoped_Filter(t *testing.T) {
	svc := setupCustomerSessionService(t)

	mine, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformDouyin, AccountID: "acct_a", UserID: "u_mine",
	})
	if err != nil {
		t.Fatalf("CreateSession mine: %v", err)
	}
	other, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformDouyin, AccountID: "acct_b", UserID: "u_other",
	})
	if err != nil {
		t.Fatalf("CreateSession other: %v", err)
	}
	unassigned, err := svc.CreateSession(context.Background(), &CreateSessionRequest{
		Platform: model.PlatformDouyin, AccountID: "acct_c", UserID: "u_pool",
	})
	if err != nil {
		t.Fatalf("CreateSession pool: %v", err)
	}
	if err := svc.sessionRepo.AssignAgent(context.Background(), mine.ID, 5, "me"); err != nil {
		t.Fatalf("assign mine: %v", err)
	}
	if err := svc.sessionRepo.AssignAgent(context.Background(), other.ID, 9, "other"); err != nil {
		t.Fatalf("assign other: %v", err)
	}
	_ = unassigned // 保持未分配

	sessions, total, err := svc.GetSessionsScoped(context.Background(), "", 1, 100, SessionScope{Role: "customer_service", UserID: 5})
	if err != nil {
		t.Fatalf("GetSessionsScoped: %v", err)
	}
	ids := map[uint]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids[mine.ID] || !ids[unassigned.ID] {
		t.Errorf("expected mine+pool, got ids=%v total=%d", ids, total)
	}
	if ids[other.ID] {
		t.Errorf("other's session must be hidden, got ids=%v", ids)
	}

	// admin 全量
	_, totalAdmin, err := svc.GetSessionsScoped(context.Background(), "", 1, 100, SessionScope{Role: "admin", UserID: 1})
	if err != nil {
		t.Fatalf("admin scoped list: %v", err)
	}
	if totalAdmin != 3 {
		t.Errorf("admin total = %d, want 3", totalAdmin)
	}
}
