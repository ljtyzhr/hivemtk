// bridge_selector_health_test.go B1-a：计数落库在**真库**上跑。
//
// 为什么不在内存里测：这条路径的全部风险都在 SQL 落地那层——
// 累加写的是 `attempts + ?` 而不是整行覆盖、并发首建撞唯一键后要补加而不是丢弃。
// mock 只会复现"我以为的 SQL"。所以这里用 testutil 的真库，断言的是**列的值**。
package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
)

func setupSelectorHealthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, &model.BridgeSelectorHealth{})
	db.SetTestDB(database)
	return database
}

func TestBridgeSelectorHealthUpsert_CreatesThenAccumulates(t *testing.T) {
	setupSelectorHealthTestDB(t)
	repo := NewBridgeSelectorHealthRepository(db.GetDB())
	if repo == nil {
		t.Fatal("仓储未构造（db 为 nil？）")
	}
	ctx := context.Background()
	now := time.Now()

	if err := repo.Upsert(ctx, []SelectorHealthDelta{
		{Channel: "douyin", Field: "messageItem", Attempts: 10, Hits: 4, ReportedAt: now},
	}); err != nil {
		t.Fatalf("首建失败：%v", err)
	}
	if err := repo.Upsert(ctx, []SelectorHealthDelta{
		{Channel: "douyin", Field: "messageItem", Attempts: 5, Hits: 5, ReportedAt: now},
	}); err != nil {
		t.Fatalf("累加失败：%v", err)
	}
	rows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 失败：%v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("同一 (channel, field) 必须只有一行，实际 %d 行", len(rows))
	}
	if rows[0].Attempts != 15 || rows[0].Hits != 9 {
		t.Fatalf("累加结果应为 attempts=15 hits=9，实际 attempts=%d hits=%d",
			rows[0].Attempts, rows[0].Hits)
	}
	// FirstSeenAt 不许被后续累加刷成"最后一次"——它是「这个渠道什么时候开始被观测」的锚点。
	if rows[0].FirstSeenAt.IsZero() {
		t.Fatal("FirstSeenAt 不应为空")
	}
}

func TestBridgeSelectorHealthUpsert_FieldsAndChannelsAreIndependent(t *testing.T) {
	setupSelectorHealthTestDB(t)
	repo := NewBridgeSelectorHealthRepository(db.GetDB())
	ctx := context.Background()
	now := time.Now()

	if err := repo.Upsert(ctx, []SelectorHealthDelta{
		{Channel: "douyin", Field: "messageItem", Attempts: 3, Hits: 3, ReportedAt: now},
		{Channel: "douyin", Field: "input", Attempts: 7, Hits: 0, ReportedAt: now},
		{Channel: "xhs", Field: "messageItem", Attempts: 11, Hits: 1, ReportedAt: now},
	}); err != nil {
		t.Fatalf("批量累加失败：%v", err)
	}
	rows, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List 失败：%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("三个 (channel, field) 应各一行，实际 %d 行", len(rows))
	}
	// 排序：channel asc, field asc —— 运营面直接按行渲染，顺序不稳定等于每次刷新都变。
	if rows[0].Channel != "douyin" || rows[0].Field != "input" {
		t.Fatalf("首行应为 douyin/input，实际 %s/%s", rows[0].Channel, rows[0].Field)
	}
	if rows[2].Channel != "xhs" {
		t.Fatalf("末行应为 xhs，实际 %s", rows[2].Channel)
	}
}

func TestBridgeSelectorHealthUpsert_EmptyIsNoop(t *testing.T) {
	setupSelectorHealthTestDB(t)
	repo := NewBridgeSelectorHealthRepository(db.GetDB())
	if err := repo.Upsert(context.Background(), nil); err != nil {
		t.Fatalf("空增量应是无操作，实际报错 %v", err)
	}
	rows, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List 失败：%v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("空增量不应写行，实际 %d 行", len(rows))
	}
}

func TestBridgeSelectorHealthRepo_NilDBReturnsNil(t *testing.T) {
	if repo := NewBridgeSelectorHealthRepository(nil); repo != nil {
		t.Fatal("db 为 nil 时应返回 nil 接口（ARC-01 未初始化守卫）")
	}
}
