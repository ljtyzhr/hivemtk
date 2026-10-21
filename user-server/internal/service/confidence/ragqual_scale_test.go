package confidence

import (
	"testing"

	"hivemtk-user/internal/dto"
)

// 回归：官网网页客服问「介绍下项目」被判「AI 置信度不足 (0.62 < 0.70)」并转人工。
//
// 线上实测：知识库里只有 6 个 chunk，智能体 rag_top_k=3 ⇒ 混合检索的候选池
// 恰好等于 topK，rerank 分支的条件 `len(fused) > topK` 永远不成立 ⇒ 分数退回
// RRF 归一化分（同一批候选内部的相对名次，量级 0.0~0.3）。而 computeRAGQual
// 把它当绝对相关度用，又按写死的 expectedK=5 折一次覆盖率 0.6，
// 于是语义上完全命中的召回（向量相似度实测 0.68~0.85）被压成 RAGQual=0.1344。
//
// 下面两条判据分别钉住两处口径：
//  1. 覆盖率必须按**本次实际召回的期望条数**算，不能写死 5；
//  2. 缺失的信号（拿不到证据时给的 0.5 中性占位值）不该按"中等证据"参与加权。

// TestComputeRAGQual_CoverageUsesConfiguredTopK 覆盖率按传入的期望条数算。
func TestComputeRAGQual_CoverageUsesConfiguredTopK(t *testing.T) {
	c := NewSignalCollector(nil)
	chunks := []dto.RAGChunk{
		{Score: 0.8},
		{Score: 0.6},
		{Score: 0.4},
	}
	// 期望条数 = 3（本次实际配置），覆盖率 = 3/3 = 1 ⇒ 质量 = 均值 0.6。
	got, _ := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks, RAGTopK: 3})
	if !approxEqual(got, 0.6) {
		t.Errorf("覆盖率应按实际 top_k=3 算（覆盖率 1.0），RAGQual 应为 0.6, got %v", got)
	}
}

// TestComputeRAGQual_NoConfiguredTopKFallsBackToChunkCount 未给期望条数时，
// 覆盖率以实际召回条数为分母（全覆盖 ⇒ 1.0），不再写死 5。
func TestComputeRAGQual_NoConfiguredTopKFallsBackToChunkCount(t *testing.T) {
	c := NewSignalCollector(nil)
	chunks := []dto.RAGChunk{
		{Score: 0.8},
		{Score: 0.6},
		{Score: 0.4},
	}
	got, _ := c.computeRAGQual(&dto.SignalCollectionInput{RAGChunks: chunks})
	if !approxEqual(got, 0.6) {
		t.Errorf("未给 top_k 时覆盖率应为 1.0，RAGQual 应为 0.6, got %v", got)
	}
}
