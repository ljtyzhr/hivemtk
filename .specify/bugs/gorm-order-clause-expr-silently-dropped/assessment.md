# Bug Assessment: GORM 丢弃 `Order(clause.Expr)` ⇒ 向量检索无序返回，Top1 随机

- **Slug**: gorm-order-clause-expr-silently-dropped
- **Created**: 2026-10-20
- **Source**: 自查（对抗审计 pgvector 召回路径时读 GORM v1.31.2 源码 + DryRun 探针 + `TestRagSearcher_RealVectorSearch` 失败）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

本轮为「前置任务一」第 3 段（对抗性检查、审核、测试整个 SaaS 多租户改造，发现问题立即修复）的延伸排查。审计多租户隔离时顺带核对 RAG 向量召回链路的排序语义，发现 5 处向量排序子句从未真正进入 SQL。

## Symptom

`db.Order(clause.Expr{...})` 是 GORM 里最常见的"按表达式排序"写法。本仓 5 处向量召回都用它排 `embedding <=> ?::vector`。

GORM v1.31.2 的 `Order(value interface{})` 只做三路类型 switch：`clause.OrderBy` / `clause.OrderByColumn` / `string`，**没有 default 分支**。传 `clause.Expr` 时两个 case 都不匹配，函数静默返回，**不报错、不 panic**。

后果：生成的 SQL 里**根本没有 ORDER BY**，向量召回退化成无序返回。`LIMIT topK` 截的是数据库随便给的 N 行，调用方拿到的 Top1 相似度不是最大值——同一 query 两次调用结果可能不同。RAG 召回质量因此不可预期，且这类退化对调用方完全不可见（没有错误、没有日志）。

## Reproduction

前置：真实 PostgreSQL + pgvector，`knowledge_chunks.embedding` 1024 维，HNSW 索引在位。

1. 准备 3 条 chunk，embedding 分别为 `[1,0,0,…]`、`[0.9,0.1,0,…]`、`[0,1,0,…]`（1024 维补零），query 向量取 `[1,0,0,…]`。
2. 修复前写法：

```go
q = q.Order(clause.Expr{SQL: "embedding <=> ?::vector", Vars: []any{vecLiteral}}).Limit(topK)
```

3. DryRun 打印 SQL（`gorm.io/driver/postgres` v1.6.3 + `gorm.Config{DryRun: true}`）⇒ 生成：

```
SELECT ... FROM "knowledge_chunks" WHERE ... LIMIT $2
```

**没有 ORDER BY**。

4. 真实库执行同样查询 ⇒ 返回的第一行不是最相似的那条；`TestRagSearcher_RealVectorSearch/退货政策检索(pgvector余弦)` 与 `/发货时间检索(pgvector余弦)` 两个子用例失败。

5. 改成 `clause.OrderBy{Expression: clause.Expr{...}}` 后，DryRun 变为：

```
SELECT ... FROM "knowledge_chunks" WHERE ... ORDER BY embedding <=> $2::vector LIMIT $3
```

真实库执行 ⇒ Top1 恒为最相似条目，两个子用例通过。

## Suspected Code Paths

| 文件 | 行 | 原写法 |
|------|----|--------|
| `user-server/internal/repository/memory.go` | 255 | `Order(clause.Expr{SQL: "embedding <=> ?::vector", ...})` |
| `user-server/internal/aiagent/rag/cache/store_pg.go` | 126 | `Order(clause.Expr{SQL: "query_vector <=> ?::vector ASC", ...})` |
| `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go` | 320 | `Order(clause.Expr{SQL: "embedding <=> ?::vector", ...})` |
| `user-server/internal/aiagent/rag/retrieval/vector_retriever.go` | 70 | `Order(clause.Expr{SQL: "embedding <=> ?::vector", ...})` |
| `user-server/internal/aiagent/knowledge/service/rag_searcher_vector.go` | 35 | `Order(clause.Expr{SQL: "embedding <=> ?::vector", ...})` |

上游源码依据：`gorm.io/gorm@v1.31.2/chainable_api.go` 的 `Order` 只匹配 `clause.OrderByColumn` / `clause.OrderBy` / `string`；`gorm.io/gorm@v1.31.2/clause/order_by.go` 的 `OrderBy.Build` 优先走 `orderBy.Expression` 分支，`clause.Expr` 的 `?` 占位符会被 `builder` 替换为绑定参数（`$1`）。

## Root Cause Hypothesis

**已验证**（非假设）：

