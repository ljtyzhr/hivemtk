# 我方核心模块实现原理纪要（2026-10-24）

> 用途：竞品深度对比文档（DEEP_DIVE）引用。所有坐标均为实读源码所得。
> 路径基准：仓库根 `user-server/internal/...`（简写 `int/`）。

## 1. 渠道接入

**channelgw 网关**：所有传输（HTTP/WS/webhook）收敛到 `IngressPipeline` 接口，四个语义：`IngestBatch`（按 conversation 分组+msg_id 去重+合并 AI 触发）、`PersistHistory`（仅落库不触发 AI）、`ClaimOutbound`（pending→inflight 原子翻转+超时惰性回收）、`AckOutbound`（幂等确认）——`int/channelgw/pipeline.go:19-56`。WS 传输层参数：注册超时 15s、读空闲 90s、帧上限 1MB、推送间隔默认 2s/批 20——`int/channelgw/ws.go:18-26`。

**企微适配**：XML 回调解析（自写 XML parser，`wecomFlatXMLMap`/`wecomXMLParser`）+ 官方 AES 解密（EncodingAESKey、IV=key 前 16 字节）+ URL 验签（echostr 解密回显）——`int/service/webhook_channel_wecom.go:62,92,163,230,274`；access_token 管理 `wecomAccessToken`（:494），媒体异步落库 `persistWeComMediaAsync`（:431）。

**Bridge 浏览器桥接**（我方独有）：Chrome 扩展 `user-web/bridge/`——background 侧 SSE 客户端拉取下发（`src/background/sse-client.js`），内容脚本抓取抖音/快手/TikTok/小红书/闲鱼私信页（`src/channels/*.js`、`src/content/*.js`），拟人输入共享基座在 `packages/browser-core/index.js`（lognormal 延时、贝塞尔轨迹、打字分 burst，A/B 双链路：CDP trusted 输入 vs 内容脚本 contenteditable）。服务端：扩展 POST `/api/bridge/ingest` 上报消息（`int/bridge/handler_http.go:343`），经 `channelgw.IngressPipeline` 入站；下发走 SSE 长连接轮询 outbox（`HandleOutboxSSE`:169，`FetchOutboundUndelivered` 认领欠交付，单次补拉上限 200 `sseBacklogLimit`:303），交付完成后靠 `/ack` 出欠交付集合，ack 分支「轮询不重投、超时未 ack 回欠交付」双收口（:246,254）。租户绑定 `int/bridge/tenant_binding.go`。CDP 远程调试端口 8206（`int/config/ports.go:23`）。

**Telegram 双模式**：webhook 模式 `dispatchTelegram`（`int/service/webhook_channel_telegram.go:74`）带 secret 验签（:658）；polling 模式 `TELEGRAM_POLLING_ENABLED` 开关——显式 1/0 或自动判定（无 public_base_url 时启用），25s 长轮询、limit 100、指数退避 1-30s（`int/service/telegram_polling.go:24-32`）；分布式锁 `telegram_polling_lock.go`（hostname:pid 心跳 60s 陈旧窗口抢占）；启动期 `ReconcileTelegramWebhooks` 对账保证 webhook/polling 不同时消费（StopAll 同步语义:51-60）。群线索挖掘 `mineTelegramGroupLead`（`telegram_lead_miner.go:165`）、DM 外呼 `telegram_lead_outreach.go`。

## 2. 入站编排九步（SmartCSOrchestrator）

