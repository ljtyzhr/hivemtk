// churn_prediction_repo_tenant_test.go 流失预测写入的租户归属契约。
//
// 背景（任务三逐路由 curl 实测）：ChurnPredictionRepository.Upsert 原先用
// `Save(prediction)` —— Save 把**整个结构体**写回，包括调用方没填的
// tenant_id（=0）。在 on 模式（多租户）下有两个后果：
//
//  1. UPDATE 语句带 tenant_id=0 ⇒ 撞「更新不可改写 tenant_id」闸门 ⇒ 整条
//     写入链路 500；
//  2. 即使闸门没拦下来，行也带着 tenant_id=0 落库 ⇒ 那不属于任何租户，
//     **任何租户都读不到它**（离线单机档 tenant_id=1 的行也读不到，
//     等于这批预测对所有人隐身）。
//
// 判据：同一租户重复 Upsert 同一个 user_id 必须成功、且 tenant_id 保持不变；
// 另一个租户读不到这一行。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	churnTenantA = uint(87)
	churnTenantB = uint(88)
)

func churnSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ChurnPrediction{})
	if db == nil {
		t.Fatal("测试库不可达：流失预测的租户归属判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestChurnPrediction_UpsertKeepsTenantID(t *testing.T) {
	db := churnSetupDB(t)
	repo := NewChurnPredictionRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), churnTenantA)
	ctxB := tenant.WithTenantID(context.Background(), churnTenantB)

	// 第一次：建行。调用方不填 tenant_id（真实调用方就是这样的）。
	first := &model.ChurnPrediction{UserID: "churn-user-1", ChurnScore: 10, ChurnRisk: "low"}
	if err := repo.Upsert(ctxA, first); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	if first.TenantID != churnTenantA {
		t.Fatalf("写入未回填租户: tenant_id=%d want %d", first.TenantID, churnTenantA)
	}

	// 第二次：同租户同 user_id ⇒ 走 UPDATE 分支（原先就是这一支 500 的）。
	second := &model.ChurnPrediction{UserID: "churn-user-1", ChurnScore: 88.5, ChurnRisk: "critical"}
	if err := repo.Upsert(ctxA, second); err != nil {
		t.Fatalf("同租户重复写入失败（Save 写回 tenant_id=0 的老坑）: %v", err)
	}
	if second.TenantID != churnTenantA {
		t.Fatalf("重复写入把租户改掉了: tenant_id=%d want %d", second.TenantID, churnTenantA)
	}

	got, err := repo.GetByUserID(ctxA, "churn-user-1")
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.TenantID != churnTenantA {
		t.Errorf("行归属错了: tenant_id=%d want %d", got.TenantID, churnTenantA)
	}
	if got.ChurnScore != 88.5 || got.ChurnRisk != "critical" {
		t.Errorf("重复写入没更新到业务列: score=%v risk=%q", got.ChurnScore, got.ChurnRisk)
	}

	// 另一个租户读不到这一行。
	if _, err := repo.GetByUserID(ctxB, "churn-user-1"); err == nil {
		t.Fatal("租户 B 读到了租户 A 的流失预测")
	}
}
