package controller

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/service"
)

// 企业微信账号密钥的出参脱敏。
//
// 问题：WeComAccount 这个 model 带着 corp_secret / agent_secret / access_token /
// callback_token / encoding_aes_key 直接序列化给前端 —— 任何登录用户都能把
// 商户的企业微信应用密钥、通讯录回调密钥、AES 密钥整个读走。
// 同一仓里短信配置那条路是脱敏的（maskSMSConfig），这里漏了。
//
// 为什么脱敏放在 controller 而不是 model：model 是内部模型，
// service 要拿**真**密钥去调企业微信 API（拿 access_token、验回调签名）。
// 在 model 上做 json:"-" 或自定义 MarshalJSON 会把内部链路一起掐断。
//
// 为什么读侧脱敏、写侧要认得脱敏值：前端编辑弹窗会把列表里回填的
// corp_secret 原样提交回来（user-web/src/views/wecomAccount/List.vue 的
// openEditDialog + 提交 payload）。脱敏后前端拿到的是掩码串，
// 若后端照单全收，保存一次就把真密钥覆盖成 "****" —— 账号直接废掉。
// 所以写侧必须把"掩码串"当成"没传"（沿用 UpdateAccount 已有的
// "密钥类字段空串=保留原值"口径）。
//
// 掩码常量刻意与 service 层的空串判定同源：写侧判 isMaskedSecret。

// 掩码串的唯一事实源在 service 层（写侧判定"这是掩码 ⇒ 保留原值"用同一个值）。
// 见 service/wecom_secret_mask.go。

// maskWeComSecret 对单个密钥串脱敏：空串原样（表示"没配"），
// 短的整串替换，长的保留末 4 位（够运维认是哪一把钥匙，又不泄密）。
func maskWeComSecret(v string) string {
	if v == "" {
		return ""
	}
	if len(v) <= 4 {
		return service.WeComSecretMask
	}
	return service.WeComSecretMask + v[len(v)-4:]
}

// maskWeComAccount 就地脱敏一个账号的密钥字段。
//
// 注意是**就地**改：调用方拿到的就是同一个指针，改完再序列化即可，
// 不用把整个 model 复制一份（字段多，复制容易漏字段）。
func maskWeComAccount(acc *model.WeComAccount) {
	if acc == nil {
		return
	}
	acc.CorpSecret = maskWeComSecret(acc.CorpSecret)
	acc.AgentSecret = maskWeComSecret(acc.AgentSecret)
	acc.AccessToken = maskWeComSecret(acc.AccessToken)
	acc.CallbackToken = maskWeComSecret(acc.CallbackToken)
	acc.EncodingAESKey = maskWeComSecret(acc.EncodingAESKey)
}

// maskWeComAccounts 批量脱敏。
func maskWeComAccounts(list []*model.WeComAccount) {
	for _, acc := range list {
		maskWeComAccount(acc)
	}
}

// maskAccountWithHealth 脱敏「账号 + 健康度」这种复合出参。
func maskAccountWithHealth(list []service.AccountWithHealth) {
	for i := range list {
		maskWeComAccount(list[i].Account)
	}
}
