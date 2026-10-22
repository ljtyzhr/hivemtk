// threshold_policy_tenant_test.go 阈值策略引擎的租户缓存契约（实体 #63 审计补充）。
//
// 引擎是进程级单例，策略却按租户配置。本文件锁住：
//  1. 每个租户懒加载到的都是自己的 active 策略，互不覆盖；
//  2. 多租户模式下未绑定租户的调用回退内置默认，且不会污染 0 号缓存；
//  3. 库中没有配置时沿用内置默认口径（行为与改前一致）；
//  4. 运营后台写入后 Invalidate 能立即重载新值。
package confidence

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

const (
	policyTenantA = uint(323)
	policyTenantB = uint(324)
)

func policyEngineSetup(t *testing.T) (*ThresholdPolicyEngine, *repository.ThresholdPolicyRepository) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ThresholdPolicy{})
	if db == nil {
		t.Fatal("测试库不可达：阈值策略的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	repo := repository.NewThresholdPolicyRepositoryWithDB(db)
	return NewThresholdPolicyEngine(repo), repo
}

func TestThresholdPolicyEngine_LoadsPerTenantPolicies(t *testing.T) {
	engine, repo := policyEngineSetup(t)
	ctxA := tenant.WithTenantID(context.Background(), policyTenantA)
	ctxB := tenant.WithTenantID(context.Background(), policyTenantB)

	if err := repo.SaveActive(ctxA, &model.ThresholdPolicy{
		PolicyID: "policy_purchase_a", IntentType: "purchase", BaseThreshold: 0.90, IsActive: true, Version: 1,
	}); err != nil {
		t.Fatalf("A 保存策略失败: %v", err)
	}
	if err := repo.SaveActive(ctxB, &model.ThresholdPolicy{
		PolicyID: "policy_purchase_b", IntentType: "purchase", BaseThreshold: 0.55, IsActive: true, Version: 1,
	}); err != nil {
		t.Fatalf("B 保存策略失败: %v", err)
	}

	// 先来一次无租户绑定的调用：只能拿内置默认，且不得写缓存。
	if got := engine.GetPolicy(context.Background(), "purchase").BaseThreshold; !approxEqual(got, 0.65) {
		t.Errorf("无租户绑定的调用应回退内置默认 0.65，实际 %v", got)
	}
	if got := engine.GetPolicy(ctxA, "purchase").BaseThreshold; !approxEqual(got, 0.90) {
		t.Errorf("A 应读到自己的 0.90，实际 %v", got)
	}
	if got := engine.GetPolicy(ctxB, "purchase").BaseThreshold; !approxEqual(got, 0.55) {
		t.Errorf("B 应读到自己的 0.55，实际 %v", got)
	}

	// 运营后台发布新版本后失效缓存，下一次调用立即生效。
	if err := repo.SaveActive(ctxA, &model.ThresholdPolicy{
		PolicyID: "policy_purchase_a_v2", IntentType: "purchase", BaseThreshold: 0.95, IsActive: true, Version: 2,
	}); err != nil {
		t.Fatalf("A 发布新版本失败: %v", err)
	}
	engine.Invalidate(policyTenantA)
	if got := engine.GetPolicy(ctxA, "purchase").BaseThreshold; !approxEqual(got, 0.95) {
		t.Errorf("失效缓存后 A 应读到 0.95，实际 %v", got)
	}
	if got := engine.GetPolicy(ctxB, "purchase").BaseThreshold; !approxEqual(got, 0.55) {
		t.Errorf("A 的版本变更不应影响 B，实际 %v", got)
	}
}

func TestThresholdPolicyEngine_FallsBackToBuiltinWhenNoRows(t *testing.T) {
	engine, _ := policyEngineSetup(t)
	ctxA := tenant.WithTenantID(context.Background(), policyTenantA)

	if got := engine.GetPolicy(ctxA, "complaint").BaseThreshold; !approxEqual(got, 0.85) {
		t.Errorf("库中无配置时应沿用内置 complaint=0.85，实际 %v", got)
	}
	if got := engine.GetPolicy(ctxA, "unknown_intent").BaseThreshold; !approxEqual(got, 0.70) {
		t.Errorf("未知意图应回退 default=0.70，实际 %v", got)
	}
}
