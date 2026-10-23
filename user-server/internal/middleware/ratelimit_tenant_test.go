package middleware

// A5（`未完成任务.md` §9）回归：限流/配额键补租户维度。
//
// 修前形态（2026-10-23 前）：`RateLimitMiddleware` 与 `PathQuotaMiddleware` 的桶键只有
// `c.ClientIP()`（外加 apikey 变体）。SaaS 部署下一台 NAT/反向代理后面的**所有租户共用
// 同一个桶**：一个租户打出流量，其他租户一起吃 429。症状看起来像"限流参数配小了"，
// 于是被当成运维问题而不是隔离缺陷。
//
// 判据（本文件钉死的四条）：
//  1. off 模式键形**逐字节不变**（离线单机零回归，且不多付一次令牌解析）；
//  2. on 模式同一 IP 的两个租户各自有桶，一个被 429 另一个仍然 200；
//  3. on 模式无令牌/坏令牌/令牌无租户 ⇒ 回落 IP 键（显式登记的语义，不是遗漏）；
//  4. 监控读数口（GetRateLimitStatus）与写入侧用同一个键函数，读得到自己租户的桶。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// withTenantMode 切换进程级形态并在用例结束时还原（同包其它用例按 off 断言的不能被污染）。
func withTenantMode(t *testing.T, m tenant.Mode) {
	t.Helper()
	prev := tenant.CurrentMode()
	tenant.SetMode(m)
	t.Cleanup(func() { tenant.SetMode(prev) })
}

// tenantToken 造一枚带租户归属的令牌（tid=0 表示不带租户维度的历史令牌）。
func tenantToken(t *testing.T, tid uint) string {
	t.Helper()
	j := utils.NewJWTUtils(utils.DefaultJWTConfig)
	tok, err := j.GenerateTokenForTenant(7, "u7", "user", "", 0, 0, tid)
	if err != nil {
		t.Fatalf("造令牌失败: %v", err)
	}
	return tok
}

// keyForRequest 在给定模式下用「IP + 可选令牌」构造一次限流键。
func keyForRequest(t *testing.T, ip, token string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.RemoteAddr = ip + ":12345"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	c.Request = req
	return scopedRateLimitKey(c, c.ClientIP())
}

// TestRateLimitKey_OffModeIsByteIdentical off 模式：无论带什么令牌，键都是原 IP 串。
//
// 这条是硬约束（"off 逐字节等价"）的唯一可执行断言：键多一个字符就是换了一个桶，
// 升级瞬间所有人的限流计数全部归零。
func TestRateLimitKey_OffModeIsByteIdentical(t *testing.T) {
	withTenantMode(t, tenant.ModeOff)

	if got := keyForRequest(t, "203.0.113.9", ""); got != "203.0.113.9" {
		t.Fatalf("off 模式无令牌：键必须是裸 IP，实际 %q", got)
	}
	if got := keyForRequest(t, "203.0.113.9", tenantToken(t, 5)); got != "203.0.113.9" {
		t.Fatalf("off 模式带租户令牌：键仍必须是裸 IP（逐字节等价），实际 %q", got)
	}
	// 坏令牌同理：off 模式下连解析都不该发生
	if got := keyForRequest(t, "203.0.113.9", "not-a-jwt"); got != "203.0.113.9" {
		t.Fatalf("off 模式坏令牌：键必须是裸 IP，实际 %q", got)
	}
}

// TestRateLimitKey_OnModeHasTenantPrefix on 模式：键带 `t<id>:` 前缀；无租户来源则回落 IP。
func TestRateLimitKey_OnModeHasTenantPrefix(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)

	if got := keyForRequest(t, "203.0.113.9", tenantToken(t, 5)); got != "t5:203.0.113.9" {
		t.Fatalf("on 模式租户 5：键应为 t5:203.0.113.9，实际 %q", got)
	}
	if got := keyForRequest(t, "203.0.113.9", tenantToken(t, 6)); got != "t6:203.0.113.9" {
		t.Fatalf("on 模式租户 6：键应为 t6:203.0.113.9，实际 %q", got)
	}
	// 无令牌 / 坏令牌 / 令牌无租户维度（历史令牌）⇒ IP 口径（显式登记的选择）
	for _, tc := range []struct{ name, tok string }{
		{"无令牌", ""},
		{"坏令牌", "not-a-jwt"},
		{"历史令牌无 tenant_id", tenantToken(t, 0)},
	} {
		if got := keyForRequest(t, "203.0.113.9", tc.tok); got != "203.0.113.9" {
			t.Fatalf("on 模式%s：应回落 IP 键，实际 %q", tc.name, got)
		}
	}
}

// newRateLimitTestRouter 仅挂全局限流的最小路由面。
func newRateLimitTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimitMiddleware(RateLimitConfig{RPS: 1, BucketSize: 1, Enabled: true}))
	r.GET("/api/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	return r
}

