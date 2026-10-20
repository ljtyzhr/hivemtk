package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// resetPathQuotaManagerForTest 换一个全新的限流管理器（并停掉旧的清理协程），
// 保证各用例的桶状态互不泄漏；用例结束后再清一次。
func resetPathQuotaManagerForTest(t *testing.T) {
	t.Helper()
	pathQuotaMgrOnce = sync.Once{}
	if globalPathQuotaMgr != nil {
		globalPathQuotaMgr.stop()
	}
	globalPathQuotaMgr = nil
	t.Cleanup(func() {
		pathQuotaMgrOnce = sync.Once{}
		if globalPathQuotaMgr != nil {
			globalPathQuotaMgr.stop()
		}
		globalPathQuotaMgr = nil
	})
}

// newQuotaTestRouter 只挂 PathQuotaMiddleware 的最小路由面。
func newQuotaTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PathQuotaMiddleware())
	for _, p := range []string{"/api/ai-suggestions", "/api/unlisted", "/api/reach"} {
		path := p
		r.GET(path, func(c *gin.Context) { c.String(http.StatusOK, "ok") })
	}
	return r
}

// doQuotaReq 以指定来源 IP 发一次请求（RemoteAddr 决定 ClientIP）。
func doQuotaReq(r *gin.Engine, path, ip string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = ip + ":12345"
	r.ServeHTTP(w, req)
	return w
}

// TestPathQuotaMiddleware_UnlistedPathPasses 未列入配额表的路径直接放行：
// 自过滤语义——中间件对不在 resolvePathQuotas 结果里的路径零干预。
func TestPathQuotaMiddleware_UnlistedPathPasses(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	r := newQuotaTestRouter()
	for i := 0; i < 20; i++ {
		w := doQuotaReq(r, "/api/unlisted", "10.9.0.1")
		if w.Code != http.StatusOK {
			t.Fatalf("未列入路径第 %d 次请求: 期望 200, 得 %d", i+1, w.Code)
		}
	}
}

// TestPathQuotaMiddleware_OverQuota429AndBodyShape 列入路径超配额返 429，
// 且 body 形状（code/msg/path/rps/retry_after）与接线前逐字段一致。
func TestPathQuotaMiddleware_OverQuota429AndBodyShape(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	cfg := DefaultPathQuota["/api/ai-suggestions"] // rps 3 / bucket 10
	r := newQuotaTestRouter()

	var denied *httptest.ResponseRecorder
	deniedAt := -1
	for i := 0; i < int(cfg.BucketSize)+5; i++ {
		w := doQuotaReq(r, "/api/ai-suggestions", "10.9.1.1")
		if w.Code == http.StatusTooManyRequests {
			denied = w
			deniedAt = i
			break
		}
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求: 期望 200, 得 %d", i+1, w.Code)
		}
	}
	if denied == nil {
		t.Fatalf("打满 bucket=%d 后未出现 429", cfg.BucketSize)
	}
	// 令牌桶初始满 token：前 bucket 次应全过，第 bucket+1 次（下标）被拒。
	if deniedAt != cfg.BucketSize {
		t.Errorf("首个 429 出现在第 %d 次（下标），期望下标 %d（=bucket 容量）", deniedAt, cfg.BucketSize)
	}

	var body map[string]any
	if err := json.Unmarshal(denied.Body.Bytes(), &body); err != nil {
		t.Fatalf("429 body 不是合法 JSON: %v", err)
	}
	if code, _ := body["code"].(float64); code != 429 {
		t.Errorf("body.code = %v, 期望 429", body["code"])
	}
	if msg, _ := body["msg"].(string); msg == "" {
		t.Error("body.msg 应为非空字符串")
	}
	if p, _ := body["path"].(string); p != "/api/ai-suggestions" {
		t.Errorf("body.path = %v, 期望 /api/ai-suggestions", body["path"])
	}
	if rps, _ := body["rps"].(float64); rps != cfg.RPS {
		t.Errorf("body.rps = %v, 期望 %v（配额表回显）", body["rps"], cfg.RPS)
	}
	if ra, _ := body["retry_after"].(float64); ra != 1 {
		t.Errorf("body.retry_after = %v, 期望 1", body["retry_after"])
	}

	// 被拒后后续请求持续 429（桶已空且 RPS=3 短窗口内不回填足以放行的量）。
	w := doQuotaReq(r, "/api/ai-suggestions", "10.9.1.1")
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("打满后的下一请求: 期望 429, 得 %d", w.Code)
	}
}

