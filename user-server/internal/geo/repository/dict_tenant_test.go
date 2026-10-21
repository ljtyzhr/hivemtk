// dict_tenant_test.go GEO 配置字典的租户契约。
//
// geo_dicts 是租户表：同一 (category, key) 在不同租户下必须互不可见，
// 写入必须显式落 tenant_id（Upsert 走 map 形态 Create，隔离 callback 填不进去），
// 跨租户删除必须报不存在而不是静默成功。
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
	gdtTenantA = uint(261)
	gdtTenantB = uint(262)
)

func gdtSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoDict{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoDictRepository_UpsertFillsTenantAndIsolates(t *testing.T) {
	db := gdtSetupDB(t)
	repo := &geoDictRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gdtTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gdtTenantB)

	if err := repo.Upsert(ctxA, &model.GeoDict{Category: "prompt_tpl", Key: "k", Value: "A", Active: true}); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if err := repo.Upsert(ctxB, &model.GeoDict{Category: "prompt_tpl", Key: "k", Value: "B", Active: true}); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	// 同名 (category, key) 必须各存一行，而不是被唯一索引撞成一行。
	var rows []model.GeoDict
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("category = ? AND \"key\" = ?", "prompt_tpl", "k").
		Find(&rows).Error; err != nil {
		t.Fatalf("回查字典行失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("跨租户同键应共存 2 行，实际 %d 行", len(rows))
	}
	tenantByValue := map[string]uint{}
	for _, row := range rows {
		tenantByValue[row.Value] = row.TenantID
	}
	if tenantByValue["A"] != gdtTenantA || tenantByValue["B"] != gdtTenantB {
		t.Fatalf("Upsert 未把 ctx 租户写进 tenant_id: %+v", tenantByValue)
	}

	gotA, err := repo.GetAny(ctxA, "prompt_tpl", "k")
	if err != nil || gotA.Value != "A" {
		t.Fatalf("租户 A 读到别人的值: %+v err=%v", gotA, err)
	}
	gotB, err := repo.GetAny(ctxB, "prompt_tpl", "k")
	if err != nil || gotB.Value != "B" {
		t.Fatalf("租户 B 读到别人的值: %+v err=%v", gotB, err)
	}

	listA, err := repo.ListByCategory(ctxA, "prompt_tpl")
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if len(listA) != 1 || listA[0].Value != "A" {
		t.Fatalf("租户 A 列表越界: %+v", listA)
	}
}

func TestGeoDictRepository_UpsertRequiresTenantInMultiTenantMode(t *testing.T) {
	db := gdtSetupDB(t)
	repo := &geoDictRepo{db: db}

	err := repo.Upsert(context.Background(), &model.GeoDict{Category: "prompt_tpl", Key: "no_tenant", Value: "x"})
	if err == nil {
		t.Fatal("on 模式下缺租户上下文的写入必须被拒绝")
	}
}

func TestGeoDictRepository_DeleteCrossTenantNotFound(t *testing.T) {
	db := gdtSetupDB(t)
	repo := &geoDictRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), gdtTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gdtTenantB)

	if err := repo.Upsert(ctxB, &model.GeoDict{Category: "prompt_tpl", Key: "b_only", Value: "B", Active: true}); err != nil {
		t.Fatalf("预置租户 B 失败: %v", err)
	}

	if err := repo.Delete(ctxA, "prompt_tpl", "b_only"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报不存在，实际 err=%v", err)
	}
	if _, err := repo.GetAny(ctxB, "prompt_tpl", "b_only"); err != nil {
		t.Fatalf("跨租户删除不应真的删掉 B 的行: %v", err)
	}
}
