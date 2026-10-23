package websocket

// WS 广播的租户分域（2026-10-23 修）。
//
// 缺陷形状：Hub.BroadcastToMerchant 遍历**全部**连接广播，注释写着"私域部署：单租户
// 全广播"。TENANT_MODE=on 之后这条形状就是跨租户泄漏 —— 租户 A 的 SOP 节点载荷
// （session_id / customer_id / content）会推进租户 B 的工作台；而 Client.tenantID
// 在握手时确实被记下来了（handler.go / visitor_handler.go），**整个仓库没有任何一处
// 用它过滤**，只有注释在守。同包 SSE（service/sse_hub.go Publish）早就按租户过滤，
// 两条推送通道口径不一致 —— 这就是"同一需求两套实现"的典型。
//
// 本组用例钉三态：on 模式按租户过滤 / on 模式无租户 fail-closed / off 模式逐字节等价。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
)

// newTestHubWithClients 起一个真 Hub 循环并注册若干「已绑定租户」的坐席连接。
func newTestHubWithClients(t *testing.T, tenants map[string]uint) *Hub {
	t.Helper()
	hub := NewHub()
	go hub.Run()
	for agentID, tid := range tenants {
		c := NewAgentClient(hub, agentID, agentID)
		c.tenantID = tid
		hub.Register(c)
	}
	// Register 走 channel，等 Run 循环消化完再断言，否则广播快照可能为空
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		n := len(hub.clients)
		hub.mu.RUnlock()
		if n == len(tenants) {
			return hub
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("测试 Hub 注册超时（期望 %d 个连接）", len(tenants))
	return nil
}

// drain 读一条推送：收到返回 true，超时返回 false（用于断言"没收到"）。
func drain(c *Client, d time.Duration) bool {
	select {
	case <-c.SendChan():
		return true
	case <-time.After(d):
		return false
	}
}

func clientOf(hub *Hub, agentID string) *Client {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return hub.clients[agentID]
}

func TestBroadcastToMerchant_OnModeIsolatesTenants(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := newTestHubWithClients(t, map[string]uint{
		"agentA": 5,
		"agentB": 6,
	})

	ctxA := tenant.WithTenantID(context.Background(), 5)
	if err := hub.BroadcastToMerchant(ctxA, "", "sop", map[string]any{
		"session_id":  "sess-a",
		"customer_id": "cust-a",
		"content":     "只有租户 5 该看到",
	}); err != nil {
		t.Fatalf("广播失败: %v", err)
	}

	if !drain(clientOf(hub, "agentA"), time.Second) {
		t.Fatal("本租户坐席未收到广播（过滤把合法连接也滤掉了）")
	}
	if drain(clientOf(hub, "agentB"), 150*time.Millisecond) {
		t.Fatal("跨租户泄漏：租户 6 的坐席收到了租户 5 的广播载荷")
	}
}

func TestBroadcastToMerchant_OnModeWithoutTenantFailsClosed(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := newTestHubWithClients(t, map[string]uint{
		"agentA": 5,
		"agentB": 6,
	})

	err := hub.BroadcastToMerchant(context.Background(), "", "sop", map[string]any{"content": "x"})
	if err == nil {
		t.Fatal("on 模式下无租户 ctx 必须 fail-closed 报错，而不是退化成全实例广播")
	}
	if drain(clientOf(hub, "agentA"), 150*time.Millisecond) {
		t.Fatal("fail-closed 之后仍有人收到广播（等于没修）")
	}
	if drain(clientOf(hub, "agentB"), 10*time.Millisecond) {
		t.Fatal("fail-closed 之后仍有人收到广播（等于没修）")
	}
}

func TestBroadcastToMerchant_OffModeUnchanged(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)

	hub := newTestHubWithClients(t, map[string]uint{
		"agentA": 1,
		"agentB": 1,
	})

	// off 模式：单租户全广播语义逐字节不变（离线单机不因本轮改动少收一条）
	if err := hub.BroadcastToMerchant(context.Background(), "", "agent_status", map[string]any{"status": "online"}); err != nil {
		t.Fatalf("off 模式广播不应报错: %v", err)
	}
	for _, id := range []string{"agentA", "agentB"} {
		if !drain(clientOf(hub, id), time.Second) {
			t.Errorf("off 模式下 %s 应收到全实例广播", id)
		}
	}

	// off 模式下即便 ctx 带着租户，也不参与过滤（回归面：历史调用方从不传租户）
	if err := hub.BroadcastToMerchant(tenant.WithTenantID(context.Background(), 9), "", "agent_status", map[string]any{"status": "busy"}); err != nil {
		t.Fatalf("off 模式广播不应报错: %v", err)
	}
	for _, id := range []string{"agentA", "agentB"} {
		if !drain(clientOf(hub, id), time.Second) {
			t.Errorf("off 模式下 %s 应收到全实例广播（租户不参与判定）", id)
		}
	}
}
