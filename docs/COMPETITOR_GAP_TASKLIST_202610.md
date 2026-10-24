# HiveMtk 竞品缺口改进任务清单（2026-10-24，v3 二次论证版）

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

## P0-1 对话级端到端评测：接线与扩集（对应缺口 G3 + 在册 G12）——**2026-10-24 源码复核后任务重定义**

**论证**：竞品参照——Dify 社区版即有「评测集→批量运行→人工/LLM 评分」闭环【高】。我方经复核**已有三层评测资产**（远多于 G12 一句话所写）：
- `rag_eval_auto.go`：RAG 侧评测集闭环（rag_eval_runs/questions，每日 cron 带租户门）✅
- `aiagent/eval/dialogue_gold.go`：**对话级 gold 框架已实现**（T-P8-04/LTC-15）：多回合样本（turn 级 ChrF/关键词覆盖/期望动作一致，composite=0.5/0.3/0.2 加权）+ 退化对照（degraded transcript 反向掉分，AC③ 自证形状）+ `dialogue_eval_runs` 表（`pkg/db/migrate.go:104` 已注册）
- gold 数据：`eval/testdata/dialogue_gold.json` 仅 **8 样本 / 3 场景**（aftersales/handoff/quote）

**真残点（G12 应改写的准确表述）**：
1. **无生产接线**：`RunDialogueGold` 仅有测试调用方，无 cron/HTTP 入口，`dialogue_eval_runs` 表无写入方——评测资产成「孤岛」；
2. **gold 集过小**：8 条，意图类/否决链覆盖远不足；
3. 评分维度不含意图识别命中（现评 chrf/keycov/action 三维）。

**任务范围（v3 修订：接线+扩集，全部走现成形状）**：
1. cron 接线：照抄 `main.go:572` `NewRagEvalCron()` 装配形状（AllowWorker 租户门），跑 `RunDialogueGold` 落 `dialogue_eval_runs`（正常行 + degraded 反向行）；
2. HTTP 面：照抄 `controller/rag_eval.go` 三端点形状（run/list/get + manage 面），**前端看板后置**（curl/接口先行）；
3. 扩 gold 集：8 → ≥50 条——优先走**现成管道** `BadCaseToDialogueSample`（`dialogue_gold.go:179`，上游 `bad_case.go` G-2 闭环 + `controller/bad_case.go` HTTP 入口已存在）：bad-case 导出转 gold，不足部分再手写；13 粗意图类每类 ≥3 + 6 道否决链各 ≥1（否决链用例与 P0-2 共用）；
4. 评分增加 `intent_hit` 维度（回放走 service 层不打 HTTP）。

**AC（v3）**：
- [ ] cron 触发后 `dialogue_eval_runs` 出现当日行，degraded 行 composite 显著低于正常行（沿用现有 AC③ 自证）
- [ ] `BadCaseToDialogueSample` 管道在扩集中真实使用（至少 N 条 gold 来自 bad_case 导出，可追溯 sample.CaseID）
- [ ] gold 集 ≥50 条且意图类覆盖计数逐类达标（对账脚本输出逐类计数）
- [ ] intent_hit 维度入 Details JSON 并有单测
- [ ] `make audit` 全绿；gold runner 用例 `-race` 无竞争
- [ ] marketing-features G12 改写为准确残点并引用本任务

**依赖**：无（全部复用既有形状）。

## P0-2 置信度两套体系打通（在册 G4）——**2026-10-24 源码复核后任务收窄**

**论证**：竞品无此能力所以无「对标」压力，但 G4 是我方**独有能力（E7 置信度转人工）的内部割裂**——领先项质量债优先于追平缺口。

**源码复核（2026-10-24）——割裂程度比文档记载的轻**：
- ✅ 已接线部分：`cmd/api/main.go:525` 启动装配 `InitConfidenceAggregator`；`app/sales_engine_factory.go:77` 注入 engine、`:140` 注入 SmartCSOrchestrator（`smart_cs_orchestrator.go:105`）；`session_assignment.go:254`、`tuning.go:140` 亦消费同一单例。五信号体系**已全局接线**。
- ❌ 真割裂残点①（单信号一票否决）：`internal/service/sales_engine_transfer.go:29` —— `intent.Confidence >= 0.7` 硬编码门槛在进入 `shouldTransferByConfidence`（五信号聚合）**之前**独立判定：意图置信 ≥0.7 且非投诉/流失时直接 `return false`（不转人工），五信号根本不参与。
- ❌ 真割裂残点②（**v3 新发现：双阈值源失同步**）：`confidence/threshold_policy.go:122-136` 已存在 per-intent 阈值表（complaint 0.85 / churn 0.85 / objection 0.75 / ask_product 0.70 / price_inquiry 0.65 / default 0.70），聚合器经 `aggregator.go:136 GetPolicy` 消费它；而 transfer 前置门槛是**通用 0.7**，与 per-intent 表**并存且互不知晓**——运营调 policy 表（如把 objection 调到 0.6）不会影响 transfer 前置判定，反之亦然。G4 的「两套体系」实际是「**三套阈值数字**」（前置 0.7 / per-intent policy 表 / 四档 Band 边界 0.40-0.75）。
- ⚠️ 附带：`confidence_init.go` 的 Embedder 是本地随机投影（1024 维 seed=42），CtxRelev 信号语义质量存疑——打通时应一并评估。

