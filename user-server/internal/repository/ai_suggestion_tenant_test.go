// ai_suggestion_tenant_test.go AI 建议仓储的租户契约。
//
// 建议行带 session_id 但 session 本身没有租户列可 JOIN：隔离只能靠
// ai_suggestions.tenant_id 上的 callback 谓词。这里钉住「创建自动落本租户、
// 同 session_id 两租户互不可见、跨租户标记已使用必须报 NotFound 且不改 B 行」。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	asgTenantA = uint(121)
	asgTenantB = uint(122)
)

func asgSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AISuggestion{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func asgSeed(t *testing.T, db *gorm.DB, tenantID uint, sessionID string) *model.AISuggestion {
	t.Helper()
	row := &model.AISuggestion{
		TenantID:   tenantID,
		SessionID:  sessionID,
		Suggestion: "suggestion-" + sessionID,
		Confidence: 80,
		Source:     "llm",
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的建议失败: %v", tenantID, err)
	}
	return row
}

func TestAISuggestionRepository_CreateFillsTenant(t *testing.T) {
	db := asgSetupDB(t)
	repo := NewAISuggestionRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), asgTenantA)

	row := &model.AISuggestion{SessionID: "s1", Suggestion: "x", Source: "llm"}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != asgTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", asgTenantA, row.TenantID)
	}
}

func TestAISuggestionRepository_SessionReadTenantScoped(t *testing.T) {
	db := asgSetupDB(t)
	asgSeed(t, db, asgTenantA, "shared-session")
	asgSeed(t, db, asgTenantB, "shared-session")
	repo := NewAISuggestionRepositoryWithDB(db)

	rows, err := repo.GetBySessionID(tenant.WithTenantID(context.Background(), asgTenantA), "shared-session")
	if err != nil {
		t.Fatalf("GetBySessionID 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].TenantID != asgTenantA {
		t.Fatalf("期望只读到租户 A 的一行，实际: %+v", rows)
	}
}

func TestAISuggestionRepository_CrossTenantMarkAsUsedBlocked(t *testing.T) {
	db := asgSetupDB(t)
	seedB := asgSeed(t, db, asgTenantB, "b-session")
	repo := NewAISuggestionRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), asgTenantA)

	if err := repo.MarkAsUsed(ctxA, seedB.ID, 9); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 MarkAsUsed 应报 NotFound（不得静默成功），实际: %v", err)
	}

	var after model.AISuggestion
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&after, seedB.ID).Error; err != nil {
		t.Fatalf("回读租户 B 失败: %v", err)
	}
	if after.IsUsed || after.UsedBy != 0 {
		t.Fatalf("租户 B 的建议被跨租户改写: is_used=%v used_by=%d", after.IsUsed, after.UsedBy)
	}

	own := asgSeed(t, db, asgTenantA, "a-session")
	if err := repo.MarkAsUsed(ctxA, own.ID, 7); err != nil {
		t.Fatalf("本租户 MarkAsUsed 失败: %v", err)
	}
	var afterOwn model.AISuggestion
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&afterOwn, own.ID).Error; err != nil {
		t.Fatalf("回读租户 A 失败: %v", err)
	}
	if !afterOwn.IsUsed || afterOwn.UsedBy != 7 || afterOwn.UsedAt == nil {
		t.Errorf("本租户标记未落库: %+v", afterOwn)
	}
}
