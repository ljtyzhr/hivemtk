// normalize_test.go 归一化的两条硬约束：①变体必须折叠到同一形态，②折叠后必须能映射回原文。
//
// 这两个断言是内容审查的证据链底线：只折叠不回映射，审查记录里只能贴被抹掉标点的文本；
// 只回映射不折叠，"敏 感 词"这类 10 行代码就能写出的规避方式会让整个词库失效。
package normalize

import (
	"testing"
	"unicode/utf8"
)

func TestText_FoldsCommonEvasion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"原样", "敏感词", "敏感词"},
		{"插入空格", "敏 感 词", "敏感词"},
		{"插入星号", "敏*感*词", "敏感词"},
		{"插入短横", "敏-感-词", "敏感词"},
		{"全角空格", "敏\u3000感\u3000词", "敏感词"},
		{"零宽字符", "敏\u200b感\u200d词", "敏感词"},
		{"BOM 与软连字符", "\ufeff敏\u00ad感词", "敏感词"},
		{"全角字母数字", "ＡＢＣ１２３", "abc123"},
		{"大小写折叠", "HeLLo", "hello"},
		{"全角标点", "敏\uFF0A感词", "敏感词"},
		{"换行制表符", "敏\t感\n词", "敏感词"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.in)
			if got.Text != tc.want {
				t.Fatalf("Text(%q) = %q，期望 %q", tc.in, got.Text, tc.want)
			}
			if len(got.Index) != utf8.RuneCountInString(got.Text) {
				t.Fatalf("Index 长度 %d 与归一化文本 rune 数 %d 不一致",
					len(got.Index), utf8.RuneCountInString(got.Text))
			}
		})
	}
}

func TestWithOptions_KeepFlags(t *testing.T) {
	const in = "A B"

	if got := WithOptions(in, Options{KeepSeparators: true}).Text; got != "a b" {
		t.Fatalf("KeepSeparators 结果 = %q，期望 %q", got, "a b")
	}
	if got := WithOptions("ＡＢ", Options{KeepWidth: true}).Text; got != "ａｂ" {
		t.Fatalf("KeepWidth 结果 = %q，期望 %q（不折叠宽度但仍折叠大小写）", got, "ａｂ")
	}
	if got := WithOptions("ＡＢ", Options{KeepWidth: true, KeepCase: true}).Text; got != "ＡＢ" {
		t.Fatalf("KeepWidth+KeepCase 结果 = %q，期望 %q", got, "ＡＢ")
	}
	if got := WithOptions("AB", Options{KeepCase: true}).Text; got != "AB" {
		t.Fatalf("KeepCase 结果 = %q，期望 %q", got, "AB")
	}
	// 零值选项必须等于生产口径：零值≠默认值是最常见的静默配置错误。
	if WithOptions(in, Options{}).Text != Text(in).Text {
		t.Fatal("零值 Options 与 Text 口径不一致")
	}
}

func TestResult_OriginalRangeMapsBack(t *testing.T) {
	const in = "前面 敏*感*词 后面"
	res := Text(in)

	start := -1
	for i, r := range res.Runes() {
		if r == '敏' {
			start = i
			break
		}
	}
	if start < 0 {
		t.Fatalf("归一化文本 %q 中找不到敏感词", res.Text)
	}
	from, to, ok := res.OriginalRange(start, start+3)
	if !ok {
		t.Fatal("OriginalRange 未能映射")
	}
	if got := string([]rune(in)[from:to]); got != "敏*感*词" {
		t.Fatalf("映射回原文 = %q，期望 %q（含插入的分隔符）", got, "敏*感*词")
	}
}

func TestResult_OriginalRangeOutOfBoundsIsClamped(t *testing.T) {
	res := Text("abc")
	cases := []struct{ start, end int }{
		{-5, 2},
		{1, 999},
		{0, 3},
	}
	for _, tc := range cases {
		from, to, ok := res.OriginalRange(tc.start, tc.end)
		if !ok {
			t.Fatalf("[%d,%d) 应当被夹取后返回 ok", tc.start, tc.end)
		}
		if from < 0 || to > 3 || from >= to {
			t.Fatalf("[%d,%d) 夹取结果越界：%d..%d", tc.start, tc.end, from, to)
		}
	}
	if _, _, ok := res.OriginalRange(2, 2); ok {
		t.Fatal("空区间不应返回 ok")
	}
}

func TestText_Empty(t *testing.T) {
	res := Text("")
	if res.Text != "" || res.Index != nil {
		t.Fatalf("空输入应得零值，得 %+v", res)
	}
	if _, _, ok := res.OriginalRange(0, 1); ok {
		t.Fatal("空结果不应能映射区间")
	}
}

func TestResult_RunesMatchesText(t *testing.T) {
	res := Text("敏 感 词")
	if got := string(res.Runes()); got != res.Text {
		t.Fatalf("Runes() = %q 与 Text = %q 不一致", got, res.Text)
	}
}
