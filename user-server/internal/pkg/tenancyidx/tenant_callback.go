package tenancyidx

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"

	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// 租户隔离 GORM callback（specs/001-saas-multi-tenant T005）
//
// 实现位于 tenancyidx（其依赖仅 gorm+tenant）：testutil 与 pkg/db 经本包注册，
// 断开 testutil→db 的 import 环（pkg/db 测试依赖 testutil 的链路会成环）。
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

// identPattern 匹配裸标识符、双引号标识符与反引号标识符（PG 仅双引号，兼容历史写法）
const identPattern = `(?:"[^"]+"|` + "`[^`]+`" + `|[A-Za-z_][A-Za-z0-9_$]*)`

// tableRefRe 从预构建 SQL / JOIN 片段中提取引用表（FROM/JOIN/INTO/UPDATE 等关键字后）
var tableRefRe = regexp.MustCompile(`(?i)\b(?:from|join|into|update|truncate(?:\s+table)?|alter\s+table|drop\s+table|copy)\s+(?:only\s+)?(?:if\s+exists\s+)?(` + identPattern + `(?:\s*\.\s*` + identPattern + `)?)`)

// tableExprRe 解析 GORM 的 TableExpr（如 `sop_executions AS se` / `"tenants"` / `public.users u`）
var tableExprRe = regexp.MustCompile(`(?i)^\s*(?:only\s+)?(` + identPattern + `(?:\s*\.\s*` + identPattern + `)?)(?:\s+(?:as\s+)?(` + identPattern + `))?\s*$`)

var aliasRe = regexp.MustCompile(`(?i)^\s*(?:as\s+)?(` + identPattern + `)`)

var joinOnRe = regexp.MustCompile(`(?i)\bON\b`)

// aliasKeyword 不能当作表别名的 SQL 关键字（FROM t WHERE / JOIN t ON ...）
var aliasKeyword = map[string]bool{
	"ON": true, "USING": true, "WHERE": true, "LEFT": true, "RIGHT": true,
	"INNER": true, "OUTER": true, "CROSS": true, "NATURAL": true, "JOIN": true,
	"SET": true, "VALUES": true, "RETURNING": true, "GROUP": true, "ORDER": true,
	"LIMIT": true, "OFFSET": true, "FOR": true, "WINDOW": true, "UNION": true,
	"EXCEPT": true, "INTERSECT": true, "FETCH": true, "TABLESAMPLE": true,
	"AS": true, "ONLY": true, "FROM": true, "INTO": true, "UPDATE": true,
	"TRUNCATE": true, "ALTER": true, "DROP": true, "COPY": true, "ADD": true,
	"COLUMN": true, "DEFAULT": true, "CHECK": true, "CONSTRAINT": true,
	"PRIMARY": true, "UNIQUE": true, "REFERENCES": true, "NOT": true,
	"NULL": true, "TRUE": true, "FALSE": true, "AND": true, "OR": true,
	"IS": true, "IN": true, "BETWEEN": true, "LIKE": true, "ILIKE": true,
	"SIMILAR": true, "DO": true, "NOTHING": true, "CONFLICT": true,
	"SELECT": true, "WITH": true, "OVER": true, "PARTITION": true,
	"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"ASC": true, "DESC": true, "NULLS": true, "FIRST": true, "LAST": true,
	"LATERAL": true, "QUALIFY": true, "EXISTS": true, "IF": true,
}

// tableRef 目标表引用：注册表按 table 判定，SQL 条件用 qualifier（别名优先）
type tableRef struct {
	table string
	alias string
}

func (r tableRef) qualifier() string {
	if r.alias != "" {
		return r.alias
	}
	return r.table
}

