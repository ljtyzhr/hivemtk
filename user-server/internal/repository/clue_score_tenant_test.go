// clue_score_tenant_test.go clue_scores / clue_engagement_events 的租户契约（实体 #61 审计补充）。
//
// 两张表的唯一性都只在租户内成立：clue_scores.clue_id 的 uniqueIndex 会被 tenancyidx
// 复合化成 (tenant_id, clue_id)，互动事件则按 tenant_id 隔离计数。本文件在真库上锁四件事：
//  1. 两租户的同 clue_id 可以各自成行（唯一性只在租户内）；
//  2. Upsert 的更新路径只命中本租户的行，不会改掉别家的评分；
//  3. 读侧（GetByClueID / ListByGrade）与互动计数只返回本租户的数据；
//  4. 裸 ctx 在 on 模式下 fail-closed（不回退成跨租户全量）。
package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	clueScoreTenantA = uint(301)
	clueScoreTenantB = uint(302)
)

func clueScoreSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ClueScore{}, &model.ClueEngagementEvent{})
	if db == nil {
		t.Fatal("测试库不可达：clue_scores 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestClueScoreRepository_UpsertIsTenantScoped(t *testing.T) {
	db := clueScoreSetupDB(t)
	repo := NewClueScoreRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), clueScoreTenantA)
	ctxB := tenant.WithTenantID(context.Background(), clueScoreTenantB)

	if err := repo.Upsert(ctxA, &model.ClueScore{ClueID: "clue-shared", Account: "a", TotalScore: 10, Grade: "D"}); err != nil {
		t.Fatalf("租户 A 首次 Upsert 失败: %v", err)
	}
	if err := repo.Upsert(ctxB, &model.ClueScore{ClueID: "clue-shared", Account: "b", TotalScore: 20, Grade: "D"}); err != nil {
		t.Fatalf("租户 B 同 clue_id 应能独立成行: %v", err)
	}

	// 第二次 Upsert 走「已存在则更新」分支：只应改动 A 自己的行。
	if err := repo.Upsert(ctxA, &model.ClueScore{ClueID: "clue-shared", Account: "a", TotalScore: 88, Grade: "A"}); err != nil {
		t.Fatalf("租户 A 二次 Upsert 失败: %v", err)
	}

	sys := tenant.WithSystemScope(context.Background())
	var rows []model.ClueScore
	if err := db.WithContext(sys).Order("tenant_id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有 2 行（每租户一条），实际 %d", len(rows))
	}
	if rows[0].TenantID != clueScoreTenantA || rows[0].TotalScore != 88 || rows[0].Grade != "A" {
		t.Errorf("A 的行未被更新或串了租户: tenant=%d score=%d grade=%s", rows[0].TenantID, rows[0].TotalScore, rows[0].Grade)
	}
	if rows[1].TenantID != clueScoreTenantB || rows[1].TotalScore != 20 {
		t.Errorf("B 的行被 A 的 Upsert 改掉了: tenant=%d score=%d", rows[1].TenantID, rows[1].TotalScore)
	}

	scoreA, err := repo.GetByClueID(ctxA, "clue-shared")
	if err != nil {
		t.Fatalf("A 读取自己的评分失败: %v", err)
	}
	if scoreA.TotalScore != 88 {
		t.Errorf("A 读到 score=%d want 88", scoreA.TotalScore)
	}
	scoreB, err := repo.GetByClueID(ctxB, "clue-shared")
	if err != nil {
		t.Fatalf("B 读取自己的评分失败: %v", err)
	}
	if scoreB.TotalScore != 20 {
		t.Errorf("B 读到 score=%d want 20", scoreB.TotalScore)
	}

	if _, err := repo.GetByClueID(ctxB, "clue-only-a"); err == nil {
		t.Error("不存在的 clue_id 应返回错误")
	}
}

