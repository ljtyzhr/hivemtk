# HiveMtk 功能模块对照清单：竞品实现 ↔ 我方实现（2026-10-24）

> **用途**：逐条对照「竞品功能模块及其实现方式」与「我方对应模块及实现方式」，一条一行形成清单。口径：✅有等价实现 ⚠️部分实现/缺口 ❌无对应。竞品证据标注【高=官方】【中=评测】；我方证据为源码坐标。
> 竞品范围：Dify、FastGPT、Chatwoot（同一自部署选型池）。

---

## 清单 A：渠道接入

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| A1 | 企微/公众号接入（FastGPT） | 官方 API，作为**发布渠道**单向输出【高】 | channelgw 企微/公众号 | 官方 API，**会话渠道**：收发一体入统一收件箱 | ✅（语义更重） |
| A2 | 网站挂件（Dify/FastGPT） | 对话窗挂件 / 分享链接 iframe【高】 | embed-sdk 客服 Widget | 原生 JS IIFE/ESM，一行脚本接入，FRP 穿透回内网 | ✅ |
| A3 | WhatsApp（Chatwoot） | WhatsApp Business API【高】 | reach.whatsapp.send | WhatsApp Cloud API + 模板消息 | ✅ |
| A4 | Telegram（Chatwoot） | Bot API【高】 | reach.telegram.send / telegram.dm | Bot 直连，webhook+polling 双模式，群线索挖掘+入群管控 | ✅ |
| A5 | Facebook/Instagram/Twitter/Line（Chatwoot） | 官方 Graph API 各自集成【高】 | — | 无对应渠道模块 | ❌ |
| A6 | 飞书/钉钉（FastGPT） | 发布渠道【高】 | reach.feishu.send / reach.dingtalk.send | 会话渠道（Webhook 回调，PUBLIC_BASE_URL 声明） | ✅ |
| A7 | 抖音/快手/小红书/闲鱼/TikTok 私信 | 三家竞品**均无此模块** | Bridge 浏览器桥接 | Chrome 扩展 CDP 抓取私信页 + 上报，平台专属卡片消息（card.show/reach.card.send） | **我方独有** |
| A8 | 邮件（Chatwoot） | IMAP/SMTP 收发【高】 | batch_send.email + reach.email.send | SMTP 发送 + 会话侧接入 | ✅ |
| A9 | 短信 | 竞品均无 | reach.sms.send | 阿里云/腾讯云/华为云三家 SDK | **我方独有** |
| A10 | 帮助中心/客户自助门户（Chatwoot） | 独立门户模块，FAQ 文档站【高】 | 帮助中心 | `controller/help_center.go` + 公开门户页 `views/public/HelpCenter.vue`（本清单 v1 误标 ❌，2026-10-24 复核修正） | ✅（深度待与 Chatwoot 门户功能逐项对齐） |
| A11 | 工单系统（Chatwoot） | 对话转工单+状态流转 | — | 无工单实体（会话即载体） | ❌ |

## 清单 B：Agent 工具调用

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| B1 | Agent 推理模式（Dify） | Function Calling / ReAct / ReWOO 三策略可选【高】 | AgentLoop | ReAct 循环（≤5 轮/≤18 工具/180s/50k token 预算），非 FC 模型自动注入协议提示词 | ✅（无 ReWOO，可忽略） |
| B2 | 内置工具库（Dify 50+） | 通用工具：搜索/DALL·E/Stable Diffusion/Wolfram【高】 | tooluse 45 工具 | **业务原子工具**：reach.* 20 / customer.* 8 / order·售后·物流·跟进 6 / knowledge 4 / pm 3 / card 1 / browser 3 | 语义不同：我做业务工具，它做通用工具 |
| B3 | 自定义工具（Dify） | **OpenAPI(Swagger) schema 导入即成工具**【高】 | MCP Server | JSON-RPC 2.0 零依赖实现，外部工具标准协议接入；无 OpenAPI 声明式导入面 | ⚠️ 缺 OpenAPI 一键导入 |
| B4 | 插件市场（Dify/FastGPT） | 插件市场生态【高】 | — | 无插件分发机制 | ❌ |
| B5 | 工具护栏 | Dify：工具级权限/限额，无逐调用治理链【中】 | 装饰器链 | 链上 9 层（权限→限流→熔断→重试→超时→审计[含成本记账]→反馈收集→审批门→风控分级）+ 独立件 6（结果缓存/参数校验/LoopGuard/双拦截/死信/DNC，`tooluse/decorator*.go`） | **我方独有** |
| B6 | 循环防护 | 竞品无专用判环【低】 | LoopGuard | 同指纹调用 60s 内 3 次判环拒绝（`loop_guard.go`） | **我方独有** |
| B7 | MCP 客户端/服务端节点（Dify/FastGPT） | 双向 MCP【高】 | MCP Server | 仅 Server 侧 | ⚠️ 缺客户端（作为 MCP client 消费外部 server） |
| B8 | 浏览器自动化工具 | 竞品无（n8n 有但非同池） | browser_open_task / task_list / task_status | browser_automation 包 + CDP :8206 | **我方独有** |

