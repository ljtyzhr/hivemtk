// Package provider 外部内容审核 API 的适配层（L3 审查源）。
//
// 定位：**默关闭**。本仓的"数据零出域"约束（CLAUDE.md 规则4.3）要求任何新功能都不得
// 引入默认外发数据的路径；外部审核 API 恰恰是把用户原文发给第三方，所以它只能是
// 显式开启的增强项，而不是主链路的一环：
//   - 策略里 APIEnabled=false（默认）⇒ 本包一个请求都不会发；
//   - 即使开启，也只按 policy.APISampleRate 抽样送审，且失败一律降级为
//     "仅本地词库结论"，绝不让审查链路因外部不可用而卡住或全量放行/全量拦截。
//
// 契约（自家网关或云厂商代理都按这个形状对接，见 docs/architecture/
// CONTENT_MODERATION_RESEARCH.md 的厂商字段映射表）：
//
//	POST <endpoint>
//	{"scene":"inbound_message","subject_type":"user","subject_id":"u_1","content":"...","trace_id":"..."}
//	→ 200 {"code":0,"data":{"risk_level":2,"labels":["porn","ad"],"score":0.92,"request_id":"..."}}
//
// risk_level 口径与本地 severity 对齐（0 无风险 / 1 低 / 2 中 / 3 高），
// 便于服务层把两个来源的结论用同一套阈值比较。
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotConfigured 未配置（默认态）。
var ErrNotConfigured = errors.New("content guard api provider not configured")

// Request 送审请求。
type Request struct {
	TenantID    uint   `json:"tenant_id"`
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Scene       string `json:"scene"`
	Content     string `json:"content"`
	TraceID     string `json:"trace_id,omitempty"`
}

// Result 审核结论。RiskLevel 与本地 severity 同口径（0..3）。
type Result struct {
	RiskLevel int      `json:"risk_level"`
	Labels    []string `json:"labels"`
	Score     float64  `json:"score"`
	RequestID string   `json:"request_id"`
}

// Auditor 外部审核源接口（便于测试注入假实现与将来接 SDK）。
type Auditor interface {
	// Name 审查源名（写进 guard_records.sources）。
	Name() string
	Audit(ctx context.Context, req Request) (Result, error)
}

// HTTPConfig HTTP 审核源配置。
type HTTPConfig struct {
	// Endpoint 完整 URL（http/https）。https 之外一律要求 AllowInsecure 显式放行。
	Endpoint string
	// Token 可选 Bearer 令牌。
	Token string
	// AllowedHosts 主机白名单（host 或 host:port）。**非空才校验**；
	// 为空时只允许 https（即"要么给白名单，要么只能走 TLS 到任意主机"）。
	AllowedHosts []string
	// Timeout 单次调用超时，<=0 取 3s。
	Timeout time.Duration
	// MaxBodyBytes 响应体上限，<=0 取 64KB（防被上游拖死内存）。
	MaxBodyBytes int64
	// AllowInsecure 允许 http 与私网地址（仅本地联调/内网自建审核服务用）。
	AllowInsecure bool
}

// HTTPAuditor 通用 JSON-over-HTTP 审核源。
type HTTPAuditor struct {
	endpoint     string
	token        string
	allowedHosts map[string]bool
	client       *http.Client
	maxBody      int64
}

// NewHTTPAuditor 构造审核源；配置不合法在**构造期**报错，而不是等到线上第一条消息。
func NewHTTPAuditor(cfg HTTPConfig) (*HTTPAuditor, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, ErrNotConfigured
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("审核 API endpoint 非法: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("审核 API endpoint 协议不支持: %s", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("审核 API endpoint 缺少主机")
	}
	allowed := make(map[string]bool, len(cfg.AllowedHosts))
	for _, h := range cfg.AllowedHosts {
		if h = strings.TrimSpace(h); h != "" {
			allowed[strings.ToLower(h)] = true
		}
	}
	if len(allowed) > 0 && !allowed[strings.ToLower(u.Host)] {
		return nil, fmt.Errorf("审核 API 主机 %s 不在白名单内", u.Host)
	}
	if u.Scheme == "http" && !cfg.AllowInsecure {
		return nil, errors.New("审核 API 使用 http 需要显式 AllowInsecure")
	}
	if !cfg.AllowInsecure {
		if ip := net.ParseIP(u.Hostname()); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
			return nil, errors.New("审核 API 指向私网/回环地址，需要显式 AllowInsecure")
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	maxBody := cfg.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = 64 << 10
	}
	return &HTTPAuditor{
		endpoint:     cfg.Endpoint,
		token:        cfg.Token,
		allowedHosts: allowed,
		maxBody:      maxBody,
		client: &http.Client{
			Timeout: timeout,
			// 不跟随重定向：审核网关一旦被 302 到别处，就是把用户原文送到白名单之外。
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("审核 API 不允许重定向")
			},
		},
	}, nil
}

// Name 审查源名。
func (a *HTTPAuditor) Name() string { return "api" }

type apiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		RiskLevel int      `json:"risk_level"`
		Labels    []string `json:"labels"`
		Score     float64  `json:"score"`
		RequestID string   `json:"request_id"`
	} `json:"data"`
}

// Audit 调用外部审核。
func (a *HTTPAuditor) Audit(ctx context.Context, req Request) (Result, error) {
	if a == nil {
		return Result{}, ErrNotConfigured
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint, bytes.NewReader(body))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+a.token)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, a.maxBody))
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("审核 API 返回 %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Result{}, fmt.Errorf("审核 API 响应解析失败: %w", err)
	}
	if env.Code != 0 {
		return Result{}, fmt.Errorf("审核 API 业务失败 code=%d msg=%s", env.Code, env.Message)
	}
	risk := env.Data.RiskLevel
	if risk < 0 {
		risk = 0
	}
	if risk > 3 {
		risk = 3
	}
	return Result{
		RiskLevel: risk,
		Labels:    env.Data.Labels,
		Score:     env.Data.Score,
		RequestID: env.Data.RequestID,
	}, nil
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
