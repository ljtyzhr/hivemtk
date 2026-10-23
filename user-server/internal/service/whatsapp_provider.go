package service

// whatsapp_provider.go A2-c：WhatsApp 发送能力的**唯一 seam**。
//
// 为什么要这一层：本仓同时支持两种 WhatsApp 账号形态——
//   · `cloud`：Meta 官方 Cloud API（`internal/channelbot/whatsapp`，维护中的 HTTP 客户端）；
//   · `personal`：个人版 WhatsApp Web 协议，靠 `github.com/Rhymen/go-whatsapp`（gowa）。
//
// 后者是**弃维护依赖**：上游 2021 年后无提交，且 Meta 一直在收紧个人版 Web 协议。
// 它不会出现在 govulncheck 的通告里（没有 CVE），却随时可能因为协议变更而整块失效。
// 对这类依赖，正确的处置不是"等它出 CVE"，而是**把引用面收敛到一个文件**：
//   · 换库时要改的位置是可数的（本文件），不是"全仓搜一下"；
//   · 上层 service 只认 `WhatsAppProvider` 接口，删掉个人版实现时业务代码零改动。
//
// 这与 `scripts/dependency-policy.baseline` 里 gowa 那一行的 seam 是同一件事的两端：
// 基线声明"只允许这一个文件 import 它"，门（`scripts/check-dependency-policy.py`）
// 每次跑都会现测这件事还成立。改这里之前先看那行的 why 与 review 日期。
//
// 分层约束：本文件属于 service 层，只做协议适配与账号形态路由，不直接碰 DB——
// 会话持久化仍走 `repository.WhatsappRepository`（由 `WhatsappService` 注入）。

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	gowa "github.com/Rhymen/go-whatsapp"
	"github.com/google/uuid"

	"hivemtk-user/internal/channelbot/core"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
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

