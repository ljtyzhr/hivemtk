package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ModuleSnapshot 平台下发的模块快照（GET /merchant-api/modules/sync）
type ModuleSnapshot struct {
	MerchantKey string             `json:"merchant_key"`
	ActiveKeys  []string           `json:"active_keys"`
	MenuKeys    []string           `json:"menu_keys"`
	Catalog     []ModuleCatalogRow `json:"catalog"`
	SyncedAt    string             `json:"synced_at"`
}

// ModuleCatalogRow 目录里的一行
type ModuleCatalogRow struct {
	ModuleKey    string            `json:"module_key"`
	Name         string            `json:"name"`
	Category     string            `json:"category"`
	PriceCents   int64             `json:"price_cents"`
	BillingCycle string            `json:"billing_cycle"`
	Icon         string            `json:"icon"`
	Summary      string            `json:"summary"`
	Description  string            `json:"description"`
	SortOrder    int               `json:"sort_order"`
	DependsOn    []string          `json:"depends_on"`
	Options      []ModuleOptionRow `json:"options"`
	MenuKeys     []string          `json:"menu_keys"`
	Owned        bool              `json:"owned"`
	OwnedStatus  string            `json:"owned_status,omitempty"`
	// OwnedSource 开通来源（平台下发）。空值时用户端按模块分类回落，
	// 不猜成"用户购买" —— 猜错的后果见 service.sourceOf 的注释。
	OwnedSource string `json:"owned_source,omitempty"`
}

// ModuleOptionRow 子选项
type ModuleOptionRow struct {
	OptionKey       string `json:"option_key"`
	OptionName      string `json:"option_name"`
	PriceDeltaCents int64  `json:"price_delta_cents"`
	Enabled         bool   `json:"enabled"`
}

// ModuleMarketRow 商户市场里的一个模块（GET /merchant-api/module-market）
type ModuleMarketRow struct {
	Modules []ModuleCatalogRow `json:"modules"`
}

