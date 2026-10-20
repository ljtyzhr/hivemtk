package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"

	"github.com/gin-gonic/gin"
)

// 平台运营方守卫的回归。
//
// 背景（2026-10-21 跨租户越权）：SaaS 模式下 "admin" 角色同时表示
// 平台运营方（跨租户）与租户管理员（租户内）。原守卫 AdminAuthMiddleware
// 只判 role=="admin" ⇒ 租户 admin 拿到了平台运营方的权限：
// 看到全部租户、停用其他租户、重启实例。
//
// 本文件钉死 PlatformOperatorMiddleware 的判定：
//   - 非 admin 角色 ⇒ 403（沿用角色前置条件）
//   - admin 角色但租户不是首租户 ⇒ 403
//   - admin 角色且租户是首租户 ⇒ 放行
//   - off 模式 ⇒ 一律放行（离线单机只有一个租户，行为与历史版本逐字节一致）
//   - 判定不出首租户 ⇒ 拒绝（权限边界上"查不到"不等于"放行"）

// withPlatformOperatorTenant 构造一个已绑定租户、已带 role 的请求。
func withPlatformOperatorTenant(role string, tenantID uint) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", role)
	ctx := context.Background()
	if tenantID != 0 {
		ctx = tenant.WithTenantID(ctx, tenantID)
	}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/manage/tenants", nil).WithContext(ctx)
	return c
}

// stubFirstTenant 注入首租户判定器（避免每个用例连库）。
func stubFirstTenant(t *testing.T, id uint, err error) {
	t.Helper()
	orig := platformOperatorResolver
	t.Cleanup(func() { platformOperatorResolver = orig })
	platformOperatorResolver = func(context.Context) (uint, error) { return id, err }
	// 缓存是进程级的，注入后必须失效，否则上一例的首租户会串到下一例。
	platformOperatorCache.mu.Lock()
	platformOperatorCache.loaded = false
	platformOperatorCache.tenantID = 0
	platformOperatorCache.at = time.Time{}
	platformOperatorCache.mu.Unlock()
}

// inMultiTenantMode 切到 SaaS 多租户形态（守卫的跨租户判定只在 on 形态生效）。
func inMultiTenantMode(t *testing.T) {
	t.Helper()
	orig := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(orig) })
}

func TestPlatformOperatorMiddlewareRejectsNonAdminRole(t *testing.T) {
	inMultiTenantMode(t)
	stubFirstTenant(t, 1, nil)
	for _, role := range []string{"user", "staff", "customer_service", ""} {
		c := withPlatformOperatorTenant(role, 1)
		PlatformOperatorMiddleware()(c)
		if !c.IsAborted() {
			t.Errorf("role=%q 应被拒绝，却放行了", role)
		}
		if c.Writer.Status() != http.StatusForbidden && c.Writer.Status() != http.StatusUnauthorized {
			t.Errorf("role=%q 期望 401/403，实际=%d", role, c.Writer.Status())
		}
	}
}

func TestPlatformOperatorMiddlewareRejectsNonFirstTenant(t *testing.T) {
	inMultiTenantMode(t)
	// 首租户 = 1；调用者属于租户 2 ⇒ 不是平台运营方
	stubFirstTenant(t, 1, nil)
	c := withPlatformOperatorTenant("admin", 2)
	PlatformOperatorMiddleware()(c)
	if !c.IsAborted() {
		t.Fatal("非首租户的 admin 应被拒绝，却放行了")
	}
	if c.Writer.Status() != http.StatusForbidden {
		t.Errorf("期望 403，实际=%d", c.Writer.Status())
	}
}

func TestPlatformOperatorMiddlewareAllowsFirstTenant(t *testing.T) {
	inMultiTenantMode(t)
	stubFirstTenant(t, 7, nil)
	c := withPlatformOperatorTenant("admin", 7)
	PlatformOperatorMiddleware()(c)
	if c.IsAborted() {
		t.Fatalf("首租户（平台运营方）应放行，却中断了，status=%d", c.Writer.Status())
	}
}

func TestPlatformOperatorMiddlewareRejectsWhenUnresolved(t *testing.T) {
	inMultiTenantMode(t)
	// 判定不出首租户 ⇒ 拒绝，而不是放行。
	stubFirstTenant(t, 0, ErrPlatformOperatorUnresolved)
	c := withPlatformOperatorTenant("admin", 1)
	PlatformOperatorMiddleware()(c)
	if !c.IsAborted() {
		t.Fatal("判定不出首租户时必须拒绝，却放行了")
	}
	if c.Writer.Status() != http.StatusInternalServerError {
		t.Errorf("期望 500，实际=%d", c.Writer.Status())
	}
}

func TestPlatformOperatorMiddlewareRejectsMissingTenantBinding(t *testing.T) {
	inMultiTenantMode(t)
	stubFirstTenant(t, 1, nil)
	// on 模式 + ctx 无租户绑定 ⇒ 拒绝
	c := withPlatformOperatorTenant("admin", 0)
	PlatformOperatorMiddleware()(c)
	if !c.IsAborted() {
		t.Fatal("on 模式下无租户绑定应被拒绝，却放行了")
	}
}

func TestPlatformOperatorMiddlewareOfflineModeAlwaysAllows(t *testing.T) {
	// off 模式（离线单机）：只有本机超管，行为与历史版本逐字节一致 ⇒ 一律放行。
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	stubFirstTenant(t, 1, nil)
	c := withPlatformOperatorTenant("admin", 999)
	PlatformOperatorMiddleware()(c)
	if c.IsAborted() {
		t.Fatalf("off 模式应一律放行，却中断了 status=%d", c.Writer.Status())
	}
}
