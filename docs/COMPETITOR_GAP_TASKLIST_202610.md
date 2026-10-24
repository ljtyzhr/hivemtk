# HiveMtk 竞品缺口改进任务清单（2026-10-24，v4 二次审核版）

> **v4 增量**（2026-10-24 二次审核）：①**批 1 三项经源码复核已确认交付**（P0-3 注码 G13 落地、P0-1 的 `NewDialogueEvalCron`+`controller/dialogue_eval.go`+gold 61 条已装配、P0-2 前置门槛已改读 per-intent policy 单一来源）——本清单相应条目改**验收模式**（交付状态 ≠ 验收状态，AC 反向测试是否在库待逐条核，见各条）；②收编 DEEP_DIVE 回灌 5 条（父子分段小刀独立任务、PARSERS 二级分发并入 P1-1、CSV 导入导出并入 P0-1、T4 QA 开关与两条架构观察项入观察项表）；③口径统一回改（工具数 45、装饰器「链上 9 层+独立件 6」、`BadCaseToDialogueSample` 行号 :213、gold 实数 61）；④G 编号消歧注记（本文件 G3/G4/G9/G12 指 marketing-features 在册缺陷，与 CHECKLIST 清单 G1-G5 评测行号不同指）。
>
> **v3 增量**（2026-10-24 二次头脑风暴+代码级复核）：发现 v2 五处盲点并修正——
> ①P0-2 的 AC 有**方向性缺陷**：`threshold_policy.go:122-136` 已存在 per-intent 阈值表（complaint 0.85/objection 0.75/ask_product 0.70…），而 transfer 前置门槛是**通用 0.7**——同一仓库两套阈值数字并存，且聚合器走 `aggregator.go:136 GetPolicy` 消费的是前者。真问题不止「一票否决」，还有「**双阈值源失同步**」（改 policy 表不会改 transfer 门槛）。AC 必须补「门槛与 per-intent policy 对齐或显式声明独立」一条。
> ②P0-1 的 cron 接线**比 v2 写的更小**：rag_eval 先例是 `main.go:572 NewRagEvalCron()` 一处装配 + controller 已有完整 HTTP 面（`controller/rag_eval.go`：run/list/get 三端点 + manage 面）——dialogue eval 照抄形状即可，v2 写的「前端看板」应降级为「复用 rag-eval 三端点形状」，前端可后置。
> ③P0-1 补新证据：gold 框架还带 **bad-case 送料口**（`dialogue_gold.go:179 BadCaseToDialogueSample`，上游 `service/bad_case.go` G-2 闭环 + `controller/bad_case.go` 已有 HTTP 入口）——扩集不是从零写 50 条，**bad_case 导出转 gold 是现成管道**，任务量再降。
> ④P1-1（文档解析）的「可插拔解析器 seam」**现状为零抽象**：调用点是 `knowledge_service_pipeline.go:45` 直调 `etl.ExtractText`，且**失败即退化原文**（`extErr → string(bytes)`）——即扫描件 PDF 现在会把二进制垃圾当内容入库。P1-1 的第一刀应是「失败路径改 markFailed」而非外挂解析器（同文件 :53 已有「疑似扫描件」口径，但 :47 的退化分支先于它吃掉一切提取失败）。外挂解析器顺位降后。
> ⑤批 3（P1-1）依赖关系修正：解析失败退化是**正确性 bug**（错误内容入库污染向量库），应升 P0 与批 1 并做，不等外挂解析器。
> 其余 v2 内容（P0-2 收窄结论、工具数 45 统一、G4/G12 改写）经复核无误。

> **输入**：`COMPETITOR_FEATURE_CHECKLIST_202610.md`（v1 修正版：A10 帮助中心、G1 RAG 评测集经磁盘复核已存在，移出缺口）。
> **v2 增量**（2026-10-24 深化）：①P0-2 经源码复核**任务收窄**（五信号已接线，真残点=transfer 前置硬编码门槛 `sales_engine_transfer.go:29`）；②P0-1 经源码复核**任务重定义**（对话级 gold 框架已存在 `dialogue_gold.go`，残点=无生产接线+gold 集 8 条过小——从「新建」改为「接线+扩集」）；③工具数口径统一为 45（`Build*Tools()` 七组实测）并已修 marketing-features 旧数 42；④marketing-features 的 G4/G12 两条已同步改写为准确残点。
> **论证方法**：每条缺口先做「值不值得做」论证（竞品参照 + 我方形态适配性 + 排除理由），再落成任务（范围/AC 验收/依赖）。
> **优先级口径**：P0=影响核心卖点可信度或已有在册缺陷（G 系列）｜P1=可感差距、半年内应补｜P2=与产品定位冲突或收益低，明确不做或远期。
> 每条 AC 可测；实施按 CLAUDE.md 规则 3 走 spec-kit（Bug 车道对应在册缺陷，新功能车道对应新增）。

---

## P0-1 对话级端到端评测：接线与扩集（对应在册 G3 + G12）——**2026-10-24 二次审核：cron/HTTP 接线已交付，状态改验收**

**源码复核（二次审核轮）**：`main.go:577` 已装配 `NewDialogueEvalCron()`；`controller/dialogue_eval.go` 存在（三端点形状）；`dialogue_gold.json` 已 **61 条**（v3 写 8 条系旧读数，意图类覆盖待对账）；`service/dialogue_eval_cron.go` 已在册——v3 断言「无 cron/HTTP 入口、表无写入方」已过期。