文件：`int/service/smart_cs_orchestrator.go`，入口 `HandleIncomingWithAgent`（:268）。九步（机制与 `docs/architecture/AI_CORE_FEATURE_INVENTORY.md` F6 一致）：①`findOrCreateSession` 查建会话，经 `CustomerIdentityService` 做 OneID 合并、群聊 `group:{id}`（:691）；②`saveInboundMessage` 存消息（5s 内容去重窗）；③`isAgentOnline` 在线座席直通（:1125）；④`incrementAIReplyCount` AI 连续回复计数、上限 10 次转人工（:1107）；⑤`isUrgentOrComplaint` 紧急词直转（:1142）；⑥智能体选择（挂载>绑定>默认）；⑦`SalesEngine.HandleWithAgent` 生成；⑧置信度门槛 0.7（有卡片视为达标，`shouldTransferOnLowConfidence`:1194）；⑨`saveAISuggestion` 落库计数（:972）。另有 FAQ 快路径缓存（`lookupFAQAnswerCache`:631，向量缓存命中直接答）、DNC 拦截（`checkDNCBlocked`:813）、Bad Case 留痕（`markBadCase`:904，判据不看步骤名看结构化字段:873-876）。

底层入站管道 `int/service/inbox_ingress.go` `HandleIngressMessage`（:366）顺序：`NormalizeEvent` 归一（SessionID 兜底 `channel:senderID`，:300）→ msg_id 幂等判定（**必须在人工锁之前**，否则回灌声保护失效，:375-410）→ 人工锁定直通（:411）→ 中间件拦截（去重/自回声，拦截仍落库除非 self-echo，:415）→ 落库 → 系统消息仅落库 → 渠道自管 AI 触发标记（防双路径，:478）→ 末条出站/超 5min 窗口不触发（钩子3，:492）→ `triggerAIWithDebounce` 防抖触发。

## 3. Agent ReAct 循环

文件：`int/service/sales_engine_agentloop.go`。核心 `runAgentLoop`（:53）：预算护栏 `agentLoopMaxIterations=5`、`agentLoopTotalTimeout=180s`、`agentLoopMaxPerIterTimeout=60s`（单轮超时重试一次）、`agentLoopMaxTotalTokens=50000`、`agentLoopMaxTotalCostUSD=0.50`（常量在 `sales_engine.go:741-763`、`sales_engine_agentloop_guard.go:39`），由 `newAgentLoopGuard` 统一记账。工具预算 `maxTools=18` 封顶（:546），按 41 具名优先级表裁剪（`reach.history`=39 等，:569）且 `ensureCardShowPresent` 保底 card.show 不被挤掉（:626）。历史按 token 预算截断：`agentLoopHistoryTokenBudget`×(100-outputReserve)%（:148）。每轮 LLM 返回 `finish_reason=tool_calls` 则执行工具再入循环，纯文本即终止；`finish_reason=length` 时 max_tokens 翻倍重试一次；工具被过滤为空时退回直连 Dispatch（:118）。上下文：prompt 拼接 `buildPrompt`（:651）+ RAG chunks + `fetchHistoryWithinTokenBudget`（:798）。

**12 层装饰器链**（`int/aiagent/agent/tooluse/`）：`ChainDecorators`（`decorator.go:64`）倒序包装。内层链 `BuildChainWithBreakerDecorator`（`decorator.go:113-133`）顺序为 **权限→限流→熔断→重试→超时→审计**；外层再叠 **反馈收集**（`executor.go:339`）→ **审批门**（冷触达专用，链外层，被拒不耗限流令牌，:342）→ **风控分级**（比审批门还外层，RiskEnforce 才真拦，:356）。另有独立装饰器件：结果缓存 `ResultCacheDecorator`（`result_cache.go:254`）、参数校验 `ParamValidatorDecorator`（`param_validator.go:20`）、循环防护 `LoopGuardDecorator`（`loop_guard.go:301`，同指纹 60s 内 3 次判环）、双拦截编排器 `DoubleInterceptOrchestrator`（`double_intercept.go:11`，流式首遍工具调用+二遍整合）、死信 `dead_letter.go`、DNC `decorator_dnc.go`。装配接线 `int/app/tool_circuit_breaker_wiring.go`（防止 nil 熔断器静默退化）。

## 4. RAG 管线

