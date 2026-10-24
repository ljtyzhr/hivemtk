package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// TestObservabilityDashboardRepo_SQLAgainstRealPG 把 007 看板仓储的六条只读 SQL
// 全部打到真实 PostgreSQL 上验证「列名/表名/聚合写法」与既有表结构对得上。
//
// 背景：交付 007 时测试库被并发 lane 占用，六条 SQL 只做过静态核对没有实跑；
// 本用例补上真库落地，任何一条 SQL 的列名漂移（例如 message_hub 的 platform/status、
// bridge_accounts 的 last_sync_at）都会在这里直接红，而不是等看板页 500。
func TestObservabilityDashboardRepo_SQLAgainstRealPG(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.LLMRoutingLog{},
		&model.MessageHub{},
		&model.MessageTrace{},
		&model.BridgeAccount{},
		&model.BridgeSelectorHealth{},
		&model.DomainPool{},
	)
	repo := NewObservabilityDashboardRepository(db)
	if repo == nil {
		t.Fatal("NewObservabilityDashboardRepository 对非 nil db 不应返回 nil")
	}
	ctx := context.Background()
	since := time.Now().Add(-time.Hour)

	// —— 夹具：LLM 成本 ——
	// success=false 那行不能用结构体 Create：GORM 对带 default:true 的 bool 列
	// 会省略零值，false 会被列默认值 true 顶掉；用 map 插入强制写 false。
	llm := []model.LLMRoutingLog{
		{TraceID: "t-llm-1", Scenario: "chat", Provider: "p-a", Model: "m", PromptTokens: 10, CompletionTokens: 5, Cost: 0.5, LatencyMs: 120, Success: true},
		{TraceID: "t-llm-3", Scenario: "embed", Provider: "p-b", Model: "m", PromptTokens: 0, CompletionTokens: 0, Cost: 0.2, LatencyMs: 8, Success: true},
	}
	if err := db.Create(&llm).Error; err != nil {
		t.Fatalf("插 LLM 夹具失败: %v", err)
	}
	if err := db.Model(&model.LLMRoutingLog{}).Create(map[string]interface{}{
		"trace_id": "t-llm-2", "scenario": "chat", "provider": "p-a", "model": "m",
		"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3,
		"cost": 0.1, "latency_ms": 30, "success": false,
		"created_at": time.Now(),
	}).Error; err != nil {
		t.Fatalf("插 LLM 失败行夹具失败: %v", err)
	}
	costRows, err := repo.LLMCost(ctx, since)
	if err != nil {
		t.Fatalf("LLMCost SQL 真库失败: %v", err)
	}
	if len(costRows) != 2 { // (p-a,chat) 与 (p-b,embed) 两个分组
		t.Fatalf("LLMCost 分组数期望 2，实得 %d", len(costRows))
	}
	for _, r := range costRows {
		if r.Provider == "p-a" && r.Scenario == "chat" {
			if r.Calls != 2 || r.SuccessCalls != 1 {
				t.Errorf("p-a/chat 聚合期望 calls=2 success=1，实得 calls=%d success=%d", r.Calls, r.SuccessCalls)
			}
		}
	}

	// —— 夹具：出站送达（delivered / failed / inflight）——
	hub := []model.MessageHub{
		{Platform: "douyin", MsgID: "hub-1", Direction: "outbound", AccountID: "acc", MsgType: "text", ConversationID: "c-1", Status: "delivered"},
		{Platform: "douyin", MsgID: "hub-2", Direction: "outbound", AccountID: "acc", MsgType: "text", ConversationID: "c-2", Status: "failed"},
		{Platform: "douyin", MsgID: "hub-3", Direction: "outbound", AccountID: "acc", MsgType: "text", ConversationID: "c-3", Status: "pending"},
		{Platform: "wechat", MsgID: "hub-4", Direction: "inbound", AccountID: "acc", MsgType: "text", ConversationID: "c-4", Status: "delivered"},
	}
	if err := db.Create(&hub).Error; err != nil {
		t.Fatalf("插 message_hub 夹具失败: %v", err)
	}
	deliveryRows, err := repo.Delivery(ctx, since)
	if err != nil {
		t.Fatalf("Delivery SQL 真库失败: %v", err)
	}
	foundDouyin := false
	for _, r := range deliveryRows {
		if r.Channel == "douyin" {
			foundDouyin = true
			// inbound 行不计入 outbound 口径
			if r.Delivered != 1 || r.Failed != 1 || r.Inflight != 1 {
				t.Errorf("douyin 送达聚合期望 delivered=1 failed=1 inflight=1，实得 %+v", r)
			}
		}
	}
	if !foundDouyin {
		t.Fatalf("Delivery 结果缺 douyin 分组：%+v", deliveryRows)
	}

	// —— 夹具：漏斗（六个生命周期节点，agent_turn 必须被排除）——
	traces := []model.MessageTrace{
		{TraceID: "tr-1", Node: "ingest", Channel: "douyin", AccountID: "acc", ConversationID: "c-1"},
		{TraceID: "tr-1", Node: "ai_dispatch", Channel: "douyin", AccountID: "acc", ConversationID: "c-1"},
		{TraceID: "tr-2", Node: "ingest", Channel: "douyin", AccountID: "acc", ConversationID: "c-2"},
		{TraceID: "tr-3", Node: "agent_turn", Channel: "douyin", AccountID: "acc", ConversationID: "c-3"},
	}
	if err := db.Create(&traces).Error; err != nil {
		t.Fatalf("插 message_trace 夹具失败: %v", err)
	}
	funnelRows, err := repo.Funnel(ctx, since)
	if err != nil {
		t.Fatalf("Funnel SQL 真库失败: %v", err)
	}
	byNode := map[string]int64{}
	for _, r := range funnelRows {
		byNode[r.Node] = r.Cnt
	}
	if byNode["ingest"] != 2 {
		t.Errorf("ingest 去重 trace 期望 2，实得 %d（全表 %+v）", byNode["ingest"], byNode)
	}
	if byNode["ai_dispatch"] != 1 {
		t.Errorf("ai_dispatch 去重 trace 期望 1，实得 %d", byNode["ai_dispatch"])
	}
	if _, ok := byNode["agent_turn"]; ok {
		t.Errorf("agent_turn 不属于六个生命周期节点，不得出现在漏斗聚合里：%+v", byNode)
	}

	// —— 夹具：渠道账号在线数（grace 窗口内 last_sync_at 新鲜 + status != offline）——
	now := time.Now()
	accounts := []model.BridgeAccount{
		{Channel: "douyin", AccountID: "a-1", AccountName: "a1", Status: "online", LastSyncAt: &now},
		{Channel: "douyin", AccountID: "a-2", AccountName: "a2", Status: "offline", LastSyncAt: &now},
	}
	if err := db.Create(&accounts).Error; err != nil {
		t.Fatalf("插 bridge_accounts 夹具失败: %v", err)
	}
	acctRows, err := repo.ChannelAccounts(ctx, 60)
	if err != nil {
		t.Fatalf("ChannelAccounts SQL 真库失败: %v", err)
	}
	if len(acctRows) != 1 || acctRows[0].Channel != "douyin" {
		t.Fatalf("ChannelAccounts 期望只有 douyin 一组，实得 %+v", acctRows)
	}
	if acctRows[0].Accounts != 2 || acctRows[0].Online != 1 {
		t.Errorf("douyin 账号聚合期望 accounts=2 online=1，实得 %+v", acctRows[0])
	}

	// —— 夹具：选择器健康计数（复用 B1-a 表）——
	selectors := []model.BridgeSelectorHealth{
		{Channel: "douyin", Field: "conversationList", Attempts: 10, Hits: 9, FirstSeenAt: now, LastReportAt: now},
		{Channel: "douyin", Field: "messageList", Attempts: 2, Hits: 2, FirstSeenAt: now, LastReportAt: now},
	}
	if err := db.Create(&selectors).Error; err != nil {
		t.Fatalf("插 bridge_selector_health 夹具失败: %v", err)
	}
	selRows, err := repo.SelectorHealthRows(ctx)
	if err != nil {
		t.Fatalf("SelectorHealthRows SQL 真库失败: %v", err)
	}
	if len(selRows) != 2 {
		t.Errorf("SelectorHealthRows 期望 2 行，实得 %d", len(selRows))
	}

	// —— 夹具：封禁/风险域名 ——
	domains := []model.DomainPool{
		{Domain: "ban.example.com", OnBlacklist: true},
		{Domain: "ok.example.com", OnBlacklist: false},
	}
	if err := db.Create(&domains).Error; err != nil {
		t.Fatalf("插 domain_pool 夹具失败: %v", err)
	}
	domRow, err := repo.DomainBanSummary(ctx)
	if err != nil {
		t.Fatalf("DomainBanSummary SQL 真库失败: %v", err)
	}
	if domRow.Blacklisted != 1 || domRow.Total != 2 {
		t.Errorf("domain_pool 聚合期望 blacklisted=1 total=2，实得 %+v", domRow)
	}
}
