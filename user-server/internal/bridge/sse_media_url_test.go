package bridge

import (
	"testing"
	"time"

	"hivemtk-user/internal/model"
)

// 多类型消息（002-multi-type-messages）：SSE 出站事件必须带 media_url，
// 且与 HTTP outbox 轮询路径（writeOutboxJSON 的 BridgeOutboxMessage）逐键对齐——
// R-B1 契约在媒体维度上的延伸：轮询路径一直带 media_url，SSE 路径缺键会让
// 同一条 image 出站在两条投递路径上呈现不同的载荷，扩展端按 msg_type 分派
// 媒体投递时行为分裂。
func TestBuildOutboundSSEEvent_MediaURL(t *testing.T) {
	now := time.Now()
	ev := BuildOutboundSSEEvent(OutboundEventData{
		HubID: 43, MsgID: "mh:00550fee", Platform: "douyin", AccountID: "a1",
		ConversationID: "c1", Content: "[图片]", MsgType: "image",
		MediaURL:   "https://files.internal/channels/douyin/2026/10/x.png",
		ReceiverID: "r1", CreatedAt: now,
	})
	if ev.Data["media_url"] != "https://files.internal/channels/douyin/2026/10/x.png" {
		t.Fatalf("SSE Data.media_url 缺失或错误: %v", ev.Data["media_url"])
	}
	if ev.Data["msg_type"] != "image" {
		t.Fatalf("msg_type 应透传: %v", ev.Data["msg_type"])
	}
}

// 空媒体_url 也必须显式出现（键集一致），否则扩展端 Data.media_url === undefined
// 与 "" 的语义分叉。
func TestBuildOutboundSSEEvent_MediaURLAlwaysPresent(t *testing.T) {
	ev := BuildOutboundSSEEvent(OutboundEventData{HubID: 1, MsgID: "m", Platform: "p", AccountID: "a", ConversationID: "c", Content: "x", MsgType: "text"})
	if _, ok := ev.Data["media_url"]; !ok {
		t.Fatal("文本消息的 SSE Data 也必须带 media_url 键（空串），保证两条投递路径键集一致")
	}
}

// DB 补拉路径的媒体维度一致性：buildOutboundEvent(row) 必须把 row.MediaURL 带进 Data。
func TestOutboxDBFetcherMediaURLParity(t *testing.T) {
	row := model.MessageHub{
		ID: 9, MsgID: "mh:media0001", Platform: "xhs", AccountID: "a1",
		ConversationID: "c1", Content: "[视频]", MsgType: "video",
		MediaURL:   "https://files.internal/channels/xhs/v.mp4",
		ReceiverID: "r1", CreatedAt: time.Now(),
	}
	ev := buildOutboundEvent(row)
	if ev.Data["media_url"] != row.MediaURL {
		t.Fatalf("DB 补拉路径 media_url 丢失: got %v want %s", ev.Data["media_url"], row.MediaURL)
	}
	// 与 outbox JSON（writeOutboxJSON → channelgw.OutboxMessage）的媒体字段对齐：
	// OutboxMessage.MediaURL 即 hub.MediaURL，两侧必须同源。
	if ev.Data["msg_type"] != row.MsgType {
		t.Fatalf("msg_type 丢失: %v", ev.Data["msg_type"])
	}
}
