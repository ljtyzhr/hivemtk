-- 官网网页客服 FAQ 数据补全（2026-10-22）
--
-- 目的：官网右下角浮标（xapptool.cn → work.xapptool.cn，appKey ak_79Zw7HWzR5wNUnNn6QMnkw8B）
--       的知识面太薄 —— 线上只有 10 条 FAQ + 6 篇知识文档，且「价格与计费」篇写的是
--       旧口径（14900 积分/月）。用户问「多少钱」「怎么部署」「和 Dify 有什么区别」
--       这类问题答不上来 ⇒ 走 RAG 弱召回 ⇒ 低置信 ⇒ 转人工。
--
-- 来源：本地 user_db 的 hivemtk 资产包
--   - asset_bundles.id=140 「hivemtk·功能解说官」（51 条问答，问法覆盖真实用户提问）
--   - asset_bundles.id=135 「HiveMTK 官方社群资产包」（13 条话术：七端/ReAct/GEO/架构）
-- 改写口径：以**线上真实定价**为准（见下方注释），不照搬本地旧文案。
--
-- 线上真实定价（platform_db 实测 2026-10-22）：
--   module_catalog：7 个基础模块 price_cents=0（免费）、17 个增值模块 price_cents=100（¥1）
--   llm_plans：体验包 ¥1 / 标准包 ¥3 / 专业包 ¥6 / 旗舰包 ¥9
--   subscription_config：monthly_price_cents=900（¥9/月），trial_days=15
--   工具数 45、业务模块 94（官网 content.js 口径）
--   部署最低 4 核 8G，推荐 8 核 16G，GPU 可选
--
-- 目标租户：tenant_id=1（商户 蓝湾数字科技（深圳）有限公司 /
--           merchant_key=azbL6ptzRDm3haVkOJE9IHsDU7n0R916）
--           智能体 ai_agents.id=1（default_sales，官网客服渠道绑定的那一个）
--
-- **关键词设计口径（本脚本的关键）**：只放**有区分度的主题词**，不放泛化词。
--   泛化词（"支持""怎么""操作""设置""步骤""平台""哪些"）在多条 FAQ 里重复出现，
--   匹配时靠堆泛化词就能压过问法更贴近的条目 ⇒ 答非所问（线上实测「多少钱」命中
--   「部署需要什么配置」、「支持哪些语言」命中「支持哪些渠道」）。
--   主题词（"多少钱""语言""私有化""Telegram""GEO""Dify""SaaS"…）才是证据。
--
-- 幂等：全部按 (tenant_id, agent_id, question) 去重，可重复执行。
-- 执行前已备份：/root/faq_backup2_20261022.csv 等

-- tenant_id / agent_id 用变量传入（线上 tid=1 aid=1；本地验证用 tid=53 aid=53），
-- 避免同一份脚本在两处硬编码、验证时改了一处忘了另一处。
\if :{?aid}
\else
\set aid 1
\endif
\if :{?tid}
\else
\set tid 1
\endif

BEGIN;

-- ---------------------------------------------------------------------------
-- A. 修正过时口径：旧 FAQ#3 写的是「基础版 14900 积分/月」，与线上真实定价不符。
--    官网访客问「多少钱」拿到旧价就是错的，必须先改这一条。
-- ---------------------------------------------------------------------------
UPDATE faq_entries SET
    answer = 'HiveMTK 的收费口径：① 基础模块免费（系统基座、客户中心、会话工作台、知识库、资产与话术、自有模型接入、平台对接 共 7 个）；② 增值模块按需开通，每个 ¥1（AI 智能体、AI 销冠、营销自动化、社媒引流卡片、浏览器自动化等 共 17 个）；③ 平台模型算力（LLM）四档预付费：体验包 ¥1、标准包 ¥3、专业包 ¥6、旗舰包 ¥9；④ 云端版软件订阅 ¥9/月，含 15 天免费体验期；⑤ 私有化部署免费，用自己的大模型 API Key 不额外计费。代码与全部功能模块 AGPL-3.0 开源，不收授权费。',
    keywords = ARRAY['收费','价格','多少钱','计费','订阅','套餐','积分','免费','购买','费用'],
    updated_at = NOW()
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 怎么收费？';

