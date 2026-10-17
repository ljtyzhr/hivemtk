package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"hivemtk-user/internal/config"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"

	"github.com/gin-gonic/gin"
)

// LLM 钱包（用户端视图）
//
// 一期只做「查询 + 展示 + 下单充值」，**不做本地扣减**：
// 扣减的权威在平台侧（llm_proxy_service 转发时实时扣），本地再扣一遍就是
// 两本账，对不齐。用户在本地看到的余额是平台下发的镜像，仅用于展示与
// 「余额不足」的快速失败提示。
type LLMWalletService struct {
	repo repository.LLMWalletRepository
}

// NewLLMWalletService 构造
func NewLLMWalletService() *LLMWalletService {
	return &LLMWalletService{repo: repository.NewLLMWalletRepository()}
}

// NewLLMWalletServiceWithRepo 带显式仓储（测试注入）
func NewLLMWalletServiceWithRepo(repo repository.LLMWalletRepository) *LLMWalletService {
	return &LLMWalletService{repo: repo}
}

// WalletView 钱包视图
type WalletView struct {
	MerchantKey     string  `json:"merchant_key"`
	InputRemaining  int64   `json:"input_tokens_remaining"`
	OutputRemaining int64   `json:"output_tokens_remaining"`
	Low             bool    `json:"low"`
	Expired         bool    `json:"expired"`
	Exists          bool    `json:"exists"`
	PlatformEnabled bool    `json:"platform_enabled"`
	ExpiresAt       *string `json:"expires_at,omitempty"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
	// LocalRemainder 本地已知的剩余 token（预扣用；一期恒 0，因为不做本地扣减）
	LocalRemainder int64 `json:"local_remainder"`
}

// Wallet 查询平台余额并落本地镜像
func (s *LLMWalletService) Wallet(ctx context.Context) (*WalletView, error) {
	if !config.PlatformEnabled() {
		// 离线：读本地镜像，没有就返回空钱包（exists=false），而不是报错
		if _, total, err := s.repo.ListTxns(ctx, "recharge", 1, 1); err == nil && total > 0 {
			return &WalletView{PlatformEnabled: false, Exists: true}, nil
		}
		return &WalletView{PlatformEnabled: false}, nil
	}
	pw, err := platform.NewModuleClient().Wallet(ctx)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "查询 LLM 余额失败（平台不可达）", err)
	}
	if pw.Exists {
		_ = s.repo.ApplyMirror(ctx, pw.InputRemaining, pw.OutputRemaining)
	}
	v := &WalletView{
		MerchantKey:     pw.MerchantKey,
		InputRemaining:  pw.InputRemaining,
		OutputRemaining: pw.OutputRemaining,
		Low:             pw.Low,
		Expired:         pw.Expired,
		Exists:          pw.Exists,
		PlatformEnabled: true,
	}
	return v, nil
}

// PlanView 套餐视图
type PlanView struct {
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

// Plans 可见套餐
func (s *LLMWalletService) Plans(ctx context.Context) ([]PlanView, error) {
	if !config.PlatformEnabled() {
		return []PlanView{}, nil
	}
	rows, err := platform.NewModuleClient().Plans(ctx)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "查询 LLM 套餐失败", err)
	}
	out := make([]PlanView, 0, len(rows))
	for _, r := range rows {
		out = append(out, PlanView(r))
	}
	return out, nil
}

// TxnView 流水视图
type TxnView struct {
	CreatedAt          string `json:"created_at"`
	Reason             string `json:"reason"`
	DeltaInput         int64  `json:"delta_input"`
	DeltaOutput        int64  `json:"delta_output"`
	BalanceInputAfter  int64  `json:"balance_input_after"`
	BalanceOutputAfter int64  `json:"balance_output_after"`
	Note               string `json:"note"`
}

// Txns 钱包流水
func (s *LLMWalletService) Txns(ctx context.Context, page, pageSize int) ([]TxnView, int64, error) {
	if !config.PlatformEnabled() {
		return []TxnView{}, 0, nil
	}
	rows, total, err := platform.NewModuleClient().WalletTxns(ctx, page, pageSize)
	if err != nil {
		return nil, 0, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "查询 LLM 流水失败", err)
	}
	out := make([]TxnView, 0, len(rows))
	for _, r := range rows {
		v := TxnView{
			Reason:             strOf(r["reason"]),
			Note:               strOf(r["note"]),
			DeltaInput:         int64Of(r["delta_input"]),
			DeltaOutput:        int64Of(r["delta_output"]),
			BalanceInputAfter:  int64Of(r["balance_input_after"]),
			BalanceOutputAfter: int64Of(r["balance_output_after"]),
			CreatedAt:          strOf(r["created_at"]),
		}
		out = append(out, v)
	}
	return out, total, nil
}

// LLMOrderInput LLM 套餐下单入参
type LLMOrderInput struct {
	PlanCode      string `json:"plan_code"`
	ClientOrderNo string `json:"client_order_no"`
	ContactPhone  string `json:"contact_phone"`
	ContactEmail  string `json:"contact_email"`
	Remark        string `json:"remark"`
}

// LLMOrderResult 下单结果
type LLMOrderResult struct {
	OrderNo      string `json:"order_no"`
	Status       string `json:"status"`
	TotalCents   int64  `json:"total_amount_cents"`
	PaymentGuide string `json:"payment_guide"`
	Duplicate    bool   `json:"duplicate"`
}

// CreateOrder 购买 LLM 套餐
//
// 与功能模块订单同一条链路（平台端 order_type=llm → MarkPaid → 钱包入账），
// 区别只是订单行类型不同。**这里不做任何扣费**。
func (s *LLMWalletService) CreateOrder(ctx context.Context, in LLMOrderInput) (*LLMOrderResult, error) {
	if !config.PlatformEnabled() {
		return nil, ErrPlatformUnavailable
	}
	if strings.TrimSpace(in.PlanCode) == "" {
		return nil, bizerr.New(bizerr.CodeParamInvalid, "请选择套餐")
	}
	if strings.TrimSpace(in.ContactPhone) == "" && strings.TrimSpace(in.ContactEmail) == "" {
		return nil, ErrNoContactForPayment
	}
	clientNo := strings.TrimSpace(in.ClientOrderNo)
	if clientNo == "" {
		clientNo = newClientOrderNo()
	}
	res, err := platform.NewModuleClient().CreateOrder(ctx, platform.CreateOrderInput{
		ClientOrderNo: clientNo,
		OrderType:     "llm",
		PlanCode:      in.PlanCode,
		ContactPhone:  in.ContactPhone,
		ContactEmail:  in.ContactEmail,
		Remark:        in.Remark,
	})
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "提交订单失败（平台不可达）", err)
	}
	guide := res.PaymentGuide
	if strings.TrimSpace(guide) == "" {
		if g, gerr := platform.NewModuleClient().PaymentGuide(ctx); gerr == nil {
			guide = g
		} else {
			guide = "订单已提交，请联系平台运营完成付款；付款确认后余额自动入账。"
		}
	}
	return &LLMOrderResult{
		OrderNo: res.OrderNo, Status: res.Status,
		TotalCents: res.TotalCents, PaymentGuide: guide,
	}, nil
}

// IsPlatformLLMProviderAvailable 平台是否下发了可用的 provider（用于前端提示）
func (s *LLMWalletService) IsPlatformLLMProviderAvailable(ctx context.Context) bool {
	if !config.PlatformEnabled() {
		return false
	}
	rows, err := platform.NewModuleClient().Providers(ctx)
	return err == nil && len(rows) > 0
}

// PlatformProviders 平台下发的 provider 元信息（无 Key）
func (s *LLMWalletService) PlatformProviders(ctx context.Context) ([]platform.PlatformLLMProvider, error) {
	if !config.PlatformEnabled() {
		return nil, ErrPlatformUnavailable
	}
	rows, err := platform.NewModuleClient().Providers(ctx)
	if err != nil {
		return nil, bizerr.Wrap(bizerr.CodeModulePlatformUnavail, "拉取平台模型失败", err)
	}
	return rows, nil
}

var (
	_ = errors.Is
	_ = model.TenantModuleActive
	_ = gin.Context{}
	_ = json.Marshal
)

func strOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func int64Of(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}
