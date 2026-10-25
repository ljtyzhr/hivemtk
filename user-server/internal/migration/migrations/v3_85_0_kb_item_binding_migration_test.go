package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// v3.85.0（P0-2/P0-3/D-8 方案 A）验收判据：
//
//	① kb_item_bindings 表存在；
//	② 幂等重入：第二次 Up 不报错；
//	③ 双路同形：迁移建的表与模型 tag（AutoMigrate）建的表列形状一致；
//	④ 唯一约束生效：(kb_id,item_type,item_id)（经 tenancyidx 复合化为
//	 租户内唯一）重复写入报错；
//	⑤ Down 拒绝删表（declineTableDrop：归属事实降级不销毁）。
func TestKBItemBindingMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	// 只带 KBItemBinding 一个模型：验证迁移自身（而非 allModels）就能把表建起来
	database := testutil.NewTestDBOrSkip(t, &model.KBItemBinding{})
	ctx := context.Background()

	m := NewKBItemBindingMigration(database)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// ② 幂等重入
	if err := m.Up(ctx); err != nil {
		t.Fatalf("幂等重入失败: %v", err)
	}
	// ① 表存在
	if !database.Migrator().HasTable(&model.KBItemBinding{}) {
		t.Fatalf("kb_item_bindings 表不存在")
	}
	// ③ 列形状：迁移侧模型即 model.KBItemBinding，直接探测关键列
	for _, col := range []string{"kb_id", "item_type", "item_id", "bound_by", "created_at"} {
		if !database.Migrator().HasColumn(&model.KBItemBinding{}, col) {
			t.Fatalf("③ kb_item_bindings.%s 列缺失", col)
		}
	}
	// ④ 唯一约束：同 (tenant_id, kb_id, item_type, item_id) 重复写入必须报错
	b1 := &model.KBItemBinding{TenantID: 1, KBID: 7, ItemType: model.KBItemFAQ, ItemID: 42}
	if err := database.Create(b1).Error; err != nil {
		t.Fatalf("首条绑定写入失败: %v", err)
	}
	dup := &model.KBItemBinding{TenantID: 1, KBID: 7, ItemType: model.KBItemFAQ, ItemID: 42}
	if err := database.Create(dup).Error; err == nil {
		t.Fatalf("④ 唯一约束未生效：重复绑定 (kb_id=7,faq,42) 未被拒绝")
	}
	// 同内容绑另一个 KB（shared 语义）必须成功
	if err := database.Create(&model.KBItemBinding{
		TenantID: 1, KBID: 8, ItemType: model.KBItemFAQ, ItemID: 42, BoundBy: "t",
	}).Error; err != nil {
		t.Fatalf("shared 语义被误伤：同内容绑第二个 KB 报错 %v", err)
	}
	// ⑤ Down 不删表
	if err := m.Down(ctx); err != nil {
		t.Fatalf("⑤ Down 应拒绝删表而不报错: %v", err)
	}
	if !database.Migrator().HasTable(&model.KBItemBinding{}) {
		t.Fatalf("⑤ Down 不得销毁在用的 kb_item_bindings 表")
	}
}