1. GORM v1.31.2 `Order` 无 default 分支 ⇒ `clause.Expr` 被静默丢弃（源码 + DryRun 双重证据）。
2. `clause.OrderBy` 有 `Expression` 字段，且 `Build` 优先执行它 ⇒ 用 `clause.OrderBy{Expression: clause.Expr{...}}` 包装即可让表达式真正进 SQL，同时保留 `?` 绑定参数（向量字面量不拼进 SQL 文本）。
3. 注意 `clause.OrderByColumn` 在 v1.31.2 **没有** `Expression` 字段（写 `clause.OrderByColumn{Expression: ...}` 编译报 `unknown field Expression`），所以带 `Vars` 的表达式排序只能走 `clause.OrderBy` 这条路。
4. `clause.OrderBy.MergeClause` 的行为：已有 ORDER BY 子句时会把旧 `Columns` 拷到新 `OrderBy` 的 `Columns` 前面，但 `Build` 仍优先走 `Expression` ⇒ 同一查询链上不要在 `Order(pgvector...)` 之后再追加别的 `Order(...)`，否则旧列被覆盖。本仓 5 处调用点各自独立成链，无冲突。

## Proposed Remediation

### Preferred

新增公共构造包 `user-server/internal/pkg/pgvector`，对外只暴露两个构造函数，返回 `clause.OrderBy`（内含 `Expression`）：

- `OrderByEmbeddingDistance(vectorLiteral string) clause.OrderBy` ⇒ `embedding <=> ?::vector`
- `OrderByQueryVectorDistance(vectorLiteral string) clause.OrderBy` ⇒ `query_vector <=> ?::vector`

把 5 处调用点改成 `q.Order(pgvector.OrderByEmbeddingDistance(vecLiteral))` / `pgvector.OrderByQueryVectorDistance(literal)`。

列名不导出（内部 `column` 类型不导出），外部包无法构造任意列名 ⇒ 避免把用户输入拼进 SQL 文本。

### Alternatives (optional)

- **A. 改用 `clause.OrderBy{Columns: []clause.OrderByColumn{{Column: clause.Column{Name: "embedding <=> '[1,2]'::vector", Raw: true}}}}`**：可行（实测生成 `ORDER BY embedding <=> '[1,2]'::vector`），但把向量字面量内联进 SQL 文本 ⇒ 有 SQL 注入面 + 无法用绑定参数 + 无法走 HNSW 索引的稳定规划。不采纳。
- **B. 改用 `db.Raw("... ORDER BY embedding <=> ?::vector", ...)`**：可行，但放弃 GORM 查询链，且与仓库 `scripts/check-gorm-only.sh` 的 `.Raw(` 扫描规则冲突（该脚本不剥注释，命中即红）。不采纳。
- **C. `db.Clauses(clause.OrderBy{Expression: ...})`**：等价于 Preferred 的 `Order` 路径，但 `Clauses` 语义更宽，不如 `Order` 直观。不采纳。

### Files likely to change

- 新增 `user-server/internal/pkg/pgvector/order.go`
- 新增 `user-server/internal/pkg/pgvector/order_test.go`
- 改 `user-server/internal/repository/memory.go`
- 改 `user-server/internal/aiagent/rag/cache/store_pg.go`
- 改 `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go`
- 改 `user-server/internal/aiagent/rag/retrieval/vector_retriever.go`
- 改 `user-server/internal/aiagent/knowledge/service/rag_searcher_vector.go`

### Tests to add or update

- `user-server/internal/pkg/pgvector/order_test.go`：锁子句形状（`Expression` 是 `clause.Expr`、SQL 文本正确、`Vars` 长度 1、向量字面量不出现在 SQL 文本里）。
- 回归依赖 `user-server/internal/aiagent/knowledge/service/rag_searcher_test.go` 的 `TestRagSearcher_RealVectorSearch`（真实 PG + pgvector，断言 Top1 相似度最高）。

## Risks & Considerations

- 排序语义从"无序"变成"距离升序"⇒ 调用方拿到的结果集顺序会变（这正是修复目标）。下游若有依赖旧的无序行为的代码，行为会改变，但旧行为本身就是缺陷。
- `store_pg.go` 原 SQL 带显式 `ASC`；新构造器产出 `query_vector <=> ?::vector`（`<=>` 默认升序）⇒ 语义等价。
- `store_pg.go` 的 SELECT 列表输出列名也叫 `query_vector`（`query_vector::text AS query_vector`）。PostgreSQL 的 ORDER BY 解析规则：**简单列名**优先匹配输出列，**表达式**按输入列解析。`query_vector <=> $1::vector` 是表达式 ⇒ 解析到表里的 vector 列，不是那个 text 别名。已在真实库上验证通过。
- HNSW 索引：ORDER BY 表达式现在能进 SQL，PostgreSQL 可用 HNSW 索引做 top-K 近似检索。小表场景规划器可能仍选全表扫描 + 排序（结果精确），测试断言精确 Top1 ⇒ 小表下仍通过。
- `clause.OrderBy.MergeClause` 与既有 `Order` 叠加的坑：已在包注释里写明"不要再叠加别的 Order"。

## Open Questions

- 无阻塞问题。更大范围的召回质量评估（是否要加 rerank 兜底）属独立议题，不在本缺陷范围。
