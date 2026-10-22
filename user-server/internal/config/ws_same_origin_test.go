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
		"https://work.example.com", // 反代终止 TLS，上游 scheme 是 http，只比 host
		"HTTP://WORK.EXAMPLE.COM",  // host 大小写不敏感
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
		"http://work.example.com:8080", // 端口不同即非同源
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
