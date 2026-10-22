// clue_tenant_test.go clues 的租户契约（实体 #60 审计补充）。
//
// clues 的去重键 (type, account) 是**应用层判据**（service.Register 先查后写），
// 库里没有对应唯一索引 —— 因为软删除（deleted_at）让「唯一索引 + 重新导入」互相打架：
// 索引会把已软删的行也算进冲突。这带来一个必须钉死的风险：一旦去重查询漏掉租户谓词，
// A 租户下过的联系人会让 B 租户的同一联系人被判成「重复数据」而拒收。
//
// 本文件在真库上锁四件事：
//  1. 两租户的同 (type, account) 可以各自成行（唯一性只在租户内）；
//  2. 同租户内 ExistsByTypeAndAccount 能看见自己的行、看不见别家的；
//  3. 读侧（GetClueList）只返回本租户的行；
//  4. 裸 ctx 在 on 模式下 fail-closed（不回退成跨租户全量）。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	clueTenantA = uint(293)
	clueTenantB = uint(294)
)

func clueSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.Clue{})
	if db == nil {
		t.Fatal("测试库不可达：clues 的租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestClueRepository_TypeAccountUniquePerTenant(t *testing.T) {
	db := clueSetupDB(t)
	repo := NewClueRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), clueTenantA)
	ctxB := tenant.WithTenantID(context.Background(), clueTenantB)

	if err := repo.Create(ctxA, &model.Clue{Type: 9, Account: "same-account", Name: "A 的线索"}); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if err := repo.Create(ctxB, &model.Clue{Type: 9, Account: "same-account", Name: "B 的线索"}); err != nil {
		t.Fatalf("租户 B 同 (type, account) 应能独立成行: %v", err)
	}

	sys := tenant.WithSystemScope(context.Background())
	var rows []model.Clue
	if err := db.WithContext(sys).Order("tenant_id").Find(&rows).Error; err != nil {
		t.Fatalf("系统作用域回读失败: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("应有 2 行（每租户一条），实际 %d", len(rows))
	}
	if rows[0].TenantID != clueTenantA || rows[1].TenantID != clueTenantB {
		t.Fatalf("tenant_id 章不对: %d, %d", rows[0].TenantID, rows[1].TenantID)
	}

	// 去重判据的读侧必须与写侧同租户。
	existsA, err := repo.ExistsByTypeAndAccount(ctxA, 9, "same-account")
	if err != nil || !existsA {
		t.Fatalf("A 应看见自己的行: exists=%v err=%v", existsA, err)
	}
	existsB, err := repo.ExistsByTypeAndAccount(ctxB, 9, "same-account")
	if err != nil || !existsB {
		t.Fatalf("B 应看见自己的行（不是 A 的行也算）: exists=%v err=%v", existsB, err)
	}
	if err := repo.Create(ctxB, &model.Clue{Type: 9, Account: "b-only-account", Name: "B 独有"}); err != nil {
		t.Fatalf("B 写入独有账号失败: %v", err)
	}
	if exists, err := repo.ExistsByTypeAndAccount(ctxA, 9, "b-only-account"); err != nil || exists {
		t.Fatalf("A 不应看见 B 的账号: exists=%v err=%v", exists, err)
	}

	// 读侧遮挡：A 的列表里只有 A 的行。
	listA, total, err := repo.GetClueList(ctxA, 1, 50)
	if err != nil {
		t.Fatalf("GetClueList(A): %v", err)
	}
	if total != 1 || len(listA) != 1 {
		t.Fatalf("A 只该看见 1 行，实际 total=%d len=%d", total, len(listA))
	}
	if listA[0].TenantID != clueTenantA {
		t.Errorf("读回的行不属于 A: tenant_id=%d", listA[0].TenantID)
	}
}

// TestClueRepository_RequiresTenantContext on 模式下无租户绑定的查询必须 fail-closed，
// 不能退化成跨租户全量。
func TestClueRepository_RequiresTenantContext(t *testing.T) {
	repo := NewClueRepositoryWithDB(clueSetupDB(t))
	if _, _, err := repo.GetClueList(context.Background(), 1, 10); err == nil {
		t.Fatal("on 模式下裸 ctx 查询应被隔离 callback 拒绝")
	}
	if _, err := repo.ExistsByTypeAndAccount(context.Background(), 9, "x"); err == nil {
		t.Fatal("on 模式下裸 ctx 去重查询应被拒绝")
	}
}

// TestClueRepository_GetClueListNormalizesPaging page=0/limit=0 不能落成负 Offset
// 或「不限制」（PG 语义），否则列表页首屏直接 500 / 全表载入。
func TestClueRepository_GetClueListNormalizesPaging(t *testing.T) {
	db := clueSetupDB(t)
	repo := NewClueRepositoryWithDB(db)
	ctx := tenant.WithTenantID(context.Background(), clueTenantA)

	for i := 0; i < 3; i++ {
		if err := repo.Create(ctx, &model.Clue{Type: 9, Account: "paging-" + string(rune('a'+i))}); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}
	rows, total, err := repo.GetClueList(ctx, 0, 0)
	if err != nil {
		t.Fatalf("page=0/limit=0 应被归一后正常返回: %v", err)
	}
	if total != 3 {
		t.Errorf("total=%d want 3", total)
	}
	if len(rows) != 3 {
		t.Errorf("limit=0 应归一为 20（不是 PG 的不限制），行数=%d", len(rows))
	}
}
