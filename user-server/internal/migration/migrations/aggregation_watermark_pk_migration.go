package migrations

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// AggregationWatermarkPKMigration aggregation_watermarks 主键复合化（specs/001-saas-multi-tenant）
//
// 水位线原主键 (source) 是实例级单行游标；SaaS 多租户下各租户消费水位必须互不越线，
// 主键升级为 (tenant_id, source)。与仓储层 ON CONFLICT (tenant_id, source) 配套，
// 幂等：主键已含 tenant_id 即跳过。
type AggregationWatermarkPKMigration struct {
	db *gorm.DB
}

// NewAggregationWatermarkPKMigration 创建水位线主键复合化迁移
func NewAggregationWatermarkPKMigration(db *gorm.DB) *AggregationWatermarkPKMigration {
	return &AggregationWatermarkPKMigration{db: db}
}

// Version 返回版本号
func (m *AggregationWatermarkPKMigration) Version() string { return "v3.60.0" }

// Name 返回迁移名称
func (m *AggregationWatermarkPKMigration) Name() string {
	return "aggregation_watermarks 主键复合化"
}

// Description 返回迁移描述
func (m *AggregationWatermarkPKMigration) Description() string {
	return "SaaS 多租户：水位线主键 (source) → (tenant_id, source)，各租户消费水位互不越线；幂等可重入"
}

// Up 执行升级
func (m *AggregationWatermarkPKMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	var def string
	err := m.db.Raw(`
		SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c
		JOIN pg_class t ON t.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = t.relnamespace
		WHERE c.contype = 'p'
		  AND n.nspname = current_schema()
		  AND t.relname = 'aggregation_watermarks'
		LIMIT 1
	`).Scan(&def).Error
	if err != nil {
		return fmt.Errorf("查询水位线主键失败: %w", err)
	}
	if strings.Contains(def, "tenant_id") {
		return nil // 已复合化，幂等跳过
	}

	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`ALTER TABLE aggregation_watermarks DROP CONSTRAINT aggregation_watermarks_pkey`).Error; err != nil {
			return fmt.Errorf("删除旧主键失败: %w", err)
		}
		if err := tx.Exec(`ALTER TABLE aggregation_watermarks ADD PRIMARY KEY (tenant_id, source)`).Error; err != nil {
			return fmt.Errorf("创建复合主键失败: %w", err)
		}
		return nil
	})
}

// Down 不回滚：主键复合化是消费隔离的正确性约束
func (m *AggregationWatermarkPKMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*AggregationWatermarkPKMigration)(nil)
