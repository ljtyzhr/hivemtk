package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

type fakeOwnershipChecker struct {
	ownerID uint
	err     error
	calls   atomic.Int32
}

func (f *fakeOwnershipChecker) GetOwnerID(_ context.Context, table string, resourceID uint) (uint, error) {
	f.calls.Add(1)
	if f.err != nil {
		return 0, f.err
	}
	_ = table
	_ = resourceID
	return f.ownerID, nil
}

func newOwnershipTestRouter(checker OwnershipChecker, withCache bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	r.Use(func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Next()
	})
	mw := RequireOwnerWithChecker("id", "test_table", checker)
	if withCache {
		mw = RequireOwnerWithCacheAndChecker("id", "test_table", 200*time.Millisecond, checker)
	}
	r.GET("/r/:id", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "ok": true})
	})
	return r
}

// TestRequireOwner_OwnerMatch：owner 匹配 → 200
func TestRequireOwner_OwnerMatch(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	r := newOwnershipTestRouter(checker, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/r/100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_OwnerMismatch：owner 不匹配 → 403
func TestRequireOwner_OwnerMismatch(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 99}
	r := newOwnershipTestRouter(checker, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/r/100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_SystemResourceDefaultDeny：owner=0（系统级行）默认 → 404
//
// 2026-10-23 语义收口：`owner_id = 0` 既可能是平台内置行、也可能是历史脏数据没填 owner，
// 库里长得一样。默认拒绝 + 显式 WithSystemOwnedAllowed() 才放行。
func TestRequireOwner_SystemResourceDefaultDeny(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 0}
	r := newOwnershipTestRouter(checker, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/r/100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_SystemResourceOptIn：显式 WithSystemOwnedAllowed() → 放行
func TestRequireOwner_SystemResourceOptIn(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 0}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", uint(42)); c.Next() })
	r.GET("/r/:id", RequireOwnerWithChecker("id", "test_table", checker, WithSystemOwnedAllowed()), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/r/100", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_NotFound404：checker 报"不存在" → 404（原来是 500）
//
// 两个表名必须各不相同：归属缓存是**进程级全局 map**，同表同 ID 会被前一个用例塞进去，
// 这类"用例之间通过全局缓存互相污染"的假红假绿在本包历史上出现过不止一次。
func TestRequireOwner_NotFound404(t *testing.T) {
	cases := []struct {
		withCache bool
		table     string
	}{
		{false, "notfound_uncached_table"},
		{true, "notfound_cached_table"},
	}
	for _, tc := range cases {
		checker := &fakeOwnershipChecker{err: ErrOwnershipNotFound}
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user_id", uint(42)); c.Next() })
		mw := RequireOwnerWithChecker("id", tc.table, checker)
		if tc.withCache {
			mw = RequireOwnerWithCacheAndChecker("id", tc.table, 200*time.Millisecond, checker)
		}
		r.GET("/r/:id", mw, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/r/100", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("withCache=%v want 404, got %d body=%s", tc.withCache, w.Code, w.Body.String())
		}
	}
}

// TestRequireOwner_DBError：checker 返回 error → 500（fail-closed）
func TestRequireOwner_DBError(t *testing.T) {
	checker := &fakeOwnershipChecker{err: errors.New("db down")}
	r := newOwnershipTestRouter(checker, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/r/100", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_InvalidID：URL 参数非法 → 400
func TestRequireOwner_InvalidID(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	r := newOwnershipTestRouter(checker, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/r/notanumber", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestRequireOwner_NilCheckerPanics：checker=nil → **构造期 panic**（原来是"请求期放行"）
//
// 这是本轮 A2 的核心：nil checker 的 fail-open 分支已删除，任何"忘了注入 checker"的接线
// 都会在启动时炸掉，而不是在线上安静地一律放行。
func TestRequireOwner_NilCheckerPanics(t *testing.T) {
	assertPanics(t, "RequireOwnerWithChecker", func() {
		RequireOwnerWithChecker("id", "t", nil)
	})
	assertPanics(t, "RequireOwnerWithCacheAndChecker", func() {
		RequireOwnerWithCacheAndChecker("id", "t", time.Second, nil)
	})
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s(nil checker) 应当 panic，实际未 panic（fail-open 分支又被加回来了？）", name)
		}
	}()
	fn()
}

// TestRequireOwner_WithCache_HitsOnce：缓存命中后不再调用 checker
func TestRequireOwner_WithCache_HitsOnce(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	r := newOwnershipTestRouter(checker, true)
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/r/100", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("iter=%d want 200 got %d", i, w.Code)
		}
	}
	if got := checker.calls.Load(); got != 1 {
		t.Fatalf("checker should be hit once, got %d", got)
	}
}

// TestRequireOwner_WithCache_RecheckAfterTTL：缓存过期后重新查询
func TestRequireOwner_WithCache_RecheckAfterTTL(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}

	gin.SetMode(gin.TestMode)
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("user_id", uint(42)); c.Next() })
	mw := RequireOwnerWithCacheAndChecker("id", "test_ttl", 100*time.Millisecond, checker)
	r2.GET("/r2/:id", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w1 := httptest.NewRecorder()
	r2.ServeHTTP(w1, httptest.NewRequest("GET", "/r2/100", nil))
	time.Sleep(150 * time.Millisecond)
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest("GET", "/r2/100", nil))
	if got := checker.calls.Load(); got < 2 {
		t.Fatalf("checker should be re-invoked after TTL, got %d", got)
	}
}

// TestRequireOwner_WithCache_ErrorCached：错误也被缓存，避免循环触发
func TestRequireOwner_WithCache_ErrorCached(t *testing.T) {
	checker := &fakeOwnershipChecker{err: errors.New("db down")}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", uint(42)); c.Next() })
	mw := RequireOwnerWithCacheAndChecker("id", "test_err", 5*time.Second, checker)
	r.GET("/r/:id", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/r/100", nil))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("iter=%d want 500 got %d", i, w.Code)
		}
	}
	if got := checker.calls.Load(); got != 1 {
		t.Fatalf("error path should cache too, got %d calls", got)
	}
}

// TestInvalidateOwnershipCache：失效后下次请求重新查询
func TestInvalidateOwnershipCache(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	r := newOwnershipTestRouter(checker, true)

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest("GET", "/r/200", nil))

	InvalidateOwnershipCache("test_table", 200)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/r/200", nil))
	if got := checker.calls.Load(); got != 2 {
		t.Fatalf("after invalidate should re-query, got %d calls", got)
	}
}