**待验收残点（按 v3 AC 逐条核对交付证据）**：
1. cron 是否带租户门（rag_eval 先例 AllowWorker 形状）——`dialogue_eval_cron.go` 需核；
2. bad_case 管道真实使用（gold 中 sample.CaseID 可追溯条数）与 61 条的意图类/否决链覆盖计数（对账脚本是否在库）；
3. `intent_hit` 维度是否已入 Details JSON + 单测；
4. HTTP 面是 manage 前缀双份还是单份（对照 `rag_eval.go` 形状）；
5. **CSV 导入导出（DEEP_DIVE 回灌）未落地**——补：评测集 CSV 模板导入/导出端点（对标 Dify 批量评测形态），与 bad_case 管道并行入口；
6. degraded 对照行是否随正常行写入 `dialogue_eval_runs`。

**AC（v4 验收口径）**：
- [x] 上述 6 项逐条核验（2026-10-24 实跑）：
  1. **租户门** ✅ `service/dialogue_eval_cron.go:68` `AllowWorker(ctx, "dialogue_eval")` + `tenant.Fanout` 两段与 `rag_eval_cron.go` 同形状；
  2. **bad_case 管道** ✅ 运行期送料（`dialogue_eval_service.go` `dialogueGoldSamples`，`sample.CaseID=b.ID` 可追溯，`TestRunGoldEvaluation_BadCaseFeed` 绿）；静态 gold 61 条 `case_id` 为空属预期（bad_case 是运行期构造，不入静态文件）。覆盖计数对账**闸已补**：`internal/aiagent/eval/dialogue_gold_coverage_test.go`（61 条/20 场景；objection 五类各 ≥2；核心场景各 ≥2；ID 唯一；带 degraded ≥2），变异（删 objection_timing 全类、61→58 仍 ≥50）实测红；
  3. **intent_hit** ✅ 入 Details JSON（`TestRunGoldEvaluation_IntentHitRate` 断言 `strings.Contains(raw,"intent_hit")`）；
  4. **HTTP 面** ✅ 单份三端点（`/api/dialogue-eval/{run,runs,runs/:id}`），无 manage 前缀双份（rag_eval 有多一份 manage，此处单份即 AC 要求的形状复用）；
  5. **CSV 导入导出** ✅ 本次补做：`aiagent/eval/dialogue_csv.go`（规范形编解码，列序 `id,scene,label,fix_layer,case_id,turns,degraded`）+ `model.DialogueEvalSet`（表 `dialogue_eval_sets`，已注册 `pkg/db/migrate.go`）+ `service/dialogue_eval_set.go` + 四端点 `GET /dialogue-eval/sets/template`、`POST /dialogue-eval/sets/import`、`GET /dialogue-eval/sets`、`GET /dialogue-eval/sets/:id/export`；`RunRequest.eval_set_id` 可对导入集跑（`RunGoldEvaluationWithSet`）。
  6. **degraded 对照行** ✅ 与正常行同表（`phase` 落 Name 后缀 `-baseline`/`-degraded`，`TestRunGoldEvaluation_WritesBothPhasesAndDegradedDrops` 绿）。
- [x] CSV 导入导出端点 + 测试（导入 2 条→ListEvalSets/loadEvalSetSamples/RunGoldEvaluationWithSet 三路读回→导出与导入**逐字节相等**；`aiagent/eval/dialogue_csv_test.go` 另覆盖错表头/turns 非 JSON/空 id/重复 id/无 user 回合/无数据行六类整体拒绝）
- [x] `make audit`：**整链 rc=0**（2026-10-24 当日复跑）。首跑时红在他泳道 `payment.go:227` 的未提交 `Exec`，该泳道收口后转绿；期间本批相关门均逐条 rc=0。
- [x] `marketing-features` G12 已改写为实际状态并引用本条（2026-10-24）

**依赖**：无。

## P0-2 置信度两套体系打通（在册 G4）——**2026-10-24 二次审核：单一阈值源已交付，状态改验收**

**源码复核（二次审核轮）**：`sales_engine_transfer.go:30-40` 前置门槛已改读 `confidenceAggregator.GetPolicy(ctx, intentType).BaseThreshold`（聚合器不可达时 fallback 0.70 保持既有行为），注释标 G4/P0-2——v3 的「对齐 GetPolicy vs tunable」二选一已有代码答案：**对齐 GetPolicy**。

**待验收残点**：
1. 对齐反向测试是否在库：把 objection policy 调低后，objection 类高意图置信用例走五信号聚合可触发否决（专测双源失同步传导）；
2. 行为锁定测试（默认阈值下现有 transfer 用例全绿）；
3. CtxRelev Embedder 升级评估结论（做/不做+理由）v3 要求写入本文件，**未见**——仍欠；
4. tunable 旋钮门（参照 T4 配额 4 旋钮先例）是否加。

