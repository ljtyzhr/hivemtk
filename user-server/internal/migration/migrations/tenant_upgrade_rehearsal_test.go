package migrations

// T019 / SaaS 审计 A17：含**存量数据**的升级演练（真库，不是"文档说验过"）。
//
// 演练的对象是 `TenantColumnBatchMigration`（v3.58.0，业务表批量补 tenant_id）—— 它是一整套
// SaaS 迁移里最不可逆的一步：一旦补错列/回填错值，线上表现是"某些租户看不见自己的数据"或
// "看见别人的数据"，而且**没有任何报错**。所以这里不测"我执行了 DDL"，而是四件事：
//
//	1. 升级后**真库内省**列形态（bigint / NOT NULL / DEFAULT 1）；
//	2. 存量数据归默认租户 1，且**内容逐字未变**（行数相同不算，内容也要相同）；
//	3. 二次执行幂等，且**不许把已有归属改回去** —— 这是最容易写错的一条：
//	   `ADD COLUMN ... DEFAULT 1` 对已存在列是 no-op，可有人"热心地"补一句
//	   `UPDATE ... SET tenant_id = 1` 做"回填"，那会把 tenant_id=2 的行静默搬到 1。
//	   正对照就是那条 tenant_id=2 的行：它必须**永远是 2**。
//	4. 回滚（Down）不销毁在用对象：列与数据都还在，且回滚后能再升级（可重复演练）。
//
// 为什么这几条非得在真库上跑：`DEFAULT 1` 是否作用到**存量行**、`ADD COLUMN IF NOT EXISTS`
// 是否真是 no-op，都是 PostgreSQL 的语义，不是 Go 的语义。用 mock 断言"我发了这条 SQL"只能
// 证明代码里写着这句话，证明不了库会照做。

import (
	"context"
	"testing"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	rehearsalLegacyTable = "a17_rehearsal_legacy"
	rehearsalEmptyTable  = "a17_rehearsal_empty"
	// 已迁移形态：有 tenant_id。用来钉"清单不许重复包含已加过列的表"，
	// 而且它必须是一张**静态**夹具 —— 若改用"先跑一遍 Up 再看清单"，这条断言就会
	// 顺手把整个测试库的表都 ALTER 一遍，污染同一槽位上的其它用例。
	rehearsalMigratedTable = "a17_rehearsal_migrated"
)

// rehearsalFixture 造一张"升级前形态"的表：**没有 tenant_id 列**，但已有存量行。
//
// 另造一张空表：两张表一起演，才能区分"存量行被正确归属"与"这张表本来就没数据"——
// 只演有数据的那张，一个把空表漏掉的实现也能全绿。
func rehearsalFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDB(t)
	if db == nil {
		// 与 v3_79_0 同口径：不 Skip。跳过等于没锁，而"没锁"与"锁对了"在 CI 上长得一样。
		t.Fatal("测试库不可达：存量数据升级演练无法判定")
	}
	t.Cleanup(func() {
		for _, tb := range []string{rehearsalLegacyTable, rehearsalEmptyTable, rehearsalMigratedTable} {
			_ = db.Exec(`DROP TABLE IF EXISTS ` + tb).Error
		}
	})
	stmts := []string{
		`DROP TABLE IF EXISTS ` + rehearsalLegacyTable,
		`DROP TABLE IF EXISTS ` + rehearsalEmptyTable,
		`DROP TABLE IF EXISTS ` + rehearsalMigratedTable,
		// 升级前形态：无 tenant_id（这正是 SaaS 改造前的库）
		`CREATE TABLE ` + rehearsalLegacyTable + ` (
			id bigserial PRIMARY KEY,
			payload text NOT NULL
		)`,
		`INSERT INTO ` + rehearsalLegacyTable + ` (payload) VALUES ('alpha'), ('beta'), ('gamma')`,
		`CREATE TABLE ` + rehearsalEmptyTable + ` (id bigserial PRIMARY KEY)`,
		// 已迁移形态（SaaS 改造后新建的表长这样）
		`CREATE TABLE ` + rehearsalMigratedTable + ` (
			id bigserial PRIMARY KEY,
			tenant_id bigint NOT NULL DEFAULT 1
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("构造演练夹具失败: %v\nSQL: %s", err, s)
		}
	}
	return db
}

// tenantColumnShape 真库内省一列的形状（"我加了列"不算数）。
func tenantColumnShape(t *testing.T, db *gorm.DB, table, column string) (dataType, nullable, def string, ok bool) {
	t.Helper()
	row := db.Raw(`
		SELECT data_type, is_nullable, COALESCE(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		table, column).Row()
	if err := row.Scan(&dataType, &nullable, &def); err != nil {
		return "", "", "", false
	}
	return dataType, nullable, def, true
}

