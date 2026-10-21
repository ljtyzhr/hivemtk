// system_config_kv_tenant_test.go SystemConfigKV 的租户契约。
//
// v3.65.0 把主键从 (key) 复合化为 (tenant_id, key)：同一键名允许每租户各存
// 一行。这里钉三件事——同名键互不覆盖、on 模式漏绑 ctx 返回专用哨兵且不落行、
// off 模式显式归默认租户。
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
	kvTenantA = uint(83)
	kvTenantB = uint(84)
)

func kvSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.SystemConfigKV{})
	if db == nil {
		t.Fatal("测试库不可达：KV 租户主键判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func TestSystemConfigKV_SameKeyIsPerTenant(t *testing.T) {
	db := kvSetupDB(t)
	repo := NewSystemConfigKVRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), kvTenantA)
	ctxB := tenant.WithTenantID(context.Background(), kvTenantB)

	if _, err := repo.Upsert(ctxA, "agent_settings", `{"a":1}`); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if _, err := repo.Upsert(ctxB, "agent_settings", `{"b":2}`); err != nil {
		t.Fatalf("租户 B 写同名键被拒（主键没复合化？）: %v", err)
	}

	gotA, err := repo.Get(ctxA, "agent_settings")
	if err != nil || gotA != `{"a":1}` {
		t.Fatalf("租户 A 读回 %q err=%v，期望自己的值", gotA, err)
	}
	gotB, err := repo.Get(ctxB, "agent_settings")
	if err != nil || gotB != `{"b":2}` {
		t.Fatalf("租户 B 读回 %q err=%v——同名键被覆盖了", gotB, err)
	}

	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.SystemConfigKV{}).Where("key = ?", "agent_settings").Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 2 {
		t.Fatalf("物理行数=%d，期望每租户各一行共 2 行", rows)
	}
}

func TestSystemConfigKV_UnboundCtxIsRejected(t *testing.T) {
	db := kvSetupDB(t)
	repo := NewSystemConfigKVRepositoryWithDB(db)

	if _, err := repo.Get(context.Background(), "k"); !errors.Is(err, ErrKVTenantContextMissing) {
		t.Fatalf("on 模式无租户 ctx 读返回 err=%v，期望 ErrKVTenantContextMissing", err)
	}
	if _, err := repo.Upsert(context.Background(), "k", "v"); !errors.Is(err, ErrKVTenantContextMissing) {
		t.Fatalf("on 模式无租户 ctx 写返回 err=%v，期望 ErrKVTenantContextMissing", err)
	}
	var rows int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.SystemConfigKV{}).Count(&rows).Error; err != nil {
		t.Fatalf("统计物理行数失败: %v", err)
	}
	if rows != 0 {
		t.Fatalf("漏绑 ctx 的写入落了 %d 行 ⇒ 静默写进了默认租户", rows)
	}
}

func TestSystemConfigKV_OffModeUsesDefaultTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.SystemConfigKV{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	repo := NewSystemConfigKVRepositoryWithDB(db)

	if _, err := repo.Upsert(context.Background(), "off_mode_key", "v"); err != nil {
		t.Fatalf("off 模式写入失败: %v", err)
	}
	var row model.SystemConfigKV
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("key = ?", "off_mode_key").First(&row).Error; err != nil {
		t.Fatalf("off 模式读取物理行失败: %v", err)
	}
	if row.TenantID != tenant.DefaultTenantID {
		t.Fatalf("off 模式 tenant_id=%d，期望默认租户 %d", row.TenantID, tenant.DefaultTenantID)
	}
}
