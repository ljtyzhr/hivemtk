# Bug Fix: 官网右下角网页客服把正常问题误判为转人工

- **Slug**: website-chat-false-handoff
- **Fixed**: 2026-10-22
- **Assessment**: ./assessment.md
- **Status**: applied

## Summary

官网 `xapptool.cn` 右下角浮标问「介绍下项目」等正常业务问题时，AI 答不出来 ⇒ 系统判「AI 置信度不足 (0.62 < 0.70)」转人工 ⇒ 30 秒后前端提示「暂无坐席在线」。

根因是**四个缺陷叠加**，共同把"AI 答得出来"变成"置信度不足 ⇒ 转人工"：混合检索的 rerank 在小知识库上从不执行（分数停在 RRF 名次分）、RAG 覆盖率的分母写死 5、缺失的信号（无历史对话/无 logprobs）按"中等证据"占位参与 35% 权重、以及网页客服走的非流式路径**根本不查 FAQ**（FAQ 里明明有「HiveMTK 是什么？」标准答案）。已从代码层修掉前三层、把 FAQ 快速通道接进网页客服路径，并补全了线上 FAQ 数据。

## Changes

| File | Change | Notes |
|------|--------|-------|
| `internal/aiagent/rag/retrieval/hybrid_searcher.go` | modified | rerank 触发条件从 `len(fused) > topK` 改为 `len(fused) > 0`；rerank 段抽成 `applyRerankToFused` 便于单测 |
| `internal/service/confidence/signal_collector.go` | modified | RAG 覆盖率分母改为本次实际 `rag_top_k`；采集器逐维标出"真取到证据"还是"中性占位值" |
| `internal/service/confidence/weighted_aggregator.go` | modified | 缺失信号不参与加权（权重从分母去掉、其余按比例放大） |
| `internal/dto/confidence.go` | modified | `FiveSignals.Available`（信号可用性）+ `SignalCollectionInput.RAGTopK` |
| `internal/dto/sales.go` | modified | `SalesResponse.DefiniteAnswer`（确定答案标记） |
| `internal/service/smart_cs_orchestrator.go` | modified | 转人工判据纳入决策区间（band）；空 intent 不算已知意图；确定答案跳过置信度判据；置信度采集带上 `rag_top_k` |
| `internal/service/sales_engine.go` | modified | 新增 FAQ 快速通道 `tryFAQFastPath`，非流式 `Handle` 路径接入 |
| `internal/repository/faq_entry.go` | modified | 关键词命中加分提为常量并加词元级包含；导出 `ScoreFAQCandidates` 供测试复用同一份打分口径 |
| `internal/app/sales_engine_factory.go` | modified | 装配层注入 FAQ 服务（启用快速通道） |
| `scripts/fix_website_faq_seed.sql` | added | 线上 FAQ 数据补全脚本（幂等）：6 条补关键词 + 补 4 条标准问答 |
| `internal/service/confidence/ragqual_scale_test.go` | added test | RAG 覆盖率按实际 top_k 算 |
| `internal/service/confidence/signal_availability_test.go` | added test | 缺失信号不参与加权；全部可用时与旧公式逐字一致 |
| `internal/service/confidence/website_handoff_regression_test.go` | added test | 「介绍下项目」整条判据链的回归 |
| `internal/aiagent/rag/retrieval/hybrid_searcher_rerank_scope_test.go` | added test | 候选池不大于 topK 时 rerank 仍执行 |
| `internal/repository/faq_website_match_test.go` | added test | 19 个官网常见问法必须命中正确 FAQ 且过 0.6 门槛 |
| `internal/service/sales_engine_faq_fastpath_test.go` | added test | FAQ 快速通道命中/未命中/无智能体三种情形 |
| `internal/service/smart_cs_handoff_band_test.go` | added test | band 判据、空 intent、确定答案 |
| `internal/service/website_e2e_integration_test.go` | added test | 真实库端到端：渠道→绑定→智能体→FAQ 匹配→直答→不转人工 |

## Diff Highlights

**一、混合检索：rerank 必须执行（`hybrid_searcher.go`）**

```go
-	if h.config.EnableRerank && h.reranker != nil && len(fused) > topK {
+	if h.config.EnableRerank && h.reranker != nil && len(fused) > 0 {
```

