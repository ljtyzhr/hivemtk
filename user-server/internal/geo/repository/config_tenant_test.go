// config_tenant_test.go GEO 配置 / 平台账号 / 发布记录的租户契约。
//
// 三张表都是租户表：一租户一份配置、账号与发布记录只在本租户可见；
// 跨租户删除必须报不存在，更新不得复活已删行。
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
	gcfgTenantA = uint(181)
	gcfgTenantB = uint(182)
)

func gcfgSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoConfig{}, &model.GeoPlatformAccount{}, &model.GeoPublishRecord{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func gcfgSeedConfig(t *testing.T, db *gorm.DB, tenantID uint, brand string) *model.GeoConfig {
	t.Helper()
	row := &model.GeoConfig{TenantID: tenantID, BrandName: brand, Language: "zh"}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 配置失败: %v", tenantID, err)
	}
	return row
}

func TestGeoConfigRepository_OneRowPerTenant(t *testing.T) {
	db := gcfgSetupDB(t)
	gcfgSeedConfig(t, db, gcfgTenantA, "brand-a")
	gcfgSeedConfig(t, db, gcfgTenantB, "brand-b")

	repo := &geoConfigRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcfgTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gcfgTenantB)

	cfgA, err := repo.Get(ctxA)
	if err != nil {
		t.Fatalf("租户 A Get 失败: %v", err)
	}
	if cfgA.BrandName != "brand-a" || cfgA.TenantID != gcfgTenantA {
		t.Fatalf("租户 A 读到了别人的配置: %+v", cfgA)
	}

	cfgA.BrandName = "brand-a2"
	if err := repo.Update(ctxA, cfgA); err != nil {
		t.Fatalf("租户 A Update 失败: %v", err)
	}

	cfgB, err := repo.Get(ctxB)
	if err != nil {
		t.Fatalf("租户 B Get 失败: %v", err)
	}
	if cfgB.BrandName != "brand-b" {
		t.Fatalf("租户 A 的更新串到了租户 B: %+v", cfgB)
	}
}

func TestGeoPlatformAccountRepository_TenantScoped(t *testing.T) {
	db := gcfgSetupDB(t)
	repo := &geoPlatformAccountRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcfgTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gcfgTenantB)

	accA := &model.GeoPlatformAccount{Platform: "github_readme", AccountName: "a", Status: "active"}
	if err := repo.Create(ctxA, accA); err != nil {
		t.Fatalf("租户 A 创建账号失败: %v", err)
	}
	accB := &model.GeoPlatformAccount{Platform: "github_readme", AccountName: "b", Status: "active"}
	if err := repo.Create(ctxB, accB); err != nil {
		t.Fatalf("租户 B 创建账号失败: %v", err)
	}
	if accA.TenantID != gcfgTenantA || accB.TenantID != gcfgTenantB {
		t.Fatalf("创建未填租户: A=%d B=%d", accA.TenantID, accB.TenantID)
	}

	list, total, err := repo.GetList(ctxA, "", 1, 20)
	if err != nil {
		t.Fatalf("GetList 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].TenantID != gcfgTenantA {
		t.Fatalf("租户 A 账号列表越界: total=%d list=%+v", total, list)
	}

	// 同名账号在不同租户互不覆盖
	if acc, err := repo.GetByPlatformAndName(ctxB, "github_readme", "b"); err != nil || acc.ID != accB.ID {
		t.Fatalf("租户 B 未读到自己账号: %+v err=%v", acc, err)
	}

	if err := repo.Delete(ctxA, accB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除账号应报不存在，实际 %v", err)
	}
	var cnt int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoPlatformAccount{}).
		Where("id = ?", accB.ID).Count(&cnt)
	if cnt != 1 {
		t.Fatalf("跨租户删除不应删掉他人账号，实际剩余 %d", cnt)
	}
}

func TestGeoPlatformAccountRepository_UpdateDeletedNotResurrected(t *testing.T) {
	db := gcfgSetupDB(t)
	repo := &geoPlatformAccountRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcfgTenantA)

	acc := &model.GeoPlatformAccount{Platform: "wordpress", AccountName: "a", Status: "active"}
	if err := repo.Create(ctxA, acc); err != nil {
		t.Fatalf("创建账号失败: %v", err)
	}
	if err := repo.Delete(ctxA, acc.ID); err != nil {
		t.Fatalf("删除账号失败: %v", err)
	}

	acc.Status = "disabled"
	if err := repo.Update(ctxA, acc); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新已删除账号应报不存在，实际 %v", err)
	}
	var active int64
	db.WithContext(tenant.WithSystemScope(context.Background())).Model(&model.GeoPlatformAccount{}).
		Where("id = ?", acc.ID).Count(&active)
	if active != 0 {
		t.Fatalf("已删除账号被更新复活（活动行 %d）", active)
	}
}

func TestGeoPublishRecordRepository_TenantScoped(t *testing.T) {
	db := gcfgSetupDB(t)
	repo := &geoPublishRecordRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gcfgTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gcfgTenantB)

	if err := repo.Create(ctxA, &model.GeoPublishRecord{ArticleID: "art-a", Platform: "github"}); err != nil {
		t.Fatalf("租户 A 写发布记录失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.GeoPublishRecord{ArticleID: "art-b", Platform: "github"}); err != nil {
		t.Fatalf("租户 B 写发布记录失败: %v", err)
	}

	list, total, err := repo.GetList(ctxA, "", "", 1, 20)
	if err != nil {
		t.Fatalf("GetList 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].ArticleID != "art-a" {
		t.Fatalf("租户 A 发布记录越界: total=%d list=%+v", total, list)
	}
}