**AC（v4 验收口径）**：
- [x] 残点 1 对齐反向测试 ✅ **本次补做** `service/sales_engine_transfer_policy_sync_test.go`：DB 写 objection 策略行（version=2 覆盖内置 1）→ `GetPolicy` 读回 0.95/0.55 → 门槛行为双向跟随（0.95 时 0.80 置信通过前置门进入聚合并因 `BandHandoffUpper=0.99` 判 handoff；0.55 时被前置门拦、返回 `(false,"")`）。变异（把门槛退回独立常量 0.70）实测红。
- [x] 残点 2 行为锁定 ✅ 默认口径下 `objection` 档 0.75、0.80 置信被前置门拦（`TestTransferGate_DefaultPolicyBehaviorLocked`）；既有三格 `sales_engine_transfer_gate_test.go`（after_sale 0.80 放行 / price_inquiry 0.65 拦截 / 投诉豁免）全绿。
- [x] 残点 3 **CtxRelev Embedder 升级评估结论（做，降为 P1 小刀，不进批 1）**：
  - 现状：CtxRelev=cosine(query, 最近 3 轮均值)，权重 0.15（`confidence/weighted_aggregator.go:21`），embedder 为 `embedding.LocalEmbedding` 1024 维随机投影（字符 n-gram 哈希→固定投影矩阵 seed=42，`aiagent/embedding/embedding.go:92`）——确定性、零依赖、无网络，但只有词形重叠，无语义。
  - 判据方向：误判代价不对称——词形重叠漏掉同义改写（「多少钱」vs「价格怎么算」）会压低 CtxRelev ⇒ 综合分更低 ⇒ 更倾向转人工/降级。**保守方向，不产生「该转人工却自动回复」的风险**。
  - 升级方案（若做）：把 `service/confidence_init.go:61` 的 nil 回退换为知识库同一套本地 Embedding 服务适配器（`llm.EmbeddingService`），保留本地投影为不可达兜底；新增「embedder 报错 → CtxRelev 标 unavailable、可比重归一」行为测试（`weighted_aggregator.go:85` 已按 Available 掩码加权，断言该路径）。
  - 代价：每轮多 1 次本地 TEI 调用（~10ms，与知识库共用端点，不引入外网）；多一条降级路径要测。
  - 结论：**做，但不进批 1**——批 1 是验收车道，这条属行为变更（会移动历史评测读数基线），应等 P0-1 的 dialogue eval 基线跑出第一份读数后再切，否则无法区分「embedder 换了」与「prompt/模型变了」的分数变化。
- [x] 残点 4 tunable 旋钮门 ✅ **已有，不再加**：`PUT /api/admin/tuning/confidence/policies`（`router/system_routes.go:200`）→ `tuningService.UpsertThresholdPolicy`（version 递增 + `SaveActive` + 按租户 `RefreshThresholdPolicies`）→ 单测 `tuning_confidence_test.go:40`。DB policy 表**就是**这个阈值的旋钮面；再加 env/flag 旋钮会重建第三套数字（本条目要消灭的正是这个）。
- [x] `make audit`：**整链 rc=0**（2026-10-24 当日复跑）。首跑时红在他泳道 `payment.go:227` 的未提交 `Exec`，该泳道收口后转绿；期间本批相关门均逐条 rc=0。

## P0-3 解析失败退化为原文入库（G13）——**2026-10-24 二次审核：已交付，状态改验收**

**源码复核**：`knowledge_service_pipeline.go:55-66` 已实现 `etl.IsPlainTextExt` 分流——非纯文本后缀提取失败直接 `markFailed`（失败信息含「文本提取失败（可能为扫描件或损坏文档）」），仅纯文本类保留退化原始字节；图片走 VL 描述独立分支。实现刀已落地（注释标 G13）。

**待验收残点**：
1. 伪 PDF 反向测试是否在库（非 PDF 魔数字节→断言 markFailed、DB 无对应 chunk 行）——grep knowledge/service 相关测试；
2. DB 侧对账口径：文档状态字段/常量名（验收时查哪张表哪个枚举值）。

**AC（v4 验收口径）**：
- [x] 反向测试在库且红绿可复现（变异：摘掉 markFailed 分支该测试必红）——`service/knowledge_pipeline_g13_test.go` `TestProcessDocumentAsync_FakePDFMarksFailedNoChunks`：伪 PDF（合法 xref 零文本页）→ embed_status='failed' + error_msg 含「文本提取失败」+ knowledge_chunks 零行；变异（`if false &&` 摘分支）实测红、还原后绿（2026-10-24 本机 PG 8202 实跑）
- [x] 纯文本退化路径有行为锁定测试——同文件 `TestProcessDocumentAsync_PlainTextFallbackPreserved`（可读 .txt 正常提取入库，防 G13 分支误伤纯文本）
- [x] `make audit`：**整链 rc=0**（2026-10-24 当日复跑；首跑时红在他泳道 `payment.go:227`，该泳道收口后转绿）

**DB 对账口径（验收时查哪张表哪个枚举值，已核出）**：文档状态=`knowledge_documents.embed_status`，失败枚举值 `'failed'`（`model.EmbedStatusFailed`），错误信息落 `knowledge_documents.error_msg`；chunk 对账表=`knowledge_chunks` 按 `document_id` 计数。测试注记：chunk 表的 `embedding`/`embed_status` 列不在 gorm model 上（走 `UpdateEmbeddingsBatch` 裸 SQL），测试夹具需手工 `ALTER TABLE` 补列。

## P1-1 文档解析升级：表格版面 + 图片知识库（对应缺口 C4）——v3 修订：首刀改向

**论证**：竞品参照——RAGFlow DeepDoc（OCR+表格重建+版面分析，复杂版式准确率约 92%，20+ 格式）【中】；FastGPT 图片知识库（视觉模型生成描述索引，商业版）【高】。行业共识「解析是天花板：解析不对，链路全废」【中】。
**v3 修订（源码核）**：我方现状比 v2 描述更弱——解析调用点 `knowledge_service_pipeline.go:45` **直调 `etl.ExtractText`，零抽象零 seam**；PDF 走 `ledongthuc/pdf` 纯文本提取（无 OCR/版面），扫描件直接失败（且失败后退化入库的 bug 见 P0-3）。所以「留 seam 接外挂解析器」是**真新建**而非 v2 暗示的「已有接缝」。
**适配性**：我方是 Go 单体+本地推理栈，自研 DeepDoc 级视觉解析不现实；正确路线是**外挂解析引擎**（RAGFlow/MinerU sidecar，「接口统一、实现可换」【中】）或本地 VL 模型（Qwen2.5-VL 系）做图片→描述索引——后者与「Embedding/Rerank 强制本地」隐私卖点同构。**前提：P0-3 先落地**（否则解析质量提升无评测基线可对）。

