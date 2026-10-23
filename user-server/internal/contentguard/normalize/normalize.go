// Package normalize 内容审查用的文本归一化（含量词/变形对抗处理）。
//
// 为什么要单独一层：敏感词命中率的大头不在词库大小，而在**变体**——
// 全角/半角、大小写、零宽字符、"敏 感 词"式的分隔符插入，都是 10 行代码就能绕过的
// 朴素子串匹配（`internal/safeprompt` 现在的实现就是朴素子串，见该包注释里
// "> 1k 词建议替换为 AC 自动机"的备忘）。归一化把文本与词条投到同一个坐标空间，
// 匹配器（matcher 包）就只需要处理一种形态。
//
// 设计约束（三条，缺一条就会引入新的误杀或证据失真）：
//  1. **索引可回映射**：归一化后的每个 rune 记住它在原文里的 rune 序号（Index）。
//     审查记录要给人看"命中的是原文哪一段"，回映射丢了这个就只能贴归一化文本——
//     而给运营看一段被抹掉标点的文本，等于让人自己猜原文。
//  2. **不做语义猜测**：只做确定性的字符级变换（宽度/大小写/去掉分隔符与零宽）。
//     繁→简、拼音、形近字这类需要大词表的变换留给外部审核 API（L3），
//     不在本地做——本地做不准的变换会把误杀率推高，而误杀的成本由业务方承担。
//  3. **变换可关闭**：词库里的词本身可能含分隔符（"7 天 无 理由"），
//     词条与文本必须用**同一套**选项归一化，否则两边坐标空间不一致，
//     表现为"词库里有这个敏感词但永远不命中"。
package normalize

import (
	"unicode"
)

// Options 归一化选项。
//
// 默认（零值）即生产口径：DropSeparators=true 是零值的反面，因此刻意用
// `KeepSeparators` 这种"反向开关"，让 WithOptions(s, Options{}) 与 Text(s) 等价——
// 零值不等于默认值是最常见的一类静默配置错误（改了一个字段，行为悄悄回到最松口径）。
type Options struct {
	// KeepSeparators 为 true 时保留空白/标点/符号；默认（false）丢弃。
	KeepSeparators bool
	// KeepWidth 为 true 时不折叠全角；默认（false）折叠为半角。
	KeepWidth bool
	// KeepCase 为 true 时不做大小写折叠；默认（false）折叠为小写。
	KeepCase bool
}

// Result 归一化结果。
type Result struct {
	// Text 归一化后的文本。
	Text string
	// Index 归一化文本第 i 个 rune 对应原文第 Index[i] 个 rune。
	// 长度恒等于 len([]rune(Text))；长度不匹配即为实现缺陷（测试会拦）。
	Index []int
}

// Text 用生产口径归一化（折叠宽度与大小写、丢弃分隔符与零宽字符）。
func Text(s string) Result {
	return WithOptions(s, Options{})
}

// WithOptions 按给定选项归一化。
func WithOptions(s string, opt Options) Result {
	if s == "" {
		return Result{Text: "", Index: nil}
	}
	src := []rune(s)
	out := make([]rune, 0, len(src))
	idx := make([]int, 0, len(src))

	for i, r := range src {
		if isZeroWidth(r) {
			continue
		}
		if !opt.KeepWidth {
			r = foldWidth(r)
		}
		if !opt.KeepSeparators && isSeparator(r) {
			continue
		}
		if !opt.KeepCase {
			r = unicode.ToLower(r)
		}
		out = append(out, r)
		idx = append(idx, i)
	}
	return Result{Text: string(out), Index: idx}
}

// Runes 返回归一化文本的 rune 切片（与 Result.Text 同源，避免调用方再转一次）。
func (r Result) Runes() []rune { return []rune(r.Text) }

// OriginalRange 把归一化文本上的 [start, end) rune 区间映射回原文的 rune 区间。
//
// 越界一律按夹取处理：调用方（审查记录）要的是"能展示的证据"，不是 panic。
// 返回 ok=false 表示区间为空或 Index 不可用（只可能在手工构造的 Result 上出现）。
func (r Result) OriginalRange(start, end int) (int, int, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(r.Index) {
		end = len(r.Index)
	}
	if start >= end || len(r.Index) == 0 {
		return 0, 0, false
	}
	from := r.Index[start]
	to := r.Index[end-1] + 1
	if to < from {
		return 0, 0, false
	}
	return from, to, true
}

// isZeroWidth 零宽/不可见字符：插入它们不改变肉眼可见文本，却能把任何词库打成失效。
func isZeroWidth(r rune) bool {
	switch r {
	case 0x200B, 0x200C, 0x200D, 0x200E, 0x200F, // ZWSP/ZWNJ/ZWJ/LRM/RLM
		0x2060, 0x2061, 0x2062, 0x2063, 0x2064, // word joiner 一族
		0xFEFF, // BOM
		0x00AD: // soft hyphen
		return true
	}
	return false
}

// isSeparator 空白/标点/符号。丢弃它们覆盖 "敏 感 词"、"敏*感*词"、"敏-感-词" 三类最常见规避。
func isSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	if unicode.IsPunct(r) || unicode.IsSymbol(r) {
		return true
	}
	// 全角空格与表格符不落在 IsSpace 的稳定子集里（口径随 Unicode 版本变），显式列出。
	switch r {
	case 0x3000, 0x00A0, 0x1680:
		return true
	}
	return false
}

// foldWidth 全角 ASCII（U+FF01–U+FF5E）与全角空格折叠为半角。
func foldWidth(r rune) rune {
	switch {
	case r == 0x3000:
		return ' '
	case r >= 0xFF01 && r <= 0xFF5E:
		return r - 0xFEE0
	}
	return r
}
