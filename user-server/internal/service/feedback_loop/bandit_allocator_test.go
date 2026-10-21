package feedbackloop

import (
	"context"
	"errors"
	"math"
	"testing"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// TestBetaSample_RangeInUnitInterval Beta 采样结果应在 [0,1]
func TestBetaSample_RangeInUnitInterval(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	cases := []struct{ alpha, beta float64 }{
		{1, 1}, {2, 2}, {10, 5}, {5, 10}, {100, 100}, {0.5, 0.5},
	}
	for _, c := range cases {
		for i := 0; i < 100; i++ {
			sample := b.betaSample(c.alpha, c.beta)
			if sample < 0 || sample > 1 {
				t.Errorf("betaSample(α=%v, β=%v) = %v 超出 [0,1]", c.alpha, c.beta, sample)
			}
		}
	}
}

// TestBetaSample_BoundaryConditions 边界条件
func TestBetaSample_BoundaryConditions(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	if got := b.betaSample(0, 5); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(0, 5) = %v want 0.5", got)
	}
	if got := b.betaSample(5, 0); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(5, 0) = %v want 0.5", got)
	}
	if got := b.betaSample(-1, 5); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(-1, 5) = %v want 0.5", got)
	}
	if got := b.betaSample(5, -1); !approxEqualF64(got, 0.5) {
		t.Errorf("betaSample(5, -1) = %v want 0.5", got)
	}
}

// TestBetaSample_MeanConverges Beta(α,β) 均值应近似 α/(α+β)
//
// 大数定律：N 次采样的均值 → 理论均值 α/(α+β)
func TestBetaSample_MeanConverges(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	cases := []struct {
		alpha, beta float64
	}{
		{2, 8},
		{5, 5},
		{8, 2},
		{50, 50},
	}
	const N = 5000
	for _, c := range cases {
		var sum float64
		for i := 0; i < N; i++ {
			sum += b.betaSample(c.alpha, c.beta)
		}
		mean := sum / N
		expected := c.alpha / (c.alpha + c.beta)
		if math.Abs(mean-expected) > 0.05 {
			t.Errorf("betaSample(α=%v, β=%v) 均值=%v want≈%v (N=%d)",
				c.alpha, c.beta, mean, expected, N)
		}
	}
}

// TestGammaSample_PositiveOutput Gamma 采样应返回正数
func TestGammaSample_PositiveOutput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	rng := b.rng
	cases := []struct{ alpha, theta float64 }{
		{0.5, 1}, {1, 1}, {2, 1}, {5, 2}, {10, 0.5}, {0.1, 1},
	}
	for _, c := range cases {
		for i := 0; i < 100; i++ {
			sample := gammaSample(c.alpha, c.theta, rng)
			if sample <= 0 {
				t.Errorf("gammaSample(α=%v, θ=%v) = %v 应为正数", c.alpha, c.theta, sample)
			}
		}
	}
}

// TestGammaSample_MeanConverges Gamma(α,θ) 均值应近似 α*θ
func TestGammaSample_MeanConverges(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	rng := b.rng
	cases := []struct {
		alpha, theta float64
	}{
		{1, 1}, {2, 1}, {5, 2}, {10, 0.5}, {0.5, 1},
	}
	const N = 5000
	for _, c := range cases {
		var sum float64
		for i := 0; i < N; i++ {
			sum += gammaSample(c.alpha, c.theta, rng)
		}
		mean := sum / N
		expected := c.alpha * c.theta
		if math.Abs(mean-expected)/expected > 0.10 {
			t.Errorf("gammaSample(α=%v, θ=%v) 均值=%v want≈%v (N=%d)",
				c.alpha, c.theta, mean, expected, N)
		}
	}
}

// TestBanditAllocator_UpdateReward_Success 成功更新 alpha
func TestBanditAllocator_UpdateReward_Success(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	ctx := context.Background()
	expID := "test_update_reward"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)

	if err := b.UpdateReward(ctx, expID, "arm_a", true, 1.5); err != nil {
		t.Fatalf("UpdateReward: %v", err)
	}
	var updated model.BanditArm
	if err := db.First(&updated, arm.ID).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if updated.Alpha != 3 {
		t.Errorf("Alpha = %v want 3", updated.Alpha)
	}
	if updated.Beta != 2 {
		t.Errorf("Beta = %v want 2", updated.Beta)
	}
	if updated.TotalTrials != 1 {
		t.Errorf("TotalTrials = %v want 1", updated.TotalTrials)
	}
	if updated.SuccessTrials != 1 {
		t.Errorf("SuccessTrials = %v want 1", updated.SuccessTrials)
	}
	if !approxEqualF64(updated.SumReward, 1.5) {
		t.Errorf("SumReward = %v want 1.5", updated.SumReward)
	}
}

// TestBanditAllocator_UpdateReward_Failure 失败更新 beta
func TestBanditAllocator_UpdateReward_Failure(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_update_reward_fail"
	arm := model.BanditArm{
		ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a",
		Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring,
	}
	if err := db.Create(&arm).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	if err := b.UpdateReward(context.Background(), expID, "arm_a", false, -0.5); err != nil {
		t.Fatalf("UpdateReward: %v", err)
	}
	var updated model.BanditArm
	_ = db.First(&updated, arm.ID).Error
	if updated.Alpha != 2 {
		t.Errorf("Alpha = %v want 2", updated.Alpha)
	}
	if updated.Beta != 3 {
		t.Errorf("Beta = %v want 3 (失败时 beta+1)", updated.Beta)
	}
	if updated.SuccessTrials != 0 {
		t.Errorf("SuccessTrials = %v want 0", updated.SuccessTrials)
	}
}

