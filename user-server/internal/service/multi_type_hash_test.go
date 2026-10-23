package service

import "testing"

// 002-multi-type-messages FR5.1：非文本消息的 msg_id 必须并入类型与媒体 URL。
//
// 修复前的口径：ContentHashMsgID(channel, convID, content) —— 同会话客户连发
// 两张图（content 同为「[图片]」占位符）产出同一 msg_id，入站幂等把第二条判重
// 吞掉。此处锁三个口径：
//  1. 文本消息（含空类型）与旧哈希逐字节一致 —— 存量行为不迁移不重算；
//  2. 同占位符、不同 media_url 的两张图 → msg_id 不同；
//  3. 同图（同 media_url）重发 → msg_id 相同（幂等仍有效）。
func TestContentHashMsgIDTyped(t *testing.T) {
	legacy := ContentHashMsgID("douyin", "c1", "在吗")

	if got := ContentHashMsgIDTyped("douyin", "c1", "text", "", "在吗"); got != legacy {
		t.Fatalf("text 类型必须与旧口径逐字节一致: got %s want %s", got, legacy)
	}
	if got := ContentHashMsgIDTyped("douyin", "c1", "", "", "在吗"); got != legacy {
		t.Fatalf("空类型按 text 处理: got %s want %s", got, legacy)
	}
	if got := ContentHashMsgIDTyped("douyin", "c1", "TEXT", "", "在吗"); got != legacy {
		t.Fatalf("大小写不敏感: got %s want %s", got, legacy)
	}

	imgA := ContentHashMsgIDTyped("douyin", "c1", "image", "https://f/x1.png", "[图片]")
	imgB := ContentHashMsgIDTyped("douyin", "c1", "image", "https://f/x2.png", "[图片]")
	if imgA == imgB {
		t.Fatalf("同会话两张不同图必须哈希不同（修复点）: %s", imgA)
	}
	if imgA == ContentHashMsgID("douyin", "c1", "[图片]") {
		t.Fatal("非文本哈希不得退化回内容-only 口径")
	}

	imgA2 := ContentHashMsgIDTyped("douyin", "c1", "image", "https://f/x1.png", "[图片]")
	if imgA != imgA2 {
		t.Fatalf("同一条图重发必须哈希稳定（幂等前提）: %s vs %s", imgA, imgA2)
	}

	// 类型不同、URL 相同（图与同 URL 视频）也应区分
	vid := ContentHashMsgIDTyped("douyin", "c1", "video", "https://f/x1.png", "[图片]")
	if vid == imgA {
		t.Fatal("类型必须参与哈希")
	}
}
