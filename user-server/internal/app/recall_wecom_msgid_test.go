package app

import (
	"testing"

	"hivemtk-user/internal/model"
)

// TestPlatformMessageID_WeCom 钉住企微从 Extra["msgid"] 读平台消息 ID 的口径。
//
// 背景：TG/飞书的平台 msgid 编码在 hub.MsgID 里（tg-out-{acc}-{msgid}），
// 企微的 hub.MsgID 是 wecom-out-{uuid}（无平台号），所以企微发送侧必须把
// 平台 msgid 写入 Extra，撤回时才能反查。本测试锁住这个约定。
func TestPlatformMessageID_WeCom(t *testing.T) {
	t.Run("Extra 有 msgid 时正确返回", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "wecom",
			MsgID:    "wecom-out-abc123",
			Extra:    model.JSONMap{"msgid": "wecom-msg-xyz"},
		}
		got, err := platformMessageID(hub)
		if err != nil {
			t.Fatalf("platformMessageID err = %v, want nil", err)
		}
		if got != "wecom-msg-xyz" {
			t.Errorf("platformMessageID = %q, want %q", got, "wecom-msg-xyz")
		}
	})

	t.Run("Extra 为 nil 时返回 missing msgid 错误", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "wecom",
			MsgID:    "wecom-out-abc123",
			Extra:    nil,
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want error")
		}
		if got := err.Error(); got != "wecom hub extra missing msgid" {
			t.Errorf("err = %q, want %q", got, "wecom hub extra missing msgid")
		}
	})

	t.Run("Extra 有 msgid 但为空字符串时返回 missing msgid 错误", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "wecom",
			MsgID:    "wecom-out-abc123",
			Extra:    model.JSONMap{"msgid": ""},
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want error")
		}
	})

	t.Run("Extra 有 msgid 但类型不是字符串时返回 missing msgid 错误", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "wecom",
			MsgID:    "wecom-out-abc123",
			Extra:    model.JSONMap{"msgid": 12345},
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want error")
		}
	})
}
