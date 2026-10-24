package middleware

import (
	"net/http"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/system/install"

	"github.com/gin-gonic/gin"
)

// InitGuard 系统初始化守卫（开源版）
//
// 行为：
//   - 系统 NOT_INSTALLED：返回 INIT_REQUIRED，前端跳 /setup
//   - 系统 INITIALIZED：放行
//   - 白名单路由（init-*/login/health 等）直接放行
//
// 本守卫只看一件事：install.lock 里 initialized 是否为真。
func InitGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.FullPath()
		if isInitWhitelist(path) {
			c.Next()
			return
		}
		checker := GetInstallStatus()
		if checker == nil {
			c.Next()
			return
		}
		status := checker.GetInitStatus()
		if status == nil {
			c.Next()
			return
		}

		// SaaS（tenant_mode=on）实例没有"单机未安装"概念：租户走 /register 自助开户，
		// install.lock 只服务离线单机 setup 向导。口径与 controller/system_init.go
		// GetInitStatus 一致——on 模式恒已初始化，否则 docker 直部的 SaaS 实例
		// （从未跑过 setup）所有非白名单接口会被打回 INIT_REQUIRED，前端全部推进 /setup。
		if tenant.IsMultiTenant() {
			c.Next()
			return
		}

		if !status.Initialized {
			logger.Info("InitGuard: 未初始化 path=" + path)
			c.AbortWithStatusJSON(200, gin.H{
				"code":     "INIT_REQUIRED",
				"message":  "系统未初始化",
				"redirect": "/setup",
				"data":     status,
			})
			return
		}

		c.Next()
	}
}

// SetupOnlyGuard 只在"尚未初始化"期间放行的守卫，挂在两条公开的安装引导路由上。
//
// 与 InitGuard 是一对反向的门：
//   - InitGuard 拦下"还没装完就访问业务接口"
//   - SetupOnlyGuard 拦下"装完之后还能匿名铸第二个超管"
//
// InitGuard 结构上拦不住这两条路由——它们在 isInitWhitelist 里（首次安装必须能被匿名访问），
// 所以"已初始化"这一侧的判定必须由这道门承担。它只读 install.lock（不回查库，
// 高频公开路径上不能带 DB 依赖）；库里是否已有超管由 service.AuthService.InitAdmin
// 收尾核对，两道门叠在一起，缺任一侧都不会留下匿名建号窗口。
func SetupOnlyGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 与 InitGuard 同口径：on 模式恒"已初始化"（多租户没有单机 setup 场景），
		// 这里也必须直接拒绝 init-admin，否则 SaaS 实例上这条白名单路由
		// 会绕过"装完即封"变成匿名铸超管的窗口。
		if tenant.IsMultiTenant() || install.GetStatus().Initialized {
			logger.Info("SetupOnlyGuard: 系统已初始化，拒绝重复创建超管 path=" + c.FullPath())
			response.Error(c, http.StatusForbidden, response.ErrSystemAlreadyInitialized)
			c.Abort()
			return
		}
		c.Next()
	}
}

func isInitWhitelist(path string) bool {
	whitelist := map[string]bool{
		"/api/system/init-status":          true,
		"/api/system/info":                 true,
		"/api/system/init-admin":           true,
		"/api/system/init-complete":        true,
		"/api/auth/login":                  true,
		"/api/auth/refresh-token":          true,
		"/api/auth/change-password":        true,
		"/api/auth/current-user":           true,
		"/api/system/create-default-admin": true,
		"/s/:code":                         true,
		"/l/:code":                         true,
		"/health":                          true,
		"/healthz":                         true,
		"/readyz":                          true,
	}
	return whitelist[path]
}
