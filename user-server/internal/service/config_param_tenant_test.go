package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// setupTenantParamFixture 建四张表并返回服务。
// 注册表由 testutil 内省装配（tenant_config_params 有 tenant_id 列即被登记）。
func setupTenantParamFixture(t *testing.T) (*ConfigParamService, *gorm.DB) {
	t.Helper()
	db := testutil.NewTestDB(t,
		&model.ConfigParam{},
		&model.ConfigParamAuditLog{},
		&model.TenantConfigParam{},
		&model.TenantConfigParamAuditLog{},
	)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	return NewConfigParamService(), db
}

func seedBaseParam(t *testing.T, db *gorm.DB, group, key, def string) {
	t.Helper()
	if err := db.Create(&model.ConfigParam{
		Group:        group,
		Key:          key,
		Name:         key,
		Description:  key,
		ValueType:    "int",
		Value:        def,
		DefaultValue: def,
	}).Error; err != nil {
		t.Fatalf("seed 平台默认值 %s.%s 失败: %v", group, key, err)
	}
}

func tenantCtx(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}

// 核心隔离：A 的改动不影响 B，B 未覆盖时读平台默认值。
func TestConfigParamTenantIsolation(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "max_items", "10")

	ctxA, ctxB := tenantCtx(41), tenantCtx(42)

	if got := svc.GetInt(ctxA, "limits", "max_items", -1); got != 10 {
		t.Fatalf("A 未覆盖时应读平台默认 10，实际 %d", got)
	}
	if err := svc.UpdateValue(ctxA, "limits", "max_items", "20", 7); err != nil {
		t.Fatalf("A 更新失败: %v", err)
	}
	if got := svc.GetInt(ctxA, "limits", "max_items", -1); got != 20 {
		t.Fatalf("A 应读到自己的 20，实际 %d", got)
	}
	if got := svc.GetInt(ctxB, "limits", "max_items", -1); got != 10 {
		t.Fatalf("B 不得读到 A 的覆盖值，期望 10，实际 %d", got)
	}

	var base model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", "limits", "max_items").First(&base).Error; err != nil {
		t.Fatal(err)
	}
	if base.Value != "10" {
		t.Fatalf("租户写入不得改平台默认值，实际 %s", base.Value)
	}

	var override model.TenantConfigParam
	if err := db.WithContext(ctxA).Where("tenant_id = ? AND param_group = ? AND key = ?", 41, "limits", "max_items").
		First(&override).Error; err != nil {
		t.Fatalf("A 的覆盖行应落库: %v", err)
	}
	if override.Value != "20" || override.UpdatedBy != 7 {
		t.Fatalf("覆盖行内容不对: %+v", override)
	}

	// 管理端列表：A 看到生效值 20，B 看到默认值 10
	listA, err := svc.ListByGroup(ctxA, "limits", false)
	if err != nil || len(listA) != 1 || listA[0].Value != "20" {
		t.Fatalf("A 列表应为 20: %v %+v", err, listA)
	}
	listB, err := svc.ListByGroup(ctxB, "limits", false)
	if err != nil || len(listB) != 1 || listB[0].Value != "10" {
		t.Fatalf("B 列表应为 10: %v %+v", err, listB)
	}

	// 审计按租户隔离
	logsA, err := svc.AuditLogs(ctxA, 10)
	if err != nil || len(logsA) != 1 || logsA[0].NewValue != "20" {
		t.Fatalf("A 审计应只有自己的一条: %v %+v", err, logsA)
	}
	logsB, err := svc.AuditLogs(ctxB, 10)
	if err != nil || len(logsB) != 0 {
		t.Fatalf("B 不应看到 A 的审计: %v %+v", err, logsB)
	}
}

