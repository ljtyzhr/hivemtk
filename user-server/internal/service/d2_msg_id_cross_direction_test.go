package service_test

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// D-2 用例（规划 §3.2 第 1 步）：证实「同会话内入站与出站出现同文本时，内容哈希相同，
// 唯一索引 (platform,msg_id,conversation_id) 是否真的跨方向冲突」。
//
// ContentHashMsgID(channel, conversationID, content) 不含 direction 与 sender：
// 客户发"好的"入库（inbound）后，AI 回复恰好也是"好的"（outbound）时，
// 两行 (platform, msg_id, conversation_id) 三元组完全一致 ⇒ 第二行撞唯一索引。
// 若本用例证实冲突，方案③（唯一索引加 direction）是最小修复。
func TestMessageHub_CrossDirectionSameContentCollides(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	database := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	ctx := context.Background()
	repo := repository.NewMessageHubRepositoryWithDB(database)

	now := time.Now()
	inbound := &model.MessageHub{
		Platform: "douyin", MsgID: service.ContentHashMsgID("douyin", "conv-d2", "好的"),
		AccountID: "acct-1", Direction: "inbound", Status: "received",
		MsgType: "text", SenderID: "customer-1", SenderName: "客户",
		Content: "好的", ConversationID: "conv-d2", SentAt: now,
	}
	require.NoError(t, repo.Create(ctx, inbound), "入站行应能正常落库")

	outbound := &model.MessageHub{
		Platform: "douyin", MsgID: service.ContentHashMsgID("douyin", "conv-d2", "好的"),
		AccountID: "acct-1", Direction: "outbound", Status: "pending",
		MsgType: "text", SenderID: "acct-1", SenderName: "账号",
		Content: "好的", ConversationID: "conv-d2", SentAt: now.Add(time.Second),
	}
	err := repo.Create(ctx, outbound)

	// 读数：冲突是否真实发生（判据是实测，不是推理）。
	if err != nil {
		t.Logf("【D-2 证实】同会话跨方向同文本确实撞唯一索引：%v", err)
		t.Logf("【D-2 证实】ContentHashMsgID=%s ⇒ 方案③（唯一索引加 direction）的冲突面为真", inbound.MsgID)
	} else {
		t.Logf("【D-2 读数】本库形状下未冲突（唯一索引形状或哈希口径可能已含方向），无需方案③")
	}
	assert.NoError(t, err, "入站+出站同文本应可共存（D-2 验收判据）")
}
