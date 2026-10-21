package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hivemtk-user/internal/config"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"
)

// 功能模块市场的服务层错误
var (
	ErrPlatformUnavailable = bizerr.New(bizerr.CodeModulePlatformUnavail, "平台未连接，无法办理模块开通")
	ErrModuleNotInCatalog  = errors.New("模块不在目录中")
	ErrModuleAlreadyOwned  = errors.New("模块已开通")
	ErrModuleOrderNoClient = errors.New("缺少订单幂等键")
	ErrNoContactForPayment = errors.New("请填写联系方式（平台运营需要据此联系你完成线下付款）")
	// ErrOrderNotPayable 这张订单当前不需要、也不允许再发起在线支付
	// （已支付 / 已关闭 / 0 元直通）。单独 sentinel 的理由：处置动作是
	// 「刷新订单页看状态」，不是「改参数重试」—— 报成参数错误会让人反复重试。
	ErrOrderNotPayable = bizerr.New(bizerr.CodeParamInvalid, "该订单当前无需在线支付")
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
	// 展示价以平台 module_catalog 现价为准：改价是平台端运营动作，
	// 内嵌元数据只是平台不可达时的兜底。不覆盖的后果是市场页显示旧价、
	// 下单却按平台新价成交（2026-10-21 线上全价改 1 元时页面还挂 99 元）。
	livePrices := s.liveMarketPrices(ctx)
	out := make([]ModuleMarketItem, 0, len(model.ModuleCatalogMeta))
	for _, meta := range model.ModuleCatalogMeta {
		menuKeys := model.ModuleMenuBindings[meta.Key]
		row := owned[meta.Key]
		price := marketPriceOf(livePriceOf(meta.PriceCents, livePrices, meta.Key), row)
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
	price := marketPriceOf(livePriceOf(meta.PriceCents, s.liveMarketPrices(ctx), moduleKey), row)
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

// liveMarketPrices 拉平台 module_catalog 现价表；平台不可达返回 nil（回落内嵌价）。
func (s *ModuleCatalogService) liveMarketPrices(ctx context.Context) map[string]int64 {
	snap, err := platform.NewModuleClient().Market(ctx, currentTenantCode(ctx))
	if err != nil || snap == nil {
		return nil
	}
	prices := make(map[string]int64, len(snap.Modules))
	for _, r := range snap.Modules {
		prices[r.ModuleKey] = r.PriceCents
	}
	return prices
}

// livePriceOf 平台有该模块现价就用平台价，否则用内嵌兜底价。
func livePriceOf(metaPrice int64, live map[string]int64, key string) int64 {
	if p, ok := live[key]; ok {
		return p
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
		// 平台端订单按 (merchant_key, tenant_code) 归属：续费单与 LLM 充值单都带，
		// 模块单此前漏了，落库后 tenant_code 是空串——平台侧看不到是谁买的，
		// 本地 module_orders 与平台 orders 也对不上账（测试数据因此无法按租户清理）。
		// 取不到租户归属（off 模式无租户行 / ctx 无绑定）时传空，平台按旧行为兜底。
		TenantCode:   currentTenantCode(ctx),
		ModuleKeys:   in.ModuleKeys,
		Options:      in.Options,
		ContactName:  in.ContactName,
		ContactPhone: in.ContactPhone,
		ContactEmail: in.ContactEmail,
		Remark:       in.Remark,
		// 买家终端 IP：平台侧 mapi 下单把它当 clientip 必填项（网关拒环回地址）。
		// 没有它，平台只能拿"用户端服务器的 IP"去下单。
		ClientIP: platform.ClientIPFromContext(ctx),
	})
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodePlatformUnavail, "提交订单失败（平台不可达）", err)
	}

	// ---- 联系人约束（仅线下待支付路径强制）----
	// 平台返回 payment 段 = 在线支付路径 → 放行；0 元单平台直通 fulfilled
	// 也没有"等付款"环节 → 同样放行；只有"无 payment 且仍待支付"才是线下引导。
	if res.Payment == nil && !hasContact && !orderSettled(res.Status) {
		// 平台单已建但本地不留：缺联系方式的单运营联系不上买家，等于流程断在这里。
		// 让用户补联系方式后用同一 client_order_no 重试（幂等命中既有单），
		// 比留一张孤儿单更干净。
		return nil, ErrNoContactForPayment
	}

	items, _ := json.Marshal(res.Items)
	guide := res.PaymentGuide
	if orderSettled(res.Status) {
		// 已落权益的单（0 元直通 fulfilled 等）不挂付款指引 —— 平台的
		// payment_guide 是"联系运营完成付款"通用文案，订单页再展示它
		// 等于让已开通的人去付第二次钱。
		guide = ""
	} else if strings.TrimSpace(guide) == "" {
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

// PayOrder 为**已下的**待支付订单重新申请在线支付参数（订单页「去支付」）。
//
// 下单只在 CreateOrder 那一刻挂 payment 段；用户关掉收银台标签页后，订单页只剩
// 付款指引（联系运营），一条本可在线完成的订单被退回线下。本方法把同一张订单
// 再交给平台现算一次参数 —— 平台按 out_trade_no 认单，不会产生第二张订单。
//
// 本地镜像只用来挡"这单根本不需要再付"（已支付 / 0 元直通）；镜像缺失（老订单、
// 或某类订单没落镜像）不阻断，交给平台按订单状态与通道配置做权威判定。
func (s *ModuleCatalogService) PayOrder(ctx context.Context, orderNo string) (*MarketOrderResult, error) {
	if !config.PlatformEnabled() {
		return nil, ErrPlatformUnavailable
	}
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "订单号不能为空")
	}
	if local, err := s.order.GetByOrderNo(ctx, orderNo); err == nil && local != nil {
		if local.Status != model.ModuleOrderPendingPayment || local.TotalAmountCents <= 0 {
			return nil, ErrOrderNotPayable
		}
	}
	res, err := platform.NewModuleClient().CreateOrderPayment(ctx, orderNo)
	if err != nil {
		// 平台把"这单为什么付不了"放在 msg 里（非待支付 / 0 元 / 通道未配置 /
		// 无权操作）。原样透出才看得懂：包成「平台不可达」会把一个状态问题
		// 说成网络问题，让人反复重试同一张已经付过的单。
		var pe *platform.PlatformError
		if errors.As(err, &pe) {
			code := bizerr.CodeModulePlatformUnavail
			switch pe.StatusCode {
			case http.StatusBadRequest:
				code = bizerr.CodeParamInvalid
			case http.StatusUnauthorized:
				code = bizerr.CodeUnauthorized
			case http.StatusForbidden:
				code = bizerr.CodeForbidden
			case http.StatusNotFound:
				code = bizerr.CodeNotFound
			}
			msg := pe.Msg()
			if strings.TrimSpace(msg) == "" {
				msg = "获取支付参数失败，请稍后重试或联系平台运营"
			}
			return nil, bizerr.New(code, msg)
		}
		return nil, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "获取支付参数失败（平台不可达）", err)
	}
	guide := res.PaymentGuide
	if strings.TrimSpace(guide) == "" {
		if g, gerr := platform.NewModuleClient().PaymentGuide(ctx); gerr == nil {
			guide = g
		} else {
			guide = "订单已提交，请联系平台运营完成付款；付款确认后权益自动开通。"
		}
	}
	return &MarketOrderResult{
		OrderNo: res.OrderNo, Status: res.Status, TotalCents: res.TotalCents,
		Items: res.Items, PaymentGuide: guide, Payment: res.Payment,
		Duplicate: res.Duplicate, PlatformEnabled: true,
	}, nil
}

func newClientOrderNo() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("MO%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("MO%d%s", time.Now().UnixMilli(), strings.ToUpper(fmt.Sprintf("%x", buf))[:10])
}

// orderSettled 判平台订单是否已无"等付款"环节：0 元单平台直通 fulfilled
// （修订 #9），回调入账后也会推到 paid/fulfilled。这类单不能按线下路径
// 拦联系方式、也不能挂"联系运营完成付款"的付款指引。
func orderSettled(status string) bool {
	switch strings.TrimSpace(status) {
	case "paid", "fulfilled":
		return true
	}
	return false
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
