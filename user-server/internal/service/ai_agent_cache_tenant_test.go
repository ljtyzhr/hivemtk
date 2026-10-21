// ai_agent_cache_tenant_test.go 智能体上下文缓存的跨租户回归。
//
// 缓存命中会绕过仓储层按租户过滤的 GetByID：若缓存键只有 agentID，
// B 租户拿 A 的 agent ID 就能直接读到 A 的人设/系统提示词/LLM 配置。
// 键必须带租户，且未命中时仍回到租户过滤的 DB 读。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	cacheTenantA = uint(111)
	cacheTenantB = uint(112)
)

func TestAIAgentService_LoadContextCacheTenantIsolated(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AIAgent{})
	if db == nil {
		t.Fatal("测试库不可达：缓存越权判不了（不 Skip，跳过等于没锁）")
	}

	seed := func(tenantID uint, code string) *model.AIAgent {
		row := &model.AIAgent{
			TenantID:  tenantID,
			AgentCode: code,
			Name:      code,
			Persona:   "persona-" + code,
			AgentType: string(model.AgentTypeSales),
			Status:    1,
		}
		if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
			t.Fatalf("预置租户 %d 智能体失败: %v", tenantID, err)
		}
		return row
	}
	agentA := seed(cacheTenantA, "cache-a")
	agentB := seed(cacheTenantB, "cache-b")

	svc := NewAIAgentService()
	ctxA := tenant.WithTenantID(context.Background(), cacheTenantA)
	ctxB := tenant.WithTenantID(context.Background(), cacheTenantB)

	first, err := svc.LoadContext(ctxA, agentA.ID)
	if err != nil || first == nil {
		t.Fatalf("租户 A 首次加载失败: err=%v ctx=%v", err, first)
	}
	second, err := svc.LoadContext(ctxA, agentA.ID)
	if err != nil || second == nil {
		t.Fatalf("租户 A 缓存加载失败: err=%v ctx=%v", err, second)
	}
	if first != second {
		t.Fatal("同租户二次加载应命中缓存")
	}

	leaked, err := svc.LoadContext(ctxB, agentA.ID)
	if err != nil {
		t.Fatalf("租户 B 读取 A 的 ID 不应报错（应视为不存在）: %v", err)
	}
	if leaked != nil {
		t.Fatalf("跨租户缓存泄漏：租户 B 读到了 A 的智能体配置 code=%s", leaked.AgentCode)
	}

	own, err := svc.LoadContext(ctxB, agentB.ID)
	if err != nil || own == nil {
		t.Fatalf("租户 B 加载自己的智能体失败: err=%v ctx=%v", err, own)
	}
	if own.AgentCode != "cache-b" {
		t.Fatalf("租户 B 加载到了错误配置: %s", own.AgentCode)
	}
}
