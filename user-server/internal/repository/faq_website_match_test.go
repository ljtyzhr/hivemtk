package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
)

// 回归：官网右下角网页客服问「介绍下项目」，AI 走 RAG 弱召回 → 置信度不足 → 转人工。
//
// 这一条锁的是**FAQ 层的可命中性**：FAQ 里明明有「HiveMTK 是什么？」这条标准答案，
// 用户用自己的话问「介绍下项目」却匹配不上（jaccard 字面相似度对自然提问几乎无效，
// 关键词又只认"原样连续子串"），于是问题只能落到 RAG 弱召回上。
//
// 数据是**补全后**的线上 FAQ 快照（faq_entries，tenant_id=1 / agent_id=1）：
// 6 条原有 + 4 条补全（技术架构 / 数据安全 / 资产市场 / 配置智能体），
// 关键词按补全口径写。判据取 FAQ 服务的命中门槛 service.faqHitThresh = 0.6。

func prodFAQSnapshot() []model.FAQEntry {
	enabled := true
	return []model.FAQEntry{
		{ID: 1, Question: "HiveMTK 是什么？", Answer: "HiveMTK 是一款开源的智能营销系统（SaaS 化多租户），提供 AI 智能体客服、多渠道消息接入（企业微信/飞书/钉钉/Telegram/QQ/WhatsApp 等 11+ 渠道）、知识库 RAG、营销自动化、数据分析等能力。",
			Keywords: []string{"HiveMTK", "是什么", "介绍", "产品", "项目", "营销系统", "开源", "简介", "介绍下", "做什么", "干嘛", "平台"},
			Confidence: 0.95, Enabled: &enabled},
		{ID: 2, Question: "HiveMTK 支持哪些渠道？", Answer: "HiveMTK 支持 13 个渠道：Telegram、QQ、企业微信、飞书、WhatsApp、钉钉、抖音、小红书、快手、闲鱼、TikTok，以及网页客服。所有渠道通过统一的 Bridge 桥接层接入。",
			Keywords: []string{"渠道", "支持", "接入", "端", "哪些"}, Confidence: 0.95, Enabled: &enabled},
		{ID: 3, Question: "HiveMTK 怎么收费？", Answer: "HiveMTK 采用 SaaS 订阅制：基础版 14900 积分/月，含 15 天免费体验。核心基础模块免费，AI 智能体/AI 销冠/营销自动化等付费模块可按需开通。",
			Keywords: []string{"收费", "价格", "计费", "订阅", "积分", "免费", "套餐", "多少钱", "费用", "怎么买", "购买"},
			Confidence: 0.90, Enabled: &enabled},
		{ID: 4, Question: "HiveMTK 支持私有化部署吗？", Answer: "支持。HiveMTK 支持私有化部署（Docker Compose），也提供 SaaS 云端版（work.xapptool.cn）。私有化部署时可使用自有模型接入模块。",
			Keywords: []string{"私有化", "部署", "Docker", "本地", "安装", "怎么装", "服务器", "环境"},
			Confidence: 0.90, Enabled: &enabled},
		{ID: 5, Question: "HiveMTK 的 AI 智能体能做什么？", Answer: "AI 智能体可 7×24 小时自动应答客户消息：支持绑定不同大模型、挂载知识库做 RAG 问答、配置 SOP 工作流自动推进销售流程。",
			Keywords: []string{"AI", "智能体", "客服", "自动回复", "RAG", "能力", "功能", "能做什么", "有什么用"},
			Confidence: 0.90, Enabled: &enabled},
		{ID: 6, Question: "怎么联系 HiveMTK 官方？", Answer: "官网：xapptool.cn，在线体验：work.xapptool.cn。您也可以点击右下角客服浮标直接对话。",
			Keywords: []string{"联系", "客服", "官网", "咨询", "联系方式", "找谁", "人工", "微信", "怎么找"},
			Confidence: 0.95, Enabled: &enabled},
		{ID: 7, Question: "HiveMTK 是什么技术架构？", Answer: "HiveMTK 采用分层架构：接入层 → 编排层（ReAct 智能体）→ 能力层（RAG 知识库、内容生成、内置工具、资产市场）→ 数据层（PostgreSQL + pgvector + Redis）。后端 Go + Gin + GORM，前端 Vue 3 + Vite。",
			Keywords: []string{"架构", "技术栈", "技术", "原理", "怎么实现", "Go", "Vue", "设计"},
			Confidence: 0.90, Enabled: &enabled},
		{ID: 8, Question: "HiveMTK 的数据安全吗？会出域吗？", Answer: "100% 私有化部署，数据零出域：所有对话、客户资料、知识库都落在你自己的 PostgreSQL / 向量库里；LLM 推理默认走本地推理栈，不依赖任何第三方云端。",
			Keywords: []string{"安全", "出域", "隐私", "数据", "合规", "私有"},
			Confidence: 0.95, Enabled: &enabled},
		{ID: 9, Question: "什么是资产市场？", Answer: "资产市场（Asset Market）是 HiveMTK 的共享能力中心，可发布与订阅「智能体资产包 / 话术包 / FAQ / SOP / 知识库 / 内容模板」等。",
			Keywords: []string{"资产市场", "资产包", "市场", "资产", "共享"}, Confidence: 0.95, Enabled: &enabled},
		{ID: 10, Question: "如何配置 AI 智能体？", Answer: "进入管理后台 → AI 智能体，可创建/编辑智能体，设置人设、欢迎语、绑定知识库、资产包、FAQ、SOP 与 LLM 提供商；保存后通过渠道绑定把智能体挂载到接待渠道即可生效。",
			Keywords: []string{"智能体", "配置", "设置", "后台", "怎么用", "使用", "怎么配置"},
			Confidence: 0.95, Enabled: &enabled},
	}
}

