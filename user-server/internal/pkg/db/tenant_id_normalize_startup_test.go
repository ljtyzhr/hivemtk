// tenant_id_normalize_startup_test.go 钉住 tenant_id 列类型归一化的两条边界。
//
// 为什么要有这一道：`normalizeTenantIDColumns` 是 AutoMigrate **之前**跑的库级修复，
// 它把 `information_schema` 读到的表名拼进 `ALTER TABLE <name>`。两条不变量此前
// 没有任何用例守着：
//
//	① 漂移列真的被修好（varchar → bigint、默认 1、NOT NULL）——否则按 tenant_id 过滤的
//	   查询仍会撞 `operator does not exist: character varying = integer`；
//	② 不合标识符规范的表名不许进 DDL——名字虽来自库元数据，但漂移库里存在何种表名不由
//	   我们决定，一条拼坏的 `ALTER TABLE` 是拿整个启动流程去换。
//
// ②臂同时是①臂的反向对账：两张表贴着一起建，只有规范那张被改，说明跳过是守卫开火，
// 而不是整条路径根本没跑（后者会让这条用例永远绿）。
package db

import (
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/testutil"
)

// 夹具表名都是本文件的编译期常量，不接受外部输入；带空格那张是故意造给守卫拦的。
const (
	driftGoodTable = "zz_tenant_drift_ok"
	driftBadTable  = "zz Tenant Drift Bad"
)

func tenantIDColumnType(t *testing.T, database *gorm.DB, table string) string {
	t.Helper()
	var dataType string
	if err := database.Raw(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = 'tenant_id'`,
		table).Scan(&dataType).Error; err != nil {
		t.Fatalf("读 %s.tenant_id 类型失败: %v", table, err)
	}
	return dataType
}

func TestNormalizeTenantIDColumnsFixesConformingAndSkipsOthers(t *testing.T) {
	database := testutil.NewTestDB(t)
	if database == nil {
		t.Fatal("测试库不可达")
	}

	stmts := []string{
		`DROP TABLE IF EXISTS ` + driftGoodTable,
		`DROP TABLE IF EXISTS "zz Tenant Drift Bad"`,
		`CREATE TABLE ` + driftGoodTable + ` (id serial primary key, tenant_id varchar(64) DEFAULT '')`,
		`CREATE TABLE "zz Tenant Drift Bad" (id serial primary key, tenant_id varchar(64) DEFAULT '')`,
	}
	for _, q := range stmts {
		if err := database.Exec(q).Error; err != nil {
			t.Fatalf("建夹具失败 %q: %v", q, err)
		}
	}
	t.Cleanup(func() {
		database.Exec(`DROP TABLE IF EXISTS ` + driftGoodTable)
		database.Exec(`DROP TABLE IF EXISTS "zz Tenant Drift Bad"`)
	})

	normalizeTenantIDColumns(database)

	if got := tenantIDColumnType(t, database, driftGoodTable); got != "bigint" {
		t.Errorf("%s.tenant_id 归一化后类型 = %q, 期望 bigint（①臂：规范表必须真的被修好）", driftGoodTable, got)
	}
	if got := tenantIDColumnType(t, database, driftBadTable); got != "character varying" {
		t.Errorf("%q.tenant_id 类型 = %q, 期望仍是 character varying（②臂：守卫应跳过不合规表名）", driftBadTable, got)
	}
}

func TestNormalizeTenantIDColumnsKeepsSemanticTenantIDs(t *testing.T) {
	database := testutil.NewTestDB(t)
	if database == nil {
		t.Fatal("测试库不可达")
	}
	const tbl = "zz_tenant_drift_named"
	if err := database.Exec(`DROP TABLE IF EXISTS ` + tbl).Error; err != nil {
		t.Fatalf("清夹具失败: %v", err)
	}
	if err := database.Exec(`CREATE TABLE ` + tbl + ` (id serial primary key, tenant_id varchar(64))`).Error; err != nil {
		t.Fatalf("建夹具失败: %v", err)
	}
	if err := database.Exec(`INSERT INTO ` + tbl + ` (tenant_id) VALUES ('acme')`).Error; err != nil {
		t.Fatalf("播种失败: %v", err)
	}
	t.Cleanup(func() { database.Exec(`DROP TABLE IF EXISTS ` + tbl) })

	normalizeTenantIDColumns(database)

	if got := tenantIDColumnType(t, database, tbl); got != "character varying" {
		t.Errorf("%s.tenant_id 类型 = %q, 期望保持 character varying —— 列里存了语义化租户标识时强转会把表打坏", tbl, got)
	}
}
