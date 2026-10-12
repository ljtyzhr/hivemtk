package middleware

// SetupOnlyGuard 的两极性单元测试。
//
// 这道门挂在两条公开的安装引导路由上（见 router/admin_routes.go），
// 它必须同时满足两件事，缺一件就算坏：
//   - 装完之后再来一发要弹回 403（否则任何能摸到端口的人都能给自己铸一个超管）；
//   - 还没装完时必须放行（否则新装的用户被自己的守卫锁在向导外面，这扇门就成了拒装服务）。
//
// 只碰 install.lock：探针显式置 nil，所以"未初始化"这一侧不会回查数据库，
// 用例在有库没库的机器上都一样跑。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hivemtk-user/internal/system/install"

	"github.com/gin-gonic/gin"
)

// pinInitializedLock 把 INSTALL_LOCK_PATH 指进临时目录并写一份"已装完"的 lock。
// 顺序要紧：install.Save 每次现读生效路径，先设 env 才会落进这份临时文件。
func pinInitializedLock(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "install.lock")
	t.Setenv("INSTALL_LOCK_PATH", path)
	install.SetAdminProbe(nil)
	t.Cleanup(func() { install.SetAdminProbe(nil) })
	if err := install.Save(&install.Lock{
		InstallID:     "ins-guard-test",
		AdminUsername: "admin",
		Initialized:   true,
		Version:       "test",
	}); err != nil {
		t.Fatalf("写测试 install.lock 失败: %v", err)
	}
}

// pinMissingLock 把 INSTALL_LOCK_PATH 指到临时目录里一份还不存在的 lock。
func pinMissingLock(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "install.lock")
	t.Setenv("INSTALL_LOCK_PATH", path)
	install.SetAdminProbe(nil)
	t.Cleanup(func() { install.SetAdminProbe(nil) })
	if install.GetStatus().Initialized {
		t.Fatal("前置不成立：没有 lock 文件时应判为未初始化")
	}
}

// postGuardedStub 用守卫 + 一个"会建号"的替身 handler 组一台最小引擎，打一次并返回结果
func postGuardedStub(t *testing.T) (int, string, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	reached := false
	engine.POST("/api/system/init-admin", SetupOnlyGuard(), func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "超管创建成功")
	})
	req := httptest.NewRequest(http.MethodPost, "/api/system/init-admin", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w.Code, w.Body.String(), reached
}

// TestSetupOnlyGuard_BlocksAfterInitialization 已初始化：守卫必须把请求拦在 handler 之前。
func TestSetupOnlyGuard_BlocksAfterInitialization(t *testing.T) {
	pinInitializedLock(t)

	code, body, reached := postGuardedStub(t)
	if code != http.StatusForbidden {
		t.Fatalf("已初始化时公开引导路由应回 403，实际 %d body=%s", code, body)
	}
	if reached {
		t.Fatalf("请求走到了 handler（那一侧还有建号逻辑），body=%s", body)
	}
	if !strings.Contains(body, "禁止重复创建超管") {
		t.Fatalf("403 要说清是重复初始化被拒，实际 body=%s", body)
	}
	if strings.Contains(body, `"code":0`) {
		t.Fatalf("响应里出现了成功语义：body=%s", body)
	}
}

// TestSetupOnlyGuard_OpensBeforeInitialization 反向一格：安装期必须还能用这条路由。
func TestSetupOnlyGuard_OpensBeforeInitialization(t *testing.T) {
	pinMissingLock(t)

	code, body, reached := postGuardedStub(t)
	if !reached {
		t.Fatalf("未初始化的实例被自己的守卫锁死了，新装用户进不了向导：code=%d body=%s", code, body)
	}
	if code != http.StatusOK {
		t.Fatalf("守卫放行后应由 handler 应答 200，实际 %d body=%s", code, body)
	}
}
