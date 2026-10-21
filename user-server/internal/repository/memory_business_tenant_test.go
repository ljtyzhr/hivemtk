package repository

// 契约锁（实体审查 #58，BusinessMemory）：计数与淘汰链上的查询不许把错误吞成「0 条」。
//
// 缺陷本体：`CountBusinessMemoriesByCustomer` 调 `Count(&count)` 之后**没有取 .Error**，
// 直接 `return count, nil`。于是任何一次查询失败（连接池打满、DDL 锁、表被摘掉）
// 都返回「这个客户有 0 条记忆」——而这个数字的用途是**容量闸**：
// `MemorySystem.L4Add` 拿它判「count+1 > L4MaxPerCust(500) 才淘汰」。
//
// 失败方向：count 被读成 0 ⇒ 永远不触发淘汰 ⇒ L4 记忆行无界增长。
// 而这一格是**静默**的：淘汰该发生时没发生，界面上看不出任何异常，
// 只有「这个客户的记忆越攒越多」这一种表现。
//
// 闸门该往哪边倒：宁可「判定不出 ⇒ 不淘汰」（保守侧，本实现修成返回错误让上层
// 显式决策）也不能「判定成 0 ⇒ 闸门关掉」。后者把「读不到」谎报成「没有」，
// 与本仓反复出现的那条纪律同一条：读不动 ≠ 不存在。

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	bmTenantA  = uint(21)
	bmTenantB  = uint(43)
	bmCustomer = "cust-bm-1"
)

// TestCountBusinessMemoriesReportsErrorOnFailure 计数失败必须报错，不能返回 (0, nil)。
//
// 制造失败的手段：把 business_memories 表摘掉——查询必然报错，而「0 条」这个答案
// 在这张表不存在时与「真的没有记忆」逐字节同形。
func TestCountBusinessMemoriesReportsErrorOnFailure(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.BusinessMemory{})
	if db == nil {
		t.Fatal("测试库不可达：计数口径无法判定（不 Skip，跳过等于没锁）")
	}
	ctxA := tenant.WithTenantID(context.Background(), bmTenantA)
	repo := NewMemoryRepositoryWithDB(db)

	// 前提：表在、计数正常。
	if n, err := repo.CountBusinessMemoriesByCustomer(ctxA, bmCustomer); err != nil || n != 0 {
		t.Fatalf("前提不成立：计数返回 (%d, %v)，want (0, nil)", n, err)
	}

	// 制造查询失败：用一个已取消的 ctx（查询立刻返回 context.Canceled）。
	//
	// 为什么不用「摘表」制造失败：Migrator 的 DDL 走预构建 SQL，被租户 raw 守卫拦下
	// （"预构建 SQL 引用租户表 business_memories 被拒绝"），那测到的是另一件事。
	// 也不去关连接池：测试库是**进程级共享**的，关掉它会让本包后续每一条用例全红。
	cancelled, cancel := context.WithCancel(ctxA)
	cancel()

	n, err := repo.CountBusinessMemoriesByCustomer(cancelled, bmCustomer)
	if err == nil {
		t.Errorf("计数失败却返回了无错（%d 条）："+
			"这个数字是 L4 的容量闸（count+1>500 才淘汰），"+
			"读失败被读成 0 ⇒ 闸门永远不关 ⇒ 记忆行无界增长且全程无声", n)
	}
}

// TestBusinessMemoryListIsTenantScoped 同一 customer_id 在两租户各有一条：各自只读得到自己那条。
//
// 种子刻意让两个租户用**同一个** customer_id：那样断言的才是 tenant_id 谓词本身，
// 而不是「两个租户的客户号恰好不撞」这种随时会被复用拆穿的巧合。
func TestBusinessMemoryListIsTenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.BusinessMemory{})
	if db == nil {
		t.Fatal("测试库不可达：跨租户可读性无法判定")
	}
	ctxA := tenant.WithTenantID(context.Background(), bmTenantA)
	ctxB := tenant.WithTenantID(context.Background(), bmTenantB)
	repo := NewMemoryRepositoryWithDB(db)

	if err := db.WithContext(ctxA).Create(&model.BusinessMemory{
		CustomerID: bmCustomer, MemoryType: "order", Content: "A 的记忆", Importance: 5,
	}).Error; err != nil {
		t.Fatalf("A 租户写入失败: %v", err)
	}
	if err := db.WithContext(ctxB).Create(&model.BusinessMemory{
		CustomerID: bmCustomer, MemoryType: "order", Content: "B 的记忆", Importance: 5,
	}).Error; err != nil {
		t.Fatalf("B 租户写入失败: %v", err)
	}

	for _, tc := range []struct {
		name        string
		ctx         context.Context
		wantContent string
	}{
		{"tenantA", ctxA, "A 的记忆"},
		{"tenantB", ctxB, "B 的记忆"},
	} {
		list, err := repo.ListBusinessMemories(tc.ctx, bmCustomer, "", 10)
		if err != nil {
			t.Fatalf("%s 读取失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s 读到 %d 条 want=1（多出来的那条是别的租户的记忆）", tc.name, len(list))
			continue
		}
		if list[0].Content != tc.wantContent {
			t.Errorf("%s 读到的内容 = %q want %q", tc.name, list[0].Content, tc.wantContent)
		}
	}

	// 裸 ctx 必须 fail-closed：读侧一旦放行，「按客户号读别人的记忆」就写回了库里。
	if _, err := repo.ListBusinessMemories(context.Background(), bmCustomer, "", 10); err == nil {
		t.Error("裸 ctx 的 ListBusinessMemories 未被拒绝")
	}
}

