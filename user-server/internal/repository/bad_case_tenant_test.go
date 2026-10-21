// bad_case_tenant_test.go bad_cases 的租户契约（#40 实体审计补充）。
//
// 既有仓储用例全部跑在 off 模式，证明的是"单租户下行为正确"。本文件把
// 多租户下真正会出错的四件事钉住：
//  1. InsertIfAbsent 的 tenant_id 由 ctx 盖章；同一 dedup_key 可在两个租户各存一行
//     （唯一索引必须已复合为 (tenant_id, dedup_key)，否则 B 的坏例被 A 的行静默吃掉）；
//  2. 读/聚合/导出取走全部按租户过滤，跨租户既读不到也改不动；
//  3. ClaimForExport 只抢本租户的行，A 导出后 B 的行仍可被 B 导出；
//  4. on 模式漏绑 ctx 必须 fail-closed，而不是回空列表（空列表会被读成"没有坏例"）。
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
	bcTenantA = uint(221)
	bcTenantB = uint(222)
)

func bcSetupTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.BadCase{})
	if db == nil {
		t.Fatal("测试库不可达：bad_cases 租户契约判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func bcNewAutoRow(id, messageID string) *model.BadCase {
	key, ok := model.BadCaseDedupKey(model.BadCaseSourceLowConfidence, "sess_shared", messageID)
	if !ok {
		panic("夹具：自动来源缺 message_id")
	}
	now := time.Now()
	return &model.BadCase{
		ID:             id,
		Source:         model.BadCaseSourceLowConfidence,
		Status:         model.BadCaseStatusPending,
		SessionID:      "sess_shared",
		MessageID:      messageID,
		QueryText:      "问句 " + id,
		AnswerText:     "答句 " + id,
		Confidence:     0.42,
		Threshold:      0.7,
		RetrievedCount: 0,
		DedupKey:       key,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func bcLabel(t *testing.T, repo BadCaseRepository, ctx context.Context, id string) {
	t.Helper()
	now := time.Now()
	applied, err := repo.ApplyAction(ctx, id, model.BadCaseStatusPending, func(row *model.BadCase) error {
		row.Status = model.BadCaseStatusLabeled
		row.Label = model.BadCaseLabelKBMissing
		row.LabelNote = "库里没有这条政策"
		row.LabelerID = "op-tenant"
		row.LabeledAt = &now
		row.FixLayer = model.FixLayerOfLabel(model.BadCaseLabelKBMissing)
		row.UpdatedAt = now
		return nil
	})
	if err != nil || !applied {
		t.Fatalf("打标 %s: applied=%v err=%v", id, applied, err)
	}
}

// TestBadCaseRepo_TenantScopedRowsAndDedup 同一 dedup_key 跨租户并存，读写互不串台。
func TestBadCaseRepo_TenantScopedRowsAndDedup(t *testing.T) {
	db := bcSetupTenantDB(t)
	repo := NewBadCaseRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), bcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), bcTenantB)

	rowA := bcNewAutoRow("bc_tenant_a", "msg_same")
	if ok, err := repo.InsertIfAbsent(ctxA, rowA); err != nil || !ok {
		t.Fatalf("租户 A 落库: ok=%v err=%v", ok, err)
	}
	if rowA.TenantID != bcTenantA {
		t.Fatalf("租户 A 行 TenantID = %d, want %d（ctx 未盖章）", rowA.TenantID, bcTenantA)
	}

	// 同一 (session, message) 在 B 租户是另一条真坏例，唯一索引必须放行。
	rowB := bcNewAutoRow("bc_tenant_b", "msg_same")
	if ok, err := repo.InsertIfAbsent(ctxB, rowB); err != nil || !ok {
		t.Fatalf("租户 B 用同一 dedup_key 落库: ok=%v err=%v（唯一索引未复合化？）", ok, err)
	}
	if rowB.TenantID != bcTenantB {
		t.Fatalf("租户 B 行 TenantID = %d, want %d", rowB.TenantID, bcTenantB)
	}

	gotB, err := repo.GetByID(ctxB, rowA.ID)
	if err != nil || gotB != nil {
		t.Fatalf("租户 B 读到了租户 A 的行: %+v err=%v", gotB, err)
	}
	gotA, err := repo.GetByID(ctxA, rowA.ID)
	if err != nil || gotA == nil || gotA.TenantID != bcTenantA {
		t.Fatalf("租户 A 读自己的行失败: %+v err=%v", gotA, err)
	}

	rowsA, totalA, err := repo.List(ctxA, BadCaseListQuery{})
	if err != nil {
		t.Fatalf("租户 A List: %v", err)
	}
	if totalA != 1 || len(rowsA) != 1 || rowsA[0].ID != rowA.ID {
		t.Fatalf("租户 A 列表串租户: total=%d rows=%v", totalA, rowsA)
	}
	statsA, err := repo.CountByStatus(ctxA)
	if err != nil {
		t.Fatalf("租户 A CountByStatus: %v", err)
	}
	if statsA[model.BadCaseStatusPending] != 1 {
		t.Fatalf("租户 A pending = %d, want 1（聚合串租户）", statsA[model.BadCaseStatusPending])
	}
	statsB, err := repo.CountByStatus(ctxB)
	if err != nil {
		t.Fatalf("租户 B CountByStatus: %v", err)
	}
	if statsB[model.BadCaseStatusPending] != 1 {
		t.Fatalf("租户 B pending = %d, want 1", statsB[model.BadCaseStatusPending])
	}
}