// TestPathQuotaMiddleware_PerIPIsolation 不同 IP 互不影响：A 打满不影响 B。
// 这是本次改造的核心语义——旧实现全站共享桶，一个活跃租户就能把所有人打到 429。
func TestPathQuotaMiddleware_PerIPIsolation(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	cfg := DefaultPathQuota["/api/reach"] // rps 50 / bucket 500
	r := newQuotaTestRouter()

	// A 打满自己的桶。
	got429 := false
	for i := 0; i < cfg.BucketSize+10; i++ {
		w := doQuotaReq(r, "/api/reach", "10.9.2.1")
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			break
		}
	}
	if !got429 {
		t.Fatal("A 连发 bucket+10 次应至少出现一次 429")
	}

	// B 同路径：首请求必须 200（自己的桶是满的）。
	w := doQuotaReq(r, "/api/reach", "10.9.2.2")
	if w.Code != http.StatusOK {
		t.Errorf("A 打满后 B 的首请求: 期望 200, 得 %d（桶应按 IP 隔离）", w.Code)
	}
	// B 再打几次也应 200。
	for i := 0; i < 5; i++ {
		w := doQuotaReq(r, "/api/reach", "10.9.2.2")
		if w.Code != http.StatusOK {
			t.Fatalf("B 第 %d 次请求: 期望 200, 得 %d", i+2, w.Code)
		}
	}
}

