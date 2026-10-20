package websocket

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/security"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// recordingVisitorResolver 记录解析调用，按传入 ref 返回 canonical 渠道视图。
// 模拟装配层适配器的对外契约：ref 可以是 app_key，返回的行自带 canonical ChannelID。
type recordingVisitorResolver struct {
	canonical map[string]*VisitorChannelView
	hits      []string
}

func (r *recordingVisitorResolver) ResolveByChannelID(_ context.Context, ref string) (*VisitorChannelView, error) {
	r.hits = append(r.hits, ref)
	if ch, ok := r.canonical[ref]; ok {
		return ch, nil
	}
	return nil, errors.New("渠道不存在: " + ref)
}

// newVisitorWSTestServer 起一个只挂访客 WS 路由的裸服务（nil DB：onConnect 直接返回）。
func newVisitorWSTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewVisitorWSHandler(nil)
	r.GET("/api/ws/visitor", h.HandleVisitorWebSocket)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func wsDialURL(srv *httptest.Server, q url.Values) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/visitor?" + q.Encode()
}

func waitVisitorClientRegistered(t *testing.T, sessionID string) *Client {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c := getVisitorClient(sessionID); c != nil {
			return c
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s 的访客 client 未在期限内注册", sessionID)
	return nil
}

func waitVisitorClientGone(t *testing.T, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if getVisitorClient(sessionID) == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c := getVisitorClient(sessionID); c != nil {
		UnregisterVisitorClient(c)
		t.Logf("readPump 未在期限内注销 %s，已手动清理", sessionID)
	}
}

// TestVisitorWSNormalizesRefBeforeTokenCheck 钉住 WS 访客链的 ref 归一化：
//
// bug 现场——SDK 以 appKey 作 channel_id query（浏览器 WS 不能带头），而
// visitor_token 由 GenerateVisitorToken(channel.ChannelID,…) 按渠道行 canonical
// channel_id 签发；修复前 appKey 形 ref 会因 token 渠道不匹配被 403，客服
// widget 连不上 WS。修复语义：解析归一化必须发生在 token 校验之前，
// 命中行后 client 与 ctx 都落到 canonical channel_id / 渠道行租户。
func TestVisitorWSNormalizesRefBeforeTokenCheck(t *testing.T) {
	const secret = "visitor-ws-ref-test-secret-0123456789"
	prevCfg := config.GetAppConfig()
	config.SetAppConfig(&config.AppConfig{Security: config.SecurityConfig{VisitorTokenSecret: secret}})
	t.Cleanup(func() { config.SetAppConfig(&prevCfg) })

	prevResolver := visitorChannelResolver
	t.Cleanup(func() { SetVisitorChannelResolver(prevResolver) })

	srv := newVisitorWSTestServer(t)

	const (
		canonical = "9885e22d-e5a1-4291-9a33-86709616d61a"
		appKeyRef = "ak_WSRefNormalizeTest0000000000"
	)

	t.Run("appKey 形 ref + canonical 绑定 token 必须连上并绑租户", func(t *testing.T) {
		res := &recordingVisitorResolver{canonical: map[string]*VisitorChannelView{
			appKeyRef: {ChannelID: canonical, TenantID: 7},
		}}
		SetVisitorChannelResolver(res)

		token, err := security.GenerateVisitorToken(secret, canonical, "visitor-1", "sess-ref", time.Hour)
		if err != nil {
			t.Fatalf("生成 token: %v", err)
		}
		q := url.Values{}
		q.Set("session_id", "sess-ref")
		q.Set("visitor_id", "visitor-1")
		q.Set("channel_id", appKeyRef)
		q.Set("visitor_token", token)

		conn, resp, err := websocket.DefaultDialer.Dial(wsDialURL(srv, q), nil)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("修复后 appKey 形 ref 必须通过 token 校验完成握手（403=归一化没发生在校验前），实际 err=%v status=%d", err, status)
		}
		defer conn.Close()

		client := waitVisitorClientRegistered(t, "sess-ref")
		if client.channelID != canonical {
			t.Errorf("client.channelID 应为 canonical %q，实际 %q", canonical, client.channelID)
		}
		if client.TenantID() != 7 {
			t.Errorf("client 租户应为 7，实际 %d", client.TenantID())
		}
		if len(res.hits) != 1 || res.hits[0] != appKeyRef {
			t.Errorf("归一化应恰解析一次且收到原始 ref，实际 %v", res.hits)
		}
		conn.Close()
		waitVisitorClientGone(t, "sess-ref")
	})

	t.Run("canonical ref 直连不受归一化影响", func(t *testing.T) {
		res := &recordingVisitorResolver{canonical: map[string]*VisitorChannelView{
			canonical: {ChannelID: canonical, TenantID: 7},
		}}
		SetVisitorChannelResolver(res)

		token, err := security.GenerateVisitorToken(secret, canonical, "visitor-2", "sess-direct", time.Hour)
		if err != nil {
			t.Fatalf("生成 token: %v", err)
		}
		q := url.Values{}
		q.Set("session_id", "sess-direct")
		q.Set("visitor_id", "visitor-2")
		q.Set("channel_id", canonical)
		q.Set("visitor_token", token)

		conn, resp, err := websocket.DefaultDialer.Dial(wsDialURL(srv, q), nil)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("canonical ref 必须握手成功，实际 err=%v status=%d", err, status)
		}
		defer conn.Close()

		client := waitVisitorClientRegistered(t, "sess-direct")
		if client.channelID != canonical || client.TenantID() != 7 {
			t.Errorf("canonical 直连应保持 channel=%q tenant=7，实际 channel=%q tenant=%d",
				canonical, client.channelID, client.TenantID())
		}
		if len(res.hits) != 1 {
			t.Errorf("应恰解析一次（不得因租户绑定重复查库），实际 %v", res.hits)
		}
		conn.Close()
		waitVisitorClientGone(t, "sess-direct")
	})

	t.Run("token 绑定渠道与 ref 不符仍必须 403", func(t *testing.T) {
		const wrongChannel = "00000000-0000-0000-0000-000000000000"
		res := &recordingVisitorResolver{canonical: map[string]*VisitorChannelView{
			appKeyRef: {ChannelID: canonical, TenantID: 7},
		}}
		SetVisitorChannelResolver(res)

		// token 按另一个渠道签发：归一化命中后 channelID=canonical，校验必须拒绝
		token, err := security.GenerateVisitorToken(secret, wrongChannel, "visitor-3", "sess-bad", time.Hour)
		if err != nil {
			t.Fatalf("生成 token: %v", err)
		}
		q := url.Values{}
		q.Set("session_id", "sess-bad")
		q.Set("visitor_id", "visitor-3")
		q.Set("channel_id", appKeyRef)
		q.Set("visitor_token", token)

		conn, resp, err := websocket.DefaultDialer.Dial(wsDialURL(srv, q), nil)
		if err == nil {
			conn.Close()
			t.Fatal("渠道不符的 token 必须被拒绝，实际握手成功")
		}
		if resp == nil || resp.StatusCode != http.StatusForbidden {
			t.Fatalf("应回 403（fail-closed），实际 %+v", resp)
		}
	})

	t.Run("解析未命中时保留原始 ref（降级路径）", func(t *testing.T) {
		const unknownRef = "ak_UnknownRefDegrade00000000000"
		res := &recordingVisitorResolver{canonical: map[string]*VisitorChannelView{}}
		SetVisitorChannelResolver(res)

		token, err := security.GenerateVisitorToken(secret, unknownRef, "visitor-4", "sess-degrade", time.Hour)
		if err != nil {
			t.Fatalf("生成 token: %v", err)
		}
		q := url.Values{}
		q.Set("session_id", "sess-degrade")
		q.Set("visitor_id", "visitor-4")
		q.Set("channel_id", unknownRef)
		q.Set("visitor_token", token)

		conn, resp, err := websocket.DefaultDialer.Dial(wsDialURL(srv, q), nil)
		if err != nil {
			status := 0
			if resp != nil {
				status = resp.StatusCode
			}
			t.Fatalf("解析未命中不应改变 ref 语义（token 按原始 ref 校验通过），实际 err=%v status=%d", err, status)
		}
		defer conn.Close()

		client := waitVisitorClientRegistered(t, "sess-degrade")
		if client.channelID != unknownRef {
			t.Errorf("未命中时应保留原始 ref %q，实际 %q", unknownRef, client.channelID)
		}
		if client.TenantID() != 0 {
			t.Errorf("未命中时不应绑定租户，实际 %d", client.TenantID())
		}
		conn.Close()
		waitVisitorClientGone(t, "sess-degrade")
	})
}
