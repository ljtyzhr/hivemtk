# Bug Verification: 官网右下角网页客服把正常问题误判为转人工

- **Slug**: website-chat-false-handoff
- **Tested**: 2026-10-22
- **Assessment**: ./assessment.md
- **Fix**: ./fix.md
- **Result**: verified

## Summary

复现路径已跑通且症状消失：修复后同一句「介绍下项目」**在生产环境**直接返回 FAQ 标准答案（置信度 1.0、`transferred=false`），而修复前线上是「AI 置信度不足 (0.62 < 0.70)」⇒ 转人工 ⇒ 30 秒后「暂无坐席在线」。新增的 8 组回归测试全部通过，受影响包的既有测试与 lint 零失败。生产发布后按四步清单实测全部通过。

## Checks Performed

| Check | Command / Action | Result | Notes |
|-------|------------------|--------|-------|
| Reproduction (post-fix, 本地真实服务) | 起 `user-server`（本地真实库 + 生产同构 FAQ 数据）→ `POST /api/chat/public/sessions` → `POST /api/chat/public/sessions/:id/messages` 问「介绍下项目」 | pass | `ai_replied=true` / `transferred=false` / `conf=1.0` / 回复=HiveMTK 项目介绍 |
| Reproduction (post-fix, 生产同构 FAQ 数据逐条) | 用生产库完整 10 条 FAQ 跑匹配（`scoreAndRank`） | pass | 10 个问法全部命中正确 FAQ 且过 0.6 门槛 |
| Reproduction (pre-fix, 线上既有证据) | 查生产 `confidence_signals` + `session_messages` | pass（复现成功） | `rag_qual=0.1344`、聚合 `0.6219 < 0.70`；会话 `sess_1792599955034342770_db6f6c94` 被判 `waiting/human` 并留下「【系统】AI 置信度不足 (0.62 < 0.70)」 |
| New / updated tests | `go test ./internal/service/ -run "TestWebsiteE2E\|TestTryFAQFastPath\|TestShouldTransfer\|TestKnownIntent\|TestIsDefiniteAnswer"` | pass | 12 passed + 1 E2E passed（需运行库环境变量） |
| New / updated tests (confidence) | `go test ./internal/service/confidence/...` | pass | ok 0.617s |
| New / updated tests (retrieval) | `go test ./internal/aiagent/rag/retrieval/...` | pass | ok 8.105s |
| New / updated tests (FAQ 匹配) | `go test ./internal/repository/ -run TestFAQScoring_WebsiteQuestionsAreMatchable` | pass | 19 个问法全命中 |
| Regression suite (service 全包) | `go test ./internal/service/ -timeout 1800s` | pass | **ok 1155.226s**（无失败） |
| Regression suite (repository) | `go test ./internal/repository/...` | pass | ok |
| Regression suite (app) | `go test ./internal/app/...` | pass | ok 52.663s |
| Regression suite (dto) | `go test ./internal/dto/...` | pass | ok |
| Lint / type-check | `go vet`（5 个改动包） | pass | 无输出 |
| Lint | `golangci-lint run`（5 个改动包，v2.10.0） | pass | **0 issues** |
| Build | `go build ./internal/...`（service/repository/dto/aiagent/app） | pass | 通过 |
| 红灯校验（确认新测试能抓住原 bug） | 逐条还原旧口径后重跑 | pass | 还原旧覆盖率 → RAGQual 0.44 / 聚合 0.683 失败；还原旧 rerank 条件 → 候选池 ≤ topK 两档失败 |
| 线上数据补全 | `scripts/fix_website_faq_seed.sql`（幂等） | pass | 6 条补关键词 + 补 4 条标准问答 = 10 条；执行前已备份 `/root/faq_backup_20261022.csv` |
| 线上发布 | 交叉编译 linux/amd64 → 上传 → `docker compose build user-server` → `up -d --force-recreate` | pass | 容器内二进制特征串 `tryFAQFastPath=3` / `DefiniteAnswer=2`、大小 98901322（旧 99009904）⇒ 跑的确实是新代码 |
| 线上验收 ①（介绍下项目） | `POST /api/chat/public/sessions/:id/messages` | pass | `transferred=false` / `conf=1.0` / 回复=项目介绍标准答案 |
| 线上验收 ②（其余问法 + 显式要人工） | 同上，逐个问 | pass | 多少钱→计费答案(0.753)；支持哪些渠道→13 渠道(1.0)；怎么安装部署→部署方式(1.0)；**我要找人工客服→正确转人工** |
| 线上验收 ③（信号分值） | 查 `confidence_signals` | pass | 新记录 `rag_qual=0.5`（不再是 0.13）、`aggregated_conf=1.0/0.7533`、`decision_band=auto`；走 RAG 的记录 `rag_qual=0.5`、`aggregated_conf=0.671`、band=review（不再直接转人工） |
| 线上验收 ④（转人工比例） | 查 `confidence_signals` 近 1 小时 | pass | 4 条信号**0 条低置信**（修复前同口径全 < 0.70）；近 3 小时唯一一条 `waiting/human` 会话是「我要找人工客服」触发的，`handoff_reason` 为空 ⇒ 不是"置信度不足"路径 |

## Output Excerpts

复现（修复前，线上既有数据）：