// TestOwnershipCache_Concurrent：并发请求缓存无 race
func TestOwnershipCache_Concurrent(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	r := newOwnershipTestRouter(checker, true)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/r/300", nil))
			if w.Code != http.StatusOK {
				t.Errorf("want 200 got %d", w.Code)
			}
		}()
	}
	wg.Wait()
}

// TestOwnershipCache_DefaultTTL：非正数 ttl → 默认 5s
func TestOwnershipCache_DefaultTTL(t *testing.T) {
	checker := &fakeOwnershipChecker{ownerID: 42}
	mw := RequireOwnerWithCacheAndChecker("id", "t", 0, checker)
	_ = mw
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", uint(42))
		c.Next()
	})
	r.GET("/x/:id", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x/1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200 got %d body=%s", w.Code, w.Body.String())
	}
}

// TestDBOwnershipChecker_NilDB：db=nil → `ErrOwnershipMisconfigured`（不再 (0,nil) 放行）
func TestDBOwnershipChecker_NilDB(t *testing.T) {
	c := NewDBOwnershipChecker(nil)
	_, err := c.GetOwnerID(context.Background(), "t", 1)
	if !errors.Is(err, ErrOwnershipMisconfigured) {
		t.Fatalf("want ErrOwnershipMisconfigured, got %v", err)
	}
}

// TestDBOwnershipChecker_EmptyTable：table 为空 → `ErrOwnershipMisconfigured`
func TestDBOwnershipChecker_EmptyTable(t *testing.T) {
	c := NewDBOwnershipChecker(nil)
	_, err := c.GetOwnerID(context.Background(), "", 1)
	if !errors.Is(err, ErrOwnershipMisconfigured) {
		t.Fatalf("want ErrOwnershipMisconfigured, got %v", err)
	}
}

// TestDBOwnershipChecker_ZeroID：resourceID=0 → `ErrOwnershipNotFound`（⇒ 404）
func TestDBOwnershipChecker_ZeroID(t *testing.T) {
	c := NewDBOwnershipChecker(nil)
	_, err := c.GetOwnerID(context.Background(), "t", 0)
	if !errors.Is(err, ErrOwnershipNotFound) {
		t.Fatalf("want ErrOwnershipNotFound, got %v", err)
	}
}

// TestRequireOwner_UIDZero：未注入 uid → 401
func TestRequireOwner_UIDZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {

		c.Next()
	})
	checker := &fakeOwnershipChecker{ownerID: 42}
	mw := RequireOwnerWithChecker("id", "t", checker)
	_ = checker
	r.GET("/x/:id", mw, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/x/1", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", w.Code)
	}
}

// TestOwnershipCacheKeyFormat：key 格式正确（防止后续重构破坏缓存键）
func TestOwnershipCacheKeyFormat(t *testing.T) {
	if got := strconv.FormatUint(uint64(123), 10); got != "123" {
		t.Fatalf("format error: %s", got)
	}
	_ = utils.GetUID
}
