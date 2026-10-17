package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	bizerr "hivemtk-user/internal/domain/errors"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
)

// ModuleMarketController 功能模块市场（用户端）
//
// 交互形态对标 assetMarket/Market.vue：分组筛选 + 卡片网格 + 一键下单。
// 与资产包的差别只有一处、但很关键：**这里没有"立即到账"**。
// 一期没有支付通道，用户提交订单后必须联系运营线下付款，
// 所以每个下单入口都要带出「付款指引」，而不是让人点了就以为买到了。
type ModuleMarketController struct {
	catalog *service.ModuleCatalogService
}

// NewModuleMarketController 构造功能模块市场控制器
func NewModuleMarketController() *ModuleMarketController {
	return &ModuleMarketController{catalog: service.NewModuleCatalogService()}
}

// svc 取服务实例
func (c *ModuleMarketController) svc() *service.ModuleCatalogService { return c.catalog }

func moduleMarketFail(ctx *gin.Context, err error) {
	var be *bizerr.BizError
	if errors.As(err, &be) {
		response.ErrorWithBusinessCode(ctx, be.Code, be.Message, gin.H{})
		return
	}
	switch {
	case errors.Is(err, service.ErrPlatformUnavailable):
		response.ErrorWithBusinessCode(ctx, bizerr.CodeModulePlatformUnavail,
			"平台未连接，无法办理模块开通。请在系统设置中配置平台地址后重试。", gin.H{})
	default:
		response.ErrorWithBusinessCode(ctx, bizerr.CodeInternal, err.Error(), gin.H{})
	}
}

// List GET /api/module-market/list
//
// 未连接平台时返回**空列表 + platform_enabled=false**，而不是报错：
// 市场页是用户最可能第一个点开的地方，让它弹一个红色错误只会让人以为系统坏了。
func (c *ModuleMarketController) List(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	list, enabled, err := c.svc().MarketList(ctx.Request.Context())
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > len(list) {
		end = len(list)
	}
	var pageItems []service.ModuleMarketItem
	if start < len(list) {
		pageItems = list[start:end]
	} else {
		pageItems = []service.ModuleMarketItem{}
	}
	response.Success(ctx, gin.H{
		"list":             pageItems,
		"total":            len(list),
		"platform_enabled": enabled,
	}, "ok")
}

// Detail GET /api/module-market/detail/:key
func (c *ModuleMarketController) Detail(ctx *gin.Context) {
	d, err := c.svc().MarketDetail(ctx.Request.Context(), ctx.Param("key"))
	if err != nil {
		if errors.Is(err, service.ErrModuleNotInCatalog) {
			response.ErrorWithBusinessCode(ctx, bizerr.CodeModuleNotFound, "模块不存在", gin.H{})
			return
		}
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, d, "ok")
}

// CreateOrder POST /api/module-market/order
func (c *ModuleMarketController) CreateOrder(ctx *gin.Context) {
	var in service.MarketOrderInput
	if err := ctx.ShouldBindJSON(&in); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	res, err := c.svc().CreateOrder(ctx.Request.Context(), in)
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, res, "订单已提交，请联系平台运营完成付款")
}

// Orders GET /api/module-market/orders
func (c *ModuleMarketController) Orders(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc().ListOrders(ctx.Request.Context(), page, pageSize)
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"list": list, "total": total}, "ok")
}

// OrderDetail GET /api/module-market/orders/:order_no
func (c *ModuleMarketController) OrderDetail(ctx *gin.Context) {
	o, err := c.svc().OrderDetail(ctx.Request.Context(), ctx.Param("order_no"))
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, o, "ok")
}

// MyModules GET /api/modules/my
func (c *ModuleMarketController) MyModules(ctx *gin.Context) {
	list, err := c.svc().MyModules(ctx.Request.Context())
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	owned := 0
	for _, m := range list {
		if m.Owned {
			owned++
		}
	}
	response.Success(ctx, gin.H{"list": list, "total": len(list), "owned": owned}, "ok")
}

// Sync POST /api/modules/sync
//
// 手动触发一次下发（设置页的「立即同步」按钮）。平台未连接时
// 不报错而是返回离线兜底结果 —— 离线部署本就常这样。
func (c *ModuleMarketController) Sync(ctx *gin.Context) {
	res, err := c.svc().Sync(ctx.Request.Context())
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, res, res.Message)
}

// SyncLogs GET /api/modules/sync-logs
func (c *ModuleMarketController) SyncLogs(ctx *gin.Context) {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))
	logs, err := c.svc().SyncLogs(ctx.Request.Context(), limit)
	if err != nil {
		moduleMarketFail(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"list": logs, "total": len(logs)}, "ok")
}

// UncoveredRoutes GET /api/modules/uncovered-routes
//
// 巡检面：列出"见过但未挂模块"的接口路径（middleware 三态里的 ③）。
// 它是"新增收费能力忘了补 module_route_patterns.go"的发现手段 —
// 补漏的兜底在 CI 的覆盖率测试里，但线上能立刻看到实际路径。
func (c *ModuleMarketController) UncoveredRoutes(ctx *gin.Context) {
	routes := middleware.UncoveredModuleRoutes()
	response.Success(ctx, gin.H{"list": routes, "total": len(routes)}, "ok")
}

var _ = logger.Infof
