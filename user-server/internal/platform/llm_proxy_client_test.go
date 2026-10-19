package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// LLM 代理调用「租户归属参数」契约测试（平台端按 (merchant_key, tenant_code)
// 每租户一行记账）：
//  1. meta.TenantCode 非空 → query 必须带 tenant_code=<code>（与
//     provider/scenario/trace_id/agent_id/user_id 同款 query 风格）；
//  2. meta.TenantCode 为空 → 该参数不上行，请求照发不报错（平台旧行为兜底）。
//
// 捕获经带缓冲 channel 回传（对齐 client_test.go refusalServer 的理由）：handler
// 跑在服务器 goroutine 上，共享变量会被 -race 判竞态。

func newChatStub(t *testing.T) (*httptest.Server, chan string) {
	t.Helper()
	queryCh := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/llm-api/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		select {
		case queryCh <- r.URL.RawQuery:
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, queryCh
}

// TestChatCompletionsSendsTenantCodeQuery 代理调用 query 必须带 tenant_code。
func TestChatCompletionsSendsTenantCodeQuery(t *testing.T) {
	srv, queryCh := newChatStub(t)
	c := SetLLMProxyClientForTest(srv.URL, "mk-test", srv.Client())

	resp, err := c.ChatCompletions(context.Background(), "plat-p", []byte(`{"model":"m"}`), LLMCallMeta{
		Scenario:   "sales",
		TraceID:    "tr-1",
		AgentID:    "a1",
		UserID:     "u1",
		TenantCode: "tABC123",
	})
	if err != nil {
		t.Fatalf("ChatCompletions: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, resp.Body)
	}

	q, err := url.ParseQuery(<-queryCh)
	if err != nil {
		t.Fatalf("解析 query: %v", err)
	}
	if got := q.Get("tenant_code"); got != "tABC123" {
		t.Errorf("query tenant_code 应为 tABC123，实际 %q", got)
	}
	// 同款风格的既有参数不许被这次改动挤掉
	if q.Get("provider") != "plat-p" || q.Get("scenario") != "sales" ||
		q.Get("trace_id") != "tr-1" || q.Get("agent_id") != "a1" || q.Get("user_id") != "u1" {
		t.Errorf("既有 query 参数被改动: %v", q)
	}
}

// TestChatCompletionsEmptyTenantCodeOmitsParam 取不到租户归属（空串）时
// tenant_code 不上行，请求照发不报错（平台旧行为兜底）。
func TestChatCompletionsEmptyTenantCodeOmitsParam(t *testing.T) {
	srv, queryCh := newChatStub(t)
	c := SetLLMProxyClientForTest(srv.URL, "mk-test", srv.Client())

	if _, err := c.ChatCompletions(context.Background(), "plat-p", []byte(`{"model":"m"}`), LLMCallMeta{
		Scenario: "sales",
	}); err != nil {
		t.Fatalf("空 tenant_code 的 ChatCompletions 不应报错: %v", err)
	}

	q, err := url.ParseQuery(<-queryCh)
	if err != nil {
		t.Fatalf("解析 query: %v", err)
	}
	if _, exists := q["tenant_code"]; exists {
		t.Errorf("空 tenant_code 应不上行，实际 %v", q["tenant_code"])
	}
	if q.Get("provider") != "plat-p" {
		t.Errorf("provider 参数丢失: %v", q)
	}
}
