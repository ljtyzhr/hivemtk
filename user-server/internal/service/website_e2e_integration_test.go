package service

import (
	"context"
	"fmt"
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// 端到端：官网网页客服问「介绍下项目」必须答出标准答案，且**不转人工**。
//
// 这条用例走真实 DB（faq_entries / ai_agents / channel_agent_bindings），
// 模拟官网那条链路的真实条件：
//   - web_embed 渠道绑定了某个智能体
//   - 该智能体名下有 FAQ「HiveMTK 是什么？」（含关键词"介绍""项目"）
//   - 访客问「介绍下项目」
//
// 判据：FAQ 命中 ⇒ 回复是标准答案、HandlerType=AI、Transferred=false。
//
// 用的库是**运行库**（DB_HOST/DB_PORT 指向的真实 user_db），不是干净的测试库：
// 官网那条链路的每一步（渠道 → 绑定 → 智能体 → FAQ 候选集 → 匹配）都要在
// 真实表结构与真实数据量下成立，干净的测试库恰恰测不到候选集上限这一层。
func TestWebsiteE2E_ProductQuestionAnsweredByFAQ(t *testing.T) {
	db := connectRuntimeDB(t)
	// 运行库句柄要挂到全局（repository.GetDB 走 dbstate）——
	// 渠道绑定与智能体仓储都是走全局句柄的。
	dbutil.SetTestDB(db)
	ctx := context.Background()

	// 1) 造一个专属智能体 + FAQ + 渠道绑定（用独立 agent_id 避免与存量数据互相干扰）。
	agentCode := "e2e_website_faq_agent"
	var agentID uint
	if err := db.Raw(`DELETE FROM channel_agent_bindings WHERE account_id = 'e2e_faq_website'`).Error; err != nil {
		t.Fatalf("清理绑定失败: %v", err)
	}
	if err := db.Raw(`DELETE FROM faq_entries WHERE agent_id IN (SELECT id FROM ai_agents WHERE agent_code = ?)`, agentCode).Error; err != nil {
		t.Fatalf("清理 FAQ 失败: %v", err)
	}
	if err := db.Raw(`DELETE FROM ai_agents WHERE agent_code = ?`, agentCode).Error; err != nil {
		t.Fatalf("清理智能体失败: %v", err)
	}
	if err := db.Exec(`INSERT INTO ai_agents (agent_code, name, agent_type, agent_mode, persona, status, rag_top_k, confidence_threshold, tenant_id, created_at, updated_at)
		VALUES (?, 'E2E 官网 FAQ 智能体', 'sales', 'passive', '测试人设', 1, 3, 0.7, 1, NOW(), NOW())`, agentCode).Error; err != nil {
		t.Fatalf("插入智能体失败: %v", err)
	}
	if err := db.Raw(`SELECT id FROM ai_agents WHERE agent_code = ?`, agentCode).Scan(&agentID).Error; err != nil {
		t.Fatalf("读取智能体 id 失败: %v", err)
	}

	enabled := true
	faq := &model.FAQEntry{
		TenantID: 1, AgentID: &agentID,
		Question:   "HiveMTK 是什么？",
		Answer:     "HiveMTK 是一款开源的智能营销系统（SaaS 化多租户），提供 AI 智能体客服、多渠道消息接入、知识库 RAG、营销自动化与数据分析。",
		Keywords:   []string{"HiveMTK", "是什么", "介绍", "产品", "项目", "营销系统", "开源"},
		Category:   "产品",
		Intent:     "",
		Confidence: 0.95,
		Enabled:    &enabled,
	}
	if err := db.Create(faq).Error; err != nil {
		t.Fatalf("插入 FAQ 失败: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Exec(`DELETE FROM faq_entries WHERE id = ?`, faq.ID).Error
		_ = db.Exec(`DELETE FROM channel_agent_bindings WHERE account_id = 'e2e_faq_website'`).Error
		_ = db.Exec(`DELETE FROM ai_agents WHERE id = ?`, agentID).Error
	})
	if err := db.Exec(`INSERT INTO channel_agent_bindings (tenant_id, channel_type, account_id, agent_id, is_primary, enabled, priority, created_at, updated_at)
		VALUES (1, 'web_embed', 'e2e_faq_website', ?, TRUE, TRUE, 0, NOW(), NOW())`, agentID).Error; err != nil {
		t.Fatalf("插入渠道绑定失败: %v", err)
	}

	// 2) 按官网那条链路解析智能体：web_embed 渠道 → 智能体上下文。
	bindingSvc := NewChannelAgentBindingService()
	agentCtx, err := bindingSvc.LoadAgentForChannel(ctx, string(model.ChannelTypeWebEmbed), "e2e_faq_website")
	if err != nil {
		t.Fatalf("解析渠道智能体失败: %v", err)
	}
	if agentCtx == nil || agentCtx.AgentID != agentID {
		t.Fatalf("渠道智能体解析错误: %+v", agentCtx)
	}

	// 3) FAQ 服务命中标准答案。
	faqSvc := NewFAQService(db, repository.NewFAQRepository(db))
	matches, err := faqSvc.MatchByAgent(ctx, agentID, "介绍下项目", 3)
	if err != nil {
		t.Fatalf("FAQ 匹配失败: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("「介绍下项目」一条 FAQ 都没匹配上")
	}
	top := matches[0]
	if top.Score < faqHitThresh {
		t.Fatalf("「介绍下项目」命中分 %.4f < 门槛 %v", top.Score, faqHitThresh)
	}
	if top.Entry == nil || top.Entry.Answer != faq.Answer {
		t.Fatalf("命中的是别的答案: %+v", top.Entry)
	}

	// 4) 引擎层：FAQ 快速通道应直接返回标准答案，且标记为确定答案。
	engine := &SalesEngine{faqSvc: faqSvc}
	resp, hit := engine.tryFAQFastPath(ctx, &SalesRequest{
		UserMessage:  "介绍下项目",
		AgentContext: agentCtx,
	})
	if !hit {
		t.Fatal("FAQ 快速通道未命中")
	}
	if resp.Reply != faq.Answer {
		t.Errorf("回复应为 FAQ 标准答案，实际 %q", resp.Reply)
	}
	if !resp.DefiniteAnswer {
		t.Error("FAQ 直答应标记为确定答案（编排器据此跳过置信度判据）")
	}

	// 5) 编排器层：确定答案不因低置信被推翻去转人工。
	//    这里 intent 为空（线上 FAQ 的 intent 字段多为空），
	//    旧判据会把空串当"已知意图" ⇒ 仍被低置信判据转人工。
	if isKnownIntent(resp.Intent) {
		t.Error("空 intent 不应算已知意图")
	}
	if !isDefiniteAnswer(resp) {
		t.Error("确定答案应跳过置信度判据")
	}
}

// connectRuntimeDB 连运行库（DB_HOST/DB_PORT/POSTGRES_PASSWORD）。
//
// 用运行库而不是干净的测试库：官网那条链路的每一步（渠道 → 绑定 → 智能体 →
// FAQ 候选集 → 匹配）都要在真实表结构与真实数据量下成立 —— 候选集上限
// （faqCacheMaxN）这一层恰恰只在有大量存量 FAQ 的库上才测得到。
func connectRuntimeDB(t *testing.T) *gorm.DB {
	t.Helper()
	host := envOr("DB_HOST", "127.0.0.1")
	port := envOr("DB_PORT", "8202")
	user := envOr("POSTGRES_USER", "admin")
	pass := os.Getenv("POSTGRES_PASSWORD")
	if pass == "" {
		t.Skip("运行库不可用（POSTGRES_PASSWORD 未设置）")
	}
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=user_db sslmode=disable", host, port, user, pass)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Skipf("运行库不可用（%s:%s）：%v", host, port, err)
	}
	return db
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