func countRows(t *testing.T, db *gorm.DB, table string, where string, args ...any) int {
	t.Helper()
	q := `SELECT count(*) FROM ` + table
	if where != "" {
		q += " WHERE " + where
	}
	var n int
	if err := db.Raw(q, args...).Row().Scan(&n); err != nil {
		t.Fatalf("统计 %s 失败: %v", table, err)
	}
	return n
}

// TestTenantUpgradeRehearsalOnExistingData 含存量数据的升级演练（T019 / A17）。
func TestTenantUpgradeRehearsalOnExistingData(t *testing.T) {
	db := rehearsalFixture(t)
	ctx := context.Background()
	m := NewTenantColumnBatchMigration(db).withTables(rehearsalLegacyTable, rehearsalEmptyTable)

	// ---- 第 1 趟：升级 ----
	if err := m.Up(ctx); err != nil {
		t.Fatalf("升级（第 1 趟 Up）失败: %v", err)
	}

	// ① 列形态：真库内省，不是"我发了 ALTER TABLE"
	for _, table := range []string{rehearsalLegacyTable, rehearsalEmptyTable} {
		dataType, nullable, def, ok := tenantColumnShape(t, db, table, "tenant_id")
		if !ok {
			t.Fatalf("%s 升级后没有 tenant_id 列", table)
		}
		if dataType != "bigint" {
			t.Errorf("%s.tenant_id 类型应为 bigint，实际 %s（改类型会让既有行需要重写，回滚无从谈起）",
				table, dataType)
		}
		if nullable != "NO" {
			t.Errorf("%s.tenant_id 必须 NOT NULL，实际 is_nullable=%s —— "+
				"可空等于允许「无主流失」的行存在，且查询侧白名单无从判定", table, nullable)
		}
		if def != "1" {
			t.Errorf("%s.tenant_id 的 DEFAULT 必须是 1（存量行靠它归默认租户），实际 %q",
				table, def)
		}
	}

	// ② 存量数据：行数不变、全部归默认租户、内容逐字未变
	if n := countRows(t, db, rehearsalLegacyTable, ""); n != 3 {
		t.Fatalf("存量行数应仍为 3，实际 %d —— 补列迁移不许动数据", n)
	}
	if n := countRows(t, db, rehearsalLegacyTable, "tenant_id = 1"); n != 3 {
		t.Fatalf("存量行应全部归默认租户 1，实际只有 %d/3", n)
	}
	var payloads []string
	rows, err := db.Raw(`SELECT payload FROM ` + rehearsalLegacyTable + ` ORDER BY id`).Rows()
	if err != nil {
		t.Fatalf("读存量行失败: %v", err)
	}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("扫描存量行失败: %v", err)
		}
		payloads = append(payloads, p)
	}
	_ = rows.Close()
	if got := len(payloads); got != 3 || payloads[0] != "alpha" || payloads[1] != "beta" || payloads[2] != "gamma" {
		t.Fatalf("存量行内容被改了（这是数据破坏，不是迁移）：%v", payloads)
	}

	// ③ SaaS 之后的新数据：显式带另一个租户，用来做下面那条正对照
	if err := db.Exec(`INSERT INTO ` + rehearsalLegacyTable + ` (payload, tenant_id) VALUES ('delta', 2)`).Error; err != nil {
		t.Fatalf("插入 tenant 2 的新行失败: %v", err)
	}

	// ---- 第 2 趟：重入（运维重跑、或迁移被中断后续跑）----
	if err := m.Up(ctx); err != nil {
		t.Fatalf("重入（第 2 趟 Up）失败: %v", err)
	}

	// ④ 正对照：已有归属**不许**被"回填"改回去。
	// 少了这一条，一个加了 `UPDATE t SET tenant_id = 1` 的实现也能让 ② 全绿 ——
	// 而那种实现会把租户 2 的数据静默搬到租户 1，是两个失效里更严重的一个。
	if n := countRows(t, db, rehearsalLegacyTable, "tenant_id = 2"); n != 1 {
		t.Fatalf("重入把 tenant_id=2 的行搬走了 —— 迁移里出现了\"回填归 1\"这种数据破坏写法")
	}
	if n := countRows(t, db, rehearsalLegacyTable, ""); n != 4 {
		t.Fatalf("重入后行数应仍为 4，实际 %d", n)
	}

	// ---- 回滚 ----
	// ⑤ Down 是显式的"不销毁在用对象"，不是"没写"：tenant_id 列承载归属数据，
	// 回滚删列 = 直接毁掉归属。判据只看结果（列与数据都还在），不看它怎么实现的。
	if err := m.Down(ctx); err != nil {
		t.Fatalf("回滚（Down）返回错误: %v", err)
	}
	if _, _, _, ok := tenantColumnShape(t, db, rehearsalLegacyTable, "tenant_id"); !ok {
		t.Fatal("回滚把 tenant_id 列删掉了 —— 归属数据随之丢失，且这个动作不可逆（老版本代码读不懂，新数据已按租户写入）")
	}
	if n := countRows(t, db, rehearsalLegacyTable, ""); n != 4 {
		t.Fatalf("回滚后行数应仍为 4，实际 %d —— 回滚不许销毁在用数据", n)
	}

	// ---- 回滚后再升级：演练必须可重复 ----
	if err := m.Up(ctx); err != nil {
		t.Fatalf("回滚后再升级失败: %v", err)
	}
	if n := countRows(t, db, rehearsalLegacyTable, "tenant_id = 1"); n != 3 {
		t.Errorf("回滚后再升级，默认租户行数应仍为 3，实际 %d", n)
	}
	if n := countRows(t, db, rehearsalLegacyTable, "tenant_id = 2"); n != 1 {
		t.Errorf("回滚后再升级，租户 2 的行应仍为 1，实际 %d", n)
	}
}

