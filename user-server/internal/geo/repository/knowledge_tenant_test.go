// knowledge_tenant_test.go 知识库文档的租户契约。
//
// geo_knowledge_documents 是租户表：列表只含本租户；跨租户更新/删除必须报不存在，
// 已软删除的文档不能被 Update 复活。
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
	gkbTenantA = uint(351)
	gkbTenantB = uint(352)
)

func gkbSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.GeoKnowledgeDocument{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoKnowledgeDocumentRepository_TenantScoped(t *testing.T) {
	db := gkbSetupDB(t)
	repo := NewGeoKnowledgeDocumentRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gkbTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gkbTenantB)

	docA := &model.GeoKnowledgeDocument{Title: "A 文档", Content: "内容", DocType: "manual"}
	if err := repo.Create(ctxA, docA); err != nil {
		t.Fatalf("租户 A 创建失败: %v", err)
	}
	if docA.TenantID != gkbTenantA {
		t.Fatalf("Create 未填租户: %d", docA.TenantID)
	}
	if err := repo.Create(ctxB, &model.GeoKnowledgeDocument{Title: "B 文档", Content: "内容"}); err != nil {
		t.Fatalf("租户 B 创建失败: %v", err)
	}

	listA, err := repo.GetList(ctxA)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != gkbTenantA {
		t.Fatalf("租户 A 列表越界: %+v", listA)
	}

	// 跨租户读取：报不存在。
	if _, err := repo.GetByID(ctxB, listA[0].ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByID 应报不存在，实际 err=%v", err)
	}

	// 跨租户更新：报不存在，且不改租户 B 的正文。
	var bDoc model.GeoKnowledgeDocument
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Where("tenant_id = ?", gkbTenantB).First(&bDoc).Error; err != nil {
		t.Fatalf("回查租户 B 文档失败: %v", err)
	}
	bDoc.Content = "hijacked"
	if err := repo.Update(ctxA, &bDoc); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户更新应报不存在，实际 err=%v", err)
	}
	var after model.GeoKnowledgeDocument
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Where("id = ?", bDoc.ID).First(&after).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if after.Content == "hijacked" {
		t.Fatal("跨租户更新修改了别的租户的文档")
	}

	// 跨租户删除：报不存在且不删。
	if err := repo.Delete(ctxA, bDoc.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报不存在，实际 err=%v", err)
	}
	if _, err := repo.GetByID(ctxB, bDoc.ID); err != nil {
		t.Fatalf("跨租户删除不应删掉租户 B 的文档: %v", err)
	}
}

func TestGeoKnowledgeDocumentRepository_UpdateDoesNotResurrectDeleted(t *testing.T) {
	db := gkbSetupDB(t)
	repo := NewGeoKnowledgeDocumentRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gkbTenantA)

	doc := &model.GeoKnowledgeDocument{Title: "待删", Content: "内容"}
	if err := repo.Create(ctxA, doc); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	if err := repo.Delete(ctxA, doc.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	doc.Title = "复活尝试"
	if err := repo.Update(ctxA, doc); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("已删除文档的更新应报不存在，实际 err=%v", err)
	}
	if _, err := repo.GetByID(ctxA, doc.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("已删除文档被 Update 复活: err=%v", err)
	}
}
