// engine_test.go L1+L2 引擎（词库匹配 → 白名单抑制 → 策略裁决）的行为断言。
//
// 引擎层必须做到"零外部依赖也能测"：词库与策略都走 Store 接口，
// 因此这两组假实现就是全部测试夹具（不需要数据库）。
package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hivemtk-user/internal/contentguard/model"
)

// ── 夹具 ───────────────────────────────────────────────────────────────

type stubWordStore struct {
	words   []model.GuardWord
	err     error
	reloads int
}

func (s *stubWordStore) ListEnabled(context.Context) ([]model.GuardWord, error) {
	s.reloads++
	if s.err != nil {
		return nil, s.err
	}
	out := make([]model.GuardWord, 0, len(s.words))
	for _, w := range s.words {
		if w.Enabled {
			out = append(out, w)
		}
	}
	return out, nil
}

func (s *stubWordStore) List(context.Context, WordQuery) ([]model.GuardWord, int64, error) {
	return s.words, int64(len(s.words)), nil
}
func (s *stubWordStore) Get(context.Context, uint) (*model.GuardWord, error) { return nil, nil }
func (s *stubWordStore) Upsert(_ context.Context, w *model.GuardWord) (*model.GuardWord, error) {
	return w, nil
}
func (s *stubWordStore) BulkUpsert(context.Context, []model.GuardWord) (int, error) { return 0, nil }
func (s *stubWordStore) Delete(context.Context, uint) error                         { return nil }

type stubPolicyStore struct {
	policy *model.GuardPolicy
	err    error
}

func (s *stubPolicyStore) Get(context.Context, string) (*model.GuardPolicy, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.policy, nil
}
func (s *stubPolicyStore) List(context.Context) ([]model.GuardPolicy, error) {
	if s.policy == nil {
		return nil, nil
	}
	return []model.GuardPolicy{*s.policy}, nil
}
func (s *stubPolicyStore) Save(_ context.Context, p *model.GuardPolicy) (*model.GuardPolicy, error) {
	return p, nil
}

func denyWord(id uint, word string, sev int) model.GuardWord {
	return model.GuardWord{ID: id, ListKind: model.ListKindDeny, Word: word, Category: "test", Severity: sev, Enabled: true}
}

func allowWord(id uint, word string) model.GuardWord {
	return model.GuardWord{ID: id, ListKind: model.ListKindAllow, Word: word, Category: "white", Severity: 1, Enabled: true}
}

func newTestEngine(words ...model.GuardWord) *Engine {
	return NewEngine(&stubWordStore{words: words}, &stubPolicyStore{})
}

// ── 用例 ───────────────────────────────────────────────────────────────

func TestEngineScan_UserBlockWithMappedEvidence(t *testing.T) {
	e := newTestEngine(denyWord(11, "毒品", 3))
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser,
		SubjectID:   "u_1",
		Scene:       model.SceneInboundMessage,
		Content:     "我要买 毒 品",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictBlock || got.Action != model.ActionReject {
		t.Fatalf("裁决 = {%s,%s}，期望 block / reject", got.Verdict, got.Action)
	}
	if got.FreezeLevel != model.SanctionLevelTemp {
		t.Fatalf("首次拦截应给临时冻结档，得 %q", got.FreezeLevel)
	}
	if len(got.Hits) != 1 {
		t.Fatalf("命中证据数 = %d，期望 1：%+v", len(got.Hits), got.Hits)
	}
	h := got.Hits[0]
	// 证据区间必须是**原文**区间（含被规避插入的空格），否则运营看不懂"为什么拦"。
	src := []rune("我要买 毒 品")
	if h.Start != 4 || h.End != 7 {
		t.Fatalf("证据区间 = [%d,%d)，期望 [4,7)：命中 %q", h.Start, h.End, string(src[h.Start:h.End]))
	}
	if strings.TrimSpace(string(src[h.Start:h.End])) != "毒 品" {
		t.Fatalf("证据切片 = %q", string(src[h.Start:h.End]))
	}
	if len(got.Sources) != 1 || got.Sources[0] != "dict" {
		t.Fatalf("sources = %v，期望 [dict]", got.Sources)
	}
}

