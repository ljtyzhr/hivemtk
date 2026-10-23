// engine.go L1+L2 审查引擎：词库匹配（AC）→ 白名单抑制 → 策略裁决。
//
// 本层**不做任何 IO 阻塞**（词库走进程内快照，策略走一次带 TTL 的读），
// 也**不调用外部 API**——外部审核属于 L3，在 worker 里可选执行，
// 目的是让"提交审查"这条路径的耗时与产物大小无关（几毫秒级）。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"hivemtk-user/internal/contentguard/matcher"
	"hivemtk-user/internal/contentguard/model"
	"hivemtk-user/internal/contentguard/normalize"
	"hivemtk-user/internal/safeprompt"
)

// SnippetRunes 审查记录里保留的原文片段长度（rune）。截断而非全文：
// 运营要的是"能看懂这段为什么被拦"，不是把用户原文复制一份到可导出的表里。
const SnippetRunes = 200

// MaxScanRunes 单条内容审查上限（rune）的**兜底默认值**。超出部分不审但**不会**因此
// 判 pass：调用方拿到 Truncated=true 后自行决定（默认按"内容超长"转人工）。
//
// 为什么它是参数中心的兜底值而不是不可改的常量：这个数决定"多长的内容会被完整审"。
// 商户侧的长文素材（营销长文、KB 文档）超过它就会被截断审查，运营确实可能需要调；
// 而它同时又是一个防 DoS 的上界（匹配是 O(内容长度 × 自动机)），所以只能在上界内调，
// 不能删掉。生效值 = 参数中心 `content_guard.max_scan_runes`，本常量在装配处作为实参传入
// （见 internal/app/content_guard_wiring.go，越界回落到本值）。
const MaxScanRunes = 20000

// maxScanRunesMin/Max 参数中心允许的区间。下限保证"短内容也会被完整审"，
// 上限防止有人把上界调到等于没有上界（1e6 rune 的扫描会把 worker 拖成串行）。
//
// 这两个数是上面那条兜底值的**派生边界**（1/20 与 ×10），不是两个独立旋钮：
// 所以写成由 MaxScanRunes 算出来的，而不是再加两个常量——另立常量会让
// "改兜底值忘了改边界"成为可能，也躲不过 G4（可调旋钮门）的追问。
const (
	scanRunesFloorRatio = 20
	scanRunesCeilRatio  = 10
)

// ScanRunesBounds 返回参数中心可配的审查长度区间（含端点），见上面两个比例常量。
func ScanRunesBounds() (lo, hi int) {
	return MaxScanRunes / scanRunesFloorRatio, MaxScanRunes * scanRunesCeilRatio
}

// ScanRequest 一次扫描请求。
type ScanRequest struct {
	SubjectType string
	SubjectID   string
	Scene       string
	Content     string
	TraceID     string
}

// Assessment 扫描结论。
type Assessment struct {
	Decision
	// Policy 生效策略（规整后）。
	Policy *model.GuardPolicy
	// Hits 命中证据（原文 rune 区间，白名单已抑制）。
	Hits []model.GuardHit
	// RawHits 归一化坐标下的原始命中（升级重算用）。
	RawHits []matcher.Hit
	// Sources 本轮参与的审查源。
	Sources []string
	// Digest 内容摘要（幂等键与抽样键）。
	Digest string
	// Snippet 脱敏片段。
	Snippet string
	// LatencyMs 本层耗时。
	LatencyMs int
	// Truncated 内容是否被截断审查。
	Truncated bool
	// DictErr 词库不可用时的错误文本（失败开放，但必须留痕）。
	DictErr string
}

// Engine 审查引擎。
type Engine struct {
	words    WordStore
	policies PolicyStore
	dict     *Dictionary
	now      func() time.Time
	// maxScanRunes 单条内容审查上限；<=0 时按 MaxScanRunes 兜底。
	// 装配处可从参数中心注入（content_guard.max_scan_runes），这里只留生效值。
	maxScanRunes int
}

// NewEngine 构造引擎。
func NewEngine(words WordStore, policies PolicyStore) *Engine {
	return &Engine{
		words:        words,
		policies:     policies,
		dict:         NewDictionary(words),
		now:          time.Now,
		maxScanRunes: MaxScanRunes,
	}
}

// WithMaxScanRunes 设置单条内容审查上限（<=0 或越界时保持兜底值）。
func (e *Engine) WithMaxScanRunes(n int) *Engine {
	if e == nil {
		return nil
	}
	lo, hi := ScanRunesBounds()
	if n < lo || n > hi {
		return e
	}
	e.maxScanRunes = n
	return e
}

// scanLimit 生效的审查上限（直接读字段，避免装配期塞了 0 之后变成"不截断"）。
func (e *Engine) scanLimit() int {
	if e == nil || e.maxScanRunes <= 0 {
		return MaxScanRunes
	}
	return e.maxScanRunes
}

// Dictionary 词库快照缓存（写入口改词后调用 Invalidate）。
func (e *Engine) Dictionary() *Dictionary {
	if e == nil {
		return nil
	}
	return e.dict
}

