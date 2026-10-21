# Bug Assessment: 官网右下角网页客服把正常问题误判为转人工

- **Slug**: website-chat-false-handoff
- **Created**: 2026-10-22
- **Source**: pasted text（用户口述线上现象）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

> 线上，官网右下角网页客服，输入「介绍下项目」先是提示转人工，随后提示人工不在，代码有问题就修复代码，配置有问题就完成配置，数据有问题如资产包智能体等就补全数据，一句话，官网人工客服必须正常正确回答问题。

官网域名 = `xapptool.cn`（nginx `root /www/wwwroot/hivemtk/website`）；客服浮标 SDK 与会话后端 = `work.xapptool.cn`（`user-server`，容器 `mtk-user-server`）。

## Symptom

官网右下角浮标里输入正常业务问题（如「介绍下项目」「我想知道价格套餐，怎么订阅？」），AI 回答不出来，系统先提示「正在为您转接人工客服，请稍候...」，30 秒后再提示「暂无坐席在线，请留言，我们会尽快与您联系」，访客侧体验是"客服坏了"。

期望：AI 用知识库直接答出项目介绍 / 价格 / 渠道等标准问题；只有在真正需要人工介入（投诉、显式要人工、确实答不出）时才转人工。

## Reproduction

线上已复现（2026-10-22，容器 `mtk-user-server`，库 `mtk-postgres` / `user_db`）：

1. 打开 `https://xapptool.cn/`，点右下角浮标（appKey `ak_79Zw7HWzR5wNUnNn6QMnkw8B`）。
2. 输入「介绍下项目」并发送。
3. 会话 `sess_1792599955034342770_db6f6c94` 被判为 `status=waiting` / `handler_type=human`，
   `session_messages` 留下一条系统消息「【系统】AI 置信度不足 (0.62 < 0.70)」。
4. 前端 `ChatWindow.vue` 收到 `transferred=true` 且无 `ai_response` ⇒ 推「正在为您转接人工客服，请稍候...」，
   并启 30s 计时器 ⇒ 推「暂无坐席在线，请留言…」。生产 `online_agent_num=0`，坐席确实不在。

线上信号（`confidence_signals`，同一问题）：

| intent_type | intent_conf | entity_comp | ctx_relev | rag_qual | llm_entropy | aggregated_conf | threshold | band |
|---|---|---|---|---|---|---|---|---|
| ask_product | 0.90 | 1.00 | 0.50 | **0.1344** | 0.50 | 0.6219 | 0.70 | review |
| price_inquiry | 0.82 | 1.00 | 0.50 | **0.1885** | 0.50 | 0.6087 | 0.70 | review |

同一问题换个会话/换句话，意图在 `unknown` / `ask_product` 之间抖动（`intent_conf` 0.30 vs 0.90），
但**只要意图不是 greeting/social，低置信一律转人工** ⇒ 表现为"问题问法一变就转人工"。

## Suspected Code Paths

