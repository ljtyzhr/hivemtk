package ragretrieval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestInferRerankProtocol Protocol 为空时按 base 特征推断（P2-3 覆盖结构复用同一函数）
func TestInferRerankProtocol(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		// DashScope/MaaS 裸域名 → dashscope（裸域名自愈）
		{"https://dashscope.aliyuncs.com", dashscopeProtocol},
		{"https://dashscope.aliyuncs.com/", dashscopeProtocol},
		{"https://dashscope.aliyuncs.com/compatible-mode/v1", dashscopeProtocol}, // 经典域名无可用兼容 rerank 端点
		{"https://ws-abc.cn-beijing.maas.aliyuncs.com", dashscopeProtocol},
		// 已是原生完整端点 → dashscope
		{"https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank", dashscopeProtocol},
		// 工作空间域名 + 兼容前缀 → openai（官方兼容端点）
		{"https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1", "openai"},
		{"https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-mode/v1", "openai"},
		// 非阿里云域名（TEI / llama.cpp / 自建代理）→ openai，历史行为不变
		{"http://127.0.0.1:8209", "openai"},
		{"http://127.0.0.1:8209/v1", "openai"},
		{"http://mtk-rerank:8209/v1", "openai"},
		{"http://tei:8080", "openai"},
	}
	for _, c := range cases {
		if got := InferRerankProtocol(c.base); got != c.want {
			t.Errorf("InferRerankProtocol(%q)=%q want %q", c.base, got, c.want)
		}
	}
}

// TestNormalizeRerankConfig_BareDashscopeHealed P1-4①：
// protocol=dashscope 且 base 是裸域名（path 为空/不含 /rerank）→ 自动补全原生端点
func TestNormalizeRerankConfig_BareDashscopeHealed(t *testing.T) {
	cfg := &RerankConfig{Protocol: "dashscope", BaseURL: "https://dashscope.aliyuncs.com"}
	NormalizeRerankConfig(cfg, false)
	want := "https://dashscope.aliyuncs.com" + dashscopeNativeRerankPath
	if cfg.BaseURL != want {
		t.Errorf("裸域名应补全为原生端点: got %s want %s", cfg.BaseURL, want)
	}
	if !isDashscopeProtocol(cfg.Protocol) {
		t.Errorf("protocol 应保持 dashscope, got %s", cfg.Protocol)
	}

	// /api/v1 结尾：追加后半段而非整段重置
	cfg2 := &RerankConfig{Protocol: "dashscope", BaseURL: "https://dashscope.aliyuncs.com/api/v1"}
	NormalizeRerankConfig(cfg2, false)
	want2 := "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
	if cfg2.BaseURL != want2 {
		t.Errorf("/api/v1 应追加端点: got %s want %s", cfg2.BaseURL, want2)
	}

	// 已是完整原生端点：幂等不变
	full := "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"
	cfg3 := &RerankConfig{Protocol: "dashscope", BaseURL: full}
	NormalizeRerankConfig(cfg3, false)
	if cfg3.BaseURL != full {
		t.Errorf("完整端点不应被改写: got %s", cfg3.BaseURL)
	}
}

// TestNormalizeRerankConfig_OpenaiDashscopeHostHealed P1-4①：
// protocol=openai 却指向 DashScope → 自愈改走 dashscope 分支（原生端点）
func TestNormalizeRerankConfig_OpenaiDashscopeHostHealed(t *testing.T) {
	cases := []struct {
		name string
		base string
	}{
		{"经典域名裸 base", "https://dashscope.aliyuncs.com"},
		{"经典域名兼容前缀（reranks 404）", "https://dashscope.aliyuncs.com/compatible-mode/v1"},
		{"工作空间域名裸 base", "https://ws-abc.cn-beijing.maas.aliyuncs.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := &RerankConfig{Protocol: "openai", BaseURL: c.base}
			NormalizeRerankConfig(cfg, false)
			if !isDashscopeProtocol(cfg.Protocol) {
				t.Errorf("应自愈为 dashscope, got %s", cfg.Protocol)
			}
			if !strings.Contains(cfg.BaseURL, dashscopeNativeRerankPath) {
				t.Errorf("应补全原生端点, got %s", cfg.BaseURL)
			}
		})
	}

	// 工作空间域名 + 兼容前缀：显式 openai 走官方兼容端点，不自愈
	compat := &RerankConfig{Protocol: "openai", BaseURL: "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1"}
	NormalizeRerankConfig(compat, false)
	if compat.Protocol != "openai" {
		t.Errorf("兼容前缀不应改协议, got %s", compat.Protocol)
	}
	if compat.BaseURL != "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1" {
		t.Errorf("兼容 base 不应被改写, got %s", compat.BaseURL)
	}

	// Protocol 为空 → 按 base 推断（复用 InferRerankProtocol）
	inferred := &RerankConfig{BaseURL: "https://dashscope.aliyuncs.com"}
	NormalizeRerankConfig(inferred, false)
	if !isDashscopeProtocol(inferred.Protocol) {
		t.Errorf("空 protocol 应推断为 dashscope, got %s", inferred.Protocol)
	}
}

