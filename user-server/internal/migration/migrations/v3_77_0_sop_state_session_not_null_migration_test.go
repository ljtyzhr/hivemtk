package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// D-6 验收判据（规划 §3.6）：
//
//	① 迁移后 information_schema.columns.is_nullable = 'NO'；
//	② 插入 NULL session_id 失败；
//	③ 存量行数与迁移前一致（本用例用"迁移前先插 2 行合法数据"钉住）；
//	④ 存在 NULL 行时迁移报错不执行（失败前置检查有牙）。
func TestSOPStateSessionIDNotNull(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	database := testutil.NewTestDBOrSkip(t, &model.SOPStateMemory{})
	ctx := context.Background()

	// 迁移前先插 2 行合法数据（存量行数一致性判据的锚）
	for _, sid := range []string{"sess-a", "sess-b"} {
		if err := database.Create(&model.SOPStateMemory{
			SessionID: sid, CustomerID: "c-1", SOPID: 1,
		}).Error; err != nil {
			t.Fatalf("插存量行失败: %v", err)
		}
	}

	m := NewSOPStateMemorySessionIDNotNullMigration(database)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	// ① is_nullable = NO
	nullable, err := columnIsNullable(database, "sop_state_memories", "session_id")
	if err != nil {
		t.Fatalf("查 is_nullable 失败: %v", err)
	}
	if nullable {
		t.Fatal("迁移后 session_id 仍可空 ⇒ 判据①不成立")
	}

	// ③ 存量行数不变
	var n int64
	if err := database.Table("sop_state_memories").Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("存量行数应仍为 2，实际 %d ⇒ 数据被动了", n)
	}

	// ② 插入 NULL 失败（绕过 GORM tag，直接 SQL 造 NULL 行）
	if err := database.Exec(
		`INSERT INTO sop_state_memories (tenant_id, session_id, customer_id, sop_id, status)
		 VALUES (1, NULL, 'c-x', 1, 'running')`).Error; err == nil {
		t.Fatal("插入 NULL session_id 竟然成功 ⇒ 判据②不成立（约束没有牙）")
	}

	// ④ 幂等：再跑一次仍成功
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移应幂等可重入，实际二次执行报错: %v", err)
	}
}

// TestSOPStateSessionIDNotNull_PrecheckBlocks 前置检查有牙：树里有 NULL 行时迁移必须报错，
// 且报错信息里带行数（运维能直接看到要处置多少行），而不是静默替他们删数据。
// 用手工可空表（不带新 tag 的旧形状）造环境，否则 AutoMigrate 按 NOT NULL tag 建表
// 时 NULL 行根本插不进去，这一格测不到。
func TestSOPStateSessionIDNotNull_PrecheckBlocks(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	database := testutil.NewTestDBOrSkip(t, &model.SOPStateMemory{})
	ctx := context.Background()

	// 把列退回旧形状（可空）——模拟"模型 tag 还没更新时的存量库"
	if err := database.Exec(
		`ALTER TABLE sop_state_memories ALTER COLUMN session_id DROP NOT NULL`).Error; err != nil {
		t.Fatalf("退回可空失败: %v", err)
	}
	// 造一条 NULL 行
	if err := database.Exec(
		`INSERT INTO sop_state_memories (tenant_id, session_id, customer_id, sop_id, status)
		 VALUES (1, NULL, 'c-bad', 1, 'running')`).Error; err != nil {
		t.Fatalf("造 NULL 行失败: %v", err)
	}

	m := NewSOPStateMemorySessionIDNotNullMigration(database)
	err := m.Up(ctx)
	if err == nil {
		t.Fatal("存在 NULL 行时迁移应报错拒绝执行（前置检查没牙）")
	}
	if got := err.Error(); !containsAll(got, "1", "NULL") {
		t.Fatalf("报错应含 NULL 行数与处置指引，实际: %v", err)
	}
	// 报错后列必须仍可空（迁移没有半途改形状）
	nullable, err := columnIsNullable(database, "sop_state_memories", "session_id")
	if err != nil {
		t.Fatalf("查 is_nullable 失败: %v", err)
	}
	if !nullable {
		t.Fatal("前置检查拒绝后列不应被改掉 ⇒ 迁移不是原子的")
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !stringContains(s, sub) {
			return false
		}
	}
	return true
}

func stringContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