**任务范围（v4 修订：吸收 DEEP_DIVE 回灌两条）**：
0. **前置小刀——父子分段**（独立可先落地，对标 Dify parent-child chunk，预估 1-2 天）：
   - 表/结构：分块产物加 `parent_chunk_id`（`pkg/db/migrate.go` 注册迁移；短文档单块时 parent 置空）；
   - 回拼层：`rag/retrieval/hybrid_searcher.go` 检索命中后、返回 `SearchIndex` 结果前回拼父块（父块超 token 上限时截断到命中块±窗口）；
   - 行为闸：父块不存在（历史数据/短文档）时行为与现状逐字节一致；
   - AC：长文档命中小块时 prompt 含父块上下文；短文档行为回归全绿；`make audit` 绿。
   - **落地记录（2026-10-24 实现完成）**：
     - 父块独立表 `knowledge_chunk_parents`（`model.KnowledgeChunkParent`，已登记 `pkg/db/migrate.go`）——不放 `knowledge_chunks`：那张表是向量/关键词 SQL、chunk 计数、发布制可见性的对象面，塞父行要么处处加过滤漏一处即错，要么父块被向量化成重复命中；独立表让既有查询面零改动（论证见模型注释）。
     - 子块 `knowledge_chunks.parent_chunk_id`（gorm model + AutoMigrate；本包手写 DDL 测试夹具已同步该列，`TestKbGate_TestSchemaCoversModelColumns` 实跑绿）。
     - 生产侧 `service/knowledge_service_parents.go`：窗口 3 块、父块字符上限 1800（= etl 默认 ChunkSize 512×3+余量；**按字符不按字节**——`strings.Builder.Len()` 是字节数，中文一字三字节会把正常窗口误判超限，此坑实测踩过一次）；单块文档不建父块；超限窗口跳过、其余窗口照建；失败按 `markFailed("写入父块失败")` 整篇失败（半套状态比整篇失败更难查）。
     - 检索侧 `rag/retrieval/hybrid_searcher_parent.go`：`finalK` 截断后按 `parent_chunk_id` 一次 `IN` 查询回拼；父块缺失/超 `parentStitchMaxChars`/db 空 → 该条或整批原样返回。三条检索 SQL（向量 / tsvector / ILIKE 兜底）均已选 `parent_chunk_id` 列。
     - 删除级联：`KnowledgeService.Delete` 同时删父块（子块删了父块留着就是一组回拼不到的孤儿）。
     - 测试：生产侧 3 格（多块建窗与窗口映射 + 父块含子块全文 / 单块无父块 / 全窗口超限跳过）；检索侧 5 格（父块替换 Content 且 ID/分数不动 / 无父块逐条不变 / 悬空父块原样返回 / 超限父块不回拼 / 无 db 不 panic）；变异两刀实测红（摘回拼调用 / 摘子块 ID 回填），还原 md5 比对一致。`./internal/aiagent/...` 全绿。
     - 遗留口径：父块不进计数（`chunk_count` 只算子块），不发版本戳（命中面已过 kbrelease 过滤）。
     - 覆盖边界：只接 `KnowledgeService` 主库链路（`/api/knowledge-bases` 的 `knowledge_chunks` + hybrid_searcher）。`KnowledgeBaseService` 的导入链路（`knowledge_base_import.go`，内存 indexer + `kb_documents` 表）不写 `knowledge_chunks`、不走 hybrid_searcher，**不在本次范围**；该链路的父子分段要等它并入同一检索面后再做。
1. 解析器接口抽象（`knowledge_pipeline` 处留 seam），**预留引擎选择二级分发**（对标 RAGFlow PARSERS：默认 etl 现实现，可切外挂引擎），默认现实现；
2. 可选外挂解析服务（HTTP 调 RAGFlow/MinerU，部署文档给出 sidecar compose 片段，默认不启用）；远期收益注记：sidecar 选配 RAGFlow 可顺带获得 RAPTOR/GraphRAG，零自研；
3. 图片知识库：VL 模型生成文本描述→入库向量索引（走本地推理栈端口规划，新增 :8210 或复用）；【注：二次审核见 `etl.IsImageExt`+`DescribeImage` 分支已在代码出现，开工前先核交付状态与未覆盖面】
4. 表格感知分块选项（启用外挂解析时生效）。