// ModuleOrderItem 订单行
type ModuleOrderItem struct {
	ItemType       string `json:"item_type"`
	ItemKey        string `json:"item_key"`
	ItemName       string `json:"item_name"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Quantity       int64  `json:"quantity"`
	AmountCents    int64  `json:"amount_cents"`
	Snapshot       string `json:"snapshot"`
}

// ModuleOrderResult 下单结果
type ModuleOrderResult struct {
	OrderNo      string            `json:"order_no"`
	Status       string            `json:"status"`
	TotalCents   int64             `json:"total_amount_cents"`
	Items        []ModuleOrderItem `json:"items"`
	PaymentGuide string            `json:"payment_guide"`
	Duplicate    bool              `json:"duplicate"`
}

// PlatformLLMWallet 平台 LLM 余额（商户视角）
type PlatformLLMWallet struct {
	MerchantKey     string `json:"merchant_key"`
	InputRemaining  int64  `json:"input_tokens_remaining"`
	OutputRemaining int64  `json:"output_tokens_remaining"`
	Low             bool   `json:"low"`
	Expired         bool   `json:"expired"`
	Exists          bool   `json:"exists"`
}

// PlatformLLMPlan 平台 LLM 套餐
type PlatformLLMPlan struct {
	PlanCode              string `json:"plan_code"`
	Name                  string `json:"name"`
	Summary               string `json:"summary"`
	InputTokens           int64  `json:"input_tokens"`
	OutputTokens          int64  `json:"output_tokens"`
	InputPricePer1kCents  int64  `json:"input_price_per_1k_cents"`
	OutputPricePer1kCents int64  `json:"output_price_per_1k_cents"`
	PriceCents            int64  `json:"price_cents"`
	ValidDays             int    `json:"valid_days"`
}

// PlatformLLMProvider 平台下发的 provider 元信息（**不含 API Key**）
type PlatformLLMProvider struct {
	ProviderName string `json:"provider_name"`
	DisplayName  string `json:"display_name"`
	Model        string `json:"model"`
	APIType      string `json:"api_type"`
	Vendor       string `json:"vendor"`
	PlanCode     string `json:"plan_code"`
	Source       string `json:"source"`
}

// TenantRegistrationPayload 开户登记上报
type TenantRegistrationPayload struct {
	MerchantKey      string `json:"merchant_key"`
	InstallID        string `json:"install_id"`
	TenantCode       string `json:"tenant_code"`
	CompanyName      string `json:"company_name"`
	LegalPerson      string `json:"legal_person"`
	Phone            string `json:"phone"`
	Email            string `json:"email"`
	LicenseNo        string `json:"license_no"`
	Address          string `json:"address"`
	RegMode          string `json:"reg_mode"`
	AgreementVersion string `json:"agreement_version"`
	RegulatoryAck    bool   `json:"regulatory_ack"`
}

// ModuleClient 功能模块 / 订单 / LLM 计费 的商户端客户端
type ModuleClient struct {
	client *Client
}

// NewModuleClient 构造模块客户端
func NewModuleClient() *ModuleClient {
	key := osGetenv("PLATFORM_MERCHANT_KEY")
	if key == "" {
		key = GetMerchantKey()
	}
	if key == "" {
		key = "local-dev-merchant"
	}
	return &ModuleClient{client: NewPlatformClient(key)}
}

// do 执行一次平台请求并解出 data
func (c *ModuleClient) do(method, path string, req any, out any) error {
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := c.client.Do(method, path, req, &env); err != nil {
		return err
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// SyncModules 拉取模块下发快照
func (c *ModuleClient) SyncModules(ctx context.Context) (*ModuleSnapshot, error) {
	var snap ModuleSnapshot
	if err := c.do("GET", "/merchant-api/modules/sync", nil, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// Market 拉取模块市场
func (c *ModuleClient) Market(ctx context.Context) (*ModuleMarketRow, error) {
	var m ModuleMarketRow
	if err := c.do("GET", "/merchant-api/module-market", nil, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// PaymentGuide 拉取付款指引
func (c *ModuleClient) PaymentGuide(ctx context.Context) (string, error) {
	var out struct {
		PaymentGuide string `json:"payment_guide"`
	}
	if err := c.do("GET", "/merchant-api/module-market/payment-guide", nil, &out); err != nil {
		return "", err
	}
	return out.PaymentGuide, nil
}

// CreateOrderInput 下单入参
type CreateOrderInput struct {
	ClientOrderNo string                     `json:"client_order_no"`
	OrderType     string                     `json:"order_type"`
	ModuleKeys    []string                   `json:"module_keys"`
	PlanCode      string                     `json:"plan_code"`
	Options       map[string]map[string]bool `json:"options"`
	ContactName   string                     `json:"contact_name"`
	ContactPhone  string                     `json:"contact_phone"`
	ContactEmail  string                     `json:"contact_email"`
	Remark        string                     `json:"remark"`
}

// CreateOrder 提交订单
func (c *ModuleClient) CreateOrder(ctx context.Context, in CreateOrderInput) (*ModuleOrderResult, error) {
	var out ModuleOrderResult
	if err := c.do("POST", "/merchant-api/orders", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOrders 商户订单列表
func (c *ModuleClient) ListOrders(ctx context.Context, status string, page, pageSize int) ([]map[string]any, int64, error) {
	q := url.Values{}
	if status != "" {
		q.Set("status", status)
	}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("page_size", fmt.Sprintf("%d", pageSize))
	var out struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	if err := c.do("GET", "/merchant-api/orders?"+q.Encode(), nil, &out); err != nil {
		return nil, 0, err
	}
	return out.List, out.Total, nil
}

// GetOrder 商户订单详情
func (c *ModuleClient) GetOrder(ctx context.Context, orderNo string) (map[string]any, error) {
	var out map[string]any
	if err := c.do("GET", "/merchant-api/orders/"+url.PathEscape(orderNo), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------- LLM 计费

// Wallet 拉取平台 LLM 余额
func (c *ModuleClient) Wallet(ctx context.Context) (*PlatformLLMWallet, error) {
	var w PlatformLLMWallet
	if err := c.do("GET", "/merchant-api/llm/wallet", nil, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// WalletTxns 拉取钱包流水
func (c *ModuleClient) WalletTxns(ctx context.Context, page, pageSize int) ([]map[string]any, int64, error) {
	q := url.Values{}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("page_size", fmt.Sprintf("%d", pageSize))
	var out struct {
		List  []map[string]any `json:"list"`
		Total int64            `json:"total"`
	}
	if err := c.do("GET", "/merchant-api/llm/wallet/txns?"+q.Encode(), nil, &out); err != nil {
		return nil, 0, err
	}
	return out.List, out.Total, nil
}

// Plans 拉取可见套餐
func (c *ModuleClient) Plans(ctx context.Context) ([]PlatformLLMPlan, error) {
	var out struct {
		List []PlatformLLMPlan `json:"list"`
	}
	if err := c.do("GET", "/merchant-api/llm/plans", nil, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

// Providers 拉取下发的平台 provider 元信息（无 Key）
func (c *ModuleClient) Providers(ctx context.Context) ([]PlatformLLMProvider, error) {
	var out struct {
		List []PlatformLLMProvider `json:"list"`
	}
	if err := c.do("GET", "/merchant-api/llm/providers", nil, &out); err != nil {
		return nil, err
	}
	return out.List, nil
}

// ---------------------------------------------------------- 开户登记

// RegulatoryNotice 拉取监管告知（未登录可取）
func (c *ModuleClient) RegulatoryNotice(ctx context.Context) (notice, agreementVersion string, required []string, err error) {
	var out struct {
		Notice           string   `json:"notice"`
		AgreementVersion string   `json:"agreement_version"`
		RequiredFields   []string `json:"required_fields"`
	}
	if err2 := c.do("GET", "/merchant-api/tenant/notice", nil, &out); err2 != nil {
		return "", "", nil, err2
	}
	return out.Notice, out.AgreementVersion, out.RequiredFields, nil
}

// RegisterTenant 提交开户登记
func (c *ModuleClient) RegisterTenant(ctx context.Context, p TenantRegistrationPayload) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do("POST", "/merchant-api/tenant/register", p, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// osGetenv 便于测试替换
var osGetenv = func(k string) string { return strings.TrimSpace(os.Getenv(k)) }