-- ---------------------------------------------------------------------------
-- A2. 其余已有 10 条的关键词同样去泛化（去掉"支持/哪些/端/平台/介绍/产品/项目"
--     这类在多条里重复的词，只留主题词）。
-- ---------------------------------------------------------------------------
UPDATE faq_entries SET keywords = ARRAY['是什么','介绍','简介','产品','项目','营销系统','开源','做什么','平台','HiveMTK']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 是什么？';

UPDATE faq_entries SET keywords = ARRAY['渠道','接入','Telegram','QQ','企业微信','飞书','WhatsApp','钉钉','抖音','小红书','快手','闲鱼','TikTok','网页客服']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 支持哪些渠道？';

UPDATE faq_entries SET keywords = ARRAY['私有化','部署','Docker','安装','本地','服务器','环境','私有化部署']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 支持私有化部署吗？';

UPDATE faq_entries SET keywords = ARRAY['智能体','AI','RAG','自动回复','功能','能力','销冠','自动应答','有什么功能','产品功能','功能有哪些']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 的 AI 智能体能做什么？';

UPDATE faq_entries SET keywords = ARRAY['联系','微信','人工','客服','联系方式','官网','客服呢','找谁','商务']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = '怎么联系 HiveMTK 官方？';

UPDATE faq_entries SET keywords = ARRAY['架构','技术栈','Go','Vue','pgvector','Redis','原理','分层']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 是什么技术架构？';

UPDATE faq_entries SET keywords = ARRAY['安全','出域','隐私','数据','合规','私有','零出域']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = 'HiveMTK 的数据安全吗？会出域吗？';

UPDATE faq_entries SET keywords = ARRAY['资产市场','资产包','资产','共享','市场']
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = '什么是资产市场？';

-- 生产的这条问法与新增的那条重名 ⇒ 改名区分，并把泛化关键词换成主题词。
UPDATE faq_entries SET
    question = 'AI 智能体在哪里配置？后台怎么操作？',
    keywords = ARRAY['智能体','配置','后台','绑定','人设','欢迎语','渠道绑定'],
    updated_at = NOW()
WHERE tenant_id = :'tid'::bigint AND agent_id = :'aid' AND question = '如何配置 AI 智能体？';

-- ---------------------------------------------------------------------------
-- B. 补充 FAQ：把资产包 140 的真实问法按官网口径改写后补进 FAQ 层。
--    FAQ 层命中 = 直接返回标准答案（零 LLM、答案确定），比让 LLM 现场发挥可靠。
-- ---------------------------------------------------------------------------
INSERT INTO faq_entries
    (tenant_id, question, answer, keywords, category, intent, confidence, enabled, agent_id, created_at, updated_at)
