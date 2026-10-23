package middleware

// 归属缓存的租户分域（2026-10-23 补）。
//
// 缺陷形状：globalOwnershipCache 的键是 `table:resourceID`，进程级全局 map。
// on 模式下同一条 `SELECT owner_id FROM t WHERE id=?` 在不同租户 ctx 下结果不同
// （隔离 callback 注入 WHERE tenant_id），键里不带租户 ⇒ 一个租户的查询结果会
// 直接命中另一个租户的判定。而 GetOwnerID 的 record-not-found 语义恰是
// "0 = 系统级资源，放行"，所以串租户的后果是 **fail-open**（最长一个 TTL 窗）。
//
// 本组用例钉两件事：
//  1. 键形：on 模式带租户前缀、off 模式与历史逐字节一致（离线零回归）
//  2. 行为：两个租户在同一资源 ID 上互不命中缓存，不会拿别人的判定结果放行自己

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

func TestOwnershipCacheKey_TenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	if got := ownershipCacheKey(context.Background(), "knowledge_bases", 42); got != "knowledge_bases:42" {
		t.Fatalf("off 模式键形必须与历史一致，实际 %q", got)
	}
	// off 模式下即便 ctx 带租户，也不带前缀（键形铁律优先于便利）
	if got := ownershipCacheKey(tenant.WithTenantID(context.Background(), 5), "knowledge_bases", 42); got != "knowledge_bases:42" {
		t.Fatalf("off 模式键形必须与历史一致（租户不参与），实际 %q", got)
	}

	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	a := ownershipCacheKey(tenant.WithTenantID(context.Background(), 5), "knowledge_bases", 42)
	b := ownershipCacheKey(tenant.WithTenantID(context.Background(), 6), "knowledge_bases", 42)
	if a == b {
		t.Fatalf("on 模式下不同租户同一资源必须不同键，实际都为 %q", a)
	}
	if a != "5|knowledge_bases:42" {
		t.Fatalf("on 模式键形 = %q，期望 5|knowledge_bases:42", a)
	}
}

// countingChecker 记录被调用次数，并返回「租户 5 拥有的资源」这一事实。
type countingChecker struct {
	calls atomic.Int32
}

func (c *countingChecker) GetOwnerID(ctx context.Context, table string, resourceID uint) (uint, error) {
	c.calls.Add(1)
	if id, ok := tenant.FromContext(ctx); ok && id == 5 {
		return 7, nil // 资源属于租户 5 的用户 7
	}
	// 其他租户：隔离 callback 下这条记录不可见 ⇒ not found（历史语义 0 = 系统级）
	return 0, nil
}

func TestRequireOwnerWithCache_TenantsDoNotShareCache(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	checker := &countingChecker{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/kb/:id", func(c *gin.Context) {
		// 模拟 JWT 中间件：把 uid 与租户放进 ctx
		c.Set(utils.CtxKeyUserID, uint(8)) // 调用者是另一个用户（uid=8）
		req := c.Request.WithContext(tenant.WithTenantID(c.Request.Context(), tenantIDOfRequest(c)))
		c.Request = req
		c.Next()
	}, RequireOwnerWithCacheAndChecker("id", "knowledge_bases", time.Minute, checker), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	do := func(tenantID uint) int {
		req := httptest.NewRequest(http.MethodGet, "/kb/42", nil)
		req = req.WithContext(tenant.WithTenantID(req.Context(), tenantID))
		req.Header.Set("X-Test-Tenant", "1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	// 租户 5：资源属于用户 7，调用者是 8 ⇒ 403
	if code := do(5); code != http.StatusForbidden {
		t.Fatalf("租户内越权应 403，实际 %d", code)
	}
	// 租户 6：同一资源 ID 在它名下不存在（owner=0）⇒ 不能复用租户 5 的 403 结论，
	// 也不能反向复用。关键断言是 checker 被再打一次（缓存未跨租户命中）。
	_ = do(6)
	if got := checker.calls.Load(); got < 2 {
		t.Fatalf("跨租户请求命中了同一缓存条目（checker 只被调用 %d 次），归属缓存未按租户分域", got)
	}
}

// tenantIDOfRequest 从请求 ctx 取出测试注入的租户（仅测试用）
func tenantIDOfRequest(c *gin.Context) uint {
	if id, ok := tenant.FromContext(c.Request.Context()); ok {
		return id
	}
	return tenant.DefaultTenantID
}
