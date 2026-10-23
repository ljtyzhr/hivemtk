// Package matcher Aho–Corasick 多模式匹配器（内容审查 L1 引擎的匹配内核）。
//
// 为什么是 AC 而不是逐词 strings.Contains（现状，见 internal/safeprompt）：
//   - 复杂度：N 词 × M 长度 ⇒ O(N·M)；AC ⇒ O(M + 命中数)，与词库规模解耦。
//     词库 1 万条、单条消息 200 字时，朴素实现是 200 万次子串比较，
//     而审查是每条消息都要做的事（聊天场景 QPS 与消息量同阶）。
//   - 只扫描一遍就能拿到**全部**命中位置，命中位置是审查证据与"白名单抑制"的前提；
//     朴素实现里"先找到第一个就返回"的写法拿不到位置，也就没法做局部抑制。
//
// 线程安全：构建完成后完全只读，可被多个 worker 并发 Match（快照整体替换，
// 见 service.Dictionary——热更新走"编译新快照再原子替换"，不在原快照上改）。
package matcher

import (
	"sort"

	"hivemtk-user/internal/contentguard/normalize"
)

// Pattern 一条待匹配模式（已由调用方给出原始词，本包负责归一化）。
type Pattern struct {
	// ID 调用方侧的稳定标识（用词库主键，便于回查命中词条）。
	ID uint
	// Word 原始词（展示用）。
	Word string
	// Category 分类（涉政/涉黄/广告/辱骂/自定义…），随命中回传。
	Category string
	// Severity 严重级别 1..3（3 = 直接拦截）。越界按 1/3 夹取。
	Severity int
}

// Hit 命中。
type Hit struct {
	PatternID uint
	Word      string
	Category  string
	Severity  int
	// Start/End 命中区间在**归一化文本**上的 rune 偏移，[Start, End)。
	// 回映射到原文见 normalize.Result.OriginalRange。
	Start int
	End   int
}

type node struct {
	next map[rune]int
	fail int
	out  []int // 在此节点结束的模式下标（patterns 切片下标）
}

// Matcher 编译后的多模式匹配器。零值不可用，必须经 New 构建。
type Matcher struct {
	nodes    []node
	patterns []Pattern
	lens     []int // 与 patterns 等长：归一化后的 rune 长度（避免扫描期重复归一化）
}

// New 构建匹配器。
//
// 归一化词的规则必须与**被扫描文本**一致（normalize.Text，即生产口径）：
// 词条里含分隔符（"7 天 无 理由"）时，文本里也可能含，两边都丢分隔符才对得上。
// 归一化后为空的词（纯标点）直接跳过——保留它只会匹配空串。
func New(patterns []Pattern) *Matcher {
	m := &Matcher{
		nodes:    []node{{next: make(map[rune]int)}},
		patterns: make([]Pattern, 0, len(patterns)),
		lens:     make([]int, 0, len(patterns)),
	}
	for _, p := range patterns {
		key := []rune(normalize.Text(p.Word).Text)
		if len(key) == 0 {
			continue
		}
		if p.Severity <= 0 {
			p.Severity = 1
		}
		if p.Severity > 3 {
			p.Severity = 3
		}
		m.patterns = append(m.patterns, p)
		m.lens = append(m.lens, len(key))
		pidx := len(m.patterns) - 1

		cur := 0
		for _, r := range key {
			nxt, ok := m.nodes[cur].next[r]
			if !ok {
				m.nodes = append(m.nodes, node{next: make(map[rune]int)})
				nxt = len(m.nodes) - 1
				m.nodes[cur].next[r] = nxt
			}
			cur = nxt
		}
		m.nodes[cur].out = append(m.nodes[cur].out, pidx)
	}
	m.buildFail()
	return m
}

// buildFail BFS 构造失配指针，并把 fail 链上的输出并入本节点（AC 的标准做法：
// 归并后扫描期在任一状态停在原地就能拿到全部命中，不必回溯 fail 链）。
func (m *Matcher) buildFail() {
	queue := make([]int, 0, len(m.nodes))
	for _, child := range m.nodes[0].next {
		m.nodes[child].fail = 0
		queue = append(queue, child)
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for r, child := range m.nodes[cur].next {
			f := m.nodes[cur].fail
			for f != 0 {
				if nxt, ok := m.nodes[f].next[r]; ok {
					f = nxt
					break
				}
				f = m.nodes[f].fail
			}
			if f == 0 {
				if nxt, ok := m.nodes[0].next[r]; ok && nxt != child {
					f = nxt
				}
			}
			m.nodes[child].fail = f
			if len(m.nodes[f].out) > 0 {
				m.nodes[child].out = append(m.nodes[child].out, m.nodes[f].out...)
			}
			queue = append(queue, child)
		}
	}
}

// Size 已编译模式数（归一化后为空的词不计入）。
func (m *Matcher) Size() int {
	if m == nil {
		return 0
	}
	return len(m.patterns)
}

// Match 扫描原始文本（内部做归一化），返回全部命中。
//
// 返回顺序：按命中次序号（结束位置递增）稳定排序后返回；不去重、不择优——
// "该不该只报最长的那个"是策略问题，不是匹配问题。
func (m *Matcher) Match(text string) []Hit {
	if m == nil || m.Size() == 0 || text == "" {
		return nil
	}
	return m.MatchResult(normalize.Text(text))
}

// MatchResult 扫描已归一化的结果（同一段文本要多次复用时避免重复归一化）。
func (m *Matcher) MatchResult(res normalize.Result) []Hit {
	if m == nil || m.Size() == 0 || len(res.Text) == 0 {
		return nil
	}
	runes := res.Runes()
	hits := make([]Hit, 0, 4)
	state := 0
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		for {
			if nxt, ok := m.nodes[state].next[r]; ok {
				state = nxt
				break
			}
			if state == 0 {
				break
			}
			state = m.nodes[state].fail
		}
		for _, pidx := range m.nodes[state].out {
			p := m.patterns[pidx]
			start := i - m.lens[pidx] + 1
			if start < 0 {
				continue
			}
			hits = append(hits, Hit{
				PatternID: p.ID,
				Word:      p.Word,
				Category:  p.Category,
				Severity:  p.Severity,
				Start:     start,
				End:       i + 1,
			})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].End != hits[j].End {
			return hits[i].End < hits[j].End
		}
		return hits[i].PatternID < hits[j].PatternID
	})
	return hits
}