## 清单 C：RAG / 知识库

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| C1 | 混合检索 | Dify：向量+关键词混合+Rerank【高】；FastGPT：语义/全文/混合+RRF+Rerank【高】 | rag.search | pgvector topK50 + BM25 topK30 → RRF 融合 → bge-reranker 精排前 20 取 5 | ✅ 同代等价 |
| C2 | 查询改写/HyDE | Dify 部分支持【中】 | rag 管线 | 改写/HyDE/MultiQuery（后两者默认关） | ✅ |
| C3 | 多级检索缓存 | 竞品无显式实现 | 四层短路 | 热缓存 LRU 1024/30min → 温 → 冷 → 关键词兜底(权重0.5)（代价 G9） | **我方独有**（有已知取舍） |
| C4 | 文档解析 | RAGFlow：DeepDoc 版面识别（表格/多栏/扫描件）开源最强【中】；FastGPT：图片知识库（视觉模型生成描述索引）【高】 | 知识库管线 | 常规格式解析 + 结构分块 + URL 抓取(SSRF 前置校验)；无版面识别、无图片知识库 | ⚠️ **明确落后** |
| C5 | QA 问答对（FastGPT） | 文档外手工补充 Q&A 语料、分段参数细控【高】 | FAQ 库 | `faq.go` MatchByKeyword/MatchByAgent/ScoreCandidates——语义相近但非知识库内 QA 对 | ⚠️ 部分等价 |
| C6 | 知识库权限（FastGPT 商业版） | 成员/部门/群组 + 文件级权限【高】 | 单商户单库 | 无多库权限分层 | ⚠️（单商户形态下需求弱） |
| C7 | 第三方知识源同步（FastGPT 商业版） | 飞书/语雀定时同步、web 站点同步【高】 | T4 冷启动爬取管道 | kb_crawler：URL 抓取→QA 自动拆分→配额 4 旋钮（近期刚落地） | ✅（方向一致，刚补齐） |
| C8 | 向量化降级兜底 | 竞品无 | Hash 兜底 | 向量化服务故障时降级 Hash 保证入库不中断 | **我方独有** |

## 清单 D：LLM 调度

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| D1 | 多提供商管理（Dify） | 100+ 提供商+Ollama，密钥/用量管理界面成熟【高】 | LLM 路由配置页 | 可选云端 LLM（LLM_BASE_URL 指 DeepSeek/OpenAI），Embedding/Rerank 强制本地 | ⚠️ 管理面落后 |
| D2 | 场景化模型路由 | 竞品无（按应用配模型） | 7 场景路由 | 意图/SOP 回复/异议/闲聊/长摘要/高质量/低成本各配独立模型 | **我方独有** |
| D3 | 降级链 | Dify 有重试无多级降级【中】 | 四级降级 | 主模型→备用小模型→语义缓存→模板兜底 | **我方独有** |
| D4 | 提供商熔断 | 竞品无 | 熔断器 | 失败≥5 次熔断 60s，延迟>3000ms 判降级，Redis 多实例同步 | **我方独有** |
| D5 | 多数投票 | 竞品无 | MultiModelVote | 采样 N=5 真实多数派（2/3 一致胜出，`dispatcher_dispatch.go:488`） | **我方独有** |
| D6 | 本地推理栈 | 竞品可接 Ollama 但不打包【中】 | 推理栈三件套 | llama.cpp(MLX) 宿主机直跑（`scripts/inference-host/`），compose 只起 PG/Redis 数据层；LLM :8207/Embedding :8208/Rerank :8209 | **我方独有** |

