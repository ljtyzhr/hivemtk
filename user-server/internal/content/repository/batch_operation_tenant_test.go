package repository

// 契约锁（实体审查 #15，TENANT_MODE=on）：`batch_operation_histories` 是租户表（有 tenant_id 列），
// 而改前整条读/改链走的是 `context.Background()`——实测两条都撞闸：
//   [tenant] 表 batch_operation_histories 需要租户上下文但 ctx 未绑定（query）；……拒绝执行
//   [tenant] 表 batch_operation_histories 需要租户上下文但 ctx 未绑定（update）；……
// ⇒ saas 形态下「批量操作历史」列表 500、取消 500，off 形态（本地独立部署）全绿。
// 另外两条独立缺陷：
//   `GetHistoryByID` 命中失败时仍返回**非 nil 结构体 + error**，handler 又把任何 error 一律回成
//     404「历史记录不存在」——租户上下文没绑定这种配置级故障被说成「记录不存在」（见 controller 层锁）。
//   `ListHistories` 里 `Where("1 = 1")` 是手拼 WHERE（4.2），GORM 无其他条件时本就不需要锚点。
// 取消口的 `status IN ('pending','running')` 是「仅未完成可取消」的业务门，命中 0 行已经如实报错，
// 这一条留作正面样本：跨租户取消必须同样报「不可取消」，不能把别人的行改成 cancelled。

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/content/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	bopTenantA  = uint(51)
	bopTenantB  = uint(52)
	bopOperator = uint(880001)
)

func bopDB(t *testing.T) (*gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BatchOperationHistory{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡与取消门全都判不了（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), bopTenantA),
		tenant.WithTenantID(context.Background(), bopTenantB)
}

// bopSeed 用 SystemScope 直插，好让用例自己决定这行属于哪个租户。
func bopSeed(t *testing.T, db *gorm.DB, tenantID uint, status string) *model.BatchOperationHistory {
	t.Helper()
	row := &model.BatchOperationHistory{
		TenantID:      tenantID,
		OperationType: "import",
		DataType:      "clue",
		Status:        status,
		TotalCount:    10,
		SuccessCount:  0,
		OperatorID:    bopOperator,
		StartedAt:     time.Now(),
	}
	scope := db.WithContext(tenant.WithSystemScope(context.Background()))
	if err := scope.Create(row).Error; err != nil {
		t.Fatalf("预置 %d 租户的历史记录失败: %v", tenantID, err)
	}
	return row
}

func bopPhysical(t *testing.T, db *gorm.DB, id uint) *model.BatchOperationHistory {
	t.Helper()
	var row model.BatchOperationHistory
	err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("物理读取 history(%d) 失败: %v", id, err)
	}
	return &row
}

// TestBatchHistoryReadsAreTenantFactored 列表与详情都只看见本租户；裸 ctx 报租户闸而不是 not found。
func TestBatchHistoryReadsAreTenantFactored(t *testing.T) {
	db, ctxA, ctxB := bopDB(t)
	repo := NewBatchOperationRepository(db)

	a1 := bopSeed(t, db, bopTenantA, "success")
	a2 := bopSeed(t, db, bopTenantA, "running")
	b1 := bopSeed(t, db, bopTenantB, "success")

	list, total, err := repo.ListHistories(ctxA, 1, 20)
	if err != nil {
		t.Fatalf("本租户列表应可查: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("A 租户应看到 2 条，实得 total=%d len=%d（租户谓词没进 COUNT 或没进列表）", total, len(list))
	}

	// 同一个 id：B 看不见 A 的行，A 也看不见 B 的行——两向遮挡才说明谓词来自 ctx 而不是常量。
	if got, err := repo.GetHistoryByID(ctxB, a1.ID); !errors.Is(err, gorm.ErrRecordNotFound) || got != nil {
		t.Errorf("B 读 A 的历史应遮挡为 nil+not found，实得 got=%v err=%v", got, err)
	}
	if got, err := repo.GetHistoryByID(ctxA, b1.ID); !errors.Is(err, gorm.ErrRecordNotFound) || got != nil {
		t.Errorf("A 读 B 的历史应遮挡为 nil+not found，实得 got=%v err=%v", got, err)
	}
	if got, err := repo.GetHistoryByID(ctxA, a2.ID); err != nil || got == nil || got.Status != "running" {
		t.Errorf("A 读本租户历史应命中，实得 got=%+v err=%v", got, err)
	}

	// 语义口径：裸 ctx + 租户表 ⇒ [tenant] 报错，绝不能退化成 ErrRecordNotFound（那会被回成 404）。
	if _, _, err := repo.ListHistories(context.Background(), 1, 20); err == nil ||
		!strings.Contains(err.Error(), "需要租户上下文") {
		t.Errorf("裸 ctx 列表应撞租户闸，实得 err=%v", err)
	}
	if got, err := repo.GetHistoryByID(context.Background(), a1.ID); err == nil ||
		!strings.Contains(err.Error(), "需要租户上下文") || errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("裸 ctx 详情应撞租户闸而不是 not found，实得 got=%v err=%v", got, err)
	}
}

// TestBatchHistoryCancelIsTenantFactored 取消门只对本租户未完成的操作生效。
func TestBatchHistoryCancelIsTenantFactored(t *testing.T) {
	db, ctxA, ctxB := bopDB(t)
	repo := NewBatchOperationRepository(db)

	running := bopSeed(t, db, bopTenantA, "running")
	done := bopSeed(t, db, bopTenantA, "success")

	if err := repo.CancelHistory(ctxB, running.ID); !errors.Is(err, ErrBatchOperationNotCancellable) {
		t.Errorf("跨租户取消应报不可取消，实得 err=%v", err)
	}
	if row := bopPhysical(t, db, running.ID); row == nil || row.Status != "running" {
		t.Fatalf("跨租户取消不该动到物理行，实得 %+v", row)
	}

	if err := repo.CancelHistory(ctxA, done.ID); !errors.Is(err, ErrBatchOperationNotCancellable) {
		t.Errorf("已完成的操作不可取消，实得 err=%v", err)
	}

	if err := repo.CancelHistory(ctxA, running.ID); err != nil {
		t.Fatalf("本租户取消 running 应成功: %v", err)
	}
	row := bopPhysical(t, db, running.ID)
	if row.Status != "cancelled" || row.FinishedAt == nil {
		t.Errorf("取消后应写 cancelled 并落结束时间，实得 status=%q finished_at=%v", row.Status, row.FinishedAt)
	}
}

// TestBatchHistoryGetReturnsNoStructWithError 命中失败时必须交出 nil：
// handler 靠 error 判失败，非 nil 结构体让它「拿得到一个空壳却报错」，只能瞎猜成 404。
func TestBatchHistoryGetReturnsNoStructWithError(t *testing.T) {
	db, ctxA, _ := bopDB(t)
	repo := NewBatchOperationRepository(db)

	got, err := repo.GetHistoryByID(ctxA, 99999999)
	if err == nil {
		t.Fatal("查不存在的 id 必须报错")
	}
	if got != nil {
		t.Errorf("报错时必须交出 nil，实得 %+v", got)
	}
}
