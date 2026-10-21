// bill_tenant_test.go 账单域的租户契约（T-P7-01）。
//
// 账单是凭证表：`uq_bills_quote_row` 经 tenancyidx.Normalize 后是
// (tenant_id, quote_row_id)——A 租户某一版报价开过账单，不得让 B 租户
// 同版本行的派生被判「已派生过」，否则 B 的应收静默缺失。
// 读侧必须双向遮挡；裸 ctx 必须撞租户闸而不是被 billOrNil 读成「还没有账单」
// （那会让一次库故障/漏绑 ctx 的派生再插一张）。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	billTenantA = uint(261)
	billTenantB = uint(262)
)

func billSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.Bill{})
	if db == nil {
		t.Fatal("测试库不可达：账单的租户隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func billRow(id, quoteRowID string, dueAt *time.Time) *model.Bill {
	return &model.Bill{
		ID:            id,
		QuoteID:       "q_shared",
		QuoteRowID:    quoteRowID,
		OpportunityID: "opp_shared",
		Amount:        369.99,
		Currency:      model.BillCurrencyDefault,
		DueAt:         dueAt,
		Status:        model.BillStatusOpen,
	}
}

func TestBillRepository_TenantScopeAndCompositeIdempotencyKey(t *testing.T) {
	db := billSetupDB(t)
	repo := NewBillRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), billTenantA)
	ctxB := tenant.WithTenantID(context.Background(), billTenantB)
	sys := tenant.WithSystemScope(context.Background())

	past := time.Now().Add(-48 * time.Hour)
	a := billRow("b_a_1", "q_row_shared", &past)
	if err := repo.Create(ctxA, a); err != nil {
		t.Fatalf("租户 A 派生账单应成功: %v", err)
	}
	var stamped model.Bill
	if err := db.WithContext(sys).First(&stamped, "id = ?", a.ID).Error; err != nil {
		t.Fatalf("回读租户 A 账单失败: %v", err)
	}
	if stamped.TenantID != billTenantA {
		t.Fatalf("tenant_id=%d, want %d（应由 ctx 盖章）", stamped.TenantID, billTenantA)
	}

	// 同一版本行在 B 租户必须能独立派生：单列唯一键会在这里回 ErrBillAlreadyDerived。
	b := billRow("b_b_1", "q_row_shared", &past)
	if err := repo.Create(ctxB, b); err != nil {
		t.Fatalf("跨租户同 quote_row_id 应可各派生一张（唯一键须为 tenant_id 复合）: %v", err)
	}

	// 按版本行的幂等读：各租户只命中自己那张。
	gotA, err := repo.GetByQuoteRowID(ctxA, "q_row_shared")
	if err != nil || gotA == nil || gotA.ID != a.ID {
		t.Fatalf("A 按版本行应读回自己的账单, got=%v err=%v", gotA, err)
	}
	gotB, err := repo.GetByQuoteRowID(ctxB, "q_row_shared")
	if err != nil || gotB == nil || gotB.ID != b.ID {
		t.Fatalf("B 按版本行应读回自己的账单, got=%v err=%v", gotB, err)
	}

	// 按主键读别人的行：遮挡成 (nil, nil)，与「不存在」同形。
	if foreign, err := repo.GetByID(ctxB, a.ID); err != nil || foreign != nil {
		t.Fatalf("B 读 A 的账单应遮挡为 (nil,nil), got=%v err=%v", foreign, err)
	}
	if own, err := repo.GetByID(ctxA, a.ID); err != nil || own == nil || own.ID != a.ID {
		t.Fatalf("A 读自己的账单应命中, got=%v err=%v", own, err)
	}

	// 按逻辑报价号列账单：租户内各一张，不得混入对方。
	listA, err := repo.ListByQuoteID(ctxA, "q_shared")
	if err != nil || len(listA) != 1 || listA[0].ID != a.ID {
		t.Fatalf("A 按报价号应只看到 1 张自己的, got=%d err=%v", len(listA), err)
	}

	// 逾期扫描按租户收窄（同一谓词、同一账期，只有租户章不同）。
	scanA, err := repo.ScanOverdue(ctxA, time.Now(), 10)
	if err != nil || len(scanA.Overdue) != 1 || scanA.Overdue[0].ID != a.ID || scanA.Undated != 0 {
		t.Fatalf("A 逾期扫描应只含自己的 1 张, got=%+v err=%v", scanA, err)
	}
	scanB, err := repo.ScanOverdue(ctxB, time.Now(), 10)
	if err != nil || len(scanB.Overdue) != 1 || scanB.Overdue[0].ID != b.ID {
		t.Fatalf("B 逾期扫描应只含自己的 1 张, got=%+v err=%v", scanB, err)
	}

	// 跨租户跃迁：报「不存在」且不动对方行。
	if err := repo.UpdateStatus(ctxB, a.ID, model.BillStatusOpen, model.BillStatusPartial); !errors.Is(err, ErrBillNotFound) {
		t.Fatalf("跨租户跃迁应报 ErrBillNotFound, got %v", err)
	}
	var after model.Bill
	if err := db.WithContext(sys).First(&after, "id = ?", a.ID).Error; err != nil {
		t.Fatalf("回读 A 账单失败: %v", err)
	}
	if after.Status != model.BillStatusOpen {
		t.Errorf("跨租户跃迁不得改动 A 的状态, got %q", after.Status)
	}
}

func TestBillRepository_OnModeUnboundContextFailsClosed(t *testing.T) {
	db := billSetupDB(t)
	repo := NewBillRepositoryWithDB(db)

	bare := billRow("b_unbound", "q_row_unbound", nil)
	if err := repo.Create(context.Background(), bare); err == nil {
		t.Fatal("on 模式裸 ctx 派生应 fail-closed")
	}

	// 关键：读侧必须报租户闸，而不是被 billOrNil 读成「还没有账单」。
	if got, err := repo.GetByQuoteRowID(context.Background(), "q_row_unbound"); err == nil || got != nil {
		t.Fatalf("裸 ctx 读应按租户闸报错而不是 (nil,nil), got=%v err=%v", got, err)
	}
	if _, err := repo.ScanOverdue(context.Background(), time.Now(), 10); err == nil {
		t.Fatal("裸 ctx 逾期扫描应报错，不得读成「今天没人逾期」")
	}

	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.Bill{}).Count(&total).Error; err != nil {
		t.Fatalf("统计账单失败: %v", err)
	}
	if total != 0 {
		t.Errorf("fail-closed 后不应落行, got %d", total)
	}
}
