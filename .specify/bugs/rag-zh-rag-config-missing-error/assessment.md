# Bug Assessment: 词法召回无条件先试 `zh_rag`，缺配置时每次检索刷一条 ERROR

- **Slug**: rag-zh-rag-config-missing-error
- **Created**: 2026-10-22T19:55:00+08:00
- **Source**: 线上 user-server 日志（容器 `mtk-user-server`，101.35.252.112）+ 源码与库内实况核对
- **Verdict**: valid
- **Severity**: low

## Report (verbatim or summarized)

线上 user-server 容器日志稳定出现：

```
hivemtk-user/internal/aiagent/rag/retrieval/lexical_retriever.go:79  ERROR: text search configuration "zh_rag" does not exist (SQLSTATE 42704)
[0.341ms] [rows:-] SELECT id, document_id, content, ts_rank(contextual_tsv, plainto_tsquery('zh_rag', '…'))::float8 AS score
  FROM "knowledge_chunks" WHERE contextual_tsv @@ plainto_tsquery('zh_rag', '…') AND "knowledge_chunks"."tenant_id" = 18 …
```

## Symptom

每次知识库词法召回都会先打一条 `text search configuration "zh_rag" does not exist` 的 ERROR 日志（随后自动降级到 `simple` 继续，**功能结果正确**）。噪音让真实错误（真失败与降级）在同一批日志里无法区分，值班读日志时每条 RAG 检索都被一条假红打断。

## Reproduction

1. 部署库为 `pgvector/pgvector:pg15`（线上 `mtk-postgres` 镜像），**不含 zhparser 扩展**：
   `select count(*) from pg_available_extensions where name='zhparser'` ⇒ `0`。
2. 于是 `zh_rag` 文本搜索配置**不存在**（`pg_ts_config` 里查无此名；线上实测）。
3. 触发任意一次 RAG 检索（桥接私信进来 → AI 走 RAG 召回）⇒ 日志出现上面那条 ERROR。

## Suspected Code Paths

- `user-server/internal/aiagent/rag/retrieval/lexical_retriever.go:33` — `tryTSQuery(..., "contextual_tsv", "zh_rag")` **无条件**先试 `zh_rag`；失败后（`:37`）再试 `simple`，再失败（`:41`）退 `ilike`。功能正确，但每次都先撞一次 42704。
- `user-server/internal/aiagent/rag/retrieval/lexical_retriever.go:69-83` — `tryTSQuery` 把错误原样返回，GORM 把它按 ERROR 级别打出来。
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go:342-357` — `detectTSVectorConfig` 用 `plainto_tsquery('test')` 探测，该**单参形式用的是默认配置**（线上 `default_text_search_config = pg_catalog.english`），与被探测的 `zh_rag` 无关 ⇒ 这段探测**测不出哪个配置存在**，恒返回第一个候选 `zh_rag`。
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go:386-408` — 拿到恒为 `zh_rag` 的 cfg 后，`tsvectorSearch` 用 `zh_rag` 必失败 ⇒ cfg 置 nil ⇒ 再循环 `["zh_rag","simple"]` 又先撞一次 `zh_rag`。**每次检索多打一条 ERROR，且探测结果毫无用处。**
- `user-server/internal/migration/migrations/rag_hybrid_migration.go:98-119` — `createZhRagConfig` 只在 **zhparser 已安装**时才建 `zh_rag`（`PARSER = zhparser`）；zhparser 不可用时**故意不建**（返回 nil，不报错）。
- `user-server/internal/migration/migrations/rag_hybrid_migration.go:148-165` — 触发器 `knowledge_chunks_tsv_trigger` 是**防御式**的：`zh_rag` 不存在时用 `simple`（线上实测：函数体确实是这个形状，48 行 chunk 的 tsv 全部已填充）。

## Root Cause Hypothesis

**置信度：高。**

