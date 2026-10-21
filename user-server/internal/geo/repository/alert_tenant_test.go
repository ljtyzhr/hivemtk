// alert_tenant_test.go GEO 告警的租户契约。
//
// geo_alerts 是租户表：写入自动落本租户，列表/未读只含本租户，跨租户确认或删除
// 必须报不存在，而不是返回成功。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gaTenantA = uint(161)
	gaTenantB = uint(162)
)

func gaSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoAlert{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func gaSeed(t *testing.T, db *gorm.DB, tenantID uint, notified bool) *model.GeoAlert {
	t.Helper()
	row := &model.GeoAlert{
		TenantID:  tenantID,
		Type:      "negative_monitor",
		Level:     "warning",
		BrandName: "brand",
		Query:     "q",
		Notified:  notified,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 告警失败: %v", tenantID, err)
	}
	return row
}

func TestGeoAlertRepository_CreateFillsTenant(t *testing.T) {
	db := gaSetupDB(t)
	repo := &geoAlertRepo{db: db}

	row := &model.GeoAlert{Type: "negative_monitor", Level: "warning"}
	if err := repo.Create(tenant.WithTenantID(context.Background(), gaTenantA), row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != gaTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", gaTenantA, row.TenantID)
	}
}

func TestGeoAlertRepository_ListAndUnreadTenantScoped(t *testing.T) {
	db := gaSetupDB(t)
	gaSeed(t, db, gaTenantA, false)
	gaSeed(t, db, gaTenantB, false)
	gaSeed(t, db, gaTenantB, true)

	repo := &geoAlertRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gaTenantA)

	list, total, err := repo.List(ctxA, "", "", 1, 20)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(list) != 1 || list[0].TenantID != gaTenantA || total != 1 {
		t.Fatalf("租户 A 应只见自己的 1 条告警，实际 list=%+v total=%d", list, total)
	}

	n, err := repo.CountUnread(ctxA)
	if err != nil {
		t.Fatalf("CountUnread 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("租户 A 未读应为 1，实际 %d", n)
	}
}

func TestGeoAlertRepository_MarkNotifiedCrossTenantNotFound(t *testing.T) {
	db := gaSetupDB(t)
	rowB := gaSeed(t, db, gaTenantB, false)

	repo := &geoAlertRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gaTenantA)

	if err := repo.MarkNotified(ctxA, rowB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户确认应报记录不存在，实际 %v", err)
	}
	var still model.GeoAlert
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&still, rowB.ID).Error; err != nil {
		t.Fatalf("读回租户 B 告警失败: %v", err)
	}
	if still.Notified {
		t.Fatal("跨租户确认不应改动他人告警")
	}

	rowA := gaSeed(t, db, gaTenantA, false)
	if err := repo.MarkNotified(ctxA, rowA.ID); err != nil {
		t.Fatalf("本租户确认失败: %v", err)
	}
}

func TestGeoAlertRepository_DeleteCrossTenantNotFound(t *testing.T) {
	db := gaSetupDB(t)
	rowB := gaSeed(t, db, gaTenantB, false)

	repo := &geoAlertRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gaTenantA)

	if err := repo.Delete(ctxA, rowB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报记录不存在，实际 %v", err)
	}
	var cnt int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoAlert{}).Where("id = ?", rowB.ID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("跨租户删除不应删掉他人告警，实际剩余 %d 条", cnt)
	}
}
