// agent_status_repo_tenant_test.go 客服状态仓储的租户契约。
//
// agent_statuses.agent_id 的唯一键已由 tenancyidx.Normalize 复合化为
// (tenant_id, agent_id)，同一系统用户在不同租户可各有一条坐席记录。这里钉死
// 「创建自动落本租户、按 agent_id 的读写只命中本租户、名册只列本租户」。
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
	agtTenantA = uint(95)
	agtTenantB = uint(96)
)

func agtSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.AgentStatus{})
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func agtSeed(t *testing.T, db *gorm.DB, tenantID, agentID uint) *model.AgentStatus {
	t.Helper()
	now := time.Now()
	row := &model.AgentStatus{
		TenantID:     tenantID,
		AgentID:      agentID,
		AgentName:    "agent",
		Status:       "online",
		MaxSessions:  5,
		LastActiveAt: &now,
	}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的坐席失败: %v", tenantID, err)
	}
	return row
}

func TestAgentStatusRepository_CreateFillsTenant(t *testing.T) {
	db := agtSetupDB(t)
	repo := NewAgentStatusRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), agtTenantA)

	row := &model.AgentStatus{AgentID: 1, AgentName: "a1", Status: "offline", MaxSessions: 5}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if row.TenantID != agtTenantA {
		t.Errorf("期望 tenant_id=%d，实际 %d", agtTenantA, row.TenantID)
	}
}

func TestAgentStatusRepository_SameAgentIDAcrossTenants(t *testing.T) {
	db := agtSetupDB(t)
	agtSeed(t, db, agtTenantA, 7)
	agtSeed(t, db, agtTenantB, 7)

	repo := NewAgentStatusRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), agtTenantA)

	got, err := repo.GetByAgentID(ctxA, 7)
	if err != nil {
		t.Fatalf("GetByAgentID 失败: %v", err)
	}
	if got.TenantID != agtTenantA {
		t.Errorf("期望命中租户 %d 的记录，实际 tenant_id=%d", agtTenantA, got.TenantID)
	}

	other, err := repo.GetByAgentID(tenant.WithTenantID(context.Background(), agtTenantB), 7)
	if err != nil {
		t.Fatalf("租户 B GetByAgentID 失败: %v", err)
	}
	if other.ID == got.ID {
		t.Fatal("两个租户的 agent_id=7 应是两条独立记录")
	}
}

func TestAgentStatusRepository_CrossTenantReadAndWriteBlocked(t *testing.T) {
	db := agtSetupDB(t)
	agtSeed(t, db, agtTenantA, 11)
	seedB := agtSeed(t, db, agtTenantB, 12)

	repo := NewAgentStatusRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), agtTenantA)

	if _, err := repo.GetByAgentID(ctxA, 12); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByAgentID 期望 ErrRecordNotFound，实际: %v", err)
	}

	if err := repo.UpdateStatus(ctxA, 12, "offline"); err != nil {
		t.Fatalf("UpdateStatus 跨租户不应报错（隔离挡 0 行），实际: %v", err)
	}
	if err := repo.TouchHeartbeat(ctxA, 12); err != nil {
		t.Fatalf("TouchHeartbeat 跨租户不应报错，实际: %v", err)
	}
	var b model.AgentStatus
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&b, seedB.ID).Error; err != nil {
		t.Fatalf("读取租户 B 记录失败: %v", err)
	}
	if b.Status != "online" {
		t.Errorf("租户 B 的记录被跨租户写改动了: status=%s", b.Status)
	}
}

func TestAgentStatusRepository_ListAllAgentsTenantScoped(t *testing.T) {
	db := agtSetupDB(t)
	agtSeed(t, db, agtTenantA, 21)
	agtSeed(t, db, agtTenantB, 22)

	repo := NewAgentStatusRepositoryWithDB(db)
	rows, err := repo.ListAllAgents(tenant.WithTenantID(context.Background(), agtTenantA))
	if err != nil {
		t.Fatalf("ListAllAgents 失败: %v", err)
	}
	if len(rows) != 1 || rows[0].AgentID != 21 {
		t.Fatalf("期望仅本租户 agent_id=21，实际: %+v", rows)
	}
}
