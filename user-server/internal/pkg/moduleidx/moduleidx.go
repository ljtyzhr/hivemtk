// Package moduleidx 功能模块相关表的唯一索引形状规范化
//
// 存在的理由见 internal/pkg/db/migrate.go 里调用处的注释：
// AutoMigrate 不删除形状已变的索引，而 ON CONFLICT 依赖索引的**精确列集**，
// 二者叠加就是「库比代码老 → 42P10 → 模块下发全量失败 → 付费模块闸恒放行」。
//
// ============================================================
// 为什么修复写在这里，而不写成 internal/migration 的迁移
//
// 实测：main.go 调用的是 migrationSvc.ExecuteUpgrade(ctx, "v1.0.0", "v1.0.0")
// —— from 与 to 相同，**迁移体系在这个入口下不执行任何迁移**。
// 把修复写进迁移等于写进一个永远不跑的地方，而 migration_records 里
// 却会（因历史原因）留着 completed 记录，让人误以为"已经修过了"。
//
// 已删除那个迁移文件。修复落在 AutoMigrate 之后、随每次启动幂等执行 ——
// 这才是"对既有库也成立"的位置。
package moduleidx

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// target 一个待规范化的索引
type target struct {
	table   string
	index   string
	columns string // 期望的列集（顺序敏感：ON CONFLICT 按此匹配）
}

// targets 需要规范化的索引清单
//
// 判据：`tenant_slot` 单独构成唯一键（当前单部署恒为 0），而不是
// (tenant_id, tenant_slot, module_key) —— 后者是模型演进中途的形状，
// 已与模型声明不符。
var targets = []target{
	{table: "tenant_modules", index: "idx_local_tenant_module", columns: "tenant_slot, module_key"},
}

// Normalize 幂等规范化。返回处理的索引数。
func Normalize(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("db is nil")
	}
	n := 0
	for _, t := range targets {
		changed, err := fixOne(db, t)
		if err != nil {
			return n, fmt.Errorf("索引 %s（表 %s）规范化失败: %w", t.index, t.table, err)
		}
		if changed {
			n++
		}
	}
	return n, nil
}

func fixOne(db *gorm.DB, t target) (bool, error) {
	if !db.Migrator().HasTable(t.table) {
		return false, nil // 全新安装：交给 AutoMigrate 按模型建
	}
	ctx := context.Background()

	var defs []string
	if err := db.WithContext(ctx).Raw(
		`SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i
		 WHERE i.indrelid = to_regclass(?::text)
		   AND i.indexrelid::regclass::text = ?`, t.table, t.index,
	).Scan(&defs).Error; err != nil {
		return false, err
	}

	if len(defs) > 0 {
		if matches(defs[0], t.columns) {
			return false, nil // 已是目标形状
		}
		// CREATE INDEX IF NOT EXISTS 不会纠正形状（同名即跳过）—— 这正是漂移成因。
		// 必须先删。
		if err := db.WithContext(ctx).Exec(`DROP INDEX IF EXISTS ` + t.index).Error; err != nil {
			return false, fmt.Errorf("删除旧索引: %w", err)
		}
	}

	if err := db.WithContext(ctx).Exec(fmt.Sprintf(
		`CREATE UNIQUE INDEX %s ON %s (%s)`, t.index, t.table, t.columns)).Error; err != nil {
		// 有重复行时降级为非唯一索引并报出，让调用方决定是否 fail-closed。
		// 这里不 panic：索引形状问题不该让服务起不来（仓储层另有慢路径兜底）。
		if derr := db.WithContext(ctx).Exec(fmt.Sprintf(
			`CREATE INDEX %s ON %s (%s)`, t.index, t.table, t.columns)).Error; derr != nil {
			return false, fmt.Errorf("重建唯一索引失败(%v)，降级重建亦失败: %w", err, derr)
		}
		return true, fmt.Errorf("存在重复键，已降级为非唯一索引（请人工清理后重跑）")
	}
	return true, nil
}

// matches 判定 pg_get_indexdef 的列集是否等于期望值
func matches(indexdef, columns string) bool {
	open := strings.Index(indexdef, "(")
	closeIdx := strings.LastIndex(indexdef, ")")
	if open < 0 || closeIdx <= open {
		return false
	}
	got := splitCols(indexdef[open+1 : closeIdx])
	want := splitCols(columns)
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func splitCols(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