// TestBadCaseRepo_CrossTenantActionsAreInvisible 跨租户读/改/导出都不得命中对方行。
func TestBadCaseRepo_CrossTenantActionsAreInvisible(t *testing.T) {
	db := bcSetupTenantDB(t)
	repo := NewBadCaseRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), bcTenantA)
	ctxB := tenant.WithTenantID(context.Background(), bcTenantB)

	rowA := bcNewAutoRow("bc_cross_a", "msg_a")
	rowB := bcNewAutoRow("bc_cross_b", "msg_b")
	if ok, err := repo.InsertIfAbsent(ctxA, rowA); err != nil || !ok {
		t.Fatalf("租户 A 落库: ok=%v err=%v", ok, err)
	}
	if ok, err := repo.InsertIfAbsent(ctxB, rowB); err != nil || !ok {
		t.Fatalf("租户 B 落库: ok=%v err=%v", ok, err)
	}

	applied, err := repo.ApplyAction(ctxB, rowA.ID, model.BadCaseStatusPending, func(row *model.BadCase) error {
		row.Status = model.BadCaseStatusDismissed
		row.CancelReason = "越权撤销"
		return nil
	})
	if err != nil {
		t.Fatalf("跨租户 ApplyAction 返回错误（应为 applied=false, nil）: %v", err)
	}
	if applied {
		t.Fatal("跨租户 ApplyAction 命中了对方行 ⇒ 隔离失效")
	}
	unchanged, err := repo.GetByID(ctxA, rowA.ID)
	if err != nil || unchanged == nil {
		t.Fatalf("回查租户 A 行: %+v err=%v", unchanged, err)
	}
	if unchanged.Status != model.BadCaseStatusPending || unchanged.CancelReason != "" {
		t.Fatalf("租户 A 的行被跨租户改动了: %+v", unchanged)
	}

	bcLabel(t, repo, ctxA, rowA.ID)
	claimed, err := repo.ClaimForExport(ctxA, nil, 10, "bce_a", time.Now())
	if err != nil {
		t.Fatalf("租户 A 导出: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != rowA.ID {
		t.Fatalf("租户 A 导出抢到了别的租户的行: %v", claimed)
	}

	// 租户 B 的行未被 A 的导出波及：仍可被 B 正常打标并导出。
	stillB, err := repo.GetByID(ctxB, rowB.ID)
	if err != nil || stillB == nil {
		t.Fatalf("回查租户 B 行: %+v err=%v", stillB, err)
	}
	if stillB.Status != model.BadCaseStatusPending {
		t.Fatalf("租户 B 的行被租户 A 的导出改成了 %s", stillB.Status)
	}
	bcLabel(t, repo, ctxB, rowB.ID)
	claimedB, err := repo.ClaimForExport(ctxB, nil, 10, "bce_b", time.Now())
	if err != nil {
		t.Fatalf("租户 B 导出: %v", err)
	}
	if len(claimedB) != 1 || claimedB[0].ID != rowB.ID {
		t.Fatalf("租户 B 导出结果异常: %v", claimedB)
	}
}

// TestBadCaseRepo_UnboundContextFailsClosed on 模式漏绑 ctx 不得回空结果。
func TestBadCaseRepo_UnboundContextFailsClosed(t *testing.T) {
	db := bcSetupTenantDB(t)
	repo := NewBadCaseRepositoryWithDB(db)
	unbound := context.Background()

	if rows, total, err := repo.List(unbound, BadCaseListQuery{}); err == nil {
		t.Fatalf("未绑定租户的 List 没有报错（rows=%d total=%d）⇒ 读到的是全量还是空集都不可知", len(rows), total)
	}
	if _, err := repo.CountByStatus(unbound); err == nil {
		t.Fatal("未绑定租户的 CountByStatus 没有报错 ⇒ 看板会把未知读成 0")
	}
	if ok, err := repo.InsertIfAbsent(unbound, bcNewAutoRow("bc_unbound", "msg_unbound")); err == nil || ok {
		t.Fatalf("未绑定租户的写入没有 fail-closed: ok=%v err=%v", ok, err)
	}
}
