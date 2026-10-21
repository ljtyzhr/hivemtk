// ai_agent_repo_tenant_test.go AI 智能体仓储的租户契约。
//
// ai_agents.agent_code 的唯一索引已由 tenancyidx.Normalize 复合化为
// (tenant_id, agent_code)。这里钉住「创建自动落本租户、同 code 两租户并存、
// 跨租户读写不可见且不假成功、列表只含本租户」。
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
	aaTenantA = uint(101)
	aaTenantB = uint(102)
)

func aaSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AIAgent{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func aaSeed(t *testing.T, db *gorm.DB, tenantID uint, code string) *model.AIAgent {
	t.Helper()
	row := &model.AIAgent{
		TenantID:  tenantID,
		AgentCode: code,
		Name:      "agent-" + code,
		Persona:   "persona",
		AgentType: string(model.AgentTypeSales),
		Status:    1,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的智能体失败: %v", tenantID, err)
	}
	return row
}

func TestAIAgentRepository_CreateFillsTenant(t *testing.T) {
	db := aaSetupDB(t)
	repo := NewAIAgentRepository()
	repo.SetDB(context.Background(), db)
	ctxA := tenant.WithTenantID(context.Background(), aaTenantA)

	row := &model.AIAgent{AgentCode: "new", Name: "n", Persona: "p", Status: 1}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != aaTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", aaTenantA, row.TenantID)
	}
}

func TestAIAgentRepository_SameAgentCodeAcrossTenants(t *testing.T) {
	db := aaSetupDB(t)
	repo := NewAIAgentRepository()
	repo.SetDB(context.Background(), db)

	for _, tc := range []struct {
		tenantID uint
		code     string
	}{
		{aaTenantA, "shared"},
		{aaTenantB, "shared"},
	} {
		ctx := tenant.WithTenantID(context.Background(), tc.tenantID)
		row := &model.AIAgent{AgentCode: tc.code, Name: "n", Persona: "p", Status: 1}
		if err := repo.Create(ctx, row); err != nil {
			t.Fatalf("租户 %d 创建同 code 智能体失败（唯一键应已复合化）: %v", tc.tenantID, err)
		}
	}

	gotA, err := repo.GetByCode(tenant.WithTenantID(context.Background(), aaTenantA), "shared")
	if err != nil {
		t.Fatalf("租户 A GetByCode 失败: %v", err)
	}
	gotB, err := repo.GetByCode(tenant.WithTenantID(context.Background(), aaTenantB), "shared")
	if err != nil {
		t.Fatalf("租户 B GetByCode 失败: %v", err)
	}
	if gotA.ID == gotB.ID {
		t.Fatal("两租户同 code 应是两条独立记录")
	}
}

func TestAIAgentRepository_CrossTenantReadAndWriteBlocked(t *testing.T) {
	db := aaSetupDB(t)
	seedB := aaSeed(t, db, aaTenantB, "b-only")
	repo := NewAIAgentRepository()
	repo.SetDB(context.Background(), db)
	ctxA := tenant.WithTenantID(context.Background(), aaTenantA)

	if _, err := repo.GetByID(ctxA, seedB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByID 期望 ErrRecordNotFound，实际: %v", err)
	}
	if _, err := repo.GetByCode(ctxA, "b-only"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByCode 期望 ErrRecordNotFound，实际: %v", err)
	}
	if err := repo.UpdateStatus(ctxA, seedB.ID, 0); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 UpdateStatus 应报 NotFound，实际: %v", err)
	}
	if err := repo.Delete(ctxA, seedB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 应报 NotFound，实际: %v", err)
	}

	var b model.AIAgent
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&b, seedB.ID).Error; err != nil {
		t.Fatalf("读取租户 B 记录失败: %v", err)
	}
	if b.Status != 1 {
		t.Errorf("租户 B 状态被跨租户改写: %d", b.Status)
	}
}

func TestAIAgentRepository_UpdateWritesZeroValuesTenantScoped(t *testing.T) {
	db := aaSetupDB(t)
	seedA := aaSeed(t, db, aaTenantA, "a-upd")
	seedB := aaSeed(t, db, aaTenantB, "b-upd")
	repo := NewAIAgentRepository()
	repo.SetDB(context.Background(), db)
	ctxA := tenant.WithTenantID(context.Background(), aaTenantA)

	updated := *seedA
	updated.Name = "renamed"
	updated.EnableRAG = false
	updated.MaxAIConsecutive = 7
	if err := repo.Update(ctxA, &updated); err != nil {
		t.Fatalf("本租户 Update 失败: %v", err)
	}

	var got model.AIAgent
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&got, seedA.ID).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("名称未落库: %s", got.Name)
	}
	if got.EnableRAG {
		t.Error("零值字段未写入：EnableRAG 应为 false")
	}
	if got.MaxAIConsecutive != 7 {
		t.Errorf("MaxAIConsecutive 期望 7，实际 %d", got.MaxAIConsecutive)
	}

	stolen := *seedB
	stolen.Name = "stolen"
	if err := repo.Update(ctxA, &stolen); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报 NotFound（不得 Save 搬行），实际: %v", err)
	}
	var bAfter model.AIAgent
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&bAfter, seedB.ID).Error; err != nil {
		t.Fatalf("回读租户 B 失败: %v", err)
	}
	if bAfter.TenantID != aaTenantB || bAfter.Name == "stolen" {
		t.Fatalf("租户 B 的行被改写/搬走: tenant=%d name=%s", bAfter.TenantID, bAfter.Name)
	}
}

func TestAIAgentRepository_ListTenantScoped(t *testing.T) {
	db := aaSetupDB(t)
	aaSeed(t, db, aaTenantA, "a-list")
	aaSeed(t, db, aaTenantB, "b-list")
	repo := NewAIAgentRepository()
	repo.SetDB(context.Background(), db)

	rows, err := repo.List(tenant.WithTenantID(context.Background(), aaTenantA), "", -1, "")
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].AgentCode != "a-list" {
		t.Fatalf("期望仅本租户 a-list，实际: %+v", rows)
	}
}