// TestTenantColumnBatchEnumeratesOnlyMigratable 演练的边界：清单只含"该加列的租户表"。
//
// 为什么单独一条：上面那趟演练只演了"该加列的表"，没演"不该加列的表"，于是
// `businessTables()` 的两条过滤（豁免清单 / 已有 tenant_id 列）失效也照样全绿。
// 误加一列到全局表（`tenants` / `migration_records` / `cron_job_leases`…）的后果与漏加相反：
// 多一列没人读，看起来无害，但它会让"豁免清单"这个唯一事实源悄悄失效 ——
// 下次真需要回答"哪些表是租户表"时就没有权威答案了。
//
// 这条**走生产路径**（不过滤入参覆盖），因此它也是"清单确实来自真库内省"的判据：
// 探针表是临时建的、测试库里有几十张真实表，清单必须自己从 information_schema 里挑。
//
// 第三条腿（头脑风暴补）：**形态错的列**。原口径只看"有没有 tenant_id 列"，一张
// `tenant_id varchar(20) NULL` 的表会被静默跳过，而 `ADD COLUMN IF NOT EXISTS` 对
// 已存在列是 no-op —— 缺口修不回来、永远不报错。收紧后的口径是"与本迁移产出完全一致"
// （bigint / NOT NULL / DEFAULT 1），形态错的表**进清单并让 Up 响亮失败**（DDL 报错
// ⇒ 升级中断 ⇒ 有人看见），而不是静默通过把缺口埋进土里。
func TestTenantColumnBatchEnumeratesOnlyMigratable(t *testing.T) {
	db := rehearsalFixture(t)
	m := NewTenantColumnBatchMigration(db)

	contains := func(tables []string, want string) bool {
		for _, x := range tables {
			if x == want {
				return true
			}
		}
		return false
	}

	tables, err := m.businessTables()
	if err != nil {
		t.Fatalf("枚举业务表失败: %v", err)
	}
	if !contains(tables, rehearsalLegacyTable) {
		t.Fatalf("升级前形态的表（无 tenant_id）必须进清单，实际清单里没有它（%d 张）", len(tables))
	}

	// 形态错（列在、但类型/可空/默认值与本迁移产出不一致）必须进清单。
	// 变异实测：原口径（只看列名）会让它静默消失，于是 varchar 的 tenant_id
	// 成为一个永远不报错的隔离缺口。
	t.Cleanup(func() { _ = db.Exec(`DROP TABLE IF EXISTS a17_rehearsal_wrongshape`).Error })
	if err := db.Exec(`CREATE TABLE a17_rehearsal_wrongshape (id bigserial PRIMARY KEY, tenant_id varchar(20))`).Error; err != nil {
		t.Fatalf("造形态错探针表失败: %v", err)
	}
	wrongShape, err := m.businessTables()
	if err != nil {
		t.Fatalf("枚举业务表失败: %v", err)
	}
	if !contains(wrongShape, "a17_rehearsal_wrongshape") {
		t.Fatal("tenant_id 列形态错（varchar）的表不在清单里 —— 它会被静默跳过，" +
			"而 ADD COLUMN IF NOT EXISTS 修不回来：这是一个永远不报错的隔离缺口")
	}
	// 进了清单就必须让 Up 响亮失败（这是"被看见"的唯一途径，比静默通过安全）。
	// 注意 PostgreSQL 的真实行为（15.18 实测）：`ADD COLUMN IF NOT EXISTS` 对**已存在**
	// 列是纯 no-op，连类型不匹配都不报错 —— 所以"进清单"本身修不了缺口，Up 必须
	// 带**后复核**：清单跑完仍有形态错列即报错中断。下面这趟 Up 的判据就来自它。
	if err := NewTenantColumnBatchMigration(db).withTables("a17_rehearsal_wrongshape").Up(context.Background()); err == nil {
		t.Fatal("形态错的表 Up 竟然成功 —— PostgreSQL 的 IF NOT EXISTS 对已存在列是纯 no-op（15.18 实测），" +
			"缺口没修还被盖成绿灯，这与静默跳过同罪")
	}

	// 豁免表不得进清单。**这条必须自己造**：现成的豁免表（`migration_records` /
	// `cron_job_leases` / `backups`）在测试库里时有时无（新槽位库只有几张表），
	// 靠"库里有 tenants 所以它没被扫到"来断言，等于赌夹具形态 —— 而赌输时这条判定
	// 会静默变成空断言（"没扫到"与"被过滤掉"长得一模一样）。
	// 所以临时把探针表登记进豁免清单：这样"清单里没有它"就只可能是过滤生效。
	// 包级 var 的临时改写：本包无用例 t.Parallel（现测 0 处），改完立刻还原。
	tenantExemptTables[rehearsalLegacyTable] = true
	exempted, err := m.businessTables()
	delete(tenantExemptTables, rehearsalLegacyTable)
	if err != nil {
		t.Fatalf("枚举业务表失败: %v", err)
	}
	if contains(exempted, rehearsalLegacyTable) {
		t.Error("登记进 tenantExemptTables 的表仍出现在加列清单里 —— 豁免清单失效等于「唯一事实源」" +
			"失效（误加列到全局表的后果是：判「哪些表是租户表」时没有权威答案）")
	}

	// 正对照：**已经加过 tenant_id 的表不许再进清单**（NOT EXISTS 过滤生效）。
	// 用静态夹具而不是"先跑一趟再查清单"：后者会顺手 ALTER 掉测试库里所有缺列的表，
	// 污染同一槽位上的其它用例；而这里要判的语义用一张现成的"已迁移形态"表就能判。
	if contains(tables, rehearsalMigratedTable) {
		t.Fatal("已经有 tenant_id 的表仍留在清单里 —— 重入时会重复执行 DDL" +
			"（`IF NOT EXISTS` 让它在库上变成 no-op，把清单语义错静默盖住了）")
	}
}

