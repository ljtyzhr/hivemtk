package service

import "strings"

// 企业微信账号密钥的脱敏口径（读侧脱敏 / 写侧认掩码）。
//
// 读侧（controller/wecom_secret_mask.go）把 corp_secret / agent_secret /
// access_token / callback_token / encoding_aes_key 换成掩码再给前端；
// 写侧（WeComService.UpdateAccount）必须认得这个掩码，
// 把它当"前端没改这个字段"⇒ 保留库里原值。
//
// 为什么两件事必须成对：前端编辑弹窗把列表里回填的 corp_secret 原样提交回来
//（user-web/src/views/wecomAccount/List.vue 的 openEditDialog + 提交 payload）。
// 只脱敏不认掩码 ⇒ 用户保存一次，真密钥就被覆盖成 "****"，账号直接废掉。
//
// 口径放在 service 层（而不是 controller）是因为**判定的那一端**在 service：
// controller 只负责产出掩码串，service 负责判断"这个值是不是掩码"。

// WeComSecretMask 密钥出参的统一掩码前缀。
//
// 是**前缀**而不是整个值：读侧脱敏产出的是"掩码 + 末 4 位"（如 "****cret"），
// 用来让运维认得出是哪一把钥匙。写侧要认的是**这一类**值，不是某一个具体串。
const WeComSecretMask = "****"

// isMaskedSecretValue 判断一个值是不是掩码串（写侧据此保留原值）。
//
// 判据是**前缀**匹配，不是全等：脱敏产出的是 "****cret" 这种"掩码+末4位"，
// 全等比对只认得 "****" 一个串，写侧就会把 "****cret" 当成用户新填的密钥，
// 把真密钥覆盖掉 —— 那正是这个修复要防的事故本身。
//
// 为什么用前缀而不是别的标记：前缀是脱敏产出的**唯一**形态，
// 而真密钥以 "****" 开头的概率可忽略（真密钥是 base64/hex 串）。
// 误判方向也要想清楚：把真密钥误认成掩码 ⇒ 保留原值（本次编辑没改这个字段，
// 结果正确）；把掩码误认成真密钥 ⇒ 覆盖成掩码（账号废掉）。宁可前者。
func isMaskedSecretValue(v string) bool {
	return strings.HasPrefix(v, WeComSecretMask)
}