func TestClueScoreRepository_ListAndEngagementAreTenantScoped(t *testing.T) {
	db := clueScoreSetupDB(t)
	scoreRepo := NewClueScoreRepositoryWithDB(db)
	engageRepo := NewClueEngagementRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), clueScoreTenantA)
	ctxB := tenant.WithTenantID(context.Background(), clueScoreTenantB)

	if err := scoreRepo.Upsert(ctxA, &model.ClueScore{ClueID: "clue-a", TotalScore: 90, Grade: "S"}); err != nil {
		t.Fatalf("A 写入评分失败: %v", err)
	}
	if err := scoreRepo.Upsert(ctxB, &model.ClueScore{ClueID: "clue-b", TotalScore: 40, Grade: "C"}); err != nil {
		t.Fatalf("B 写入评分失败: %v", err)
	}

	listA, total, err := scoreRepo.ListByGrade(ctxA, "", 1, 50)
	if err != nil {
		t.Fatalf("ListByGrade(A): %v", err)
	}
	if total != 1 || len(listA) != 1 || listA[0].ClueID != "clue-a" {
		t.Fatalf("A 只该看见自己的 1 行，实际 total=%d len=%d", total, len(listA))
	}

	for i := 0; i < 3; i++ {
		if err := engageRepo.Create(ctxA, &model.ClueEngagementEvent{ClueID: "clue-shared", EventType: "click"}); err != nil {
			t.Fatalf("A 写入互动事件失败: %v", err)
		}
	}
	if err := engageRepo.Create(ctxB, &model.ClueEngagementEvent{ClueID: "clue-shared", EventType: "click"}); err != nil {
		t.Fatalf("B 写入互动事件失败: %v", err)
	}

	since := time.Time{}
	countA, err := engageRepo.CountByClueID(ctxA, "clue-shared", since)
	if err != nil {
		t.Fatalf("CountByClueID(A): %v", err)
	}
	if countA != 3 {
		t.Errorf("A 的互动计数=%d want 3（不应算进 B 的事件）", countA)
	}
	batchA, err := engageRepo.CountByClueIDsBatch(ctxA, []string{"clue-shared"}, since)
	if err != nil {
		t.Fatalf("CountByClueIDsBatch(A): %v", err)
	}
	if batchA["clue-shared"] != 3 {
		t.Errorf("A 的批量计数=%d want 3", batchA["clue-shared"])
	}
}

func TestClueScoreRepository_RequiresTenantContext(t *testing.T) {
	db := clueScoreSetupDB(t)
	scoreRepo := NewClueScoreRepositoryWithDB(db)
	engageRepo := NewClueEngagementRepositoryWithDB(db)

	if err := scoreRepo.Upsert(context.Background(), &model.ClueScore{ClueID: "no-tenant", TotalScore: 10}); err == nil {
		t.Fatal("on 模式下裸 ctx 写入应被隔离 callback 拒绝")
	}
	if _, err := scoreRepo.GetByClueID(context.Background(), "no-tenant"); err == nil {
		t.Fatal("on 模式下裸 ctx 查询应被拒绝")
	}
	if _, _, err := scoreRepo.ListByGrade(context.Background(), "", 1, 10); err == nil {
		t.Fatal("on 模式下裸 ctx 列表应被拒绝")
	}
	if _, err := engageRepo.CountByClueID(context.Background(), "no-tenant", time.Time{}); err == nil {
		t.Fatal("on 模式下裸 ctx 计数应被拒绝")
	}
}

// TestClueScoreRepository_UpsertRejectsEmptyClueID 空 clue_id 是调用方错误，
// 不能在库里落一行无主评分。
func TestClueScoreRepository_UpsertRejectsEmptyClueID(t *testing.T) {
	repo := NewClueScoreRepositoryWithDB(clueScoreSetupDB(t))
	ctx := tenant.WithTenantID(context.Background(), clueScoreTenantA)
	if err := repo.Upsert(ctx, &model.ClueScore{TotalScore: 50}); err == nil {
		t.Fatal("空 clue_id 应被拒绝")
	}
}