// normalizeIdent 归一化标识符：schema.table → table；去引号；裸标识符按 PG 语义转小写
func normalizeIdent(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.LastIndex(raw, "."); i >= 0 {
		raw = strings.TrimSpace(raw[i+1:])
	}
	if len(raw) >= 2 {
		switch {
		case raw[0] == '"' && raw[len(raw)-1] == '"':
			return strings.ReplaceAll(raw[1:len(raw)-1], `""`, `"`)
		case raw[0] == '`' && raw[len(raw)-1] == '`':
			return strings.ReplaceAll(raw[1:len(raw)-1], "``", "`")
		}
	}
	return strings.ToLower(raw)
}

// parseTableExpr 解析 TableExpr 原文，返回基础表名与别名。
// 解析不了（派生表 `(?) AS u`、多表 FROM）返回 ok=false——绝不把别名当表名。
func parseTableExpr(raw string) (tableRef, bool) {
	m := tableExprRe.FindStringSubmatch(raw)
	if m == nil {
		return tableRef{}, false
	}
	table := normalizeIdent(m[1])
	if table == "" {
		return tableRef{}, false
	}
	ref := tableRef{table: table}
	if m[2] != "" {
		if aliasKeyword[strings.ToUpper(m[2])] {
			return tableRef{}, false
		}
		ref.alias = normalizeIdent(m[2])
	}
	return ref, true
}

// scanTableRefs 提取 SQL 片段中的全部表引用（含别名）
func scanTableRefs(sqlText string) []tableRef {
	matches := tableRefRe.FindAllStringSubmatchIndex(sqlText, -1)
	out := make([]tableRef, 0, len(matches))
	for _, m := range matches {
		table := normalizeIdent(sqlText[m[2]:m[3]])
		if table == "" {
			continue
		}
		out = append(out, tableRef{table: table, alias: parseTableAlias(sqlText[m[1]:])})
	}
	return out
}

// parseTableAlias 从表名之后的剩余文本解析别名；关键字/标点/括号一律视为无别名
func parseTableAlias(rest string) string {
	s := strings.TrimSpace(rest)
	if s == "" || s[0] == '(' || s[0] == ',' || s[0] == ';' {
		return ""
	}
	m := aliasRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	if aliasKeyword[strings.ToUpper(m[1])] {
		return ""
	}
	return normalizeIdent(m[1])
}

// resolveTenantRef 解析 Statement 的目标表：显式 TableExpr 优先（其中 Table 字段是
// 别名本身，不能直接查登记表），其次 Schema/Table。解析不出返回 ok=false。
func resolveTenantRef(stmt *gorm.Statement) (tableRef, bool) {
	if stmt == nil {
		return tableRef{}, false
	}
	if stmt.TableExpr != nil {
		return parseTableExpr(stmt.TableExpr.SQL)
	}
	if stmt.Schema == nil && stmt.Model != nil {
		_ = stmt.Parse(stmt.Model)
	}
	if stmt.Table != "" {
		return tableRef{table: normalizeIdent(stmt.Table)}, true
	}
	if stmt.Schema != nil {
		return tableRef{table: normalizeIdent(stmt.Schema.Table)}, true
	}
	return tableRef{}, false
}

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
	// Scan()/Rows() 走 Row 链（callbacks.RowQuery→BuildQuerySQL），不是 Query 链：
	// 只在 Query 链注入会让所有 .Scan() 查询（大量 Table+Joins 仓储）整体绕过隔离。
	_ = cb.Row().Before("gorm:row").Register("tenant:scope_row", scopeQueryForTenant)
	_ = cb.Raw().Before("gorm:raw").Register("tenant:guard_raw", guardRawForTenant)
}

// tenantInjectionActive 是否处于注入生效状态（on 模式）。off 一律直通。
func tenantInjectionActive() bool { return tenant.IsMultiTenant() }

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
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return
	}
	if !ensureRegistryForTenant(db, "query") {
		return
	}
	ref, known := resolveTenantRef(stmt)
	id, bound := tenant.FromContext(ctx)
	if known && IsTenantTable(ref.table) {
		if !bound {
			tenantGuardFail(db, ref.table, "query")
			return
		}
		stmt.AddClause(clauseWhereTenant(ref, id))
	}
	if err := scopeTenantJoins(stmt, bound, id); err != nil {
		_ = db.AddError(err)
	}
}