// TestResolveRerankEndpoint_ThreeBaseKinds 端点解析的三类 base 行为
// （TEI/llama.cpp 单数 /rerank 不受 DashScope 修正影响）
func TestResolveRerankEndpoint_ThreeBaseKinds(t *testing.T) {
	cases := []struct {
		name string
		cfg  *RerankConfig
		want string
	}{
		{
			name: "TEI 裸 host（历史行为：补 /v1 + 单数 /rerank）",
			cfg:  &RerankConfig{BaseURL: "http://tei-host:8080"},
			want: "http://tei-host:8080/v1/rerank",
		},
		{
			name: "llama.cpp base 已带 /v1（单数 /rerank）",
			cfg:  &RerankConfig{BaseURL: "http://127.0.0.1:8209/v1"},
			want: "http://127.0.0.1:8209/v1/rerank",
		},
		{
			name: "DashScope 兼容模式（复数 /reranks，官方端点）",
			cfg:  &RerankConfig{Protocol: "openai", BaseURL: "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1"},
			want: "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1/reranks",
		},
		{
			name: "DashScope 兼容模式 base 缺 /v1 时补齐",
			cfg:  &RerankConfig{Protocol: "openai", BaseURL: "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api"},
			want: "https://ws-abc.cn-beijing.maas.aliyuncs.com/compatible-api/v1/reranks",
		},
		{
			name: "DashScope 原生完整端点原样",
			cfg:  &RerankConfig{Protocol: "dashscope", BaseURL: "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank"},
			want: "https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank",
		},
		{
			name: "DashScope 裸域名 + openai：解析时兜底回原生（防 404）",
			cfg:  &RerankConfig{Protocol: "openai", BaseURL: "https://dashscope.aliyuncs.com"},
			want: "https://dashscope.aliyuncs.com" + dashscopeNativeRerankPath,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveRerankEndpoint(c.cfg); got != c.want {
				t.Errorf("endpoint=%s want %s", got, c.want)
			}
		})
	}
}

// TestRerankHTTPClientTimeoutFromConfig P2-1：cfg.Timeout 必须真正生效
// （历史实现 http.Client 硬编码 30s，配 60s 时被 client 截断）
func TestRerankHTTPClientTimeoutFromConfig(t *testing.T) {
	r := NewLocalRerankerWithConfig(&RerankConfig{BaseURL: "http://127.0.0.1:8209/v1", Timeout: 60})
	if r.httpClient.Timeout != 60*time.Second {
		t.Errorf("Timeout=60 应传导到 http.Client, got %v", r.httpClient.Timeout)
	}

	r2 := NewLocalRerankerWithConfig(&RerankConfig{BaseURL: "http://127.0.0.1:8209/v1", Timeout: 10})
	if r2.httpClient.Timeout != 30*time.Second {
		t.Errorf("更小的 cfg.Timeout 不放大默认值（client 下限 30s，实际由 ctx 控制）, got %v", r2.httpClient.Timeout)
	}

	r3 := NewLocalReranker()
	if r3.httpClient.Timeout != 30*time.Second {
		t.Errorf("默认重排器超时应保持 30s, got %v", r3.httpClient.Timeout)
	}
}

// TestRerank_NoRetryOnClientError P2-2：4xx（除 429）不重试，一次即返错
func TestRerank_NoRetryOnClientError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusNotFound} {
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
		}))

		cfg := &RerankConfig{
			BaseURL:    srv.URL + "/v1",
			Model:      "bge-reranker-v2-m3",
			Enabled:    true,
			Timeout:    5,
			MaxRetries: 3,
		}
		reranker := NewLocalRerankerWithConfig(cfg)
		_, err := reranker.Rerank(context.Background(), "q", []RerankDoc{{ID: "d0", Content: "c"}})
		srv.Close()

		if err == nil {
			t.Fatalf("status=%d 应返回错误", status)
		}
		if got := atomic.LoadInt32(&calls); got != 1 {
			t.Errorf("status=%d 不应重试: calls=%d want 1", status, got)
		}
		if !strings.Contains(err.Error(), "status=") {
			t.Errorf("错误应携带状态码: %v", err)
		}
	}
}

// TestIsRetryableRerankErr 重试判定边界：4xx（除 429）不重试，5xx/429/网络错误可重试
func TestIsRetryableRerankErr(t *testing.T) {
	notRetryable := []int{400, 401, 403, 404, 422}
	for _, s := range notRetryable {
		err := &rerankStatusError{StatusCode: s, Body: "x"}
		if isRetryableRerankErr(err) {
			t.Errorf("status=%d 不应重试", s)
		}
	}
	retryable := []int{429, 500, 502, 503}
	for _, s := range retryable {
		err := &rerankStatusError{StatusCode: s, Body: "x"}
		if !isRetryableRerankErr(err) {
			t.Errorf("status=%d 应可重试", s)
		}
	}
	// 无状态码（网络错误/超时/解码失败）→ 可重试
	if !isRetryableRerankErr(context.DeadlineExceeded) {
		t.Error("网络/超时错误应可重试")
	}
}