**AC（2026-10-24 批 3 验收）**：
- [x] 外挂解析后表格单元格召回可用（自动化代理判据）——`etl/table_chunking_test.go`：正文+表混排的 10 行 4 列表，断言**每个单元格值都完整出现在某个 chunk**、任何 chunk 里不出现被切断的表行、表格块带 `chunk_type=table` 标记；超长表按行拆且每片重复表头+分隔行（`TestTableAware_LongTableSplitsByRowWithHeaderRepeated`）。**口径说明**：真实「3 页扫描件 PDF」需外挂引擎与测试文件，此处用外挂引擎的标准输出形态（markdown 表格）做等价夹具；单元格召回的下游（向量/关键词检索）由既有检索链路覆盖，不在本用例重复 mock。
- [x] 图片条目入库后可被文本 query 命中（链路代理）——`TestImageDescribe_E2E_DescriptionBecomesSearchableChunk`：假 VL 服务（OpenAI chat 兼容）→ `DescribeImage` 返回描述 → 文本管线分块，断言关键词（`2027-06`、`HiveMtk`）落在 chunk 内容里。真实 query 命中依赖 embedding/rerank 服务，属既有 RAG 用例面。
- [x] 未启用外挂时行为与现状逐字节一致——`TestTableAware_OffMatchesLegacyByteForByte`（关闭时逐块比对改造前三条策略的输出，内容与 chunk ID 都比）；且 `TableAware` 默认值由「是否配置 sidecar」推导（`NewDocumentProcessor(nil)`），默认部署下走原路径。知识库全包回归（controller/repository/service/kbcrawler）全绿。
- [x] 隐私口径：默认零外网请求——`TestSidecar_E2E_TableOutputChunksKeepRows` 的**极性对**：配置 URL 时 fake sidecar 命中 1 次（证明探针有效），清空后命中数不增（证明默认不外发）；文档已声明（见下条）。
- [x] **状态核验（本批确认已交付，不重做）**：parser seam `etl.ParseDocument`（builtin/sidecar 二级分发、sidecar 失败**不回退**内置、纯文本直通内置）、图片分支 `etl.IsImageExt`+`DescribeImage`（默认禁用→按不支持格式 markFailed）均已在库且有测试；本批补的是**表格感知分块**（`etl/table_chunking.go`，spec 步骤 4 的唯一缺口）＋部署文档的 compose 片段（`docs/DEPLOYMENT_GUIDE.md`「外挂解析与图片知识库」）＋`.env-example` 三个键。
- [x] 变异三刀实测红并还原（md5 比对）：①表格块当正文处理（单元格被切断）②拆表后不重复表头 ③忽略 `TableAware` 开关恒走新路径。

## P1-2 OpenAPI 声明式工具导入（对应缺口 B3）

**论证**：竞品参照——Dify「OpenAPI schema 导入即成工具」是自定义工具接入的最低摩擦形态【高】。我方已有 MCP Server + 45 内置工具，但用户加自有业务工具（查库存 ERP 等）需要写 Go 代码。补此面后「护栏优势（B5）+ 低摩擦接入」合并成完整故事。

**任务范围**：
1. OpenAPI 3.x 解析（go-openapi 库）→ 自动生成 `Tool` 实现（Name/Schema/Handler=HTTP 转发）；
2. 生成的工具自动套装饰器链（链上 9 层+独立件 6，与内置工具同权治理——这即是相对 Dify 的差异点）；
3. 工具注册表持久化（表 `custom_tools`：schema 来源/鉴权凭据 secrets 包接入）；
4. 前端管理页（user-web）：贴 schema → 预览 operations → 勾选导入。

**AC（2026-10-24 批 2 验收）**：
- [x] 导入后经 Agent 调用链执行成功（E2E）——`tooluse/openapi_tools_decorator_test.go`：导入规范 → 注册 → `ToolExecutor.Execute` → httptest 上游，断言路径参数替换与响应回读。**口径说明**：用本地等价规范（单 operation）做夹具而非公网 petstore——CI 无外网，拉 petstore 会成为抽奖式红；协议面（解析/Schema/转发）与 petstore 同构。
- [x] 导入工具的调用过审计+限流装饰器——同测试断言 `AuditLogger` 有记录且条目工具名一致、`RateLimiter.Acquire` 收到工具名；机制上装饰器链挂在 `ToolExecutor.buildHandler`（按工具注册，不按来源），导入工具与内置工具同一条链。
- [x] 凭据不明文落库——`custom_tool_boot_test.go` 断言落库值带 `enc:v1:` 前缀且不含明文片段、`dbencrypt.Decrypt` 还原后上游收到原始 Bearer 头。
- [x] `make audit`：**整链 rc=0**（2026-10-24 当日复跑，见执行记录）。
- [x] **额外修掉两个真缺陷（本批新发现）**：
  1. **重启后导入的工具全部消失**：`RegisterToolProvider` 只写进程内切片，启动装配不读 `custom_tools` 表，而服务层注释声称"重启由 GetAutoRegisteredProviders 恢复"——**该说法不成立**。新增 `tooluse.LoadCustomToolProviders` + 装配点（`app/tool_provider_wiring.go`），逐行容错（坏行跳过、好行照装）；变异（不读表）实测红。
  2. 服务层那句过期注释已改写为实际机制。
- [x] **前端管理页已补**：`user-web/src/views/aiAgent/CustomIntegration.vue`（OpenAPI 工具集 + 外部 MCP 服务两页签）＋`api/customIntegration.js`＋路由 `/aiAgent/integrations`＋菜单节点 `aiCustomIntegration`（`model/menu.go` 静态注册表）＋9 语言词条（i18n 门 resolved_ratio 35.01%→35.9%）；`npm run build` 通过、ESLint 0 error。

## P1-3 MCP 客户端侧（对应缺口 B7）

**论证**：竞品参照——Dify/FastGPT 均双向 MCP【高】。我方只有 Server（暴露自身给外部）。补客户端后，ReAct Agent 的工具池可动态挂外部 MCP server（与 P1-2 组成「声明式接入双通道」）。工程量小（协议已实现一半）。

**任务范围**：JSON-RPC 2.0 client + tools/list 发现 → 动态注册为带装饰器的工具；配置面（user-web）管理 server 列表。

