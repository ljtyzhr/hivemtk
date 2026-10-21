package migrations

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/migration"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// TenantSiteConfigAndFeatureFlagMigration 站点配置与功能开关租户化
//
// SaaS 形态下 site_name/注册开关/上传上限是商户自己的运营口径，功能开关的
// 灰度与 kill switch 也是每租户独立决策——两者都不能再是"实例唯一一行/一键"。
//
// 本迁移做四件事（单事务，幂等可重入）：
//  1. system_config / feature_flags / 两张开关日志表补 tenant_id（存量归默认租户 1）
//  2. system_config 去重后建 tenant_id 唯一索引（一租户一站点配置）
//  3. feature_flags 上旧的 key 单列唯一索引全部删除（跨租户同名 key 必须允许）
//  4. 建复合唯一索引 (tenant_id, key)
//
// AutoMigrate 只加列/建索引，不会删除既有的 key 唯一索引；不删它，第二个租户
// 创建同名开关必然撞唯一约束 —— 这是必须显式重建索引的原因。
type TenantSiteConfigAndFeatureFlagMigration struct {
	db *gorm.DB
}

// NewTenantSiteConfigAndFeatureFlagMigration 创建迁移实例
func NewTenantSiteConfigAndFeatureFlagMigration(db *gorm.DB) *TenantSiteConfigAndFeatureFlagMigration {
	return &TenantSiteConfigAndFeatureFlagMigration{db: db}
}

// Version 返回版本号
func (m *TenantSiteConfigAndFeatureFlagMigration) Version() string { return "v3.66.0" }

// Name 返回迁移名称
func (m *TenantSiteConfigAndFeatureFlagMigration) Name() string {
	return "站点配置与功能开关租户化"
}

// Description 返回迁移描述
func (m *TenantSiteConfigAndFeatureFlagMigration) Description() string {
	return "SaaS 多租户：system_config 一租户一行；feature_flags 唯一键 key → (tenant_id, key)；存量归默认租户；幂等可重入"
}

func (m *TenantSiteConfigAndFeatureFlagMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	return db.Transaction(func(tx *gorm.DB) error {
		// 1. 补列（全新库由 AutoMigrate 直接建对；独立跑迁移时兜底）
		for _, table := range []string{
			"system_config", "feature_flags",
			"feature_flag_audit_logs", "feature_flag_eval_logs",
		} {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			if err := tx.Exec(fmt.Sprintf(
				`ALTER TABLE %s ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`, table)).Error; err != nil {
				return fmt.Errorf("%s 补 tenant_id 失败: %w", table, err)
			}
		}

		// 2. system_config：一租户一行。用 First() 定位单例行，正常
		//    不会出现同租户多行；若确有（人工导入/早期并发），保留最小 id
		//    （First 命中的就是它，后续 Save 一直更新这一行）。
		if tx.Migrator().HasTable("system_config") {
			if err := tx.Exec(`
DELETE FROM system_config a USING system_config b
WHERE a.tenant_id = b.tenant_id AND a.id > b.id`).Error; err != nil {
				return fmt.Errorf("system_config 同租户重复行清理失败: %w", err)
			}
			if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uni_system_config_tenant
ON system_config (tenant_id)`).Error; err != nil {
				return fmt.Errorf("system_config 租户唯一索引创建失败: %w", err)
			}
		}

		// 3. feature_flags：删除 key 单列唯一索引（列组合从库元数据现读，
		//    不依赖 GORM 生成的索引名）
		if tx.Migrator().HasTable("feature_flags") {
			indexes, err := keyOnlyUniqueIndexes(tx)
			if err != nil {
				return err
			}
			for _, name := range indexes {
				if !identRe.MatchString(name) {
					return fmt.Errorf("feature_flags 唯一索引名不可信: %q", name)
				}
				if err := tx.Exec(fmt.Sprintf(`DROP INDEX IF EXISTS %s`, name)).Error; err != nil {
					return fmt.Errorf("删除 feature_flags 旧唯一索引 %s 失败: %w", name, err)
				}
			}
			// 4. 复合唯一键：key 的唯一性语义降为租户内
			if err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS uni_feature_flags_tenant_key
ON feature_flags (tenant_id, key)`).Error; err != nil {
				return fmt.Errorf("feature_flags 复合唯一索引创建失败: %w", err)
			}
		}

		// 日志表按租户过滤是常态查询，补普通索引（AutoMigrate 已建时幂等跳过）
		for _, table := range []string{"feature_flag_audit_logs", "feature_flag_eval_logs"} {
			if !tx.Migrator().HasTable(table) {
				continue
			}
			idx := table + "_tenant_id"
			if !identRe.MatchString(idx) {
				return fmt.Errorf("索引名不可信: %q", idx)
			}
			if err := tx.Exec(fmt.Sprintf(
				`CREATE INDEX IF NOT EXISTS %s ON %s (tenant_id)`, idx, table)).Error; err != nil {
				return fmt.Errorf("%s 租户索引创建失败: %w", table, err)
			}
		}
		return nil
	})
}

// keyOnlyUniqueIndexes 返回 feature_flags 上"列组合恰为 (key)"的唯一索引名。
// 表达式索引/多列索引不会被误删（多列索引的 attnum 为 0，JOIN 后不计入）。
func keyOnlyUniqueIndexes(tx *gorm.DB) ([]string, error) {
	rows, err := tx.Raw(`
SELECT i.relname AS index_name, array_agg(a.attname ORDER BY k.ord) AS columns
FROM pg_index ix
JOIN pg_class i ON i.oid = ix.indexrelid
JOIN pg_class t ON t.oid = ix.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
CROSS JOIN LATERAL unnest(ix.indkey) WITH ORDINALITY AS k(attnum, ord)
JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = k.attnum
WHERE n.nspname = current_schema()
  AND t.relname = 'feature_flags'
  AND ix.indisunique
GROUP BY i.relname`).Rows()
	if err != nil {
		return nil, fmt.Errorf("查询 feature_flags 唯一索引失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var name string
		var columns pq.StringArray
		if err := rows.Scan(&name, &columns); err != nil {
			return nil, fmt.Errorf("扫描 feature_flags 索引元数据失败: %w", err)
		}
		if len(columns) == 1 && strings.EqualFold(columns[0], "key") {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// Down 不回滚：复合唯一键是隔离正确性约束；回滚会把同名 key 重新锁成实例唯一
func (m *TenantSiteConfigAndFeatureFlagMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*TenantSiteConfigAndFeatureFlagMigration)(nil)