// fillCreateForTenant Create 前置：自动填充 tenant_id 列
func fillCreateForTenant(db *gorm.DB) {
	if db == nil || db.Statement == nil || !tenantInjectionActive() {
		return
	}
	stmt := db.Statement
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return // 系统操作：交由列默认值（1）
	}
	if !ensureRegistryForTenant(db, "create") {
		return
	}
	ref, known := resolveTenantRef(stmt)
	id, bound := tenant.FromContext(ctx)
	table := ref.table
	isTenant := known && IsTenantTable(table)
	if isTenant {
		if err := guardOnConflictTenantAssignments(stmt, id, bound); err != nil {
			_ = db.AddError(err)
			return
		}
	}

	// schema-less（map）Create：没有 Model 字段可填，INSERT 也不会带 tenant_id。
	// 旧实现直接 return ⇒ 行落列默认值 1（非默认租户的数据整批串进租户 1）。
	if stmt.Schema == nil {
		if !isTenant {
			return
		}
		fillMapCreateForTenant(db, stmt, table, id, bound)
		return
	}
	field := stmt.Schema.LookUpField("tenant_id")
	if field == nil {
		// 目标是租户表但模型没声明 tenant_id：INSERT 会静默落列默认值 1，必须拒绝
		if isTenant {
			tenantGuardFail(db, table, "create：模型未声明 tenant_id 字段，无法填充")
		}
		return
	}

	// 批量 Create(&[]*Model / []Model)：stmt.ReflectValue 是**切片**。
	// 曾经对整片直接 ValueOf/Set —— reflect 在切片上调 Field 直接 panic
	//（on 模式 + 有租户绑定 ctx + 批量写租户表 = 进程级崩溃；单行路径不受
	// 影响，所以长期没暴露）。逐元素执行与单行完全相同的填充/校验语义。
	rv := stmt.ReflectValue
	switch rv.Kind() {
	case reflect.Map:
		// Model 与 map 混用（Schema 非空、Dest 是 map）：同样走 map 填充
		if isTenant {
			fillMapCreateForTenant(db, stmt, table, id, bound)
		}
		return
	case reflect.Slice:
		if rv.Len() > 0 && derefKind(rv.Index(0)) == reflect.Map {
			if isTenant {
				fillMapCreateForTenant(db, stmt, table, id, bound)
			}
			return
		}
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Interface {
				if elem.IsNil() {
					break
				}
				elem = elem.Elem()
			}
			if elem.Kind() != reflect.Struct {
				continue
			}
			if !fillOneForTenant(db, field, elem, id, bound, table) {
				return // 已 AddError：停止处理剩余行（与单行 fail-closed 同后果）
			}
		}
		return
	case reflect.Struct:
		fillOneForTenant(db, field, rv, id, bound, table)
	default:
		// 目标形态无法填充（非结构体/切片/map）：绝不能静默落列默认值
		if isTenant {
			tenantGuardFail(db, table, "create：目标行形态不支持 tenant_id 填充")
		}
	}
}

func derefKind(v reflect.Value) reflect.Kind {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return v.Kind()
		}
		v = v.Elem()
	}
	if !v.IsValid() {
		return reflect.Invalid
	}
	return v.Kind()
}

// fillMapCreateForTenant 为 map[string]interface{}（或切片）逐行填充/校验 tenant_id。
// 直接改写 map 本身：GORM ConvertToCreateValues 读的是同一份 Dest。
func fillMapCreateForTenant(db *gorm.DB, stmt *gorm.Statement, table string, id uint, bound bool) {
	rv := stmt.ReflectValue
	switch rv.Kind() {
	case reflect.Map:
		if !fillOneMapForTenant(db, rv, table, id, bound) {
			return
		}
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			for elem.Kind() == reflect.Pointer || elem.Kind() == reflect.Interface {
				if elem.IsNil() {
					tenantGuardFail(db, table, "create：map 批量行含 nil")
					return
				}
				elem = elem.Elem()
			}
			if elem.Kind() != reflect.Map {
				tenantGuardFail(db, table, "create：批量行不是 map")
				return
			}
			if !fillOneMapForTenant(db, elem, table, id, bound) {
				return
			}
		}
	default:
		tenantGuardFail(db, table, "create：map 目标形态不支持 tenant_id 填充")
	}
}