## 清单 E：业务流程编排

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| E1 | 可视化工作流画布 | Dify：~12 类通用节点（LLM/知识检索/HTTP/代码沙箱/分支/迭代/模板/变量聚合…）【高】；FastGPT Flow【高】 | SOP 编辑器 | 20 节点型（19+reach_send）DAG + AB 分流实验 | 语义分层：通用平台 vs 销售应用（见 #2.5 论证） |
| E2 | 代码执行节点（Dify） | Python/JS 沙箱【高】 | — | 无代码节点 | ❌（应用层定位可不补） |
| E3 | 迭代循环节点（Dify） | 数组循环处理【高】 | — | 无对应节点型 | ❌ |
| E4 | 销售/客服语义节点 | 竞品无销售语义节点 | 9 话术节点 | 问候/探需/介绍/异议/促单/邀约/跟进/激活/培育 | **我方独有** |
| E5 | 调度可靠性 | Dify 平台级执行引擎【中】 | SOP 调度器 | 16 worker/队列 1000 背压拒绝/LLM 并发信号量 4/超时 5min/重试 3 次指数退避/幂等键防重发/Saga 补偿（G1 已接线） | ✅ 等价偏上 |
| E6 | 意图识别 | Dify：问题分类器节点（单轮 LLM）【高】 | 级联意图识别 | 13 粗类+27 子类，三级级联：规则先行（微秒级）→Embedding 锚点中间层（pgvector 预计算，失败 fail-open 退两级）→LLM 低置信复判 + 投机识别（异步 buffer=1/60s 收割） | **我方独有且更深** |
| E7 | 转人工/接管 | Chatwoot 规则转接；Dify 需画布手搭 | 置信度转人工 | 五信号加权(意图.30/实体.15/相关.15/RAG.20/熵.20)+Temperature×Platt×Conformal 校准+六道否决链+四档决策+动态阈值[0.40,0.95] | **我方独有**（短板 G4 两套体系割裂在册） |
| E8 | 异议处理 | 竞品均无 | 异议处理器 | 七类识别（价格/需求/信任/时机/竞品/功能/其他）+话术库回流上浮（`objection_handler.go`） | **我方独有** |
| E9 | 紧急/危机直通 | Chatwoot 有对话优先级 | 危机门禁 | 高危 23 词直转人工/中危 14/低危 9 分级+愤怒≥0.7 升级 | ✅ 更深 |

## 清单 F：客户数据（CDP）

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| F1 | 联系人/轻 CRM（Chatwoot） | 联系人资料+自定义属性【高】 | 客户 360 | OneID 跨渠道归一+全渠道时间线+订单/售后/行为事件 | ✅ 超集 |
| F2 | 标签 | Chatwoot 会话/联系人标签 | 标签与分层 | 行为标签+RFM 模型+客群圈选（customer.segment 工具） | ✅ 更深 |
| F3 | 客户记忆 | Dify 对话变量级记忆 | 四层记忆 | 短期/长期/SOP 状态/业务记忆 | **我方独有** |
| F4 | 线索/公海/跟进任务 | 竞品均无 | 线索管理 | 收集/分配/follow_task 工具/公海 | **我方独有** |

## 清单 G：质检评测

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| G1 | 评测集+批量运行（Dify 社区版） | 标准问答对→批量运行→人工/LLM 评分→迭代【高】 | RAG 自动评测 | `rag_eval_auto.go`：文档切片自动生成评测问题→rag_eval_runs/questions→每日 cron（rag_eval_cron，带租户门）——**RAG 侧已有评测集闭环**（v1 误标，2026-10-24 复核修正）；缺口剩**对话级端到端评测**（见 G3） | ⚠️→✅（RAG 侧）/❌（对话侧） |
| G2 | 应用评测（FastGPT 商业版） | 商业版独占功能【高】 | — | 无 | ❌ |
| G3 | 端到端对话级评测 | 竞品亦弱 | — | 无（G12 在册） | 双方都缺，我方更缺 |
| G4 | LLMOps 日志/标注（Dify） | 应用日志+生产数据标注迭代【高】 | TraceLearning | 每小时批审 200 条四维评分（bad 权重×0.85）+FeedbackLearner 实时回流 | ✅ 方向一致 |
| G5 | 自学习闭环 | 竞品无 | 六件套 | 销冠聚类入库/PromptIterator 版本递增可回滚/SOPAutoOptimizer 5 类结构动作/BanditAllocator Thompson 采样 | **我方独有** |

