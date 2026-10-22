package controller

// 契约：企业微信账号的**读接口不得把密钥原文吐给前端**。
//
// 坑：WeComAccount 带着 corp_secret / agent_secret / access_token /
// callback_token / encoding_aes_key 直接序列化 ⇒ 任何登录用户都能把商户的
// 企业微信应用密钥、通讯录回调密钥、AES 密钥整个读走。
// 同一仓里短信配置那条路是脱敏的（maskSMSConfig），这里漏了。
//
// 写侧（掩码串进来要当"没改"）在 service/wecom_secret_mask_test.go 里锁。

import (
	"encoding/json"
	"strings"
	"testing"

	"hivemtk-user/internal/model"
)

// TestMaskWeComAccountMasksAllSecretFields 五个密钥字段都要被换成掩码。
func TestMaskWeComAccountMasksAllSecretFields(t *testing.T) {
	acc := &model.WeComAccount{
		CorpID:         "corp1",
		CorpSecret:     "real_corp_secret_1234",
		AgentSecret:    "real_agent_secret_5678",
		AccessToken:    "real_access_token_9012",
		CallbackToken:  "real_cb_token_3456",
		EncodingAESKey: "real_aes_key_7890",
	}
	maskWeComAccount(acc)

	if strings.Contains(acc.CorpSecret, "real_corp_secret") {
		t.Errorf("corp_secret 未脱敏：%q", acc.CorpSecret)
	}
	if strings.Contains(acc.AgentSecret, "real_agent_secret") {
		t.Errorf("agent_secret 未脱敏：%q", acc.AgentSecret)
	}
	if strings.Contains(acc.AccessToken, "real_access_token") {
		t.Errorf("access_token 未脱敏：%q", acc.AccessToken)
	}
	if strings.Contains(acc.CallbackToken, "real_cb_token") {
		t.Errorf("callback_token 未脱敏：%q", acc.CallbackToken)
	}
	if strings.Contains(acc.EncodingAESKey, "real_aes_key") {
		t.Errorf("encoding_aes_key 未脱敏：%q", acc.EncodingAESKey)
	}
}

// TestMaskWeComAccountNeverLeaksSecretSubstring 脱敏后的串不得含原文的任何片段。
func TestMaskWeComAccountNeverLeaksSecretSubstring(t *testing.T) {
	acc := &model.WeComAccount{CorpSecret: "super_secret_value"}
	maskWeComAccount(acc)

	// 掩码保留末 4 位是设计（够运维认是哪一把钥匙），但不得保留更长片段。
	if strings.Contains(acc.CorpSecret, "super_secret") {
		t.Errorf("corp_secret 仍含原文长片段：%q", acc.CorpSecret)
	}
}

// TestMaskWeComAccountEmptySecretStaysEmpty 没配的密钥保持空串（不要凭空造出 "****"）。
func TestMaskWeComAccountEmptySecretStaysEmpty(t *testing.T) {
	acc := &model.WeComAccount{CorpSecret: "", AgentSecret: ""}
	maskWeComAccount(acc)

	if acc.CorpSecret != "" {
		t.Errorf("未配置的 corp_secret 应保持空串，实际 %q", acc.CorpSecret)
	}
	if acc.AgentSecret != "" {
		t.Errorf("未配置的 agent_secret 应保持空串，实际 %q", acc.AgentSecret)
	}
}

// TestMaskWeComAccountIsIdempotent 脱敏两次不能变成 "****1234" 这种叠加（前端会重复提交掩码）。
func TestMaskWeComAccountIsIdempotent(t *testing.T) {
	acc := &model.WeComAccount{CorpSecret: "real_secret_1234"}
	maskWeComAccount(acc)
	first := acc.CorpSecret
	maskWeComAccount(acc)
	if acc.CorpSecret != first {
		t.Errorf("脱敏必须幂等：第一次 %q，第二次 %q", first, acc.CorpSecret)
	}
	// 且第二次之后仍然要能被写侧认成掩码。
	if !strings.HasPrefix(acc.CorpSecret, "****") {
		t.Errorf("脱敏结果必须以掩码开头（写侧据此判定保留原值），实际 %q", acc.CorpSecret)
	}
}

// TestMaskWeComAccountJSONHasNoPlainSecret 序列化后的 JSON 里不得出现明文密钥。
func TestMaskWeComAccountJSONHasNoPlainSecret(t *testing.T) {
	acc := &model.WeComAccount{CorpSecret: "plaintext_secret_value"}
	maskWeComAccount(acc)

	raw, err := json.Marshal(acc)
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	if strings.Contains(string(raw), "plaintext_secret_value") {
		t.Errorf("JSON 里仍含明文密钥：%s", raw)
	}
}

// TestMaskWeComAccountNilSafe nil 不能 panic（controller 里拿到 nil 账号是可能的）。
func TestMaskWeComAccountNilSafe(t *testing.T) {
	maskWeComAccount(nil) // 不应 panic
}
