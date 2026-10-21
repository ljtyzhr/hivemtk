-- 官网网页客服 FAQ 数据补全（2026-10-22）
--
-- 起因：官网右下角浮标问「介绍下项目」被判「AI 置信度不足 (0.62 < 0.70)」转人工。
-- 排查发现 FAQ 层虽已写入 6 条标准答案，但：
--   1. 关键词太少且只认"原样连续子串"（「产品介绍」匹配不上「介绍下项目」）；
--   2. 缺 4 个高频问题（技术架构 / 数据安全 / 资产市场 / 配置智能体）；
--   3. 问法覆盖不到「多少钱」「怎么用」这类自然口语。
--
-- 本脚本做两件事，均幂等（可重复执行）：
--   A. 给现有 6 条补齐关键词（question/answer 一律保留原样，只动 keywords）；
--   B. 补 4 条缺失的标准问答。
--
-- 配套代码改动（同一批上线）：
--   - repository/faq_entry.go：关键词命中加分 + 词元级包含（语序变了也认）；
--   - service/sales_engine.go：网页客服的非流式路径接上 FAQ 快速通道。
--
-- 执行：
--   docker exec -i mtk-postgres psql -U admin -d user_db -h 127.0.0.1 -p 8202 < fix_website_faq_seed.sql
--
-- 执行前已备份：生产机 /root/faq_backup_20261022.csv

BEGIN;

-- A. 补齐已有 6 条的关键词
UPDATE faq_entries SET keywords = ARRAY['HiveMTK','是什么','介绍','产品','项目','营销系统','开源','简介','介绍下','做什么','干嘛','平台']
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 是什么？';

UPDATE faq_entries SET keywords = ARRAY['渠道','支持','接入','端','哪些']
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 支持哪些渠道？';

UPDATE faq_entries SET keywords = ARRAY['收费','价格','计费','订阅','积分','免费','套餐','多少钱','费用','怎么买','购买']
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 怎么收费？';

UPDATE faq_entries SET keywords = ARRAY['私有化','部署','Docker','本地','安装','怎么装','服务器','环境']
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 支持私有化部署吗？';

UPDATE faq_entries SET keywords = ARRAY['AI','智能体','客服','自动回复','RAG','能力','功能','能做什么','有什么用']
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 的 AI 智能体能做什么？';

UPDATE faq_entries SET keywords = ARRAY['联系','客服','官网','咨询','联系方式','找谁','人工','微信','怎么找']
WHERE tenant_id = 1 AND agent_id = 1 AND question = '怎么联系 HiveMTK 官方？';

-- B. 补 4 条缺失的标准问答
-- faq_entries 没有唯一约束（只有主键与普通索引），所以用 WHERE NOT EXISTS 做幂等，
-- 不能写 ON CONFLICT —— 那样会因缺少唯一约束直接报错。
INSERT INTO faq_entries
    (question, answer, keywords, category, intent, confidence, enabled, agent_id, created_at, updated_at)
