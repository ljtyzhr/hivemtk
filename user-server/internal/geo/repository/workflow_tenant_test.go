// workflow_tenant_test.go GEO 工作流三张表的租户契约。
//
// geo_workflows / geo_workflow_executions / geo_workflow_templates 都是租户表：
// 读、写、删都必须落在本租户；跨租户的 id 一律按"不存在"处理。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	gwfTenantA = uint(379)
	gwfTenantB = uint(380)
)

func gwfSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t,
		&model.GeoWorkflow{},
		&model.GeoWorkflowExecution{},
		&model.GeoWorkflowTemplate{},
	)
	if db == nil {
		t.Fatal("测试库不可达：租户遮挡判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestGeoWorkflowRepository_TenantScoped(t *testing.T) {
	db := gwfSetupDB(t)
	repo := NewGeoWorkflowRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gwfTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gwfTenantB)

	wfA := &model.GeoWorkflow{
		Name:    "内容生产工作流",
		Steps:   `[{"name":"s1","type":"content_generate"}]`,
		Enabled: true,
	}
	if err := repo.Create(ctxA, wfA); err != nil {
		t.Fatalf("租户 A 创建工作流失败: %v", err)
	}
	if wfA.TenantID != gwfTenantA {
		t.Fatalf("Create 未填租户: %d", wfA.TenantID)
	}
	if wfA.ID == "" {
		t.Fatal("Create 未生成主键")
	}
	if err := repo.Create(ctxB, &model.GeoWorkflow{Name: "B 的工作流", Enabled: true}); err != nil {
		t.Fatalf("租户 B 创建工作流失败: %v", err)
	}

	// 跨租户按 id 读 → 不存在。
	if _, err := repo.GetByID(ctxB, wfA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 GetByID 应报不存在，实际 err=%v", err)
	}
	// 列表只含本租户。
	listA, err := repo.GetList(ctxA)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != gwfTenantA {
		t.Fatalf("租户 A 列表越界: %+v", listA)
	}

	// 更新：enabled=true → false 必须真正落库（零值走 map 显式赋值），租户不被改写。
	wfA.Name = "改名后的工作流"
	wfA.Enabled = false
	if err := repo.Update(ctxA, wfA); err != nil {
		t.Fatalf("租户 A 更新失败: %v", err)
	}
	got, err := repo.GetByID(ctxA, wfA.ID)
	if err != nil {
		t.Fatalf("回查失败: %v", err)
	}
	if got.Enabled {
		t.Fatal("enabled=false 未落库（零值被忽略）")
	}
	if got.Name != "改名后的工作流" {
		t.Fatalf("名称未更新: %q", got.Name)
	}
	if got.TenantID != gwfTenantA {
		t.Fatalf("更新改写了租户: %d", got.TenantID)
	}

	// 跨租户更新 → 不存在。
	if err := repo.Update(ctxB, wfA); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报不存在，实际 err=%v", err)
	}
	// 跨租户删除 → 不存在。
	if err := repo.Delete(ctxB, wfA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Delete 应报不存在，实际 err=%v", err)
	}

	// 本租户删除成功；重复删除（已软删）同样按不存在处理。
	if err := repo.Delete(ctxA, wfA.ID); err != nil {
		t.Fatalf("租户 A 删除失败: %v", err)
	}
	if err := repo.Delete(ctxA, wfA.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("重复删除应报不存在，实际 err=%v", err)
	}
}

func TestGeoWorkflowExecutionRepository_TenantScoped(t *testing.T) {
	db := gwfSetupDB(t)
	wfRepo := NewGeoWorkflowRepositoryWithDB(db)
	execRepo := NewGeoWorkflowExecutionRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gwfTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gwfTenantB)

	wfA := &model.GeoWorkflow{Name: "A", Enabled: true}
	if err := wfRepo.Create(ctxA, wfA); err != nil {
		t.Fatalf("租户 A 创建工作流失败: %v", err)
	}

	execA := &model.GeoWorkflowExecution{WorkflowID: wfA.ID, Status: "running"}
	if err := execRepo.Create(ctxA, execA); err != nil {
		t.Fatalf("租户 A 创建执行记录失败: %v", err)
	}
	if execA.TenantID != gwfTenantA {
		t.Fatalf("Create 未填租户: %d", execA.TenantID)
	}

	// 同 workflow id 在租户 B 看不到任何执行记录。
	execsB, err := execRepo.GetByWorkflowID(ctxB, wfA.ID)
	if err != nil {
		t.Fatalf("租户 B 查询执行记录失败: %v", err)
	}
	if len(execsB) != 0 {
		t.Fatalf("租户 B 读到了租户 A 的执行记录: %+v", execsB)
	}

	// 跨租户落终态 → 不存在。
	execA.Status = "success"
	if err := execRepo.Update(ctxB, execA); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户 Update 应报不存在，实际 err=%v", err)
	}

	// 本租户落终态成功，且不会被租户 B 的失败更新影响。
	if err := execRepo.Update(ctxA, execA); err != nil {
		t.Fatalf("租户 A 更新执行记录失败: %v", err)
	}
	execsA, err := execRepo.GetByWorkflowID(ctxA, wfA.ID)
	if err != nil {
		t.Fatalf("租户 A 查询执行记录失败: %v", err)
	}
	if len(execsA) != 1 || execsA[0].Status != "success" || execsA[0].TenantID != gwfTenantA {
		t.Fatalf("租户 A 执行记录状态不对: %+v", execsA)
	}

	// 不存在的执行 id → 不存在，不能当成功。
	if err := execRepo.Update(ctxA, &model.GeoWorkflowExecution{ID: "no-such-exec", Status: "failed"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("不存在的执行 id 应报不存在，实际 err=%v", err)
	}
}

func TestGeoWorkflowTemplateRepository_TenantScoped(t *testing.T) {
	db := gwfSetupDB(t)
	repo := NewGeoWorkflowTemplateRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), gwfTenantA)
	ctxB := tenant.WithTenantID(context.Background(), gwfTenantB)

	tplA := &model.GeoWorkflowTemplate{
		Name:  "模板 A",
		Steps: `[{"name":"s1","type":"content_generate"}]`,
	}
	if err := repo.Create(ctxA, tplA); err != nil {
		t.Fatalf("租户 A 创建模板失败: %v", err)
	}
	if tplA.TenantID != gwfTenantA || tplA.ID == "" {
		t.Fatalf("模板落库字段不对: tenant=%d id=%q", tplA.TenantID, tplA.ID)
	}

	listB, err := repo.GetList(ctxB)
	if err != nil {
		t.Fatalf("租户 B 列表失败: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("租户 B 读到了租户 A 的模板: %+v", listB)
	}
	listA, err := repo.GetList(ctxA)
	if err != nil {
		t.Fatalf("租户 A 列表失败: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != gwfTenantA {
		t.Fatalf("租户 A 模板列表越界: %+v", listA)
	}
}