// fillOneMapForTenant 单行 map 填充：bound 时强制==上下文租户，未绑定仅接受行自证。
func fillOneMapForTenant(db *gorm.DB, m reflect.Value, table string, id uint, bound bool) bool {
	if m.Type().Key().Kind() != reflect.String {
		tenantGuardFail(db, table, "create：map 键类型不是字符串")
		return false
	}
	if m.IsNil() {
		tenantGuardFail(db, table, "create：map 为 nil")
		return false
	}
	key := reflect.ValueOf("tenant_id").Convert(m.Type().Key())
	existing := m.MapIndex(key)
	if existing.IsValid() && !isNilableNil(existing) {
		value := existing.Interface()
		if bound {
			if n, ok := asUint(value); !ok || (n != 0 && n != id) {
				_ = db.AddError(fmt.Errorf("[tenant] 写入 tenant_id=%v 与上下文租户 %d 不一致，拒绝跨租户写", value, id))
				return false
			}
		} else if n, ok := asUint(value); !ok || n == 0 {
			tenantGuardFail(db, table, "create：无租户上下文且 tenant_id 不是有效自证值")
			return false
		}
	} else if !bound {
		tenantGuardFail(db, table, "create：无租户上下文且 map 未携带 tenant_id")
		return false
	}
	elemType := m.Type().Elem()
	tenantVal := reflect.ValueOf(id)
	if !tenantVal.Type().AssignableTo(elemType) {
		if !tenantVal.Type().ConvertibleTo(elemType) {
			tenantGuardFail(db, table, "create：map 值类型无法承载 tenant_id")
			return false
		}
		tenantVal = tenantVal.Convert(elemType)
	}
	m.SetMapIndex(key, tenantVal)
	return true
}

