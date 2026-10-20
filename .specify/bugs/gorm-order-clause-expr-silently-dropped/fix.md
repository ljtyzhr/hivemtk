# Fix Report: 向量召回排序子句改为 `clause.OrderBy{Expression: ...}`，ORDER BY 真正进 SQL

- **Slug**: gorm-order-clause-expr-silently-dropped
- **Fixed**: 2026-10-20
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

GORM v1.31.2 的 `Order(value interface{})` 只识别 `clause.OrderBy` / `clause.OrderByColumn` / `string`，**没有 default 分支**。本仓 5 处向量召回传的是 `clause.Expr`，两个 case 都不匹配 ⇒ 子句被静默丢弃、**不报错** ⇒ 生成的 SQL 里没有 ORDER BY ⇒ `LIMIT topK` 截的是无序结果 ⇒ RAG 召回的 Top1 不是最相似条目，且同一 query 两次调用可能返回不同结果。

修法：新增公共包 `user-server/internal/pkg/pgvector`，对外只暴露两个构造函数，返回 `clause.OrderBy{Expression: clause.Expr{...}}`。`clause.OrderBy.Build` 优先执行 `Expression` 分支，`clause.Expr` 的 `?` 占位符会被替换成绑定参数（`$1`）⇒ ORDER BY 真正进 SQL，且向量字面量不拼进 SQL 文本。

## Changes

| 文件 | 改动 |
|------|------|
| `user-server/internal/pkg/pgvector/order.go` | 新增。提供 `OrderByEmbeddingDistance` / `OrderByQueryVectorDistance`，列名用不导出的 `column` 类型收口，外部包无法构造任意列名 |
| `user-server/internal/pkg/pgvector/order_test.go` | 新增。锁子句形状：Expression 类型、SQL 文本、Vars 长度、向量字面量不入 SQL 文本 |
| `user-server/internal/repository/memory.go` | `Order(clause.Expr{...})` → `Order(pgvector.OrderByEmbeddingDistance(queryVecStr))` |
| `user-server/internal/aiagent/rag/cache/store_pg.go` | `Order(clause.Expr{...})` → `Order(pgvector.OrderByQueryVectorDistance(literal))` |
| `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go` | `Order(clause.Expr{...})` → `Order(pgvector.OrderByEmbeddingDistance(vecStr))` |
| `user-server/internal/aiagent/rag/retrieval/vector_retriever.go` | 2 处 `Order(clause.Expr{...})` → `Order(pgvector.OrderByEmbeddingDistance(vecLiteral))` |
| `user-server/internal/aiagent/knowledge/service/rag_searcher_vector.go` | `Order(clause.Expr{...})` → `Order(pgvector.OrderByEmbeddingDistance(vecLiteral))` |

5 个调用点各删掉一行 `gorm.io/gorm/clause` import（改用新包后不再需要）。

## Diff Highlights

新增包（`user-server/internal/pkg/pgvector/order.go`）的核心：

```go
func orderByDistance(col column, vectorLiteral string) clause.OrderBy {
	return clause.OrderBy{
		Expression: clause.Expr{
			SQL:  string(col) + " <=> ?::vector",
			Vars: []any{vectorLiteral},
		},
	}
}
```

调用点（以 `user-server/internal/aiagent/knowledge/service/rag_searcher_vector.go:35` 为例）：

```go
-	q = q.Order(clause.Expr{SQL: "embedding <=> ?::vector", Vars: []any{vecLiteral}}).Limit(topK)
+	q = q.Order(pgvector.OrderByEmbeddingDistance(vecLiteral)).Limit(topK)
```

`user-server/internal/aiagent/rag/cache/store_pg.go:126`（原 SQL 带显式 `ASC`）：

```go
-		Order(clause.Expr{SQL: "query_vector <=> ?::vector ASC", Vars: []any{literal}}).
+		Order(pgvector.OrderByQueryVectorDistance(literal)).
```

`<=>` 本身是距离度、默认升序 ⇒ 与原 `ASC` 语义等价。

## Tests Added or Updated

- 新增 `user-server/internal/pkg/pgvector/order_test.go`（2 个用例）：
  - `TestOrderByDistance_SQLContainsOrderBy`：断言 `OrderBy.Expression` 是 `clause.Expr`、SQL 文本 = `embedding <=> ?::vector`、`Vars` 长度 1 且值为向量字面量、**向量字面量不出现在 SQL 文本里**。
  - `TestOrderByDistance_QueryVectorColumn`：断言 `query_vector <=> ?::vector`。
- 依赖既有真实库回归 `user-server/internal/aiagent/knowledge/service/rag_searcher_test.go` 的 `TestRagSearcher_RealVectorSearch`（真实 PG + pgvector，断言 Top1 相似度最高）。

## Local Verification

DryRun 探针（`gorm.io/driver/postgres` v1.6.3 + `gorm.Config{DryRun: true, DisableAutomaticPing: true}`，不连库）：

```
SQL: SELECT "id" FROM "knowledge_chunks" ORDER BY embedding <=> $1::vector LIMIT $2
VARS: [[1,2,3] 5]
```

⇒ ORDER BY 进 SQL；向量字面量走绑定参数 `$1`，不拼进语句文本。

包级回归（真实 PG，口令取自容器 `mtk-postgres`）：

| 命令 | 结果 |
|------|------|
| `go test ./internal/pkg/pgvector/ -count=1` | `ok hivemtk-user/internal/pkg/pgvector 0.430s` |
| `go test ./internal/aiagent/knowledge/service -count=1` | `ok hivemtk-user/internal/aiagent/knowledge/service 29.222s`（修复前 `FAIL`，`TestRagSearcher_RealVectorSearch/退货政策检索(pgvector余弦)` + `/发货时间检索(pgvector余弦)` 两个子用例红） |
| `go test ./internal/aiagent/knowledge/controller -count=1` | `ok hivemtk-user/internal/aiagent/knowledge/controller 52.799s` |
| `go test ./internal/aiagent/rag/... ./internal/repository/... ./internal/pkg/pgvector/... -count=1 -p 1` | 全 ok（rag/cache 4.625s、rag/core 0.427s、rag/customer_service 0.860s、rag/eval 2.097s、rag/incremental 2.280s、rag/retrieval 6.224s、rag/service 0.487s、repository 107.716s、pkg/pgvector 0.430s） |
| `go build ./...` | 通过 |
| `gofmt -l`（6 个改动文件） | 输出为空 |

## Deviations from Assessment

- Assessment 提的备选 A（`clause.OrderByColumn{Raw: true}` 内联向量字面量）已放弃，理由与 assessment 一致：向量字面量进 SQL 文本有注入面且不能用绑定参数。
- Assessment 未提"用 DryRun 锁住生成 SQL 文本"这一层；本地验证阶段补了 DryRun 探针（一次性脚本，跑完已删除，未入库），因为只断言子句形状不能证明 ORDER BY 真进了 SQL。

## Follow-ups

- `clause.OrderBy.MergeClause` 与既有 `Order` 叠加的坑已写进包注释：同一查询链上不要在 `Order(pgvector...)` 之后再追加别的 `Order(...)`（后写的 `Expression` 会覆盖前一个）。本仓 5 处调用点各自独立成链，无冲突。
- 更大的召回质量评估（是否加 rerank 兜底）属独立议题，不在本缺陷范围。
