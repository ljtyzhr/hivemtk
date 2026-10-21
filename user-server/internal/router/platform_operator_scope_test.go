package router

import (
	"testing"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 平台运营方守卫的**档位归属**回归。
//
// 背景（2026-10-21 二次审核）：第一轮修跨租户越权时，把整个 `systemAdmin`
// 路由组换成了 `PlatformOperatorMiddleware`。方向对（跨租户越权确实堵住了），
// 但**过度收紧**——那一组里混着操作**租户表**的端点（system_config /
// system_config_kv / obs_config / tenant_config_params …），
// 租户 admin 本就该读写自己租户的这些数据，于是被一并打成 403：
//
//	GET /api/system/config            → 403（应为 200）
//	GET /api/agent/tool-integrations  → 403（应为 200）
//	GET /api/agent/settings           → 403（应为 200）
//	GET /api/obs/config               → 403（应为 200）
//	GET /api/manage/config-params     → 403（应为 200）
//	GET /api/admin/config             → 403（应为 200）
//
// 教训：**平台运营方守卫是"降敌意"方向**——把本该能访问的人挡在外面。
// 套在错误的档位上就会误伤租户内管理功能，而且这类回归**编译照过、
// 常规单测照绿**（守卫本身是对的），只有拿真实租户令牌打活体端点才看得见。
//
// 本文件把"哪些端点归平台运营方档、哪些归租户档"钉成两张表。
// 判据是**端点操作的数据落在哪张表**：
//   - 租户表（system_config / system_config_kv / obs_config / module_sync_logs
//     / tenant_config_params）⇒ 租户档（普通 admin 守卫）
//   - 全局表与实例运行态（tenants / backups / restore_records /
//     migration_records / 进程内路由注册表）⇒ 平台运营方档

// platformOperatorPaths 必须挂平台运营方守卫的端点。
var platformOperatorPaths = []string{
	// 租户运营：tenants 是全局表，看全部租户 / 停用别的租户 = 平台运营语义
	"/api/manage/tenants",
	"/api/manage/tenants/:id/status",
	"/api/manage/tenants/:id/retry-registration-sync",
	// 实例运行态与备份恢复：进程级，不属于任何租户
	"/api/system/restart",
	"/api/system/logs",
	"/api/system/stats",
	"/api/stats/system",
	"/api/system/backup",
	"/api/system/restore",
	// 备份恢复（权威路由）：backups / restore_records 是全局表，恢复会覆盖
	// 全实例数据 ⇒ 只有平台运营方可以触达
	"/api/backups",
	"/api/backups/:id",
	"/api/restore/list",
	"/api/restore/last",
	"/api/restore",
	// 备份恢复（前端兼容别名）：与上面同一批全局表，档位必须一致
	"/api/backup/list",
	"/api/backup/stats",
	"/api/backup/strategy",
	"/api/backup/create",
	"/api/backup/:id/preview",
	"/api/backup/:id/restore",
	"/api/backup/:id",
	"/api/backups/stats",
	// 进程内路由注册表巡检（全局）
	"/api/modules/uncovered-routes",
	// 业务字典写侧：dictionaries / dictionary_transitions 是全局只读共享数据，
	// 改词条是全局治理动作
	"/api/manage/dictionaries/:parent/items",
	"/api/manage/dictionaries/:parent/transitions",
}

// tenantScopedAdminPaths 必须留在租户档的端点（操作租户表，租户 admin 要能读写自己那份）。
var tenantScopedAdminPaths = []string{
	"/api/system/config",
	"/api/agent/tool-integrations",
	"/api/agent/settings",
	"/api/obs/config",
	"/api/admin/config",
	"/api/manage/config-params",
	"/api/modules/sync-logs",
	"/api/sso/config",
}

// routePathsOf 收集一个引擎里注册的全部路径（去掉方法，只看路径）。
func routePathsOf(engine *gin.Engine) map[string]bool {
	out := map[string]bool{}
	for _, ri := range engine.Routes() {
		out[ri.Path] = true
	}
	return out
}

// newScopedEngine 起一个引擎，把「租户档」与「平台运营方档」分别装进去，
// 返回两组的路径集合 —— 端点落在哪一组就是它实际挂的守卫。
func newScopedEngine(t *testing.T) (tenantPaths, systemPaths map[string]bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tenantEngine := gin.New()
	// 复刻 router.go 的组层级：auth := r.Group("/api")，两个 admin 档都是它的子组。
	tenantAuth := tenantEngine.Group("/api")
	tenantAdmin := tenantAuth.Group("")
	tenantAdmin.Use(middleware.AdminAuthMiddleware())

	systemEngine := gin.New()
	systemAuth := systemEngine.Group("/api")
	systemAdmin := systemAuth.Group("")
	systemAdmin.Use(middleware.PlatformOperatorMiddleware())

	// 同一批 setup 函数，分别喂两个引擎 —— 端点进哪个引擎就是挂哪个守卫。
	setupSystemRoutes(tenantAdmin, systemAdmin, nil)
	setupTenantRoutes(nil, tenantAuth, systemAdmin)
	setupModuleRoutes(nil, tenantAuth, tenantAdmin, systemAdmin)
	setupConfigParamRoutes(tenantAuth, tenantAdmin, nil)
	setupSSOConfigRoutes(tenantAdmin, controller.NewSSOController(service.NewSSOService(nil)))
	setupDictionaryRoutes(tenantAuth, systemAdmin, (*gorm.DB)(nil))
	// 备份/恢复只有平台运营方档：全局表 backups / restore_records
	setupBackupRoutes(systemAdmin, nil)
	setupBackupAliases(systemAdmin, nil)

	return routePathsOf(tenantEngine), routePathsOf(systemEngine)
}

// TestPlatformOperatorScopeEndpointOwnership 钉住每个端点挂在哪一档。
//
// 症状与判据：
//   - 租户级端点落进平台运营方档 ⇒ 租户 admin 被 403（第一轮的真实回归）
//   - 平台运营方端点落进租户档 ⇒ 跨租户越权重新敞开（第一轮修掉的那个洞）
func TestPlatformOperatorScopeEndpointOwnership(t *testing.T) {
	tenantPaths, systemPaths := newScopedEngine(t)

	for _, p := range tenantScopedAdminPaths {
		if !tenantPaths[p] {
			t.Errorf("租户级端点 %s 必须挂租户 admin 档（它操作的是租户表，租户 admin 要能读写自己那份）", p)
		}
		if systemPaths[p] {
			t.Errorf("租户级端点 %s 误挂在平台运营方档 ⇒ 租户 admin 会被 403", p)
		}
	}

	for _, p := range platformOperatorPaths {
		if !systemPaths[p] {
			t.Errorf("平台运营方端点 %s 必须挂平台运营方档（它操作的是全局表或实例运行态）", p)
		}
		if tenantPaths[p] {
			t.Errorf("平台运营方端点 %s 误挂在租户档 ⇒ 跨租户越权重新敞开", p)
		}
	}
}

// TestPlatformOperatorScopeNoDuplicateRegistration 钉住「一个端点只能挂一档」。
//
// 同一个路径同时注册在两档 ⇒ gin 会 panic（同一 group 同路径），但跨 group
// 不会 —— 于是端点变成"两个守卫都跑"，实际行为取决于注册顺序，
// 是一条谁也说不准的语义。这道门把它显式化。
func TestPlatformOperatorScopeNoDuplicateRegistration(t *testing.T) {
	tenantPaths, systemPaths := newScopedEngine(t)
	for p := range tenantPaths {
		if systemPaths[p] {
			t.Errorf("端点 %s 同时注册在两档（tenantPaths 与 systemPaths 都有）", p)
		}
	}
}

var _ = service.NewTenantService // 保持 service 包引用（本文件不直接用，留作将来扩展）
