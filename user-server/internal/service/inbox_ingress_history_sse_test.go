package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// 历史帧是「已经发生过的事实」，不是待下发队列。历史里的 outbound 行一旦再触发下行 SSE，
// 扩展会把同一条文案重新打进页面；下一轮上行帧的窗口历史又包含它，形成自我放大的回环。
// 因此：历史 outbound 行只落库（供工作台展示），不得发布 SSE。
func TestPersistBridgeHistory_OutboundHistory_DoesNotPublishSSE(t *testing.T) {
	db := testutil.NewTestDBOrSkip(t, &model.MessageHub{})
	svc := NewInboxIngressService()

	published := make(chan uint64, 4)
	prev := GlobalSSEPublisher
	SetGlobalSSEPublisher(func(ctx context.Context, channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time) {
		published <- hubID
	})
	t.Cleanup(func() { SetGlobalSSEPublisher(prev) })

	const (
		account = "acct-xhs-hs"
		conv    = "conv-xhs-hs-1"
		content = "您好，这款商品支持七天无理由退换，请放心下单"
		msgID   = "mh:hs0000a1"
	)
	ev := &model.MessageEvent{
		Channel:        model.ChannelXHS,
		SenderID:       account,
		SenderType:     "agent",
		ReceiverID:     conv,
		Content:        content,
		EventID:        msgID,
		ConversationID: conv,
		MsgType:        "text",
		Timestamp:      time.Now().Add(-2 * time.Minute),
		Extra:          map[string]interface{}{"account_id": account},
	}
	if err := svc.PersistBridgeHistory(context.Background(), ev, "outbound"); err != nil {
		t.Fatalf("PersistBridgeHistory(outbound) 不应报错: %v", err)
	}

	var row model.MessageHub
	if err := db.Where("msg_id=? AND direction='outbound'", msgID).First(&row).Error; err != nil {
		t.Fatalf("历史 outbound 行应落库: %v", err)
	}
	if !row.IsAIReply {
		t.Fatalf("历史 outbound 行仍应标记 is_ai_reply=true")
	}

	select {
	case hubID := <-published:
		t.Fatalf("历史 outbound 行触发了 SSE 下行（hub_id=%d）：历史帧不得回灌成待下发消息", hubID)
	case <-time.After(400 * time.Millisecond):
	}
}
