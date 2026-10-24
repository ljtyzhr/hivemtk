package app

import (
	"strings"
	"testing"

	"hivemtk-user/internal/model"
)

// TestPlatformMessageID_FourChannelMatrix 四渠道反查矩阵（D-9 验收判据，
// 规划 §3.9："四渠道各一条'发送→回执→按回执查库→撤回'矩阵"的库内半段——
// hub 行进 platformMessageID 能否得出各自渠道的平台消息号）。
//
// 与 recall_wecom_msgid_test.go 的分工：那边钉企微一格（Extra["msgid"] 口径），
// 这里补齐其余三格（TG/飞书行键末段、WA wamid 本体、platform_msg_id 一等列
// 对四渠道的优先命中）与不支持渠道的明确报错。撤回的平台 API 半段
// （recallTelegram/Feishu/WeCom 的外呼）由集成侧测试覆盖，这里不碰网络。
func TestPlatformMessageID_FourChannelMatrix(t *testing.T) {
	t.Run("TG 行键末段（tg-out-{acc}-{平台号}）", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "telegram",
			MsgID:    "tg-out-acc-1-4242",
		}
		got, err := platformMessageID(hub)
		if err != nil {
			t.Fatalf("platformMessageID err = %v, want nil", err)
		}
		if got != "4242" {
			t.Errorf("platformMessageID = %q, want %q（行键末段=平台消息号）", got, "4242")
		}
	})

	t.Run("飞书行键末段（feishu-out-{acc}-{平台id}）", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "feishu",
			MsgID:    "feishu-out-acc-2-om_9f8e",
		}
		got, err := platformMessageID(hub)
		if err != nil {
			t.Fatalf("platformMessageID err = %v, want nil", err)
		}
		if got != "om_9f8e" {
			t.Errorf("platformMessageID = %q, want %q", got, "om_9f8e")
		}
	})

	t.Run("WA 行（列为空）报 unsupported——撤回在分发层就被明确拒绝", func(t *testing.T) {
		// WA Cloud API 无主动撤回接口：recall 分发层对 whatsapp 先行返回
		// ErrRecallNotSupported（不伪装成功，规划判据），根本不会走到反查。
		// 所以 platformMessageID 不认 whatsapp 是**对的**——给它加解析
		// 是预留死代码（与预留列同罪）。这里钉住的是：万一未来有人把
		// 分发顺序改反（先反查后判渠道），WA 会得到明确错误而不是猜一个 wamid。
		hub := &model.MessageHub{
			Platform: "whatsapp",
			MsgID:    "wamid.7f3a2b9c0d1e",
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want unsupported（WA 撤回由分发层拒绝，反查不猜 wamid）")
		}
		if !strings.Contains(err.Error(), "unsupported platform") {
			t.Errorf("err = %q, want 含 unsupported platform", err.Error())
		}
	})

	t.Run("WA 行（列有值）照常命中一等列", func(t *testing.T) {
		// v3.82.0 迁移把 WA 出站行回填 platform_msg_id=wamid 本体：
		// 列有值 ⇒ 反查直接用（撤回不支持，但回执对账等读路径共用这一列）。
		hub := &model.MessageHub{
			Platform:      "whatsapp",
			MsgID:         "wamid.7f3a2b9c0d1e",
			PlatformMsgID: "wamid.7f3a2b9c0d1e",
		}
		got, err := platformMessageID(hub)
		if err != nil {
			t.Fatalf("platformMessageID err = %v, want nil", err)
		}
		if got != "wamid.7f3a2b9c0d1e" {
			t.Errorf("platformMessageID = %q, want %q（列优先，不看渠道白名单）", got, "wamid.7f3a2b9c0d1e")
		}
	})

	t.Run("TG 行键段数不足时报格式错而不是猜", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "telegram",
			MsgID:    "tg-out", // 只有 2 段，切不出平台号
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want error（不猜）")
		}
		if !strings.Contains(err.Error(), "invalid hub key format") {
			t.Errorf("err = %q, want 含 invalid hub key format", err.Error())
		}
	})

	t.Run("platform_msg_id 一等列对四渠道都优先命中", func(t *testing.T) {
		// D-9 新合同：列有值 ⇒ 直接用，不再走行键解析/Extra 兜底。
		// 四渠道同一判据——列是迁移 v3.82.0 起的一等合同，不该有渠道特权。
		for _, p := range []string{"telegram", "feishu", "whatsapp", "wecom"} {
			hub := &model.MessageHub{
				Platform:      p,
				MsgID:         "legacy-row-key",
				PlatformMsgID: "pmid-from-column",
			}
			got, err := platformMessageID(hub)
			if err != nil {
				t.Fatalf("platform=%s err = %v, want nil", p, err)
			}
			if got != "pmid-from-column" {
				t.Errorf("platform=%s platformMessageID = %q, want %q（列优先）", p, got, "pmid-from-column")
			}
		}
	})

	t.Run("不支持反查的渠道报 unsupported 而不是伪装", func(t *testing.T) {
		hub := &model.MessageHub{
			Platform: "dingtalk",
			MsgID:    "dt-out-abc",
		}
		_, err := platformMessageID(hub)
		if err == nil {
			t.Fatal("platformMessageID err = nil, want error（规划判据：不支持撤回的渠道有明确错误，不伪装成功）")
		}
		if !strings.Contains(err.Error(), "unsupported platform") {
			t.Errorf("err = %q, want 含 unsupported platform", err.Error())
		}
	})
}
