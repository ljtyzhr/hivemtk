// schema_template_tenant_test.go Schema 模板库的租户契约。
//
// geo_schema_templates 是租户表：列表只含本租户；跨租户读/改/删必须报不存在。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	gstTenantA = uint(369)
	gstTenantB = uint(370)
)

func gstSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoSchemaTemplate{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoSchemaTemplateRepository_TenantScoped(t *testing.T) {
	db := gstSetupDB(t)
	repo := NewGeoSchemaTemplateRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gstTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gstTenantB)
	sysCtx := tenant.WithSystemScope(context.Background())

	tplA := &model.GeoSchemaTemplate{
		PageType:     "article",
		SchemaType:   "Article",
		TemplateJSON: datatypes.JSON(`{"@type":"Article"}`),
		Active:       true,
	}
	if err := repo.Create(ctxA, tplA); err != nil {
		t.Fatalf("租户 A 创建失败: %v", err)
	}
	if tplA.TenantID != gstTenantA {
		t.Fatalf("Create 未填租户: %d", tplA.TenantID)
	}
	if err := repo.Create(ctxB, &model.GeoSchemaTemplate{
		PageType: "faq", SchemaType: "FAQPage", TemplateJSON: datatypes.JSON(`{"@type":"FAQPage"}`),
	}); err != nil {
		t.Fatalf("租户 B 创建失败: %v", err)
	}

	listA, totalA, err := repo.List(ctxA, 1, 20)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if totalA != 1 || len(listA) != 1 || listA[0].TenantID != gstTenantA {
		t.Fatalf("租户 A 列表越界: total=%d list=%+v", totalA, listA)
	}

	if _, err := repo.GetByID(ctxB, tplA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByID 应报不存在，实际 err=%v", err)
	}

	hijack := &model.GeoSchemaTemplate{
		ID: tplA.ID, PageType: "hijack", SchemaType: "Hijack",
		TemplateJSON: datatypes.JSON(`{}`), Active: false,
	}
	if err := repo.Update(ctxB, hijack); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报不存在，实际 err=%v", err)
	}
	var after model.GeoSchemaTemplate
	if err := db.WithContext(sysCtx).Where("id = ?", tplA.ID).First(&after).Error; err != nil {
		t.Fatalf("回查租户 A 模板失败: %v", err)
	}
	if after.PageType != "article" || !after.Active {
		t.Fatalf("跨租户 Update 改动了租户 A 的行: %+v", after)
	}

	if err := repo.Delete(ctxB, tplA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 应报不存在，实际 err=%v", err)
	}
	var countA int64
	if err := db.WithContext(sysCtx).Model(&model.GeoSchemaTemplate{}).
		Where("tenant_id = ?", gstTenantA).Count(&countA).Error; err != nil {
		t.Fatalf("回查租户 A 模板数失败: %v", err)
	}
	if countA != 1 {
		t.Fatalf("跨租户 Delete 删掉了租户 A 的行: count=%d", countA)
	}

	if err := repo.Create(context.Background(), &model.GeoSchemaTemplate{
		PageType: "article", SchemaType: "Article", TemplateJSON: datatypes.JSON(`{}`),
	}); err == nil {
		t.Fatal("无租户 ctx 创建应报错，实际成功")
	}
}

// TestGeoSchemaTemplateRepository_停用值不被列默认值覆盖 Active 声明了 gorm
// default:true 时，false 是零值、会被 GORM 从 INSERT 里省略，落库变成 true。
func TestGeoSchemaTemplateRepository_停用值不被列默认值覆盖(t *testing.T) {
	db := gstSetupDB(t)
	repo := NewGeoSchemaTemplateRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gstTenantA)

	tpl := &model.GeoSchemaTemplate{
		PageType: "faq", SchemaType: "FAQPage",
		TemplateJSON: datatypes.JSON(`{"@type":"FAQPage"}`), Active: false,
	}
	if err := repo.Create(ctxA, tpl); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	got, err := repo.GetByID(ctxA, tpl.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Active {
		t.Fatal("显式 active=false 落库变成了 true")
	}
}
