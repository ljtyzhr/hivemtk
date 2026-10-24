package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// TenantTableMigration tenants 表创建
//
// SaaS 多租户形态的租户实体表。离线单机部署同样建表（空表为正常状态），
// 保证同一份代码两种形态共享同一 schema 演进。
type TenantTableMigration struct {
	db *gorm.DB
}

func NewTenantTableMigration(db *gorm.DB) *TenantTableMigration {
	return &TenantTableMigration{db: db}
}

func (m *TenantTableMigration) Version() string { return "v3.57.0" }

func (m *TenantTableMigration) Name() string { return "tenants 表创建" }

func (m *TenantTableMigration) Description() string {
	return "SaaS 多租户：创建租户实体表（code 全局唯一/status 正常·停用/plan 预留），幂等可重入"
}

func (m *TenantTableMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	ddl := `
CREATE TABLE IF NOT EXISTS tenants (
	id            BIGSERIAL PRIMARY KEY,
	code          VARCHAR(32) NOT NULL,
	name          VARCHAR(100) NOT NULL,
	status        SMALLINT NOT NULL DEFAULT 1,
	plan          VARCHAR(32) DEFAULT 'free',
	created_at    TIMESTAMPTZ,
	updated_at    TIMESTAMPTZ
)`
	if err := m.db.Exec(ddl).Error; err != nil {
		return fmt.Errorf("创建 tenants 表失败: %w", err)
	}

	idxSQL := `CREATE UNIQUE INDEX IF NOT EXISTS uni_tenants_code ON tenants (code)`
	if err := m.db.Exec(idxSQL).Error; err != nil {
		return fmt.Errorf("创建 tenants.code 唯一索引失败: %w", err)
	}
	return nil
}

// Down 执行降级：租户表承载归属数据，回滚会造成数据丢失，不允许
func (m *TenantTableMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantTableMigration)(nil)