**四层缓存短路**：`int/aiagent/rag/retrieval/three_tier.go` `RAGThreeTierService`——L1 热缓存（LRU 1024 条/30min，:80-83）→ L2 暖索引 → L3 冷索引 → L4 关键词兜底（score 固定 0.5，:167），命中即返回，TierStats 分级计数（:38）。

**混合检索**：`hybrid_searcher.go` `SearchIndex`（:139）——pgvector 向量 topK50（:81）+ BM25/关键词 topK30（:82）→ RRF 融合（`RRFK=60`，:87）→ rerank 取 `FusedTopN=20` 精排（:86，`applyRerankToFused`:241，rerank 作用是换量纲而非筛量；失败降级 RRF 归一化名次分）→ `FinalTopK=5` 上限截断（:222，FinalTopK 是上限不覆盖调用方 topK）。reranker 默认 bge-reranker-v2-m3（`user-server/config.yaml:110`），embedding bge-m3 经 TEI。

**查询改写三模式**：`query_rewriter.go:22-24`——`rewrite`/`hyde`/`multiquery`/`hyde_multiquery` 四策略；改写缓存双层（Redis `rag:rewrite:{sha256}` + DB `query_rewrite_cache` 表，:77-91），未命中并行调 HyDE+MultiQuery 生成器；HyDE/MultiQuery 各有 `IsEnabled()` 开关（`hyde_generator.go:49`、`multi_query_generator.go:49`），后两者默认关。

## 5. LLM 调度

**7 场景路由表**：`int/aiagent/llm/dispatcher_register.go`。本地优先模式 `registerLocalFirstRoutes`（:103-131）与云端默认 `registerDefaultRoutes`（:207）：intent_recognize/sop_reply/objection/friendly_chat/long_summary/high_quality/low_cost（+jev_choice）各配 Provider+Fallbacks+CostWeight+MaxLatency+MinQuality；调度时逐候选过滤 not_in_registry/disabled/min_quality/max_latency/circuit_open/rpm_limit（`dispatcher.go:213-256`）。路由改动走 `SetRouteWithAudit` 落库+审计（:226）。

**四级降级链**：`fallback_tree.go:18-38` `LevelPrimary(7b)→LevelSecondary(3b)→LevelCache→LevelTemplate`，DecisionTree 决策；`fallback_chain` 特性旗关闭时直接跳模板（:100-102）；语义缓存 key `llm_fallback:{provider}:{hash}:{prefix}`（:244）；模板按场景轮换文案池（:262-307）。

**熔断器**（双套）：工具级 `int/aiagent/agent/tooluse/circuit_breaker.go`——三态 open/half_open/closed、指数退避 cooldown（`BackoffMultiplier`^openCount，:74-78）；LLM provider 级 `aiagent/llm/provider_failover.go`——`DefaultFailureThreshold=5` 次失败熔断、`DefaultCircuitOpenDuration=60s`（:35-36），参数中心可覆盖，`DegradedLatencyMs=3000` 判延迟降级（:146），Redis 多实例同步（`cache.GlobalIsRedis()` 分支 :437-551）。

**多数投票 N=5**：`aiagent/llm/selfconsistency/selfconsistency.go:30-33`——samples≤0 默认 5，并发采样后聚类；`MultiModelVote`（`dispatcher_dispatch.go:541`）按 bigramJaccard 相似度归簇（:505），`majoritySize=len/2+1`，2/3 一致胜出（:572-583）；high_quality 场景默认开 vote fanout 超时 8s（`dispatcher.go:194-197`）。

**本地推理栈**：llama.cpp/MLX 宿主机直跑（不入容器，`docker-compose.yml:17-19`）——LLM :8207、Embedding :8208、Rerank :8209，启动脚本 `scripts/inference-host/start-{llm,embedding,rerank}.sh`。

## 6. SOP 引擎