// TestBanditAllocator_UpdateReward_InvalidInput 空参数返回错误
func TestBanditAllocator_UpdateReward_InvalidInput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	if err := b.UpdateReward(context.Background(), "", "arm_a", true, 1.0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 experimentID 应返回 ErrInvalidInput, got %v", err)
	}
	if err := b.UpdateReward(context.Background(), "exp", "", true, 1.0); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 armKey 应返回 ErrInvalidInput, got %v", err)
	}
}

// TestBanditAllocator_UpdateReward_UnknownArmFails 不存在的臂必须报错而不是静默成功
//
// 静默成功会让回流台账记下一笔"已入账"，而臂后验一字未动。
func TestBanditAllocator_UpdateReward_UnknownArmFails(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	err := b.UpdateReward(context.Background(), "exp-missing", "arm_missing", true, 1.0)
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("不存在的臂应返回 ErrRecordNotFound, got %v", err)
	}
}

// TestBanditAllocator_PromoteArm_UnknownWinnerKeepsArms 胜出臂不存在时不得退役其他臂
func TestBanditAllocator_PromoteArm_UnknownWinnerKeepsArms(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_promote_unknown"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 2, Beta: 2, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	err := b.PromoteArm(context.Background(), expID, "typo_arm")
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("不存在的胜出臂应返回 ErrRecordNotFound, got %v", err)
	}
	var rows []model.BanditArm
	if err := db.Where("experiment_id = ?", expID).Find(&rows).Error; err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	for _, row := range rows {
		if row.Status != model.BanditArmStatusExploring {
			t.Errorf("胜出臂不存在时不应改动其他臂 %s: status=%q", row.ArmKey, row.Status)
		}
	}
}

// TestBanditAllocator_CheckConvergence_InsufficientSamples 样本不足不收敛
func TestBanditAllocator_CheckConvergence_InsufficientSamples(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_convergence_insufficient"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 50, Beta: 10, TotalTrials: 50, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 50, TotalTrials: 50, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	winner, ok := b.CheckConvergence(context.Background(), expID)
	if ok {
		t.Errorf("样本不足不应收敛，但 winner=%s", winner)
	}
}

// TestBanditAllocator_CheckConvergence_Converged 已收敛
//
// arm_a: alpha=100, beta=10 → 均值 ~0.91
// arm_b: alpha=10, beta=100 → 均值 ~0.09
// P(arm_a 最优) ≈ 1.0 ≥ 0.95
func TestBanditAllocator_CheckConvergence_Converged(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_convergence_yes"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 100, Beta: 10, TotalTrials: 110, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 100, TotalTrials: 110, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	winner, ok := b.CheckConvergence(context.Background(), expID)
	if !ok {
		t.Errorf("应收敛但未收敛")
	}
	if winner != "arm_a" {
		t.Errorf("winner = %q want arm_a", winner)
	}
}

// TestBanditAllocator_PromoteArm 提升胜出臂 + 退役其他
func TestBanditAllocator_PromoteArm(t *testing.T) {
	db := setupFeedbackLoopTestDB(t)
	expID := "test_promote"
	arms := []model.BanditArm{
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_a", Alpha: 100, Beta: 10, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_b", Alpha: 10, Beta: 100, Status: model.BanditArmStatusExploring},
		{ExperimentID: expID, ExperimentType: "prompt", ArmKey: "arm_c", Alpha: 5, Beta: 50, Status: model.BanditArmStatusExploring},
	}
	if err := db.Create(&arms).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	b := NewBanditAllocator(db, DefaultBanditConfig(), 42)
	if err := b.PromoteArm(context.Background(), expID, "arm_a"); err != nil {
		t.Fatalf("PromoteArm: %v", err)
	}

	var winner model.BanditArm
	_ = db.Where("experiment_id = ? AND arm_key = ?", expID, "arm_a").First(&winner).Error
	if winner.Status != model.BanditArmStatusPromoted {
		t.Errorf("arm_a status = %q want promoted", winner.Status)
	}
	if winner.PromotedAt == nil {
		t.Errorf("arm_a promoted_at 应非空")
	}

	var losers []model.BanditArm
	_ = db.Where("experiment_id = ? AND arm_key IN ?", expID, []string{"arm_b", "arm_c"}).Find(&losers).Error
	if len(losers) != 2 {
		t.Errorf("应找到 2 个 retired 臂, got %d", len(losers))
	}
	for _, l := range losers {
		if l.Status != model.BanditArmStatusRetired {
			t.Errorf("arm %s status = %q want retired", l.ArmKey, l.Status)
		}
	}
}

// TestBanditAllocator_PromoteArm_InvalidInput 空参数错误
func TestBanditAllocator_PromoteArm_InvalidInput(t *testing.T) {
	b := NewBanditAllocator(nil, DefaultBanditConfig(), 42)
	if err := b.PromoteArm(context.Background(), "", "arm_a"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 experimentID 应返回 ErrInvalidInput, got %v", err)
	}
	if err := b.PromoteArm(context.Background(), "exp", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("空 winnerKey 应返回 ErrInvalidInput, got %v", err)
	}
}