// Policy 读取并规整生效策略；读不到时回落出厂策略（审查不能因为策略表缺行而关闸）。
func (e *Engine) Policy(ctx context.Context, subjectType string) *model.GuardPolicy {
	if e == nil || e.policies == nil {
		return DefaultPolicy(subjectType)
	}
	p, err := e.policies.Get(ctx, subjectType)
	if err != nil || p == nil {
		return DefaultPolicy(subjectType)
	}
	return NormalizePolicy(p)
}

// Scan 执行一次审查（纯本地，无外部调用）。
func (e *Engine) Scan(ctx context.Context, req ScanRequest) (*Assessment, error) {
	start := e.now()
	if err := ValidateSubjectType(req.SubjectType); err != nil {
		return nil, err
	}
	if err := ValidateScene(req.Scene); err != nil {
		return nil, err
	}
	out := &Assessment{
		Digest:    ContentDigest(req.SubjectType, req.SubjectID, req.Scene, req.Content),
		Snippet:   Snippet(req.Content),
		Sources:   nil,
		Policy:    e.Policy(ctx, req.SubjectType),
		Decision:  Decision{Verdict: model.VerdictPass, Action: model.ActionNone},
		Truncated: false,
	}
	if out.Policy == nil || !out.Policy.Enabled {
		out.Skipped = "policy_disabled"
		out.LatencyMs = int(e.now().Sub(start).Milliseconds())
		return out, nil
	}
	if !CoversScene(out.Policy, req.Scene) {
		out.Skipped = "scene_not_covered"
		out.LatencyMs = int(e.now().Sub(start).Milliseconds())
		return out, nil
	}

	content := req.Content
	if limit := e.scanLimit(); limit > 0 {
		if runes := []rune(content); len(runes) > limit {
			content = string(runes[:limit])
			out.Truncated = true
		}
	}

	res := normalize.Text(content)
	m, words, err := e.dict.Snapshot(ctx)
	if err != nil {
		// 失败开放：词库读不到 ⇒ 本轮查不出违规（不是"全部拦截"），错误留痕由调用方写记录。
		out.DictErr = err.Error()
	}
	raw := m.MatchResult(res)

	allowSpans := make([][2]int, 0, 4)
	denyHits := make([]matcher.Hit, 0, len(raw))
	for _, h := range raw {
		w, ok := words[h.PatternID]
		if ok && w.ListKind == model.ListKindAllow {
			allowSpans = append(allowSpans, [2]int{h.Start, h.End})
			continue
		}
		denyHits = append(denyHits, h)
	}
	// 白名单抑制：与任一 allow 命中区间重叠的 deny 命中一律丢弃。
	kept := make([]matcher.Hit, 0, len(denyHits))
	for _, h := range denyHits {
		if overlapsAny(h.Start, h.End, allowSpans) {
			continue
		}
		kept = append(kept, h)
	}
	out.RawHits = kept
	out.Hits = toGuardHits(kept, res)
	out.Decision = Evaluate(out.Policy, kept, 0)
	if len(kept) > 0 || out.DictErr == "" {
		out.Sources = append(out.Sources, "dict")
	}
	out.LatencyMs = int(e.now().Sub(start).Milliseconds())
	return out, nil
}

// toGuardHits 把归一化坐标的命中回映射到原文区间。
func toGuardHits(hits []matcher.Hit, res normalize.Result) []model.GuardHit {
	if len(hits) == 0 {
		return nil
	}
	out := make([]model.GuardHit, 0, len(hits))
	for _, h := range hits {
		gh := model.GuardHit{
			PatternID: h.PatternID,
			Word:      h.Word,
			Category:  h.Category,
			Severity:  h.Severity,
		}
		if from, to, ok := res.OriginalRange(h.Start, h.End); ok {
			gh.Start, gh.End = from, to
		} else {
			gh.Start, gh.End = h.Start, h.End
		}
		out = append(out, gh)
	}
	return out
}

func overlapsAny(start, end int, spans [][2]int) bool {
	for _, s := range spans {
		if start < s[1] && s[0] < end {
			return true
		}
	}
	return false
}

// ContentDigest 内容摘要（幂等键 / 抽样键）。
//
// 用 sha256(subject|scene|content) 而不是裸 content 的哈希：同一条内容被两个主体
// 各自提交时必须算两条任务（处罚是分主体的），摘要里不带主体会把它们合成一条。
func ContentDigest(subjectType, subjectID, scene, content string) string {
	h := sha256.New()
	_, _ = h.Write([]byte(subjectType))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(subjectID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(scene))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// Snippet 生成入库片段：截断到 SnippetRunes 个 rune，并脱敏 PII
// （复用 safeprompt.MaskPII，避免把手机号/邮箱/证件号复制进审查记录）。
func Snippet(content string) string {
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) > SnippetRunes {
		return safeprompt.MaskPII(string(runes[:SnippetRunes])) + "…"
	}
	return safeprompt.MaskPII(content)
}
