package migrations

import (
	"context"
	"fmt"
	"strings"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// tenantExemptTables 不参与租户隔离的全局/系统表（与 model 侧登记的清单互为对账）：
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

// TenantColumnBatchMigration 业务表批量补 tenant_id 列
//
// 动态扫描 public schema 全部 BASE TABLE（豁免清单除外），幂等加列：
//
//	ALTER TABLE x ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1
//
// 存量数据经 DEFAULT 1 自动归属默认租户，无需回填步骤。
type TenantColumnBatchMigration struct {
	db *gorm.DB

	// tables 仅测试用覆盖清单（与 v3_79_0 的 withTables 同一惯例）：演练要能在临时探针表上
	// 复现"存量数据升级"，否则断言会被测试库里的其它表搅成不确定。生产路径永远是 nil ⇒
	// 走 businessTables() 的真库内省。
	//
	// 注意：这个字段是"仅测试用"的约定，不是语言保证。生产装配点在
	// initial_schema.go 的 register(NewTenantColumnBatchMigration(db)) —— 那里若有人
	// 链了 .withTables(...)，单表覆盖会让"真库内省"静默退化成"只处理指定表"。这句话
	// 没法用 if 防住（字段就是给测试开的口子），靠的是 TestTenantColumnBatchRegistration
	// 用例钉死生产注册点不得携带覆盖清单。
	tables []string
}

// withTables 仅测试用：把清单换成临时探针表，验证同一套 DDL 在真库上的行为。
func (m *TenantColumnBatchMigration) withTables(tables ...string) *TenantColumnBatchMigration { //nolint:unused // 仅被 *_test.go 引用，生产路径未用
	m.tables = tables
	return m
}

func NewTenantColumnBatchMigration(db *gorm.DB) *TenantColumnBatchMigration {
	return &TenantColumnBatchMigration{db: db}
}

func (m *TenantColumnBatchMigration) Version() string { return "v3.58.0" }

func (m *TenantColumnBatchMigration) Name() string { return "业务表批量补 tenant_id 列" }

func (m *TenantColumnBatchMigration) Description() string {
	return "SaaS 多租户：public schema 全部业务表幂等加 tenant_id bigint NOT NULL DEFAULT 1（存量数据归默认租户），分批独立事务可中断续跑"
}

func (m *TenantColumnBatchMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	tables := m.tables
	if tables == nil {
		var err error
		tables, err = m.businessTables()
		if err != nil {
			return err
		}
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

	// 后复核：PostgreSQL 的 `ADD COLUMN IF NOT EXISTS` 对已存在列是纯 no-op
	// （连类型不匹配都不报错），"清单收紧"修不了形态错的列 —— 必须跑完后回到
	// information_schema 现查：清单里的表若仍有隔离缺口级形态错（类型错 / 可空），
	// 报错中断升级。静默通过等于把"永远查不出的隔离缺口"埋进土里。
	//
	// 复核口径只判隔离缺口（bigint / NOT NULL），不判默认值：`DEFAULT 1` 的语义只属于
	// 本迁移的加列语境（存量行靠它归默认租户）；别的路径建的列（如 AutoMigrate 从
	// `TenantConfigParam` 模型建的 `tenant_config_params.tenant_id`，`not null` 无默认）
	// 写入端必填、无存量行需要归属，是合法形态 —— 复核若连默认值一起判，会把合法建表判红。
	if err := m.verifyColumns(tables); err != nil {
		return err
	}
	return nil
}

// verifyColumns Up 后复核：清单里的每张表都必须有隔离缺口级的正确形态（bigint / NOT NULL）。
func (m *TenantColumnBatchMigration) verifyColumns(tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	rows, err := m.db.Raw(`
		SELECT table_name, data_type, is_nullable
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND column_name = 'tenant_id'
		  AND (data_type <> 'bigint' OR is_nullable <> 'NO')
	`).Rows()
	if err != nil {
		return fmt.Errorf("复核 tenant_id 列形态失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	inList := make(map[string]bool, len(tables))
	for _, t := range tables {
		inList[t] = true
	}
	var problems []string
	for rows.Next() {
		var name, dataType, nullable string
		if err := rows.Scan(&name, &dataType, &nullable); err != nil {
			return fmt.Errorf("复核扫描失败: %w", err)
		}
		if inList[name] {
			problems = append(problems, fmt.Sprintf(
				"表 %s 的 tenant_id 列形态错（%s / 可空=%s）：ADD COLUMN IF NOT EXISTS "+
					"对已存在列是 no-op，此缺口本迁移修不回来 —— 请人工裁决该表列的来路", name, dataType, nullable))
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("复核遍历失败: %w", err)
	}
	if len(problems) > 0 {
		return fmt.Errorf("v3.58.0 后复核失败（%d 处）：\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	return nil
}

// businessTables 枚举需要加列的业务表：
// public schema 全部 BASE TABLE，按 tenantExemptTables（唯一事实源）过滤，
// 并排除已有形态正确的 tenant_id 列的表（幂等重入）。
//
// "形态正确"= bigint / NOT NULL（隔离缺口级）：一张 `tenant_id varchar(20) NULL` 的表
// 会被 `IF NOT EXISTS` 静默跳过（列已存在，ALTER 变 no-op），成为永不报错的隔离缺口，
// 所以 Up 末尾还有后复核：清单跑完仍查到形态错即报错中断。
// 默认值刻意不判：`DEFAULT 1` 的语义只属于本迁移的加列语境（存量行靠它归默认租户）；
// 别的路径建的列（AutoMigrate 从模型建的 `tenant_config_params.tenant_id`，写入端必填、
// 无存量行需要归属）是合法形态，判它会把合法建表判红。
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
			  AND c.data_type = 'bigint'
			  AND c.is_nullable = 'NO'
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

// TenantExemptTablesSnapshot 供测试对账用（只读快照，调用方不得修改）
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
