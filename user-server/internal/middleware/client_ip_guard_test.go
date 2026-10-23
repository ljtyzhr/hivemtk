package middleware

// A13（`未完成任务.md` §9）回归：可信反代名单可配 + 失配必须响亮。
//
// 修前形态：`r.SetTrustedProxies([]string{私网四条})` 写死在 main 里，且**配置面为零**。
// 反代/云 LB 落在公网地址上时，gin 不认它是可信代理 ⇒ `ClientIP()` 恒为反代出口地址：
// 全站共用一个限流桶、审计 IP 全写成同一个，且**没有任何报错**。
//
// 判据（本文件钉死四条）：
//  1. env 未设置 ⇒ 名单逐项等于修复前的硬编码值（升级零变更）；
//  2. env 设置 ⇒ 生效的是 env 的值（可配），非法条目**整体报错**而不是静默丢弃；
//  3. 对端可信 ⇒ 不告警（正常形态不许吵）；
//  4. 对端不可信却带了 XFF ⇒ 告警一次（同对端去重）且计数增加。

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTrustedProxiesFromEnv_DefaultUnchanged(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", "")
	list, fromEnv, err := TrustedProxiesFromEnv()
	if err != nil {
		t.Fatalf("未设置 env 不应报错: %v", err)
	}
	if fromEnv {
		t.Fatalf("未设置 env 时 fromEnv 应为 false")
	}
	want := []string{"127.0.0.0/8", "::1/128", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	if !reflect.DeepEqual(list, want) {
		t.Fatalf("默认名单必须与修复前硬编码值逐项一致，实际 %v", list)
	}
}

func TestTrustedProxiesFromEnv_OverrideAndReject(t *testing.T) {
	t.Setenv("TRUSTED_PROXIES", " 203.0.113.7 , 2001:db8::/32 ")
	list, fromEnv, err := TrustedProxiesFromEnv()
	if err != nil {
		t.Fatalf("合法 env 不应报错: %v", err)
	}
	if !fromEnv {
		t.Fatalf("设置 env 时 fromEnv 应为 true")
	}
	if len(list) != 2 || list[0] != "203.0.113.7" || list[1] != "2001:db8::/32" {
		t.Fatalf("env 名单应原样生效（含去空白），实际 %v", list)
	}

	// 非法条目：整体报错，不静默丢弃（丢掉一条的后果是"我明明配了却还是全站一个 IP"）
	t.Setenv("TRUSTED_PROXIES", "10.0.0.0/8,not-an-ip")
	if _, _, err := TrustedProxiesFromEnv(); err == nil {
		t.Fatalf("含非法条目必须报错，实际 nil")
	}
}

// TestClientIPGuard_WarnsOnlyWhenMismatched 对端可信不吵；对端不可信带 XFF 才吵（且只吵一次）。
func TestClientIPGuard_WarnsOnlyWhenMismatched(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var warned []string
	g := newClientIPGuard([]string{"10.0.0.0/8"}, func(peer, xff string) {
		warned = append(warned, peer+"|"+xff)
	})

	r := gin.New()
	r.Use(g.handler())
	r.GET("/x", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	do := func(remoteAddr, xff string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = remoteAddr
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		r.ServeHTTP(w, req)
	}

	// 1) 可信反代：正常形态，不许吵
	do("10.1.2.3:5555", "203.0.113.9")
	if len(warned) != 0 {
		t.Fatalf("对端在可信名单里不应告警，实际 %v", warned)
	}

	// 2) 无 XFF：直连客户端，与本门无关
	do("198.51.100.7:5555", "")
	if len(warned) != 0 {
		t.Fatalf("没有 X-Forwarded-For 的直连请求不应告警，实际 %v", warned)
	}

	// 3) 不可信对端 + XFF：这正是"反代不在名单里"的确定证据
	before := ClientIPMismatchCount()
	do("198.51.100.7:5555", "203.0.113.9")
	do("198.51.100.7:5555", "203.0.113.10") // 同一对端第二次：计数增，不再刷日志
	if got := ClientIPMismatchCount() - before; got != 2 {
		t.Fatalf("失配观测计数应 +2，实际 +%d", got)
	}
	if len(warned) != 1 {
		t.Fatalf("同一对端只应告警一次（防日志刷爆），实际 %d 次: %v", len(warned), warned)
	}
	if warned[0] != "198.51.100.7|203.0.113.9" {
		t.Fatalf("告警内容应为对端+原始的 XFF，实际 %q", warned[0])
	}

	// 4) 另一个不可信对端：各报一次
	do("198.51.100.8:5555", "203.0.113.11")
	if len(warned) != 2 {
		t.Fatalf("不同对端应各告警一次，实际 %v", warned)
	}
}

// TestConfigureTrustedProxies_WiresEngineAndGuard 装配入口：名单经 env 生效，
// 且引擎的 ClientIP 语义随之改变（可信对端读 XFF，不可信对端读对端地址）。
func TestConfigureTrustedProxies_WiresEngineAndGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "203.0.113.7")

	var warnings int
	SetClientIPGuardWarnHook(func(peer, xff string) { warnings++ })
	t.Cleanup(func() { SetClientIPGuardWarnHook(nil) })

	r := gin.New()
	r.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
	if _, err := ConfigureTrustedProxies(r); err != nil {
		// 这个路由先注册了也不用担心：SetTrustedProxies 只改引擎配置，不影响已注册路由的语义
		t.Fatalf("装配失败: %v", err)
	}

	get := func(remoteAddr string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/ip", nil)
		req.RemoteAddr = remoteAddr
		req.Header.Set("X-Forwarded-For", "198.51.100.42")
		r.ServeHTTP(w, req)
		return w.Body.String()
	}

	// 反代在名单里 ⇒ ClientIP 采信 XFF（真实客户端）
	if got := get("203.0.113.7:1234"); got != "198.51.100.42" {
		t.Fatalf("可信反代应采信 XFF，ClientIP=%q", got)
	}
	if warnings != 0 {
		t.Fatalf("可信反代不应告警，实际 %d 次", warnings)
	}

	// 反代不在名单里 ⇒ ClientIP 退化成反代出口地址（这正是 A5 之外另一种"全站一个桶"的成因），
	// 此时必须告警
	if got := get("198.51.100.200:1234"); got != "198.51.100.200" {
		t.Fatalf("不可信对端时 ClientIP 应为对端地址，实际 %q", got)
	}
	if warnings != 1 {
		t.Fatalf("不可信对端带 XFF 必须告警 1 次，实际 %d", warnings)
	}
}
