package service

// rag_searcher_preview_test.go 回归：父块回拼的内容不得被预览截断砍回 500。
//
// 缺陷链（UI 走查定位）：HybridSearcher 已把命中的 Content 换成父块（~1500 字），
// 但 chunksToRAGChunks / chunksToMerchantChunks 统一按 chunk_preview_max_len(500)
// 截断 ⇒ 接口返回的仍是 500 字碎片，父子分段的意义在接口层被抹掉。

import (
	"strings"
	"testing"

	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
)

func TestConverters_KeepParentStitchedContent(t *testing.T) {
	long := strings.Repeat("父块上下文。", 200) // 1200 字 > 预览上限
	chunks := []ragretrieval.Chunk{
		{ID: "1", DocumentID: "9", Content: long, ParentChunkID: 7},
		{ID: "2", DocumentID: "9", Content: long}, // 无父块：仍按预览截断
	}

	rag := chunksToRAGChunks(chunks)
	if len(rag) != 2 {
		t.Fatalf("RAGChunk 数 = %d want 2", len(rag))
	}
	if len([]rune(rag[0].Content)) != len([]rune(long)) {
		t.Errorf("父块命中被截断：got %d runes want %d（回拼在接口层等于无效）",
			len([]rune(rag[0].Content)), len([]rune(long)))
	}
	if len([]rune(rag[1].Content)) >= len([]rune(long)) {
		t.Errorf("无父块命中应仍走预览截断，实际保留 %d runes", len([]rune(rag[1].Content)))
	}

	mch := chunksToMerchantChunks(chunks)
	if len(mch) != 2 {
		t.Fatalf("MerchantRAGChunk 数 = %d want 2", len(mch))
	}
	if len([]rune(mch[0].Content)) != len([]rune(long)) {
		t.Errorf("商家侧父块命中被截断：got %d runes want %d", len([]rune(mch[0].Content)), len([]rune(long)))
	}
	if len([]rune(mch[1].Content)) >= len([]rune(long)) {
		t.Errorf("商家侧无父块命中应仍走预览截断")
	}
}