**节点型**：`int/service/sop.go:47-92` 共 20 个常量、`SOPNodeSupportedTypes`（:100-110）收录 19 型：结构 2（start/end）+ 销售话术 9（greeting/inquire/introduce/handle/close/invite/follow_up/activate/nurture）+ 控制 3（condition/branch/wait）+ 智能 2（llm/ai_decide）+ 旧兼容 3（message/action/send_offer）+ 新外发 reach_send（双侧一致性有专门校验注释 :108-109）。`ai_decide` 用 LLM 选下一跳（temp=0.3）；wait 四档（timer/customer_reply/external/approval），approval 档挂起=插 `sop_timers` 行、审批结论回写 `ExecutionData._approval_*`。

**调度器**：`int/service/sop_dispatcher.go`——`DefaultSOPDispatcherWorkerCount=16`（:37）、`DefaultSOPDispatcherQueueCapacity=1000`（:38，队列满即拒 `dispatch queue full`:348）、worker 按租户回填 ctx 处理任务（:360-407）。定时器外派 `sop_outbox_dispatcher.go`：`processDueTimersForTenant` 逐租户扫到期 `sop_timers`（:151），认领计数+死信 `bumpTimerClaimOrDeadLetter`（:288），`SOPStuckDetector` 扫卡死执行（:527）。

**幂等与 Saga**：外发节点 `sop_reach_send.go:15` 幂等键 `reach_sent:<exec>:<node>` 防重投二次发送（:127-142 四步：幂等→内容→审批→三判据发送），撤销时幂等键有意保留（:121-124）。Saga 补偿 `sop_compensation.go`：节点实现 `Compensable` 接口（:22-36）+ `CompensationNoter` 自述补偿边界（:41-51），失败不阻断其他补偿、失败可重试。

## 7. 意图/转人工

**级联意图**：`int/service/intent_recognition.go`——三级级联「规则匹配（微秒级，关键词累加 conf 封顶 0.92）→ Embedding 中间层（锚点预计算入 pgvector，:174-272，失败静默退回两级 fail-open）→ LLM 复判（低置信才走）」；粗类 13 + 精细 `intent_recognition_fine.go` 8 大类 27 子类；投机识别 `intent_speculative.go:11` `RecognizeSpeculative`：规则同步返回，LLM 后台 goroutine 异步（buffer=1，60s 收割）升级结果；few-shot `intent_fewshot.go`；指标注册 `intent_metrics.go`。

**五信号聚合与校准**：`int/service/confidence/`。信号采集 `signal_collector.go`；权重 `weighted_aggregator.go:19-23`——Intent 0.30/Entity 0.15/CtxRelev 0.15/RAGQual 0.20/LLMEntropy 0.20，按可用性重归一（:83-85）。校准链：Temperature scaler → Platt scaling（`platt_scaling.go`）→ Conformal（`conformal_predictor.go`+`conformal_calibrator.go`，覆盖率保证）；`aggregator.go:94` `Aggregate` 产出 `dto.ConfidenceDecision`（signal_id/aggregated_conf/dynamic_threshold/decision_band/veto_triggered）。

**六道否决链**：`veto_rule.go:176-196` `NewVetoChain` 固定顺序：VetoExplicit（客户意愿最优先）→VetoComplaint（投诉必人工）→VetoLoop（循环检测）→VetoLowEntity（阈 0.2）→VetoLowRAG（阈 0，参数中心 `confidence.veto_low_rag_threshold` 可覆盖）→VetoHighEntropy（阈 0.2）。

**四档决策**：band 常量 `dto/confidence.go:101-104` handoff/llm_fallback/review/auto；动态阈值 `dynamic_threshold.go` 4 因子计算 clip [0.40,0.95]（:84），按租户意图策略 `threshold_policy.go`；消费侧 `sales_engine_transfer.go:87` switch DecisionBand——handoff 转人工、llm_fallback/review 不转、veto 触发即转（`一票否决: `）。入口 `shouldTransferToHuman`（:16）：显式关键词/转人工词直通，其余走 `shouldTransferByConfidence`（:58）。

## 8. CDP/记忆

