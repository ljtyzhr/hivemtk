// ab_experiment_repo_tenant_test.go A/B 实验与变体的租户隔离契约。
//
// 两个租户可以绑定同一个 source_id（各自投放自己的页面），读/改/删都必须
// 只看得到自己的行；on 模式漏绑 ctx 必须被隔离 callback 拒绝。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/ops/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	abExpTenantA = uint(89)
	abExpTenantB = uint(90)
)

func abExpSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ABExperiment{}, &model.ABVariant{})
	if db == nil {
		t.Fatal("测试库不可达：实验租户隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestABExperiment_PerTenantIsolation(t *testing.T) {
	db := abExpSetupDB(t)
	expRepo := NewABExperimentRepositoryWithDB(db)
	variantRepo := NewABVariantRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), abExpTenantA)
	ctxB := tenant.WithTenantID(context.Background(), abExpTenantB)

	expA := &model.ABExperiment{Name: "A", SourceType: "page", SourceID: "same-source", Status: "running"}
	if err := expRepo.Create(ctxA, expA); err != nil {
		t.Fatalf("租户 A 创建实验失败: %v", err)
	}
	expB := &model.ABExperiment{Name: "B", SourceType: "page", SourceID: "same-source", Status: "running"}
	if err := expRepo.Create(ctxB, expB); err != nil {
		t.Fatalf("租户 B 创建同名 source 实验失败: %v", err)
	}
	if expA.TenantID != abExpTenantA || expB.TenantID != abExpTenantB {
		t.Fatalf("创建未回填租户: A=%d B=%d", expA.TenantID, expB.TenantID)
	}

	if _, err := expRepo.GetByID(ctxA, expB.ID); err == nil {
		t.Fatal("租户 A 读到了租户 B 的实验")
	}
	if err := expRepo.UpdateStatus(ctxA, expB.ID, "paused"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户改状态应报不存在，实际 err=%v", err)
	}
	if got, err := expRepo.GetByID(ctxB, expB.ID); err != nil || got.Status != "running" {
		t.Fatalf("跨租户更新生效了: status=%v err=%v", got.Status, err)
	}
	if err := expRepo.Delete(ctxA, expB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应报不存在，实际 err=%v", err)
	}
	if _, err := expRepo.GetByID(ctxB, expB.ID); err != nil {
		t.Fatalf("跨租户删除动了 B 的实验: %v", err)
	}

	gotLatest, err := expRepo.GetLatestBySourceID(ctxA, "same-source")
	if err != nil || gotLatest.ID != expA.ID {
		t.Fatalf("同 source 的反查串租户: got=%+v err=%v", gotLatest, err)
	}
	listA, totalA, err := expRepo.GetAll(ctxA, 1, 10)
	if err != nil || totalA != 1 || len(listA) != 1 || listA[0].ID != expA.ID {
		t.Fatalf("租户 A 列表串租户: total=%d rows=%d err=%v", totalA, len(listA), err)
	}

	varA := &model.ABVariant{ExperimentID: expA.ID, Name: "A1", Weight: 100}
	if err := variantRepo.Create(ctxA, varA); err != nil {
		t.Fatalf("租户 A 创建变体失败: %v", err)
	}
	varB := &model.ABVariant{ExperimentID: expB.ID, Name: "B1", Weight: 100}
	if err := variantRepo.Create(ctxB, varB); err != nil {
		t.Fatalf("租户 B 创建变体失败: %v", err)
	}
	if varA.TenantID != abExpTenantA || varB.TenantID != abExpTenantB {
		t.Fatalf("变体创建未回填租户: A=%d B=%d", varA.TenantID, varB.TenantID)
	}

	// 租户 B 用 A 的实验 ID 删变体：0 行，不得动 A 的数据。
	if err := variantRepo.DeleteByExperiment(ctxB, expA.ID); err != nil {
		t.Fatalf("跨租户删变体失败: %v", err)
	}
	rowsA, err := variantRepo.GetByExperiment(ctxA, expA.ID)
	if err != nil || len(rowsA) != 1 {
		t.Fatalf("租户 A 的变体被跨租户删除了: rows=%d err=%v", len(rowsA), err)
	}

	var physical int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ABExperiment{}).Count(&physical).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if physical != 2 {
		t.Fatalf("实验物理行数=%d，期望两租户各 1 行", physical)
	}
}

func TestABExperiment_UnboundCtxIsRejected(t *testing.T) {
	db := abExpSetupDB(t)
	expRepo := NewABExperimentRepositoryWithDB(db)
	variantRepo := NewABVariantRepositoryWithDB(db)
	ctx := context.Background()

	if err := expRepo.Create(ctx, &model.ABExperiment{Name: "no-tenant", SourceID: "x"}); err == nil {
		t.Fatal("on 模式下无租户绑定的创建应被拒绝")
	}
	if _, err := expRepo.GetByID(ctx, 1); err == nil {
		t.Fatal("on 模式下无租户绑定的读取应被拒绝")
	}
	if _, err := expRepo.GetLatestBySourceID(ctx, "x"); err == nil {
		t.Fatal("on 模式下无租户绑定的反查应被拒绝")
	}
	if err := variantRepo.Create(ctx, &model.ABVariant{ExperimentID: 1, Name: "v"}); err == nil {
		t.Fatal("on 模式下无租户绑定的变体创建应被拒绝")
	}

	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.ABExperiment{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("被拒绝的写入不应落行，实际 %d 行", rows)
	}
}