SELECT * FROM (VALUES
    (
        'HiveMTK 是什么技术架构？',
        'HiveMTK 采用分层架构：接入层（七端渠道 + 桥接）→ 编排层（ReAct 智能体 + 对话编排）→ 能力层（RAG 知识库、内容生成、内置工具、资产市场）→ 数据层（PostgreSQL + pgvector + Redis）。后端 Go + Gin + GORM，前端 Vue 3 + Vite，支持 x86 / ARM 纯 Docker 部署。',
        ARRAY['架构','技术栈','技术','原理','怎么实现','Go','Vue','设计'],
        '技术架构', 'architecture', 0.90, TRUE, 1, NOW(), NOW()
    ),
    (
        'HiveMTK 的数据安全吗？会出域吗？',
        '100% 私有化部署，数据零出域：所有对话、客户资料、知识库都落在你自己的 PostgreSQL / 向量库里；LLM 推理默认走本地推理栈（llama.cpp + TEI），不依赖任何第三方云端。如需接入云端大模型，可自主配置并承担相应出域范围。',
        ARRAY['安全','出域','隐私','数据','合规','私有'],
        '数据安全', 'data_security', 0.95, TRUE, 1, NOW(), NOW()
    ),
    (
        '什么是资产市场？',
        '资产市场（Asset Market）是 HiveMTK 的共享能力中心，可发布与订阅「智能体资产包 / 话术包 / FAQ / SOP / 知识库 / 内容模板」等，打通不同商户、不同行业之间的营销能力复用，支持私有与公开两种范围。',
        ARRAY['资产市场','资产包','市场','资产','共享'],
        '资产市场', 'asset_market', 0.95, TRUE, 1, NOW(), NOW()
    ),
    (
        '如何配置 AI 智能体？',
        '进入管理后台 → AI 智能体，可创建/编辑智能体，设置人设（persona）、欢迎语、绑定知识库（RAG 产品）、资产包（话术）、FAQ、SOP 与 LLM 提供商；保存后通过渠道绑定把智能体挂载到具体接待渠道即可生效。',
        ARRAY['智能体','配置','设置','后台','怎么用','使用','怎么配置'],
        '配置', 'agent_config', 0.95, TRUE, 1, NOW(), NOW()
    )
) AS v(question, answer, keywords, category, intent, confidence, enabled, agent_id, created_at, updated_at)
WHERE NOT EXISTS (
    SELECT 1 FROM faq_entries f
    WHERE f.tenant_id = 1 AND f.agent_id = 1 AND f.question = v.question
);

-- 已存在同 question 的行则更新其答案与关键词（上面 ON CONFLICT 不命中的情况）
UPDATE faq_entries SET
    answer = 'HiveMTK 采用分层架构：接入层（七端渠道 + 桥接）→ 编排层（ReAct 智能体 + 对话编排）→ 能力层（RAG 知识库、内容生成、内置工具、资产市场）→ 数据层（PostgreSQL + pgvector + Redis）。后端 Go + Gin + GORM，前端 Vue 3 + Vite，支持 x86 / ARM 纯 Docker 部署。',
    keywords = ARRAY['架构','技术栈','技术','原理','怎么实现','Go','Vue','设计'],
    updated_at = NOW()
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 是什么技术架构？';

UPDATE faq_entries SET
    answer = '100% 私有化部署，数据零出域：所有对话、客户资料、知识库都落在你自己的 PostgreSQL / 向量库里；LLM 推理默认走本地推理栈（llama.cpp + TEI），不依赖任何第三方云端。如需接入云端大模型，可自主配置并承担相应出域范围。',
    keywords = ARRAY['安全','出域','隐私','数据','合规','私有'],
    updated_at = NOW()
WHERE tenant_id = 1 AND agent_id = 1 AND question = 'HiveMTK 的数据安全吗？会出域吗？';

UPDATE faq_entries SET
    answer = '资产市场（Asset Market）是 HiveMTK 的共享能力中心，可发布与订阅「智能体资产包 / 话术包 / FAQ / SOP / 知识库 / 内容模板」等，打通不同商户、不同行业之间的营销能力复用，支持私有与公开两种范围。',
    keywords = ARRAY['资产市场','资产包','市场','资产','共享'],
    updated_at = NOW()
WHERE tenant_id = 1 AND agent_id = 1 AND question = '什么是资产市场？';

UPDATE faq_entries SET
    answer = '进入管理后台 → AI 智能体，可创建/编辑智能体，设置人设（persona）、欢迎语、绑定知识库（RAG 产品）、资产包（话术）、FAQ、SOP 与 LLM 提供商；保存后通过渠道绑定把智能体挂载到具体接待渠道即可生效。',
    keywords = ARRAY['智能体','配置','设置','后台','怎么用','使用','怎么配置'],
    updated_at = NOW()
WHERE tenant_id = 1 AND agent_id = 1 AND question = '如何配置 AI 智能体？';

COMMIT;

-- 结果自检
SELECT id, question, confidence, enabled
FROM faq_entries
WHERE tenant_id = 1 AND agent_id = 1
ORDER BY id;
