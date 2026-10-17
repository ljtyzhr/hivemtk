package db

import (
	"fmt"
	"regexp"
	"strings"

	"hivemtk-user/internal/pkg/tenancyidx"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 租户隔离 GORM callback（specs/001-saas-multi-tenant T005）
//
// 隔离的唯一执行者，业务代码零入侵：
//   - off 模式（TENANT_MODE=off，默认）：全部 callback 直通，SQL 形状与历史版本逐字节等价
//   - on 模式：Query/Update/Delete 自动注入 `WHERE tenant_id = ?`；Create 自动填充
//     tenant_id 列；上下文无租户 ⇒ fail-closed 报错（绝不默认全量）
//   - Raw/Exec（预构建 SQL）：off 直通；on 模式引用租户表即拒绝——数据访问唯一形态
//     是 GORM API（T012-T014 已清偿历史债务，门禁防回归），运行时这里兜第二道闸
//
// 注册点：db.AutoMigrate()（生产全局 DB）与 testutil.NewTestDB（测试连接）；
// 同名重复注册等价替换，幂等。

// tenantTableRefRe 从预构建 SQL 中提取疑似表名（FROM/INTO/UPDATE/JION 后的标识符）
var tenantTableRefRe = regexp.MustCompile(`(?i)\b(?:from|into|update|join)\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)

// RegisterTenantCallbacks 在给定连接上注册六类租户隔离 callback（幂等）
func RegisterTenantCallbacks(gdb *gorm.DB) {
	if gdb == nil {
		return
	}
	cb := gdb.Callback()
	_ = cb.Query().Before("gorm:query").Register("tenant:scope_query", scopeQueryForTenant)
	_ = cb.Create().Before("gorm:before_create").Register("tenant:fill_create", fillCreateForTenant)
	_ = cb.Update().Before("gorm:update").Register("tenant:scope_update", scopeUpdateForTenant)
	_ = cb.Delete().Before("gorm:delete").Register("tenant:scope_delete", scopeDeleteForTenant)
	_ = cb.Row().Before("gorm:row").Register("tenant:guard_row", guardRawForTenant)
	_ = cb.Raw().Before("gorm:raw").Register("tenant:guard_raw", guardRawForTenant)
}

// tenantInjectionActive 是否处于注入生效状态（on 模式）。off 一律直通。
func tenantInjectionActive() bool { return tenant.IsMultiTenant() }

// resolveTenantTable 解析 Statement 目标表名；解析不出返回空串。
// Schema 惰性解析：gorm:query 回调跑在 BuildQuerySQL 之前，Model 场景下
// Schema/Table 可能尚未填充，这里主动 Parse（schema 缓存幂等）。
func resolveTenantTable(stmt *gorm.Statement) string {
	if stmt == nil {
		return ""
	}
	if stmt.Table != "" {
		return stmt.Table
	}
	if stmt.Schema == nil && stmt.Model != nil {
		_ = stmt.Parse(stmt.Model)
	}
	if stmt.Schema != nil {
		return stmt.Schema.Table
	}
	return ""
}

// tenantGuardFail fail-closed：注入失败即报错，绝不放行为全量查询
func tenantGuardFail(db *gorm.DB, table, reason string) {
	_ = db.AddError(fmt.Errorf("[tenant] 表 %s 需要租户上下文但 ctx 未绑定（%s）；"+
		"多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户", table, reason))
}

// scopeQueryForTenant Query/Row 前置：自动注入 WHERE tenant_id = ?
func scopeQueryForTenant(db *gorm.DB) {
	if db == nil || db.Statement == nil || !tenantInjectionActive() {
		return
	}
	stmt := db.Statement
	// 预构建 SQL（Raw().Scan 形态）：不走子句注入，交由 raw 守卫裁决
	if stmt.SQL.Len() > 0 {
		guardRawSQLForTenant(db)
		return
	}
	table := resolveTenantTable(stmt)
	if !tenancyidx.IsTenantTable(table) {
		return
	}
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return
	}
	id, ok := tenant.FromContext(ctx)
	if !ok {
		tenantGuardFail(db, table, "query")
		return
	}
	stmt.AddClause(clauseWhereTenant(id))
}

// fillCreateForTenant Create 前置：自动填充 tenant_id 列
func fillCreateForTenant(db *gorm.DB) {
	if db == nil || db.Statement == nil || !tenantInjectionActive() {
		return
	}
	stmt := db.Statement
	if stmt.Schema == nil {
		return
	}
	field := stmt.Schema.LookUpField("tenant_id")
	if field == nil {
		return // 模型未声明 tenant_id：非租户表
	}
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return // 系统操作：交由列默认值（1）
	}
	id, ok := tenant.FromContext(ctx)
	if !ok {
		table := resolveTenantTable(stmt)
		// 字段已有显式非零值（行自证写入路径）：放行
		if v, zero := field.ValueOf(stmt.Context, stmt.ReflectValue); !zero {
			if fv, isUint := v.(uint); isUint && fv != 0 {
				return
			}
		}
		tenantGuardFail(db, table, "create")
		return
	}
	if v, zero := field.ValueOf(stmt.Context, stmt.ReflectValue); !zero {
		if fv, isUint := v.(uint); isUint && fv != 0 && fv != id {
			_ = db.AddError(fmt.Errorf("[tenant] 写入 tenant_id=%d 与上下文租户 %d 不一致，拒绝跨租户写", fv, id))
			return
		}
	}
	_ = field.Set(stmt.Context, stmt.ReflectValue, id)
}

// scopeUpdateForTenant / scopeDeleteForTenant：与 query 同一注入语义
func scopeUpdateForTenant(db *gorm.DB) { scopeLikeForTenant(db, "update") }
func scopeDeleteForTenant(db *gorm.DB) { scopeLikeForTenant(db, "delete") }

func scopeLikeForTenant(db *gorm.DB, op string) {
	if db == nil || db.Statement == nil || !tenantInjectionActive() {
		return
	}
	stmt := db.Statement
	if stmt.SQL.Len() > 0 {
		guardRawSQLForTenant(db)
		return
	}
	table := resolveTenantTable(stmt)
	if !tenancyidx.IsTenantTable(table) {
		return
	}
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return
	}
	id, ok := tenant.FromContext(ctx)
	if !ok {
		tenantGuardFail(db, table, op)
		return
	}
	stmt.AddClause(clauseWhereTenant(id))
}

// guardRawForTenant Row/Raw 前置：预构建 SQL 守卫
func guardRawForTenant(db *gorm.DB) {
	if db == nil || db.Statement == nil || !tenantInjectionActive() {
		return
	}
	guardRawSQLForTenant(db)
}

// guardRawSQLForTenant 预构建 SQL 守卫：on 模式引用租户表即拒绝。
// 数据访问唯一形态是 GORM API（check-gorm-only 门禁）；此处是运行时第二道闸。
func guardRawSQLForTenant(db *gorm.DB) {
	if db == nil || db.Statement == nil {
		return
	}
	sqlText := db.Statement.SQL.String()
	if sqlText == "" || tenant.IsSystemScope(db.Statement.Context) {
		return
	}
	for _, m := range tenantTableRefRe.FindAllStringSubmatch(sqlText, -1) {
		name := lastSegment(m[1])
		if tenancyidx.IsTenantTable(name) {
			_ = db.AddError(fmt.Errorf("[tenant] 预构建 SQL 引用租户表 %s 被拒绝：数据访问必须走 GORM API"+
				"（specs/001-saas-multi-tenant D2 / check-gorm-only 门禁）", name))
			return
		}
	}
}

// clauseWhereTenant 构造 `tenant_id = ?` WHERE 子句
func clauseWhereTenant(id uint) clause.Where {
	return clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: clause.Column{Name: "tenant_id"}, Value: id},
	}}
}

// lastSegment "schema.table" → "table"
func lastSegment(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return name
}
