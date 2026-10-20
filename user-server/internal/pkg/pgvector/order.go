// Package pgvector 提供 pgvector 向量检索的公共构造。
//
// 当前只有一件事：按余弦距离排序的 ORDER BY 子句。
package pgvector

import (
	"gorm.io/gorm/clause"
)

// column 是允许参与距离排序的向量列名。
//
// 列名会被拼进 SQL 文本（不是绑定参数），所以类型不导出：外部包无法构造任意
// 列名，只有本包的两个构造函数能产出可用的排序列。
type column string

const (
	// colEmbedding 是知识库分片的向量列（knowledge_chunks.embedding）。
	colEmbedding column = "embedding"
	// colQueryVector 是 RAG 答案缓存的向量列（rag_answer_cache.query_vector）。
	colQueryVector column = "query_vector"
)

// OrderByEmbeddingDistance 构造按 knowledge_chunks.embedding 余弦距离升序的 ORDER BY。
func OrderByEmbeddingDistance(vectorLiteral string) clause.OrderBy {
	return orderByDistance(colEmbedding, vectorLiteral)
}

// OrderByQueryVectorDistance 构造按 rag_answer_cache.query_vector 余弦距离升序的 ORDER BY。
func OrderByQueryVectorDistance(vectorLiteral string) clause.OrderBy {
	return orderByDistance(colQueryVector, vectorLiteral)
}

// orderByDistance 构造「按余弦距离升序」的 ORDER BY 子句。
//
// 距离升序 = 相似度降序，调用方拿到的第一行就是最相似的一条。
//
// 为什么不能写 db.Order(clause.Expr{...})：
// GORM v1.31.2 的 Order 只识别 clause.OrderBy / clause.OrderByColumn / string 三种
// 类型，传 clause.Expr 会走不进去，被静默丢弃——生成的 SQL 里根本没有 ORDER BY，
// 而且不报错。向量检索因此退化成无序返回，调用方拿到的 Top1 是随机的。
//
// 为什么用 clause.OrderBy{Expression: clause.Expr{...}}：
// clause.OrderBy.Build 会优先执行 Expression 分支，clause.Expr 的 ? 占位符
// 会被替换成绑定参数（$1），既让 ORDER BY 真正进 SQL，又不把向量字面量拼进语句。
func orderByDistance(col column, vectorLiteral string) clause.OrderBy {
	return clause.OrderBy{
		Expression: clause.Expr{
			SQL:  string(col) + " <=> ?::vector",
			Vars: []any{vectorLiteral},
		},
	}
}