// TestBusinessMemoryHardDeleteStaysInTenant 硬删（Unscoped）不许跨租户。
//
// 这条专门盯 `DeleteBusinessMemoriesByIDs`：它走 Unscoped()（硬删，因为软删的行会继续
// 占着读取面），而 Unscoped 只影响软删过滤、**不影响租户谓词**——租户条件仍由隔离
// callback 注入。判据：A 租户拿 B 租户的行 id 去删，必须一行都删不掉。
//
// 前提：A 租户的 ctx 确实绑定了租户（否则「一行都没删」是因为 ctx 未绑、被 fail-closed
// 挡下，而不是因为租户谓词生效——那两件事必须分开验）。
func TestBusinessMemoryHardDeleteStaysInTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.ResetRegistryForTest(); tenant.SetMode(tenant.ModeOff) })

	db := testutil.NewTestDB(t, &model.BusinessMemory{})
	if db == nil {
		t.Fatal("测试库不可达：硬删的跨租户面无法判定")
	}
	ctxA := tenant.WithTenantID(context.Background(), bmTenantA)
	ctxB := tenant.WithTenantID(context.Background(), bmTenantB)
	repo := NewMemoryRepositoryWithDB(db)

	// 前提一：同租户删得掉（否则「跨租户也删不掉」可能只是删除口本身坏了）。
	own := &model.BusinessMemory{CustomerID: bmCustomer, MemoryType: "order", Content: "A 的记忆", Importance: 5}
	if err := db.WithContext(ctxA).Create(own).Error; err != nil {
		t.Fatalf("A 租户写入失败: %v", err)
	}
	if err := repo.DeleteBusinessMemoriesByIDs(ctxA, []uint{own.ID}); err != nil {
		t.Fatalf("A 租户删自己的行报错: %v", err)
	}
	var ownLeft int64
	if err := db.WithContext(ctxA).Model(&model.BusinessMemory{}).Count(&ownLeft).Error; err != nil {
		t.Fatalf("读 A 租户行数失败: %v", err)
	}
	if ownLeft != 0 {
		t.Fatalf("前提一不成立：A 租户删自己的行后还剩 %d 行 want=0（删除口本身坏了，"+
			"后面的「跨租户删不掉」就分不清是租户谓词生效还是口坏了）", ownLeft)
	}

	// 前提二：B 的行确实存在。
	bRow := &model.BusinessMemory{CustomerID: bmCustomer, MemoryType: "order", Content: "B 的记忆", Importance: 5}
	if err := db.WithContext(ctxB).Create(bRow).Error; err != nil {
		t.Fatalf("B 租户写入失败: %v", err)
	}

	// 判据：A 租户去删 B 的行（按 id 删，无 customer_id 过滤——过滤只能来自隔离 callback）。
	if err := repo.DeleteBusinessMemoriesByIDs(ctxA, []uint{bRow.ID}); err != nil {
		t.Fatalf("跨租户硬删报错（应当是「删不掉」而不是报错）: %v", err)
	}

	var left int64
	if err := db.WithContext(ctxB).Model(&model.BusinessMemory{}).Count(&left).Error; err != nil {
		t.Fatalf("读 B 租户行数失败: %v", err)
	}
	if left != 1 {
		t.Errorf("B 租户的记忆被别的租户删掉了（剩 %d 行 want=1）："+
			"隔离 callback 没把 tenant_id 条件注进 WHERE", left)
	}
}
