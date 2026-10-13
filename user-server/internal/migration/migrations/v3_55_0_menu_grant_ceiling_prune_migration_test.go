package migrations

import (
	"context"
	"sort"
	"testing"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// v3.55.0 摘的是"库里写着、注册表已经不认"的分配行，所以夹具必须同时具备三种行：
// 上界内的（要留下）、上界外的（要摘掉）、注册表里根本没这个 key 的（也要摘掉）。
// 缺任何一档，Up 的空跑都能被读成收敛成功 —— 比如只造越界行，那"把整表删光"的实现也能全绿。
//
// inKey / outKey 都是从注册表当场算出来的，不写死字面量：词表一改（比如把某个节点重新
// 放开给 staff），写死的夹具会悄悄变成"两个都在上界内"，那条腿就此失明。

type pruneFixture struct {
	db       *gorm.DB
	csIn     string
	csOut    string
	stIn     string
	stOut    string
	ghostKey string
}

// ceilingPair 当场挑出该角色"上界内/上界外"各一个 key。
func ceilingPair(t *testing.T, role string) (string, string) {
	t.Helper()
	eligible := model.EligibleMenuKeys(role)
	all := model.FlattenMenuKeys(model.SystemMenuTree)
	sort.Strings(all)

	var inKey, outKey string
	for _, k := range all {
		if _, ok := eligible[k]; ok {
			if inKey == "" {
				inKey = k
			}
			continue
		}
		if outKey == "" {
			outKey = k
		}
	}
	if inKey == "" {
		t.Fatalf("注册表里找不到 role=%s 上界内的节点：夹具前提没了（上界整体为空时本迁移本该拒绝收敛）", role)
	}
	if outKey == "" {
		t.Fatalf("注册表里找不到 role=%s 上界外的节点：本迁移无东西可摘，这条锁失去对象（先确认是不是词表被整体放开）", role)
	}
	return inKey, outKey
}

// newMenuGrantPruneFixture 建表并造九行分配：
// cs/staff 各有"上界内 + 上界外 + 幽灵"三行，admin 两行（迁移按角色循环，本不该碰），
// manager 一行（非三档角色码，只报告不删）。
func newMenuGrantPruneFixture(t *testing.T) *pruneFixture {
	t.Helper()
	db := testutil.NewTestDB(t, &model.RoleMenuPermission{})
	if db == nil {
		t.Fatal("测试库不可达：分配行的收敛无法判定（不 Skip，跳过等于没锁）")
	}
	csIn, csOut := ceilingPair(t, model.SystemRoleCodeCustomerService)
	stIn, stOut := ceilingPair(t, model.SystemRoleCodeStaff)
	ghostKey := "zzNodeNotInRegistry"
	if model.ExistsMenuKey(ghostKey) {
		t.Fatalf("夹具的幽灵 key %s 竟在注册表里，换名字", ghostKey)
	}

	rows := []*model.RoleMenuPermission{
		{RoleCode: model.SystemRoleCodeCustomerService, MenuKey: csIn},
		{RoleCode: model.SystemRoleCodeCustomerService, MenuKey: csOut},
		{RoleCode: model.SystemRoleCodeCustomerService, MenuKey: ghostKey},
		{RoleCode: model.SystemRoleCodeStaff, MenuKey: stIn},
		{RoleCode: model.SystemRoleCodeStaff, MenuKey: stOut},
		{RoleCode: model.SystemRoleCodeStaff, MenuKey: ghostKey},
		// admin 的分配行是历史脏数据（设计上 admin 不落库、恒全量），本迁移不许动它。
		{RoleCode: model.SystemRoleCodeAdmin, MenuKey: csOut},
		{RoleCode: model.SystemRoleCodeAdmin, MenuKey: csIn},
		// 非三档角色码：今天没有任何读路径会取到它，迁移只念名字不删行。
		{RoleCode: "manager", MenuKey: csOut},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("造分配行失败: %v", err)
	}
	return &pruneFixture{db: db, csIn: csIn, csOut: csOut, stIn: stIn, stOut: stOut, ghostKey: ghostKey}
}

func (f *pruneFixture) row(role, key string) string {
	return role + "|" + key
}

// rows 读回 (role_code, menu_key) 全集。
func (f *pruneFixture) rows(t *testing.T) map[string]struct{} {
	t.Helper()
	var got []prunedRow
	if err := f.db.Table("role_menu_permissions").Select("role_code", "menu_key").
		Order("role_code").Order("menu_key").Scan(&got).Error; err != nil {
		t.Fatalf("读回分配行失败: %v", err)
	}
	set := make(map[string]struct{}, len(got))
	for _, r := range got {
		set[r.RoleCode+"|"+r.MenuKey] = struct{}{}
	}
	if len(set) != len(got) {
		t.Fatalf("读回 %d 行但去重后只剩 %d 个键，唯一索引形状与预期不符", len(got), len(set))
	}
	return set
}

func (f *pruneFixture) keysOf(set map[string]struct{}) []string {
	return keysOfStatic(set)
}

func assertRows(t *testing.T, got map[string]struct{}, want []string) {
	t.Helper()
	// 集合比对而不是有序比对：库里 ORDER BY 的排序键是 Postgres 的 collation，
	// 按 Go 的字符串序手抄期望顺序会红得莫名其妙。
	wantSet := make(map[string]struct{}, len(want))
	for _, w := range want {
		wantSet[w] = struct{}{}
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("期望留下 %s，实际余下 %v", w, keysOfStatic(got))
		}
	}
	for g := range got {
		if _, ok := wantSet[g]; !ok {
			t.Errorf("多出一行 %s（余下 %v）", g, keysOfStatic(got))
		}
	}
}

