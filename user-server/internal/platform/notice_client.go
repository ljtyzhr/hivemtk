package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PlatformNotice 平台下发的一条站内消息。
type PlatformNotice struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Level       string    `json:"level"`
	PublishedAt time.Time `json:"published_at"`
}

// NoticeListResult 拉取结果。
type NoticeListResult struct {
	List       []PlatformNotice `json:"list"`
	ServerTime time.Time        `json:"server_time"`
}

// NoticeClient 平台站内消息的商户端客户端。
//
// 与 ModuleClient 同形：同一个 HMAC 签名通道（merchant-api 组），
// 同样的 query 传 tenant_code、同样的"签名 PATH 不含 query string"。
type NoticeClient struct {
	client *Client
}

// NewNoticeClient 构造通知客户端。
func NewNoticeClient() *NoticeClient {
	key := osGetenv("PLATFORM_MERCHANT_KEY")
	if key == "" {
		key = GetMerchantKey()
	}
	if key == "" {
		key = "local-dev-merchant"
	}
	return &NoticeClient{client: NewPlatformClient(key)}
}

// List 拉取面向指定租户的平台消息。
//
// afterID 是增量游标（0 = 全量拉取，首次同步用），语义是"已拉取到的最大
// 平台消息 ID"——平台消息 ID 单调递增，所以它天然覆盖"两次拉取之间新发的
// 消息"，不存在游标跳过某条的情况。
func (c *NoticeClient) List(ctx context.Context, tenantCode string, afterID int64) (*NoticeListResult, error) {
	path := "/merchant-api/notices/list?tenant_code=" + url.QueryEscape(tenantCode)
	if afterID > 0 {
		path += "&after_id=" + strconv.FormatInt(afterID, 10)
	}

	var out struct {
		List       []PlatformNotice `json:"list"`
		ServerTime string           `json:"server_time"`
	}
	if err := c.do(ctx, "GET", path, nil, &out); err != nil {
		return nil, err
	}

	res := &NoticeListResult{List: out.List}
	if out.ServerTime != "" {
		if t, err := time.Parse(time.RFC3339, out.ServerTime); err == nil {
			res.ServerTime = t
		}
	}
	return res, nil
}

// ReportDelivery 回报一条消息的投递结果。
//
// 回报失败**不返回错误**：投递台账是观测数据，写不进去不该让调用方
// 把"消息已收到"读成"投递失败"。
func (c *NoticeClient) ReportDelivery(ctx context.Context, messageID int64, tenantCode, status, errMsg string) error {
	body := map[string]any{
		"message_id":  messageID,
		"tenant_code": tenantCode,
		"status":      status,
		"error_msg":   errMsg,
	}
	return c.do(ctx, "POST", "/merchant-api/notices/report", body, nil)
}

// do 执行一次平台请求并解出 data。
//
// 成败判据走**信封 code**而不是 HTTP 状态码：平台侧 response.Success 与
// response.Error 都写 HTTP 200，真值只在信封的 code 字段里（与仓内
// 资产市场客户端、模块客户端同一口径）。
func (c *NoticeClient) do(ctx context.Context, method, path string, req any, out any) error {
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := c.client.Do(method, path, req, &env); err != nil {
		return err
	}
	// **成功码有两个**：平台侧 response.Success 写的是 `code:200`（不是 0），
	// 而仓内既有的 envelopeRefusal（internal/platform/client.go）把 `0` 与
	// `200` 都当成功。只认 0 会把每一次成功读成"平台拒绝" —— 这正是仓内
	// 宪法 §IV「以外部成败以业务码为准」要防的那类误判，故与之同一口径。
	if env.Code != 0 && env.Code != http.StatusOK {
		return fmt.Errorf("平台拒绝通知接口: code=%d msg=%s", env.Code, env.Msg)
	}
	if out == nil || len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("响应解析失败: %w", err)
	}
	return nil
}