**AC（2026-10-24 批 2 验收）**：
- [x] tools/list 发现并可在 Agent 调用链中执行——`mcp/e2e_test.go`：httptest 里跑**本仓 mcp.Server**（真 JSON-RPC），客户端真 HTTP 往返 → `LoadConfiguredServers` 发现 → `ToolExecutor.Execute` 调用成功，且断言审计+限流装饰器开火。**口径说明**：执行面验到 ToolExecutor（AgentLoop 的实际派发口），未跑完整 AgentLoop 回合（那需要 LLM 与 session 夹具，属另一层）。
- [x] 不可达 server 熔断降级不影响其他工具——同文件第二格：一个活的 + 一个 `127.0.0.1:1`，断言接入数 1/1、死行 `last_status=error` 且活工具可调；另 `TestE2E_CircuitBreakerOnDeadServer` 验 `FailLimit=1` 第二次直接命中熔断（不再发网络请求）。
- [x] `make audit`：**整链 rc=0**（2026-10-24 当日复跑）。
- [x] **额外修掉的真缺陷：客户端从不发 `initialize`** —— 协议要求先握手，合规 server（含本仓 `mcp.Server`）在未握手时对 `tools/list` 回 `-32002 server not initialized`。换言之原实现**对任何标准 MCP server 都不可用**，而单测因 mock 掉传输层没发现。已补 `EnsureInitialized`（initialize → notifications/initialized，失败不缓存可重试）＋ `Mcp-Session-Id` 透传；变异（跳过握手）实测红。
- [x] **配置面已补（后端+前端）**：表 `mcp_servers`（`model.MCPServer`，已登记 AutoMigrate）＋`service/mcp_client_service.go`（建/列/删/启停/测试，保存即发现）＋5 端点 `/api/mcp-servers*`＋启动发现 `mcp.LoadConfiguredServers`（逐 server 隔离）＋前端页签（同上）。租户隔离由 tenancyidx 全局 callback 覆盖（不手写 Where；`TestMCPClient_ListTenantScopedInMultiTenantMode` 守这张新表确实被覆盖）。

## P1-4 模型管理界面增强（对应缺口 D1）

**论证**：竞品参照——Dify 100+ 提供商管理+用量看板【高】。我方有 LLM 路由可视化配置页（七场景），缺**提供商/密钥/用量**集中面。注意边界：不追「100+ 提供商清单」（那是平台型产品面），只做「多提供商注册 + 密钥管理 + 按 token 用量统计」三件。

**AC（2026-10-24 批 4 验收）**：
- [x] 提供商增删改（凭据走 secrets）——后端早在库（`POST/PUT/DELETE /api/llm/models`、`LLMRoutingService.Create/Update/DeleteModel`、`providers` 落库 + `provider_crypto.go` 加密、启动 `LoadProvidersFromDB` 回装）。**本批补验收测试** `service/llm_provider_crud_test.go`：建→列表可见→改（留空保留原值、密文语义按解密后比对）→删（行与内存态双清）；另覆盖 `__CLEAR_API_KEY__` 清空标记；变异（关掉加密）实测红。
- [x] 用量按日/按场景聚合看板——后端 `/api/llm/usage/daily?window=&group_by=day|day_scenario|day_provider` 早已存在但**前端零消费方（数据面空转）**；本批在成本统计页签补「按日用量趋势」卡片（日期/场景/提供方/调用/失败/Token/平均延迟，维度单选），`api/llmRouting.js` 加 `getDailyUsage`；`npm run build` 通过，契约门调用数 1106→1107。
- [x] 场景路由页与提供商页联动——已在同一页签组（模型列表 / 场景路由配置 / Fallback / 成本 / 审计 / 分类统计），场景路由下拉直接读 `ListModels` 结果（既有实现）。
- [x] **核验结论**：P1-4 后端与页面主体为「已交付」，本批只补前端口径缺口与验收测试，不重做实现。

## P2（明确不做/远期，附排除论证）

| 缺口 | 排除或推迟理由 |
|---|---|
| A5 海外社媒（FB/IG/Twitter/Line） | 与「中文私域」核心定位正交；Chatwoot 已覆盖该需求且 MIT 可组合——用户可双跑。若未来出海客户群成型再立项 |
| A11 工单系统 | 我方会话即载体 + follow_task 已覆盖跟进语义；完整工单状态机是客服台产品形态（Chatwoot 领域），做即产品定位漂移 |
| B4 插件市场 | 需中心化分发基础设施与社区规模（我方 star 量级不支持）；P1-2 OpenAPI 导入已覆盖「自定义工具」实质需求 |
| C5 知识库内 QA 对 | 与现有 FAQ 库（faq.go）语义重叠，先做「FAQ↔知识库统一检索入口」小整合即可，不新增 QA 对数据结构 |
| C6 多库权限分层 | 单商户形态下需求弱；TENANT_MODE=on 时属 SaaS 生产约束范畴另行评估 |
| E2/E3 代码执行/循环节点 | SOP 引擎定位是「销售流程预编译」非通用计算平台；代码沙箱引入安全面大（容器逃逸/资源）。维持与 Dify 分层，不追 |

---

## 排期建议（v4：批 1 改验收车道，其余不变）

| 批次 | 内容 | 依据 |
|---|---|---|
| 批 1（改验收车道） | P0-3 → P0-1 → P0-2 的**残点核验 + 补测**（三刀实现已交付，见各条「待验收残点」） | 验收残点以补反向测试/CSV 端点/CtxRelev 评估结论文档为主，不重做实现 |
| 批 1.5（新增小刀） | 父子分段（P1-1 前置小刀）——**已完成 2026-10-24**（见 P1-1 落地记录） | 独立可先落地，1-2 天，收益先于解析器升级 |
| 批 2 | P1-2 + P1-3——**已完成 2026-10-24**（见两条 AC 与执行记录） | 同一接缝（工具注册表+装饰器），合并做省一轮 |
| 批 3 | P1-1——**已完成 2026-10-24**（seam/sidecar/图片分支已在库，本批补表格感知分块＋部署文档） | 最大单项；解析 seam 前置 P0-3 已落地 |
| 批 4 | P1-4——**已完成 2026-10-24**（后端已在库，补按日看板与 CRUD 验收测试） | 独立前端为主 |