早先把**候选池**跟**截断阈值**比大小：官网知识库只有 6 个 chunk、`rag_top_k=3` ⇒ 池恰好 3 条、topK 也是 3 ⇒ 条件恒不成立 ⇒ rerank 整段被跳过 ⇒ 分数停在 RRF 名次分（top1 恒 1.0、其余 0.1~0.3）。rerank 的作用是"换量纲"不是"筛数量"，有候选就必须做。

**二、覆盖率分母按实际配置（`signal_collector.go`）**

```go
-	expectedK := 5.0
+	expectedK := in.RAGTopK
+	if expectedK <= 0 {
+		expectedK = len(chunks)
+	}
```

写死 5 而智能体普遍配 3 ⇒ 每次正常召回都被折成 0.6 倍。召回条数少是**配置**（限上下文预算），不是缺陷。

**三、缺失信号不参与加权（`weighted_aggregator.go`）**

采集器在拿不到证据时给 0.5 中性占位值（无历史对话 / 无 logprobs）。0.5 不是"中等证据"而是"这里没数据"，但等权聚合把它当实打实的分数算：首轮对话里 `ctx_relev` 与 `llm_entropy` 两项占 **35% 权重**，把总置信度按在阈值之下。改法是缺失信号的权重从分母里去掉、其余按比例放大；可用信号一个都没有时退回等权平均（不返回 0 —— 0 会被判成"完全不可信"而直接转人工）。

**四、决策区间参与转人工判据（`smart_cs_orchestrator.go`）**

```go
-	shouldTransfer := salesResp.TransferredToHuman ||
-		(knownIntent && !safeIntent && effectiveConf < threshold)
+	shouldTransfer := salesResp.TransferredToHuman ||
+		(!isDefiniteAnswer(salesResp) && knownIntent && !safeIntent &&
+			shouldTransferOnLowConfidence(band, effectiveConf, threshold))
```

聚合器早就算出了四档决策区间，编排器却根本不看它 —— `review`（人工审核）与 `llm_fallback`（LLM 兜底）两档被当成了 `handoff`。线上「介绍下项目」的信号落在 **review** 档却仍被转人工。

**五、FAQ 快速通道接进网页客服（`sales_engine.go` + 装配层）**

非流式 `Handle` 早先从不查 FAQ（Layer1 路由只在 `HandleStream` 里被调用，且装配层从未 `SetLayerRouter`）⇒ FAQ 形同虚设。新增 `tryFAQFastPath`：命中标准答案就直接作答（零 LLM、零 Token、答案确定），并标记 `DefiniteAnswer` 让编排器跳过置信度判据。

**六、FAQ 匹配：关键词要认语序（`faq_entry.go`）**

```go
-			if kw != "" && strings.Contains(msg, kw) {
-				score += 0.3
+			if strings.Contains(msg, kw) {
+				score += faqKeywordBonusExact   // 0.7
+				continue
+			}
+			if keywordTokenOverlap(msg, kw) {
+				score += faqKeywordBonusPartial // 0.4
+			}
```

早先关键词必须是用户原句的**连续子串**：关键词「产品介绍」遇到「介绍下项目」完全对不上。中文里语序一变整句就不含原关键词，但核心词元还在 —— 这正是 FAQ 该起作用的地方。

**七、线上数据补全（`scripts/fix_website_faq_seed.sql`）**

线上 `faq_entries` 只有 6 条（仓库迁移 036 里有 11 条，线上被换成手工补的 6 条），且关键词太少。脚本幂等：给 6 条补齐关键词 + 补 4 条缺失的标准问答（技术架构 / 数据安全 / 资产市场 / 配置智能体）。执行前已备份 `/root/faq_backup_20261022.csv`，**已执行**。

## Tests Added or Updated

