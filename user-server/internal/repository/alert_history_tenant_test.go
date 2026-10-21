// alert_history_tenant_test.go 告警历史的租户契约。
//
// 读写都经 tenant callback：创建自动落本租户、列表/未恢复统计只含本租户、
// 跨租户恢复与通知回写不得命中他人行。
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
	ahTenantA = uint(151)
	ahTenantB = uint(152)
)

func ahSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AlertHistory{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func ahSeed(t *testing.T, db *gorm.DB, tenantID uint, ruleID uint, status string) *model.AlertHistory {
	t.Helper()
	row := &model.AlertHistory{
		TenantID:    tenantID,
		RuleID:      ruleID,
		RuleName:    "rule",
		Source:      "metrics.test",
		Severity:    model.AlertSeverityWarning,
		Status:      status,
		TriggeredAt: time.Now().Add(-time.Minute),
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 告警历史失败: %v", tenantID, err)
	}
	return row
}

func TestAlertHistoryRepository_CreateFillsTenant(t *testing.T) {
	db := ahSetupDB(t)
	repo := &alertHistoryRepo{db: db}

	row := &model.AlertHistory{
		RuleID:      1,
		RuleName:    "rule",
		Source:      "metrics.test",
		Severity:    model.AlertSeverityWarning,
		Status:      model.AlertHistoryFiring,
		TriggeredAt: time.Now(),
	}
	if err := repo.Create(tenant.WithTenantID(context.Background(), ahTenantA), row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != ahTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", ahTenantA, row.TenantID)
	}
}

func TestAlertHistoryRepository_ListAndFiringTenantScoped(t *testing.T) {
	db := ahSetupDB(t)
	ahSeed(t, db, ahTenantA, 7, model.AlertHistoryFiring)
	ahSeed(t, db, ahTenantB, 7, model.AlertHistoryFiring)
	ahSeed(t, db, ahTenantB, 7, model.AlertHistoryResolved)

	repo := &alertHistoryRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), ahTenantA)

	list, total, err := repo.List(ctxA, 1, 20, 7, "")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != ahTenantA || total != 1 {
		t.Fatalf("租户 A 应只见自己的 1 条历史，实际 list=%+v total=%d", list, total)
	}

	firing, err := repo.ListFiring(ctxA, 20)
	if err != nil {
		t.Fatalf("ListFiring 失败: %v", err)
	}
	if len(firing) != 1 || firing[0].TenantID != ahTenantA {
		t.Fatalf("ListFiring 跨租户泄漏: %+v", firing)
	}

	count, err := repo.CountFiring(ctxA)
	if err != nil {
		t.Fatalf("CountFiring 失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("CountFiring 期望 1，实际 %d", count)
	}
}

func TestAlertHistoryRepository_CrossTenantResolveBlocked(t *testing.T) {
	db := ahSetupDB(t)
	seedB := ahSeed(t, db, ahTenantB, 9, model.AlertHistoryFiring)
	repo := &alertHistoryRepo{db: db}

	// 恢复是幂等动作：命中 0 行不报错，但绝不能改到别的租户
	if err := repo.ResolveFiring(tenant.WithTenantID(context.Background(), ahTenantA), 9, time.Now()); err != nil {
		t.Fatalf("跨租户 ResolveFiring 不应报错（幂等语义）: %v", err)
	}
	var after model.AlertHistory
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 历史失败: %v", err)
	}
	if after.Status != model.AlertHistoryFiring {
		t.Fatalf("租户 B 的告警被跨租户恢复: status=%s", after.Status)
	}

	if err := repo.ResolveFiring(tenant.WithTenantID(context.Background(), ahTenantB), 9, time.Now()); err != nil {
		t.Fatalf("本租户 ResolveFiring 失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 历史失败: %v", err)
	}
	if after.Status != model.AlertHistoryResolved {
		t.Fatalf("本租户恢复未落库: status=%s", after.Status)
	}
}

func TestAlertHistoryRepository_UpdateNotifyTenantScoped(t *testing.T) {
	db := ahSetupDB(t)
	seedB := ahSeed(t, db, ahTenantB, 11, model.AlertHistoryFiring)
	repo := &alertHistoryRepo{db: db}

	err := repo.UpdateNotify(tenant.WithTenantID(context.Background(), ahTenantA), seedB.ID, "ok")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 UpdateNotify 期望 ErrRecordNotFound，实际: %v", err)
	}
	var after model.AlertHistory
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 历史失败: %v", err)
	}
	if after.NotifyResult != "" {
		t.Fatalf("租户 B 的通知结果被跨租户改写: %q", after.NotifyResult)
	}

	if err := repo.UpdateNotify(tenant.WithTenantID(context.Background(), ahTenantB), seedB.ID, "ok"); err != nil {
		t.Fatalf("本租户 UpdateNotify 失败: %v", err)
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 历史失败: %v", err)
	}
	if after.NotifyResult != "ok" {
		t.Fatalf("本租户通知结果未落库: %q", after.NotifyResult)
	}
}
