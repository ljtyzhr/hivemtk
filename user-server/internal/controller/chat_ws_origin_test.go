package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func makeOriginReq(origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws/chat", nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// TestBuildCheckOrigin_StrictMatch 验证严格匹配
func TestBuildCheckOrigin_StrictMatch(t *testing.T) {
	check := buildCheckOrigin([]string{"https://app.example.com", "https://admin.example.com"})

	if !check(makeOriginReq("https://app.example.com")) {
		t.Error("expected allow for whitelisted origin")
	}
	if !check(makeOriginReq("https://admin.example.com")) {
		t.Error("expected allow for whitelisted origin")
	}

	if check(makeOriginReq("https://evil.com")) {
		t.Error("expected reject for non-whitelisted origin")
	}
	if check(makeOriginReq("https://app.example.com.evil.com")) {
		t.Error("expected reject for subdomain spoofing")
	}
	if check(makeOriginReq("http://app.example.com")) {
		t.Error("expected reject for http vs https mismatch")
	}
}

// TestBuildCheckOrigin_EmptyOrigin 验证无 Origin 头默认放行
//
// 场景: 移动端 SDK、服务端、curl 测试通常不带 Origin 头
// 严格放行这些可避免误伤合法客户端
func TestBuildCheckOrigin_EmptyOrigin(t *testing.T) {
	check := buildCheckOrigin([]string{"https://app.example.com"})
	if !check(makeOriginReq("")) {
		t.Error("expected allow for empty Origin (non-browser client)")
	}
}

// TestBuildCheckOrigin_Wildcard 验证 "*" 通配符
func TestBuildCheckOrigin_Wildcard(t *testing.T) {
	check := buildCheckOrigin([]string{"*"})
	if !check(makeOriginReq("https://any.com")) {
		t.Error("wildcard should allow any origin")
	}
	if !check(makeOriginReq("http://random.evil.com")) {
		t.Error("wildcard should allow any origin (debug only)")
	}
}

// TestBuildCheckOrigin_EmptyList 验证空白名单行为
//
// 严格模式下, 空白名单应拒绝所有带 Origin 头的请求。
// 但无 Origin 头的请求仍放行 (兼容非浏览器)。
func TestBuildCheckOrigin_EmptyList(t *testing.T) {
	check := buildCheckOrigin([]string{})
	if !check(makeOriginReq("")) {
		t.Error("empty Origin should still be allowed (non-browser)")
	}
	if check(makeOriginReq("https://any.com")) {
		t.Error("with empty whitelist, non-empty origin should be rejected")
	}
}

// TestBuildCheckOrigin_NoExternalMutation 验证 buildCheckOrigin 不修改输入切片
func TestBuildCheckOrigin_NoExternalMutation(t *testing.T) {
	origins := []string{"https://a.com", "https://b.com"}
	_ = buildCheckOrigin(origins)
	if len(origins) != 2 || origins[0] != "https://a.com" {
		t.Errorf("input slice should not be mutated, got %v", origins)
	}
}

// TestBuildCheckOrigin_SameOriginAllowed 验证同源放行。
//
// 任务四 UI 拟人测试实测缺陷：同源部署（前端与 user-server 同一反向代理层）
// 下聊天流式 WS 的 Origin 是本站域，而白名单按部署域维护、没有任何部署脚本
// 配过 ⇒ 同源被 403，聊天流式输出直接不可用。跨源仍必须走白名单。
func TestBuildCheckOrigin_SameOriginAllowed(t *testing.T) {
	check := buildCheckOrigin([]string{"https://allowed.example.com"})

	// httptest.NewRequest 的 Host 为 example.com
	same := []string{
		"http://example.com",
		"https://example.com", // 反代终止 TLS，上游 scheme 是 http，只比 host
	}
	for _, o := range same {
		if !check(makeOriginReq(o)) {
			t.Errorf("同源应放行: origin=%q", o)
		}
	}

	// 跨源冒牌仍必须拒绝
	deny := []string{
		"https://example.com.evil.com",
		"https://evil.example.com",
		"https://other.com",
	}
	for _, o := range deny {
		if check(makeOriginReq(o)) {
			t.Errorf("跨源冒牌应拒绝: origin=%q", o)
		}
	}
}