- `user-server/internal/service/smart_cs_orchestrator.go:437` — `shouldTransfer := salesResp.TransferredToHuman || (knownIntent && !safeIntent && effectiveConf < threshold)`，低置信直接转人工的判据。
- `user-server/internal/service/confidence/signal_collector.go:112` — `computeRAGQual`：`meanScore * coverage`，`coverage = min(1, len(chunks)/5)` 写死 5。
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go:212` — rerank 触发条件 `len(fused) > topK`；`hybrid_searcher.go:235` — `rerankCount == 0` 时 `normalizeRRFScores` 把分数归一到 0~1 相对分。
- `user-server/internal/service/sales_engine.go:296` — `recallRAG` 返回 chunk 的 `Score` 直接进 `resp.RAGChunks`，供 `computeRAGQual` 当"质量"用。
- `user-server/internal/service/sales_engine.go:538` — `layerRouter.Route` 只在 `HandleStream` 里被调用；非流式 `Handle`（网页客服 HTTP 路径走的正是它）**不查 FAQ**。
- `user-server/internal/service/sales_engine.go:165` / `internal/app/*` — `SetLayerRouter` 在生产装配层**从未被调用**，`e.layerRouter` 恒为 nil。
- `user-server/internal/service/layer.go:104` — `featureflag.Get("layer1")` 默认 false，生产 env 未设 `FF_LAYER1` ⇒ Layer1 FAQ 快速通道整体关闭。
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go:276` — `logSearch` 写 `knowledge_search_logs` 时把 UUID 型 `product_id` 强转 bigint ⇒ 每次检索都报错（次要，非本 bug 主因）。

## Root Cause Hypothesis

**高置信度。** 根因是**三个缺陷叠加**，共同把"AI 答得出来"变成"AI 置信度不足 ⇒ 转人工"：

1. **RAG 分数的量纲被压错（主因）**。混合检索在 `rerankCount == 0` 时退回 RRF 归一化分（`normalizeRRFScores`），那是**同一批候选内部的相对分**（量级 0.0~1.0，通常 0.03~0.25），不是语义相关度。而 `computeRAGQual` 把它当绝对相关度用 ⇒ RAGQual = 0.1344。
   rerank 为什么没触发：`hybrid_searcher.go:212` 的条件是 `len(fused) > topK`。本知识库只有 6 个 chunk，`rag_top_k=3` ⇒ `fused=3`、`topK=3` ⇒ **条件不成立，rerank 整段被跳过**。生产日志佐证：`knowledge_search_logs` 插入行里 `rerank_count=2` 但那是另一路（`rag_retrieval.go`），混合检索这条 `fused_count=3, rerank_count=2`……实际线上 `Rerank` 客户端已配置（dashscope `qwen3-rerank`，实测对「介绍下项目」的相似文档给出 0.385/0.307，**高于 `rerankScoreFloor=0.3` 本该通过**），说明是**触发条件**把它挡掉了，不是分数不够。
   更正：日志里 `[Hybrid] logSearch` 那行的 `rerank_count=2` 说明 rerank **确实跑过**；但线上 `rag_qual` 仍是 0.1344 ⇒ rerank 分数写回后又被 `rankMerchantChunks` 乘权重、`computeRAGQual` 再乘一次覆盖率 0.6 折掉。**这条需要 fix 阶段用实测分值钉死，不在 assess 阶段下结论。**

2. **覆盖率口径写死（确定）**。`computeRAGQual` 的 `expectedK` 写死 5.0，而智能体 `ai_agents.rag_top_k=3` ⇒ 覆盖率恒 0.6，再乘一次折扣。**召回条数少不是缺陷，是配置**（top_k=3 是刻意限预算），用它当质量分母是口径错误。

3. **网页客服走的路径根本不查 FAQ（确定）**。`faq_entries` 里有 6 条标准答案，含「HiveMTK 是什么？」（keywords `HiveMTK,是什么,产品介绍,营销系统`），「介绍下项目」按关键词应能命中。命不中的原因是：非流式 `Handle` 路径不调 `layerRouter.Route`；即便调了，`SetLayerRouter` 在生产装配层从未被调用；即便注入了，`FF_LAYER1` 默认 off 且生产未开 ⇒ Layer1 FAQ 快速通道整体不存在。

4. **配置层**：`config_params.smart_cs.confidence_threshold=0.7`（与代码默认一致，未被误改）——**配置本身没问题**，问题在喂给判据的分值不对。

5. **数据层**：FAQ 6 条、知识文档 6 篇（`embed_status=indexed`，向量齐全）、`rag_products` 1 条、`agent_kb_bindings` 1 条（agent 1 ← kb 1）、`channel_agent_bindings` 1 条（web_embed 渠道 ← agent 1）——**数据基本齐全**。唯一缺口是 `knowledge_bases.doc_count=0`，但该列是**已知冗余列**（`internal/service/knowledge_base.go:140` 明示"无人维护"），不计入缺陷。`ai_agents` 存在 16 条 `default_sales` 重复行（id 1/18/21~34），其中仅 id=1 被绑定，其余是历史孤儿行。

## Proposed Remediation

**Preferred**：在**置信度判据的输入口径**上修，而不是抬高阈值或关掉转人工。

1. **让 RAG 分数回到可比的绝对量纲**（`internal/aiagent/rag/retrieval/` + `internal/service/confidence/signal_collector.go`）：
   - 修 `hybrid_searcher.go:212` 的 rerank 触发条件——`len(fused) > topK` 在"候选池本来就小"时永远不成立；应改为对**已有的 fused 池**也做 rerank（池内重排，不要求池大于 topK），保证小知识库也能拿到绝对相关度分。
   - `computeRAGQual` 的 `expectedK` 不应写死 5：改成"本次实际配置的 `rag_top_k`"（或直接去掉覆盖率对绝对分数的连乘，改成独立惩罚项），否则 top_k=3 永远被扣 0.4 倍。
2. **给网页客服的 HTTP 路径接上 FAQ 快速通道**（`internal/service/sales_engine.go` + 装配层）：
   - 在非流式 `Handle` 入口也走一次 `layerRouter.Route`，命中 FAQ 就直接返回标准答案（跳过 LLM 与 RAG），零 Token、答案确定。
   - 装配层真正 `SetLayerRouter`（`internal/app/`），并让 `FF_LAYER1` 在生产可开（默认 off 是安全默认 ⇒ 走 env 显式开启，不改默认值）。
3. **转人工判据分层**（`internal/service/smart_cs_orchestrator.go:437`）：
   - 现在的判据把"低置信"直接等同于"转人工"。应区分 `BandHandoff`（真该转）与 `BandLLMFallback`/`BandReview`（应先走 LLM 兜底/审核）。生产信号里 0.62 分被判 `review` band 却仍被转人工，说明 band 判定结果**没有参与**编排器的转人工决策——两者口径脱节。
4. **兜底文案**（`user-web/src/views/chat/embed/ChatWindow.vue:227`）：
   - 30s 后固定推「暂无坐席在线」是对的（生产确实 0 坐席），但转人工前应先确认 AI 没能给出可用答案——现在的 `transferred=true` 分支不区分"AI 答了但置信度低"与"AI 完全答不出"。

**Alternatives**:
- *调低 `smart_cs.confidence_threshold` 到 0.6*：一行改动、见效快，但把 0.62 判为"可自动回复"等于给一个**量纲错误**的分放行，治标不治本，且 RAGQual 长期在 0.13 徘徊 ⇒ 阈值要一路调到 0.4 才能盖住，代价是质量闸门整体失效。**不推荐**。
- *只在生产把 `FF_LAYER1` 打开*：能让 FAQ 命中，但 `SetLayerRouter` 从未被调用 ⇒ 打开也无效果，且非流式路径仍不查 FAQ。**单靠配置不成立**。

**Files likely to change**:
- `user-server/internal/service/confidence/signal_collector.go`
- `user-server/internal/aiagent/rag/retrieval/hybrid_searcher.go`
- `user-server/internal/service/sales_engine.go`
- `user-server/internal/app/`（装配层：注入 LayerRouter）
- `user-server/internal/service/smart_cs_orchestrator.go`
- `user-web/src/views/chat/embed/ChatWindow.vue`（兜底文案口径）

**Tests to add or update**:
- `computeRAGQual`：top_k=3 的配置下，召回 3 条高分 chunk 不应被覆盖率折成 0.6 倍（口径回归）。
- 混合检索：小知识库（候选 ≤ top_k）时 rerank 仍执行，chunk.Score 是绝对相关度而非 RRF 相对分。
- 非流式 `Handle`：命中 FAQ 时直接返回标准答案，不调用 LLM。
- 编排器：`BandReview`/`BandLLMFallback` 的低置信**不**直接转人工；`BandHandoff` 才转。
- 端到端：网页客服问「介绍下项目」返回项目介绍，**不**产生转人工会话。

**Risks & Considerations**:
- 改 RAGQual 口径会**整体抬高**置信度 ⇒ 历史"低置信转人工"策略整体放松；需确认这是期望行为（当前线上阈值 0.7 实际把 RAG 正常的问题也拦了）。可在改后用 `confidence_signals` 历史数据回算分布。
- 接 FAQ 快速通道会让一部分问题**不再产生 RAG/LLM 调用** ⇒ 成本下降，但 `ai_suggestions` / `confidence_signals` 的采样口径会变（FAQ 命中不再落置信度信号），需在 fix 阶段确认是否要仍落一条。
- 装配层注入 LayerRouter 属于跨包改动 ⇒ 需跑 `make lint` / `make vet`（含 depguard）。
- 生产发布需重启 `mtk-user-server` 容器；`FF_LAYER1` 是 env ⇒ 需在 compose/env 中持久化，否则容器重建即失效。

## Open Questions

- [NEEDS CLARIFICATION: 生产 `knowledge_search_logs` 显示 `rerank_count=2`，而 `rag_qual` 仍只有 0.1344 —— rerank 分数写回后到 `computeRAGQual` 之间还经过了 `rankMerchantChunks`（乘 chunk 权重）与覆盖率连乘。fix 阶段必须用实测分值逐段核对，确认「rerank 已跑但分被折掉」这一判断成立，再决定改哪一段。]
- [NEEDS CLARIFICATION: `ai_agents` 有 16 条 `agent_code=default_sales` 的重复行（仅 id=1 被绑定）。是否为历史脏数据需要清理？本次不擅自删除线上数据。]