func keysOfStatic(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestMenuGrantCeilingPrune_PrunesOnlyOutOfCeilingRows(t *testing.T) {
	f := newMenuGrantPruneFixture(t)
	if err := NewMenuGrantCeilingPruneMigration(f.db).Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}

	got := f.rows(t)
	assertRows(t, got, []string{
		f.row("manager", f.csOut),
		f.row(model.SystemRoleCodeAdmin, f.csIn),
		f.row(model.SystemRoleCodeAdmin, f.csOut),
		f.row(model.SystemRoleCodeCustomerService, f.csIn),
		f.row(model.SystemRoleCodeStaff, f.stIn),
	})
	// 该摘的六类都真摘掉了（两档角色 × 越界节点/幽灵节点，加上各自的第二行越界）。
	for _, doomed := range []string{
		f.row(model.SystemRoleCodeCustomerService, f.csOut),
		f.row(model.SystemRoleCodeCustomerService, f.ghostKey),
		f.row(model.SystemRoleCodeStaff, f.stOut),
		f.row(model.SystemRoleCodeStaff, f.ghostKey),
	} {
		if _, ok := got[doomed]; ok {
			t.Errorf("该行本该被摘掉却还在: %s", doomed)
		}
	}
}

// 收敛只该摘上界之外的：换个方向补一刀，"有效授权一个不许丢"。
func TestMenuGrantCeilingPrune_KeepsEveryInCeilingRow(t *testing.T) {
	f := newMenuGrantPruneFixture(t)
	before := f.rows(t)

	eligible := map[string]struct{}{}
	for _, role := range []string{model.SystemRoleCodeCustomerService, model.SystemRoleCodeStaff} {
		for k := range model.EligibleMenuKeys(role) {
			eligible[f.row(role, k)] = struct{}{}
		}
	}
	var inCeiling []string
	for r := range before {
		if _, ok := eligible[r]; ok {
			inCeiling = append(inCeiling, r)
		}
	}
	if len(inCeiling) == 0 {
		t.Fatal("夹具造出的行没有一行在上界内，这条「不许误删」的锁打的是空集")
	}

	if err := NewMenuGrantCeilingPruneMigration(f.db).Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	after := f.rows(t)
	for _, r := range inCeiling {
		if _, ok := after[r]; !ok {
			t.Errorf("上界内的分配行被误删: %s", r)
		}
	}
}

func TestMenuGrantCeilingPrune_Idempotent(t *testing.T) {
	f := newMenuGrantPruneFixture(t)
	m := NewMenuGrantCeilingPruneMigration(f.db)
	ctx := context.Background()

	if err := m.Up(ctx); err != nil {
		t.Fatalf("首次 Up 失败: %v", err)
	}
	first := f.keysOf(f.rows(t))
	if err := m.Up(ctx); err != nil {
		t.Fatalf("二次 Up 失败: %v", err)
	}
	second := f.keysOf(f.rows(t))
	if len(first) != len(second) {
		t.Fatalf("二次 Up 改变行数：first=%d second=%d", len(first), len(second))
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("二次 Up 多摘了行：first=%v second=%v", first, second)
		}
	}
}

// 表还不存在（首次部署、AutoMigrate 之前）时不许报错，更不许把启动流程带崩：
// 没有存量分配，就没有可收敛的东西。
func TestMenuGrantCeilingPrune_MissingTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t, &model.RoleMenuPermission{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := db.Migrator().DropTable(&model.RoleMenuPermission{}); err != nil {
		t.Fatalf("删表失败: %v", err)
	}
	if db.Migrator().HasTable("role_menu_permissions") {
		t.Fatal("表没删掉，这条用例测的不是「表缺失」这一档")
	}
	if err := NewMenuGrantCeilingPruneMigration(db).Up(context.Background()); err != nil {
		t.Fatalf("表缺失时 Up 应返回 nil，实得 %v", err)
	}
}

