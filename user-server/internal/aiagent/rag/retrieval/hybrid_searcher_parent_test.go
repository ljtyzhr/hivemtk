package ragretrieval

// hybrid_searcher_parent_test.go 父子分段的检索侧回拼（P1-1 前置小刀）。
//
// 五格：
//  1. 命中子块带 parent_chunk_id 且父块存在 → 返回 Content 换成父块（分数/ID 不动）；
//  2. 父块行缺失（悬空引用）→ 该条原样返回；
//  3. 父块超字符上限 → 该条原样返回（宁少给上下文，不挤掉预算）；
//  4. 无父块引用（历史数据/短文档）→ 整批逐字节一致；
//  5. db 未装配（裸构造）→ 整批不动，且不 panic。
//
// 走 SearchIndex 全链路而不是只测 attachParentContext：回拼点在 finalK 截断之后，
// 只测内部函数会漏掉「调用点没接上」这类缺陷。

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/knowledge/model"
)

// fakeKwSearcher 关键词假实现：恒返回给定条目（embedding client 为 nil ⇒ 向量腿自动跳过）。
type fakeKwSearcher struct{ results []Chunk }

func (f *fakeKwSearcher) SearchKeyword(_ context.Context, _ string, _ string, _ int) ([]Chunk, error) {
	return f.results, nil
}

func newSearcherForParentTest(t *testing.T, hits []Chunk) *HybridSearcher {
	t.Helper()
	db := setupHybridTestDB(t)
	if err := db.AutoMigrate(&model.KnowledgeChunkParent{}); err != nil {
		t.Fatalf("建父块表失败: %v", err)
	}
	h := NewHybridSearcherWithInterfaces(nil, &fakeKwSearcher{results: hits}, nil, 0.7, 0.3)
	h.db = db
	return h
}

func TestSearchIndex_StitchesParentContext(t *testing.T) {
	dbHit := func() []Chunk {
		return []Chunk{{ID: "1", DocumentID: "9", Content: "子块内容：年付 4999", Score: 0.9}}
	}
	h := newSearcherForParentTest(t, dbHit())
	parent := model.KnowledgeChunkParent{DocumentID: 9, ProductID: "p", Content: "父块上下文：完整段落，含年付 4999 与 12 个月。", ChildCount: 3, CharCount: 30}
	if err := h.db.Create(&parent).Error; err != nil {
		t.Fatalf("写父块失败: %v", err)
	}
	h.keywordSearcher = &fakeKwSearcher{results: []Chunk{{
		ID: "1", DocumentID: "9", Content: "子块内容：年付 4999", Score: 0.9, ParentChunkID: parent.ID,
	}}}

	out, err := h.SearchIndex(context.Background(), "p", "年付", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("结果数 = %d want 1", len(out))
	}
	if out[0].Content != parent.Content {
		t.Errorf("Content = %q want 父块 %q", out[0].Content, parent.Content)
	}
	if out[0].ID != "1" || out[0].Score == 0 {
		t.Errorf("回拼不得改 ID/分数：%+v", out[0])
	}
}

func TestSearchIndex_NoParentKeepsBehavior(t *testing.T) {
	hits := []Chunk{
		{ID: "1", DocumentID: "9", Content: "甲", Score: 0.9},
		{ID: "2", DocumentID: "9", Content: "乙", Score: 0.8},
	}
	h := newSearcherForParentTest(t, hits)

	out, err := h.SearchIndex(context.Background(), "p", "甲", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("结果数 = %d want 2", len(out))
	}
	for i, want := range hits {
		if out[i].Content != want.Content || out[i].ID != want.ID {
			t.Errorf("无父块时第 %d 条被改动：%+v want %+v", i, out[i], want)
		}
	}
}

func TestSearchIndex_DanglingParentKeepsHit(t *testing.T) {
	h := newSearcherForParentTest(t, []Chunk{{
		ID: "1", DocumentID: "9", Content: "子块原文", Score: 0.9, ParentChunkID: 424242, // 不存在的父块
	}})
	out, err := h.SearchIndex(context.Background(), "p", "子块", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(out) != 1 || out[0].Content != "子块原文" {
		t.Errorf("悬空父块引用应原样返回，实际 %+v", out)
	}
}

func TestSearchIndex_OversizeParentKeepsHit(t *testing.T) {
	old := parentStitchMaxChars
	parentStitchMaxChars = 5
	t.Cleanup(func() { parentStitchMaxChars = old })

	h := newSearcherForParentTest(t, nil)
	parent := model.KnowledgeChunkParent{DocumentID: 9, ProductID: "p", Content: "这是一个超过五个字符的父块内容", ChildCount: 1}
	if err := h.db.Create(&parent).Error; err != nil {
		t.Fatalf("写父块失败: %v", err)
	}
	h.keywordSearcher = &fakeKwSearcher{results: []Chunk{{
		ID: "1", DocumentID: "9", Content: "短命中", Score: 0.9, ParentChunkID: parent.ID,
	}}}
	out, err := h.SearchIndex(context.Background(), "p", "短", 5)
	if err != nil {
		t.Fatalf("检索失败: %v", err)
	}
	if len(out) != 1 || out[0].Content != "短命中" {
		t.Errorf("超限父块不应回拼，实际 %+v", out)
	}
}

func TestAttachParentContext_NoDBIsNoop(t *testing.T) {
	h := &HybridSearcher{}
	hits := []Chunk{{ID: "1", Content: "x", ParentChunkID: 7}}
	out := h.attachParentContext(context.Background(), hits)
	if len(out) != 1 || !strings.EqualFold(out[0].Content, "x") {
		t.Errorf("无 db 时应原样返回，实际 %+v", out)
	}
}
