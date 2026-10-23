// llm_proxy_sign_test.go `/llm-api/v1` 请求签名的跨端口径锁（SaaS 审计 A19）。
//
// # 为什么这组用例要用"硬编码向量"而不只是自己算一遍
//
// 签名口径是**跨仓契约**：平台端 `internal/middleware/merchant_llm_auth.go` 算一遍，
// 租户端 `internal/platform/llm_proxy_client.go` 算一遍。如果两侧都"按自己的公式算、
// 再拿自己的公式验"，任何一次口径改动都会同改同绿 —— 那种用例证明的是自洽，不是一致。
//
// 所以这里钉一个**固定的期望十六进制**（下面的 vectorSig）：它是一次性手算出来的常量。
// 任何一端的公式漂移（少签 query、把 body 直接拼进去、换掉域前缀）都会让它红，
// 而红线会同时出现在两侧的仓里。
//
// 平台端 counterpart：platform-server/internal/middleware/merchant_llm_auth_test.go
// 的 TestLLMProxySignatureContractVector（同一组输入、同一个期望值）。
package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/config"
)

// 契约向量（两侧逐字一致；改这里必须同时改平台端同名字面量）
const (
	vectorSecret = "merchant-secret-vector"
	vectorMethod = "POST"
	vectorURI    = "/llm-api/v1/chat/completions?provider=openai&tenant_code=t1"
	vectorTS     = "1700000000"
	vectorNonce  = "0123456789abcdef0123456789abcdef"
	vectorSig    = "3ff32cbdab7edb30d1535c5f451c9191e5d55eee1e28976f312850437c50a676"
)

var vectorBody = []byte(`{"model":"gpt-x","messages":[{"role":"user","content":"hi"}]}`)

// TestLLMProxySignatureMatchesPlatformContract 钉住跨端签名公式。
func TestLLMProxySignatureMatchesPlatformContract(t *testing.T) {
	got := LLMProxySignature(vectorSecret, vectorMethod, vectorURI, vectorTS, vectorNonce, vectorBody)
	if got != vectorSig {
		t.Fatalf("签名口径与平台端契约不一致：\n  期望 %s\n  实际 %s\n"+
			"常见漂移：签了 path 没签 query、把 body 原文拼进载荷而不是 sha256、域前缀不是 llm-proxy-v1",
			vectorSig, got)
	}
}

// TestSignLLMProxyRequestAddsVerifiableHeaders 证明真正发出去的请求带上了可验的签名头。
func TestSignLLMProxyRequestAddsVerifiableHeaders(t *testing.T) {
	t.Setenv("MERCHANT_API_SECRET", "env-fallback-secret")
	// 空的状态目录 ⇒ 文件那一档不存在，这条腿测的是 env 档
	t.Setenv("MERCHANT_STATE_DIR", t.TempDir())

	body := []byte(`{"model":"gpt-x"}`)
	req := httptest.NewRequest(http.MethodPost, "/llm-api/v1/chat/completions?provider=openai", nil)
	if err := signLLMProxyRequest(req, body); err != nil {
		t.Fatalf("签名失败: %v", err)
	}

	ts, nonce, sig := req.Header.Get("X-Timestamp"), req.Header.Get("X-Nonce"), req.Header.Get("X-Signature")
	if ts == "" || nonce == "" || sig == "" {
		t.Fatalf("签名头缺失：ts=%q nonce=%q sig=%q", ts, nonce, sig)
	}
	if len(nonce) != 32 {
		t.Errorf("nonce 应是 16 字节 hex（32 字符），实际 %q", nonce)
	}
	// 用**平台侧的口径**重算一遍并比对（请求 URI 必须含 query —— 这正是 tenant_code 不被换掉的前提）
	want := LLMProxySignature("env-fallback-secret", http.MethodPost, req.URL.RequestURI(), ts, nonce, body)
	if sig != want {
		t.Fatalf("签名与请求实际发出的 URI 不一致（query 必须进签名）：\n  期望 %s\n  实际 %s", want, sig)
	}
}

// TestSignLLMProxyRequestUsesPerMerchantSecretFirst 证明解析链的第一档（平台开户时下发、
// 注册时落盘的 per-merchant 文件密钥）优先于 env 回退 —— 否则平台换发密钥后本机仍用
// 旧的全局值签名，症状是"注册成功了但 LLM 代理恒 401"。
func TestSignLLMProxyRequestUsesPerMerchantSecretFirst(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MERCHANT_API_SECRET", "global-fallback")
	t.Setenv("MERCHANT_STATE_DIR", dir)
	if err := saveMerchantSecret("per-merchant-value"); err != nil {
		t.Fatalf("写 per-merchant secret 文件失败: %v", err)
	}

	if got := GetMerchantSecret(); got != "per-merchant-value" {
		t.Fatalf("解析链第一档（文件）应命中 per-merchant secret，实际 %q", got)
	}

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/llm-api/v1/chat/completions?provider=x", nil)
	if err := signLLMProxyRequest(req, body); err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	want := LLMProxySignature("per-merchant-value", http.MethodPost, req.URL.RequestURI(),
		req.Header.Get("X-Timestamp"), req.Header.Get("X-Nonce"), body)
	if got := req.Header.Get("X-Signature"); got != want {
		t.Fatalf("应使用 per-merchant secret 签名，实际签名不是它算出来的")
	}
}

// TestSignLLMProxyRequestWithoutSecretStillSends 无密钥时照发未签名请求（兼容窗口期），不阻断。
//
// 这条**不是**"允许不安全"：平台侧默认要求签名，未签名会被 401；这里只保证
// "没有密钥"不会变成"本地抛错"，否则排查方向会被引到本机代码而不是平台配置上。
func TestSignLLMProxyRequestWithoutSecretStillSends(t *testing.T) {
	t.Setenv("MERCHANT_API_SECRET", "")
	t.Setenv("MERCHANT_STATE_DIR", t.TempDir())
	// config.PlatformCfg 若被别的用例装配过，这里显式清空到"无密钥"态（用完还原）
	prevCfg := config.PlatformCfg
	config.PlatformCfg = nil
	t.Cleanup(func() { config.PlatformCfg = prevCfg })

	if got := GetMerchantSecret(); got != "" {
		t.Fatalf("前置条件不成立：期望无密钥可签，实际解析出 %q", got)
	}
	req := httptest.NewRequest(http.MethodPost, "/llm-api/v1/chat/completions?provider=x", nil)
	if err := signLLMProxyRequest(req, []byte(`{}`)); err != nil {
		t.Fatalf("无密钥时不该让请求失败（会诱导排查本机而不是平台配置）: %v", err)
	}
	if sig := req.Header.Get("X-Signature"); sig != "" {
		t.Fatalf("无密钥却发了签名头 %q —— 那会让平台按错密钥验签并回 401", sig)
	}
}