func TestMenuGrantCeilingPrune_NilDB(t *testing.T) {
	if err := (&MenuGrantCeilingPruneMigration{}).Up(context.Background()); err == nil {
		t.Fatal("db 为 nil 时应报错，静默通过等于把装配错误咽进启动日志")
	}
}

// 上界读成空集时 pruneRole 必须收手并留下警告。这条锁的可观测面到此为止：把守卫摘掉，
// 库里也不会少一行（GORM 把空切片渲染成 `NOT IN (NULL)`，一条也匹配不上），差别只在日志里
// 少了一句 warning —— 而本包没有可注入的日志 sink（logger 是包级 zap 全局单例），
// 那句 warning 断不到。因此这一条用例锁的是"拒绝收敛且不删行、不报错"，不是"必出声"。
//
// 角色码当场验空，不写死：注册表里只要有哪个节点没写 Roles（= 对所有角色放行），
// 非法角色就会拿到非空上界，这条锁随之失去对象，此时必须把话说响而不是测个空集蒙过去。
func TestMenuGrantCeilingPrune_EmptyCeilingRefusesPrune(t *testing.T) {
	f := newMenuGrantPruneFixture(t)
	const bogusRole = "no_such_role"
	if len(model.EligibleMenuKeys(bogusRole)) != 0 {
		t.Fatalf("非法角色 %s 的上界不为空（注册表里有节点没写 Roles），本用例的前提没了", bogusRole)
	}
	if err := f.db.Create(&model.RoleMenuPermission{RoleCode: bogusRole, MenuKey: f.csOut}).Error; err != nil {
		t.Fatalf("造非法角色的分配行失败: %v", err)
	}

	before := f.rows(t)
	if err := NewMenuGrantCeilingPruneMigration(f.db).pruneRole(context.Background(), bogusRole); err != nil {
		t.Fatalf("上界为空时应返回 nil（拒绝收敛、交给读侧交集兜底），实得 %v", err)
	}
	after := f.rows(t)
	if len(before) != len(after) {
		t.Fatalf("上界为空却删了行：before=%d after=%d（余下 %v）", len(before), len(after), keysOfStatic(after))
	}
	if _, ok := after[f.row(bogusRole, f.csOut)]; !ok {
		t.Errorf("拒绝收敛却没把那一行留在库里")
	}
}

// 元信息：版本号是本迁移的身份（重复注册会被 registry.Register 判 panic），
// 这里钉的是"号没写错、名字与说明不为空"——管理台的升级列表靠这两句给人读出这一步干什么。
func TestMenuGrantCeilingPrune_Meta(t *testing.T) {
	m := NewMenuGrantCeilingPruneMigration(nil)
	if m.Version() != "v3.55.0" {
		t.Errorf("Version() = %s，期望 v3.55.0", m.Version())
	}
	if m.Name() == "" || m.Description() == "" {
		t.Error("Name()/Description() 不能为空")
	}
}

// Down 不许把越界行回填回来 —— 那是把"注册表已经不授予"的授权重新写进库里。
func TestMenuGrantCeilingPrune_DownDoesNotRestorePrunedRows(t *testing.T) {
	f := newMenuGrantPruneFixture(t)
	m := NewMenuGrantCeilingPruneMigration(f.db)
	ctx := context.Background()

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	before := f.rows(t)
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down 失败: %v", err)
	}
	after := f.rows(t)
	if len(before) != len(after) {
		t.Fatalf("Down 改变了行数：before=%d after=%d", len(before), len(after))
	}
	for _, k := range []string{f.csOut, f.ghostKey} {
		if _, ok := after[f.row(model.SystemRoleCodeCustomerService, k)]; ok {
			t.Errorf("Down 回填了越界分配行 %s", f.row(model.SystemRoleCodeCustomerService, k))
		}
	}
}

// 注册过才算数：只写文件不 register 的迁移是死码，谁都读不到它跑过。
func TestMenuGrantCeilingPrune_Registered(t *testing.T) {
	db := testutil.NewTestDB(t, &model.RoleMenuPermission{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	registry := migration.NewMigrationRegistry()
	RegisterMigrations(registry, db)
	if _, ok := registry.Get("v3.55.0"); !ok {
		t.Fatal("v3.55.0 没注册进 RegisterMigrations：库里那些越界分配行永远没人摘")
	}
}
