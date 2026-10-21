// ab_experiment_result_tenant_test.go A/B 实验结果仓库的租户与键位契约。
//
// 结果粒度是「每实验每变体一行」：唯一键必须是 (tenant_id, experiment_id, variant_id)。
// 历史上模型把 experiment_id 标成单列唯一，第二个变体的 Upsert 直接撞 23505；
// 本用例同时钉住复合键、租户隔离、更新不覆盖 tenant_id/created_at。
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
	abResultTenantA = uint(87)
	abResultTenantB = uint(88)
)

func abResultSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ABExperimentResult{})
	if db == nil {
		t.Fatal("测试库不可达：实验结果键位判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestABExperimentResult_PerVariantRowsAndTenantScope(t *testing.T) {
	db := abResultSetupDB(t)
	repo := NewABExperimentResultRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), abResultTenantA)
	ctxB := tenant.WithTenantID(context.Background(), abResultTenantB)

	const experimentID = uint(501)
	for _, variantID := range []uint{1, 2} {
		if err := repo.Upsert(ctxA, &model.ABExperimentResult{
			ExperimentID: experimentID, VariantID: variantID,
			VariantName: "A", TrafficCount: 100, ConversionCount: 10,
		}); err != nil {
			t.Fatalf("租户 A 变体 %d 首次写入失败（单列唯一会在这里撞 23505）: %v", variantID, err)
		}
		if err := repo.Upsert(ctxB, &model.ABExperimentResult{
			ExperimentID: experimentID, VariantID: variantID,
			VariantName: "B", TrafficCount: 200, ConversionCount: 20,
		}); err != nil {
			t.Fatalf("租户 B 变体 %d 写入失败: %v", variantID, err)
		}
	}

	rowsA, err := repo.GetByExperiment(ctxA, experimentID)
	if err != nil {
		t.Fatalf("租户 A 读结果失败: %v", err)
	}
	if len(rowsA) != 2 {
		t.Fatalf("租户 A 结果行数=%d，期望每变体一行共 2 行", len(rowsA))
	}
	first := rowsA[0]
	if first.TenantID != abResultTenantA {
		t.Fatalf("租户 A 结果 tenant_id=%d，期望 %d", first.TenantID, abResultTenantA)
	}
	createdAt := first.CreatedAt
	if createdAt.IsZero() {
		t.Fatal("结果行 created_at 不应为零值")
	}

	// 同一 (实验, 变体) 再写一次：原地更新，不新增行、不改租户、不重写创建时间。
	if err := repo.Upsert(ctxA, &model.ABExperimentResult{
		ExperimentID: experimentID, VariantID: first.VariantID,
		VariantName: "A", TrafficCount: 999, ConversionCount: 99,
	}); err != nil {
		t.Fatalf("租户 A 更新结果失败: %v", err)
	}

	rowsB, err := repo.GetByExperiment(ctxB, experimentID)
	if err != nil {
		t.Fatalf("租户 B 读结果失败: %v", err)
	}
	if len(rowsB) != 2 {
		t.Fatalf("租户 B 结果行数=%d，期望 2 行且看不到租户 A 的行", len(rowsB))
	}

	var physical int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ABExperimentResult{}).Where("experiment_id = ?", experimentID).Count(&physical).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if physical != 4 {
		t.Fatalf("物理行数=%d，期望两租户各 2 行共 4 行", physical)
	}

	var updated model.ABExperimentResult
	if err := db.WithContext(ctxA).Where("experiment_id = ? AND variant_id = ?", experimentID, first.VariantID).
		First(&updated).Error; err != nil {
		t.Fatalf("读回更新行失败: %v", err)
	}
	if updated.TenantID != abResultTenantA {
		t.Fatalf("更新后 tenant_id=%d，期望保持 %d", updated.TenantID, abResultTenantA)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("更新后 created_at=%s，期望保持 %s", updated.CreatedAt, createdAt)
	}
	if updated.TrafficCount != 999 || updated.ConversionCount != 99 {
		t.Fatalf("更新未生效: traffic=%d conversion=%d", updated.TrafficCount, updated.ConversionCount)
	}
}

func TestABExperimentResult_UnboundCtxIsRejected(t *testing.T) {
	db := abResultSetupDB(t)
	repo := NewABExperimentResultRepositoryWithDB(db)
	ctx := context.Background()

	if _, err := repo.GetByExperiment(ctx, 501); err == nil {
		t.Fatal("on 模式下无租户绑定的读取应被隔离 callback 拒绝")
	}
	if err := repo.Upsert(ctx, &model.ABExperimentResult{ExperimentID: 501, VariantID: 1}); err == nil {
		t.Fatal("on 模式下无租户绑定的写入应被隔离 callback 拒绝")
	}

	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ABExperimentResult{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("被拒绝的写入不应落行，实际 %d 行", rows)
	}
}