```
 id |            session_id             | intent_type | rag_qual | aggregated_conf | threshold | decision_band
  6 | sess_1792599955034342770_db6f6c94 | ask_product |  0.1344  |      0.6219     |   0.70   | review
```

```
 id | sender_type |            content
 31 |    system    | 【系统】AI 置信度不足 (0.62 < 0.70)
```

复现（修复后，本地真实服务，同一句问法）：

```
=== 问: 介绍下项目
  transferred: False | conf: 1 | handler: ai
  reply: HiveMTK 是一款开源的智能营销系统（SaaS 化多租户），提供 AI 智能体客服、多渠道消息接入…
=== 问: 支持哪些渠道
  transferred: False | conf: 1 | handler: ai
  reply: HiveMTK 支持 13 个渠道：Telegram、QQ、企业微信、飞书、WhatsApp、钉钉、抖音、小红书、快手、闲鱼、TikTok，以及网页客服…
=== 问: 怎么安装部署
  transferred: False | conf: 1 | handler: ai
  reply: 支持。HiveMTK 支持私有化部署（Docker Compose），也提供 SaaS 云端版（work.xapptool.cn）…
=== 问: 我要找人工客服
  transferred: True | conf: 0 | handler: human        ← 显式要人工仍正确转人工
```

生产同构 FAQ 数据逐条匹配（10/10 命中）：

```
介绍下项目       -> FAQ#1  分 2.3750 ✓ HiveMTK 是什么？
多少钱           -> FAQ#3  分 0.6300 ✓ HiveMTK 怎么收费？
支持哪些渠道      -> FAQ#2  分 2.1074 ✓ HiveMTK 支持哪些渠道？
怎么安装部署      -> FAQ#4  分 1.6538 ✓ HiveMTK 支持私有化部署吗？
我要找人工客服     -> FAQ#6  分 2.1332 ✓ 怎么联系 HiveMTK 官方？
价格套餐怎么订阅    -> FAQ#3  分 2.2900 ✓ HiveMTK 怎么收费？
数据安全吗       -> FAQ#8  分 1.4147 ✓ HiveMTK 的数据安全吗？会出域吗？
```

门禁：

```
ok  	hivemtk-user/internal/service           	1155.226s
ok  	hivemtk-user/internal/repository        	(cached)
ok  	hivemtk-user/internal/app               	52.663s
ok  	hivemtk-user/internal/service/confidence	0.617s
ok  	hivemtk-user/internal/aiagent/rag/retrieval	8.105s
0 issues.
```

装配层生效证据：

```
SalesEngine 已注入 FAQ 快速通道（命中标准答案直答）
```

## Residual Risks

- **首次发布曾失败并回滚**：本地构建出的是 macOS(darwin/arm64) 二进制，直接发布导致容器 `exec format error` 起不来；已用备份 `user-server.prev-20261022-webfaq` 立即回滚（线上 30 秒内恢复），随后改用 `CGO_ENABLED=0 GOOS=linux GOARCH=amd64` 交叉编译重新发布并通过验收。**教训：本仓发布必须交叉编译，不能拿本地 `go build` 的产物直接传。**
- **「docker compose up」不等于发布成功**：user-server 的二进制是 `COPY` 进镜像的（`Dockerfile.user-server`），不是卷挂载 ⇒ 只重建容器会跑**旧镜像里的旧二进制**。必须 `docker compose build user-server` 再 `up -d --force-recreate`。本次已按内容特征串验过容器内二进制。
- **`faqCacheMaxN = 5000` 候选集上限**：本地库实测有 24 万条同 agent 的 FAQ 时，新建 FAQ 会因 `ORDER BY hit_count DESC, confidence DESC, id ASC` 被挤出候选集 ⇒ FAQ 快速通道完全不生效。生产库当前只有 10 条，不受影响；但这是同类故障的下一个入口。已记入 fix.md 的 Follow-ups。
- **`internal/app/sales_engine_factory.go` 里 `cfg.ConfidenceThreshold = 0.5` 硬编码覆盖 DB 值**：会让运营在后台调 `smart_cs.confidence_threshold` 无效。本轮未改（超出本 bug 范围，且改动会改变现网行为），已记入 Follow-ups。
- **共享工作区并发干扰**：本轮我的代码改动一度被另一条泳道的提交整片吞掉（`git log --all -S tryFAQFastPath` 查无此串），最终改在**独立 worktree**（`/tmp/hivemtk-website-fix`）里重建并提交，避开该竞态。生产库未受影响。
- **`logSearch` 写 `knowledge_search_logs` 失败**（UUID 型 `product_id` 强转 bigint）：检索日志缺失，不影响回答链路，本轮未修。

## Recommendation

**可以关闭这个 bug。** 生产环境已实测：官网同一句问法、同一套生产数据，修复后直接答出标准答案且不再转人工；四步验收清单全部通过；近 1 小时置信度信号 0 条低置信；显式要人工仍正确转人工。

后续观察项（不影响本 bug 关闭）：
- 24 小时内 `customer_sessions` 里 `handler_type='human'` 且 `handoff_reason` 含「置信度不足」的比例应持续下降。
- `faqCacheMaxN` 候选集上限与 `cfg.ConfidenceThreshold` 硬编码这两项按 fix.md 的 Follow-ups 排期。