func TestEngineScan_AllowListSuppressesOverlappingDeny(t *testing.T) {
	e := newTestEngine(denyWord(1, "毒品", 3), allowWord(2, "毒品交易"))
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "毒品交易是犯罪",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictPass {
		t.Fatalf("白名单未抑制误杀：裁决 = %s，命中 = %+v", got.Verdict, got.Hits)
	}
	if len(got.Hits) != 0 {
		t.Fatalf("被抑制的命中不得进证据：%+v", got.Hits)
	}

	// 同一词库下，白名单没覆盖的用法仍须命中（抑制只针对重叠区间，不是整条放行）。
	got2, _ := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_2",
		Scene: model.SceneInboundMessage, Content: "这里能买到毒品",
	})
	if got2.Verdict != model.VerdictBlock {
		t.Fatalf("非白名单语境应拦截，得 %s", got2.Verdict)
	}
}

func TestEngineScan_SceneNotCoveredSkips(t *testing.T) {
	e := newTestEngine(denyWord(1, "违法", 3))
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneMaterial, Content: "违法内容",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictPass || got.Skipped != "scene_not_covered" {
		t.Fatalf("用户策略不覆盖素材场景，得 {%s, skipped=%q}", got.Verdict, got.Skipped)
	}
	if len(got.Hits) != 0 {
		t.Fatal("未审查的场景不应产生命中")
	}
}

func TestEngineScan_MerchantUsesItsOwnThresholds(t *testing.T) {
	e := newTestEngine(denyWord(1, "最便宜", 2))
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectMerchant, SubjectID: "t_9",
		Scene: model.SceneMarketingCopy, Content: "全网最便宜，错过等一年",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	// 同一级别（sev2）在 user 侧只转人工，在 merchant 侧必须直接拦：这就是"不同规则"的含义。
	if got.Verdict != model.VerdictBlock || got.Action != model.ActionBlockPublish {
		t.Fatalf("merchant sev2 裁决 = {%s,%s}，期望 block / block_publish", got.Verdict, got.Action)
	}
	if got.FreezeLevel != model.SanctionLevelTemp {
		t.Fatalf("merchant 首次拦截冻结档 = %q，期望 temp", got.FreezeLevel)
	}
}

func TestEngineScan_ReviewVerdict(t *testing.T) {
	e := newTestEngine(denyWord(1, "有点黄", 2))
	got, _ := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "这段话有点黄",
	})
	if got.Verdict != model.VerdictReview || got.Action != model.ActionReview {
		t.Fatalf("user sev2 应为 review，得 {%s,%s}", got.Verdict, got.Action)
	}
	if got.FreezeLevel != "" {
		t.Fatalf("转人工不应同时施加冻结，得 %q", got.FreezeLevel)
	}
}

func TestEngineScan_DisabledPolicySkips(t *testing.T) {
	e := NewEngine(&stubWordStore{}, &stubPolicyStore{policy: &model.GuardPolicy{
		SubjectType: model.SubjectUser, Enabled: false, Scenes: model.SceneInboundMessage,
		BlockSeverity: 3, ReviewSeverity: 2,
	}})
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "毒品",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictPass || got.Skipped != "policy_disabled" {
		t.Fatalf("关闸后应 pass + skipped，得 {%s,%q}", got.Verdict, got.Skipped)
	}
}

func TestEngineScan_PolicyReadFailureFallsBackToFactoryPolicy(t *testing.T) {
	// 策略表读不到 ≠ 关闸：审查不能因为一次库抖动变成"全部放行"。
	e := NewEngine(&stubWordStore{words: []model.GuardWord{denyWord(1, "毒品", 3)}},
		&stubPolicyStore{err: errors.New("db down")})
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "毒品",
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictBlock {
		t.Fatalf("策略读失败应回落出厂策略（sev3 拦），得 %s", got.Verdict)
	}
	if got.Policy == nil || got.Policy.BlockSeverity != 3 {
		t.Fatalf("回落策略不正确：%+v", got.Policy)
	}
}