// WhatsAppLoginProvider 仅个人版实现的登录能力（Cloud API 没有扫码登录这一步）。
//
// 单独一个接口而不是塞进 WhatsAppProvider：Cloud API 实现被迫写两个空方法
// （`return "", errors.New("unsupported")`）时，调用侧就只能靠错误串判断能力，
// 那正是本仓反复踩过的「控制流建在字符串上」。用类型断言问能力，形态是明确的。
type WhatsAppLoginProvider interface {
	StartLogin(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (string, error)
	LoginQR(accountID uuid.UUID) (string, bool)
}

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

// ── 个人版（gowa）实现 ──────────────────────────────────────────────────────

// legacyWhatsAppProvider 个人版 WhatsApp Web 协议实现。**全仓唯一允许 import gowa 的地方**。
//
// 连接与会话都挂在这个结构上（原来散在 WhatsappService 里）：换库/删库时，
// 连同字段一起删掉即可，`WhatsappService` 不需要知道存在过 gowa。
type legacyWhatsAppProvider struct {
	repo repository.WhatsappRepository

	connMu sync.RWMutex
	conns  map[uuid.UUID]*gowa.Conn
	qrMu   sync.RWMutex
	qrs    map[uuid.UUID]string
}

func newLegacyWhatsAppProvider(repo repository.WhatsappRepository) *legacyWhatsAppProvider {
	return &legacyWhatsAppProvider{
		repo:  repo,
		conns: make(map[uuid.UUID]*gowa.Conn),
		qrs:   make(map[uuid.UUID]string),
	}
}

func (*legacyWhatsAppProvider) Kind() string { return "personal" }

// StartLogin 建连并返回扫码二维码；会话在后台 goroutine 里落库并置账号在线。
func (p *legacyWhatsAppProvider) StartLogin(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (string, error) {
	wac, err := gowa.NewConn(timeout)
	if err != nil {
		return "", err
	}
	p.connMu.Lock()
	p.conns[accountID] = wac
	p.connMu.Unlock()

	qrChan := make(chan string)
	utils.SafeGo(ctx, "whatsapp.StartLogin", func(ctx context.Context) {
		sess, err := wac.Login(qrChan)
		if err != nil {
			return
		}
		b, _ := json.Marshal(sess)
		utils.WarnErrKV("whatsapp.UpsertSession",
			p.repo.UpsertSession(ctx, &model.WhatsappSession{AccountID: accountID.String(), SessionJSON: string(b)}),
			"account_id", accountID.String())
		acc, _ := p.repo.GetAccount(ctx, accountID)
		if acc != nil {
			acc.Status = model.WhatsappStatusOnline
			utils.WarnErrKV("whatsapp.UpdateAccount",
				p.repo.UpdateAccount(ctx, acc),
				"account_id", accountID.String())
		}
	})

	qr := <-qrChan
	p.qrMu.Lock()
	p.qrs[accountID] = qr
	p.qrMu.Unlock()
	return qr, nil
}

func (p *legacyWhatsAppProvider) LoginQR(accountID uuid.UUID) (string, bool) {
	p.qrMu.RLock()
	defer p.qrMu.RUnlock()
	qr, ok := p.qrs[accountID]
	return qr, ok
}

// SendText 走个人版会话发文本。
func (p *legacyWhatsAppProvider) SendText(ctx context.Context, acc *model.WhatsappAccount, toJID, text string) (string, error) {
	wac, err := p.warmConn(ctx, acc.ID, 30*time.Second)
	if err != nil {
		return "", err
	}
	msg := gowa.TextMessage{Info: gowa.MessageInfo{RemoteJid: toJID}, Text: text}
	return wac.Send(msg)
}

// SendTemplate 个人版无模板消息：明确报「不支持」，而不是退化成发文本。
//
// 退化成纯文本的诱惑很大（"至少发出去了"），但模板消息存在的理由就是**客服窗口外**
// 才必须用它——退化成文本会被平台侧丢弃或封号，而调用方拿到的却是成功。
func (*legacyWhatsAppProvider) SendTemplate(context.Context, *model.WhatsappAccount, string, string, string, map[string]string) (string, error) {
	return "", ErrWhatsAppProviderUnsupported
}

// warmConn 取已缓存连接，没有则用库里的会话恢复一条。
func (p *legacyWhatsAppProvider) warmConn(ctx context.Context, accountID uuid.UUID, timeout time.Duration) (*gowa.Conn, error) {
	p.connMu.RLock()
	c := p.conns[accountID]
	p.connMu.RUnlock()
	if c != nil {
		return c, nil
	}
	sess, err := p.repo.GetSession(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, errors.New("no session for account")
	}
	wac, err := gowa.NewConn(timeout)
	if err != nil {
		return nil, err
	}
	var ws gowa.Session
	if err := json.Unmarshal([]byte(sess.SessionJSON), &ws); err != nil {
		return nil, err
	}
	if _, err := wac.RestoreWithSession(ws); err != nil {
		return nil, err
	}
	p.connMu.Lock()
	p.conns[accountID] = wac
	p.connMu.Unlock()
	return wac, nil
}

// ── 账号形态路由 ────────────────────────────────────────────────────────────

// whatsAppProviderFor 按账号形态选实现。
//
// 判据只有一条：**云端凭据齐不齐**。历史数据里存在 `type=cloud` 但 PhoneID/Token 为空的账号
// （早期建号时页面还没收这两个字段），对它走 Cloud API 只会得到 "cloud api credentials missing"，
// 而同一个账号的个人版会话可能还在——所以形态由「凭据是否可用」决定，类型字段只是提示。
// 这一点是刻意保留旧行为的：原 `SendTextMessage` 用的就是 `Type == "cloud" && PhoneID != "" && Token != ""`。
func whatsAppProviderFor(acc *model.WhatsappAccount, legacy *legacyWhatsAppProvider) WhatsAppProvider {
	if acc != nil && acc.Type == "cloud" && acc.PhoneID != "" && acc.Token != "" {
		return cloudWhatsAppProvider{}
	}
	if legacy == nil {
		logger.Warnf("[WhatsApp] 个人版 provider 未构造（repo 缺失），账号 %s 无法发送", accountIDOrEmpty(acc))
		return unavailableWhatsAppProvider{}
	}
	return legacy
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