- `internal/service/confidence/website_handoff_regression_test.go::TestConfidence_WebsiteProductQuestion_NotPushedBelowHandoffThreshold` — 「介绍下项目」的整条判据链：意图 0.9 + 召回正常时，聚合置信度必须越过 0.70
- `internal/service/confidence/ragqual_scale_test.go::TestComputeRAGQual_CoverageUsesConfiguredTopK` — 覆盖率按实际 `rag_top_k` 算（top_k=3 时不被折成 0.6）
- `internal/service/confidence/signal_availability_test.go::TestWeightedAggregator_UnavailableSignalsAreExcluded` — 缺失信号不参与加权；`..._AllAvailableKeepsLegacyFormula` 钉住全部可用时与旧公式逐字一致
- `internal/aiagent/rag/retrieval/hybrid_searcher_rerank_scope_test.go::TestRerankRunsWhenCandidatePoolNotLargerThanTopK` — 候选池 ≤ topK 时 rerank 仍执行，chunk 分数是绝对相关度而非 RRF 名次分
- `internal/repository/faq_website_match_test.go::TestFAQScoring_WebsiteQuestionsAreMatchable` — 19 个官网常见问法（含「介绍下项目」「多少钱」「怎么安装部署」「我要找人工客服」）必须命中正确 FAQ 且过 0.6 门槛
- `internal/service/sales_engine_faq_fastpath_test.go::TestTryFAQFastPath_HitsStandardAnswer` — FAQ 命中直接返回标准答案、不转人工；无智能体时不走本通道（防串话术）
- `internal/service/smart_cs_handoff_band_test.go::TestShouldTransfer_LowConfidenceButNotHandoffBand` — band 不到 handoff 时低置信不转人工；`::TestKnownIntent_EmptyIntentIsNotKnown` — 空 intent 不算已知意图
- `internal/service/website_e2e_integration_test.go::TestWebsiteE2E_ProductQuestionAnsweredByFAQ` — 真实库端到端：web_embed 渠道 → 绑定 → 智能体 → FAQ 匹配 → 引擎直答标准答案 → 标记确定答案 → 不转人工

## Local Verification

- `go build ./internal/service/... ./internal/repository/... ./internal/dto/... ./internal/aiagent/... ./internal/app/...` → 通过
- `go test ./internal/service/ -timeout 1800s` → **ok 1155.226s**（全包无失败）
- `go test ./internal/repository/... ./internal/app/...` → ok
- `go test ./internal/service/confidence/...` → ok 0.617s
- `go test ./internal/aiagent/rag/retrieval/...` → ok 8.105s
- `go test ./internal/dto/...` → ok
- `go vet`（上述五个包）→ 通过
- `golangci-lint run`（上述五个包，v2.10.0）→ **0 issues**
- 红灯校验（逐条确认新测试能抓住原 bug）：
  - 还原旧覆盖率口径 → `TestComputeRAGQual_CoverageUsesConfiguredTopK` / `TestConfidence_WebsiteProductQuestion_...` 失败（0.44 / 0.683）
  - 还原旧 rerank 条件 → `TestRerankRunsWhenCandidatePoolNotLargerThanTopK` 失败（候选池等于/小于 topK 两档）
- 线上实测（修复前）：`confidence_signals` 表 `rag_qual=0.1344`（意图 0.9 / 实体 1.0 / 上下文 0.5 / 熵 0.5）⇒ 聚合 0.6219 < 0.70 ⇒ 转人工
- 线上实测（生产 rerank API 直连，6 条真实 chunk × query「介绍下项目」）：分数 0.341 / 0.320 / 0.287 / 0.277 / 0.274 / 0.269，**top1 命中的正是「HiveMTK 产品介绍」那条**
- 修复后推算（用上述真实分数）：RAGQual 0.22~0.33，聚合置信度 **0.714~0.748** ⇒ 越过 0.70 阈值，不再转人工
- 线上数据：FAQ 6 条 → 10 条（补关键词 + 补 4 条标准问答），已备份后执行

## Deviations from Assessment