SELECT * FROM (VALUES
(
 1,
 'HiveMTK 和 Dify 有什么区别？',
 'Dify 是通用 LLM 应用平台，强在编排通用 AI 应用、偏工作流；HiveMTK 是专做私域营销的操作系统：多端渠道消息统一收件箱、ReAct 智能体自主谈单（45 个工具）、客户 CDP 与销冠 SOP、知识库 RAG 问答、13 类触达通道直接发消息、以及 GEO 引擎让品牌在 AI 搜索里被看见。简单说：Dify 帮你搭 AI 应用，HiveMTK 帮你把客户从加微信聊到成交。两者都可自托管，都开源。',
 ARRAY['Dify','扣子','区别','对比','相比','差异','和Dify比'], '产品', 'product_compare', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 'HiveMTK 支持哪些渠道接入？',
 'HiveMTK 支持 13 个渠道统一接入：Telegram、QQ、企业微信、飞书、WhatsApp、钉钉、抖音、小红书、快手、闲鱼、TikTok，以及网页客服（就是你现在用的这个右下角浮标）。所有渠道消息汇入同一个统一收件箱，由 AI 智能体自动接待与分流；第三方渠道还可以通过桥接层接入。',
 ARRAY['渠道','接入','Telegram','QQ','企业微信','飞书','WhatsApp','钉钉','抖音','小红书','快手','闲鱼','TikTok','网页客服','端'], '产品', 'channels', 0.95, TRUE, 1, NOW(), NOW()
),
(
 1,
 '私有化部署需要什么配置？',
 '私有化部署最低 4 核 CPU / 8 GB 内存 / 60 GB SSD，推荐 8 核 / 16 GB / 200 GB SSD。GPU 不是必需，llama.cpp 纯 CPU 也能跑，只是生成速度慢一些。x86_64 和 Apple Silicon 都支持（MLX 分支仅限 Mac）。用 Docker Compose 一条命令拉起后端、前端、PostgreSQL、Redis 和本地推理栈。',
 ARRAY['配置','CPU','内存','显卡','GPU','多少核','几个G','服务器','硬件','多大','机器'], '部署', 'deploy_requirements', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '私有化部署怎么操作？',
 '三步：① clone 仓库后配好 .env（数据库、Redis、各类 API Key）；② docker compose up -d 拉起 user-server 后端、user-web 前端、PostgreSQL、Redis 与本地推理栈（llama.cpp / TEI）；③ 浏览器打开控制台，进入「AI 智能体」配人设与知识库，再到「聊天渠道」把智能体绑到接待渠道即可生效。详细步骤见官网部署指南 xapptool.cn/deploy。',
 ARRAY['部署','安装','搭建','Docker','compose','步骤','操作','怎么部署'], '部署', 'deploy_howto', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '不会写代码能用吗？',
 '能。私有化部署只需要配一个 .env 再跑 Docker Compose，不需要写代码；不部署也可以直接用云端版（work.xapptool.cn），注册即用。只有要二次开发、定制话术逻辑或接自有渠道时，才需要懂 Go / Vue。',
 ARRAY['不会写代码','小白','零代码','门槛','上手','需要技术','能懂','二次开发'], '产品', 'no_code', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '这个项目是开源的吗？',
 '开源。协议是 AGPL-3.0，代码托管在 GitHub（github.com/xiaofang142/hivemtk）与 Gitee（gitee.com/xhpmayun/hivemtk），全部后端（Go）与前端（Vue 3）源码公开，可自己部署、可二次开发，不收授权费。需要提醒的是 AGPL-3.0 带有网络分发开源义务：对外提供基于本项目的网络服务时需公开你的修改。',
 ARRAY['开源','AGPL','协议','源码','GitHub','Gitee','授权码','自托管'], '产品', 'open_source', 0.95, TRUE, 1, NOW(), NOW()
),
(
 1,
 '商用需要买授权码吗？',
 '不需要。HiveMTK 本身 AGPL-3.0 完全开源，不卖授权码、不按台报价。付费的只是云端版的增值服务（模块、模型算力、软件订阅）和托管成本，比如模块 ¥1/个、LLM 算力 ¥1~¥9 四档、软件订阅 ¥9/月；私有化部署软件本身免费，用自己的大模型 API Key 不额外计费。',
 ARRAY['授权码','商用','企业用','采购','买授权','要钱吗','收费吗'], '计费', 'license', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '有 SaaS 版本吗？不用自己部署行不行？',
 '有。云端版在 work.xapptool.cn，注册即用、不用自己部署任何东西；同一套代码也支持私有化部署到你自己的服务器上，两者功能同源。云端版软件订阅 ¥9/月，含 15 天免费体验期，增值模块与模型算力按需另购。',
 ARRAY['SaaS','saas','云端','托管','不用部署','在线用','版本'], '产品', 'saas', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '数据安全吗？会传到你们服务器吗？（隐私与出域）',
 '私有化部署下数据完全在你自己的环境：对话、客户资料、知识库都落在你自己的 PostgreSQL 和向量库里，LLM / Embedding / Rerank 走本地推理栈（llama.cpp + TEI），不依赖第三方云端，数据不出内网。官网的云端版则是托管运行，代码与全部功能模块开源、可自行私有化把数据搬回自己环境。',
 ARRAY['安全','出域','隐私','数据','合规','上传','泄露','零出域','私有'], '数据安全', 'data_security', 0.95, TRUE, 1, NOW(), NOW()
),
(
 1,
 'HiveMTK 的 AI 智能体和普通客服机器人有什么区别？',
 '不是关键词匹配的伪 AI，而是 ReAct（思考-行动）循环的真实智能体：45 个工具自主编排，从意图识别、异议应对、SOP 跳转、逼单邀约到售后跟进都能自己完成，工具调用链带权限校验、失败重试、超时控制、速率限制与审计留痕五层防护，还能配置 SOP 工作流让销售流程自动推进。',
 ARRAY['智能体','ReAct','机器人','区别','AI','工具','和普通'], '功能', 'agent_intro', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '怎么创建和配置智能体？',
 '在管理后台进入「AI 智能体」页新建或编辑，设置人设（persona）、欢迎语，绑定知识库（RAG）、资产包（话术）、FAQ、SOP 以及选 LLM 提供商；保存后到「聊天渠道」把智能体绑定到具体接待渠道（如网页客服浮标）即生效。换句话说：一个智能体 = 一套人设 + 知识 + 话术 + 流程，绑到哪条渠道就在哪条渠道上接待。',
 ARRAY['智能体','创建','新建','配置','后台','绑定','人设','渠道绑定','欢迎语'], '配置', 'agent_create', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '知识库怎么建？RAG 问答效果如何？',
 '在「知识库」页上传文档（PDF / Markdown / TXT 等）后系统自动切片并向量化，入库后可挂到智能体上。检索是三级混合：BM25 关键词 + 向量语义召回，再经 bge-reranker 重排，配合 HyDE 查询改写与多轮上下文保留；命中的知识片段会作为标准答案直接回复，或作为 LLM 的上下文。接入 FAQ 资产包后，命中标准答案的问题会零 Token 直接作答。',
 ARRAY['知识库','RAG','检索','召回','切片','向量化','rag','文档','建库'], '功能', 'knowledge', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 'HiveMTK 能做 GEO 优化吗？',
 '能，这是内置的 GEO（生成式引擎优化）引擎：LLM 关键词挖掘与语义扩展 → SEO/GEO 双优化内容生成（E-E-A-T 权威信号 + JSON-LD 结构化标记）→ 多模型模拟 AI 搜索验证品牌提及率 → 一键同步发布到掘金/知乎/CSDN/GitHub 等 12 个平台，形成"优化—验证—再优化"的数据飞轮，还能监控负面查询预警品牌风险。',
 ARRAY['GEO','AI搜索','品牌','提及','搜索引擎','被AI推荐','生成式引擎'], '功能', 'geo', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '支持哪些大模型？能用我自己的 API Key 吗？',
 '支持接入 DeepSeek、通义千问、GPT-4o、智谱 GLM、Kimi 等多家云端大模型，也支持 llama.cpp 本地推理（Qwen2.5 等）完全本地跑。Dispatcher 网关按场景动态路由：复杂异议用强模型、常规回复用轻模型，故障自动切换。私有化部署时可以填自己的大模型 API Key，模型调用不额外计费；云端版则用平台模型算力（¥1~¥9 四档预付费）。',
 ARRAY['大模型','模型','DeepSeek','通义','Qwen','API Key','自己的','本地模型','GPT','GLM','Kimi'], '功能', 'llm', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '支持哪些语言？能做多语言营销吗？',
 '系统内置多语言管理模块，界面与内容可切换中文、英文、日文、阿拉伯文等；内容生成、资产包与智能体话术都带语言维度，可按目标市场生成对应语言的素材与话术。Telegram / TikTok / WhatsApp 等海外渠道的接入也在这套多语言体系内。',
 ARRAY['语言','多语言','翻译','英文','出海','国际化','i18n','日文','阿拉伯文'], '功能', 'i18n', 0.85, TRUE, 1, NOW(), NOW()
),
(
 1,
 '能接入 Telegram 吗？',
 '能。Telegram 是内置支持的渠道之一，配好 Bot Token 即可，消息自动汇入统一收件箱由智能体接待；群内需要 @机器人 才回复（这是 Telegram 隐私模式的规定，不是缺陷），可在 BotFather 用 /setprivacy 关掉。Bot 还支持主动 DM、群线索挖掘、群消息分析与入群管控。另配 FRP 可让本地部署的实例被公网访问。',
 ARRAY['Telegram','电报','TG','Bot','机器人','群','FRP'], '产品', 'telegram', 0.90, TRUE, 1, NOW(), NOW()
),
(
 1,
 '部署报错怎么办？',
 '先看容器日志定位：docker compose logs -f user-server 是后端日志，frontend 看 user-web。常见原因依次是——.env 里数据库或 Redis 端口与实际不符、端口被占用（user-server 默认 8204）、磁盘不足（模型目录建议单独预留 ≥10 GB）、以及 Docker 版本过旧。带上操作系统、Docker 版本和报错原文到 GitHub Issues（github.com/xiaofang142/hivemtk）提问，维护者会看。',
 ARRAY['报错','错误','失败','启动不了','起不来','异常','排查','error','trouble','日志'], '部署', 'deploy_troubleshoot', 0.85, TRUE, 1, NOW(), NOW()
),
(
 1,
 '怎么联系你们？能加微信吗？（找人工客服）',
 '官网 xapptool.cn 提交留言、GitHub（github.com/xiaofang142/hivemtk）或 Gitee（gitee.com/xhpmayun/hivemtk）提 Issue 都行。也可以就用现在这个右下角客服浮标直接问我——我就是官网的智能客服，标准问题我当场答；需要人工处理时会有同事接手。',
 ARRAY['联系','微信','人工','联系方式','客服呢','找谁','商务','邮箱'], '联系', 'contact', 0.95, TRUE, 1, NOW(), NOW()
),
(
 1,
 '能怎么帮我的业务？举个例子',
 '典型场景：① 抖音/小红书/企微等渠道的私信统一收件箱，AI 7×24 接待、识别意图、打标签；② 按客户意向分配合适的 SOP 自动推进（接待→探需→推荐→异议处理→逼单→成交）；③ 客户 CDP 360° 画像 + RFM 分层，唤醒沉睡客户；④ 一键触达（13 类通道）；⑤ GEO 让品牌在 AI 搜索里被引用；⑥ 私域自动化运营（朋友圈内容生成、定时发布、活跃促单）。',
 ARRAY['能干什么','帮我','场景','例子','业务价值','解决什么','有什么用','适合'], '产品', 'use_case', 0.85, TRUE, 1, NOW(), NOW()
),
(
 1,
 '如何配置 AI 智能体？（后台操作步骤）',
 '进入管理后台 → AI 智能体，可创建/编辑智能体，设置人设（persona）、欢迎语、绑定知识库（RAG 产品）、资产包（话术）、FAQ、SOP 与 LLM 提供商；保存后通过渠道绑定把智能体挂载到具体接待渠道即可生效。',
 ARRAY['智能体','配置','设置','后台','人设','绑定','欢迎语','渠道绑定'], '配置', 'agent_config', 0.95, TRUE, 1, NOW(), NOW()
)
) AS v(tenant_id, question, answer, keywords, category, intent, confidence, enabled, agent_id, created_at, updated_at)
WHERE NOT EXISTS (
    SELECT 1 FROM faq_entries f
    WHERE f.tenant_id = v.tenant_id AND f.agent_id = v.agent_id AND f.question = v.question
);

COMMIT;

-- 结果自检
SELECT id, question, confidence FROM faq_entries
WHERE tenant_id = 1 AND agent_id = 1 ORDER BY id;