**工作量依据**：已交付项不估新工时，只估残点（批 1 合计约 1-2 天：反向测试 3 组 + CSV 端点半天 + 评估结论文档）；父子分段 1-2 天（迁移+回拼+行为闸三件）；P1-2+P1-3 合计 5-8 天（go-openapi 解析+HTTP 转发 2-3 天、注册表+凭据 1-2 天、MCP client 1-2 天、前端两页 1 天）；P1-1 剩余 6-10 天（外挂 sidecar 联调 3-5 天+表格感知分块 2-3 天+部署文档 1 天）；P1-4 3-5 天（后端聚合接口 1 天+前端看板 2-4 天）。

## 观察项（不立项，二次审核收录）

| 观察项 | 来源 | 触发条件再评估 |
|---|---|---|
| 渠道接入模板化（对标 Chatwoot 多态 Channel） | DEEP_DIVE §2.5 | 新增官方 API 渠道第 2 个起 |
| SOP 调度器按租户配额（对标 Dify 租户队列） | DEEP_DIVE §2.8 | TENANT_MODE=on 商用前 |
| 上传文档可选 QA 增强（对标 FastGPT QA 拆分，T4 管道开关扩展） | DEEP_DIVE §2.3 | P0-1 评测读数建立后 |

## 开发就绪度补充（二次审核新增，开工前逐项确认）

1. **TENANT_MODE 两形态**：批 1 验收中核 dialogue_eval_cron 租户门；P1-2/P1-3 的 `custom_tools`/MCP server 配置须带 tenant_id 隔离（SaaS 形态下跨租户可见=越界）。
2. **迁移注册**：父子分段 `parent_chunk_id`、P1-2 `custom_tools` 表均走 `pkg/db/migrate.go` 注册（先例：`dialogue_eval_runs` :104）。
3. **测试落位**：P0-3 反向测试落 `aiagent/knowledge/service`（或 etl）包；P0-2 反向测试落 `service`/`confidence` 包；P1-2 E2E 用现有 decorator 测试形状。
4. **DB 对账手段**：chunk 行对账用 `knowledge_chunks`（或实际 chunk 表名，开工时核）计数；文档状态枚举值开工时从 markFailed 写入路径核出并写回本文件。

## 实施入口

每项开工按规则 3 spec-kit：**批 1 三项走验收车道**（spec 阶段以本文件「待验收残点」为清单，不重做实现——如发现实现与残点描述不符再转 Bug 车道）；**父子分段小刀走新功能车道**出 spec.md；P1-2/P1-3/P1-1/P1-4 走新功能车道。本清单即 backlog 输入，spec 阶段以本文件 AC 为验收基线。

> G 编号消歧：本文件与 DEEP_DIVE 中的 **G3/G4/G9/G12/G13 指 marketing-features 在册缺陷系列**；CHECKLIST 的「清单 G1-G5」是评测行编号，两套同字母不同指，引用时注意。

---

## 架构债务（2026-10-24 登记，来自本清单上一笔 2422fe6c）

**service 层直连 `*gorm.DB` 的存量**：净增来自 `report_service.go` / `reflux/{adopter,clusterer,collector}.go` / `asset_store.go` / `webhook.go` 六个查询面，均无对应 Repository（规划 §4.4 要求走 Repository 注入）。
**触发条件**：下次动这些文件的任意一个（新增查询/改表）时，顺手把该文件的查询面抽成 Repository 并同步下调棘轮；在此之前不做整批搬迁。

**口径提醒**：该门正在被另一泳道整体重写为 AST 版（把「注释也算数」与「事务回调混比」两个失真修掉，拆句柄档/事务回调档）——重写未提交前，`grep` 版在 HEAD 上的 181>166 是**存量红**，与本清单四批无关（本批新增代码对该判据零贡献，已用 `git grep` 逐文件核过）。引用该门读数时必须写明测在哪个 SHA、哪一版实现。

## 执行记录（2026-10-24，批 1 + 批 1.5）

### 交付面