**OneID**：`int/service/customer_identity.go`——`IdentifyOrCreate`（:38）+ `LinkIdentity`（:131）跨渠道路由 phone/email/wechat/douyin/xiaohongshu；输入先归一化（手机去 +86/空格/横线、邮箱小写，`customer_identity_normalize.go`）；`unifiedIDFromIdentifiers` 优先级 Phone>Email>Wechat>Douyin>Xhs（:360-379）；合并冲突 `customer_identity_conflict.go`、重试上限 `identity_max_attempts` 默认 8（:385-394）；合并预览/规则 `oneid_merge_preview.go`/`oneid_merge_rule.go`。编排第①步消费（`smart_cs_orchestrator.go:691`）。

**四层记忆**：`int/service/memory_system.go`——L1 短期对话窗口（默认 10 条、TTL 24h，运行时读参数中心 `memory.l1_window_size`/`l1_ttl_hours`，:33-93）；L2 事实（`L2SaveFact`:211，`memory_items` 表）；L2 向量长期（`customer_long_term_memory`，客户上限 500 `L4MaxPerCust`:35）；衰减归档 `memory_decay.go` `RunMemoryDecayJob`（:66）扫陈旧事实软失效+归档；统一读写门面 `memory_facade.go`（Write:72/Read:96 合并 item+vector 双库）。

**RFM/标签**：`int/service/customer_rfm.go`——唯一 RFM 口径入口（:16-19 注释），R/F/M 三维打分（`rfmScoreRecencyCfg`:85 等），`determineSegment` 分层（churn 段可自动入恢复队列 `AutoEnqueueRecovery`:211），cron `customer_rfm_cron.go`；行为标签 AI 打标 `ai_tagger.go`、标签门面 `customer_domain.go:36`；客群圈选 customer.segment 工具（tooluse）。360 视图 `customer_360.go`/旅程 `customer_journey.go`。

## 9. 评测

**rag_eval_auto 闭环**：`int/service/rag_eval_auto.go:47` `RunAutoEvaluation`——取知识库切片 → LLM `generateQuestions`（:87）自动生成问题 → RAG 检索 → `evaluateRetrievalHit` 评 recall/precision（:146）→ 记 `RagEvalRun`/`RagEvalQuestion`；每日 cron `rag_eval_cron.go`（模块闸 AllowWorker("rag_eval") 无租户开通则跳过，:70）。

**dialogue_gold 框架**：`int/aiagent/eval/dialogue_gold.go`——composite = **0.5×chrf + 0.3×keycov + 0.2×action**，动作跳过按 /0.8 重归一（:14）；keyCoverage 必含关键词覆盖（:118）；`RunDialogueGold`（:134）支持 degraded 对照集（同长度校验，:142-145），Bad Case 自动转样本 `BadCaseToDialogueSample`（:213）；chrf 评估器 `chrf_eval.go`、LLM judge `llm_judge.go`+校准 `judge_calibration.go`、RAGAS `ragas.go`。对话评测 cron `service/dialogue_eval_cron.go`。

## 10. 部署形态

`docker-compose.yml`：**容器只跑数据层**——`mtk-postgres`（pgvector/pgvector:pg15，容器内 8202）+ `mtk-redis`（redis:7-alpine，8203），专用网络 `mtk_user_network`（172.32.0.0/16）；user-server（Go）宿主机直跑不入容器，user-web/embed-sdk 宿主机构建后由 user-server 托管（:5-9）；**推理栈三件套宿主机 llama.cpp/MLX**：LLM :8207、Embedding :8208、Rerank :8209（:17-19，`scripts/inference-host/`），CDP :8206（`int/config/ports.go:23`）。一键全栈 `make dev-all`；端口变更须走 `docs/PORT_REGISTRY.md §3.2`+`scripts/audit-cross-package-ports.sh`。开发态 `docker-compose.dev.yml` 含 mtk-user-server-dev 容器。
