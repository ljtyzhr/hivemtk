package migrations

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// tenantExemptTables 不参与租户隔离的全局/系统表（specs/001-saas-multi-tenant data-model.md 全局表清单）。
// 与 model 侧登记（T006 交叉校验测试）互为对账：
//   - 清单外的表一律视为租户表（fail-safe：多余加列可回滚清理，漏加列即隔离缺口）
//   - platform_accounts/platform_account_configs 属业务数据，不在豁免之列
var tenantExemptTables = map[string]bool{
	"tenants":                 true, // 租户实体本身
	"migration_records":       true, // 迁移簿记
	"migration_checkpoints":   true, // 迁移簿记
	"config_params":           true, // 参数定义目录（全局只读声明；值在 tenant_config_params）
	"config_param_groups":     true, // 参数分组目录（全局只读声明）
	"config_param_audit_logs": true, // 平台侧参数审计
	"platform_config_kv":      true, // 平台接入令牌/策略（实例唯一，租户不可见）
	"cron_job_leases":         true, // 进程协调租约：key 是任务名、天然全局（扇出留在租约之内逐租户跑）
	"bridge_metrics":          true, // 运维指标时间序列（metric_name/labels/ts），无租户维度；按租户切分只会把同一张看板拆成 N 份
	"dictionaries":            true, // 全局只读共享
	"dictionary_transitions":  true, // 全局只读共享
	"backups":                 true, // 系统操作
	"restore_records":         true, // 系统操作
}

// tenantColumnBatchSize 每批处理的表数：批间独立事务，中断可续跑（幂等）
const tenantColumnBatchSize = 30

// TenantColumnBatchMigration 业务表批量补 tenant_id 列（specs/001-saas-multi-tenant T007）
//
// 动态扫描 public schema 全部 BASE TABLE（豁免清单除外），幂等加列：
//
//	ALTER TABLE x ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1
//
// 存量数据经 DEFAULT 1 自动归属默认租户，无需回填步骤。
type TenantColumnBatchMigration struct {
	db *gorm.DB
}

// NewTenantColumnBatchMigration 创建业务表加列迁移
func NewTenantColumnBatchMigration(db *gorm.DB) *TenantColumnBatchMigration {
	return &TenantColumnBatchMigration{db: db}
}

// Version 返回版本号
func (m *TenantColumnBatchMigration) Version() string { return "v3.58.0" }

// Name 返回迁移名称
func (m *TenantColumnBatchMigration) Name() string { return "业务表批量补 tenant_id 列" }

// Description 返回迁移描述
func (m *TenantColumnBatchMigration) Description() string {
	return "SaaS 多租户：public schema 全部业务表幂等加 tenant_id bigint NOT NULL DEFAULT 1（存量数据归默认租户），分批独立事务可中断续跑"
}

// Up 执行升级
func (m *TenantColumnBatchMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	tables, err := m.businessTables()
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}

	var altered int
	for start := 0; start < len(tables); start += tenantColumnBatchSize {
		end := start + tenantColumnBatchSize
		if end > len(tables) {
			end = len(tables)
		}
		batch := tables[start:end]

		tx := m.db.WithContext(ctx).Begin()
		if tx.Error != nil {
			return fmt.Errorf("开启批次事务失败: %w", tx.Error)
		}
		var batchAltered int
		for _, t := range batch {
			ddl := fmt.Sprintf(`ALTER TABLE %q ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`, t)
			if err := tx.Exec(ddl).Error; err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("表 %s 加列失败: %w", t, err)
			}
			batchAltered++
		}
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("批次提交失败（%d..%d）: %w", start, end, err)
		}
		altered += batchAltered
	}

	if m.db.Migrator() != nil {
		fmt.Printf("[v3.58.0] tenant_id 加列完成：处理 %d 张业务表\n", altered)
	}
	return nil
}

// businessTables 枚举需要加列的业务表：
// public schema 全部 BASE TABLE，按 tenantExemptTables（唯一事实源）过滤，
// 并排除已有 tenant_id 列的表（幂等重入）。
func (m *TenantColumnBatchMigration) businessTables() ([]string, error) {
	rows, err := m.db.Raw(`
		SELECT t.table_name
		FROM information_schema.tables t
		WHERE t.table_schema = current_schema()
		  AND t.table_type = 'BASE TABLE'
		  AND NOT EXISTS (
			SELECT 1 FROM information_schema.columns c
			WHERE c.table_schema = t.table_schema
			  AND c.table_name = t.table_name
			  AND c.column_name = 'tenant_id'
		  )
		ORDER BY t.table_name
	`).Rows()
	if err != nil {
		return nil, fmt.Errorf("枚举业务表失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("扫描表名失败: %w", err)
		}
		if tenantExemptTables[name] {
			continue
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// Down 不回滚：tenant_id 列承载归属数据，回滚破坏业务数据
func (m *TenantColumnBatchMigration) Down(ctx context.Context) error {
	return nil
}

// TenantExemptTablesSnapshot 供 T006 交叉校验测试对账用（只读快照，调用方不得修改）
func TenantExemptTablesSnapshot() []string {
	out := make([]string, 0, len(tenantExemptTables))
	for k := range tenantExemptTables {
		out = append(out, k)
	}
	// 稳定排序便于测试断言
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if strings.Compare(out[i], out[j]) < 0 {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

var _ migration.Migration = (*TenantColumnBatchMigration)(nil)
