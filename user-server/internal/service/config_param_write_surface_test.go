package service

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// registeredValueTypes 是 validateValue 里**有判据分支**的那批类型。
//
// 这张表与种子里实际用到的类型集合必须对齐：往登记表加一个 value_type 而忘了
// 在 validateValue 里加分支时，那个类型的所有参数会变成「什么都写得进、
// 读出来全是兜底值」——而写侧的测试如果只测已有类型，一个都发现不了它。
var registeredValueTypes = []string{"int", "float", "duration", "bool", "json", "string"}

// TestEverySeedValueTypeHasAValidationBranch 种子表用到的类型 ⊆ 有判据的类型集合。
func TestEverySeedValueTypeHasAValidationBranch(t *testing.T) {
	seen := map[string]int{}
	for _, def := range DefaultParamDefs() {
		seen[def.ValueType]++
	}
	var unvalidated []string
	for vt, n := range seen {
		if !slices.Contains(registeredValueTypes, vt) {
			unvalidated = append(unvalidated, vt+"（"+strconv.Itoa(n)+" 条）")
		}
	}
	if len(unvalidated) > 0 {
		t.Errorf("种子里有 %d 个类型在 validateValue 没有判据分支：%v；给它们写值会静默存下读不出的形状",
			len(unvalidated), unvalidated)
	}
	// 正控制：未知类型必须被 default 那条穷举判据拦住，而不是走进「没分支＝放行」。
	if err := validateValue("not_a_registered_type", "1", nil, nil); err == nil {
		t.Error("未知 value_type 走了「什么都不校验」那条路")
	}
	t.Logf("类型覆盖：种子用到 %d 个类型，全部有判据分支；登记表 %v", len(seen), registeredValueTypes)
}

// TestEverySeedDefaultPassesItsOwnValidation 全部 217 条（不限 lock 组）：
// 每条的 DefaultValue 必须过自己类型的校验。
//
// 为什么这条要覆盖全表：ResetToDefault / BulkResetGroup 是把 DefaultValue 直接写回参数行，
// 而**不**过 validateValue。一条默认值本身不合法的种子等于埋着两颗雷——
// 点「重置」那一刻写进库的是脏值，读取侧从此每次解析失败、每次静默回落兜底值。
// 只测 lock 组（历史那道用例的口径）时，其余 213 条的默认值从来没被这一层看过。
func TestEverySeedDefaultPassesItsOwnValidation(t *testing.T) {
	defs := DefaultParamDefs()
	var bad int
	for _, def := range defs {
		if err := validateValue(def.ValueType, def.DefaultValue, def.Min, def.Max); err != nil {
			bad++
			t.Errorf("%s.%s（type=%s）默认值 %q 过不了自身校验：%v",
				def.Group, def.Key, def.ValueType, def.DefaultValue, err)
		}
	}
	if bad == 0 {
		t.Logf("默认值自检：%d 条全部通过", len(defs))
	}
}

// TestWritableRowsAreExactlyNonReadOnlyNonDeprecated 「能不能写」只有一个口径来源：
// 只读标记与下架标记。这条把它与种子表对账，防止出现「页面可点保存、接口恒拒」
// 或反过来「标了只读却写得进去」这类两侧各说各话的形状。
func TestWritableRowsAreExactlyNonReadOnlyNonDeprecated(t *testing.T) {
	var writable, readOnly, deprecated int
	for _, def := range DefaultParamDefs() {
		switch {
		case def.Deprecated:
			deprecated++
		case def.ReadOnly:
			readOnly++
		default:
			writable++
		}
	}
	if writable == 0 {
		t.Fatal("一条可写参数都没有（种子表或本判据的口径坏了）")
	}
	if deprecated+readOnly == 0 {
		t.Error("既没有只读也没有下架条目，判据失去判别力")
	}
	t.Logf("写入面分类：可写 %d、只读 %d、下架 %d，合计 %d", writable, readOnly, deprecated, writable+readOnly+deprecated)
	if total := writable + readOnly + deprecated; total != len(DefaultParamDefs()) {
		t.Errorf("分类合计 %d ≠ 种子总数 %d", total, len(DefaultParamDefs()))
	}
}

// TestBoolWriteCanonicalizesToTrueFalse 布尔别名写进去要折成 true/false。
// 判据落在**库里那一行**而不是内存视图：页面渲染读的就是这一列。
func TestBoolWriteCanonicalizesToTrueFalse(t *testing.T) {
	db := newBoolCanonicalDB(t)
	svc := NewConfigParamService()
	ctx := context.Background()
	const group, key = "cache", "faq_answer_enabled"
	var p model.ConfigParam
	for _, alias := range []string{"1", "t", "TRUE", "True"} {
		if err := svc.UpdateValue(ctx, group, key, alias, 1); err != nil {
			t.Fatalf("写入别名 %q 失败：%v", alias, err)
		}
		if err := db.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
			t.Fatalf("读回失败：%v", err)
		}
		if p.Value != "true" {
			t.Errorf("库里存的是 %q，期望规范化成 \"true\"", p.Value)
		}
		if got := svc.GetBool(ctx, group, key, false); got != true {
			t.Errorf("GetBool 读回 %v，别名 %q 本该是真", got, alias)
		}
	}
	// 反向：布尔位上写非布尔必须红，且被拒之后库里那一行不许变。
	if err := svc.UpdateValue(ctx, group, key, "随便什么", 1); err == nil {
		t.Error("布尔参数收下了非布尔值")
	}
	if err := db.Where(&model.ConfigParam{Group: group, Key: key}).First(&p).Error; err != nil {
		t.Fatalf("读回失败：%v", err)
	}
	if p.Value != "true" {
		t.Errorf("被拒之后库里值却变了：%q", p.Value)
	}
}

func newBoolCanonicalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamGroup{}, &model.ConfigParamAuditLog{})
	row := model.ConfigParam{
		Group: "cache", Key: "faq_answer_enabled", Name: "开启 FAQ 语义答案缓存",
		ValueType: "bool", Value: "true", DefaultValue: "true",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("建夹具行失败：%v", err)
	}
	return db
}
