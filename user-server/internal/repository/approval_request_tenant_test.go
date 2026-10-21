// approval_request_tenant_test.go 审批请求的租户契约。
//
// uq_approval_request_open（pending 幂等键）与 uq_approval_request_token（恢复凭证）
// 都是部分唯一索引：租户化后必须复合为 (tenant_id, ...) 且保留 WHERE 谓词——
// 否则要么两个租户无法各自拥有同键 pending，要么裁决过的行永久占坑。
// 读取/裁决/清扫同样不得命中他人租户的行。
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
	aprTenantA = uint(171)
	aprTenantB = uint(172)
)

func aprSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ApprovalRequest{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func aprSeed(t *testing.T, db *gorm.DB, tenantID uint, id, subjectID, token, status string, expires *time.Time) *model.ApprovalRequest {
	t.Helper()
	now := time.Now().Add(-time.Hour)
	row := &model.ApprovalRequest{
		TenantID:    tenantID,
		ID:          id,
		SubjectType: "quote",
		SubjectID:   subjectID,
		PolicyKey:   "quote.send",
		Status:      status,
		ResumeToken: token,
		ExpiresAt:   expires,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 审批失败: %v", tenantID, err)
	}
	return row
}

func TestApprovalRequestRepo_CreateFillsTenantAndOpenKeyPerTenant(t *testing.T) {
	db := aprSetupDB(t)
	repo := NewApprovalRequestRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), aprTenantA)
	ctxB := tenant.WithTenantID(context.Background(), aprTenantB)
	exp := time.Now().Add(time.Hour)

	mk := func(id, token string) *model.ApprovalRequest {
		return &model.ApprovalRequest{
			ID: id, SubjectType: "quote", SubjectID: "q-1", PolicyKey: "quote.send",
			Status: model.ApprovalStatusPending, ResumeToken: token, ExpiresAt: &exp,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
	}
	a, b := mk("apr_t_a", "rt_t_a"), mk("apr_t_b", "rt_t_b")
	if err := repo.Insert(ctxA, a); err != nil {
		t.Fatalf("租户 A 入队失败: %v", err)
	}
	if a.TenantID != aprTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", aprTenantA, a.TenantID)
	}
	// 幂等键若仍是三列，这里会撞 A 的 pending
	if err := repo.Insert(ctxB, b); err != nil {
		t.Fatalf("两个租户同键 pending 应各自可建（唯一索引未复合化）: %v", err)
	}
	if b.TenantID != aprTenantB {
		t.Errorf("期望 tenant_id=%d，实际 %d", aprTenantB, b.TenantID)
	}

	got, err := repo.GetPendingBySubject(ctxA, "quote", "q-1", "quote.send")
	if err != nil {
		t.Fatalf("GetPendingBySubject 失败: %v", err)
	}
	if got == nil || got.ID != "apr_t_a" || got.TenantID != aprTenantA {
		t.Fatalf("租户 A 查到了别家的 pending: %+v", got)
	}
}

func TestApprovalRequestRepo_ResumeTokenTenantScoped(t *testing.T) {
	db := aprSetupDB(t)
	repo := NewApprovalRequestRepositoryWithDB(db)
	exp := time.Now().Add(time.Hour)
	aprSeed(t, db, aprTenantA, "apr_tok_a", "q-a", "rt_shared", model.ApprovalStatusPending, &exp)
	aprSeed(t, db, aprTenantB, "apr_tok_b", "q-b", "rt_shared", model.ApprovalStatusPending, &exp)

	got, err := repo.GetByResumeToken(tenant.WithTenantID(context.Background(), aprTenantA), "rt_shared")
	if err != nil {
		t.Fatalf("GetByResumeToken 失败: %v", err)
	}
	if got == nil || got.ID != "apr_tok_a" || got.TenantID != aprTenantA {
		t.Fatalf("按凭证查到了别家的审批: %+v", got)
	}
}

func TestApprovalRequestRepo_MutatePendingCrossTenantBlocked(t *testing.T) {
	db := aprSetupDB(t)
	repo := NewApprovalRequestRepositoryWithDB(db)
	exp := time.Now().Add(time.Hour)
	seedB := aprSeed(t, db, aprTenantB, "apr_mut_b", "q-b", "rt_mut_b", model.ApprovalStatusPending, &exp)
	asOf := time.Now()

	applied, err := repo.MutatePending(tenant.WithTenantID(context.Background(), aprTenantA), seedB.ID, asOf,
		func(a *model.ApprovalRequest) { a.Status = model.ApprovalStatusApproved; a.DecidedBy = "operator:1" })
	if err != nil {
		t.Fatalf("跨租户 MutatePending 不应报错（行不可见）: %v", err)
	}
	if applied {
		t.Fatal("跨租户裁决不应生效")
	}
	var after model.ApprovalRequest
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, "id = ?", seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 审批失败: %v", err)
	}
	if after.Status != model.ApprovalStatusPending || after.DecidedBy != "" {
		t.Fatalf("租户 B 的审批被跨租户改写: %+v", after)
	}

	applied, err = repo.MutatePending(tenant.WithTenantID(context.Background(), aprTenantB), seedB.ID, asOf,
		func(a *model.ApprovalRequest) { a.Status = model.ApprovalStatusApproved; a.DecidedBy = "operator:2" })
	if err != nil || !applied {
		t.Fatalf("本租户裁决失败: applied=%v err=%v", applied, err)
	}
}

func TestApprovalRequestRepo_ExpirePendingBatchTenantScoped(t *testing.T) {
	db := aprSetupDB(t)
	repo := NewApprovalRequestRepositoryWithDB(db)
	overdue := time.Now().Add(-time.Minute)
	seedB := aprSeed(t, db, aprTenantB, "apr_exp_b", "q-b", "rt_exp_b", model.ApprovalStatusPending, &overdue)

	flipped, err := repo.ExpirePendingBatch(tenant.WithTenantID(context.Background(), aprTenantA), time.Now(), 10)
	if err != nil {
		t.Fatalf("跨租户清扫报错: %v", err)
	}
	if len(flipped) != 0 {
		t.Fatalf("租户 A 的清扫翻到了租户 B 的审批: %+v", flipped)
	}
	var after model.ApprovalRequest
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, "id = ?", seedB.ID).Error; err != nil {
		t.Fatalf("回查租户 B 审批失败: %v", err)
	}
	if after.Status != model.ApprovalStatusPending {
		t.Fatalf("租户 B 的审批被跨租户清扫: %+v", after)
	}

	flipped, err = repo.ExpirePendingBatch(tenant.WithTenantID(context.Background(), aprTenantB), time.Now(), 10)
	if err != nil {
		t.Fatalf("本租户清扫失败: %v", err)
	}
	if len(flipped) != 1 || flipped[0].ID != seedB.ID {
		t.Fatalf("本租户清扫未翻到到期审批: %+v", flipped)
	}
}
