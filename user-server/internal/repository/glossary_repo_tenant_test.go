// glossary_repo_tenant_test.go 术语表写入的租户归属契约。
//
// 背景（任务三逐路由 curl 实测 + 对抗性复核）：GlossaryRepository.Update 原先用
// `Save(g)` —— Save 把**整个结构体**写回，包括本层从未赋值的 tenant_id（=0）。
//
// 后果（两处都实测过）：
//   - 离线单机（off）档：隔离 callback 不注入 ⇒ 行带着 tenant_id=0 落库 ⇒
//     那不属于任何租户，**默认租户自己也读不到** —— 术语表里一条都读不出来。
//     实测：PUT /api/glossaries/:term_id 更新一次后，
//     `select tenant_id from glossaries where term_id=…` 得到 0。
//   - SaaS（on）档：撞「更新不可改写 tenant_id」闸门 ⇒ 整条更新 500。
//
// 判据：更新一条已存在的术语后，它的 tenant_id 必须**不变**（还是它原来那个租户），
// 且业务列真的被更新到。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

func TestGlossaryRepository_UpdateKeepsTenantID(t *testing.T) {
	// off 模式：隔离 callback 不注入 ⇒ 正是「Save 写零值 tenant_id」会露出问题的那一档。
	prev := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(prev) })

	db := testutil.NewTestDB(t, &model.Glossary{})
	if db == nil {
		t.Fatal("测试库不可达：术语表的租户归属判不了（不 Skip，跳过等于没锁）")
	}
	repo := NewGlossaryRepositoryWithDB(db)
	ctx := context.Background()

	// 先建一条归属默认租户 1 的术语（Create 走列默认值）。
	created := &model.Glossary{TermID: "repo-tenant-term", Category: "probe", Translations: model.JSONMap{"en": "x"}}
	if err := repo.Create(ctx, created); err != nil {
		t.Fatalf("seed 术语失败: %v", err)
	}
	if created.TenantID != 1 {
		t.Fatalf("建行未落默认租户: tenant_id=%d want 1", created.TenantID)
	}

	// 更新：只带业务列，**不带 tenant_id**（与控制器 translation.ToGlossaryModelUpdate
	// 的形状一致 —— 它从不设 TenantID）。
	updated := &model.Glossary{ID: created.ID, TermID: created.TermID, Category: "probe2",
		Translations: model.JSONMap{"en": "y"}}
	if err := repo.Update(ctx, updated); err != nil {
		t.Fatalf("更新术语失败: %v", err)
	}

	var got model.Glossary
	if err := db.WithContext(ctx).Where("id = ?", created.ID).First(&got).Error; err != nil {
		t.Fatalf("回读术语失败: %v", err)
	}
	if got.TenantID != 1 {
		t.Errorf("更新把租户改掉了: tenant_id=%d want 1（Save 写零值的老坑）", got.TenantID)
	}
	if got.Category != "probe2" {
		t.Errorf("业务列没更新到: category=%q want %q", got.Category, "probe2")
	}
}
