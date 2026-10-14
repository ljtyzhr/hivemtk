package app

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	dbpkg "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// TestMessageHubUpdateExtra_Integration 验证 UpdateExtra 能正确写入并读出 hub 行的 Extra 字段。
//
// 背景：企微发送侧需要把平台 msgid 写入 hub 行 Extra 供撤回时读取。
// 本测试用真实 DB 验证写入→读出的完整链路。
func TestMessageHubUpdateExtra_Integration(t *testing.T) {
	db := testutil.NewTestDB(t, &model.MessageHub{})
	ctx := context.Background()

	// MessageHubService 内部的 repo 用全局 GetDB()，必须注入本用例的库。
	dbpkg.SetTestDB(db)
	t.Cleanup(func() { dbpkg.SetTestDB(nil) })

	// 创建一条 hub 行（模拟企微发送后的状态）
	hub := &model.MessageHub{
		Platform:       "wecom",
		AccountID:      "1",
		MsgID:          "wecom-out-test-uuid-001",
		Direction:      "outbound",
		MsgType:        "text",
		SenderID:       "1",
		ReceiverID:     "external-user-001",
		Content:        "你好，这是一条测试消息",
		ConversationID: "wecom-1-external-user-001",
		SentAt:         time.Now(),
	}
	if err := db.Create(hub).Error; err != nil {
		t.Fatalf("创建 hub 行失败: %v", err)
	}

	svc := service.NewMessageHubService()

	t.Run("写入 Extra 后能正确读出", func(t *testing.T) {
		extra := map[string]any{"msgid": "wecom-msg-real-12345"}
		if err := svc.UpdateExtra(ctx, hub.ID, extra); err != nil {
			t.Fatalf("UpdateExtra 失败: %v", err)
		}

		// 从 DB 重新读取验证
		var got model.MessageHub
		if err := db.First(&got, hub.ID).Error; err != nil {
			t.Fatalf("读取 hub 行失败: %v", err)
		}

		if got.Extra == nil {
			t.Fatal("Extra 应为非 nil")
		}
		msgid, ok := got.Extra["msgid"].(string)
		if !ok {
			t.Fatalf("Extra[\"msgid\"] 应为 string, got %T", got.Extra["msgid"])
		}
		if msgid != "wecom-msg-real-12345" {
			t.Errorf("Extra[\"msgid\"] = %q, want %q", msgid, "wecom-msg-real-12345")
		}
	})

	t.Run("全量覆盖 Extra", func(t *testing.T) {
		// 第一次写入
		if err := svc.UpdateExtra(ctx, hub.ID, map[string]any{"msgid": "first"}); err != nil {
			t.Fatalf("第一次 UpdateExtra 失败: %v", err)
		}
		// 第二次写入不同 key，应全量覆盖
		if err := svc.UpdateExtra(ctx, hub.ID, map[string]any{"other": "value"}); err != nil {
			t.Fatalf("第二次 UpdateExtra 失败: %v", err)
		}

		var got model.MessageHub
		if err := db.First(&got, hub.ID).Error; err != nil {
			t.Fatalf("读取 hub 行失败: %v", err)
		}

		if _, exists := got.Extra["msgid"]; exists {
			t.Error("Extra[\"msgid\"] 应被全量覆盖删除")
		}
		if got.Extra["other"] != "value" {
			t.Errorf("Extra[\"other\"] = %v, want %q", got.Extra["other"], "value")
		}
	})

	t.Run("id=0 时不执行任何操作", func(t *testing.T) {
		// 不应 panic 或报错
		if err := svc.UpdateExtra(ctx, 0, map[string]any{"msgid": "x"}); err != nil {
			t.Errorf("UpdateExtra(id=0) err = %v, want nil", err)
		}
	})
}