// TestFAQScoring_WebsiteQuestionsAreMatchable 官网常见问法必须能命中 FAQ。
//
// 判据取 FAQ 服务的命中门槛（service.faqHitThresh = 0.6）的同一条尺度：
// 命中分 ≥ 0.6 才算 Layer1 命中、直接返回标准答案。
func TestFAQScoring_WebsiteQuestionsAreMatchable(t *testing.T) {
	repo := &FAQRepository{}
	entries := prodFAQSnapshot()

	cases := []struct {
		query string
		want  uint // 期望命中的 FAQ id
	}{
		// 用户报的现象原句
		{"介绍下项目", 1},
		// 同义问法
		{"HiveMTK 是什么？", 1},
		{"这个平台是做什么的", 1},
		{"你们产品有什么功能", 5},
		{"支持哪些渠道", 2},
		{"平台能接入哪些渠道", 2},
		{"我想知道价格套餐，怎么订阅？", 3},
		{"怎么收费", 3},
		{"多少钱", 3},
		{"支持私有化部署吗", 4},
		{"怎么安装部署", 4},
		{"部署需要什么环境", 4},
		{"AI 智能体能做什么", 5},
		{"数据安全吗", 8},
		{"什么是资产市场", 9},
		{"怎么配置智能体", 10},
		{"怎么用", 10},
		{"怎么联系你们", 6},
		{"我要找人工客服", 6},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			got, err := repo.scoreAndRank(context.Background(), entries, tc.query, 3)
			if err != nil {
				t.Fatalf("scoreAndRank 失败: %v", err)
			}
			if len(got) == 0 {
				t.Fatalf("「%s」一条 FAQ 都没匹配上（FAQ 层形同虚设）", tc.query)
			}
			top := got[0]
			if top.ID != tc.want {
				t.Errorf("「%s」应命中 FAQ#%d，实际命中 FAQ#%d（%q，分 %.4f）",
					tc.query, tc.want, top.ID, top.Question, top.Confidence)
			}
			if top.Confidence < 0.6 {
				t.Errorf("「%s」命中分 %.4f < 0.6，达不到 Layer1 命中门槛（FAQ 再全也用不上）",
					tc.query, top.Confidence)
			}
		})
	}
}
