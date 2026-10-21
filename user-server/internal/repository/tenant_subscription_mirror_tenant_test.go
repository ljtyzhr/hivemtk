// tenant_subscription_mirror_tenant_test.go 订阅镜像的租户契约。
//
// 行锚 id = tenant_id：每租户一行，读写都必须落自己的行。on 模式下
// 「无绑定且非系统域」必须 fail-closed —— 回落默认租户会让漏绑中间件的
// 租户静默读到租户 1 的订阅（续费墙永不出现）。系统作用域是后台同步的
// 唯一豁免，落默认租户。
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
	subTenantA = uint(85)
	subTenantB = uint(86)
)

func subSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.TenantSubscriptionMirror{})
	if db == nil {
		t.Fatal("测试库不可达：订阅镜像的行锚判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestTenantSubscriptionMirror_PerTenantRows(t *testing.T) {
	db := subSetupDB(t)
	repo := NewTenantSubscriptionMirrorRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), subTenantA)
	ctxB := tenant.WithTenantID(context.Background(), subTenantB)
	periodA := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	periodB := time.Now().Add(48 * time.Hour).Truncate(time.Second)

	if err := repo.Upsert(ctxA, &model.TenantSubscriptionMirror{Status: "active", CurrentPeriodEnd: &periodA}); err != nil {
		t.Fatalf("租户 A 写镜像失败: %v", err)
	}
	if err := repo.Upsert(ctxB, &model.TenantSubscriptionMirror{Status: "trial", CurrentPeriodEnd: &periodB}); err != nil {
		t.Fatalf("租户 B 写镜像失败: %v", err)
	}

	gotA, err := repo.Get(ctxA)
	if err != nil || gotA == nil {
		t.Fatalf("租户 A 读镜像失败: row=%+v err=%v", gotA, err)
	}
	if gotA.ID != subTenantA || gotA.TenantID != subTenantA || gotA.Status != "active" {
		t.Fatalf("租户 A 读到 %+v，期望 id=tenant_id=%d status=active", gotA, subTenantA)
	}
	gotB, err := repo.Get(ctxB)
	if err != nil || gotB == nil {
		t.Fatalf("租户 B 读镜像失败: row=%+v err=%v", gotB, err)
	}
	if gotB.ID != subTenantB || gotB.TenantID != subTenantB || gotB.Status != "trial" {
		t.Fatalf("租户 B 读到 %+v，期望 id=tenant_id=%d status=trial", gotB, subTenantB)
	}

	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.TenantSubscriptionMirror{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 2 {
		t.Fatalf("物理行数=%d，期望每租户一行共 2 行", rows)
	}
}

func TestTenantSubscriptionMirror_UnboundCtxIsRejected(t *testing.T) {
	db := subSetupDB(t)
	repo := NewTenantSubscriptionMirrorRepositoryWithDB(db)

	if _, err := repo.Get(context.Background()); !errors.Is(err, ErrMirrorTenantContextMissing) {
		t.Fatalf("on 模式无绑定读返回 err=%v，期望 ErrMirrorTenantContextMissing", err)
	}
	if err := repo.Upsert(context.Background(), &model.TenantSubscriptionMirror{Status: "active"}); !errors.Is(err, ErrMirrorTenantContextMissing) {
		t.Fatalf("on 模式无绑定写返回 err=%v，期望 ErrMirrorTenantContextMissing", err)
	}
	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.TenantSubscriptionMirror{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("漏绑 ctx 的写入落了 %d 行 ⇒ 静默写进了默认租户", rows)
	}
}

func TestTenantSubscriptionMirror_SystemScopeUsesDefaultTenant(t *testing.T) {
	db := subSetupDB(t)
	repo := NewTenantSubscriptionMirrorRepositoryWithDB(db)
	sysCtx := tenant.WithSystemScope(context.Background())

	if err := repo.Upsert(sysCtx, &model.TenantSubscriptionMirror{Status: "active"}); err != nil {
		t.Fatalf("系统作用域写镜像失败（后台同步路径）: %v", err)
	}
	row, err := repo.Get(sysCtx)
	if err != nil || row == nil {
		t.Fatalf("系统作用域读镜像失败: row=%+v err=%v", row, err)
	}
	if row.ID != tenant.DefaultTenantID || row.TenantID != tenant.DefaultTenantID {
		t.Fatalf("系统作用域行锚=%d tenant_id=%d，期望默认租户 %d", row.ID, row.TenantID, tenant.DefaultTenantID)
	}
}

func TestTenantSubscriptionMirror_OffModeUsesDefaultTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.TenantSubscriptionMirror{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	repo := NewTenantSubscriptionMirrorRepositoryWithDB(db)

	if err := repo.Upsert(context.Background(), &model.TenantSubscriptionMirror{Status: "trial"}); err != nil {
		t.Fatalf("off 模式写镜像失败: %v", err)
	}
	row, err := repo.Get(context.Background())
	if err != nil || row == nil {
		t.Fatalf("off 模式读镜像失败: row=%+v err=%v", row, err)
	}
	if row.TenantID != tenant.DefaultTenantID {
		t.Fatalf("off 模式 tenant_id=%d，期望默认租户 %d", row.TenantID, tenant.DefaultTenantID)
	}
}