func TestEngineScan_DictionaryFailureIsFailOpenAndRecorded(t *testing.T) {
	// 词库读失败的正确后果是"这轮查不出违规"，不是"全部拦截"：
	// 拦全量用户比漏审是更大的事故。但必须留痕（sources 缺 dict）。
	e := NewEngine(&stubWordStore{err: errors.New("word table unavailable")}, &stubPolicyStore{})
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "毒品",
	})
	if err != nil {
		t.Fatalf("词库失败不应让 Scan 报错：%v", err)
	}
	if got.Verdict != model.VerdictPass {
		t.Fatalf("词库失败必须失败开放（pass），得 %s", got.Verdict)
	}
	if got.DictErr == "" {
		t.Fatal("词库失败必须留痕（DictErr 为空会让排障无从下手）")
	}
	if len(got.Sources) != 0 {
		t.Fatalf("词库未参与时 sources 不得写 dict：%v", got.Sources)
	}
}

func TestEngineScan_DisabledWordIgnoredAndSnapshotRefresh(t *testing.T) {
	words := &stubWordStore{words: []model.GuardWord{denyWord(1, "毒品", 3)}}
	e := NewEngine(words, &stubPolicyStore{})
	req := ScanRequest{SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: "毒品"}

	if got, _ := e.Scan(context.Background(), req); got.Verdict != model.VerdictBlock {
		t.Fatalf("首次应拦，得 %s", got.Verdict)
	}

	// 停用词条后必须能立刻生效（写入口调用 Invalidate，不能等 TTL）。
	words.words[0].Enabled = false
	e.Dictionary().Invalidate(1)
	if got, _ := e.Scan(context.Background(), req); got.Verdict != model.VerdictPass {
		t.Fatalf("停用后应放行，得 %s（词库快照没有失效？）", got.Verdict)
	}

	// 关闭失效路径后靠 TTL 兜底：TTL 归零等价于"每次重建"。
	words.words[0].Enabled = true
	e.Dictionary().InvalidateAll()
	e.Dictionary().SetTTL(1)
	if got, _ := e.Scan(context.Background(), req); got.Verdict != model.VerdictBlock {
		t.Fatalf("重新启用后应拦，得 %s", got.Verdict)
	}
}

// TestEngineScan_MaxScanRunesInjectedByParam 审查长度上限可从装配处注入（参数中心
// content_guard.max_scan_runes 的落地点），且越界值必须回落到兜底常量。
//
// 为什么这条要有测试：注入点走的是 `WithMaxScanRunes`，一旦它静默接受 0 或负值，
// 生效上限会变成"不截断"——长文整段进 AC 匹配，DoS 上界就没了，而现象只是"慢"。
func TestEngineScan_MaxScanRunesInjectedByParam(t *testing.T) {
	lo, hi := ScanRunesBounds()
	if lo >= hi || lo <= 0 {
		t.Fatalf("可用区间必须非空且为正：lo=%d hi=%d", lo, hi)
	}
	if lo != MaxScanRunes/20 || hi != MaxScanRunes*10 {
		t.Fatalf("区间应随兜底值派生：lo=%d hi=%d MaxScanRunes=%d", lo, hi, MaxScanRunes)
	}

	// 合法值：按注入的上限截断（这里直接把上限压到区间下限，内容比它长即应截断）。
	e := NewEngine(&stubWordStore{}, &stubPolicyStore{policy: &model.GuardPolicy{
		SubjectType: model.SubjectUser, Enabled: true,
		Scenes: model.SceneInboundMessage, BlockSeverity: 3, ReviewSeverity: 2,
	}}).WithMaxScanRunes(lo)

	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: strings.Repeat("安", lo+1),
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if !got.Truncated {
		t.Fatalf("注入上限 %d 后，长度 %d 的内容必须被标记截断", lo, lo+1)
	}

	// 正好等于上限：不算截断（边界不许"多截一个字符"，否则上限语义含糊）。
	got, err = e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: strings.Repeat("安", lo),
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if got.Truncated {
		t.Fatalf("长度恰好等于上限 %d 时不应标记截断", lo)
	}

	// 越界/非法值：一律保持兜底常量，不允许"设 0 等于不截断"。
	for _, bad := range []int{0, -1, lo - 1, hi + 1} {
		e := NewEngine(&stubWordStore{}, &stubPolicyStore{})
		if got := e.WithMaxScanRunes(bad); got.scanLimit() != MaxScanRunes {
			t.Fatalf("非法上限 %d 应回落到兜底 %d，实际生效 %d", bad, MaxScanRunes, got.scanLimit())
		}
	}
}

