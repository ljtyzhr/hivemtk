package service

// whatsapp_provider.go A2-c：WhatsApp 发送能力的**唯一 seam**。
//
// 账号形态只剩一种：`cloud`（Meta 官方 Cloud API，`internal/channelbot/whatsapp`
// 维护中的 HTTP 客户端）。个人版（`github.com/Rhymen/go-whatsapp`，gowa）已于
// 2026-10-25 按 seam 里的预案删除（W-3 换库收口）：上游 2021 年后无提交、协议
// 已被 Meta 收紧，且换库前实测共享库 30 个账号全部 type=cloud——个人版实现
// 已经没有服务对象。`dependency-policy.baseline` 的 gowa 行同批移除。
//
// 本文件由此回归"协议适配 + 账号形态路由"单一职责：非 cloud 凭据形态的账号
// 统一落 `unavailableWhatsAppProvider`（明确的失败，不是 nil panic）。
//
// 分层约束：本文件属于 service 层，只做协议适配与账号形态路由，不直接碰 DB。

import (
	"context"
	"errors"
	"sort"
	"strings"

	"hivemtk-user/internal/channelbot/core"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// WhatsAppProvider 一种 WhatsApp 发送实现。
//
// 三个方法都带 account：Cloud API 要用 PhoneID/Token，个人版要用会话连接，
// 两者都按账号取，所以接口按账号构造而不是按渠道构造。
type WhatsAppProvider interface {
	// Kind 账号形态标识（"cloud" / "personal"），进日志与错误串，排障第一眼要能分辨。
	Kind() string
	// SendText 发纯文本。返回平台侧消息 ID。
	SendText(ctx context.Context, acc *model.WhatsappAccount, toJID, text string) (string, error)
	// SendTemplate 发模板消息（客服窗口外必须走模板）。个人版不支持，返回明确错误。
	SendTemplate(ctx context.Context, acc *model.WhatsappAccount, to, templateName, language string, params map[string]string) (string, error)
}

// 单独一个接口而不是塞进 WhatsAppProvider：Cloud API 实现被迫写两个空方法
// （`return "", errors.New("unsupported")`）时，调用侧就只能靠错误串判断能力，
// 那正是本仓反复踩过的「控制流建在字符串上」。用类型断言问能力，形态是明确的。
// ErrWhatsAppProviderUnsupported 该账号形态不支持该操作（如个人版不支持模板消息）。
var ErrWhatsAppProviderUnsupported = errors.New("whatsapp provider does not support this operation")

// ── Cloud API 实现 ──────────────────────────────────────────────────────────

// cloudWhatsAppProvider Meta 官方 Cloud API。
type cloudWhatsAppProvider struct{}

func (cloudWhatsAppProvider) Kind() string { return "cloud" }

func (cloudWhatsAppProvider) SendText(ctx context.Context, acc *model.WhatsappAccount, toJID, text string) (string, error) {
	cli := whatsapp.NewCloudClient(acc.PhoneID, acc.Token)
	return cli.SendText(ctx, normalizeWhatsAppPhone(toJID), text)
}

func (cloudWhatsAppProvider) SendTemplate(ctx context.Context, acc *model.WhatsappAccount, to, templateName, language string, params map[string]string) (string, error) {
	cli := whatsapp.NewCloudClient(acc.PhoneID, acc.Token)
	return cli.SendTemplate(ctx, normalizeWhatsAppPhone(to), templateName, language, buildWhatsAppTemplateComponents(params))
}

// normalizeWhatsAppPhone Cloud API 只认 E.164（`+8613800138000`），而库里两个来源的写法不同：
// 个人版 JID 是 `13800138000@s.whatsapp.net`，Cloud API 账号存的可能是裸号码。
// 两种都得归一，否则 Cloud API 会以 "Invalid parameter" 拒收，且错误串里看不出是号码格式问题。
func normalizeWhatsAppPhone(to string) string {
	phone := to
	if strings.Contains(phone, "@") {
		phone = strings.Split(phone, "@")[0]
	}
	if !strings.HasPrefix(phone, "+") {
		phone = "+" + phone
	}
	return phone
}

// buildWhatsAppTemplateComponents 模板参数 → Cloud API body 组件。
//
// 注意 map 的遍历顺序不稳定：多参数模板的占位符顺序由 key 排序固定，
// 否则同一条消息两次发送会给出不同的参数顺序（模板参数错位在客户端表现为
// "字段串行"，排查成本极高）。调用方传单参数时无影响。
func buildWhatsAppTemplateComponents(params map[string]string) []core.TemplateComponent {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	bodyParams := make([]core.TemplateParameter, 0, len(keys))
	for _, k := range keys {
		bodyParams = append(bodyParams, core.TemplateParameter{Type: "text", Text: params[k]})
	}
	return []core.TemplateComponent{{Type: "body", Parameters: bodyParams}}
}

// ── 账号形态路由 ────────────────────────────────────────────────────────────

// whatsAppProviderFor 按账号形态选实现。
//
// 判据只有一条：**云端凭据齐不齐**。历史数据里存在 `type=cloud` 但 PhoneID/Token 为空的账号
// （早期建号时页面还没收这两个字段），对它走 Cloud API 只会得到 "cloud api credentials missing"，
// 而同一个账号的个人版会话可能还在——所以形态由「凭据是否可用」决定，类型字段只是提示。
// 这一点是刻意保留旧行为的：原 `SendTextMessage` 用的就是 `Type == "cloud" && PhoneID != "" && Token != ""`。
func whatsAppProviderFor(acc *model.WhatsappAccount) WhatsAppProvider {
	if acc != nil && acc.Type == "cloud" && acc.PhoneID != "" && acc.Token != "" {
		return cloudWhatsAppProvider{}
	}
	kind := "<nil>"
	if acc != nil {
		kind = acc.Type
	}
	logger.Warnf("[WhatsApp] 账号 %s 非 Cloud 凭据形态（type=%s，个人版已下线），无法发送",
		accountIDOrEmpty(acc), kind)
	return unavailableWhatsAppProvider{}
}

// unavailableWhatsAppProvider 兜底实现：把「没有可用实现」变成一次明确的失败。
//
// 存在意义：让 service 层不必写 `if provider == nil`。nil 判空漏一处就是 panic，
// 而 panic 在发送协程里会被 SafeGo 吃掉、调用方永远等不到结果。
type unavailableWhatsAppProvider struct{}

func (unavailableWhatsAppProvider) Kind() string { return "unavailable" }

func (unavailableWhatsAppProvider) SendText(context.Context, *model.WhatsappAccount, string, string) (string, error) {
	return "", ErrWhatsAppProviderUnsupported
}

func (unavailableWhatsAppProvider) SendTemplate(context.Context, *model.WhatsappAccount, string, string, string, map[string]string) (string, error) {
	return "", ErrWhatsAppProviderUnsupported
}

func accountIDOrEmpty(acc *model.WhatsappAccount) string {
	if acc == nil {
		return "<nil>"
	}
	return acc.ID.String()
}
