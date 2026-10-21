// automation_rule_tenant_test.go 自动化规则的租户契约。
//
// 规则表没有租户级唯一键，同名规则允许并存；真正要钉住的是：
// 列表/按事件调度只能看见本租户的规则，运行次数回写碰不到别人的行，
// 延迟执行记录也必须由 ctx 盖章。
package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	aruleTenantA = uint(201)
	aruleTenantB = uint(202)
	aruleEvent   = "message_inbound"
)

func aruleSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AutomationRule{}, &model.RulePendingExecution{})
	if db == nil {
		t.Fatal("测试库不可达：规则租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func aruleSeed(t *testing.T, db *gorm.DB, tenantID uint, name string, enabled bool) *model.AutomationRule {
	t.Helper()
	row := &model.AutomationRule{
		TenantID: tenantID,
		Name:     name,
		Event:    aruleEvent,
		Actions:  `[{"type":"add_tag","value":"vip"}]`,
		Enabled:  enabled,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 规则失败: %v", tenantID, err)
	}
	// GORM 对带 default 标签的零值字段会落 DB 默认值（enabled 默认 true），
	// 需要停用的夹具必须显式回写，否则"停用"规则其实仍是启用态。
	if !enabled {
		if err := db.WithContext(tenant.WithSystemScope(context.Background())).
			Model(row).UpdateColumn("enabled", false).Error; err != nil {
			t.Fatalf("停用夹具失败: %v", err)
		}
		row.Enabled = false
	}
	return row
}

// TestAutomationRuleRepo_ListAndDispatchIsolated 列表与调度入口都只看见本租户的规则。
func TestAutomationRuleRepo_ListAndDispatchIsolated(t *testing.T) {
	db := aruleSetupDB(t)
	repo := NewAutomationRuleRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), aruleTenantA)
	ctxB := tenant.WithTenantID(context.Background(), aruleTenantB)

	aruleSeed(t, db, aruleTenantA, "A-启用", true)
	aruleSeed(t, db, aruleTenantA, "A-停用", false)
	aruleSeed(t, db, aruleTenantB, "B-启用", true)

	listA, err := repo.ListRules(ctxA, aruleEvent)
	if err != nil {
		t.Fatalf("租户 A ListRules: %v", err)
	}
	if len(listA) != 2 {
		t.Fatalf("租户 A 规则数 = %d, want 2（看到了别人的行或漏了自己的）", len(listA))
	}
	for _, r := range listA {
		if r.TenantID != aruleTenantA {
			t.Fatalf("租户 A 读到租户 %d 的规则: %+v", r.TenantID, r)
		}
	}

	enabledA, err := repo.ListEnabledRulesByEvent(ctxA, aruleEvent)
	if err != nil {
		t.Fatalf("租户 A ListEnabledRulesByEvent: %v", err)
	}
	if len(enabledA) != 1 || enabledA[0].Name != "A-启用" {
		t.Fatalf("调度入口返回 = %+v, want 仅 A-启用", enabledA)
	}

	enabledB, err := repo.ListEnabledRulesByEvent(ctxB, aruleEvent)
	if err != nil {
		t.Fatalf("租户 B ListEnabledRulesByEvent: %v", err)
	}
	if len(enabledB) != 1 || enabledB[0].Name != "B-启用" {
		t.Fatalf("租户 B 调度入口返回 = %+v, want 仅 B-启用", enabledB)
	}
}

// TestAutomationRuleRepo_RunCountCrossTenantNoop 用别的租户的 ctx 回写运行次数，
// 不得改到原行。
func TestAutomationRuleRepo_RunCountCrossTenantNoop(t *testing.T) {
	db := aruleSetupDB(t)
	repo := NewAutomationRuleRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), aruleTenantA)
	ctxB := tenant.WithTenantID(context.Background(), aruleTenantB)

	ruleA := aruleSeed(t, db, aruleTenantA, "A-规则", true)
	_ = repo.IncrementRuleRunCount(ctxA, ruleA)

	// 跨租户再自增一次：即使仓储不校验命中行数，也不能改到 A 的行。
	_ = repo.IncrementRuleRunCount(ctxB, ruleA)

	got, err := repo.GetRuleByID(ctxA, ruleA.ID)
	if err != nil {
		t.Fatalf("租户 A 回查: %v", err)
	}
	if got.RunCount != 1 {
		t.Errorf("RunCount = %d, want 1（跨租户回写改到了行）", got.RunCount)
	}
}

// TestAutomationRuleRepo_PendingExecutionStampedAndIsolated 延迟执行行由 ctx 盖章，
// 到期扫描按租户隔离，跨租户状态回写不生效。
func TestAutomationRuleRepo_PendingExecutionStampedAndIsolated(t *testing.T) {
	db := aruleSetupDB(t)
	repo := NewAutomationRuleRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), aruleTenantA)
	ctxB := tenant.WithTenantID(context.Background(), aruleTenantB)

	ruleA := aruleSeed(t, db, aruleTenantA, "A-延迟规则", true)
	pending := &model.RulePendingExecution{
		RuleID:    ruleA.ID,
		SessionID: "arule_sess_1",
		ExecuteAt: time.Now().Add(-time.Minute),
	}
	if err := repo.CreatePendingRuleExecution(ctxA, pending); err != nil {
		t.Fatalf("写入延迟执行: %v", err)
	}
	if pending.TenantID != aruleTenantA {
		t.Fatalf("TenantID = %d, want %d（ctx 未盖章）", pending.TenantID, aruleTenantA)
	}

	dueA, err := repo.ListDuePendingExecutions(ctxA, time.Now())
	if err != nil {
		t.Fatalf("租户 A 到期扫描: %v", err)
	}
	if len(dueA) != 1 {
		t.Fatalf("租户 A 到期记录 = %d, want 1", len(dueA))
	}
	dueB, err := repo.ListDuePendingExecutions(ctxB, time.Now())
	if err != nil {
		t.Fatalf("租户 B 到期扫描: %v", err)
	}
	if len(dueB) != 0 {
		t.Fatalf("租户 B 看到 %d 条别人的到期记录", len(dueB))
	}

	_ = repo.UpdatePendingExecutionStatus(ctxB, pending.ID, "done")
	var got model.RulePendingExecution
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		First(&got, pending.ID).Error; err != nil {
		t.Fatalf("回查延迟执行: %v", err)
	}
	if got.Status != "pending" {
		t.Errorf("跨租户状态回写生效了: status=%s", got.Status)
	}
}
