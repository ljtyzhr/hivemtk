package migrations

import (
	"context"
	"fmt"
	"sort"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// TenantLeadingIndexMigration v3.79.0（SaaS 审计 A6）：给 11 张租户表补 `tenant_id` 前导索引。
//
// 为什么这 11 张表会漏（真库 2026-10-23 复测，381 张表里恰好这 11 张）：
// 它们的建表走的是**手写 DDL 迁移**（`CREATE TABLE …` + 若干 `CREATE INDEX`），
// 而不是 AutoMigrate 从模型 tag 生成。模型的 `TenantID` 字段确实写了 `index` tag
// （如 `internal/model/live_code_click_log.go:7`），但那份 tag 只对"由 AutoMigrate 建的表"生效，
// 对手写 DDL 建出来的表**不起作用**；于是"模型说建了索引、库里没有"长期共存，没人红。
//
// 判据不是 grep 模型 tag，是对真库查"该表的索引族里有没有以 tenant_id 打头的列"：
//
//	SELECT c.relname FROM pg_class c JOIN pg_attribute a
//	  ON a.attrelid=c.oid AND a.attname='tenant_id' AND NOT a.attisdropped
//	 WHERE c.relkind='r' AND c.relnamespace='public'::regnamespace
//	   AND NOT EXISTS (SELECT 1 FROM pg_index i JOIN pg_attribute ia
//	                     ON ia.attrelid=i.indrelid AND ia.attnum=i.indkey[0]
//	                    WHERE i.indrelid=c.oid AND ia.attname='tenant_id');
//
// 表清单写死在常量里而不是"运行时扫全库"：迁移必须对同一份代码给出同一组动作，
// 而"扫全库"会在不同环境做不同事（生产多一张临时表就多建一个索引），且不可回滚推定。
//
// 幂等：已有 tenant_id 前导索引的表跳过；表或列不存在（全新部署尚未建表）也跳过。
type TenantLeadingIndexMigration struct {
	db     *gorm.DB
	tables []string
}

var _ migration.Migration = (*TenantLeadingIndexMigration)(nil)

// tenantLeadingIndexTables 2026-10-23 真库实测缺口清单（与审计报告 §P2-2 逐表一致）。
var tenantLeadingIndexTables = []string{
	"bridge_metrics",
	"email_lists",
	"email_smtps",
	"fts_test",
	"intent_logs",
	"livecode_click_log",
	"llm_routing_audit",
	"llm_usage_records",
	"obs_configs",
	"qr_code_click_log",
	"script_recommends",
}

func NewTenantLeadingIndexMigration(db *gorm.DB) *TenantLeadingIndexMigration {
	return &TenantLeadingIndexMigration{db: db, tables: tenantLeadingIndexTables}
}

// withTables 仅测试用：把清单换成临时探针表，验证同一套判据/DDL 在真库上的行为。
func (m *TenantLeadingIndexMigration) withTables(tables ...string) *TenantLeadingIndexMigration { //nolint:unused //// 仅被 *_test.go 引用，生产路径未用
	m.tables = tables
	return m
}

func (m *TenantLeadingIndexMigration) Version() string { return "v3.79.0" }

func (m *TenantLeadingIndexMigration) Name() string { return "租户表 tenant_id 前导索引补齐" }

func (m *TenantLeadingIndexMigration) Description() string {
	return "为 11 张手写 DDL 建表的租户表补 idx_<table>_tenant_id（幂等；Up 后复核，仍有缺口即报错）"
}

func (m *TenantLeadingIndexMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	db := m.db.WithContext(ctx)

	var missing []string
	for _, table := range m.tables {
		if !db.Migrator().HasTable(table) {
			continue // 全新部署尚未建表：由建表迁移自己负责
		}
		if !db.Migrator().HasColumn(table, "tenant_id") {
			continue // 非租户表（或列已改名）：与本迁移无关
		}
		has, err := hasLeadingTenantIndex(db, table)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if err := db.Exec(tenantLeadingIndexDDL(table)).Error; err != nil {
			return fmt.Errorf("为 %s 建 tenant_id 索引失败: %w", table, err)
		}
	}

	// Up 之后复核：语言层面的"我建了"不算，库里查得到才算。
	for _, table := range m.tables {
		if !db.Migrator().HasTable(table) || !db.Migrator().HasColumn(table, "tenant_id") {
			continue
		}
		has, err := hasLeadingTenantIndex(db, table)
		if err != nil {
			return err
		}
		if !has {
			missing = append(missing, table)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("以下租户表在迁移执行后仍缺 tenant_id 前导索引: %v", missing)
	}
	return nil
}

// Down 走 declineIndexDrop 口径（降级不销毁在用对象），不按名字无脑 DROP。
//
// 本迁移只给"确实缺 tenant_id 前导索引"的表补索引；那些本来就有前导索引的表在 Up 里
// 被跳过，而它们的索引名恰好与本迁移的命名规则相同（`idx_<表>_tenant_id`，模型 tag
// `index` 经 AutoMigrate 建出来）⇒ 按名字 DROP 会把**不属于本迁移的**在用索引删掉，
// 降级后的实例就此长期 seq scan（`a_full_chain_migration_test.go` 实测抓出
// livecode_click_log / qr_code_click_log 两枚）。索引是纯收益对象、off 模式同样有
// tenant_id 列，留着无副作用。
func (m *TenantLeadingIndexMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	names := make([]string, 0, len(m.tables))
	for _, table := range m.tables {
		names = append(names, tenantLeadingIndexName(table))
	}
	declineIndexDrop(m.Version(), names...)
	return nil
}

func tenantLeadingIndexName(table string) string {
	return "idx_" + table + "_tenant_id"
}

func tenantLeadingIndexDDL(table string) string {
	return fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON %s (tenant_id)", tenantLeadingIndexName(table), table)
}

// hasLeadingTenantIndex 该表的索引族里是否存在**以 tenant_id 打头**的索引。
//
// 只认前导列：`(id, tenant_id)` 这种对隔离条件（`WHERE tenant_id = ? AND …`）没用，
// 否则"有索引"会变成一句自我安慰。
func hasLeadingTenantIndex(db *gorm.DB, table string) (bool, error) {
	var cnt int64
	err := db.Raw(`
		SELECT COUNT(1) FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute ia ON ia.attrelid = i.indrelid AND ia.attnum = i.indkey[0]
		WHERE n.nspname = 'public' AND c.relname = ? AND ia.attname = 'tenant_id' AND NOT ia.attisdropped`,
		table).Scan(&cnt).Error
	if err != nil {
		return false, fmt.Errorf("查 %s 的索引族失败: %w", table, err)
	}
	return cnt > 0, nil
}
