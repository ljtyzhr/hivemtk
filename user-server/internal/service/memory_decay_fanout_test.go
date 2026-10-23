// memory_decay_fanout_test.go 钉住 L-4 记忆衰减归档的两条装配契约。
//
// 背景：这个作业此前**只有实现没有装配点**——`StartMemoryDecayLoop` 在全仓库
// 没有任何生产调用方，于是 memory_items / customer_long_term_memory 的 invalid_at
// 永远不会被写，L2 事实只进不出。补装配时有两个人容易踩的坑，正是这里要钉的：
//
//  1. 用裸 ctx 一把梭全表扫。两张都是租户表，on 模式下隔离 callback 只认 ctx 里的
//     租户 ⇒ fail-closed，一行也读不到，而"读不到"和"没数据可归档"在统计上长得
//     一模一样（scanned=0），作业会以成功的样子空转。正解是 tenant.Fanout。
//  2. 拿 SystemScope 当后台任务的万能钥匙。那样确实能扫全表，但绕开了整套隔离，
//     且下游 Update 在 SystemScope 下不填列——归档动作本身就成了跨租户写入面。
//
// 所以判据是「两个租户各自的过期行都被归档、且彼此没串」，加上「枚举器缺失时
// 必须报错而不是静默成功」。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

func mdSeedDecayItem(t *testing.T, db *gorm.DB, tenantID uint, customer string, importance int) *model.MemoryItem {
	t.Helper()
	row := &model.MemoryItem{
		TenantID:   tenantID,
		Layer:      model.MemoryLayerLongTerm,
		CustomerID: customer,
		ItemType:   "fact",
		Content:    "c-" + customer,
		Importance: importance,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的事实行失败: %v", tenantID, err)
	}
	return row
}

func mdSeedDecayMemory(t *testing.T, db *gorm.DB, tenantID uint, customer string, importance int) *model.CustomerLongTermMemory {
	t.Helper()
	row := &model.CustomerLongTermMemory{
		TenantID:   tenantID,
		CustomerID: customer,
		MemoryType: model.LongTermMemoryPreference,
		Content:    "m-" + customer,
		Importance: importance,
		Source:     model.LongTermMemorySourceConversation,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Omit("Embedding"). // vector(1024) 不接受空串，NULL 才是"未算向量"
		Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的客户长期记忆失败: %v", tenantID, err)
	}
	return row
}

func mdItemPhysical(t *testing.T, db *gorm.DB, id uint) *model.MemoryItem {
	t.Helper()
	var row model.MemoryItem
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error; err != nil {
		t.Fatalf("物理读取 memory_items(%d) 失败: %v", id, err)
	}
	return &row
}

func mdMemoryPhysical(t *testing.T, db *gorm.DB, id uint64) *model.CustomerLongTermMemory {
	t.Helper()
	var row model.CustomerLongTermMemory
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error; err != nil {
		t.Fatalf("物理读取 customer_long_term_memory(%d) 失败: %v", id, err)
	}
	return &row
}

