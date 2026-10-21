package service

import (
	"context"
	"testing"
)

// TestViaPortConstructor_WiresAllRepositories 钉住 ViaPort 构造器的装配完整性。
//
// 生产事故（2026-10-21）：NewCustomerServiceAgentServiceViaPort 只填了 agentSvc，
// repo / agentRepo / agentStatusRepo 全为 nil，而 controller.NewAgentStatusController
// 走的就是这条路径。GET /api/agents/me 调 GetOrCreateAgentStatusByUserID →
// agentStatusRepo.GetByAgentID → r.db.WithContext（r.db 为 nil）⇒ nil pointer panic ⇒
// 线上 500。既有测试都用无参构造器（全量装配），照不到这条腿。
func TestViaPortConstructor_WiresAllRepositories(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewCustomerServiceAgentServiceViaPort(NewAIAgentService())

	if svc.repo == nil {
		t.Error("repo 未装配：Create/LoadAgentForSeat 会 nil pointer panic")
	}
	if svc.agentRepo == nil {
		t.Error("agentRepo 未装配：Create 校验智能体时 nil pointer panic")
	}
	if svc.agentStatusRepo == nil {
		t.Error("agentStatusRepo 未装配：GetOrCreateAgentStatusByUserID 会 nil pointer panic")
	}
	if svc.agentSvc == nil {
		t.Error("agentSvc 未装配：LoadContext 无法调用")
	}
}

// TestViaPort_GetOrCreateAgentStatusByUserID_NoPanic 走真实调用路径，
// 确认 ViaPort 构造出的服务不会 panic，并能落库 + 复用同一条座席状态。
func TestViaPort_GetOrCreateAgentStatusByUserID_NoPanic(t *testing.T) {
	setupAgentTestDB(t)
	svc := NewCustomerServiceAgentServiceViaPort(NewAIAgentService())
	ctx := context.Background()

	st, err := svc.GetOrCreateAgentStatusByUserID(ctx, 9001, "坐席甲")
	if err != nil {
		t.Fatalf("GetOrCreateAgentStatusByUserID 失败: %v", err)
	}
	if st.AgentID != 9001 {
		t.Errorf("AgentID 应为 9001, 实际 %d", st.AgentID)
	}

	st2, err := svc.GetOrCreateAgentStatusByUserID(ctx, 9001, "坐席甲")
	if err != nil {
		t.Fatalf("第二次查询失败: %v", err)
	}
	if st2.ID != st.ID {
		t.Errorf("应命中同一条座席状态, 期望 ID=%d, 实际 ID=%d", st.ID, st2.ID)
	}
}