// 缓存按租户分域：先读 A 再读 B，不能命中 A 的缓存。
func TestConfigParamTenantCacheIsolation(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "batch", "5")

	ctxA, ctxB := tenantCtx(51), tenantCtx(52)
	if err := svc.UpdateValue(ctxA, "limits", "batch", "9", 1); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetInt(ctxA, "limits", "batch", -1); got != 9 {
		t.Fatalf("A 期望 9，实际 %d", got)
	}
	if got := svc.GetInt(ctxB, "limits", "batch", -1); got != 5 {
		t.Fatalf("B 命中 A 的缓存（串租户），期望 5，实际 %d", got)
	}
	// 再读 A：缓存仍应返回 9
	if got := svc.GetInt(ctxA, "limits", "batch", -1); got != 9 {
		t.Fatalf("A 二次读取期望 9，实际 %d", got)
	}
}

// 重置只删本租户覆盖行，B 的覆盖值不受影响。
func TestConfigParamTenantResetAndBulk(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "a", "1")
	seedBaseParam(t, db, "limits", "b", "2")

	ctxA, ctxB := tenantCtx(61), tenantCtx(62)
	for _, ctx := range []context.Context{ctxA, ctxB} {
		if err := svc.UpdateValue(ctx, "limits", "a", "10", 1); err != nil {
			t.Fatal(err)
		}
		if err := svc.UpdateValue(ctx, "limits", "b", "20", 1); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.ResetToDefault(ctxA, "limits", "a", 2); err != nil {
		t.Fatalf("A 重置失败: %v", err)
	}
	if got := svc.GetInt(ctxA, "limits", "a", -1); got != 1 {
		t.Fatalf("A 重置后应回默认 1，实际 %d", got)
	}
	if got := svc.GetInt(ctxB, "limits", "a", -1); got != 10 {
		t.Fatalf("A 重置不得影响 B，期望 10，实际 %d", got)
	}

	if err := svc.BulkResetGroup(ctxA, "limits", 3); err != nil {
		t.Fatalf("A 整组重置失败: %v", err)
	}
	if got := svc.GetInt(ctxA, "limits", "b", -1); got != 2 {
		t.Fatalf("A 整组重置后 b 应回默认 2，实际 %d", got)
	}
	if got := svc.GetInt(ctxB, "limits", "b", -1); got != 20 {
		t.Fatalf("A 整组重置不得影响 B，期望 20，实际 %d", got)
	}

	var cnt int64
	if err := db.WithContext(ctxA).Model(&model.TenantConfigParam{}).Where("tenant_id = ?", 61).Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("A 重置后不应残留覆盖行，实际 %d", cnt)
	}
}

// on 模式无租户上下文：读允许回落平台默认值，写必须拒绝。
func TestConfigParamTenantMissingContextFailsClosed(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "x", "3")

	ctx := context.Background()
	if got := svc.GetInt(ctx, "limits", "x", -1); got != 3 {
		t.Fatalf("无租户读取应回落平台默认 3，实际 %d", got)
	}
	if err := svc.UpdateValue(ctx, "limits", "x", "4", 1); !errors.Is(err, repository.ErrTenantParamContextMissing) {
		t.Fatalf("无租户写入必须 fail-closed，实际 err=%v", err)
	}
	var base model.ConfigParam
	if err := db.Where("param_group = ? AND key = ?", "limits", "x").First(&base).Error; err != nil {
		t.Fatal(err)
	}
	if base.Value != "3" {
		t.Fatalf("无租户写入不得改平台默认值，实际 %s", base.Value)
	}
}

// off 模式保持历史语义：写直接落平台行，不产生租户覆盖。
func TestConfigParamOffModeWritesBaseRow(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	svc, db := setupTenantParamFixture(t)
	seedBaseParam(t, db, "limits", "legacy", "1")

	if err := svc.UpdateValue(context.Background(), "limits", "legacy", "2", 9); err != nil {
		t.Fatalf("off 模式更新失败: %v", err)
	}
	if got := svc.GetInt(context.Background(), "limits", "legacy", -1); got != 2 {
		t.Fatalf("off 模式应读到 2，实际 %d", got)
	}
	var cnt int64
	if err := db.Model(&model.TenantConfigParam{}).Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("off 模式不得写租户覆盖表，实际 %d 行", cnt)
	}
}
