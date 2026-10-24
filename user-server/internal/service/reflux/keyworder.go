// Package reflux 低置信度问题回流管道（T1）。
//
// 设计文档：docs/architecture/design/T1-confidence-reflux.md
//
// 链路：触发源（低置信/转人工/坏例/CSAT 负反馈/未命中）→ collector 异步去重沉淀
// unknown_questions（有界队列，绝不阻塞问答主链路）→ clusterer 指纹+trgm 归簇
// → 运营审核（controller 层）→ adopter 写回 faq_entries（关键词去泛化硬校验）。
package reflux

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

// NormalizeQuestion 问句规范化：去首尾空白 + 全角转半角 + 去标点 + 小写。
// 指纹稳定性的前提：等价问句必须得到同一 normalize 结果。
func NormalizeQuestion(q string) string {
	q = strings.TrimSpace(q)
	// 第一遍：全角 ASCII 区转半角（含全角标点；FF01 全角'!'→'!'后由第二遍去标点）
	var b strings.Builder
	for _, r := range q {
		if r >= 0xFF01 && r <= 0xFF5E {
			b.WriteRune(r - 0xFEE0)
		} else {
			b.WriteRune(r)
		}
	}
	q = b.String()
	// 第二遍：去标点/符号/空白 + 小写
	b.Reset()
	for _, r := range q {
		switch {
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// QuestionHash 规范化后取 sha256 十六进制指纹
func QuestionHash(q string) string {
	sum := sha256.Sum256([]byte(NormalizeQuestion(q)))
	return hex.EncodeToString(sum[:])
}

var (
	spaceRe = regexp.MustCompile(`\s+`)
	// generalizedStopwords 泛化停用词（历史教训：FAQ 关键词含泛化子串会大面积误命中）
	generalizedStopwords = map[string]bool{
		"怎么": true, "什么": true, "如何": true, "为什么": true, "为啥": true,
		"怎么办": true, "怎么办呢": true, "怎么回事": true, "怎么回事啊": true,
		"请问": true, "帮忙": true, "帮助": true, "可以吗": true, "能吗": true,
		"的": true, "吗": true, "了": true, "呢": true, "吧": true, "啊": true,
		"我": true, "你": true, "他": true, "她": true, "它": true, "我们": true,
		"一个": true, "一下": true, "一些": true, "这个": true, "那个": true,
		"问题": true, "东西": true, "事情": true, "情况": true, "时候": true,
		"价格": true, "多少钱": true, // 跨品类泛词——"多少钱"不能单独当关键词
	}
)

// GenerateKeywords 从问句生成候选关键词（去停用词、剔短词元）。
// 这是 adopter 的第一层；黑名单/区分性校验在 ValidateKeywords。
func GenerateKeywords(question string) []string {
	normalized := NormalizeQuestion(question)
	// 贪心切词：优先 2~4 字词元（无分词依赖的最小可用方案；租户侧接入分词器后可替换）
	seen := map[string]bool{}
	var out []string
	runes := []rune(normalized)
	for size := 4; size >= 2; size-- {
		for i := 0; i+size <= len(runes); i++ {
			w := string(runes[i : i+size])
			if generalizedStopwords[w] || seen[w] {
				continue
			}
			// 词元必须是某个"完整词"——简化为不含该词的更长候选已收录（长词优先天然满足）
			seen[w] = true
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return out
	}
	return spaceRe.Split(strings.Join(out, " "), -1)
}

// ValidateKeywords 关键词去泛化硬校验（adopter 采纳前必过）。
// 返回 error 文案直接给运营看。
func ValidateKeywords(keywords []string, existingFaqKeywords map[string]int) error {
	kept := make([]string, 0, len(keywords))
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if k == "" || generalizedStopwords[k] {
			continue
		}
		if len([]rune(k)) < 2 {
			continue
		}
		// 词元主体是泛化词的也剔除（如"怎么办"含"怎么办"整体、"么办"是"怎么办"的残片——
		// 残片判定：词元去掉泛化停用词后剩余部分 < 1 字视为无实义）
		remainder := stripStopwords(k)
		if len([]rune(remainder)) < 2 {
			continue
		}
		kept = append(kept, k)
	}
	if len(kept) < 2 {
		return errTooGeneral
	}
	// 区分性词元：至少一个关键词在既有 FAQ 关键词全集中未出现过
	hasDistinctive := false
	for _, k := range kept {
		if existingFaqKeywords[k] == 0 {
			hasDistinctive = true
			break
		}
	}
	if !hasDistinctive {
		return errNotDistinctive
	}
	return nil
}

// KeywordError 关键词校验失败
type KeywordError struct{ msg string }

func (e *KeywordError) Error() string { return e.msg }

var (
	errTooGeneral     = &KeywordError{msg: "关键词过于泛化，请人工补充专属关键词"}
	errNotDistinctive = &KeywordError{msg: "关键词与既有 FAQ 完全同键，请补充区分性关键词"}
)

// stripStopwords 把词元中的泛化停用词片段剥掉，看还剩多少实义部分
func stripStopwords(k string) string {
	prev := ""
	for prev != k {
		prev = k
		for w := range generalizedStopwords {
			k = strings.ReplaceAll(k, w, "")
		}
	}
	return strings.TrimSpace(k)
}
