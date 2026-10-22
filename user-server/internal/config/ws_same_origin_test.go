package config

import (
	"net/http"
	"net/url"
	"testing"
)

// 任务四 UI 拟人测试实测缺陷：同源部署下 WS 握手带的 Origin 是前端站点域，
// 白名单按部署域维护且没有部署脚本配过 ⇒ 同源被 403 拒绝。
// IsSameOriginWS 必须把同源判出来，跨源/伪造域名判 false。
func TestIsSameOriginWS(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://work.example.com/api/ws/agent", nil)
	if err != nil {
		t.Fatal(err)
	}

	same := []string{
		"http://work.example.com",
		"https://work.example.com", // scheme 不可知（无 XFP、非 TLS）⇒ 只比主机名
		"HTTP://WORK.EXAMPLE.COM",  // 主机名大小写不敏感
	}
	for _, o := range same {
		if !IsSameOriginWS(o, req) {
			t.Errorf("同源应判定为 true: origin=%q", o)
		}
	}

	diff := []string{
		"",
		"http://other.example.com",
		"https://work.example.com.evil.com",
		"https://evil.work.example.com",
		"not-a-url",
	}
	for _, o := range diff {
		if IsSameOriginWS(o, req) {
			t.Errorf("非同源应判定为 false: origin=%q", o)
		}
	}

	// 缺 Host 的请求无法判定同源
	noHost := &http.Request{URL: &url.URL{Scheme: "http", Host: "work.example.com"}}
	if IsSameOriginWS("http://work.example.com", noHost) {
		t.Error("请求缺 Host 时不应判定为同源")
	}
	if IsSameOriginWS("http://work.example.com", nil) {
		t.Error("请求为 nil 时不应判定为同源")
	}
}

// 反代形态（nginx 模板：proxy_set_header Host $host + X-Forwarded-Proto $scheme）
// 下 Host 头**没有端口**（$host 去端口），而浏览器发的 Origin 带端口。
// 这种部署下同源必须仍然放行 —— 否则要修的 403 在非默认端口上原样复发。
func TestIsSameOriginWS_BehindProxyWithPort(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://work.example.com/api/ws/agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "work.example.com" // nginx $host：去端口
	req.Header.Set("X-Forwarded-Proto", "https")

	if !IsSameOriginWS("https://work.example.com:8443", req) {
		t.Error("反代去端口形态下，同源（Origin 带端口）应放行")
	}
	if !IsSameOriginWS("https://work.example.com", req) {
		t.Error("反代去端口形态下，同源（Origin 不带端口）应放行")
	}
	// scheme 已知为 https ⇒ http 的 Origin 属于降级冒充，必须拒
	if IsSameOriginWS("http://work.example.com:8443", req) {
		t.Error("scheme 已知时，跨 scheme 的同主机 Origin 应拒绝")
	}
	// 主机名不同 ⇒ 拒
	if IsSameOriginWS("https://evil.example.com:8443", req) {
		t.Error("不同主机名应拒绝")
	}
}

// 直连 Go（明文，无 X-Forwarded-Proto）时 scheme 不可知，只比主机名；
// 端口差异（同主机不同端口）在这层不可知，只按主机名放行。
func TestIsSameOriginWS_DirectPlainHost(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:8214/api/ws/agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSameOriginWS("http://127.0.0.1:8214", req) {
		t.Error("直连形态下同源应放行")
	}
	if IsSameOriginWS("http://127.0.0.1:9999", req) {
		t.Error("同主机不同端口在直连形态下应拒绝（Host 头带端口可比）")
	}
	if IsSameOriginWS("http://other.local:8214", req) {
		t.Error("不同主机名应拒绝")
	}
}

// RequestScheme 的三态口径
func TestRequestScheme(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://x.local/a", nil)
	if s, known := RequestScheme(r); known || s != "http" {
		t.Errorf("无 XFP 非 TLS 时应为 (http,false)，实际 (%q,%v)", s, known)
	}

	r.Header.Set("X-Forwarded-Proto", "https")
	if s, known := RequestScheme(r); !known || s != "https" {
		t.Errorf("XFP=https 时应为 (https,true)，实际 (%q,%v)", s, known)
	}

	r.Header.Set("X-Forwarded-Proto", "https,http")
	if s, known := RequestScheme(r); !known || s != "https" {
		t.Errorf("链式 XFP 应取首段，实际 (%q,%v)", s, known)
	}

	if s, known := RequestScheme(nil); known || s != "http" {
		t.Errorf("nil 请求应为 (http,false)，实际 (%q,%v)", s, known)
	}
}
