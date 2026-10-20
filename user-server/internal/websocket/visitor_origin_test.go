package websocket

import (
	"net/http"
	"testing"
)

// checkVisitorOrigin 以给定 Origin 头走一遍 upgraderVisitor.CheckOrigin。
// ALLOWED_WS_ORIGINS 置为哨兵白名单，隔离运行机上可能存在的 yaml/env 配置，
// 使"配置白名单命中"与"私网精确匹配命中"两类放行可分别断言。
func checkVisitorOrigin(t *testing.T, origin string) bool {
	t.Helper()
	t.Setenv("ALLOWED_WS_ORIGINS", "http://allowed.test")
	req, err := http.NewRequest(http.MethodGet, "http://server.local/api/ws/visitor", nil)
	if err != nil {
		t.Fatal(err)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	return upgraderVisitor.CheckOrigin(req)
}

// 同源放行：嵌入页 Origin 与请求 Host 同域时必须能连（白名单漏配新部署域
// 不应把 iframe 自己的 WS 也拦掉）。反代终止 TLS 后上游请求是 http，故只比
// host 不比 scheme。跨域冒牌域名仍必须落到白名单/私网判定并拒绝。
func TestVisitorCheckOrigin_SameOriginAllowed(t *testing.T) {
	allow := []string{
		"http://server.local",  // 与请求 Host 相同
		"https://server.local", // TLS 由反代终止，上游请求 scheme 是 http
	}
	for _, o := range allow {
		if !checkVisitorOrigin(t, o) {
			t.Errorf("同源应放行 Origin=%q", o)
		}
	}

	deny := []string{
		"https://server.local.evil.com",
		"https://evil.server.local",
		"https://server.localhost",
	}
	for _, o := range deny {
		if checkVisitorOrigin(t, o) {
			t.Errorf("非同源且不在白名单时应拒绝 Origin=%q", o)
		}
	}
}

func TestVisitorCheckOrigin_PrivateHostExactMatch(t *testing.T) {
	allow := []string{
		"", // 非浏览器客户端不带 Origin：按设计放行
		"http://allowed.test",
		"http://localhost",
		"http://localhost:5173",
		"https://localhost:3000",
		"http://127.0.0.1:8080",
		"http://10.1.2.3",
		"http://192.168.31.5:9000",
		"http://172.20.1.1",
		"http://[::1]:3000",
	}
	for _, o := range allow {
		if !checkVisitorOrigin(t, o) {
			t.Errorf("应放行 Origin=%q", o)
		}
	}

	deny := []string{
		// 冒牌前缀域名：旧 HasPrefix 实现全部放行（本轮收口的真实伪造面）
		"http://localhost.attacker.tld",
		"http://10.evil.com",
		"http://192.168.evil.com",
		"http://127.0.0.1.evil.com",
		// 172. 前缀旧实现放行整个 172/8，公网 IP 也过
		"http://172.99.99.99",
		"https://10.0.0.99.evil.cn",
		// 非 http(s) 协议与畸形值
		"ftp://localhost",
		"http://",
		"not a url",
		"http://8.8.8.8",
		"https://evil.example.org",
	}
	for _, o := range deny {
		if checkVisitorOrigin(t, o) {
			t.Errorf("应拒绝 Origin=%q", o)
		}
	}
}