func TestEngineScan_TruncationAndSnippet(t *testing.T) {
	e := newTestEngine(denyWord(1, "毒品", 3))

	long := strings.Repeat("安", MaxScanRunes+50)
	got, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, SubjectID: "u_1",
		Scene: model.SceneInboundMessage, Content: long,
	})
	if err != nil {
		t.Fatalf("Scan 报错：%v", err)
	}
	if !got.Truncated {
		t.Fatal("超长内容必须标记 Truncated（否则调用方会以为全文都审过了）")
	}

	snip := Snippet("联系我 13800138000 领取")
	if strings.Contains(snip, "13800138000") {
		t.Fatalf("入库片段必须脱敏 PII，得 %q", snip)
	}
	if !strings.Contains(snip, "[REDACTED:PII]") {
		t.Fatalf("脱敏占位符缺失：%q", snip)
	}

	// 超过片段上限时截断并加省略号（不把用户原文整段搬进可导出的表）。
	longSnippet := Snippet(strings.Repeat("a", SnippetRunes+10))
	if !strings.HasSuffix(longSnippet, "…") {
		t.Fatalf("超长片段应以省略号结尾：%q", longSnippet)
	}
	if len([]rune(longSnippet)) != SnippetRunes+1 {
		t.Fatalf("片段长度 = %d，期望 %d", len([]rune(longSnippet)), SnippetRunes+1)
	}
	if Snippet("") != "" {
		t.Fatal("空内容片段应为空")
	}
}

func TestContentDigest_IsSubjectScoped(t *testing.T) {
	const content = "同一句话"
	a := ContentDigest(model.SubjectUser, "u_1", model.SceneInboundMessage, content)
	b := ContentDigest(model.SubjectUser, "u_1", model.SceneInboundMessage, content)
	if a != b {
		t.Fatal("摘要必须确定性（幂等键依赖它）")
	}
	if a == ContentDigest(model.SubjectUser, "u_2", model.SceneInboundMessage, content) {
		t.Fatal("不同主体的同一内容必须算出不同摘要（处罚是分主体的）")
	}
	if a == ContentDigest(model.SubjectMerchant, "u_1", model.SceneInboundMessage, content) {
		t.Fatal("不同主体类型必须算出不同摘要")
	}
	if a == ContentDigest(model.SubjectUser, "u_1", model.SceneMaterial, content) {
		t.Fatal("不同场景必须算出不同摘要")
	}
	if len(a) != 64 {
		t.Fatalf("摘要长度 = %d，期望 64（sha256 hex）", len(a))
	}
}

func TestEngineScan_InvalidInput(t *testing.T) {
	e := newTestEngine()
	if _, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: "tenant", Scene: model.SceneInboundMessage, Content: "x",
	}); err == nil {
		t.Fatal("未知主体类型必须报错")
	}
	if _, err := e.Scan(context.Background(), ScanRequest{
		SubjectType: model.SubjectUser, Scene: "chat", Content: "x",
	}); err == nil {
		t.Fatal("未知场景必须报错")
	}
}

func TestEngineNilSafety(t *testing.T) {
	var e *Engine
	if e.Dictionary() != nil {
		t.Fatal("nil Engine 的 Dictionary 应为 nil")
	}
	if p := e.Policy(context.Background(), model.SubjectUser); p == nil || p.BlockSeverity != 3 {
		t.Fatalf("nil Engine 应回落出厂策略，得 %+v", p)
	}
}