func doRateLimitReq(r *gin.Engine, ip, token string) int {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	req.RemoteAddr = ip + ":12345"
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w.Code
}

// TestRateLimitMiddleware_OnMode_TwoTenantsSameIPDoNotShareBucket
// on 模式下同一出口 IP 的两个租户各自有桶：租户 A 打满吃 429 时，租户 B 仍然 200。
//
// 这条在修前必红：两者键都是裸 IP，A 的第一个请求就会把桶用掉，B 立刻 429。
func TestRateLimitMiddleware_OnMode_TwoTenantsSameIPDoNotShareBucket(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)

	r := newRateLimitTestRouter()
	const ip = "198.51.100.7"
	tokA := tenantToken(t, 5)
	tokB := tenantToken(t, 6)

	if code := doRateLimitReq(r, ip, tokA); code != http.StatusOK {
		t.Fatalf("租户 A 第一次请求：期望 200，得 %d", code)
	}
	if code := doRateLimitReq(r, ip, tokA); code != http.StatusTooManyRequests {
		t.Fatalf("租户 A 第二次请求：桶应已空，期望 429，得 %d", code)
	}
	// 关键断言：换租户即可用，说明 A 的流量没有污染 B 的桶
	if code := doRateLimitReq(r, ip, tokB); code != http.StatusOK {
		t.Fatalf("租户 B 与 A 同 IP，应有自己的桶：期望 200，得 %d（跨租户共桶 = A5 未修）", code)
	}
}

// TestRateLimitMiddleware_OffMode_SameIPSharesBucket 对照组：off 模式下同 IP 共用桶
// （逐字节等价的直接体现——这正是升级前的行为）。
func TestRateLimitMiddleware_OffMode_SameIPSharesBucket(t *testing.T) {
	withTenantMode(t, tenant.ModeOff)

	r := newRateLimitTestRouter()
	const ip = "198.51.100.8"

	if code := doRateLimitReq(r, ip, tenantToken(t, 5)); code != http.StatusOK {
		t.Fatalf("off 模式第一次请求：期望 200，得 %d", code)
	}
	if code := doRateLimitReq(r, ip, tenantToken(t, 6)); code != http.StatusTooManyRequests {
		t.Fatalf("off 模式同 IP 共桶：第二次（另一租户令牌）必须 429，得 %d", code)
	}
}

// TestPathQuotaMiddleware_OnMode_TwoTenantsSameIPDoNotShareBucket 路径配额同一结论。
func TestPathQuotaMiddleware_OnMode_TwoTenantsSameIPDoNotShareBucket(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	resetPathQuotaManagerForTest(t)

	r := newQuotaTestRouter()
	const ip = "198.51.100.9"
	// /api/ai-suggestions = rps 3 / burst 10：10 次用光，第 11 次 429
	tokA := tenantToken(t, 5)
	tokB := tenantToken(t, 6)

	do := func(tok string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/ai-suggestions", nil)
		req.RemoteAddr = ip + ":12345"
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 10; i++ {
		if code := do(tokA); code != http.StatusOK {
			t.Fatalf("租户 A 第 %d 次请求：期望 200，得 %d", i+1, code)
		}
	}
	if code := do(tokA); code != http.StatusTooManyRequests {
		t.Fatalf("租户 A 桶应用尽：期望 429，得 %d", code)
	}
	if code := do(tokB); code != http.StatusOK {
		t.Fatalf("租户 B 与 A 同 IP 应有自己的路径桶：期望 200，得 %d（跨租户共桶 = A5 未修）", code)
	}
}

// TestGetRateLimitStatus_TenantScoped 监控读数口与写入侧同键域：
// 租户 A 用掉的令牌不会算进租户 B 的剩余量。
func TestGetRateLimitStatus_TenantScoped(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	InitRateLimiter(RateLimitConfig{RPS: 1, BucketSize: 10, Enabled: true})
	t.Cleanup(func() { InitRateLimiter(DefaultRateLimitConfig) })

	ctxA := tenant.WithTenantID(context.Background(), 5)
	ctxB := tenant.WithTenantID(context.Background(), 6)

	// 直接消费 A 的桶 4 次（喂给 Allow 的键与中间件同一处构造）
	for i := 0; i < 4; i++ {
		if !globalRateLimiter.Allow(context.Background(), scopedRateLimitKeyFromCtx(ctxA, "203.0.113.9")) {
			t.Fatalf("租户 A 第 %d 次 Allow 应通过（burst=10）", i+1)
		}
	}

	remA, _ := GetRateLimitStatus(ctxA, "203.0.113.9")
	remB, _ := GetRateLimitStatus(ctxB, "203.0.113.9")
	if remA >= 10 {
		t.Fatalf("租户 A 的剩余量应小于 burst（已用 4），实际 %d：读数口没带上租户段？", remA)
	}
	if remB != 10 {
		t.Fatalf("租户 B 未发过请求，剩余量应为 10（不被 A 污染），实际 %d", remB)
	}
}