**任务范围（v3 修订）**：
1. 把 `sales_engine_transfer.go:29` 的 0.7 前置门槛**对齐 per-intent policy 源**（读 `GetPolicy` 而非独立常量），或经 tunable 显式声明为独立旋钮——二选一在 spec 阶段定夺，但**禁止继续第三套魔法数**；
2. 评估 CtxRelev Embedder 从随机投影升级为真实 bge-m3（走本地推理栈 :8208）的代价；
3. 更新 `marketing-features` G4 描述（已改写为准确残点，本任务完成后改「已修」）。

**AC（v3）**：
- [ ] 门槛值来源唯一化：前置门槛读 policy/tunable（代码中不再有裸 0.7 常量），并带旋钮门（参照 T4 配额 4 旋钮先例）
- [ ] 行为锁定测试：默认值下现有 transfer 用例全绿（阈值行为不变）
- [ ] **对齐反向测试**：把 objection policy 调低（如 0.5）后，objection 类高意图置信用例走五信号聚合可触发否决（证明 policy 表变化能传导到 transfer 路径——v2 版 AC 缺这条，测不出双源失同步）
- [ ] CtxRelev 升级评估结论写入本文件（做/不做+理由）
- [ ] `make audit` 全绿

## P0-3 解析失败退化为原文入库（v3 新增——二次论证发现的正确性 bug，从 P1-1 拆出升 P0）

**论证**：非竞品缺口，是本轮代码级复核 P1-1 时发现的**正确性缺陷**：`aiagent/knowledge/service/knowledge_service_pipeline.go:45-48` —— `etl.ExtractText` 失败（扫描件 PDF、损坏文档）时代码执行 `extracted = string(bytes)` **把二进制原文当内容入库**，随后分块、向量化全链路吃垃圾，污染向量库并浪费算力。同文件 :53 明明已有「疑似扫描件」markFailed 口径，但 :47 的退化分支先于它吃掉一切提取失败——两条失败路径互斥且选错了分支。`etl/document_text.go:148` PDF 走 ledongthuc/pdf 纯文本提取，扫描件必然空/失败，这是**常见输入**不是边角。

**与竞品清单的关系**：C4 缺口（文档解析落后）的「第一刀」不是外挂 DeepDoc，而是**先不把垃圾入库**——解析质量升级前，错误内容入库会让任何检索评测（rag_eval）读数失真，也直接拖累 P0-1 的评测可信度。

**任务范围**：
1. `knowledge_service_pipeline.go:47` 退化分支收紧：提取失败 → markFailed（复用 :53 口径），仅当提取成功但内容为空才走现有「扫描件/空文件」路径；「退化为原始内容」只允许对纯文本类后缀（.txt/.md/.csv）保留；
2. 补反向测试：构造「提取必失败」的伪 PDF 字节（非 PDF 魔数），断言 markFailed 而非入库。

**AC**：
- [ ] 伪 PDF 字节导入 → 文档状态 failed + 错误信息含提取失败原因（不落向量库，DB 无对应 chunk 行）
- [ ] 纯文本后缀提取失败仍可退化入库（现有行为保留，有测试锁定）
- [ ] `make audit` 全绿

**依赖**：无。**建议与批 1 并做**（改动 ~10 行 + 测试）。

## P1-1 文档解析升级：表格版面 + 图片知识库（对应缺口 C4）——v3 修订：首刀改向

**论证**：竞品参照——RAGFlow DeepDoc（OCR+表格重建+版面分析，复杂版式准确率约 92%，20+ 格式）【中】；FastGPT 图片知识库（视觉模型生成描述索引，商业版）【高】。行业共识「解析是天花板：解析不对，链路全废」【中】。
**v3 修订（源码核）**：我方现状比 v2 描述更弱——解析调用点 `knowledge_service_pipeline.go:45` **直调 `etl.ExtractText`，零抽象零 seam**；PDF 走 `ledongthuc/pdf` 纯文本提取（无 OCR/版面），扫描件直接失败（且失败后退化入库的 bug 见 P0-3）。所以「留 seam 接外挂解析器」是**真新建**而非 v2 暗示的「已有接缝」。
**适配性**：我方是 Go 单体+本地推理栈，自研 DeepDoc 级视觉解析不现实；正确路线是**外挂解析引擎**（RAGFlow/MinerU sidecar，「接口统一、实现可换」【中】）或本地 VL 模型（Qwen2.5-VL 系）做图片→描述索引——后者与「Embedding/Rerank 强制本地」隐私卖点同构。**前提：P0-3 先落地**（否则解析质量提升无评测基线可对）。