1. **多出一层根因：缺失信号按"中等证据"占位参与加权。** assessment 只定位到「RAG 分数量纲」与「FAQ 层没接线」两层；实测发现即便 rerank 跑通、分数回到绝对量纲（0.33），聚合置信度仍是 0.66 < 0.70 —— 因为 `ctx_relev` 与 `llm_entropy` 两个**中性占位值**合计占 35% 权重。为此新增了「信号可用性」机制（`FiveSignals.Available`）。这是本轮最重要的发现，也是 assessment 阶段用线上数据回算才暴露的。
2. **assessment 提的"转人工判据分层"改成了"band 参与判据 + 确定答案豁免"两处。** 只按 band 分层不够：FAQ 直答的 `intent` 字段线上多为空 ⇒ 旧判据 `IntentType != IntentUnknown` 把空串当"已知意图" ⇒ 仍被低置信判据推翻。故再加一条「确定答案（FAQ 标准答案）不适用置信度判据」+「空 intent 不算已知意图」。
3. **assessment 里"降 confidence_threshold"这个替代方案被明确排除。** 理由已记在 assessment：那是给一个量纲错误的分放行，治标不治本。
4. **assessment 说"配置层（`smart_cs.confidence_threshold=0.7`）没问题"，这一点成立；但漏了另一处配置事实**：`internal/app/sales_engine_factory.go` 里 `cfg.ConfidenceThreshold = 0.5` 硬编码覆盖了 DB 值（0.7）。本轮未改这一处 —— 它是另一个问题（硬编码覆盖 DB 配置），不在本 bug 范围内，但它解释了为什么线上有时表现得比 0.7 宽松。**留作后续项。**
5. **未改 `knowledge_bases.doc_count=0`。** 已确认该列是**已知冗余列**（`internal/service/knowledge_base.go:140` 明示"无人维护"），不是本 bug 根因。

## Follow-ups

- **`internal/app/sales_engine_factory.go` 的 `cfg.ConfidenceThreshold = 0.5` 硬编码**：它覆盖了 `config_params.smart_cs.confidence_threshold` 的 DB 值，让运营在后台调阈值无效。建议改回读 DB 值（本轮未动，因为它会改变现网行为，超出本 bug 范围）。
- **`faqCacheMaxN = 5000` 的候选集上限**：本地库实测有 24 万条同 agent 的 FAQ，新加的 FAQ 会因 `ORDER BY hit_count DESC, confidence DESC, id ASC` 被挤出候选集（本地实测：新建的 10 条 FAQ 全部落在 5000 条之外 ⇒ FAQ 快速通道完全不生效）。生产库当前只有 10 条，不受影响；但 FAQ 规模上去后会成为下一个同类故障。修法方向：候选集按"该 agent 的 FAQ 全量"取，或调整排序让新 FAQ 有机会进池。
- **`ai_agents` 有 16 条 `agent_code=default_sales` 的重复行**（仅 id=1 被绑定，其余是历史孤儿行）。本轮未擅自删除线上数据。
- **`hybrid_searcher.go` 的 `logSearch` 把 UUID 型 `product_id` 强转 bigint** ⇒ 每次检索都报 `invalid input syntax for type bigint`（`knowledge_search_logs` 写不进去，检索日志缺失）。本轮未修（不影响回答链路），但它让检索可观测性缺失，排查时帮不上忙。
- ~~**发布未执行**~~：**已发布并通过线上四步验收**（2026-10-22）。发布过程踩到两个坑，值得记下来：
  1. **必须交叉编译**：本地 `go build` 出的是 darwin/arm64，直接传上去容器 `exec format error` 起不来（线上 30 秒内用备份 `user-server.prev-20261022-webfaq` 回滚恢复）。正确姿势：`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath`。
  2. **`docker compose up` 不等于发布成功**：user-server 的二进制是 `COPY` 进镜像的（`Dockerfile.user-server`），不是卷挂载 ⇒ 只重建容器会跑**旧镜像里的旧二进制**（本次第一次 `up -d` 后容器内二进制大小仍是旧的 99009904）。必须 `docker compose build user-server` 再 `up -d --force-recreate`，并按**容器内**的特征串（`strings /app/user-server | grep -c tryFAQFastPath`）验内容，不能只看"发布成功"日志。

  另外：共享工作区里本轮改动一度被另一条泳道的提交整片吞掉（`git log --all -S tryFAQFastPath` 查无此串），最终改在**独立 worktree**（`/tmp/hivemtk-website-fix`）重建并提交，避开该竞态。

- 原发布计划（供追溯）：发布后建议按这个顺序验收：① 官网浮标问「介绍下项目」应直接答出项目介绍；② 问「多少钱」「支持哪些渠道」「怎么安装部署」应各自命中对应 FAQ；③ `confidence_signals` 表新记录的 `rag_qual` 应落在 0.2~0.4（不再是 0.13）；④ 显式说「我要转人工」仍应转人工（不被快速通道吞掉）。
