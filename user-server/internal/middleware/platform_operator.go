package middleware

import (
	"context"
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"

	"github.com/gin-gonic/gin"
)

// PlatformOperatorMiddleware 平台运营方守卫：role 判定**之上**再加一道
// 「你是不是平台运营方」判定。
//
// 为什么需要它（不是"admin 判定"就够了）：
//
//	SaaS 模式下 "admin" 这个角色承载了两种语义——平台运营方（跨租户）与
//	租户管理员（租户内）。而 AdminAuthMiddleware 只判 role == "admin"，
//	两者都过 ⇒ 租户 admin 拿到了平台运营方的权限：
//	  - GET  /api/manage/tenants            ⇒ 看到全部租户（公司名/负责人/电话/邮箱/地址）
//	  - PATCH /api/manage/tenants/:id/status ⇒ 停用其他租户
//	  - POST /api/system/restart             ⇒ 重启整个实例
//
//	路由注释本来就写着这些端点"必须硬超管守卫"——守卫是漏的，不是设计如此。
//
// 判据：**调用者所属租户是本实例的首个租户**（tenants 表 ID 最小）。
//   - 首个租户是平台自己开的那个（setup 向导或最早的自助开户）；
//     它的管理员就是平台运营方。
//   - 租户只增不删（停用是软删，不改 ID）⇒ ID 最小者恒定 ⇒ 判据稳定。
//   - 与"code 前缀"无关：前缀是生成策略的副产品，改生成规则就漂移。
//
// off 模式（离线单机）**恒真**：单机形态只有一个租户，平台运营方 = 本机超管，
// 行为与历史版本逐字节一致（这是本仓双形态的硬要求）。
//
// 挂载约束：必须挂在 JWTAuthMiddleware 与 TenantMiddleware **之后**
// （前者提供 role，后者把租户写进 request context）。
func PlatformOperatorMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 角色不达标先拒：平台运营方本身也是 admin 角色，role 判定是前置条件。
		role, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "未找到用户角色信息",
			})
			return
		}
		roleStr, ok := role.(string)
		if !ok || roleStr != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "权限不足，需要管理员权限",
			})
			return
		}

		// 离线单机：没有跨租户这回事，全部放行。
		if !tenant.IsMultiTenant() {
			c.Next()
			return
		}

		tenantID, bound := tenant.FromContext(c.Request.Context())
		if !bound || tenantID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    401,
				"message": "缺少租户归属，请重新登录",
			})
			return
		}

		ok2, err := isPlatformOperatorTenant(c.Request.Context(), tenantID)
		if err != nil {
			// 判定不了就拒：这是权限边界，"查不到"不等于"放行"。
			logger.Errorf("[platform-operator] 平台运营方判定失败，拒绝访问 tenant_id=%d path=%s: %v",
				tenantID, c.FullPath(), err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "权限判定失败，请稍后重试",
			})
			return
		}
		if !ok2 {
			logger.Warnf("[platform-operator] 非平台运营方访问平台级端点，已拒绝 tenant_id=%d role=%s path=%s",
				tenantID, roleStr, c.FullPath())
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "权限不足，该操作仅平台运营方可执行",
			})
			return
		}
		c.Next()
	}
}

// platformOperatorCacheTTL 首租户判定的缓存时长。
//
// 缓存的是「本实例的首租户 ID」——它**恒定不变**（租户只增不删），
// 所以 TTL 只是兜底（万一将来允许删租户），不是新鲜度要求。
const platformOperatorCacheTTL = 5 * time.Minute

var platformOperatorCache struct {
	mu       sync.RWMutex
	tenantID uint
	loaded   bool
	at       time.Time
}

// isPlatformOperatorTenant 判定给定租户是不是本实例的平台运营方租户（首个租户）。
//
// 单测可注入判定函数（SetPlatformOperatorResolverForTest），
// 避免每个用例都要真的连库。
var platformOperatorResolver = defaultPlatformOperatorResolver

// SetPlatformOperatorResolverForTest 注入首租户判定器（仅测试用）。
func SetPlatformOperatorResolverForTest(fn func(ctx context.Context) (uint, error)) {
	if fn != nil {
		platformOperatorResolver = fn
	}
}

// ResetPlatformOperatorResolver 恢复默认判定器（仅测试用）。
func ResetPlatformOperatorResolver() { platformOperatorResolver = defaultPlatformOperatorResolver }

// defaultPlatformOperatorResolver 默认判定器：查 tenants 表 ID 最小的行。
//
// 走 repository 而非全局 db：middleware 直接摸 db 违反架构门（service 不得
// 全局摸 db.GetDB，middleware 同理）。tenants 是全局表，不受隔离 callback 影响。
func defaultPlatformOperatorResolver(ctx context.Context) (uint, error) {
	repo := repository.NewTenantRepository()
	if repo == nil {
		return 0, ErrPlatformOperatorUnresolved
	}
	t, err := repo.GetFirstTenant(ctx)
	if err != nil {
		return 0, err
	}
	if t == nil {
		return 0, ErrPlatformOperatorUnresolved
	}
	return t.ID, nil
}

// ErrPlatformOperatorUnresolved 首租户判定不出结果（库没行 / DB 不可用）。
var ErrPlatformOperatorUnresolved = errPlatformOperatorUnresolved{}

type errPlatformOperatorUnresolved struct{}

func (errPlatformOperatorUnresolved) Error() string {
	return "middleware: 平台运营方判定失败：取不到首租户"
}

// isPlatformOperatorTenant 对外判定入口（带缓存）。
func isPlatformOperatorTenant(ctx context.Context, tenantID uint) (bool, error) {
	firstID, err := platformOperatorFirstTenantID(ctx)
	if err != nil {
		return false, err
	}
	if firstID == 0 {
		return false, ErrPlatformOperatorUnresolved
	}
	return tenantID == firstID, nil
}

// platformOperatorFirstTenantID 取首租户 ID（带进程级缓存）。
func platformOperatorFirstTenantID(ctx context.Context) (uint, error) {
	platformOperatorCache.mu.RLock()
	if platformOperatorCache.loaded && time.Since(platformOperatorCache.at) < platformOperatorCacheTTL {
		id := platformOperatorCache.tenantID
		platformOperatorCache.mu.RUnlock()
		return id, nil
	}
	platformOperatorCache.mu.RUnlock()

	id, err := platformOperatorResolver(ctx)
	if err != nil {
		return 0, err
	}

	platformOperatorCache.mu.Lock()
	platformOperatorCache.tenantID = id
	platformOperatorCache.loaded = true
	platformOperatorCache.at = time.Now()
	platformOperatorCache.mu.Unlock()
	return id, nil
}
