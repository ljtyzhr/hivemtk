package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupModuleRoutes 注册功能模块市场的路由
//
// 路由组分三档，每一档的语义都不同：
//
//	public —— 未登录可取。只有两类：市场列表（让人先看到有什么）与
//	          开户监管告知（注册页要在登录前展示）。其余一律要登录。
//	plainAuth —— 登录即可。见「我的模块」「我的订单」：读了也只是自己那点数据。
//	          **不挂模块闸的只有购买入口本身**（市场列表/详情/下单、订单查询、
//	          我的模块、手动同步）—— 它们必须对未订阅者可点，否则"关了就
//	          没法再买"。
//	          注意 wallet/plans（llm-cloud 的账）**是要过闸的**（见
//	          middleware/module_route_supplement.go 把 /api/llm/wallet|plans
//	          归到 llm-cloud）：未订阅时 4301 引导去市场，而下单动作走
//	          POST /api/llm/recharge —— 它不在闸表里，购买链路永远开着，
//	          不存在"关掉 llm-cloud 就看不到套餐 → 死锁"（上一版注释把
//	          wallet/plans 说成不可拦，与补充表相矛盾，以补充表为准：
//	          订阅者看到余额与套餐，未订阅者看前端空态页）。
//	admin —— 管理面（同步日志、巡检面）。
//
// 模块闸在 auth 组上统一挂 GuardModulePath（见 router.go），本文件只做映射，
// 不在这里重复挂 RequireModule：路径→模块的判定必须集中在
// middleware/module_route_patterns.go 一张表里，否则必然漂移。
// setupModuleRoutes 注册模块市场路由。
//
// admin 档按**数据域**分两档（判据是"落到哪张表"）：
//   - tenantAdmin：sync-logs 落 module_sync_logs（**租户表**，有 tenant_id 列）
//     ⇒ 租户 admin 读自己租户的同步日志即可，隔离 callback 已限域。
//   - systemAdmin：uncovered-routes 是**进程内路由巡检**（middleware 的全局注册表，
//     与租户无关）⇒ 平台运营方档。
func setupModuleRoutes(public, auth, tenantAdmin, systemAdmin *gin.RouterGroup) {
	ctrl := controller.NewModuleMarketController()

	// 各档 nil 防护（Setup 早期阶段仅 public 就绪；鉴权档由 auth 组就绪后二次挂载）
	if public != nil {
		// public：市场列表（未登录也要能逛）与监管告知
		public.GET("/module-market/list", ctrl.List)
		public.GET("/module-market/payment-guide", ctrl.PaymentGuide)
		public.GET("/tenant/regulatory-notice", ctrl.RegulatoryNotice)
	}

	if auth != nil {
		// plainAuth：下单、查单、支付状态直查、我的模块、手动同步
		auth.GET("/module-market/detail/:key", ctrl.Detail)
		auth.POST("/module-market/order", ctrl.CreateOrder)
		auth.GET("/module-market/orders", ctrl.Orders)
		auth.GET("/module-market/orders/:order_no", ctrl.OrderDetail)
		// 支付状态直查（003 T018）：比 10 分钟 sync 快，收银台轮询用
		auth.GET("/module-market/orders/:order_no/pay-status", ctrl.PayStatus)
		// 「去支付」（003 收银台补口）：关掉收银台标签页后，订单页要能重新取
		// 支付参数开收银台，否则该单只剩付款指引一条线
		auth.POST("/module-market/orders/:order_no/pay", ctrl.PayOrder)
		auth.GET("/modules/my", ctrl.MyModules)
		auth.POST("/modules/sync", ctrl.Sync)
		auth.GET("/llm/wallet", ctrl.LLMWallet)
		auth.GET("/llm/plans", ctrl.LLMPlans)
		auth.GET("/llm/wallet/txns", ctrl.LLMWalletTxns)
		auth.POST("/llm/recharge", ctrl.CreateLLMOrder)

		// 订阅状态与续费（003 T018/T022）。两条都在订阅闸白名单内
		// （/api/subscription/*），墙页依赖它们 —— 自身不得 4305（oracle P1-12）。
		subCtrl := controller.NewSubscriptionController()
		auth.GET("/subscription/status", subCtrl.Status)
		auth.POST("/subscription/renew", subCtrl.Renew)
	}

	if tenantAdmin != nil {
		// 同步日志：落租户表 module_sync_logs ⇒ 租户档
		tenantAdmin.GET("/modules/sync-logs", ctrl.SyncLogs)
	}
	if systemAdmin != nil {
		// 未挂模块的巡检面：进程内路由注册表（全局）⇒ 平台运营方档
		systemAdmin.GET("/modules/uncovered-routes", ctrl.UncoveredRoutes)
	}
}
