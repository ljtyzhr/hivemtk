// memory_decay_tenant_test.go L-4 衰减归档仓储的租户契约。
//
// 这个仓储**不收显式 tenantID**，隔离完全靠 ctx（RunMemoryDecayAllTenants 经
// tenant.Fanout 逐租户给 ctx）。因此这里要钉住的不是"参数传对不对"，而是：
//   - 单租户 ctx 只能读到自己的行（读侧过滤生效）；
//   - 软失效 UPDATE 的 id 列表即使混进别的租户的 id，也物理改不动那一行
//     （写侧过滤生效）——这是 L-4 唯一的写动作，串了就是"给别的租户归档数据"；
//   - on 模式无租户 ctx 直接报错，而不是退化成全表（fail-closed 仍是 fail-closed）。
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
	mdTenantA = uint(91)
	mdTenantB = uint(92)
)

func mdSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.MemoryItem{}, &model.CustomerLongTermMemory{})
	if db == nil {
		t.Fatal("测试库不可达：租户读写隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func mdSeedItem(t *testing.T, db *gorm.DB, tenantID uint, content string) *model.MemoryItem {
	t.Helper()
	row := &model.MemoryItem{
		TenantID:   tenantID,
		Layer:      model.MemoryLayerLongTerm,
		CustomerID: "c-" + content,
		ItemType:   "fact",
		Content:    content,
		Importance: 1, // confidence=0.1 ⇒ 命中归档阈值，判定与时间无关
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的事实行失败: %v", tenantID, err)
	}
	return row
}

func mdSeedMemory(t *testing.T, db *gorm.DB, tenantID uint, content string) *model.CustomerLongTermMemory {
	t.Helper()
	row := &model.CustomerLongTermMemory{
		TenantID:   tenantID,
		CustomerID: "c-" + content,
		MemoryType: model.LongTermMemoryPreference,
		Content:    content,
		Importance: 1,
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

func TestMemoryDecay_ListIsTenantScoped(t *testing.T) {
	db := mdSetupDB(t)
	itemA := mdSeedItem(t, db, mdTenantA, "A-事实")
	mdSeedItem(t, db, mdTenantB, "B-事实")
	memA := mdSeedMemory(t, db, mdTenantA, "A-偏好")
	mdSeedMemory(t, db, mdTenantB, "B-偏好")

	repo := NewMemoryDecayRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), mdTenantA)

	items, err := repo.ListActiveLongTermItems(ctxA, 100)
	if err != nil {
		t.Fatalf("租户 A 扫描事实行失败: %v", err)
	}
	if len(items) != 1 || items[0].ID != itemA.ID {
		t.Fatalf("租户 A 扫到 %d 行（%+v），期望只有自己的行 %d", len(items), items, itemA.ID)
	}

	mems, err := repo.ListActiveLongTermMemory(ctxA, 100)
	if err != nil {
		t.Fatalf("租户 A 扫描客户长期记忆失败: %v", err)
	}
	if len(mems) != 1 || mems[0].ID != memA.ID {
		t.Fatalf("租户 A 扫到 %d 行客户长期记忆（%+v），期望只有自己的行 %d", len(mems), mems, memA.ID)
	}
}

func TestMemoryDecay_MarkInvalidCannotCrossTenant(t *testing.T) {
	db := mdSetupDB(t)
	itemA := mdSeedItem(t, db, mdTenantA, "A-事实")
	itemB := mdSeedItem(t, db, mdTenantB, "B-事实")
	memA := mdSeedMemory(t, db, mdTenantA, "A-偏好")
	memB := mdSeedMemory(t, db, mdTenantB, "B-偏好")

	repo := NewMemoryDecayRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), mdTenantA)
	now := time.Now()

	// id 列表故意混进 B 的 id：写侧过滤没生效的话，B 的行会被 A 的作业归档掉。
	if err := repo.MarkItemsInvalid(ctxA, []uint{itemA.ID, itemB.ID}, now); err != nil {
		t.Fatalf("租户 A 软失效事实行失败: %v", err)
	}
	if got := mdItemPhysical(t, db, itemA.ID); got.InvalidAt == nil {
		t.Fatal("租户 A 自己的行没被软失效 ⇒ 作业实际没生效")
	}
	if got := mdItemPhysical(t, db, itemB.ID); got.InvalidAt != nil {
		t.Fatal("租户 A 的作业改到了租户 B 的事实行 ⇒ 跨租户写")
	}

	if err := repo.MarkLongTermMemoryInvalid(ctxA, []uint64{memA.ID, memB.ID}, now); err != nil {
		t.Fatalf("租户 A 软失效客户长期记忆失败: %v", err)
	}
	if got := mdMemoryPhysical(t, db, memA.ID); got.InvalidAt == nil {
		t.Fatal("租户 A 自己的客户长期记忆没被软失效 ⇒ 作业实际没生效")
	}
	if got := mdMemoryPhysical(t, db, memB.ID); got.InvalidAt != nil {
		t.Fatal("租户 A 的作业改到了租户 B 的客户长期记忆 ⇒ 跨租户写")
	}
}

func TestMemoryDecay_UnboundCtxIsRejected(t *testing.T) {
	db := mdSetupDB(t)
	mdSeedItem(t, db, mdTenantA, "A-事实")
	repo := NewMemoryDecayRepository(db)

	if _, err := repo.ListActiveLongTermItems(context.Background(), 100); err == nil {
		t.Fatal("on 模式无租户 ctx 扫到了事实行 ⇒ 会归档到别的租户的数据")
	}
	if _, err := repo.ListActiveLongTermMemory(context.Background(), 100); err == nil {
		t.Fatal("on 模式无租户 ctx 扫到了客户长期记忆 ⇒ 会归档到别的租户的数据")
	}
}
