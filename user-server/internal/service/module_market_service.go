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
}

// MarketList 市场列表。第二个返回值是平台是否连通。
func (s *ModuleCatalogService) MarketList(ctx context.Context) ([]ModuleMarketItem, bool, error) {
	owned, err := s.ownedMap(ctx)
	if err != nil {
		return nil, false, err
	}
	out := make([]ModuleMarketItem, 0, len(model.ModuleCatalogMeta))
	for _, meta := range model.ModuleCatalogMeta {
		menuKeys := model.ModuleMenuBindings[meta.Key]
		st, has := owned[meta.Key]
		item := ModuleMarketItem{
			ModuleKey:    meta.Key,
			Name:         meta.Name,
			Category:     meta.Category,
			PriceCents:   meta.PriceCents,
			BillingCycle: meta.BillingCycle,
			Icon:         meta.Icon,
			Summary:      meta.Summary,
			DependsOn:    meta.DependsOn,
			MenuCount:    len(menuKeys),
			Owned:        has && st == model.TenantModuleActive,
			OwnedStatus:  st,
			Free:         meta.Category == model.ModuleCategoryBase,
			FreeToEnable: meta.Category == model.ModuleCategoryPaid && meta.PriceCents == 0,
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
	owned, err := s.ownedMap(ctx)
	if err != nil {
		return nil, err
	}
	st, has := owned[moduleKey]
	menuKeys := model.ModuleMenuBindings[moduleKey]

	d := &ModuleMarketDetail{
		ModuleMarketItem: ModuleMarketItem{
			ModuleKey: moduleKey, Name: meta.Name, Category: meta.Category,
			PriceCents: meta.PriceCents, BillingCycle: meta.BillingCycle,
			Icon: meta.Icon, Summary: meta.Summary, DependsOn: meta.DependsOn,
			MenuCount:    len(menuKeys),
			Owned:        has && st == model.TenantModuleActive,
			OwnedStatus:  st,
			Free:         meta.Category == model.ModuleCategoryBase,
			FreeToEnable: meta.Category == model.ModuleCategoryPaid && meta.PriceCents == 0,
		},
		MenuKeys: menuKeys,
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

func (s *ModuleCatalogService) ownedMap(ctx context.Context) (map[string]string, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.ModuleKey] = r.Status
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
	// PlatformEnabled=false 时本地只留了一单，运营看到它再补同步
	PlatformEnabled bool `json:"platform_enabled"`
}

// CreateOrder 创建订单。
//
// 一期无支付通道，本方法**不做任何扣费、不做任何开通** —— 只建单 + 提示联系本人。
// 开通发生在平台端确认收款之后（服务端 MarkPaid → tenant_modules），
// 用户端只通过模块同步感知到变化。
func (s *ModuleCatalogService) CreateOrder(ctx context.Context, in MarketOrderInput) (*MarketOrderResult, error) {
	if !config.PlatformEnabled() {
		return nil, ErrPlatformUnavailable
	}
	if len(in.ModuleKeys) == 0 {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "请选择要开通的模块")
	}
	if strings.TrimSpace(in.ContactPhone) == "" && strings.TrimSpace(in.ContactEmail) == "" {
		// 需求原文：提交订单后买方联系本人。这条联系不上就等于流程断在这里。
		return nil, ErrNoContactForPayment
	}
	clientNo := strings.TrimSpace(in.ClientOrderNo)
	if clientNo == "" {
		clientNo = newClientOrderNo()
	}
	// 幂等：同 client_order_no 已存在则直接返回既有单
	if existing, err := s.order.GetByClientOrderNo(ctx, clientNo); err == nil {
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
		Items: res.Items, PaymentGuide: guide, PlatformEnabled: true,
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
