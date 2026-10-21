// system_config_repo_tenant_test.go SystemConfig 的租户契约。
//
// system_config 是「一租户一行」：租户隔离 callback 负责读侧过滤与写侧填充，
// 仓储负责把更新落在既有行上并继承其 tenant_id。这里钉三件事：
// 两个租户各读各写、创建时填充/拒绝越权 tenant_id、on 模式漏绑 ctx 直接报错。
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
	scTenantA = uint(81)
	scTenantB = uint(82)
)

func scSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.SystemConfig{})
	if db == nil {
		t.Fatal("测试库不可达：租户读写隔离判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func scSeed(t *testing.T, db *gorm.DB, tenantID uint, name string) *model.SystemConfig {
	t.Helper()
	row := &model.SystemConfig{TenantID: tenantID, Name: name, MaxUploadSizeMB: 50}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(row).Error; err != nil {
		t.Fatalf("预置租户 %d 的站点配置失败: %v", tenantID, err)
	}
	return row
}

func scPhysical(t *testing.T, db *gorm.DB, id uint) *model.SystemConfig {
	t.Helper()
	var row model.SystemConfig
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).First(&row, id).Error; err != nil {
		t.Fatalf("物理读取 system_config(%d) 失败: %v", id, err)
	}
	return &row
}

func TestSystemConfig_GetAndSaveAreTenantScoped(t *testing.T) {
	db := scSetupDB(t)
	rowA := scSeed(t, db, scTenantA, "站点 A")
	rowB := scSeed(t, db, scTenantB, "站点 B")

	repo := &systemConfigRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), scTenantA)
	ctxB := tenant.WithTenantID(context.Background(), scTenantB)

	gotA, err := repo.GetConfig(ctxA)
	if err != nil {
		t.Fatalf("租户 A 读站点配置失败: %v", err)
	}
	if gotA.ID != rowA.ID || gotA.TenantID != scTenantA || gotA.Name != "站点 A" {
		t.Fatalf("租户 A 读到 %+v，期望自己的行(%d)", gotA, rowA.ID)
	}
	gotB, err := repo.GetConfig(ctxB)
	if err != nil {
		t.Fatalf("租户 B 读站点配置失败: %v", err)
	}
	if gotB.ID != rowB.ID || gotB.TenantID != scTenantB || gotB.Name != "站点 B" {
		t.Fatalf("租户 B 读到 %+v，期望自己的行(%d)", gotB, rowB.ID)
	}

	// 更新 A 只许落在 A 的行上：B 的物理行原样不动、A 的 tenant_id 不被改写。
	gotA.Name = "站点 A2"
	if _, err := repo.SaveConfig(ctxA, gotA); err != nil {
		t.Fatalf("租户 A 保存站点配置失败: %v", err)
	}
	physicalA := scPhysical(t, db, rowA.ID)
	if physicalA.Name != "站点 A2" || physicalA.TenantID != scTenantA {
		t.Fatalf("A 的行更新后为 %+v，期望 name=站点 A2 tenant_id=%d", physicalA, scTenantA)
	}
	physicalB := scPhysical(t, db, rowB.ID)
	if physicalB.Name != "站点 B" || physicalB.TenantID != scTenantB {
		t.Fatalf("A 的保存改到了 B 的行：%+v", physicalB)
	}
}

func TestSystemConfig_SaveConfigCreateFillsTenantAndRejectsForeignTenant(t *testing.T) {
	db := scSetupDB(t)
	repo := &systemConfigRepo{db: db}
	ctxA := tenant.WithTenantID(context.Background(), scTenantA)
	ctxB := tenant.WithTenantID(context.Background(), scTenantB)

	saved, err := repo.SaveConfig(ctxA, &model.SystemConfig{Name: "新建站点"})
	if err != nil {
		t.Fatalf("租户 A 首次保存失败: %v", err)
	}
	if saved.TenantID != scTenantA {
		t.Fatalf("Create 填充 tenant_id=%d，期望 %d", saved.TenantID, scTenantA)
	}

	// 一租户一行：同租户再插一行必须被唯一索引挡住（否则 First 读哪行变成掷骰子）。
	dup := &model.SystemConfig{TenantID: scTenantA, Name: "第二行"}
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).Create(dup).Error; err == nil {
		t.Fatal("同租户插入第二行成功 ⇒ uni_system_config_tenant 没生效")
	}

	// 请求体里自带别人的 tenant_id：必须被隔离 callback 拒绝，而不是把行写进 A。
	if _, err := repo.SaveConfig(ctxB, &model.SystemConfig{TenantID: scTenantA, Name: "越权"}); err == nil {
		t.Fatal("租户 B 携带 tenant_id=A 创建成功 ⇒ 出现跨租户写")
	}
	var count int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.SystemConfig{}).Where("tenant_id = ?", scTenantA).Count(&count).Error; err != nil {
		t.Fatalf("统计租户 A 行数失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("租户 A 的配置行数=%d，期望仍为 1", count)
	}
}

func TestSystemConfig_UnboundCtxIsRejected(t *testing.T) {
	db := scSetupDB(t)
	repo := &systemConfigRepo{db: db}

	if _, err := repo.GetConfig(context.Background()); err == nil {
		t.Fatal("on 模式无租户 ctx 读配置成功 ⇒ 会读到别的租户的行")
	}
	if _, err := repo.SaveConfig(context.Background(), &model.SystemConfig{Name: "无主"}); err == nil {
		t.Fatal("on 模式无租户 ctx 写配置成功 ⇒ 行会落默认租户")
	}
}
