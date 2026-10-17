package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/config"
)

// LLMProxyClient 平台 LLM 代理客户端
//
// ============================================================
// 为什么是代理而不是直连
//
// 平台垫资的真实厂商 Key **永不下发**（plan.md ADR-M7）：下发等于把垫资凭据
// 交出去，商户可以直连厂商无限量用，平台既无法计量也无法止损。
// 所以调用链是 user-server → 平台代理 → 厂商，扣费由平台在转发时实时执行。
//
// ===== 零出域铁律的例外边界 =====
// 这是本项目「数据零出域」的唯一显式例外，成立条件（缺一不可）：
//  1. 商户在平台端开通了 llm-cloud 并买了套餐（= 显式同意）；
//  2. 代理地址是商户自己配置的（PLATFORM_API_URL），平台不主动外联；
//  3. 只有 source='platform' 的 provider 会走这条路；自带 Key 的 provider
//     一次都不会调到这里。
// ============================================================

// LLMProxyResponse 代理响应（含平台回传的真实用量）
type LLMProxyResponse struct {
	StatusCode       int
	Body             []byte
	ContentType      string
	PromptTokens     int64
	CompletionTokens int64
	RequestID        string
	Provider         string
}

// LLMProxyClient 平台模型代理客户端
type LLMProxyClient struct {
	baseURL string
	key     string
	client  *http.Client
}

// NewLLMProxyClient 构造代理客户端
func NewLLMProxyClient() *LLMProxyClient {
	return &LLMProxyClient{
		baseURL: strings.TrimRight(platformURLForLLM(), "/"),
		key:     merchantKeyForLLM(),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// SetLLMProxyClientForTest 测试注入（避免单测去读真实 env）
var SetLLMProxyClientForTest = func(baseURL, key string, c *http.Client) *LLMProxyClient {
	return &LLMProxyClient{baseURL: strings.TrimRight(baseURL, "/"), key: key, client: c}
}

func platformURLForLLM() string {
	if v := strings.TrimSpace(os.Getenv("PLATFORM_API_URL")); v != "" {
		return v
	}
	// 复用 config.PlatformURL()：它是平台地址的唯一事实源
	// （已装配配置 > env > 空串），另起一份解析迟早与它漂移。
	return config.PlatformURL()
}

func merchantKeyForLLM() string {
	if v := strings.TrimSpace(os.Getenv("PLATFORM_MERCHANT_KEY")); v != "" {
		return v
	}
	if GetMerchantKey() != "" {
		return GetMerchantKey()
	}
	return "local-dev-merchant"
}

// Available 平台是否可用（未配置地址/密钥时返回 false，调用方据此把
// source='platform' 的 provider 降级为不可用，而不是发一次注定失败的请求）
func (c *LLMProxyClient) Available() bool {
	return c.baseURL != "" && c.key != ""
}

// ChatCompletions 转发一次对话补全
//
// 报文的 provider/scenario/trace 等上下文放在 **query** 而不是 body：
// body 是 OpenAI 协议原文，往里塞我们自己的字段会让不支持的厂商直接 400；
// 而 query 不进协议、也不会被厂商解析。
func (c *LLMProxyClient) ChatCompletions(ctx context.Context, provider string, body []byte, meta LLMCallMeta) (*LLMProxyResponse, error) {
	if !c.Available() {
		return nil, fmt.Errorf("平台未配置（PLATFORM_API_URL / PLATFORM_MERCHANT_KEY 缺失），无法调用平台模型")
	}
	if provider == "" {
		return nil, fmt.Errorf("provider 不能为空")
	}
	q := []string{"provider=" + urlQueryEscape(provider)}
	if meta.Scenario != "" {
		q = append(q, "scenario="+urlQueryEscape(meta.Scenario))
	}
	if meta.TraceID != "" {
		q = append(q, "trace_id="+urlQueryEscape(meta.TraceID))
	}
	if meta.AgentID != "" {
		q = append(q, "agent_id="+urlQueryEscape(meta.AgentID))
	}
	if meta.UserID != "" {
		q = append(q, "user_id="+urlQueryEscape(meta.UserID))
	}
	target := c.baseURL + "/llm-api/v1/chat/completions?" + strings.Join(q, "&")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("构造平台代理请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	if meta.TraceID != "" {
		req.Header.Set("X-Trace-Id", meta.TraceID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用平台代理失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, fmt.Errorf("读取平台代理响应失败: %w", err)
	}

	out := &LLMProxyResponse{
		StatusCode:  resp.StatusCode,
		Body:        raw,
		ContentType: resp.Header.Get("Content-Type"),
		RequestID:   resp.Header.Get("X-Llm-Usage-Request-Id"),
		Provider:    resp.Header.Get("X-Llm-Provider"),
	}
	out.PromptTokens = parseIntHeader(resp.Header.Get("X-Llm-Usage-Prompt-Tokens"))
	out.CompletionTokens = parseIntHeader(resp.Header.Get("X-Llm-Usage-Completion-Tokens"))
	return out, nil
}

// LLMCallMeta 一次调用的业务上下文（用于计量归因）
type LLMCallMeta struct {
	Scenario string
	TraceID  string
	AgentID  string
	UserID   string
}

func parseIntHeader(v string) int64 {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func urlQueryEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == '~':
			b.WriteRune(r)
		default:
			b.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return b.String()
}

// ParseChatCompletionUsage 从 OpenAI 兼容响应里取 usage（本地路径复用）。
//
// 与平台端 llm_proxy_service.parseUsage 同形：报文合法但 usage 全 0 时
// **不能**直接返回 0 —— 那会让「厂商省略 usage」的响应把计量悄悄变成 0，
// 商户用了算力却一直没扣费。
func ParseChatCompletionUsage(body []byte) (promptTokens, completionTokens int64) {
	var out struct {
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &out); err == nil {
		in, comp := out.Usage.PromptTokens, out.Usage.CompletionTokens
		if in <= 0 && comp <= 0 && out.Usage.TotalTokens > 0 {
			in = out.Usage.TotalTokens / 2
			comp = out.Usage.TotalTokens - in
		}
		if in > 0 || comp > 0 {
			return in, comp
		}
	}
	return 0, 0
}