// TestTenantColumnBatchRegistration 生产注册点不得携带覆盖清单（攻击 G）。
//
// `withTables` 是"仅测试用"的约定，不是语言保证：生产装配点在 initial_schema.go 的
// `register(NewTenantColumnBatchMigration(db))` —— 那里若有人链上 `.withTables("orders")`，
// "真库内省"就静默退化成"只处理指定表"，而这恰恰是"仅测试用"注释挡不住的未来事故
// （注释不是闸，判据才是）。这条用例从注册表里现取 v3.58.0 实例，断言它的覆盖清单为空。
// 取不到实例本身也是红：说明注册点被改掉了（改名/删除都要有人看见）。
func TestTenantColumnBatchRegistration(t *testing.T) {
	db := testutil.NewTestDB(t)
	if db == nil {
		t.Fatal("测试库不可达：生产注册形态无法判定")
	}
	// initial_schema.go 的注册路径：RegisterMigrations → registry.Register(...)。
	registry := migration.NewMigrationRegistry()
	RegisterMigrations(registry, db)
	mig, ok := registry.Get("v3.58.0")
	if !ok || mig == nil {
		t.Fatalf("注册表里没有 v3.58.0 —— 生产装配点被移除或改名，tenant_id 补列这条腿退出迁移链")
	}
	batch, ok := mig.(*TenantColumnBatchMigration)
	if !ok {
		t.Fatalf("v3.58.0 的实现类型变了（%T）—— 注册点被改写，请人工复核", mig)
	}
	if len(batch.tables) != 0 {
		t.Fatalf("生产注册的 v3.58.0 携带覆盖清单 %v —— withTables 是测试专用口子，"+
			"真库内省会静默退化成只处理这几张表（这是隔离缺口，不是配置）", batch.tables)
	}
}
