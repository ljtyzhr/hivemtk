// customer_service_agent_tenant_test.go 客服座席↔智能体挂载的租户契约（实体审查 #46）。
//
// 该表有两格需要钉死：
//
// ① `idx_cs_agent_unique (agent_status_id, ai_agent_id)` 在模型标签里没有 tenant_id，
//
//	靠 tenancyidx.Normalize 在运行期复合化。若不复合，A 租户给某座席挂了智能体后，
//	B 租户用相同 ID 走到同一行时会被唯一键拒绝 —— 一条挂载只属于一个租户。
//
// ② is_primary/enabled 带 `default:true`：bool 会把显式 false 吞成 true，
//
//	「辅助挂载」会变成主挂载、「停用挂载」会变成启用。这里钉住指针语义。
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
	csaTenantA = uint(383)
	csaTenantB = uint(384)
)

func csaDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.CustomerServiceAgent{})
	if db == nil {
		t.Fatal("测试库不可达：挂载归属无法判定（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestCustomerServiceAgentSamePairAcrossTenants(t *testing.T) {
	db := csaDB(t)
	ctxA := tenant.WithTenantID(context.Background(), csaTenantA)
	ctxB := tenant.WithTenantID(context.Background(), csaTenantB)
	repo := NewCustomerServiceAgentRepositoryWithDB(db)

	// 两租户使用完全相同的 (agent_status_id, ai_agent_id)：断言的才是 tenant_id 谓词本身。
	for _, ctx := range []context.Context{ctxA, ctxB} {
		row := &model.CustomerServiceAgent{
			AgentStatusID: 7001, AIAgentID: 8001,
			IsPrimary: boolPtr(true), Enabled: boolPtr(true),
		}
		if err := repo.Create(ctx, row); err != nil {
			t.Fatalf("同 (座席,智能体) 两租户各挂一条失败（唯一键未复合 tenant_id）: %v", err)
		}
	}

	// 同租户内同键重复必须被唯一键拒绝。
	dup := &model.CustomerServiceAgent{AgentStatusID: 7001, AIAgentID: 8001}
	if err := repo.Create(ctxA, dup); err == nil {
		t.Error("同租户重复挂载未被唯一键挡住")
	}

	if listA, err := repo.ListByAgentStatusID(ctxA, 7001); err != nil || len(listA) != 1 {
		t.Fatalf("租户 A 应只读到自己的 1 条挂载，实际 %d 条（err=%v）", len(listA), err)
	}
	if listB, err := repo.ListByAgentStatusID(ctxB, 7001); err != nil || len(listB) != 1 {
		t.Fatalf("租户 B 应只读到自己的 1 条挂载，实际 %d 条（err=%v）", len(listB), err)
	}
}

func TestCustomerServiceAgentExplicitFalsePersists(t *testing.T) {
	db := csaDB(t)
	ctxA := tenant.WithTenantID(context.Background(), csaTenantA)
	repo := NewCustomerServiceAgentRepositoryWithDB(db)

	row := &model.CustomerServiceAgent{
		AgentStatusID: 7101, AIAgentID: 8101,
		IsPrimary: boolPtr(false), Enabled: boolPtr(false),
	}
	if err := repo.Create(ctxA, row); err != nil {
		t.Fatalf("创建辅助挂载失败: %v", err)
	}

	var got model.CustomerServiceAgent
	if err := db.WithContext(ctxA).First(&got, row.ID).Error; err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Primary() {
		t.Error("is_primary 显式 false 被 DB 默认值覆盖成 true：辅助挂载会变成主挂载")
	}
	if got.IsActive() {
		t.Error("enabled 显式 false 被 DB 默认值覆盖成 true：停用挂载会直接生效")
	}
}

func TestCustomerServiceAgentUpdateDeleteStayInTenant(t *testing.T) {
	db := csaDB(t)
	ctxA := tenant.WithTenantID(context.Background(), csaTenantA)
	ctxB := tenant.WithTenantID(context.Background(), csaTenantB)
	repo := NewCustomerServiceAgentRepositoryWithDB(db)

	seeded := &model.CustomerServiceAgent{
		AgentStatusID: 7201, AIAgentID: 8201,
		IsPrimary: boolPtr(true), Enabled: boolPtr(true),
	}
	if err := repo.Create(ctxB, seeded); err != nil {
		t.Fatalf("预置租户 B 挂载失败: %v", err)
	}

	stolen := *seeded
	stolen.AIAgentID = 9999
	if err := repo.Update(ctxA, &stolen); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报 ErrRecordNotFound，实际: %v", err)
	}
	if err := repo.Delete(ctxA, seeded.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 应报 ErrRecordNotFound，实际: %v", err)
	}

	var after model.CustomerServiceAgent
	if err := db.WithContext(ctxB).First(&after, seeded.ID).Error; err != nil {
		t.Fatalf("租户 B 的行不应被删除/搬走: %v", err)
	}
	if after.AIAgentID != 8201 || after.TenantID != csaTenantB {
		t.Fatalf("租户 B 的行被跨租户改写: tenant=%d ai_agent_id=%d", after.TenantID, after.AIAgentID)
	}
}
