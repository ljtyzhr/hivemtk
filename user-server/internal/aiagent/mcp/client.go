// Package mcp — client.go P1-3：MCP 客户端侧。
//
// 我方此前只有 Server（对外暴露工具）；本文件补 client：连外部 MCP server，
// tools/list 发现 → 动态注册为 tooluse.Tool（Execute=JSON-RPC tools/call 转发）。
// server 不可达时熔断（连续失败 N 次后进入冷却，期间直接报错不发起网络请求），
// 不影响其他工具。
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
)

// ClientConfig MCP server 连接配置。
type ClientConfig struct {
	Name      string        // server 标识（工具名前缀 mcp_<name>_）
	Endpoint  string        // JSON-RPC HTTP(S) 端点
	APIKey    string        // 可选 Bearer
	Timeout   time.Duration // 单请求超时（默认 30s）
	FailLimit int           // 连续失败 N 次熔断（默认 3）
	Cooldown  time.Duration // 熔断冷却（默认 60s）
}

// Client MCP JSON-RPC 客户端。
type Client struct {
	cfg    ClientConfig
	client *http.Client
	nextID int64

	mu        sync.Mutex
	failCnt   int
	openUntil time.Time // 熔断开启截止时刻（零值=未开）
}

// NewClient 构造。
func NewClient(cfg ClientConfig) *Client {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.FailLimit <= 0 {
		cfg.FailLimit = 3
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 60 * time.Second
	}
	return &Client{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}, nextID: 1}
}

// rpcRequest JSON-RPC 2.0 请求。
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// call 单次 JSON-RPC 调用（带熔断判定）。
func (c *Client) call(ctx context.Context, method string, params any, out any) error {
	c.mu.Lock()
	if !c.openUntil.IsZero() && time.Now().Before(c.openUntil) {
		c.mu.Unlock()
		return fmt.Errorf("mcp %s 熔断中（冷却至 %s）", c.cfg.Name, c.openUntil.Format("15:04:05"))
	}
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	raw, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.recordFailure()
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.recordFailure()
		return err
	}
	if resp.StatusCode != http.StatusOK {
		c.recordFailure()
		return fmt.Errorf("mcp %s HTTP %d: %s", c.cfg.Name, resp.StatusCode, truncateStr(string(body), 200))
	}
	var rpcResp rpcResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		c.recordFailure()
		return fmt.Errorf("mcp %s 响应非 JSON-RPC: %w", c.cfg.Name, err)
	}
	if rpcResp.Error != nil {
		// 协议层业务错误不计入熔断（server 活着，是这次调用的事）
		return fmt.Errorf("mcp %s JSON-RPC error %d: %s", c.cfg.Name, rpcResp.Error.Code, rpcResp.Error.Message)
	}
	c.mu.Lock()
	c.failCnt = 0
	c.mu.Unlock()
	if out != nil && len(rpcResp.Result) > 0 {
		return json.Unmarshal(rpcResp.Result, out)
	}
	return nil
}

func (c *Client) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failCnt++
	if c.failCnt >= c.cfg.FailLimit {
		c.openUntil = time.Now().Add(c.cfg.Cooldown)
		c.failCnt = 0
	}
}

// ListTools 发现外部工具集。
func (c *Client) ListTools(ctx context.Context) ([]MCPTool, error) {
	var res ListToolsResult
	if err := c.call(ctx, "tools/list", map[string]any{}, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool 转发 tools/call。
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (string, bool, error) {
	var res CallToolResult
	if err := c.call(ctx, "tools/call", CallToolParams{Name: name, Arguments: args}, &res); err != nil {
		return "", true, err
	}
	var sb strings.Builder
	for _, b := range res.Content {
		if b.Type == "text" {
			sb.WriteString(b.Text)
		}
	}
	return sb.String(), res.IsError, nil
}

// mcpRemoteTool 外部 MCP 工具的 tooluse.Tool 适配。
type mcpRemoteTool struct {
	client *Client
	def    MCPTool
}

func (t *mcpRemoteTool) Name() string {
	return "mcp_" + sanitizeMCPToken(t.client.cfg.Name) + "_" + sanitizeMCPToken(t.def.Name)
}
func (t *mcpRemoteTool) Category() tooluse.ToolCategory { return tooluse.CategoryBusiness }
func (t *mcpRemoteTool) Description() string {
	return fmt.Sprintf("[MCP:%s] %s", t.client.cfg.Name, t.def.Description)
}

// Parameters inputSchema(map) → ToolParameters（尽力映射，未知形状退化为自由对象）。
func (t *mcpRemoteTool) Parameters() tooluse.ToolParameters {
	params := tooluse.ToolParameters{Type: "object"}
	if t.def.InputSchema == nil {
		return params
	}
	if v, ok := t.def.InputSchema["type"].(string); ok {
		params.Type = v
	}
	if raw, ok := t.def.InputSchema["properties"]; ok {
		if props, ok := raw.(map[string]any); ok {
			params.Properties = map[string]tooluse.ToolParam{}
			for k, pv := range props {
				pm, _ := pv.(map[string]any)
				tp := tooluse.ToolParam{}
				if pm != nil {
					if s, ok := pm["type"].(string); ok {
						tp.Type = s
					}
					if s, ok := pm["description"].(string); ok {
						tp.Description = s
					}
				}
				params.Properties[k] = tp
			}
		}
	}
	if raw, ok := t.def.InputSchema["required"].([]any); ok {
		for _, r := range raw {
			if s, ok := r.(string); ok {
				params.Required = append(params.Required, s)
			}
		}
	}
	return params
}

// Execute 转发 tools/call。
func (t *mcpRemoteTool) Execute(ctx context.Context, args map[string]any) (tooluse.ToolResult, error) {
	text, isErr, err := t.client.CallTool(ctx, t.def.Name, args)
	if err != nil {
		return tooluse.ErrorResult(t.Name(), err), err
	}
	if isErr {
		err := fmt.Errorf("mcp 工具报告错误: %s", truncateStr(text, 300))
		return tooluse.ErrorResult(t.Name(), err), nil
	}
	return tooluse.SuccessResult(t.Name(), map[string]any{"text": text}), nil
}

func sanitizeMCPToken(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// DiscoverAndRegister 连接 server、发现工具并注册到 registry。
// server 不可达返回错误（调用方记日志，不影响其他工具注册）。
func DiscoverAndRegister(ctx context.Context, cfg ClientConfig, registry *tooluse.ToolRegistry) (int, error) {
	if registry == nil {
		return 0, fmt.Errorf("tool registry 为 nil")
	}
	c := NewClient(cfg)
	tools, err := c.ListTools(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, def := range tools {
		if def.Name == "" {
			continue
		}
		t := &mcpRemoteTool{client: c, def: def}
		if err := registry.Register(t); err != nil {
			continue // 重名等非致命，跳过
		}
		n++
	}
	return n, nil
}