## 清单 H：安全与部署

| # | 竞品功能模块 | 竞品实现方式 | 我方对应模块 | 我方实现方式 | 状态 |
|---|---|---|---|---|---|
| H1 | 一键部署 | 三家均 docker-compose【高】 | make install | .env 模板+前后端构建+模型下载+数据层+推理栈，5 分钟 | ✅ |
| H2 | 数据不出域 | Chatwoot 自托管即满足；Dify/FastGPT 模型可本地 | 零出域设计 | 对话/向量/检索全程内网；FRP 穿透云端不落对话；PII 本地脱敏后才出 | ✅ 更强（强制级） |
| H3 | 多租户（FastGPT 商业版/Dify 企业版） | 商业版能力【高】 | TENANT_MODE | off 单机/on SaaS 双形态 | ✅ |
| H4 | 审计日志 | Chatwoot 基础操作日志 | 审计三件套 | 操作审计+敏感字段加密+安全审计报表 | ✅ |
| H5 | 合规触达治理 | 竞品无主动触达 | [COMPLIANCE] 不可关日志 + 12 层护栏 + DNC 装饰器（decorator_dnc） | **我方独有** |

---

## 汇总

- **我方独有（竞品全无对应模块）**：A7 中文社媒私信桥接、A9 短信、B5 工具护栏链、B6 LoopGuard、B8 浏览器自动化、C3 四层检索缓存、C8 向量化降级、D2-D6 全部调度韧性、E4 销售语义节点、E6 级联意图、E7 置信度转人工、E8 异议处理、F3 四层记忆、F4 线索公海、G5 自学习、H5 合规治理
- **等价或更深（✅）**：A1-A4/A6/A8、B1、C1/C2/C7、E5/E9、F1/F2、G4、H1-H4
- **明确缺口（⚠️/❌ 待补）**：A5 海外社媒（FB/IG/Twitter/Line）、A11 工单、B3 OpenAPI 工具导入、B4 插件市场、B7 MCP 客户端、C4 文档解析（版面识别/图片知识库）、C5 知识库内 QA 对、C6 多库权限、D1 模型管理面、E2/E3 代码与循环节点、G3 对话级端到端评测（优先级最高）
>
> 2026-10-24 复核修正：A10 帮助中心、G1 RAG 评测集经磁盘核实**已存在**（help_center.go / rag_eval_auto.go），从缺口清单移除；对话级 E2E 评测仍缺（G12 在册）。

> 口径已统一（2026-10-24）：工具数按 `Build*Tools()` 七组实测 = **45**（reach 20/business 6/customer 8/knowledge 4/browser 3/pm 3/card 1）；marketing-features 旧数 42 已修正；B2 行的「46」为首轮误数（含测试字面量），以 45 为准。
>
> **2026-10-24 二次审核同步**：SOP 节点型 20（19+reach_send）、推理栈宿主机直跑（compose 仅数据层）、意图三级级联、装饰器「链上 9 层+独立件 6」四处口径已按源码实测回改本表；批 1 三项（P0-1/P0-2/P0-3）经复核**已在代码交付**，任务状态见 GAP_TASKLIST v4 验收车道。

## 证据来源

- 我方：`user-server/internal/aiagent/agent/tooluse/`（`Build*Tools()` 七组 45 工具）、`internal/service/{intent_recognition,objection_handler,faq}.go`、`docs/marketing-features/README.md`
- [Dify 官方仓库](https://gitee.com/AllenFan/dify)、[Dify 节点类型](https://www.cnblogs.com/znlgis/p/22065484)、[Dify 实战指南](https://blog.csdn.net/QingyunAI/article/details/168429392)
- [FastGPT wiki](https://fastgpt.cn/wiki)、[FastGPT 商业版差异](https://doc.fastgpt.cn/zh-CN/guide/version/commercial)
- [Chatwoot 官网](http://www.chatwoot.com/)、[Chatwoot 功能清单](https://aws.amazon.com/marketplace/pp/prodview-tolblk4kmdqd4)
- [开源 RAG 选型决策树](https://blog.csdn.net/wangzhiguo9261/article/details/166773810)
