package controller

// 公开安装态口的脱敏腿。
//
// install.Status 里带着 install_id —— 它是这台实例在平台侧的商户身份主键，心跳上报
// 必须带它，所以字段不能从 DTO 上摘掉；但它同时是一个跨系统关联键，谁拿到就能把同一台
// 实例在不同上报里的记录对上，而 /api/system/init-status 是**安装向导之前就要能读**的
// 公开口（没有登录态可用）。两边只能靠 HTTP 层的响应形状分开。
//
// 这条用例因此两头都断：响应里既没有 install_id 这个键、也没有它的值；
// 同进程读 install.GetStatus() 又必须还能拿到那个值（证明脱敏发生在出口，
// 而不是有人把身份从真相源上删了——那种"修好"会让心跳静默失去商户身份）。

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/system/install"

	"github.com/gin-gonic/gin"
)

// gateTestInstallID 固定一枚测试身份值，不用随机：判据要能断言"这个值没出现在响应里"，
// 随机值只能断言键不存在，而泄漏走的是别名字段时键名会变、值不会。
const gateTestInstallID = "r71gateinstallid0123456789abcdef"

func gateWriteInstallLock(t *testing.T) {
	t.Helper()
	lock := filepath.Join(t.TempDir(), "install.lock")
	body := `{"install_id":"` + gateTestInstallID + `",` +
		`"install_time":"2026-01-01T00:00:00Z",` +
		`"admin_username":"gate-seed-admin",` +
		`"initialized":true,"version":"1.2.3"}`
	if err := os.WriteFile(lock, []byte(body), 0o600); err != nil {
		t.Fatalf("写测试 install.lock 失败: %v", err)
	}
	t.Setenv("INSTALL_LOCK_PATH", lock)
}

func gateReadInitStatus(t *testing.T) (int, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/system/init-status", NewSystemInitController().GetInitStatus)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/system/init-status", nil))
	return w.Code, w.Body.String()
}

func TestGetInitStatus_PublicBodyRedactsInstallID(t *testing.T) {
	gateWriteInstallLock(t)
	// 安装态检查器是无字段的单例（安装态全部就地读 install.lock），装配一次即可，
	// 没有需要还原的可变状态；未装配时 GetInstallStatus() 返回 nil，
	// 处理函数会走"检查器未初始化"的兜底分支，那条腿量不到脱敏。
	middleware.InitInstallStatus()

	code, body := gateReadInitStatus(t)
	if code != http.StatusOK {
		t.Fatalf("公开口应 200，实得 %d / %s", code, strings.TrimSpace(body))
	}
	// 正控制：这次响应确实读到了我写的那份安装态，
	// 否则"没有 install_id"可能只是因为状态压根没加载成功。
	if !strings.Contains(body, "INITIALIZED") || !strings.Contains(body, "1.2.3") {
		t.Fatalf("响应没反映出写入的安装态（state/version 缺席），脱敏断言无从谈起：%s",
			strings.TrimSpace(body))
	}
	if strings.Contains(body, "install_id") {
		t.Errorf("公开响应里仍有 install_id 字段：%s", strings.TrimSpace(body))
	}
	if strings.Contains(body, gateTestInstallID) {
		t.Errorf("公开响应里泄漏了安装身份值：%s", strings.TrimSpace(body))
	}

	// 反向对账：真相源仍带身份，心跳那侧不许被一起摘掉。
	if got := install.GetStatus().InstallID; got != gateTestInstallID {
		t.Errorf("install.GetStatus() 的 InstallID 不再是那枚测试身份（得到 %q）："+
			"若有人把字段从 DTO 上删了，公开口是干净了，心跳上报会静默丢商户身份", got)
	}
}

func TestGetInitStatus_PublicBodyKeepsClientFields(t *testing.T) {
	gateWriteInstallLock(t)
	middleware.InitInstallStatus()

	_, body := gateReadInitStatus(t)
	for _, key := range []string{`"state"`, `"initialized"`, `"has_admin"`, `"version"`} {
		if !strings.Contains(body, key) {
			t.Errorf("脱敏顺手摘掉了客户端在用的字段 %s：%s", key, strings.TrimSpace(body))
		}
	}
	if strings.Contains(body, "gate-seed-admin") {
		t.Errorf("公开响应回出了超管账号名（安装向导之外的读者不该知道）：%s", strings.TrimSpace(body))
	}
}