// TestPathQuotaManager_ParamChangeRebuildsLimiter 参数热更新：某 path 的 cfg 与
// 现存桶构建参数不一致（BucketSize 变小）时，下一次请求按新参数重建桶并生效。
func TestPathQuotaManager_ParamChangeRebuildsLimiter(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	const p = "/api/quota-rebuild-test"
	old, had := DefaultPathQuota[p]
	t.Cleanup(func() {
		if had {
			DefaultPathQuota[p] = old
		} else {
			delete(DefaultPathQuota, p)
		}
	})
	// RPS=1 且回填慢：若未重建（bug），空桶在毫秒窗口内回填不出 token，
	// 首请求就会 429——测试因此能抓住"没重建"这一侧。
	DefaultPathQuota[p] = PathQuotaConfig{Path: p, RPS: 1, BucketSize: 5}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(PathQuotaMiddleware())
	r.GET(p, func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	// 阶段一：bucket=5，打满。
	for i := 0; i < 5; i++ {
		w := doQuotaReq(r, p, "10.9.3.1")
		if w.Code != http.StatusOK {
			t.Fatalf("阶段一第 %d 次: 期望 200, 得 %d", i+1, w.Code)
		}
	}
	if w := doQuotaReq(r, p, "10.9.3.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("阶段一打满后: 期望 429, 得 %d", w.Code)
	}

	// 阶段二：改小 BucketSize → 参数变化触发桶重建（满桶=1）。
	DefaultPathQuota[p] = PathQuotaConfig{Path: p, RPS: 1, BucketSize: 1}
	if w := doQuotaReq(r, p, "10.9.3.1"); w.Code != http.StatusOK {
		t.Fatalf("参数改小后首请求: 期望 200（桶按新参数重建、满桶 1），得 %d", w.Code)
	}
	if w := doQuotaReq(r, p, "10.9.3.1"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("参数改小后第二请求: 期望 429（新桶容量 1 已耗尽），得 %d", w.Code)
	}

	// 管理器内桶参数应已同步为新值。
	mgr := getPathQuotaManager()
	mgr.mu.Lock()
	b, ok := mgr.buckets["10.9.3.1\x00"+p]
	mgr.mu.Unlock()
	if !ok {
		t.Fatal("管理器中找不到 (ip,path) 桶")
	}
	if b.bucketSize != 1 || b.rps != 1 {
		t.Errorf("桶构建参数 = rps %v/bucket %d, 期望 rps 1/bucket 1（已按新 cfg 重建）", b.rps, b.bucketSize)
	}
}

// TestGetQuotaSnapshots_MatchesConfig 快照条目与配置一致：条数、路径集合、
// ConfiguredRPS/ConfiguredBurst 逐项等于 resolvePathQuotas 的结果；
// 有流量后 Used/Remaining 读数来自新管理器且自洽。
func TestGetQuotaSnapshots_MatchesConfig(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	snaps := GetQuotaSnapshots(context.Background())
	if len(snaps) != len(DefaultPathQuota) {
		t.Fatalf("快照条数 = %d, 期望 = 配额表条数 %d", len(snaps), len(DefaultPathQuota))
	}
	byPath := make(map[string]quotaUsageSnapshot, len(snaps))
	for _, s := range snaps {
		byPath[s.Path] = s
	}
	for path, want := range DefaultPathQuota {
		got, ok := byPath[path]
		if !ok {
			t.Errorf("快照缺少路径 %s", path)
			continue
		}
		if got.ConfiguredRPS != want.RPS || got.ConfiguredBurst != want.BucketSize {
			t.Errorf("快照 %s 配置 = rps %v/burst %d, 期望 rps %v/burst %d",
				path, got.ConfiguredRPS, got.ConfiguredBurst, want.RPS, want.BucketSize)
		}
	}

	// 打 3 次后：Used 反映本进程计数，Remaining 为各桶余量之和（< 满桶）。
	r := newQuotaTestRouter()
	for i := 0; i < 3; i++ {
		if w := doQuotaReq(r, "/api/ai-suggestions", "10.9.4.1"); w.Code != http.StatusOK {
			t.Fatalf("第 %d 次请求: 期望 200, 得 %d", i+1, w.Code)
		}
	}
	snaps = GetQuotaSnapshots(context.Background())
	for _, s := range snaps {
		if s.Path != "/api/ai-suggestions" {
			continue
		}
		if s.Used != 3 {
			t.Errorf("Used = %d, 期望 3（本进程计数）", s.Used)
		}
		burst := int64(DefaultPathQuota["/api/ai-suggestions"].BucketSize)
		if s.Remaining < 0 || s.Remaining > burst {
			t.Errorf("Remaining = %d, 期望在 [0, %d]", s.Remaining, burst)
		}
		if s.Remaining >= burst {
			t.Errorf("Remaining = %d, 消耗 3 个 token 后应 < 满桶 %d", s.Remaining, burst)
		}
	}
}

// TestPathQuotaMiddleware_GlobalRateLimiterNil 全局限流器未初始化（globalRateLimiter
// 为 nil）时路径级限流照常工作——新实现不依赖全局桶。
func TestPathQuotaMiddleware_GlobalRateLimiterNil(t *testing.T) {
	service.SetGlobalForTest(nil)
	t.Cleanup(func() { service.SetGlobalForTest(nil) })
	resetPathQuotaManagerForTest(t)

	orig := globalRateLimiter
	globalRateLimiter = nil
	t.Cleanup(func() { globalRateLimiter = orig })

	r := newQuotaTestRouter()
	if w := doQuotaReq(r, "/api/ai-suggestions", "10.9.5.1"); w.Code != http.StatusOK {
		t.Errorf("globalRateLimiter=nil 时首请求: 期望 200, 得 %d", w.Code)
	}
	if snaps := GetQuotaSnapshots(context.Background()); len(snaps) != len(DefaultPathQuota) {
		t.Errorf("globalRateLimiter=nil 时快照条数 = %d, 期望 %d", len(snaps), len(DefaultPathQuota))
	}
}
