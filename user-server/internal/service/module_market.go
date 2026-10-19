package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/config"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

// 功能模块市场的服务层错误
var (
	ErrPlatformUnavailable = bizerr.New(bizerr.CodeModulePlatformUnavail, "平台未连接，无法办理模块开通")
	ErrModuleNotInCatalog  = errors.New("模块不在目录中")
	ErrModuleAlreadyOwned  = errors.New("模块已开通")
	ErrModuleOrderNoClient = errors.New("缺少订单幂等键")
	ErrNoContactForPayment = errors.New("请填写联系方式（平台运营需要据此联系你完成线下付款）")
)

// ModuleMarketItem 市场卡片项
type ModuleMarketItem struct {
	ModuleKey    string   `json:"module_key"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	PriceCents   int64    `json:"price_cents"`
	BillingCycle string   `json:"billing_cycle"`
	Icon         string   `json:"icon"`
	Summary      string   `json:"summary"`
	DependsOn    []string `json:"depends_on"`
	MenuCount    int      `json:"menu_count"`
	Owned        bool     `json:"owned"`
	OwnedStatus  string   `json:"owned_status,omitempty"`
	// FreeToEnable 表示「收费模块但 0 元」：需开通、不收钱
	FreeToEnable bool `json:"free_to_enable"`
	// Free 表示基础模块：本来就有
	Free bool `json:"free"`
}

// ModuleMarketDetail 市场详情
type ModuleMarketDetail struct {
	ModuleMarketItem
	Description string                  `json:"description"`
	MenuKeys    []string                `json:"menu_keys"`
	Options     []model.ModuleOptionRow `json:"options"`
	// Closure 依赖闭包里"会被自动开通"的那些（基础/免费）
	AutoEnabled []string `json:"auto_enabled"`
	// PayableDependencies 依赖闭包里"必须自己买"的那些（收费）
	PayableDependencies []string `json:"payable_dependencies"`
	// PlatformEnabled 平台是否连通。前端据此禁用订阅按钮 ——
	// 少了它，离线部署时详情页的「去订阅」按钮会照常可点，
	// 用户点下去才收到一句"平台未连接"，而按钮本身在界面上没有任何异样。
	PlatformEnabled bool `json:"platform_enabled"`
}

// MarketList 市场列表。第二个返回值是平台是否连通。
func (s *ModuleCatalogService) MarketList(ctx context.Context) ([]ModuleMarketItem, bool, error) {
	owned, err := s.ownedRows(ctx)
	if err != nil {
		return nil, false, err
	}
	out := make([]ModuleMarketItem, 0, len(model.ModuleCatalogMeta))
	for _, meta := range model.ModuleCatalogMeta {
		menuKeys := model.ModuleMenuBindings[meta.Key]
		row := owned[meta.Key]
		price := marketPriceOf(meta.PriceCents, row)
		item := ModuleMarketItem{
			ModuleKey:    meta.Key,
			Name:         meta.Name,
			Category:     meta.Category,
			PriceCents:   price,
			BillingCycle: meta.BillingCycle,
			Icon:         meta.Icon,
			Summary:      meta.Summary,
			DependsOn:    meta.DependsOn,
			MenuCount:    len(menuKeys),
			Owned:        row != nil && row.Status == model.TenantModuleActive,
			OwnedStatus:  ownedStatusOf(row),
			Free:         meta.Category == model.ModuleCategoryBase,
			FreeToEnable: meta.Category == model.ModuleCategoryPaid && price == 0,
		}
		out = append(out, item)
	}
	return out, config.PlatformEnabled(), nil
}

// MarketDetail 模块详情（含依赖闭包拆分）
func (s *ModuleCatalogService) MarketDetail(ctx context.Context, moduleKey string) (*ModuleMarketDetail, error) {
	var meta *model.ModuleMeta
	for i := range model.ModuleCatalogMeta {
		if model.ModuleCatalogMeta[i].Key == moduleKey {
			meta = &model.ModuleCatalogMeta[i]
			break
		}
	}
	if meta == nil {
		return nil, ErrModuleNotInCatalog
	}
	owned, err := s.ownedRows(ctx)
	if err != nil {
		return nil, err
	}
	row := owned[moduleKey]
	price := marketPriceOf(meta.PriceCents, row)
	menuKeys := model.ModuleMenuBindings[moduleKey]

	d := &ModuleMarketDetail{
		ModuleMarketItem: ModuleMarketItem{
			ModuleKey: moduleKey, Name: meta.Name, Category: meta.Category,
			PriceCents: price, BillingCycle: meta.BillingCycle,
			Icon: meta.Icon, Summary: meta.Summary, DependsOn: meta.DependsOn,
			MenuCount:    len(menuKeys),
			Owned:        row != nil && row.Status == model.TenantModuleActive,
			OwnedStatus:  ownedStatusOf(row),
			Free:         meta.Category == model.ModuleCategoryBase,
			FreeToEnable: meta.Category == model.ModuleCategoryPaid && price == 0,
		},
		MenuKeys:        menuKeys,
		PlatformEnabled: config.PlatformEnabled(),
	}
	// 依赖闭包拆分：基础/免费会自动开通，收费的要用户自己勾
	for _, dep := range model.ClosureOfModules(meta.Key) {
		if dep == moduleKey {
			continue
		}
		if model.OwnerModuleCategory(dep) == model.ModuleCategoryBase {
			d.AutoEnabled = append(d.AutoEnabled, dep)
		} else {
			d.PayableDependencies = append(d.PayableDependencies, dep)
		}
	}
	return d, nil
}

// marketPriceOf 市场定价口径（003 T007）：sync 镜像优先，本地常量兜底。
//
// 平台端是运行时定价权威（003 spec R3/Q4）：镜像行里的 PriceCents 是最近
// 一轮同步落下的目录快照，平台改价后经 sync 即在市场页生效；没有镜像行的
// 模块（从未同步 / 未拥有）回落编译期种子价 —— 与平台种子同源，不会出现
// "两边两个价"。镜像价为 0 也以镜像为准：平台把模块改成免费是合法运营
// 动作，不该被本地兜底价顶回去。
func marketPriceOf(metaPrice int64, row *model.TenantModule) int64 {
	if row != nil {
		return row.PriceCents
	}
	return metaPrice
}

// ownedStatusOf 镜像行状态（无行返回空串）
func ownedStatusOf(row *model.TenantModule) string {
	if row == nil {
		return ""
	}
	return row.Status
}

// ownedRows 本地镜像：module_key → 行（含状态与目录快照价格）。
// 匿名可逛语义（003 活体联调修复）：模块市场列表/详情是公开橱窗（MODULE_MARKET_UI），
// on 模式下未登录访问无租户上下文——此时不查库、直接返回空映射（owned 全 false），
// 否则 tenancyidx fail-closed 会把匿名橱窗打成 5000。已登录路径照常按租户过滤。
func (s *ModuleCatalogService) ownedRows(ctx context.Context) (map[string]*model.TenantModule, error) {
	if tenant.IsMultiTenant() {
		if _, ok := tenant.FromContext(ctx); !ok {
			return map[string]*model.TenantModule{}, nil
		}
	}
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*model.TenantModule, len(rows))
	for _, r := range rows {
		m[r.ModuleKey] = r
	}
	return m, nil
}

// ---------------------------------------------------------- 下单

// MarketOrderInput 下单入参
type MarketOrderInput struct {
	ModuleKeys   []string                   `json:"module_keys"`
	Options      map[string]map[string]bool `json:"options"`
	ContactName  string                     `json:"contact_name"`
	ContactPhone string                     `json:"contact_phone"`
	ContactEmail string                     `json:"contact_email"`
	Remark       string                     `json:"remark"`
	// ClientOrderNo 前端可传；不传则本地生成。传了才能做"重试不重复下单"。
	ClientOrderNo string `json:"client_order_no"`
}

// MarketOrderResult 下单结果
type MarketOrderResult struct {
	OrderNo      string                     `json:"order_no"`
	Status       string                     `json:"status"`
	TotalCents   int64                      `json:"total_amount_cents"`
	Items        []platform.ModuleOrderItem `json:"items"`
	PaymentGuide string                     `json:"payment_guide"`
	Duplicate    bool                       `json:"duplicate"`
	// Payment 平台下发的支付参数段（003 T018：epay pay_url/params/method）。
	// 平台未配置通道/关渠道 → 空，前端回落线下付款指引（零代码回滚路径）。
	Payment *platform.OrderPayment `json:"payment,omitempty"`
	// PlatformEnabled=false 时本地只留了一单，运营看到它再补同步
	PlatformEnabled bool `json:"platform_enabled"`
}

// CreateOrder 创建订单。
//
// 一期无支付通道，本方法**不做任何扣费、不做任何开通** —— 只建单 + 提示联系本人。
// 开通发生在平台端确认收款之后（服务端 MarkPaid → tenant_modules），
// 用户端只通过模块同步感知到变化。
//
// 联系人约束（003 T018 / ADR-M9 #6 放宽）：**线下路径**（无 payment 段返回、
// 走 payment_guide 引导）仍强制留联系方式；**在线支付路径**（平台返回
// payment.pay_url）不强制 —— 在线支付不需要先加好友，拦联系方式只会把
// 已经扫码付款的人挡在门外。判定只能在平台响应之后：下单前不知道平台会
// 走哪条路（渠道开关在平台端）。
func (s *ModuleCatalogService) CreateOrder(ctx context.Context, in MarketOrderInput) (*MarketOrderResult, error) {
	if !config.PlatformEnabled() {
		return nil, ErrPlatformUnavailable
	}
	if len(in.ModuleKeys) == 0 {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "请选择要开通的模块")
	}
	hasContact := strings.TrimSpace(in.ContactPhone) != "" || strings.TrimSpace(in.ContactEmail) != ""
	clientNo := strings.TrimSpace(in.ClientOrderNo)
	if clientNo == "" {
		clientNo = newClientOrderNo()
	}
	// 幂等：同 client_order_no 已存在则直接返回既有单。
	// 注意仓储未找到时返回 (nil, nil)——只判 err 会对首单空指针 panic（003 活体联调发现）。
	if existing, err := s.order.GetByClientOrderNo(ctx, clientNo); err == nil && existing != nil {
		return &MarketOrderResult{
			OrderNo: existing.OrderNo, Status: existing.Status,
			TotalCents:   existing.TotalAmountCents,
			PaymentGuide: existing.PaymentGuide, Duplicate: true,
			PlatformEnabled: true,
		}, nil
	}

	client := platform.NewModuleClient()
	res, err := client.CreateOrder(ctx, platform.CreateOrderInput{
		ClientOrderNo: clientNo,
		OrderType:     "module",
		ModuleKeys:    in.ModuleKeys,
		Options:       in.Options,
		ContactName:   in.ContactName,
		ContactPhone:  in.ContactPhone,
		ContactEmail:  in.ContactEmail,
		Remark:        in.Remark,
	})
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodePlatformUnavail, "提交订单失败（平台不可达）", err)
	}

	// ---- 联系人约束（仅线下路径强制）----
	// 平台返回 payment 段 = 在线支付路径 → 放行；没有 = 线下引导 → 必须留联系人。
	if res.Payment == nil && !hasContact {
		// 平台单已建但本地不留：缺联系方式的单运营联系不上买家，等于流程断在这里。
		// 让用户补联系方式后用同一 client_order_no 重试（幂等命中既有单），
		// 比留一张孤儿单更干净。
		return nil, ErrNoContactForPayment
	}

	items, _ := json.Marshal(res.Items)
	guide := res.PaymentGuide
	if strings.TrimSpace(guide) == "" {
		// 平台没给（老版本平台端）就自己拉一次；仍拿不到就用固定文案，
		// 绝不让"付款指引"为空 —— 用户不知道找谁付款就等于没下单
		if g, gerr := client.PaymentGuide(ctx); gerr == nil {
			guide = g
		} else {
			guide = "订单已提交，请联系平台运营完成付款；付款确认后权益自动开通。"
		}
	}
	local := &model.ModuleOrder{
		OrderNo: res.OrderNo, ClientOrderNo: clientNo,
		OrderType: "module", Status: res.Status,
		TotalAmountCents: res.TotalCents, Currency: "CNY",
		Items: string(items), PaymentGuide: guide,
	}
	if err := s.order.Upsert(ctx, local); err != nil {
		return nil, err
	}
	return &MarketOrderResult{
		OrderNo: res.OrderNo, Status: res.Status, TotalCents: res.TotalCents,
		Items: res.Items, PaymentGuide: guide, Payment: res.Payment,
		PlatformEnabled: true,
	}, nil
}

func newClientOrderNo() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("MO%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("MO%d%s", time.Now().UnixMilli(), strings.ToUpper(fmt.Sprintf("%x", buf))[:10])
}

// ListOrders 本地订单镜像列表（状态以平台为准，本地由同步刷新）
func (s *ModuleCatalogService) ListOrders(ctx context.Context, page, pageSize int) ([]*model.ModuleOrder, int64, error) {
	return s.order.List(ctx, page, pageSize)
}

// OrderDetail 订单详情：先查本地镜像，未命中则回源平台
func (s *ModuleCatalogService) OrderDetail(ctx context.Context, orderNo string) (*model.ModuleOrder, error) {
	local, err := s.order.GetByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	if local != nil {
		return local, nil
	}
	if !config.PlatformEnabled() {
		return nil, bizerr.New(bizerr.CodeNotFound, "订单不存在")
	}
	remote, err := platform.NewModuleClient().GetOrder(ctx, orderNo)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodePlatformUnavail, "查询订单失败", err)
	}
	return &model.ModuleOrder{
		OrderNo: orderNo,
		Status:  fmt.Sprintf("%v", remote["status"]),
	}, nil
}

var _ = repository.MenuKeysOf
