package service

// 契约：企业微信账号的密钥**读出去是掩码，写回来时掩码被当成"没改"**。
//
// 坑（两个方向同时成立才安全）：
//  1. 读侧：WeComAccount 带 corp_secret / agent_secret / access_token /
//     callback_token / encoding_aes_key 直接序列化 ⇒ 任何登录用户都能把商户的
//     企业微信应用密钥、通讯录回调密钥、AES 密钥整个读走。读侧已改脱敏
//     （见 controller/wecom_secret_mask.go）。
//  2. 写侧：前端编辑弹窗把列表里回填的 corp_secret 原样提交回来
//     （user-web/src/views/wecomAccount/List.vue 的 openEditDialog + 提交 payload）。
//     **只脱敏不认掩码** ⇒ 用户保存一次，真密钥就被覆盖成 "****"，账号直接废掉。
//
// 本文件锁写侧这一半：掩码串进来 ⇒ 原值保留；真值进来 ⇒ 覆盖。
// 读侧那一半在 controller 包里测（那里才是脱敏发生的地方）。

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// TestWeComUpdateAccountMaskedSecretKeepsOriginal 掩码串进来 ⇒ 库里原值不动。
func TestWeComUpdateAccountMaskedSecretKeepsOriginal(t *testing.T) {
	db, svc := newWeComServiceForMaskTest(t)

	const secret = "real_corp_secret_value_1234"
	acc := &model.WeComAccount{
		CorpID:         "corp1",
		CorpSecret:     secret,
		AgentSecret:    "real_agent_secret_5678",
		CallbackToken:  "real_cb_token_9012",
		EncodingAESKey: "real_aes_key_3456",
	}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("种子账号落库失败：%v", err)
	}

	// 模拟前端：把列表里回填的**掩码串**原样提交回来。
	// 注意掩码的真实形态是"掩码前缀 + 末 4 位"（如 "****1234"），
	// 不是裸的 "****" —— 写侧必须按**前缀**认，否则认不出来，
	// 就会把掩码当用户新填的密钥、把真密钥覆盖掉（正是本测试要防的事故）。
	_, err := svc.UpdateAccount(context.Background(), acc.ID, &CreateAccountRequest{
		CorpID:         "corp1",
		CorpSecret:     WeComSecretMask + "1234",
		AgentSecret:    WeComSecretMask + "5678",
		CallbackToken:  WeComSecretMask + "3456",
		EncodingAESKey: WeComSecretMask + "7890",
	})
	if err != nil {
		t.Fatalf("UpdateAccount 失败：%v", err)
	}

	var got model.WeComAccount
	if err := db.Where("id = ?", acc.ID).First(&got).Error; err != nil {
		t.Fatalf("读回账号失败：%v", err)
	}
	if got.CorpSecret != secret {
		t.Errorf("掩码串不得覆盖真密钥：corp_secret 期望保持 %q，实际 %q", secret, got.CorpSecret)
	}
	if got.AgentSecret != "real_agent_secret_5678" {
		t.Errorf("掩码串不得覆盖真密钥：agent_secret 实际 %q", got.AgentSecret)
	}
	if got.CallbackToken != "real_cb_token_9012" {
		t.Errorf("掩码串不得覆盖真密钥：callback_token 实际 %q", got.CallbackToken)
	}
	if got.EncodingAESKey != "real_aes_key_3456" {
		t.Errorf("掩码串不得覆盖真密钥：encoding_aes_key 实际 %q", got.EncodingAESKey)
	}
}

// TestWeComUpdateAccountRealSecretStillOverwrites 真值进来仍然要能覆盖（脱敏不能把更新功能焊死）。
func TestWeComUpdateAccountRealSecretStillOverwrites(t *testing.T) {
	db, svc := newWeComServiceForMaskTest(t)

	acc := &model.WeComAccount{CorpID: "corp1", CorpSecret: "old_secret_0000"}
	if err := db.Create(acc).Error; err != nil {
		t.Fatalf("种子账号落库失败：%v", err)
	}

	newSecret := "brand_new_secret_9999"
	if _, err := svc.UpdateAccount(context.Background(), acc.ID, &CreateAccountRequest{
		CorpID:     "corp1",
		CorpSecret: newSecret,
	}); err != nil {
		t.Fatalf("UpdateAccount 失败：%v", err)
	}

	var got model.WeComAccount
	if err := db.Where("id = ?", acc.ID).First(&got).Error; err != nil {
		t.Fatalf("读回账号失败：%v", err)
	}
	if got.CorpSecret != newSecret {
		t.Errorf("真值必须能覆盖旧密钥：期望 %q，实际 %q", newSecret, got.CorpSecret)
	}
}

// newWeComServiceForMaskTest 起一个带测试库的 WeComService。
func newWeComServiceForMaskTest(t *testing.T) (*gorm.DB, *WeComService) {
	t.Helper()
	_, db := newWeComHealthService(t)
	return db, NewWeComService()
}
