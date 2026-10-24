package ragretrieval

// vector_retriever_parent_test.go 回归：父块回拼的「取数腿」必须在真实 SQL 上成立。
//
// UI 走查抓到的缺陷：HybridSearcher 三条自带 SQL 都取了 parent_chunk_id，
// 但正常装配下向量腿走的是 VectorRetriever.SearchVector，它的 SELECT 没取该列
// ⇒ 命中条目 ParentChunkID 恒 0 ⇒ attachParentContext 整段空转（回拼从不发生）。
// 单测用的是假 searcher（直接构造 Chunk），所以一直绿——必须打真实 SQL。

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/knowledge/model"
)

func TestVectorRetriever_ReturnsParentChunkID(t *testing.T) {
	db := setupHybridTestDB(t)
	if err := db.AutoMigrate(&model.KnowledgeChunkParent{}); err != nil {
		t.Fatalf("建父块表失败: %v", err)
	}
	parent := model.KnowledgeChunkParent{DocumentID: 9, ProductID: "p", Content: "父块上下文", ChildCount: 1, CharCount: 12}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatalf("写父块失败: %v", err)
	}

	// 1024 维向量（DDL 是 vector(1024)）
	vec := "[" + strings.TrimSuffix(strings.Repeat("0.01,", 1024), ",") + "]"
	ins := fmt.Sprintf(`INSERT INTO knowledge_chunks
		(document_id, product_id, content, chunk_index, embedding, embedding_source, parent_chunk_id, kb_version, retired_version, change_id)
		VALUES (9, 'p', '子块内容', 0, '%s'::vector, 'tei', %d, 0, 0, '')`, vec, parent.ID)
	if err := db.Exec(ins).Error; err != nil {
		t.Fatalf("写子块失败: %v", err)
	}

	r := NewVectorRetriever(db, nil, 0)
	chunks, err := r.SearchVector(context.Background(), "", make([]float32, 1024), 5)
	if err != nil {
		t.Fatalf("SearchVector 失败: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("无命中（夹具或 SQL 前置不满足）")
	}
	if chunks[0].ParentChunkID != parent.ID {
		t.Errorf("向量腿 ParentChunkID = %d want %d：SELECT 没取 parent_chunk_id ⇒ 回拼整段空转", chunks[0].ParentChunkID, parent.ID)
	}

	// 关键词腿同一条判据：RRF 融合后只要有一腿丢列，该腿召回的分支就永远回拼不到。
	lex := NewLexicalRetriever(db)
	kwHits, err := lex.SearchKeyword(context.Background(), "", "子块内容", 5)
	if err != nil {
		t.Fatalf("SearchKeyword 失败: %v", err)
	}
	if len(kwHits) == 0 {
		t.Fatal("关键词腿无命中（夹具或 SQL 前置不满足）")
	}
	if kwHits[0].ParentChunkID != parent.ID {
		t.Errorf("关键词腿 ParentChunkID = %d want %d", kwHits[0].ParentChunkID, parent.ID)
	}
}
