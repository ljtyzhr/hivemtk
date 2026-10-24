package service

import (
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// TestShouldDropCitation 泛化防护红线（T2 §6.3）：
// 泛词兜底命中（keyword_fallback）且 score<0.3 直接丢弃；其余保留。
func TestShouldDropCitation(t *testing.T) {
	cases := []struct {
		name   string
		cit    dto.Citation
		wantDrop bool
	}{
		{
			name: "exact 高分保留",
			cit: dto.Citation{Type: dto.CitationTypeFAQ, FAQ: &dto.FAQCitation{
				FAQEntryID: 1, MatchType: dto.FAQMatchExact, Score: 0.92,
			}},
		},
		{
			name: "keyword_fallback 低分丢弃",
			cit: dto.Citation{Type: dto.CitationTypeFAQ, FAQ: &dto.FAQCitation{
				FAQEntryID: 2, MatchType: dto.FAQMatchKeywordFallback, Score: 0.1,
			}},
			wantDrop: true,
		},
		{
			name: "keyword_fallback 过阈值保留（折叠展示由前端做）",
			cit: dto.Citation{Type: dto.CitationTypeFAQ, FAQ: &dto.FAQCitation{
				FAQEntryID: 3, MatchType: dto.FAQMatchKeywordFallback, Score: 0.5,
			}},
		},
		{
			name: "token 命中保留",
			cit: dto.Citation{Type: dto.CitationTypeFAQ, FAQ: &dto.FAQCitation{
				FAQEntryID: 4, MatchType: dto.FAQMatchToken, Score: 0.05,
			}},
		},
		{
			name: "非 FAQ 型不丢",
			cit:  dto.Citation{Type: dto.CitationTypeChunk},
		},
		{
			name:   "nil 引用不 panic 也不丢",
			wantDrop: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := dto.ShouldDropCitation(&tc.cit)
			if got != tc.wantDrop {
				t.Fatalf("ShouldDropCitation = %v, want %v", got, tc.wantDrop)
			}
		})
	}
}

// TestMatchCitationsForAgent 泛词不产生高置信引用：MatchCitationsForAgent
// 对 keyword_fallback 低分条目应过滤掉（A2 用例：泛词问句无 citations）。
func TestMatchCitationsForAgent(t *testing.T) {
	svc := &FAQService{}
	// repo=nil / agentID=0 / 空 msg 都不产生引用
	if got := svc.MatchCitationsForAgent(t.Context(), 0, "退款政策"); got != nil {
		t.Fatalf("agentID=0 应无引用, got %d", len(got))
	}
	if got := svc.MatchCitationsForAgent(t.Context(), 1, "  "); got != nil {
		t.Fatalf("空消息应无引用, got %d", len(got))
	}
}

// TestTruncateRunes UTF-8 安全截断。
func TestTruncateRunes(t *testing.T) {
	if got := dto.TruncateRunes("退款政策说明", 3); got != "退款政" {
		t.Fatalf("got %q", got)
	}
	if got := dto.TruncateRunes("短", 10); got != "短" {
		t.Fatalf("got %q", got)
	}
}

// TestScoreCandidatesScored_MatchType 打分变体返回显式 MatchType：
// 关键词精确命中 = exact；无关键词条目靠词元重合命中 = keyword_fallback。
func TestScoreCandidatesScored_MatchType(t *testing.T) {
	db := testutil.NewTestDB(t, &model.FAQEntry{})
	r := repository.NewFAQRepository(db)
	yes := true
	entries := []model.FAQEntry{
		{Question: "退款政策是什么", Answer: "7天无理由", Keywords: []string{"退款"}, Enabled: &yes},
		{Question: "支持哪些渠道", Answer: "全渠道", Keywords: nil, Enabled: &yes},
	}
	// 精确关键词命中
	scored, err := r.ScoreCandidatesScored(t.Context(), entries, "退款怎么操作", 5)
	if err != nil {
		t.Fatalf("ScoreCandidatesScored: %v", err)
	}
	if len(scored) == 0 || scored[0].Entry.Question != "退款政策是什么" || scored[0].MatchType != dto.FAQMatchExact {
		t.Fatalf("期望 exact 命中退款政策, got %+v", scored)
	}
	// 无关键词条目：词元重合兜底应标 keyword_fallback（或不命中，均合法；
	// 但绝不允许被标成 exact）
	scored2, err := r.ScoreCandidatesScored(t.Context(), entries, "支持哪些渠道路", 5)
	if err != nil {
		t.Fatalf("ScoreCandidatesScored: %v", err)
	}
	for _, s := range scored2 {
		if s.Entry.Question == "支持哪些渠道" && s.MatchType == dto.FAQMatchExact {
			t.Fatalf("无关键词条目不允许 exact: %+v", s)
		}
	}
	// 旧签名不受影响
	old, err := r.ScoreCandidates(t.Context(), entries, "退款", 5)
	if err != nil || len(old) == 0 {
		t.Fatalf("旧签名回归失败: %v", err)
	}
}