`zh_rag` 缺失是**设计内的状态**，不是漏配：它依赖 `zhparser` 扩展，而 `pgvector/pgvector:pg15` 镜像没有该扩展；迁移正是按"有 zhparser 才建 zh_rag"的判据写的（`rag_hybrid_migration.go:98-106`）。触发器也知道这件事，缺 `zh_rag` 时用 `simple` 兜底 —— 这条兜底对中文**正确**（实测 `to_tsvector('simple','链路自检 测试')` ⇒ `'测试':2 '链路自检':1`）。

缺陷在**读取侧**：代码把 `zh_rag` 当成"一定存在的首选"，无条件先试，于是每次检索都先撞一次 42704；`hybrid_searcher` 的探测函数又用单参 `plainto_tsquery('test')`（走默认配置 `english`）去"探测"被测配置是否存在，**结构上测不出来**，恒返回 `zh_rag`。

**顺带排除一个危险方向**：不可用"造一个 `zh_rag`"来消除报错。PG 的默认解析器把 CJK 当 blank 丢掉 —— 实测 `to_tsvector('zh_rag','链路自检 测试')`（`PARSER = pg_catalog."default"`）⇒ **空 tsvector**，而 `simple` ⇒ 有词元。若这样建 `zh_rag`，触发器里 `IF EXISTS(zh_rag)` 分支就会给**所有新写入的中文 chunk 写空 tsv**，比现在更糟（且 `hybrid_searcher` 会因为 `err == nil` 而直接返回空结果、不再降级）。

## Proposed Remediation

**Preferred**：让读取侧**先探测、后使用**，把"用哪个 text search 配置"从硬编码改成"库里有什么用什么"：

1. 新增一个共享的探测helper（放在 `ragretrieval` 包内，两个检索器共用）：查 `pg_ts_config` 里存在哪些候选配置（`zh_rag` → `simple` 优先级），**只把真实存在的放进可用集合**；探测结果按进程缓存（配置不会在运行期变化）。
2. `lexical_retriever.Retrieve`：按可用集合依次尝试，不再无条件先试 `zh_rag`。
3. `hybrid_searcher.detectTSVectorConfig`：改成用**带配置名**的形式探测（`plainto_tsquery('<cfg>', 'test')`），只认**真实存在且查询不报错**的配置；探测不到就留空，走既有的 `simple` / `ilike` 兜底链。

**Alternatives**:
- 在库里手工建一个 `zh_rag`（无论用 default 还是 zhparser）：**不可取**，理由见「Root Cause Hypothesis」——默认解析器会把中文写成空 tsv，比现状更糟。
- 把 `zh_rag` 换成 `simple` 硬编码：能消错，但把"有 zhparser 的部署用 zh_rag"这条能力也砍了，且与迁移的判据脱钩。**不取**。

**Files likely to change**:
- `user-server/internal/aiagent/rag/retrieval/lexical_retriever.go`
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go`
- `user-server/internal/aiagent/rag/retrieval/`（新增共用探测 helper 文件，或并入上两者之一）

**Tests to add or update**:
- 缺 `zh_rag` 的库上跑一次词法召回：断言**召回结果不为空**（走 `simple` 兜底）、且**不产生 ERROR 级日志**（可用 GORM logger 钩子或直接断言可用配置集合不含 `zh_rag`）。
- 有 `zh_rag` 的库上：断言探测结果选中 `zh_rag`（不被降级到 `simple`）。
- 回归：`hybrid_searcher` / `lexical_retriever` 既有用例保持绿。

## Risks & Considerations

- **只读侧改动**，不写库、不改数据、不改迁移 ⇒ 风险低。
- 探测结果按进程缓存 ⇒ 配置在运行期被改动（`DROP TEXT SEARCH CONFIGURATION`）不会立刻生效，需要重启进程；这与"迁移在启动期执行"的事实源口径一致（配置属启动期装配，不属运行期可变项）。
- 行为变化：缺 `zh_rag` 时**少一次注定失败的查询**（性能略好），召回结果**不变**（本来就靠 `simple` 兜底拿到结果）。
- 不引入新依赖（`pg_ts_config` 是 PG 内置系统表）。

## Open Questions

- [无阻塞项]。是否要把"探测结果"也写进 `config_params` 让运营可见（当前倾向不写：属实现细节，且探测是启动期/首次使用期的内部事实）。
