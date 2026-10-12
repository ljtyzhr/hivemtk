package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// machineProfileKeys 只有超管该看到的机器画像字段。
//
// 逐条点名而不是断言「响应体只有一两个键」：后者会随 DTO 加字段自然变红，
// 也证不出被裁的恰好是泄露面。
var machineProfileKeys = []string{
	`"hostname"`,
	`"go_version"`,
	`"uptime"`,
	`"num_cpu"`,
	`"num_goroutine"`,
	`"cpu_usage"`,
	`"memory_usage"`,
	`"disk_usage"`,
	`"alloc_memory"`,
	`"sys_memory"`,
}

func systemInfoEngine(t *testing.T) *gin.Engine {
	t.Helper()
	database := testutil.NewTestDB(t)
	dbutil.SetTestDB(database)
	t.Cleanup(func() { dbutil.SetTestDB(nil) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	Setup(r, database)
	return r
}

// TestSystemInfoExposesPlatformEnabled 走真实路由表打 GET /api/system/info，
// 断言响应体带 platform_enabled，且**不带任何 Authorization 头**就能拿到。
//
// 为什么钉在路由层而不是 controller：前端拿这个字段是为了决定要不要渲染"上架到官方商城"
// 入口，它依赖两件事同时成立——字段名对、这个端点在 public 组里（Playground 可能在
// 令牌刷新完成前就发这个请求）。少任何一件，入口就会永久隐藏或请求 401。
func TestSystemInfoExposesPlatformEnabled(t *testing.T) {
	r := systemInfoEngine(t)

	for _, tc := range []struct {
		env  string
		want string
	}{
		{"false", `"platform_enabled":false`},
		{"true", `"platform_enabled":true`},
	} {
		t.Setenv("PLATFORM_ENABLED", tc.env)
		req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("PLATFORM_ENABLED=%s 时 GET /api/system/info 应匿名 200，实际 %d body=%s",
				tc.env, w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), tc.want) {
			t.Errorf("PLATFORM_ENABLED=%s 响应体应含 %s，实际=%s", tc.env, tc.want, w.Body.String())
		}
	}
}

// TestSystemInfoAnonymousCannotReadMachineProfile 匿名侧的字段收窄：
// 同一条口、同一个 200，但机器画像一片都不许出现。
//
// 这条断言的存在理由是「public 组里的口不能靠状态码收口」：路由链上只有
// OptionalAuthMiddleware，它认不出身份也照样放行，所以唯一的判据是响应体里有没有那些键。
func TestSystemInfoAnonymousCannotReadMachineProfile(t *testing.T) {
	t.Setenv("PLATFORM_ENABLED", "false")
	r := systemInfoEngine(t)

	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("匿名读 GET /api/system/info 应 200（前端启动期依赖），实际 %d body=%s",
			w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, key := range machineProfileKeys {
		if strings.Contains(body, key) {
			t.Errorf("匿名响应体泄露机器画像字段 %s：%s", key, body)
		}
	}

	// 反向控制：匿名这一趟必须真的走到了 handler 并拿到收窄后的体，
	// 否则上面十个"没有出现"可以是被 401/404 挡在门外量出来的。
	if !strings.Contains(body, `"platform_enabled"`) {
		t.Fatalf("匿名响应体没有 platform_enabled，说明没走到 handler，上面的裁剪断言无从谈起：%s", body)
	}
}

// TestSystemInfoAdminReadsMachineProfile 超管侧的正向对照：机器画像字段仍在，
// 只是从匿名那一档升上来了。没有这一条，上面那条判据在「handler 把整片字段都删了」
// 的世界里也会绿，而系统监控页会当场空表。
func TestSystemInfoAdminReadsMachineProfile(t *testing.T) {
	t.Setenv("PLATFORM_ENABLED", "false")
	r := systemInfoEngine(t)

	token, err := utils.NewJWTUtils(utils.DefaultJWTConfig).GenerateToken(1, "machine-profile-admin", "admin")
	if err != nil {
		t.Fatalf("签超管令牌失败: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/system/info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("超管读 GET /api/system/info 应 200，实际 %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, key := range []string{`"hostname"`, `"go_version"`, `"uptime"`, `"num_cpu"`} {
		if !strings.Contains(body, key) {
			t.Errorf("超管响应体缺机器画像字段 %s（裁剪按身份走，不该把 admin 也裁了）：%s", key, body)
		}
	}
}