| 项 | 产物 |
|---|---|
| P0-3 反向测试 | `user-server/internal/aiagent/knowledge/service/knowledge_pipeline_g13_test.go`（伪 PDF→markFailed+零 chunk；纯文本退化锁） |
| P0-1 覆盖对账闸 | `user-server/internal/aiagent/eval/dialogue_gold_coverage_test.go`（61 条/20 场景/否决链五类各 ≥2/ID 唯一/退化相 ≥2） |
| P0-1 CSV 评测集 | `eval/dialogue_csv.go`＋`model/dialogue_eval_set.go`＋`service/dialogue_eval_set.go`＋`controller/dialogue_eval.go`（4 端点）＋`service.route` 注册＋`migrate.go` 登记；测试 `eval/dialogue_csv_test.go`、`service/dialogue_eval_set_test.go` |
| P0-2 双源传导反向测试 | `user-server/internal/service/sales_engine_transfer_policy_sync_test.go`（DB policy 0.95/0.55 双向 + 默认口径锁定） |
| 批 1.5 父子分段 | `model/knowledge_chunk_parent.go`、`repository/knowledge_chunk_parent.go`、`service/knowledge_service_parents.go`、`rag/retrieval/hybrid_searcher_parent.go`；模型列 `knowledge_chunks.parent_chunk_id`；测试 `knowledge_service_parents_test.go`（3 格）、`hybrid_searcher_parent_test.go`（5 格） |
| 状态回写 | `docs/marketing-features/README.md` G4/G12/G13 改「已修/验收」并引用本文件 |
| P1-2 重启恢复 | `tooluse/custom_tool_boot.go`＋`app/tool_provider_wiring.go` 装配；测试 `tooluse/custom_tool_boot_test.go`（2 格） |
| P1-2 治理链 E2E | `tooluse/openapi_tools_decorator_test.go`（审计+限流+上游往返） |
| P1-3 协议握手 | `mcp/client.go` `EnsureInitialized` + `Mcp-Session-Id` 透传 |
| P1-3 配置面 | `model/mcp_server.go`＋`service/mcp_client_service.go`＋`controller/mcp_client.go`＋5 路由＋`mcp/config.go`（启动发现）＋测试 `mcp/e2e_test.go`（2 格）、`service/mcp_client_service_test.go`（3 格） |
| P1-2/P1-3 前端 | `views/aiAgent/CustomIntegration.vue`＋`api/customIntegration.js`＋路由＋`model/menu.go` 菜单节点＋9 语言词条 |
| P1-1 表格感知分块 | `etl/table_chunking.go`＋`ProcessingConfig.TableAware`（默认由 sidecar 配置推导）；测试 `etl/table_chunking_test.go`（6 格：单元格完整/超长拆表重复表头/关闭逐字节一致/小表豁免/ sidecar 端到端与零外网极性对/图片描述入库） |
| P1-1 部署面 | `docs/DEPLOYMENT_GUIDE.md`「外挂解析与图片知识库」（契约/compose 片段/隐私口径）＋`.env-example` 三个键 |
| P1-4 按日看板 | `api/llmRouting.js` `getDailyUsage`＋`views/llmRouting/List.vue` 成本页签「按日用量趋势」（维度单选 day/day_scenario/day_provider） |
| P1-4 验收测试 | `service/llm_provider_crud_test.go`（CRUD 全链 + 加密落库 + 清空标记） |

### 变异验证（每刀真跑、还原后 md5 比对）

| 刀 | 目标用例 | 结果 |
|---|---|---|
| `if false && !etl.IsPlainTextExt(...)`（G13 分支恒假） | `TestProcessDocumentAsync_FakePDFMarksFailedNoChunks` | 红 ✔ |
| 删 `objection_timing` 全类样本（61→58 仍 ≥50） | `TestDialogueGold_SceneCoverageReconciliation` | 红 ✔ |
| 门槛退回独立常量 `0.70` | `TestTransferGate_FollowsDBPolicy_Objection` | 红 ✔ |
| 摘 `fused = h.attachParentContext(...)` | `TestSearchIndex_StitchesParentContext` | 红 ✔ |
| 摘子块 `parent_chunk_id` 回填 | `TestProcessDocumentAsync_BuildsParentWindows` | 红 ✔ |
| 启动期不读 `custom_tools` 表 | `TestLoadCustomToolProviders_RestoresFromTable` | 红 ✔ |
| MCP 客户端跳过 initialize | `TestE2E_DiscoverRegisterExecuteAndIsolation` | 红 ✔ |
| 服务层手写租户 Where（多余过滤） | — | 变异存活 ⇒ 已删除该手工代码，改由 tenancyidx callback 承担并把用例改成守"新表被隔离机制覆盖" |
| 表格块当正文处理 | `TestTableAware_KeepsEveryCellIntact` / `…LongTableSplits…` | 红 ✔ |
| 拆表后不重复表头 | `TestTableAware_LongTableSplitsByRowWithHeaderRepeated` | 红 ✔ |
| 忽略 `TableAware` 开关 | `TestTableAware_OffMatchesLegacyByteForByte` | 红 ✔ |
| 关闭 provider Key 加密 | `TestLLMProviderCRUD_EncryptedAtRestAndPersisted` | 红 ✔ |

### 门禁状态

- `go build ./...`、`go vet`（改动包）、`gofmt`（改动文件）：绿。
- `./internal/aiagent/...` 全量：全绿（含 knowledge service 102s / retrieval 50s / eval 8s）。
- `make audit`：**整链 rc=0**（2026-10-24 批 2 收口时复跑，19 条判据全绿）。首跑（批 1 期间）红在 `check-gorm-only.sh → internal/repository/payment.go:227`——当时属他泳道未提交改动（`git show HEAD:` 版本无该 `Exec`），本批未代改；该泳道收口后转绿。
- `./internal/service/` 全量：两朵**在册红、均非本批**：
  1. `TestArchServiceNoNewGormDB`：在 **HEAD（641b76f6）本身**就 181 处/111 文件 > 基线 166/106（在 HEAD 的干净 worktree 里实测同数），本批新增文件对该判据零贡献（`dialogue_eval_set.go`/`sales_engine_transfer_policy_sync_test.go` 均无 `*gorm.DB`）。
  2. 默认 600s 超时（`TestM02_WhatsAppMultiMessageBatchFeedsEveryMessageToAI` 卡住）——与本仓「service 包 450–880s 随负载摆动、默认 600s 是抽奖式假红」的在册口径一致；复跑结果（`-timeout 40m`，793s 跑完）：**唯一红就是这条在册的 arch 棘轮**（0 次超时 panic），本批新增/改动用例全绿。
- `go test ./internal/router/`：1 红 `TestAnonymousParamRoutesDeclareTenantMechanism`（`GET /api/template-market/detail/:id` 匿名参数路由未登记租户机制），摘掉本批新增路由后**同样复现**，归他泳道在册项。
