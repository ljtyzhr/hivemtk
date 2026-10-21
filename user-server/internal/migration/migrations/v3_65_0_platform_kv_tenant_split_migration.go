package migrations

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// identRe DDL 标识符白名单（约束名来自库元数据，拼进 DDL 前仍需校验）
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// PlatformKVTenantSplitMigration system_config_kv 租户分域 + 平台键拆分（SaaS 多租户）
//
// 背景：system_config_kv 曾同时承载两类键——平台接入令牌（桥接/MCP/回调验签/
// 宿主令牌）与租户业务配置（智能体/工具集成/知识库连接器…）。单实例语义下
// 两者混在一张 (key) 主键表里没有冲突；SaaS 化后同一键名必须允许每租户各存
// 一行，否则 B 租户保存配置会覆盖 A 租户。
//
// 本迁移做三件事（单个事务，幂等可重入）：
//  1. 建 platform_config_kv（平台键的新家，key 主键，实例一份）
//  2. 把平台键存量行复制过去（保留原值；ON CONFLICT DO NOTHING 不覆盖新值）
//  3. system_config_kv 补 tenant_id（存量归默认租户 1）并把主键从 (key)
//     复合化为 (tenant_id, key)，随后删除已迁走的平台键行（单一事实源）
//
// 与 AutoMigrate 的分工：模型 Tag 已声明复合主键，新库由 AutoMigrate 直接建对；
// 存量库的主键**不会**被 AutoMigrate 改写（实测 gorm v1.31 AutoMigrate 只加列/
// 建索引，不动既有主键约束），必须由本迁移显式重建。
type PlatformKVTenantSplitMigration struct {
	db *gorm.DB
}

// NewPlatformKVTenantSplitMigration 创建迁移实例
func NewPlatformKVTenantSplitMigration(db *gorm.DB) *PlatformKVTenantSplitMigration {
	return &PlatformKVTenantSplitMigration{db: db}
}

// Version 返回版本号
func (m *PlatformKVTenantSplitMigration) Version() string { return "v3.65.0" }

// Name 返回迁移名称
func (m *PlatformKVTenantSplitMigration) Name() string {
	return "system_config_kv 租户分域与平台键拆分"
}

// Description 返回迁移描述
func (m *PlatformKVTenantSplitMigration) Description() string {
	return "SaaS 多租户：平台键迁至 platform_config_kv；system_config_kv 主键 (key) → (tenant_id, key)，存量归默认租户；幂等可重入"
}

// platformKVKeyWhere 平台键的筛选谓词（键名常量见 model.PlatformConfigKV 注释）。
// 用 SQL 字面量而不是 Go 常量拼接：键名是持久化格式，迁移必须对"代码改名"
// 免疫——迁移引用的是**当时**写进库里的字符串。
// password_policy 不在名单内：密码策略由各租户 admin 维护，属于租户级安全设置，
// 存量行留在 system_config_kv 并随主键复合化归入默认租户。
const platformKVKeyWhere = `(
	key IN ('bridge_ingest_token', 'bridge_ingest_token_prev',
	        'mcp_token', 'security.rules', 'backup.strategy')
	OR key LIKE 'order_webhook_secret_%'
	OR key LIKE 'browser_host_token%'
	OR key LIKE 'embedding_global%'
)`

func (m *PlatformKVTenantSplitMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	return db.Transaction(func(tx *gorm.DB) error {
		// 1. platform_config_kv：列形状与 model.PlatformConfigKV 一致
		if err := tx.Exec(`
CREATE TABLE IF NOT EXISTS platform_config_kv (
	key VARCHAR(100) PRIMARY KEY,
	value TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
)`).Error; err != nil {
			return fmt.Errorf("创建 platform_config_kv 失败: %w", err)
		}

		// 旧表不存在（全新库）：AutoMigrate 已按模型建出新形状，无事可做
		if !tx.Migrator().HasTable("system_config_kv") {
			return nil
		}

		// 2. 复制平台键存量（旧库可能没有 system_config_kv 的更新时间列？该表
		//    自 auth_security_migration v1 起就是四列，直接整行复制）
		if err := tx.Exec(`
INSERT INTO platform_config_kv (key, value, created_at, updated_at)
SELECT key, value, created_at, updated_at FROM system_config_kv
WHERE ` + platformKVKeyWhere + `
ON CONFLICT (key) DO NOTHING`).Error; err != nil {
			return fmt.Errorf("复制平台键到 platform_config_kv 失败: %w", err)
		}

		// 3. tenant_id 列（AutoMigrate 通常已加；迁移独立跑时兜底）
		if err := tx.Exec(`ALTER TABLE system_config_kv ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`).Error; err != nil {
			return fmt.Errorf("system_config_kv 补 tenant_id 失败: %w", err)
		}

		// 4. 主键复合化：先读现有主键定义与约束名
		var pkName, pkDef string
		if err := tx.Raw(`
SELECT c.conname, pg_get_constraintdef(c.oid)
FROM pg_constraint c
JOIN pg_class t ON t.oid = c.conrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE c.contype = 'p'
  AND n.nspname = current_schema()
  AND t.relname = 'system_config_kv'
LIMIT 1`).Row().Scan(&pkName, &pkDef); err != nil {
			return fmt.Errorf("查询 system_config_kv 主键失败: %w", err)
		}
		if !strings.Contains(pkDef, "tenant_id") {
			// 约束名来自 pg_constraint（库元数据，非请求输入）；仍按标识符白名单
			// 校验后再拼 DDL，杜绝任何非标识符字符进入语句
			if !identRe.MatchString(pkName) {
				return fmt.Errorf("system_config_kv 主键约束名不可信: %q", pkName)
			}
			if err := tx.Exec(fmt.Sprintf(`ALTER TABLE system_config_kv DROP CONSTRAINT %s`, pkName)).Error; err != nil {
				return fmt.Errorf("删除旧主键 %s 失败: %w", pkName, err)
			}
			if err := tx.Exec(`ALTER TABLE system_config_kv ADD PRIMARY KEY (tenant_id, key)`).Error; err != nil {
				return fmt.Errorf("创建复合主键 (tenant_id, key) 失败: %w", err)
			}
		}

		// 5. 平台键在租户表中不再保留（单一事实源；重复的副本会让"改哪个才生效"
		//    变成要靠运气判断的问题）
		if err := tx.Exec(`DELETE FROM system_config_kv WHERE ` + platformKVKeyWhere).Error; err != nil {
			return fmt.Errorf("清理已迁走的平台键失败: %w", err)
		}
		return nil
	})
}

// Down 不回滚：主键复合化与键迁址是隔离正确性约束；回滚会把平台键重新混回租户表
func (m *PlatformKVTenantSplitMigration) Down(ctx context.Context) error {
	return nil
}

var _ migration.Migration = (*PlatformKVTenantSplitMigration)(nil)
