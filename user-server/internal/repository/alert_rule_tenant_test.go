// alert_rule_tenant_test.go 告警规则的租户契约。
//
// 规则名唯一索引必须复合化到 (tenant_id, name)：两个租户可各自拥有同名规则；
// 更新/批量启停/触发时间回写都不得命中他人租户的行。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	arTenantA = uint(161)
	arTenantB = uint(162)
)

func arSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AlertRule{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func arSeed(t *testing.T, db *gorm.DB, tenantID uint, name string) *model.AlertRule {
	t.Helper()
	row := &model.AlertRule{
		TenantID:        tenantID,
		Name:            name,
		Source:          "metrics.test",
		Operator:        "gt",
		Threshold:       1,
		WindowSeconds:   60,
		CooldownSeconds: 300,
		Severity:        model.AlertSeverityWarning,
		Channels:        "[]",
		Targets:         "{}",
		Enabled:         true,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 告警规则失败: %v", tenantID, err)
	}
	return row
}

func TestAlertRuleRepository_CreateFillsTenantAndNameUniquePerTenant(t *testing.T) {
	db := arSetupDB(t)
	repo := &alertRuleRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), arTenantA)
	ctxB := tenant.WithTenantID(context.Background(), arTenantB)

	mk := func() *model.AlertRule {
		return &model.AlertRule{
			Name:          "same-name-rule",
			Source:        "metrics.test",
			Operator:      "gt",
			Threshold:     1,
			WindowSeconds: 60,
			Severity:      model.AlertSeverityWarning,
			Channels:      "[]",
			Targets:       "{}",
			Enabled:       true,
		}
	}
	a, b := mk(), mk()
	if err := repo.Create(ctxA, a); err != nil {
		t.Fatalf("租户 A 创建失败: %v", err)
	}
	if a.TenantID != arTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", arTenantA, a.TenantID)
	}
	// 唯一索引若仍是 name 单列，这里会撞 A 的行
	if err := repo.Create(ctxB, b); err != nil {
		t.Fatalf("两个租户同名规则应各自可建（唯一索引未复合化）: %v", err)
	}
	if b.TenantID != arTenantB {
		t.Errorf("期望 tenant_id=%d，实际 %d", arTenantB, b.TenantID)
	}
}

func TestAlertRuleRepository_ListTenantScoped(t *testing.T) {
	db := arSetupDB(t)
	arSeed(t, db, arTenantA, "a-rule")
	arSeed(t, db, arTenantB, "b-rule-1")
	arSeed(t, db, arTenantB, "b-rule-2")
	repo := &alertRuleRepo{db: db}

	list, total, err := repo.List(tenant.WithTenantID(context.Background(), arTenantA), 1, 20, false)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].TenantID != arTenantA {
		t.Fatalf("租户 A 应只见自己的 1 条规则，实际 list=%+v total=%d", list, total)
	}

	enabled, err := repo.ListEnabled(tenant.WithTenantID(context.Background(), arTenantB))
	if err != nil {
		t.Fatalf("ListEnabled 失败: %v", err)
	}
	if len(enabled) != 2 {
		t.Fatalf("租户 B 应有 2 条启用规则，实际 %d", len(enabled))
	}
	for _, r := range enabled {
		if r.TenantID != arTenantB {
			t.Fatalf("ListEnabled 跨租户泄漏: %+v", r)
		}
	}
}

func TestAlertRuleRepository_UpdateCrossTenantBlocked(t *testing.T) {
	db := arSetupDB(t)
	seedB := arSeed(t, db, arTenantB, "b-rule")
	repo := &alertRuleRepo{db: db}

	seedB.Threshold = 99
	seedB.Name = "hijacked"
	err := repo.Update(tenant.WithTenantID(context.Background(), arTenantA), seedB)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 期望 ErrRecordNotFound，实际: %v", err)
	}
	var after model.AlertRule
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 规则失败: %v", err)
	}
	if after.TenantID != arTenantB || after.Name != "b-rule" || after.Threshold != 1 {
		t.Fatalf("租户 B 规则被跨租户改写: %+v", after)
	}

	after.Threshold = 5
	if err := repo.Update(tenant.WithTenantID(context.Background(), arTenantB), &after); err != nil {
		t.Fatalf("本租户 Update 失败: %v", err)
	}
	var own model.AlertRule
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&own, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 规则失败: %v", err)
	}
	if own.Threshold != 5 || own.TenantID != arTenantB {
		t.Fatalf("本租户更新未落库或租户被改: %+v", own)
	}
}

func TestAlertRuleRepository_BatchStatusAndLastTriggeredTenantScoped(t *testing.T) {
	db := arSetupDB(t)
	seedB := arSeed(t, db, arTenantB, "b-rule")
	repo := &alertRuleRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), arTenantA)
	ctxB := tenant.WithTenantID(context.Background(), arTenantB)

	if err := repo.BatchUpdateStatus(ctxA, []uint{seedB.ID}, false); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 BatchUpdateStatus 期望 ErrRecordNotFound，实际: %v", err)
	}
	var after model.AlertRule
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 规则失败: %v", err)
	}
	if !after.Enabled {
		t.Fatal("租户 B 的规则被跨租户禁用")
	}
	if err := repo.BatchUpdateStatus(ctxB, []uint{seedB.ID}, false); err != nil {
		t.Fatalf("本租户 BatchUpdateStatus 失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 规则失败: %v", err)
	}
	if after.Enabled {
		t.Fatal("本租户禁用未落库")
	}

	if err := repo.UpdateLastTriggered(ctxA, seedB.ID, time.Now()); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 UpdateLastTriggered 期望 ErrRecordNotFound，实际: %v", err)
	}
	if err := repo.UpdateLastTriggered(ctxB, seedB.ID, time.Now()); err != nil {
		t.Fatalf("本租户 UpdateLastTriggered 失败: %v", err)
	}
}
