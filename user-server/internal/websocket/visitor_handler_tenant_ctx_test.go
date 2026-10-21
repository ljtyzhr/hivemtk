package websocket

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/tenancyidx"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupVisitorTenantDB 起一个带租户隔离回调的测试库，登记 session_messages 等租户表。
func setupVisitorTenantDB(t *testing.T) *gorm.DB {
	t.Helper()
	database := testutil.NewTestDB(t, &model.SessionMessage{})
	tenancyidx.RegisterTenantCallbacks(database)
	t.Cleanup(func() {
		tenant.SetMode(tenant.ModeOff)
		tenancyidx.ResetRegistryForTest()
	})
	return database
}

// TestVisitorOnConnect_OfflineMessages_CarriesTenantContext 钉住「访客 WS 拉离线
// 消息的查询必须带租户上下文」。
//
// 生产事故（2026-10-21）：onConnect 里两处 h.db.Table("session_messages") 没有
// WithContext(ctx) —— 租户绑定虽然算出来了（ctx 上有 tenantID），但没传进
// GORM，tenancyidx 的隔离闸按「ctx 未绑定」fail-closed 拒绝查询，于是：
//   - 访客重连后离线消息恒空（AI 回复在断线期间收不到）
//   - delivered_at 永远打不上（离线消息反复重投）
// 线上日志原文：「[tenant] 表 session_messages 需要租户上下文但 ctx 未绑定（query）」。
func TestVisitorOnConnect_OfflineMessages_CarriesTenantContext(t *testing.T) {
	database := setupVisitorTenantDB(t)
	tenant.SetMode(tenant.ModeOn)

	const tid uint = 88
	ctx := tenant.WithTenantID(context.Background(), tid)

	// 租户 tid 名下留一条未投递的 AI 消息
	msg := &model.SessionMessage{
		TenantID:  tid,
		SessionID: "sess_tenant_ctx_1",
		Content:   "离线期间 AI 的回复",
		SenderType: "ai",
		SenderName: "AI 助手",
	}
	if err := database.WithContext(ctx).Create(msg).Error; err != nil {
		t.Fatalf("创建离线消息失败: %v", err)
	}

	h := NewVisitorWSHandler(database)
	client := &Client{sessionID: msg.SessionID, visitorID: "v1", channelID: "ch1"}

	// onConnect 内部对失败只记日志、不返回 error —— 用「delivered_at 是否被写上」
	// 作为判据：查询+更新两步都过了，才会写 delivered_at。
	h.onConnect(client, 0, ctx)

	var got model.SessionMessage
	if err := database.WithContext(ctx).
		Where("session_id = ?", msg.SessionID).
		First(&got).Error; err != nil {
		t.Fatalf("回查消息失败: %v", err)
	}
	if got.DeliveredAt == nil {
		t.Fatal("离线消息未被标记为已投递：onConnect 的查询/更新没带上租户上下文，" +
			"被 tenancyidx 隔离闸 fail-closed 拒绝了")
	}
}

// TestVisitorOnConnect_OfflineMessages_WithoutTenantContext_Fails 钉住反向：
// on 模式下不带租户上下文时，隔离闸必须拒绝（证明上面那条不是"闸关了"，
// 而是"闸正常但 ctx 缺失"）。
func TestVisitorOnConnect_OfflineMessages_WithoutTenantContext_Fails(t *testing.T) {
	database := setupVisitorTenantDB(t)
	tenant.SetMode(tenant.ModeOn)

	const tid uint = 89
	ctx := tenant.WithTenantID(context.Background(), tid)
	msg := &model.SessionMessage{
		TenantID:  tid,
		SessionID: "sess_tenant_ctx_2",
		Content:   "x",
		SenderType: "ai",
	}
	if err := database.WithContext(ctx).Create(msg).Error; err != nil {
		t.Fatalf("创建离线消息失败: %v", err)
	}

	// 不带租户 ctx 直查：必须被拒
	if err := database.Table("session_messages").
		Where("session_id = ?", msg.SessionID).
		Scan(&[]map[string]any{}).Error; err == nil {
		t.Fatal("on 模式下无租户上下文的查询应被隔离闸拒绝（否则说明闸失效）")
	}

	// 给点时间让异步清理不干扰
	time.Sleep(10 * time.Millisecond)
}
