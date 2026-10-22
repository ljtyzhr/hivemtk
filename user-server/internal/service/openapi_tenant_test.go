package service

// 契约：知识 OpenAPI 数据源的**更新不得改写租户归属**。
//
// 坑（与 SMS 配置、流失预测同一族的通用形状）：
// 调用方传进来的是**新建的零值 model**（tenant_id=0），而 repo.Update 走 GORM 的
// Save —— Save 写**全部字段含零值** ⇒ 这一行的租户被改写成 0。
// 多租户模式下 tenant 回调 fail-closed 当场拒掉：
//
//	[tenant] 更新不可改写 tenant_id（上下文租户 1，SET 值 0）
//
// ⇒ 接口 500；而独立部署模式（TENANT_MODE=off）没有这层校验 ⇒ 同一个接口
// SaaS 下 500、私域下 200 —— 能用的那边把坏的那边掩盖了。
//
// 修法：更新前把库里的 tenant_id 带回来。
//
// 本文件锁两条腿：
//  1. 更新后该行**仍**属于原租户（不被改写成 0、也不被改到别的租户）；
//  2. 更新**确实生效**（改了名字要真的存下去，不能为了保 tenant 而不写）。

import (
	"context"
	"testing"

	knowledgemodel "hivemtk-user/internal/aiagent/knowledge/model"
	knowledgerepo "hivemtk-user/internal/aiagent/knowledge/repository"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestOpenAPISourceUpdateKeepsTenant 更新不得把行改成孤儿行（tenant_id=0）。
func TestOpenAPISourceUpdateKeepsTenant(t *testing.T) {
	db := testutil.NewTestDB(t, &model.KnowledgeOpenAPISource{})
	if db == nil {
		t.Fatal("测试库不可达：租户归属结论无法判定（不 Skip，跳过等于没锁）")
	}
	repo := knowledgerepo.NewKnowledgeOpenAPIRepository(db)
	svc := &OpenAPIService{srcRepo: repo, docRepo: knowledgerepo.NewKnowledgeDocumentRepository(db)}

	const tid = uint(5)
	seed := &model.KnowledgeOpenAPISource{
		TenantID:  tid,
		ProductID: "p1",
		Name:      "old-name",
		Type:      "rest",
		Endpoint:  "https://api.example.com/v1",
		Method:    "GET",
		AuthType:  "none",
		Enabled:   1,
	}
	if err := repo.Create(context.Background(), seed); err != nil {
		t.Fatalf("种子数据源落库失败：%v", err)
	}

	// 模拟 controller 的真实入参：**新建的零值 model**（tenant_id=0）。
	err := svc.UpdateSource(context.Background(), &model.KnowledgeOpenAPISource{
		ID:        seed.ID,
		ProductID: "p1",
		Name:      "new-name",
		Type:      "rest",
		Endpoint:  "https://api.example.com/v2",
		Method:    "GET",
		AuthType:  "none",
		Enabled:   1,
	})
	if err != nil {
		t.Fatalf("多租户模式下 UpdateSource 失败：%v", err)
	}

	var got model.KnowledgeOpenAPISource
	if err := db.Where("id = ?", seed.ID).First(&got).Error; err != nil {
		t.Fatalf("读回数据源失败：%v", err)
	}
	if got.TenantID != tid {
		t.Errorf("tenant_id 应仍为 %d，实际 %d —— Save 把零值 tenant_id 一起写进去了，"+
			"这行要么被 tenant 回调拒掉、要么被静默改到别的租户", tid, got.TenantID)
	}
	if got.Name != "new-name" {
		t.Errorf("更新必须真的生效：name 期望 %q，实际 %q", "new-name", got.Name)
	}
}

// TestOpenAPISourceUpdateNotFound 不存在的 id 要报"不存在"，不能变成 INSERT。
func TestOpenAPISourceUpdateNotFound(t *testing.T) {
	db := testutil.NewTestDB(t, &model.KnowledgeOpenAPISource{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	repo := knowledgerepo.NewKnowledgeOpenAPIRepository(db)
	svc := &OpenAPIService{srcRepo: repo, docRepo: knowledgerepo.NewKnowledgeDocumentRepository(db)}

	err := svc.UpdateSource(context.Background(), &knowledgemodel.KnowledgeOpenAPISource{
		ID: 999999, Name: "x",
	})
	if err == nil {
		t.Fatal("不存在的 id 必须报错")
	}
}
