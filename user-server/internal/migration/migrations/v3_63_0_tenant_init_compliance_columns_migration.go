package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// TenantInitComplianceColumnsMigration 初始化静默合规采集列（003 T010）
//
// tenants 表加三列（只加列不改老列，零回滚风险）：
//
//	init_ip                  VARCHAR(64)  —— 注册/初始化请求的 ClientIP
//	init_ua                  VARCHAR(128) —— User-Agent 的 sha256 hex（不存原文）
//	init_device_fingerprint  VARCHAR(128) —— UA+IP 派生的设备指纹（sha256 hex）
//
// 模型侧（model.Tenant）同步声明了这三列供 AutoMigrate 兜底，但生产口径
// 以本迁移为准（与 tenant_table/column_batch 同一惯例：DDL 走显式迁移，
// 不依赖 AutoMigrate 在启动时"顺手"加列）。
type TenantInitComplianceColumnsMigration struct {
	db *gorm.DB
}

// NewTenantInitComplianceColumnsMigration 创建迁移实例
func NewTenantInitComplianceColumnsMigration(db *gorm.DB) *TenantInitComplianceColumnsMigration {
	return &TenantInitComplianceColumnsMigration{db: db}
}

func (m *TenantInitComplianceColumnsMigration) Version() string { return "v3.63.0" }

func (m *TenantInitComplianceColumnsMigration) Name() string {
	return "tenants_init_compliance_columns"
}

func (m *TenantInitComplianceColumnsMigration) Description() string {
	return "v3.63.0：tenants 加 init_ip/init_ua/init_device_fingerprint 三列（静默合规采集，003 T010）；幂等可重入"
}

func (m *TenantInitComplianceColumnsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)
	if !db.Migrator().HasTable("tenants") {
		return nil // 表还没建（由 tenant_table 迁移/AutoMigrate 负责），此处无物可加
	}
	ddl := []string{
		`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS init_ip varchar(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS init_ua varchar(128) NOT NULL DEFAULT ''`,
		`ALTER TABLE tenants ADD COLUMN IF NOT EXISTS init_device_fingerprint varchar(128) NOT NULL DEFAULT ''`,
	}
	for _, q := range ddl {
		if err := db.Exec(q).Error; err != nil {
			return fmt.Errorf("tenants 加列失败（%s）: %w", q, err)
		}
	}
	return nil
}

// Down 不回滚：三列承载合规留痕数据，删列即销毁留痕
func (m *TenantInitComplianceColumnsMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantInitComplianceColumnsMigration)(nil)
