package ragretrieval

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

// 回归：官网网页客服问「介绍下项目」被判低置信并转人工。
//
// 根因在混合检索的 rerank 触发条件：早先写成 `len(fused) > topK` ——
// rerank 拿候选池跟**截断阈值**比大小。官网那个知识库只有 6 个 chunk、
// 智能体 rag_top_k=3 ⇒ 候选池恰好 3 条、topK 也是 3 ⇒ 条件不成立 ⇒
// rerank 整段被跳过 ⇒ 分数停在 RRF 名次分（top1 恒 1.0、其余 0.1~0.3）上，
// 被上层当绝对相关度用，把语义完全命中的召回说成"质量 0.13"。
//
// rerank 的作用是"把名次分换成绝对相关度分"，所以只要**有候选**就必须做，
// 与最终要截几条无关。

// countingReranker 记录被调用的次数，并给每条文档一个绝对相关度分。
type countingReranker struct {
	calls int32
	score float64
}

func (r *countingReranker) Rerank(_ context.Context, _ string, docs []RerankDoc) ([]RerankResult, error) {
	atomic.AddInt32(&r.calls, 1)
	out := make([]RerankResult, 0, len(docs))
	for _, d := range docs {
		out = append(out, RerankResult{ID: d.ID, Score: r.score})
	}
	return out, nil
}

var _ RerankerInterface = (*countingReranker)(nil)

// TestRerankRunsWhenCandidatePoolNotLargerThanTopK 候选池不大于 topK 时 rerank 仍要跑。
func TestRerankRunsWhenCandidatePoolNotLargerThanTopK(t *testing.T) {
	for _, tc := range []struct {
		name     string
		poolSize int
		topK     int
	}{
		{"候选池恰好等于 topK", 3, 3},
		{"候选池小于 topK", 2, 3},
		{"候选池大于 topK", 8, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reranker := &countingReranker{score: 0.8}
			h := &HybridSearcher{
				config:        DefaultHybridSearcherConfig(),
				vectorWeight:  0.7,
				keywordWeight: 0.3,
				reranker:      reranker,
			}

			vec := make([]Chunk, tc.poolSize)
			for i := range vec {
				vec[i] = Chunk{ID: fmt.Sprintf("v%d", i), DocumentID: "1", Content: "内容"}
			}
			fused := h.reciprocalRankFusion(vec, nil, 0.7, 0.3)

			fused, rerankCount := h.applyRerankToFused(context.Background(), "介绍下项目", fused, tc.topK)

			if rerankCount == 0 {
				t.Fatal("rerank 未执行：候选池不大于 topK 时分数会停在 RRF 名次分上，" +
					"被上层当绝对相关度用（线上 RAG 质量 0.1344 → 置信度 0.62 < 0.70 → 转人工）")
			}
			// rerank 之后分数必须是绝对相关度（这里 mock 给 0.8），不是名次分。
			for i, c := range fused {
				if c.Score != 0.8 {
					t.Errorf("fused[%d].Score=%v，应为 rerank 写回的绝对相关度 0.8", i, c.Score)
				}
			}
		})
	}
}
