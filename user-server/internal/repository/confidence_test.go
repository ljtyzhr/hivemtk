// confidence_repository_test.go 置信度四张表的租户契约与写路径语义（实体 #63 审计补充）。
//
// 本文件在真库上锁住：
//  1. signal_id / calibration_id / policy_id 的唯一性只在租户内成立（tenancyidx 复合化）；
//  2. SaveActive 只下线本租户、同意图的旧版本，别家租户的 active 行不受影响；
//  3. 管理面列表分页归一、按租户遮挡；
//  4. 更新不存在的策略返回 ErrRecordNotFound，而不是 Save 的补插兜底。
package repository

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	confidenceTenantA = uint(321)
	confidenceTenantB = uint(322)
)

func confidenceSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t,
		&model.ConfidenceSignal{},
		&model.ConfidenceCalibration{},
		&model.ThresholdPolicy{},
	)
	if db == nil {
		t.Fatal("测试库不可达：confidence 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestThresholdPolicyRepository_SaveActiveIsTenantScoped(t *testing.T) {
	db := confidenceSetupDB(t)
	repo := NewThresholdPolicyRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), confidenceTenantA)
	ctxB := tenant.WithTenantID(context.Background(), confidenceTenantB)

	pA := &model.ThresholdPolicy{PolicyID: "policy_purchase", IntentType: "purchase", BaseThreshold: 0.61, IsActive: true, Version: 1}
	if err := repo.SaveActive(ctxA, pA); err != nil {
		t.Fatalf("A 保存策略失败: %v", err)
	}
	// 同 policy_id 在另一租户独立成立（唯一索引已复合化）。
	pB := &model.ThresholdPolicy{PolicyID: "policy_purchase", IntentType: "purchase", BaseThreshold: 0.66, IsActive: true, Version: 1}
	if err := repo.SaveActive(ctxB, pB); err != nil {
		t.Fatalf("B 同 policy_id 应能独立保存: %v", err)
	}

	// A 发布同意图新版本：只下线 A 的旧版本。
	pA2 := &model.ThresholdPolicy{PolicyID: "policy_purchase_v2", IntentType: "purchase", BaseThreshold: 0.72, IsActive: true, Version: 2}
	if err := repo.SaveActive(ctxA, pA2); err != nil {
		t.Fatalf("A 保存新版本失败: %v", err)
	}

	activeA, err := repo.ListActive(ctxA)
	if err != nil {
		t.Fatalf("A 列策略失败: %v", err)
	}
	if len(activeA) != 1 || activeA[0].PolicyID != "policy_purchase_v2" || activeA[0].Version != 2 {
		t.Fatalf("A 应只剩新版本 active，实际 %+v", activeA)
	}
	activeB, err := repo.ListActive(ctxB)
	if err != nil {
		t.Fatalf("B 列策略失败: %v", err)
	}
	if len(activeB) != 1 || activeB[0].PolicyID != "policy_purchase" {
		t.Fatalf("B 的 active 策略不应被 A 的发布影响，实际 %+v", activeB)
	}
}

func TestThresholdPolicyRepository_SaveActiveUpdateRequiresExistingRow(t *testing.T) {
	repo := NewThresholdPolicyRepositoryWithDB(confidenceSetupDB(t))
	ctx := tenant.WithTenantID(context.Background(), confidenceTenantA)

	err := repo.SaveActive(ctx, &model.ThresholdPolicy{
		ID: 999999, PolicyID: "policy_missing", IntentType: "purchase", BaseThreshold: 0.6, Version: 1,
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("更新不存在的行应返回 ErrRecordNotFound，实际 %v", err)
	}
}

func TestConfidenceSignalRepository_ListAndStatsAreTenantScoped(t *testing.T) {
	db := confidenceSetupDB(t)
	repo := NewConfidenceSignalRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), confidenceTenantA)
	ctxB := tenant.WithTenantID(context.Background(), confidenceTenantB)

	for i := 0; i < 3; i++ {
		if err := repo.Create(ctxA, &model.ConfidenceSignal{
			SignalID: fmt.Sprintf("sig-a-%d", i), SessionID: "s-a", CustomerID: "c-a", MessageID: "m",
			IntentType: "purchase", DecisionBand: "auto",
		}); err != nil {
			t.Fatalf("A 写信号失败: %v", err)
		}
	}
	if err := repo.Create(ctxB, &model.ConfidenceSignal{
		SignalID: "sig-b-0", SessionID: "s-b", CustomerID: "c-b", MessageID: "m",
		IntentType: "purchase", DecisionBand: "handoff",
	}); err != nil {
		t.Fatalf("B 写信号失败: %v", err)
	}

	rows, total, err := repo.List(ctxA, "", 0, 0)
	if err != nil {
		t.Fatalf("A 列信号失败: %v", err)
	}
	if total != 3 || len(rows) != 3 {
		t.Fatalf("A 只该看见自己的 3 条，实际 total=%d len=%d", total, len(rows))
	}
	for _, r := range rows {
		if r.TenantID != confidenceTenantA {
			t.Errorf("读回别家租户的行: tenant=%d", r.TenantID)
		}
	}

	statsA, err := repo.StatsByBand(ctxA, time.Time{})
	if err != nil {
		t.Fatalf("A 统计失败: %v", err)
	}
	for _, s := range statsA {
		if s.DecisionBand == "handoff" {
			t.Errorf("A 的统计混入了 B 的决策带: %+v", statsA)
		}
	}
}

func TestConfidenceCalibrationRepository_ActiveIsTenantScoped(t *testing.T) {
	db := confidenceSetupDB(t)
	repo := NewConfidenceCalibrationRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), confidenceTenantA)
	ctxB := tenant.WithTenantID(context.Background(), confidenceTenantB)

	if err := repo.SaveActive(ctxA, &model.ConfidenceCalibration{
		CalibrationID: "cal-a-1", SignalType: "intent_conf", Method: "temperature", Temperature: 1.2, SampleSize: 100,
	}); err != nil {
		t.Fatalf("A 保存校准失败: %v", err)
	}
	if err := repo.SaveActive(ctxB, &model.ConfidenceCalibration{
		CalibrationID: "cal-b-1", SignalType: "intent_conf", Method: "temperature", Temperature: 0.8, SampleSize: 50,
	}); err != nil {
		t.Fatalf("B 保存校准失败: %v", err)
	}
	if err := repo.SaveActive(ctxA, &model.ConfidenceCalibration{
		CalibrationID: "cal-a-2", SignalType: "intent_conf", Method: "temperature", Temperature: 1.1, SampleSize: 200,
	}); err != nil {
		t.Fatalf("A 保存第二版校准失败: %v", err)
	}

	activeA, err := repo.GetActive(ctxA, "intent_conf")
	if err != nil {
		t.Fatalf("A 取 active 失败: %v", err)
	}
	if activeA.CalibrationID != "cal-a-2" {
		t.Errorf("A 的 active 应是新版 cal-a-2，实际 %s", activeA.CalibrationID)
	}
	activeB, err := repo.GetActive(ctxB, "intent_conf")
	if err != nil {
		t.Fatalf("B 取 active 失败: %v", err)
	}
	if activeB.CalibrationID != "cal-b-1" || activeB.Temperature != 0.8 {
		t.Errorf("B 的 active 被 A 的发布影响: %+v", activeB)
	}
}
