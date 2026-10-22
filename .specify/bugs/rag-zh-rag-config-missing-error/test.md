# Bug Verification: 词法召回按库里真实存在的文本搜索配置挑

- **Slug**: rag-zh-rag-config-missing-error
- **Tested**: 2026-10-22T20:22:00+08:00
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

线上症状已消失：修复前每次知识库召回都打一条 `ERROR: text search configuration "zh_rag" does not exist (SQLSTATE 42704)`，修复后近 20 分钟内该类报错为 **0**。新增的三条回归锁在去掉修复后如期变红，且红的时候复现的正是线上原文。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix) | 线上 `docker logs mtk-user-server --since 20m \| grep -c "zh_rag"` | pass | **0**（修复前：每条 RAG 召回 1 条 42704 ERROR） |
| New / updated tests | `go test ./internal/aiagent/rag/retrieval/ -run 'TestAvailableTSConfigs\|TestLexicalRetrieve_' -v` | pass | 3 个用例全 PASS |
| Regression suite | `go test ./internal/aiagent/rag/... -timeout 600s` | pass | 7 个子包全 `ok`（retrieval 13.727s） |
| 回归有效性（去修复须红） | `git stash push -- lexical_retriever.go hybrid_searcher.go` 后重跑 `TestLexicalRetrieve_NoMissingConfigError` | pass | 如期红，**且复现线上原文**：`ERROR: text search configuration "zh_rag" does not exist (SQLSTATE 42704)` |
| Lint / type-check | `cd user-server && go build ./...` | pass | exit 0 |
| 线上部署 | 与另一 bug 同批发布（同一二进制、同一次重建/重启） | pass | 7 秒就绪，`/health` 全绿 |

## Output Excerpts

去掉修复后的红灯（**线上原文**）：

```
--- FAIL: TestLexicalRetrieve_NoMissingConfigError (10.09s)
    tsconfig_probe_test.go:142: 执行期间仍出现针对不存在配置的失败查询：
    SELECT id, document_id, content, ts_rank(contextual_tsv, plainto_tsquery('zh_rag', '私域'))::float8 AS score
      FROM "knowledge_chunks" WHERE contextual_tsv @@ plainto_tsquery('zh_rag', '私域') ORDER BY score DESC LIMIT 10
      | ERROR: text search configuration "zh_rag" does not exist (SQLSTATE 42704)
```

修复后：

```
--- PASS: TestAvailableTSConfigs_ExcludesMissingConfig (7.86s)
--- PASS: TestAvailableTSConfigs_PrefersZhRag (0.52s)
--- PASS: TestLexicalRetrieve_NoMissingConfigError (0.38s)
ok  	hivemtk-user/internal/aiagent/rag/retrieval	9.415s
```

线上：

```
近 20 分钟 zh_rag 报错次数：0
```

## Residual Risks

- **探测结果按 `*gorm.DB` 缓存** ⇒ 运行期手工 `DROP TEXT SEARCH CONFIGURATION` 需重启进程才生效。配置属启动期/安装期事实（迁移在启动期执行），与事实源口径一致；若将来允许运行期切换文本搜索配置，需在 `resetTSConfigProbe` 之外补一个对外的失效入口。
- **线上 20 分钟窗口内未发生真实 RAG 召回**（该窗口无桥接入站消息），因此「zh_rag 报错归零」这一条是**日志计数证据**，不是「触发过一次召回且无错」的端到端证据。端到端侧由单测 `TestLexicalRetrieve_NoMissingConfigError` 覆盖（它在缺配置形态下真的跑了一次召回，断言结果非空且执行期间无失败 SQL）。
- `internal/service` 全包测试超时问题同另一 bug 的 test.md（已用 stash 对照证明与本轮无关）。

## 补充说明（避免后人重踩）

本轮评估阶段曾在线上**手工建过**一个 `PARSER = pg_catalog."default"` 的 `zh_rag`，随后发现它对中文产出**空 tsvector**（PG 默认解析器把 CJK 当 blank 丢掉，而 `simple` 能正确切出 `'测试':2 '链路自检':1`），若留着会让触发器给新写入的中文 chunk 写空 tsv（比报错更糟），**已 `DROP` 撤销**并复查 `pg_ts_config` 为 0 条；期间无 chunk 被写入，48 行 tsv 全部完好。
正确解法是本轮落地的「按库里真实存在的配置挑」，而不是造一个假的 `zh_rag`。
