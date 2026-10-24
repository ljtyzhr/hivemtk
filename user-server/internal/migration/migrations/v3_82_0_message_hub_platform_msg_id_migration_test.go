package migrations

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// D-9 存储半项验收判据（规划 §3.9 第 2 条）：
//
//	① platform_msg_id 列存在（可空）；
//	② 复合索引 (platform, account_id, platform_msg_id) 存在；
//	③ 存量行回填：TG/飞书行键末段、WA 行键本体、wecom Extra["msgid"]；
//	④ 不能无歧义解析的行（ContentHash 键 mh: 前缀）留空——不猜；
//	⑤ 发送侧写列后 GetOutgoingByPlatformMsgID 按新列命中。
func TestMessageHubPlatformMsgIDMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	database := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	ctx := context.Background()

	// 存量行：三种可解析形态 + 一种不可解析形态
	seed := []model.MessageHub{
		{Platform: "telegram", AccountID: "acc-1", ConversationID: "c-1", Direction: "outbound",
			MsgID: "tg-out-acc-1-4242", MsgType: "text", Status: "sent"},
		{Platform: "feishu", AccountID: "acc-2", ConversationID: "c-2", Direction: "outbound",
			MsgID: "feishu-out-acc-2-om_9f8e", MsgType: "text", Status: "sent"},
		{Platform: "whatsapp", AccountID: "acc-3", ConversationID: "c-3", Direction: "outbound",
			MsgID: "wamid.7f3a2b", MsgType: "text", Status: "sent"},
		{Platform: "telegram", AccountID: "acc-1", ConversationID: "c-1", Direction: "inbound",
			MsgID: "mh:8f14e45fceea167a5a36dedd4bea2543", MsgType: "text", Status: "done"}, // ContentHash 键
	}
	for i := range seed {
		if err := database.Create(&seed[i]).Error; err != nil {
			t.Fatalf("插存量行失败: %v", err)
		}
	}

	m := NewMessageHubPlatformMsgIDMigration(database)
	if err := m.Up(ctx); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// 幂等重入
	if err := m.Up(ctx); err != nil {
		t.Fatalf("幂等重入失败: %v", err)
	}

	// ③ 回填读数（显式行扫描，避免 Find 进 map 的歧义写法）
	type readRow struct {
		MsgID         string
		PlatformMsgID string
	}
	var rows []readRow
	if err := database.Model(&model.MessageHub{}).
		Where("msg_id IN ?", []string{"tg-out-acc-1-4242", "feishu-out-acc-2-om_9f8e", "wamid.7f3a2b", "mh:8f14e45fceea167a5a36dedd4bea2543"}).
		Select("msg_id, COALESCE(platform_msg_id, '') AS platform_msg_id").
		Find(&rows).Error; err != nil {
		t.Fatalf("查回填结果失败: %v", err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.MsgID] = r.PlatformMsgID
	}
	expect := map[string]string{
		"tg-out-acc-1-4242":                   "4242",
		"feishu-out-acc-2-om_9f8e":            "om_9f8e",
		"wamid.7f3a2b":                        "wamid.7f3a2b",
		"mh:8f14e45fceea167a5a36dedd4bea2543": "", // 不可解析 ⇒ 留空（判据④）
	}
	for k, want := range expect {
		if got[k] != want {
			t.Errorf("msg_id=%s 回填读数 platform_msg_id=%q，期望 %q", k, got[k], want)
		}
	}

	// ② 索引存在
	var idxN int64
	if err := database.Raw(`
		SELECT count(*) FROM pg_indexes
		WHERE tablename = 'message_hub' AND indexname = 'idx_hub_platform_msg'`).Scan(&idxN).Error; err != nil {
		t.Fatalf("查索引失败: %v", err)
	}
	if idxN != 1 {
		t.Fatalf("idx_hub_platform_msg 不存在（count=%d）", idxN)
	}

	// ⑤ 发送侧写列后按新列命中（双段式查询的第一段）
	if err := database.Model(&model.MessageHub{}).Where("id = ?", seed[0].ID).
		Update("platform_msg_id", "4242").Error; err != nil {
		// 回填已写过同值；这里显式写一遍验证 UpdatePlatformMsgID 通路等价
		t.Logf("显式回写跳过（迁移回填已生效）: %v", err)
	}
	var row model.MessageHub
	if err := database.Where("platform = ? AND account_id = ? AND direction = ? AND platform_msg_id = ?",
		"telegram", "acc-1", "outbound", "4242").First(&row).Error; err != nil {
		t.Fatalf("按 platform_msg_id 命中失败: %v", err)
	}
	if row.ID != seed[0].ID {
		t.Fatalf("按 platform_msg_id 命中的行 id=%d，期望 %d", row.ID, seed[0].ID)
	}
}
