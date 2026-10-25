// frame_policy_test.go A13：点击劫持护栏必须落在**响应头**上，且只对客服浮标放行。
//
// 缺陷形状（修复前）：`index.html` 的 meta CSP 里没有 frame-ancestors（按规范，
// meta 交付的这一条浏览器直接忽略），服务端也不下发任何 XFO/CSP 头 ⇒ 实际
// **没有**点击劫持防护，而注释曾写"有防护"（ui-test-progress.md 第 8/遗留②）。
//
// 判据成对（少一条都会把"整页不放行"与"整页放行"读成同一件事）：
//   - 普通页面：200 且同时带 XFO=SAMEORIGIN 与 CSP frame-ancestors 'self'；
//   - embed 落地页（?embed=1）：200、CSP frame-ancestors *，且**没有** XFO
//     （XFO 没有标准"全放行"值，带上就是把浮标关回去）；
//   - /chat/embed/{ref} 的 302 必须把 embed=1 带到 Location 里——否则浏览 302 之后
//     加载的 `/` 拿不到放行标记，第三方 iframe 会被挡（这条是护栏可用性的前置）。
package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// framePolicyEngine 装一台只含静态页路由的引擎；dist 里放一份最小 index.html。
func framePolicyEngine(t *testing.T) *gin.Engine {
	t.Helper()
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "index.html"),
		[]byte("<!doctype html><html><body>spa</body></html>"), 0o644); err != nil {
		t.Fatalf("写 index.html 失败: %v", err)
	}
	prev, had := os.LookupEnv("USER_WEB_DIST")
	if err := os.Setenv("USER_WEB_DIST", dist); err != nil {
		t.Fatalf("设置 USER_WEB_DIST 失败: %v", err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("USER_WEB_DIST", prev)
		} else {
			_ = os.Unsetenv("USER_WEB_DIST")
		}
	})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())
	setupEmbedStaticRoutes(r)
	return r
}

func TestSPAFramePolicyOnResponseHeaders(t *testing.T) {
	r := framePolicyEngine(t)

	t.Run("普通页面不许被第三方 iframe", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("/ 回 %d，期望 200", w.Code)
		}
		if got := w.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
			t.Errorf("X-Frame-Options = %q，期望 SAMEORIGIN（meta 里的 frame-ancestors 浏览器不认，只有响应头算数）", got)
		}
		if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors 'self'") {
			t.Errorf("CSP = %q，缺 frame-ancestors 'self'", got)
		}
	})

	t.Run("embed 落地页对第三方放行且不带 XFO", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/?embed=1", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("/?embed=1 回 %d，期望 200", w.Code)
		}
		if got := w.Header().Get("Content-Security-Policy"); !strings.Contains(got, "frame-ancestors *") {
			t.Errorf("embed 页 CSP = %q，缺 frame-ancestors *", got)
		}
		if got := w.Header().Get("X-Frame-Options"); got != "" {
			t.Errorf("embed 页带了 X-Frame-Options=%q：XFO 没有标准全放行值，带上就会把浮标挡在第三方站外", got)
		}
	})

	t.Run("embed 入口的 302 必须把放行标记带过去", func(t *testing.T) {
		for _, tc := range []struct{ path, wantLoc string }{
			{"/chat/embed/acc_123", "/?embed=1#/chat/embed/acc_123"},
			{"/chat/embed", "/?embed=1#/chat/embed/default"},
		} {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if w.Code != http.StatusFound {
				t.Fatalf("%s 回 %d，期望 302", tc.path, w.Code)
			}
			if got := w.Header().Get("Location"); got != tc.wantLoc {
				t.Errorf("%s 的 Location = %q，期望 %q（丢了这个标记，第三方 iframe 落地时会被 SAMEORIGIN 挡）", tc.path, got, tc.wantLoc)
			}
		}
	})

	t.Run("index.html 直链同样受护", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/index.html", nil))
		if got := w.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
			t.Errorf("/index.html 的 X-Frame-Options = %q，期望 SAMEORIGIN（绕过 / 就能绕开护栏）", got)
		}
	})
}
