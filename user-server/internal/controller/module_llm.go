package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/service"
)

// 本文件补齐「功能模块市场」在 user-web 里被直接调用、但不经过闸的那几条：
// 付款指引、监管告知、我的 LLM 余额/套餐/充值下单。
//
// 它们与 module_market.go 分文件的理由：那一支是"市场浏览与开通"，
// 这一支是"平台代付能力的账"。LLM 计量是收费模块（llm-cloud），
// 而查余额与买套餐恰恰**不能**被它自己的闸拦住 —— 见 module_routes.go 的说明。

// PaymentGuide GET /api/module-market/payment-guide
//
// 文案来自平台端配置（site_contact_config），运营换联系方式后用户端自动跟着变。
// 平台不可达时给固定兜底文案 —— 这里绝不能返回空串：
// "订单已提交"后面没有联系方式，用户就不知道找谁，流程到此断掉。
func (c *ModuleMarketController) PaymentGuide(ctx *gin.Context) {
	svc := service.NewModuleCatalogService()
	guide, err := svc.PaymentGuide(ctx.Request.Context())
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"payment_guide": guide}, "ok")
}

// RegulatoryNotice GET /api/tenant/regulatory-notice
//
// 开户注册的监管告知与协议版本。未登录可取：注册页在用户还没有任何身份时
// 就要展示这段文本并要求勾选，文案与版本号由平台端统一下发，避免两端漂移。
func (c *ModuleMarketController) RegulatoryNotice(ctx *gin.Context) {
	svc := service.NewModuleCatalogService()
	n := svc.RegulatoryNotice(ctx.Request.Context())
	response.Success(ctx, gin.H{
		"notice":            n.Notice,
		"agreement_version": n.AgreementVersion,
		"required_fields":   n.RequiredFields,
	}, "ok")
}

// LLMWallet GET /api/llm/wallet
func (c *ModuleMarketController) LLMWallet(ctx *gin.Context) {
	w, err := service.NewLLMWalletService().Wallet(ctx.Request.Context())
	if err != nil {
		response.ErrorWithBusinessCode(ctx, bizerr.CodeModulePlatformUnavail,
			"平台未连接，无法查询 LLM 余额", gin.H{})
		return
	}
	response.Success(ctx, w, "ok")
}

// LLMWalletTxns GET /api/llm/wallet/txns
func (c *ModuleMarketController) LLMWalletTxns(ctx *gin.Context) {
	list, total, err := service.NewLLMWalletService().Txns(ctx.Request.Context(),
		pageOf(ctx), pageSizeOf(ctx))
	if err != nil {
		response.ErrorWithBusinessCode(ctx, bizerr.CodeModulePlatformUnavail,
			"平台未连接，无法查询 LLM 流水", gin.H{})
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": total}, "ok")
}

// LLMPlans GET /api/llm/plans
func (c *ModuleMarketController) LLMPlans(ctx *gin.Context) {
	list, err := service.NewLLMWalletService().Plans(ctx.Request.Context())
	if err != nil {
		response.ErrorWithBusinessCode(ctx, bizerr.CodeModulePlatformUnavail,
			"平台未连接，无法查看 LLM 套餐", gin.H{})
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// CreateLLMOrder POST /api/llm/recharge
//
// 购买平台 LLM 套餐（预付费 token 包）。与功能模块订单走**同一个** MarkPaid
// 入账路径（平台端 order_type=llm），只是订单行类型不同。
func (c *ModuleMarketController) CreateLLMOrder(ctx *gin.Context) {
	var in service.LLMOrderInput
	if err := ctx.ShouldBindJSON(&in); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	res, err := service.NewLLMWalletService().CreateOrder(ctx.Request.Context(), in)
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, res, "订单已提交，请联系平台运营完成付款；付款确认后余额自动入账")
}

var _ = platform.ModuleSnapshot{}
