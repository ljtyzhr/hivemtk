// lead_mining_config_tenant_test.go 线索发掘配置（#111）的租户契约与写口语义。
//
// 原先的实现把单例行主键**写死成 1**，而本表有 tenant_id：租户 2 第一次保存时
// INSERT 的 id=1 已被租户 1 占着 ⇒ 主键冲突、接口 500，也就是"多租户下第二个租户
// 永远存不上线索发掘配置"。同时 Save 是"先 First 再 Create/Updates"：并发保存两边
// 都能通过预检，后落的那条撞主键；Updates 分支命中 0 行也只体现在 RowsAffected 上。
//
// 这里钉死三条：各租户各一行且互不覆盖、重复保存走 upsert 不撞主键、跨租户读不到。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

// TestLeadMiningConfig_SingletonPerTenant 两个租户各自保存/读取单例配置，互不覆盖。
func TestLeadMiningConfig_SingletonPerTenant(t *testing.T) {
	b1SetupDB(t, &model.LeadMiningConfig{})
	repo := NewLeadMiningConfigRepository()

	ctxA, ctxB := b1Ctx(b1TenantA), b1Ctx(b1TenantB)

	if err := repo.Save(ctxA, &model.LeadMiningConfig{
		Enabled: true, MinIntentScore: 70, Keywords: model.JSONStrings{"采购"},
	}); err != nil {
		t.Fatalf("租户 A 保存失败: %v", err)
	}
	if err := repo.Save(ctxB, &model.LeadMiningConfig{
		Enabled: false, MinIntentScore: 30,
	}); err != nil {
		t.Fatalf("租户 B 保存失败（若报主键冲突，说明单例行 id 又被写死成 1 了）: %v", err)
	}

	gotA, err := repo.GetSingleton(ctxA)
	if err != nil {
		t.Fatalf("租户 A 读取失败: %v", err)
	}
	if !gotA.Enabled || gotA.MinIntentScore != 70 {
		t.Errorf("租户 A 读到的不是自己那行: enabled=%v min=%d", gotA.Enabled, gotA.MinIntentScore)
	}

	gotB, err := repo.GetSingleton(ctxB)
	if err != nil {
		t.Fatalf("租户 B 读取失败: %v", err)
	}
	if gotB.Enabled || gotB.MinIntentScore != 30 {
		t.Errorf("租户 B 读到的不是自己那行: enabled=%v min=%d", gotB.Enabled, gotB.MinIntentScore)
	}
	if gotA.ID == gotB.ID {
		t.Errorf("两个租户的单例行主键相同（%d）：多租户下必然互相覆盖", gotA.ID)
	}
}

// TestLeadMiningConfig_SaveTwiceIsUpsert 连续保存走 ON CONFLICT DO UPDATE：
// 不新增行、字段按第二次的值落库。
func TestLeadMiningConfig_SaveTwiceIsUpsert(t *testing.T) {
	b1SetupDB(t, &model.LeadMiningConfig{})
	repo := NewLeadMiningConfigRepository()
	ctx := b1Ctx(b1TenantA)

	if err := repo.Save(ctx, &model.LeadMiningConfig{Enabled: true, MinIntentScore: 60, Requirement: "首版"}); err != nil {
		t.Fatalf("首次保存失败: %v", err)
	}
	if err := repo.Save(ctx, &model.LeadMiningConfig{Enabled: false, MinIntentScore: 20, Requirement: "改版"}); err != nil {
		t.Fatalf("二次保存失败: %v", err)
	}

	got, err := repo.GetSingleton(ctx)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.Enabled || got.MinIntentScore != 20 || got.Requirement != "改版" {
		t.Errorf("upsert 没把第二次的值写进去: %+v", got)
	}
}

// TestLeadMiningConfig_SystemScopeDefaultsToRowOne 系统域（不绑租户的运维/启动路径）
// 仍落在 id=1，与历史单租户行完全兼容。
func TestLeadMiningConfig_SystemScopeDefaultsToRowOne(t *testing.T) {
	b1SetupDB(t, &model.LeadMiningConfig{})
	repo := NewLeadMiningConfigRepository()

	got, err := repo.GetSingleton(tenant.WithSystemScope(context.Background()))
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if got.ID != 1 {
		t.Errorf("未绑定租户时应回落到 id=1，实际 %d", got.ID)
	}
	if got.MinIntentScore != 50 {
		t.Errorf("缺省配置 min_intent_score 应为 50，实际 %d", got.MinIntentScore)
	}
}
