package migrations

import (
	"context"
	"sort"
	"testing"

	geomodel "hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// 这组夹具的任务是同时把「该改判」与「不该改判」两侧的行摆满一张表。
// 只造该改判的行，实现退化成"整表 type=9 都改"也能全绿；只造不该改判的行，
// 实现退化成"一条 UPDATE 改全表"同样能全绿（因为它一个都没改，而那正是期望）。
// 所以两侧都必须有，且每一行都要能说清它在哪一档、由哪道闸留下。

const (
	geoChainMatch    = "verify:BrandA:11111111-1111-1111-1111-111111111111"
	geoChainSoftDel  = "verify:BrandB:22222222-2222-2222-2222-222222222222"
	geoChainEmptyKey = ""
)

type geoSplitFixture struct {
	db *gorm.DB
}

// newGeoSplitFixture 建两张表并造 7 条线索 + 3 条思维链。
//
// 思维链里刻意放一条 chain_id 为空的：库里今天没有这种行（现测开发库 58 条链、空 chain_id 0 条），
// 它是 `source_id <> ”` 那道闸的靶子 —— 没有它，那条闸删掉也不会有任何一行改判错，
// 测试就只是在证明"少一个条件也没关系"。
func newGeoSplitFixture(t *testing.T) *geoSplitFixture {
	t.Helper()
	db := testutil.NewTestDB(t, &model.Clue{}, &geomodel.GeoQueryChain{})
	if db == nil {
		t.Fatal("测试库不可达：改判是否发生无法判定（不 Skip，跳过等于没锁）")
	}
	ctx := context.Background()

	chains := []*geomodel.GeoQueryChain{
		{ChainID: geoChainMatch, Query: "有没有好用的 AI 客服", BrandName: "BrandA"},
		{ChainID: geoChainSoftDel, Query: "BrandB 怎么样", BrandName: "BrandB"},
		{ChainID: geoChainEmptyKey, Query: "空号链", BrandName: "BrandC"},
	}
	if err := db.WithContext(ctx).Create(&chains).Error; err != nil {
		t.Fatalf("造思维链失败: %v", err)
	}
	// 一条链软删：GEO 侧的链会被删（用户清理会话），而"这条线索当初是不是 GEO 捕获的"
	// 不跟着一起失效，所以子查询必须连软删的链一起认。
	if err := db.WithContext(ctx).Where("chain_id = ?", geoChainSoftDel).
		Delete(&geomodel.GeoQueryChain{}).Error; err != nil {
		t.Fatalf("软删思维链失败: %v", err)
	}

	clues := []*model.Clue{
		// 该改判：命中在册链的存量 GEO 捕获行。
		{Account: "hitChain", Type: 9, SourceID: geoChainMatch},
		// 该改判：命中已被软删的链。
		{Account: "hitDeletedChain", Type: 9, SourceID: geoChainSoftDel},
		// 该改判：线索自己也被软删过 —— 一行"当初是谁"不该因为被删就换身份。
		{Account: "hitSoftDeletedClue", Type: 9, SourceID: geoChainMatch},
		// 不该改判：source_id 空（开发库 type=9 的 162 行全是这一档）。
		{Account: "emptySource", Type: 9},
		// 不该改判：source_id 是别的来源写的字面量（抖音群号形态）。
		{Account: "otherSource", Type: 9, SourceID: "666888111"},
		// 不该改判：已经是新号，重复跑不许再动它，也不许报错。
		{Account: "alreadyNew", Type: 18, SourceID: geoChainMatch},
		// 不该改判：命中链但不是老号那一档（Telegram 的 source_id 也存群号，形状与链无关，
		// 这一行是为了钉住"判据带 type=9 这个前提"，去掉它会把别的来源一起卷进来）。
		{Account: "otherTypeHitChain", Type: 4, SourceID: geoChainMatch},
	}
	if err := db.WithContext(ctx).Create(&clues).Error; err != nil {
		t.Fatalf("造线索失败: %v", err)
	}
	// 不带 Unscoped：那会硬删这一行，而本迁移要测的是"软删行也一起改判"。
	if err := db.WithContext(ctx).Where("account = ?", "hitSoftDeletedClue").
		Delete(&model.Clue{}).Error; err != nil {
		t.Fatalf("软删线索失败: %v", err)
	}

	// 夹具前提自己得是真的：两行该软删的确实带着 deleted_at，否则上面两条"软删档"
	// 只是在测没被删的行，而测试读起来像在测软删。
	var deletedClues, deletedChains int64
	if err := db.WithContext(ctx).Table("clues").Where("deleted_at IS NOT NULL").Count(&deletedClues).Error; err != nil {
		t.Fatalf("数软删线索失败: %v", err)
	}
	if err := db.WithContext(ctx).Table("geo_query_chains").Where("deleted_at IS NOT NULL").Count(&deletedChains).Error; err != nil {
		t.Fatalf("数软删思维链失败: %v", err)
	}
	if deletedClues != 1 || deletedChains != 1 {
		t.Fatalf("夹具的软删档没落地：clues 软删 %d 行（期望 1）、链软删 %d 行（期望 1）", deletedClues, deletedChains)
	}
	return &geoSplitFixture{db: db}
}

// snapshot 读回整表（含软删行）的 account → type。
// 用 account 当键而不是 id：UUID 主键由钩子当场生成，写死不了，而 account 是夹具逐条起的名字。
func (f *geoSplitFixture) snapshot(t *testing.T) map[string]int64 {
	t.Helper()
	type row struct {
		Account string
		Type    int64
	}
	var rows []row
	// Unscoped：本判据要看的是"库里每一行写着什么"，软删行也在内。
	if err := f.db.Unscoped().Table("clues").Select("account", "type").Scan(&rows).Error; err != nil {
		t.Fatalf("读回线索失败: %v", err)
	}
	got := make(map[string]int64, len(rows))
	for _, r := range rows {
		if _, dup := got[r.Account]; dup {
			t.Fatalf("夹具里出现重名 account=%s，逐行断言会失去归属", r.Account)
		}
		got[r.Account] = r.Type
	}
	if len(got) != 7 {
		t.Fatalf("夹具本该有 7 条线索，读回 %d 条：%v", len(got), geoAccountsOf(got))
	}
	return got
}

func geoAccountsOf(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func assertGeoTypes(t *testing.T, got map[string]int64, want map[string]int64) {
	t.Helper()
	for account, wantType := range want {
		if actual, ok := got[account]; !ok {
			t.Errorf("少了 account=%s（现有 %v）", account, geoAccountsOf(got))
		} else if actual != wantType {
			t.Errorf("account=%s 的 type = %d，期望 %d", account, actual, wantType)
		}
	}
	for account, actual := range got {
		if _, ok := want[account]; !ok {
			t.Errorf("多出行 account=%s（type=%d），夹具里没有这一档：%v", account, actual, geoAccountsOf(got))
		}
	}
}

func TestClueGeoCaptureTypeSplit_MovesOnlyChainMatchedLegacyRows(t *testing.T) {
	f := newGeoSplitFixture(t)
	if err := NewClueGeoCaptureTypeSplitMigration(f.db).Up(context.Background()); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	assertGeoTypes(t, f.snapshot(t), map[string]int64{
		"hitChain":           geoCaptureType,
		"hitDeletedChain":    geoCaptureType,
		"hitSoftDeletedClue": geoCaptureType,
		"emptySource":        legacyGeoCaptureType,
		"otherSource":        legacyGeoCaptureType,
		"alreadyNew":         geoCaptureType,
		"otherTypeHitChain":  4,
	})
}

// 开发库今天的形状（type=9 全为空 source_id）：本迁移在那种库上必须一行都不动。
// 单独留这一条是因为整表断言里"没动"这件事是七行里的四行，混在一起读不出"这就是线上实形"。
func TestClueGeoCaptureTypeSplit_DevDBShapeUntouched(t *testing.T) {
	db := testutil.NewTestDB(t, &model.Clue{}, &geomodel.GeoQueryChain{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	ctx := context.Background()
	if err := db.WithContext(ctx).Create(&geomodel.GeoQueryChain{ChainID: geoChainEmptyKey}).Error; err != nil {
		t.Fatalf("造空 chain_id 的链失败: %v", err)
	}
	rows := []*model.Clue{
		{Account: "a", Type: 9},
		{Account: "b", Type: 9},
		{Account: "c", Type: 9},
	}
	if err := db.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("造线索失败: %v", err)
	}
	if err := NewClueGeoCaptureTypeSplitMigration(db).Up(ctx); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	var moved int64
	if err := db.WithContext(ctx).Table("clues").Where("type = ?", geoCaptureType).Count(&moved).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if moved != 0 {
		t.Fatalf("空 source_id 的存量行被改判 %d 行：判据退化成了全表扫（开发库那 162 条抖音线索就是这么被卷进去的）", moved)
	}
}

func TestClueGeoCaptureTypeSplit_Idempotent(t *testing.T) {
	f := newGeoSplitFixture(t)
	ctx := context.Background()
	m := NewClueGeoCaptureTypeSplitMigration(f.db)

	if err := m.Up(ctx); err != nil {
		t.Fatalf("首次 Up 失败: %v", err)
	}
	first := f.snapshot(t)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("二次 Up 失败: %v", err)
	}
	assertGeoTypes(t, f.snapshot(t), first)
}

// 两张表各缺一次：clues 不在＝无物可改；geo_query_chains 不在＝没有任何一行能被认成
// GEO 捕获，而且子查询会直接报错把启动流程带崩。后者是本迁移唯一能把服务起不来
// 传染给别人的路径，所以单独一条，不并入"表缺失即空跑"那句泛泛的话。
func TestClueGeoCaptureTypeSplit_MissingCluesTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t, &model.Clue{}, &geomodel.GeoQueryChain{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := db.Migrator().DropTable(&model.Clue{}); err != nil {
		t.Fatalf("删 clues 表失败: %v", err)
	}
	if db.Migrator().HasTable("clues") {
		t.Fatal("clues 没删掉，这条测的不是「表缺失」")
	}
	if err := NewClueGeoCaptureTypeSplitMigration(db).Up(context.Background()); err != nil {
		t.Fatalf("clues 缺失时 Up 应返回 nil，实得 %v", err)
	}
}

func TestClueGeoCaptureTypeSplit_MissingChainTableIsNoop(t *testing.T) {
	db := testutil.NewTestDB(t, &model.Clue{}, &geomodel.GeoQueryChain{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	if err := db.Create(&model.Clue{Account: "a", Type: 9, SourceID: geoChainMatch}).Error; err != nil {
		t.Fatalf("造线索失败: %v", err)
	}
	if err := db.Migrator().DropTable(&geomodel.GeoQueryChain{}); err != nil {
		t.Fatalf("删 geo_query_chains 失败: %v", err)
	}
	if db.Migrator().HasTable("geo_query_chains") {
		t.Fatal("链表没删掉，这条测的不是「子查询的表不存在」")
	}
	if err := NewClueGeoCaptureTypeSplitMigration(db).Up(context.Background()); err != nil {
		t.Fatalf("链表缺失时 Up 应返回 nil（没有链就没有 GEO 捕获），实得 %v", err)
	}
	var got int64
	if err := db.Table("clues").Where("type = ?", geoCaptureType).Count(&got).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if got != 0 {
		t.Errorf("链表都不在却改判了 %d 行", got)
	}
}

func TestClueGeoCaptureTypeSplit_NilDB(t *testing.T) {
	if err := (&ClueGeoCaptureTypeSplitMigration{}).Up(context.Background()); err == nil {
		t.Fatal("db 为 nil 时应报错，静默通过等于把装配错误咽进启动日志")
	}
}

// Down 不许把 18 折回 9：回填的判据只能与 Up 同形，而它分不清"本迁移搬动的行"与
// "迁移之后新代码按 18 写入的行"，回滚会把后者一并误标。
// 夹具里的 alreadyNew 走的就是"新代码写入的行"这一档。
func TestClueGeoCaptureTypeSplit_DownDoesNotRollBack(t *testing.T) {
	f := newGeoSplitFixture(t)
	ctx := context.Background()
	m := NewClueGeoCaptureTypeSplitMigration(f.db)

	if err := m.Up(ctx); err != nil {
		t.Fatalf("Up 失败: %v", err)
	}
	before := f.snapshot(t)
	if err := m.Down(ctx); err != nil {
		t.Fatalf("Down 失败: %v", err)
	}
	assertGeoTypes(t, f.snapshot(t), before)
	if before["hitChain"] != geoCaptureType {
		t.Fatal("夹具的 Up 没把命中行改成新号，下面的「Down 不回填」断言打的是空集")
	}
}

func TestClueGeoCaptureTypeSplit_Meta(t *testing.T) {
	m := NewClueGeoCaptureTypeSplitMigration(nil)
	if m.Version() != "v3.56.0" {
		t.Errorf("Version() = %s，期望 v3.56.0", m.Version())
	}
	if m.Name() == "" || m.Description() == "" {
		t.Error("Name()/Description() 不能为空，管理台的升级列表靠这两句读出这一步干什么")
	}
}

// 两个号必须不同：写成同一个号时 Up 变成空转（改了又改回同一个值），
// 而上面所有断言看的是"库里写着什么"，它照样全绿。
func TestClueGeoCaptureTypeSplit_LegacyAndNewDiffer(t *testing.T) {
	if legacyGeoCaptureType == geoCaptureType {
		t.Fatal("老号与新号相同：本迁移什么都不做也会被读成「已改判」")
	}
}

// 注册过才算数：只写文件不 register 的迁移是死码，谁都读不到它跑过。
func TestClueGeoCaptureTypeSplit_Registered(t *testing.T) {
	db := testutil.NewTestDB(t, &model.Clue{}, &geomodel.GeoQueryChain{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	registry := migration.NewMigrationRegistry()
	RegisterMigrations(registry, db)
	if _, ok := registry.Get("v3.56.0"); !ok {
		t.Fatal("v3.56.0 没注册进 RegisterMigrations：老库里那些写着 9 的 GEO 捕获行会一直显示成抖音")
	}
}