**任务范围**：
1. 解析器接口抽象（`knowledge_pipeline` 处留 seam），默认现实现；
2. 可选外挂解析服务（HTTP 调 RAGFlow/MinerU，部署文档给出 sidecar compose 片段，默认不启用）；
3. 图片知识库：VL 模型生成文本描述→入库向量索引（走本地推理栈端口规划，新增 :8210 或复用）；
4. 表格感知分块选项（启用外挂解析时生效）。

**AC**：
- [ ] 带 3 页复杂表格 PDF：外挂解析器入库后表格单元格召回可用（人工抽检 ≥10 格正确）
- [ ] 图片条目入库后可被文本 query 命中
- [ ] 未启用外挂时行为与现状逐字节一致（回归：现有知识库用例全绿）
- [ ] 隐私口径：默认配置零外网请求（外挂需显式 opt-in + 文档声明）

## P1-2 OpenAPI 声明式工具导入（对应缺口 B3）

**论证**：竞品参照——Dify「OpenAPI schema 导入即成工具」是自定义工具接入的最低摩擦形态【高】。我方已有 MCP Server + 46 内置工具，但用户加自有业务工具（查库存 ERP 等）需要写 Go 代码。补此面后「护栏优势（B5）+ 低摩擦接入」合并成完整故事。

**任务范围**：
1. OpenAPI 3.x 解析（go-openapi 库）→ 自动生成 `Tool` 实现（Name/Schema/Handler=HTTP 转发）；
2. 生成的工具自动套 12 层装饰器链（与内置工具同权治理——这即是相对 Dify 的差异点）；
3. 工具注册表持久化（表 `custom_tools`：schema 来源/鉴权凭据 secrets 包接入）；
4. 前端管理页（user-web）：贴 schema → 预览 operations → 勾选导入。

**AC**：
- [ ] 对一个公开 OpenAPI 规范（如 petstore）导入后 Agent 可调用成功（E2E 测试）
- [ ] 导入工具的调用过审计+限流装饰器（用现有 decorator 测试形状）
- [ ] 凭据不明文落库（secrets 包）
- [ ] `make audit` 全绿

## P1-3 MCP 客户端侧（对应缺口 B7）

**论证**：竞品参照——Dify/FastGPT 均双向 MCP【高】。我方只有 Server（暴露自身给外部）。补客户端后，ReAct Agent 的工具池可动态挂外部 MCP server（与 P1-2 组成「声明式接入双通道」）。工程量小（协议已实现一半）。

**任务范围**：JSON-RPC 2.0 client + tools/list 发现 → 动态注册为带装饰器的工具；配置面（user-web）管理 server 列表。

**AC**：
- [ ] 对一个标准 MCP server（如 filesystem 参考实现）tools/list 发现并可在 AgentLoop 中调用
- [ ] Server 不可达时熔断降级不影响其他工具
- [ ] `make audit` 全绿

## P1-4 模型管理界面增强（对应缺口 D1）

**论证**：竞品参照——Dify 100+ 提供商管理+用量看板【高】。我方有 LLM 路由可视化配置页（七场景），缺**提供商/密钥/用量**集中面。注意边界：不追「100+ 提供商清单」（那是平台型产品面），只做「多提供商注册 + 密钥管理 + 按 token 用量统计」三件。

**AC**：
- [ ] 提供商增删改（凭据走 secrets）
- [ ] 用量按日/按场景聚合看板（数据源已有 dispatcher 记录）
- [ ] 场景路由页与提供商页联动

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

## 排期建议（v3，串行，符合 RPM 限制）

| 批次 | 内容 | 依据 |
|---|---|---|
| 批 1 | P0-3（半天内小刀）→ P0-1 → P0-2 | P0-3 是解析垃圾入库 bug，先修它，P0-1 的 rag_eval 读数才不失真；P0-1/P0-2 共用否决链反向用例 |
| 批 2 | P1-2 + P1-3 | 同一接缝（工具注册表+装饰器），合并做省一轮 |
| 批 3 | P1-1 | 最大单项，且**前置依赖 P0-3 已落地**；涉及部署文档与推理栈端口 |
| 批 4 | P1-4 | 独立前端为主 |

**总工作量粗估（v3）**：P0-3 半天；P0-1 约 2-3 天（bad_case 管道+现成形状）；P0-2 约 1-2 天；P1-2+P1-3 合计 5-8 天；P1-1 8-12 天；P1-4 3-5 天。

## 实施入口

每项开工按规则 3 spec-kit：**P0-2、P0-3 走 Bug 车道**（assessment 坐实 file:line——transfer 前置门槛双源失同步 / 解析失败退化入库）；P0-1、P1-* 走新功能车道出 spec.md。本清单即 backlog 输入，spec 阶段以本文件 AC 为验收基线。