// TestMemoryDecayAllTenants_FansOutPerTenant 从生产同款的 context.Background()
// 入口调度，只有 Fanout 真正逐租户跑起来，两个租户的过期行才会同时被软失效。
func TestMemoryDecayAllTenants_FansOutPerTenant(t *testing.T) {
	seedOnModeTenantEnv(t, []uint{91, 92})
	db := testutil.NewTestDB(t, &model.MemoryItem{}, &model.CustomerLongTermMemory{})
	if db == nil {
		t.Fatal("测试库不可达：扇出判不了（不 Skip，跳过等于没锁）")
	}

	// importance=1 ⇒ confidence=0.1 < 0.2，判定不依赖时间（确定性归档）
	staleItemA := mdSeedDecayItem(t, db, 91, "cA", 1)
	staleItemB := mdSeedDecayItem(t, db, 92, "cB", 1)
	staleMemA := mdSeedDecayMemory(t, db, 91, "cA", 1)
	staleMemB := mdSeedDecayMemory(t, db, 92, "cB", 1)
	// 新鲜行（confidence=0.9、刚创建）不该被动到
	freshItem := mdSeedDecayItem(t, db, 91, "cFresh", 9)

	stats, err := RunMemoryDecayAllTenants(context.Background(), db, false)
	if err != nil {
		t.Fatalf("跨租户衰减失败: %v", err)
	}
	if stats.Archived != 4 {
		t.Fatalf("archived=%d，期望 4（两租户各 1 事实行 + 1 长期记忆）", stats.Archived)
	}

	for _, c := range []struct {
		id   uint
		want bool
		why  string
	}{
		{staleItemA.ID, true, "租户 91 的过期事实行没被软失效 ⇒ 扇出没覆盖到它"},
		{staleItemB.ID, true, "租户 92 的过期事实行没被软失效 ⇒ 扇出只跑了第一个租户"},
		{freshItem.ID, false, "新鲜事实行被归档 ⇒ 衰减判定/阈值被改坏了"},
	} {
		// 每次读都用新变量：复用带主键的 dest 会让 GORM 把旧主键当成附加条件
		// （表现为 record not found，而不是读错行）。
		got := mdItemPhysical(t, db, c.id)
		if (got.InvalidAt != nil) != c.want {
			t.Fatal(c.why)
		}
	}

	for _, id := range []uint64{staleMemA.ID, staleMemB.ID} {
		got := mdMemoryPhysical(t, db, id)
		if got.InvalidAt == nil {
			t.Fatalf("客户长期记忆(%d)没被软失效 ⇒ 第二张表的归档没生效", id)
		}
	}
}

// TestMemoryDecayAllTenants_FailClosedWithoutLoader 枚举器未装配时必须报错。
// 静默返回成功会让 on 模式下的 L-4 以 scanned=0 的样子长期空转——这正是它被漏装
// 这么久还没人发现的原因，所以这条必须红。
func TestMemoryDecayAllTenants_FailClosedWithoutLoader(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	tenant.SetActiveTenantLoader(nil)
	t.Cleanup(func() {
		tenant.SetActiveTenantLoader(nil)
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.MemoryItem{}, &model.CustomerLongTermMemory{})
	if db == nil {
		t.Fatal("测试库不可达：fail-closed 判不了（不 Skip，跳过等于没锁）")
	}
	mdSeedDecayItem(t, db, 91, "cA", 1)

	stats, err := RunMemoryDecayAllTenants(context.Background(), db, false)
	if err == nil {
		t.Fatalf("枚举器未装配却返回成功（stats=%+v）⇒ on 模式下会静默空转", stats)
	}
	if stats.Archived != 0 {
		t.Fatalf("fail-closed 路径上仍然归档了 %d 行", stats.Archived)
	}
}

// TestMemoryDecayAllTenants_DryRunDoesNotWrite 干跑只统计不落 invalid_at。
func TestMemoryDecayAllTenants_DryRunDoesNotWrite(t *testing.T) {
	seedOnModeTenantEnv(t, []uint{91})
	db := testutil.NewTestDB(t, &model.MemoryItem{}, &model.CustomerLongTermMemory{})
	if db == nil {
		t.Fatal("测试库不可达：干跑判不了（不 Skip，跳过等于没锁）")
	}
	stale := mdSeedDecayItem(t, db, 91, "cA", 1)

	stats, err := RunMemoryDecayAllTenants(context.Background(), db, true)
	if err != nil {
		t.Fatalf("干跑失败: %v", err)
	}
	if stats.Stale != 1 || stats.Archived != 0 {
		t.Fatalf("干跑统计 stale=%d archived=%d，期望 stale=1 archived=0", stats.Stale, stats.Archived)
	}
	if got := mdItemPhysical(t, db, stale.ID); got.InvalidAt != nil {
		t.Fatal("干跑写入了 invalid_at ⇒ dryRun 语义被破坏")
	}
}
