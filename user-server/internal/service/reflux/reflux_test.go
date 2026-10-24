// Package reflux T1 回流管道测试。
// 纯逻辑用例（指纹/关键词/状态机）无 DB 依赖；集成用例需 POSTGRES_TEST_DSN。
package reflux

import (
	"testing"

	"hivemtk-user/internal/model"
)

func TestNormalizeQuestion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"  怎么退款？  ", "怎么退款"},
		{"怎么退款", "怎么退款"},
		{"ＷＩＦＩ 密码！", "wifi密码"}, // 全角+标点+小写
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeQuestion(c.in); got != c.want {
			t.Errorf("Normalize(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

// 等价问句必须同指纹（全半角/标点/大小写/空白差异）
func TestQuestionHashStable(t *testing.T) {
	a := QuestionHash("怎么退款？")
	b := QuestionHash("  怎么退款！ ")
	c := QuestionHash("ＷＩＦＩ密码")
	d := QuestionHash("wifi密码")
	if a != b {
		t.Errorf("等价问句指纹不同: %s vs %s", a, b)
	}
	if c != d {
		t.Errorf("全半角指纹不同: %s vs %s", c, d)
	}
	if a == c {
		t.Error("不同问句不应同指纹")
	}
}

func TestGenerateKeywords(t *testing.T) {
	kws := GenerateKeywords("怎么申请退款")
	if len(kws) == 0 {
		t.Fatal("应产出关键词")
	}
	for _, k := range kws {
		if generalizedStopwords[k] {
			t.Errorf("关键词 %q 是泛化停用词", k)
		}
	}
}

// 教训锁定（设计文档 §6.2 第 5 条）：泛化问句直接拒绝
func TestKeyworderRejectsGeneralized(t *testing.T) {
	// "怎么办"：候选全为泛化 → 拒绝
	err := ValidateKeywords(GenerateKeywords("怎么办"), map[string]int{})
	if err == nil {
		t.Fatal("『怎么办』应被拒绝")
	}
	// "怎么退款"：保留"退款""申请退款"等 → 通过（且含区分词）
	err = ValidateKeywords(GenerateKeywords("怎么申请退款"), map[string]int{"退款": 1})
	if err != nil {
		t.Fatalf("『怎么申请退款』应通过: %v", err)
	}
	// 与既有 FAQ 完全同键 → 拒绝
	existing := map[string]int{"申请退款": 1, "请退款": 1, "我要退款": 1}
	// 构造一个生成关键词全是既有键的问句较难（贪心切词覆盖广），直接校验函数语义：
	err = ValidateKeywords([]string{"申请退款", "我要退款"}, existing)
	if err == nil {
		t.Fatal("全同键应拒绝")
	}
}

func TestValidateKeywordsTooShort(t *testing.T) {
	if err := ValidateKeywords([]string{"a", "b"}, map[string]int{}); err == nil {
		t.Fatal("单字符关键词应拒绝")
	}
}

// 状态机白名单语义（无 DB：直接查表）
func TestTransitionWhitelist(t *testing.T) {
	if !allowedTransitions[model.UQStatusDraft][ActionSubmit] {
		t.Error("draft→submit 应允许")
	}
	if allowedTransitions[model.UQStatusDraft][ActionAdopt] {
		t.Error("draft→adopt 应拒绝（非法迁移）")
	}
	if !allowedTransitions[model.UQStatusPendingReview][ActionAdopt] {
		t.Error("pending_review→adopt 应允许")
	}
	if _, ok := allowedTransitions[model.UQStatusAdopted]; ok {
		t.Error("终态不应有出边")
	}
}

// Collector 关闭语义：Emit 零开销不 panic
func TestCollectorDisabledNoop(t *testing.T) {
	c := NewCollector(nil, 8, false)
	c.Emit(Event{Question: "测试问题"})
	if c.Enabled() {
		t.Error("refluxEnabled=false 时应保持关闭")
	}
	c.Stop() // 关闭态 Stop 应安全
}