func isNilableNil(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// fillOneForTenant 对**一个结构体行**执行 tenant_id 填充/校验。
// 返回 true=可继续处理后续行；false=已 fail-closed（db 已挂错误）。
func fillOneForTenant(db *gorm.DB, field *schema.Field, v reflect.Value, id uint, bound bool, table string) bool {
	if !bound {
		// 字段已有显式非零值（行自证写入路径）：放行
		if fv, zero := field.ValueOf(db.Statement.Context, v); !zero {
			if fvUint, isUint := fv.(uint); isUint && fvUint != 0 {
				return true
			}
		}
		tenantGuardFail(db, table, "create")
		return false
	}
	if fv, zero := field.ValueOf(db.Statement.Context, v); !zero {
		if fvUint, isUint := fv.(uint); isUint && fvUint != 0 && fvUint != id {
			_ = db.AddError(fmt.Errorf("[tenant] 写入 tenant_id=%d 与上下文租户 %d 不一致，拒绝跨租户写", fvUint, id))
			return false
		}
	}
	if !v.CanAddr() {
		// 不可寻址就填不进去：宁可 fail-closed 报错，也不能静默落列默认值 1
		//（那会把行写到默认租户 = 串租户）。单行 Create 的 Dest 恒可寻址。
		tenantGuardFail(db, table, "create：目标行不可寻址，无法填充 tenant_id")
		return false
	}
	_ = field.Set(db.Statement.Context, v, id)
	return true
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
	ctx := stmt.Context
	if tenant.IsSystemScope(ctx) {
		return
	}
	if !ensureRegistryForTenant(db, op) {
		return
	}
	ref, known := resolveTenantRef(stmt)
	if !known || !IsTenantTable(ref.table) {
		return
	}
	id, bound := tenant.FromContext(ctx)
	if !bound {
		tenantGuardFail(db, ref.table, op)
		return
	}
	if op == "update" {
		if err := guardUpdateTenantAssignments(stmt, id); err != nil {
			_ = db.AddError(err)
			return
		}
	}
	stmt.AddClause(clauseWhereTenant(ref, id))
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
	if !ensureRegistryForTenant(db, "raw") {
		return
	}
	for _, ref := range scanTableRefs(sqlText) {
		if IsTenantTable(ref.table) {
			_ = db.AddError(fmt.Errorf("[tenant] 预构建 SQL 引用租户表 %s 被拒绝：数据访问必须走 GORM API"+
				"（specs/001-saas-multi-tenant D2 / check-gorm-only 门禁）", ref.table))
			return
		}
	}
}

// ensureRegistryForTenant 登记表未装配时 on 模式拒绝一切非系统操作：
// nil 登记表会把任何表都判成"非租户表" = 全量放行。
func ensureRegistryForTenant(db *gorm.DB, op string) bool {
	if RegistryReady() {
		return true
	}
	_ = db.AddError(fmt.Errorf("[tenant] 租户登记表未初始化，无法判定表归属（%s）；"+
		"多租户模式拒绝执行，请在装配期先调用 InitTenantRegistry", op))
	return false
}

// clauseWhereTenant 构造 `tenant_id = ?` WHERE 子句；
// 有别名时限定别名，避免 JOIN 场景下 tenant_id 列歧义。
func clauseWhereTenant(ref tableRef, id uint) clause.Where {
	col := clause.Column{Name: "tenant_id"}
	if q := ref.qualifier(); q != "" {
		col.Table = q
	}
	return clause.Where{Exprs: []clause.Expression{
		clause.Eq{Column: col, Value: id},
	}}
}

// scopeTenantJoins 给 JOIN 到的租户表追加租户条件。
// 条件必须进 ON 子句（而不是 WHERE）：LEFT JOIN 的未匹配父行必须保留。
func scopeTenantJoins(stmt *gorm.Statement, bound bool, id uint) error {
	for i := range stmt.Joins {
		j := &stmt.Joins[i]
		if !strings.ContainsAny(j.Name, " \t\r\n") {
			// 关联式 Joins("Relation")：无法可靠推导 SQL 别名，目标为租户表即拒绝
			if stmt.Schema != nil {
				if target, ok := associationTable(stmt.Schema, j.Name); ok && IsTenantTable(target) {
					return fmt.Errorf("[tenant] 关联式 JOIN %s 指向租户表 %s，无法自动附加租户条件；"+
						"请改用带 ON 的显式 JOIN", j.Name, target)
				}
			}
			continue
		}
		var tenantRefs []tableRef
		for _, ref := range scanTableRefs(j.Name) {
			if IsTenantTable(ref.table) {
				tenantRefs = append(tenantRefs, ref)
			}
		}
		if len(tenantRefs) == 0 {
			continue
		}
		if !bound {
			return fmt.Errorf("[tenant] JOIN 引用租户表 %s 但 ctx 未绑定租户；多租户模式拒绝执行", tenantRefs[0].table)
		}
		if !joinOnRe.MatchString(j.Name) {
			return fmt.Errorf("[tenant] JOIN 租户表 %s 未使用 ON 子句，无法附加租户条件，拒绝执行", tenantRefs[0].table)
		}
		for _, ref := range tenantRefs {
			q := ref.qualifier()
			if q == "" {
				return fmt.Errorf("[tenant] JOIN 租户表 %s 无法解析别名", ref.table)
			}
			j.Name += fmt.Sprintf(` AND %s.%s = ?`, quoteIdent(q), quoteIdent("tenant_id"))
			j.Conds = append(j.Conds, id)
		}
	}
	return nil
}

// associationTable 解析 Joins("A.B") 关系链的最终表名
func associationTable(s *schema.Schema, name string) (string, bool) {
	cur := s
	for _, part := range strings.Split(name, ".") {
		rel, ok := cur.Relationships.Relations[part]
		if !ok {
			return "", false
		}
		cur = rel.FieldSchema
	}
	return cur.Table, true
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// guardUpdateTenantAssignments 更新不得改写 tenant_id：
// Update 列式 / Updates map / Updates struct / Clauses(Assignments) 四种形态全覆盖。
func guardUpdateTenantAssignments(stmt *gorm.Statement, id uint) error {
	reject := func(v interface{}) error {
		if n, ok := asUint(v); ok && n == id {
			return nil
		}
		return fmt.Errorf("[tenant] 更新不可改写 tenant_id（上下文租户 %d，SET 值 %v）", id, v)
	}
	if stmt.Schema != nil && stmt.Dest != nil {
		rv := reflect.ValueOf(stmt.Dest)
		for rv.Kind() == reflect.Pointer {
			if rv.IsNil() {
				rv = reflect.Value{}
				break
			}
			rv = rv.Elem()
		}
		if rv.IsValid() && rv.Kind() == reflect.Struct {
			if field := stmt.Schema.LookUpField("tenant_id"); field != nil {
				if v, zero := field.ValueOf(stmt.Context, rv); !zero {
					if err := reject(v); err != nil {
						return err
					}
				}
			}
		}
	}
	if dest, ok := stmt.Dest.(map[string]interface{}); ok {
		for key, v := range dest {
			if isTenantColumnName(key) {
				if err := reject(v); err != nil {
					return err
				}
			}
		}
	}
	if c, ok := stmt.Clauses["SET"]; ok {
		if set, ok := c.Expression.(clause.Set); ok {
			for _, a := range set {
				if isTenantColumnName(a.Column.Name) {
					if err := reject(a.Value); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// guardOnConflictTenantAssignments upsert 的 DO UPDATE 不得把 tenant_id 改成别的值。
// excluded.tenant_id 是本次插入值（已填充为上下文租户），放行。
func guardOnConflictTenantAssignments(stmt *gorm.Statement, id uint, bound bool) error {
	c, ok := stmt.Clauses["ON CONFLICT"]
	if !ok {
		return nil
	}
	oc, ok := c.Expression.(clause.OnConflict)
	if !ok {
		return nil
	}
	for _, a := range oc.DoUpdates {
		if !isTenantColumnName(a.Column.Name) {
			continue
		}
		if col, ok := a.Value.(clause.Column); ok && strings.EqualFold(col.Table, "excluded") && isTenantColumnName(col.Name) {
			continue
		}
		if n, ok := asUint(a.Value); bound && ok && n == id {
			continue
		}
		return fmt.Errorf("[tenant] upsert 的 ON CONFLICT DO UPDATE 不可改写 tenant_id（上下文租户 %d）", id)
	}
	return nil
}

func isTenantColumnName(name string) bool {
	return strings.EqualFold(normalizeIdent(name), "tenant_id")
}

// asUint 把常见数值形态归一为 uint；表达式/字符串等一律 false（fail-closed）
func asUint(v interface{}) (uint, bool) {
	switch n := v.(type) {
	case uint:
		return n, true
	case uint8:
		return uint(n), true
	case uint16:
		return uint(n), true
	case uint32:
		return uint(n), true
	case uint64:
		return uint(n), true
	case int:
		if n >= 0 {
			return uint(n), true
		}
	case int8:
		if n >= 0 {
			return uint(n), true
		}
	case int16:
		if n >= 0 {
			return uint(n), true
		}
	case int32:
		if n >= 0 {
			return uint(n), true
		}
	case int64:
		if n >= 0 {
			return uint(n), true
		}
	case float32:
		return floatToUint(float64(n))
	case float64:
		return floatToUint(n)
	}
	return 0, false
}

func floatToUint(f float64) (uint, bool) {
	if f < 0 || f != math.Trunc(f) || f > math.MaxUint32 {
		return 0, false
	}
	return uint(f), true
}
