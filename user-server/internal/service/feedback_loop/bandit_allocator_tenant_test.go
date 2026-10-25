// bandit_allocator_tenant_test.go 臂后验链路的租户契约。
//
// 覆盖三件事：
//  1. arm_key 只在实验内唯一——两个实验可复用同一 key（模型复合唯一键）；
//  2. 分配器缓存按租户隔离——同名实验不会串读他人臂后验；
//  3. 跨租户 UpdateReward/PromoteArm 不得命中他人臂。
package feedbackloop

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
	"hivemtk-user/internal/repository"
)

const (
	banditTenantA = uint(231)
	banditTenantB = uint(232)
)

func setupBanditTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.BanditArm{}, &model.PromptABTest{})
	if db == nil {
		t.Fatal("测试库不可达：租户隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestBanditArm_SameArmKeyAcrossExperimentsAllowed(t *testing.T) {
	db := setupBanditTenantDB(t)
	ctx := tenant.WithTenantID(context.Background(), banditTenantA)

	for _, expID := range []string{"exp-shared-key-1", "exp-shared-key-2"} {
		arm := &model.BanditArm{
			ExperimentID:   expID,
			ExperimentType: model.BanditExperimentTypePrompt,
			ArmKey:         "arm_0_original",
			Status:         model.BanditArmStatusExploring,
		}
		if err := db.WithContext(ctx).Create(arm).Error; err != nil {
			t.Fatalf("实验 %s 复用 arm_0_original 失败（唯一键仍是 arm_key 单列？）: %v", expID, err)
		}
		if arm.TenantID != banditTenantA {
			t.Errorf("实验 %s 的臂 tenant_id=%d，期望 %d", expID, arm.TenantID, banditTenantA)
		}
	}

	var count int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BanditArm{}).
		Where("arm_key = ?", "arm_0_original").Count(&count).Error; err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if count != 2 {
		t.Errorf("同名臂应有 2 行，实际 %d", count)
	}
}

func TestBanditAllocator_CheckConvergenceCacheIsTenantScoped(t *testing.T) {
	db := setupBanditTenantDB(t)
	const expID = "exp-shared-id"
	sys := tenant.WithSystemScope(context.Background())

	seed := func(tenantID uint, armAAlpha, armABeta, armBAlpha, armBBeta float64) {
		t.Helper()
		arms := []model.BanditArm{
			{TenantID: tenantID, ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt,
				ArmKey: "arm_a", Alpha: armAAlpha, Beta: armABeta, TotalTrials: 500, Status: model.BanditArmStatusExploring},
			{TenantID: tenantID, ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt,
				ArmKey: "arm_b", Alpha: armBAlpha, Beta: armBBeta, TotalTrials: 500, Status: model.BanditArmStatusExploring},
		}
		if err := db.WithContext(sys).Create(&arms).Error; err != nil {
			t.Fatalf("预置租户 %d 的臂失败: %v", tenantID, err)
		}
	}
	// 两个租户用同一个 experiment_id，但胜者相反：缓存键若不含租户，
	// 后一次调用会直接命中前一个租户的臂列表。
	seed(banditTenantA, 100, 10, 10, 100)
	seed(banditTenantB, 10, 100, 100, 10)

	b := NewBanditAllocator(repository.NewFeedbackLoopRepositoryWithDB(db), DefaultBanditConfig(), 42)
	winnerA, ok := b.CheckConvergence(tenant.WithTenantID(context.Background(), banditTenantA), expID)
	if !ok || winnerA != "arm_a" {
		t.Errorf("租户 A 应收敛到 arm_a, got winner=%q ok=%v", winnerA, ok)
	}
	winnerB, ok := b.CheckConvergence(tenant.WithTenantID(context.Background(), banditTenantB), expID)
	if !ok || winnerB != "arm_b" {
		t.Errorf("租户 B 应收敛到 arm_b（缓存串租户？），got winner=%q ok=%v", winnerB, ok)
	}
}

func TestBanditAllocator_UpdateRewardCrossTenantNotFound(t *testing.T) {
	db := setupBanditTenantDB(t)
	const expID = "exp-cross-tenant"
	arm := &model.BanditArm{
		ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt,
		ArmKey: "arm_a", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.WithContext(tenant.WithTenantID(context.Background(), banditTenantA)).Create(arm).Error; err != nil {
		t.Fatalf("预置租户 A 的臂失败: %v", err)
	}

	b := NewBanditAllocator(repository.NewFeedbackLoopRepositoryWithDB(db), DefaultBanditConfig(), 42)
	err := b.UpdateReward(tenant.WithTenantID(context.Background(), banditTenantB), expID, "arm_a", true, 1.0)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户更新应返回 ErrRecordNotFound, got %v", err)
	}

	var got model.BanditArm
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&got, arm.ID).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Alpha != 2 || got.TotalTrials != 0 {
		t.Errorf("跨租户更新不应改动臂: alpha=%v total_trials=%d", got.Alpha, got.TotalTrials)
	}
}

func TestBanditAllocator_PromoteArmCrossTenantNotFound(t *testing.T) {
	db := setupBanditTenantDB(t)
	const expID = "exp-promote-cross-tenant"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt, ArmKey: "arm_a", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: model.BanditExperimentTypePrompt, ArmKey: "arm_b", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
	}
	if err := db.WithContext(tenant.WithTenantID(context.Background(), banditTenantA)).Create(&arms).Error; err != nil {
		t.Fatalf("预置租户 A 的臂失败: %v", err)
	}

	b := NewBanditAllocator(repository.NewFeedbackLoopRepositoryWithDB(db), DefaultBanditConfig(), 42)
	err := b.PromoteArm(tenant.WithTenantID(context.Background(), banditTenantB), expID, "arm_a")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户晋升应返回 ErrRecordNotFound, got %v", err)
	}

	var rows []model.BanditArm
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("experiment_id = ?", expID).Find(&rows).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	for _, row := range rows {
		if row.Status != model.BanditArmStatusExploring {
			t.Errorf("跨租户晋升不应改动臂 %s: status=%q", row.ArmKey, row.Status)
		}
	}
}
