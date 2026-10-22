// clue_score_tenant_test.go 线索评分链路的租户契约（实体 #61 审计补充）。
//
// 评分与互动事件都挂在 clue_id 上，而 clue_id 的唯一性在 SaaS 模式下只在租户内成立。
// 本文件锁住 service 层的三件事：
//  1. 不同租户的线索各自评分，互不覆盖；
//  2. 读侧查不到别家的评分（返回 ErrClueScoreNotFound，而不是串租户数据）；
//  3. 温度回写只落在本租户的线索上。
package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

const (
	clueScoreSvcTenantA = uint(303)
	clueScoreSvcTenantB = uint(304)
)

func TestClueScoreService_ScoringIsTenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	database := testutil.NewTestDB(t, &model.Clue{}, &model.ClueScore{}, &model.ClueEngagementEvent{})
	clueRepo := repository.NewClueRepositoryWithDB(database)
	svc := NewClueScoreService(database)
	ctxA := tenant.WithTenantID(context.Background(), clueScoreSvcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), clueScoreSvcTenantB)

	clueA := &model.Clue{Type: 9, Account: "shared-account", Name: "A", City: "上海", IsVerify: 1}
	if err := clueRepo.Create(ctxA, clueA); err != nil {
		t.Fatalf("A 写入线索失败: %v", err)
	}
	clueB := &model.Clue{Type: 9, Account: "shared-account", IsVerify: 0}
	if err := clueRepo.Create(ctxB, clueB); err != nil {
		t.Fatalf("B 写入线索失败: %v", err)
	}

	if _, err := svc.ScoreClue(ctxA, clueA); err != nil {
		t.Fatalf("A 评分失败: %v", err)
	}
	if _, err := svc.ScoreClue(ctxB, clueB); err != nil {
		t.Fatalf("B 评分失败: %v", err)
	}

	scoreA, err := svc.GetByClueID(ctxA, clueA.ID)
	if err != nil {
		t.Fatalf("A 读取自己的评分失败: %v", err)
	}
	if _, err := svc.GetByClueID(ctxB, clueA.ID); !errors.Is(err, ErrClueScoreNotFound) {
		t.Errorf("B 不应读到 A 的评分，err=%v", err)
	}
	if scoreA.TotalScore < 0 || scoreA.TotalScore > 100 {
		t.Errorf("评分越界: %d", scoreA.TotalScore)
	}

	sys := tenant.WithSystemScope(context.Background())
	var scores []model.ClueScore
	if err := database.WithContext(sys).Order("tenant_id").Find(&scores).Error; err != nil {
		t.Fatalf("系统作用域回读评分失败: %v", err)
	}
	if len(scores) != 2 {
		t.Fatalf("两租户各应有 1 行评分，实际 %d", len(scores))
	}
	if scores[0].TenantID != clueScoreSvcTenantA || scores[1].TenantID != clueScoreSvcTenantB {
		t.Errorf("评分行的 tenant_id 章不对: %d, %d", scores[0].TenantID, scores[1].TenantID)
	}

	scoreB, err := svc.GetByClueID(ctxB, clueB.ID)
	if err != nil {
		t.Fatalf("B 读取自己的评分失败: %v", err)
	}

	// 温度回写只应改本租户的线索：重评 A 之后，B 的评分与温度都不应变化。
	if _, err := svc.ScoreClue(ctxA, clueA); err != nil {
		t.Fatalf("A 重评失败: %v", err)
	}
	scoreBAfter, err := svc.GetByClueID(ctxB, clueB.ID)
	if err != nil {
		t.Fatalf("B 复读评分失败: %v", err)
	}
	if scoreBAfter.TotalScore != scoreB.TotalScore {
		t.Errorf("A 重评改动了 B 的评分: %d -> %d", scoreB.TotalScore, scoreBAfter.TotalScore)
	}

	var clues []model.Clue
	if err := database.WithContext(sys).Order("tenant_id").Find(&clues).Error; err != nil {
		t.Fatalf("系统作用域回读线索失败: %v", err)
	}
	if len(clues) != 2 {
		t.Fatalf("应有 2 条线索，实际 %d", len(clues))
	}
	if clues[0].Level == "" {
		t.Errorf("A 的线索应被本租户评分回写温度")
	}
	if clues[1].Level != "cold" {
		t.Errorf("B 的线索温度应保持自身评分结果 cold，实际 %q", clues[1].Level)
	}
}

func TestClueScoreService_GetByClueIDTranslatesNotFound(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	database := testutil.NewTestDB(t, &model.Clue{}, &model.ClueScore{}, &model.ClueEngagementEvent{})
	svc := NewClueScoreService(database)
	ctx := tenant.WithTenantID(context.Background(), clueScoreSvcTenantA)

	if _, err := svc.GetByClueID(ctx, "missing-clue"); !errors.Is(err, ErrClueScoreNotFound) {
		t.Fatalf("不存在的评分应映射为 ErrClueScoreNotFound，err=%v", err)
	}
	if _, err := svc.LoadClueForScoring(ctx, "missing-clue"); !errors.Is(err, ErrClueNotFound) {
		t.Fatalf("不存在的线索应映射为 ErrClueNotFound，err=%v", err)
	}
}
