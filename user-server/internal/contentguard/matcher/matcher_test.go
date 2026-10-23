// matcher_test.go AC 自动机的行为断言。
//
// 重点不在"能匹配到"（朴素子串也能），而在三件朴素实现做不到的事：
//  1. 命中**位置**（证据与白名单抑制都依赖它）；
//  2. **全部**命中（不是找到第一个就返回）；
//  3. 归一化之后仍能对上（"敏 感 词"/全角/零宽/大小写四类规避）。
package matcher

import (
	"fmt"
	"testing"
)

func pattern(id uint, word string, sev int) Pattern {
	return Pattern{ID: id, Word: word, Category: "test", Severity: sev}
}

func TestMatch_Basic(t *testing.T) {
	m := New([]Pattern{pattern(7, "毒品", 3)})
	if m.Size() != 1 {
		t.Fatalf("Size = %d，期望 1", m.Size())
	}
	hits := m.Match("我要买毒品")
	if len(hits) != 1 {
		t.Fatalf("命中数 = %d，期望 1", len(hits))
	}
	h := hits[0]
	if h.PatternID != 7 || h.Word != "毒品" || h.Severity != 3 {
		t.Fatalf("命中内容不对：%+v", h)
	}
	if h.Start != 3 || h.End != 5 {
		t.Fatalf("命中区间 = [%d,%d)，期望 [3,5)", h.Start, h.End)
	}
}

func TestMatch_AllHitsNotJustFirst(t *testing.T) {
	m := New([]Pattern{pattern(1, "ab", 1), pattern(2, "bc", 1), pattern(3, "abc", 2)})
	hits := m.Match("abc")
	if len(hits) != 3 {
		t.Fatalf("命中数 = %d，期望 3（重叠命中不能只报一个）：%+v", len(hits), hits)
	}
	// 稳定排序：先按结束位置，再按 PatternID。
	for i := 1; i < len(hits); i++ {
		prev, cur := hits[i-1], hits[i]
		if cur.End < prev.End || (cur.End == prev.End && cur.PatternID < prev.PatternID) {
			t.Fatalf("命中未按 (End, PatternID) 排序：%+v", hits)
		}
	}
}

func TestMatch_SuffixPatternViaFailChain(t *testing.T) {
	// AC 的失配指针用例：后缀词必须经由 fail 链命中，否则"hers/she"这类词库会漏。
	m := New([]Pattern{pattern(1, "he", 1), pattern(2, "she", 2), pattern(3, "hers", 3)})
	hits := m.Match("ushers")
	got := map[uint]bool{}
	for _, h := range hits {
		got[h.PatternID] = true
	}
	for _, id := range []uint{1, 2, 3} {
		if !got[id] {
			t.Fatalf("模式 %d 未命中，命中集合=%v（全部命中：%+v）", id, got, hits)
		}
	}
}

func TestMatch_EvasionVariants(t *testing.T) {
	m := New([]Pattern{pattern(1, "敏感词", 3)})
	texts := []string{
		"敏感词",
		"敏 感 词",
		"敏*感*词",
		"敏-感-词",
		"敏\u200b感\u200c词",
	}
	for _, txt := range texts {
		if hits := m.Match(txt); len(hits) == 0 {
			t.Fatalf("%q 未命中（归一化对抗失效）", txt)
		}
	}

	en := New([]Pattern{pattern(2, "sensitive", 2)})
	for _, txt := range []string{"Sensitive", "ＳＥＮＳＩＴＩＶＥ", "s e n s i t i v e", "sen\u200bsitive"} {
		if hits := en.Match(txt); len(hits) == 0 {
			t.Fatalf("%q 未命中英文词的变体", txt)
		}
	}
}

func TestMatch_WordWithSeparatorItself(t *testing.T) {
	// 词条本身含分隔符（"7 天 无 理 由"）时，词与文本必须用同一套归一化，否则永不命中。
	m := New([]Pattern{pattern(1, "7 天 无 理 由", 2)})
	if hits := m.Match("活动说明：7天无理由退货"); len(hits) != 1 {
		t.Fatalf("命中数 = %d，期望 1：%+v", len(hits), hits)
	}
}

func TestNew_SkipsEmptyAndClampsSeverity(t *testing.T) {
	m := New([]Pattern{
		pattern(1, "***", 2), // 归一化后为空 ⇒ 丢弃
		pattern(2, "  ", 2),  // 同上
		pattern(3, "甲", 0),   // severity 越界 ⇒ 夹到 1
		pattern(4, "乙", 99),  // ⇒ 夹到 3
	})
	if m.Size() != 2 {
		t.Fatalf("Size = %d，期望 2（纯标点词必须丢弃，否则会匹配空串）", m.Size())
	}
	for _, h := range m.Match("甲乙") {
		switch h.PatternID {
		case 3:
			if h.Severity != 1 {
				t.Fatalf("severity 夹取错误：%d", h.Severity)
			}
		case 4:
			if h.Severity != 3 {
				t.Fatalf("severity 夹取错误：%d", h.Severity)
			}
		}
	}
}

func TestMatch_EdgeInputs(t *testing.T) {
	m := New([]Pattern{pattern(1, "毒品", 3)})
	if hits := m.Match(""); hits != nil {
		t.Fatalf("空文本应返回 nil，得 %+v", hits)
	}
	var nilMatcher *Matcher
	if hits := nilMatcher.Match("毒品"); hits != nil {
		t.Fatalf("nil Matcher 应返回 nil，得 %+v", hits)
	}
	if nilMatcher.Size() != 0 {
		t.Fatal("nil Matcher 的 Size 应为 0")
	}
	empty := New(nil)
	if hits := empty.Match("毒品"); hits != nil {
		t.Fatalf("空词库应返回 nil，得 %+v", hits)
	}
}

func TestMatch_ManyPatterns(t *testing.T) {
	// 规模用例：同时校验"与词库规模解耦"的实现没有被退化成逐词 Contains。
	patterns := make([]Pattern, 0, 2000)
	for i := 0; i < 2000; i++ {
		patterns = append(patterns, pattern(uint(i+1), fmt.Sprintf("违规词%04d", i), 2))
	}
	m := New(patterns)
	if m.Size() != 2000 {
		t.Fatalf("Size = %d，期望 2000", m.Size())
	}
	hits := m.Match("这里有违规词1234和违规词1999两个")
	if len(hits) != 2 {
		t.Fatalf("命中数 = %d，期望 2：%+v", len(hits), hits)
	}
}
