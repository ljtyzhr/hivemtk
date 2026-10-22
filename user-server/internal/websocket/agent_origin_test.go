package websocket

import (
	"net/http"
	"testing"
)

// checkAgentOrigin 以给定 Origin 头走一遍坐席 WS 升级器的 CheckOrigin。
// ALLOWED_WS_ORIGINS 置为哨兵白名单，隔离运行机上可能存在的 yaml/env 配置，
// 使「同源放行」与「白名单命中」两类放行可分别断言。
func checkAgentOrigin(t *testing.T, origin string, host string) bool {
	t.Helper()
	t.Setenv("ALLOWED_WS_ORIGINS", "http://allowed.test")
	req, err := http.NewRequest(http.MethodGet, "http://"+host+"/api/ws/agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return upgrader.CheckOrigin(req)
}

// 任务四 UI 拟人测试实测缺陷：同源部署（前端与 user-server 同一反向代理层，
// VITE_API_BASE_URL=/）下，坐席实时通知 WS 的 Origin 是前端站点域，而
// ALLOWED_WS_ORIGINS 没有任何部署脚本配过 ⇒ WS 恒 403，坐席页面控制台
// 持续报 WebSocket 连接失败并陷入重连循环。
// 同源必须放行；跨源仍必须走白名单，冒牌域名仍必须拒绝。
func TestAgentCheckOrigin_SameOriginAllowed(t *testing.T) {
	allow := []string{
		"http://server.local",  // 与请求 Host 完全相同
		"https://server.local", // TLS 由反代终止，上游请求 scheme 是 http，只比 host
	}
	for _, o := range allow {
		if !checkAgentOrigin(t, o, "server.local") {
			t.Errorf("同源应放行 Origin=%q", o)
		}
	}

	deny := []string{
		"https://server.local.evil.com", // 后缀冒牌
		"https://evil.server.local",     // 前缀冒牌
		"http://other.local",            // 其他域
		"http://allowed.test",           // 白名单命中（本用例另有断言）
	}
	for _, o := range deny {
		if o == "http://allowed.test" {
			continue
		}
		if checkAgentOrigin(t, o, "server.local") {
			t.Errorf("跨源冒牌应拒绝 Origin=%q", o)
		}
	}
}

// 白名单命中的跨源仍然放行（显式配置优先，不被同源规则改变）。
func TestAgentCheckOrigin_WhitelistStillAllowed(t *testing.T) {
	if !checkAgentOrigin(t, "http://allowed.test", "server.local") {
		t.Error("白名单命中的 Origin 应放行")
	}
}

// 无 Origin 头（非浏览器客户端）放行。
func TestAgentCheckOrigin_NoOriginAllowed(t *testing.T) {
	if !checkAgentOrigin(t, "", "server.local") {
		t.Error("无 Origin 应放行")
	}
}
