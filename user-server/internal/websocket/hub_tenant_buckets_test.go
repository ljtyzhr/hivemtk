package websocket

// A14（`未完成任务.md` §9）回归：WS 连接改两级桶 `connections[tenantID][agentID]`，
// 并且**所有**下行发送口都必须带租户。
//
// 修前形态：`clients map[agentID]*Client` 一级桶 + `agentOnline map[agentID]bool`。
// 坐席 ID 只是各租户库里的自增主键，"两个租户各有一个 id=7 的坐席"是正常现象 ⇒
// 后连的注册会把先连的**顶掉**（先连的那条连接既不报错也不再收到任何推送，
// 而且没被清理）。发送侧 SendToAgent / NotifyNewSession / NotifyNewMessage /
// NotifySessionUpdate / BroadcastToAgents 全都没有租户维度：
//   - 单播按 agentID 找，命中谁看谁先注册 ⇒ **误投**（不是"漏了隔离"，是投给了别人）；
//   - `BroadcastToAgents` 直接遍历全部连接 ⇒ 访客会话开场/转人工时把 session 详情
//     推进**所有租户**的工作台（service/chat_visitor.go 与 inbox_ingress_persist.go 真实调用）。
// 而 Client.tenantID 在两侧握手里都绑好了、全仓只有测试在读它 —— 本仓最典型的
// "值被记下来了，但没有任何一处 if 用它判定"。
//
// 判据（本文件钉死的五条）：
//  1. 同 agentID 跨租户不互相顶掉，单播只到本租户；
//  2. on 模式无租户的发送口 fail-closed（报错且谁都不发）；
//  3. on 模式 BroadcastToAgents 只发给本租户坐席（真调用点的那条路）；
//  4. off 模式逐字节等价：不限桶、发全部（不因本轮改动少收一条）；
//  5. 在线态查询（IsAgentOnline / GetOnlineAgents / GetOnlineCount）按租户。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/pkg/tenant"
)

// registerAgent 注册一条绑定指定租户的坐席连接并等到 Run 循环消化完。
func registerAgent(t *testing.T, hub *Hub, tenantID uint, agentID string) *Client {
	t.Helper()
	c := NewAgentClient(hub, agentID, agentID)
	c.tenantID = tenantID
	hub.Register(c)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		found := hub.connections[tenantID][agentID] == c
		hub.mu.RUnlock()
		if found {
			return c
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("注册超时：tenant=%d agent=%s", tenantID, agentID)
	return nil
}

// TestSendToAgentCtx_TwoTenantsSameAgentID 同 agentID 的两个租户互不干扰。
//
// 修前必红：一级 map 上第二次 Register 覆盖第一次 ⇒ `clientOf` 只剩一个连接，
// 且 `SendToAgentCtx(ctx5, "7")` 会把租户 5 的消息投给租户 6 的那条连接。
func TestSendToAgentCtx_TwoTenantsSameAgentID(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := NewHub()
	go hub.Run()

	// 两个租户各有一个 id=7 的坐席（同一 ID，正常现象）
	c5 := registerAgent(t, hub, 5, "7")
	c6 := registerAgent(t, hub, 6, "7")

	ctx5 := tenant.WithTenantID(context.Background(), 5)
	ctx6 := tenant.WithTenantID(context.Background(), 6)

	if err := hub.SendToAgentCtx(ctx5, "7", "new_message", map[string]any{"content": "只给租户5"}); err != nil {
		t.Fatalf("租户 5 单播失败: %v", err)
	}
	if !drain(c5, time.Second) {
		t.Fatal("租户 5 自己的坐席没收到单播（两级桶把合法连接丢哪去了）")
	}
	if drain(c6, 150*time.Millisecond) {
		t.Fatal("跨租户误投：租户 5 的消息被推给了租户 6 里同 ID 的坐席")
	}

	// 反向再验一次，确认不是"只有第一条连接能收到"
	if err := hub.SendToAgentCtx(ctx6, "7", "new_message", map[string]any{"content": "只给租户6"}); err != nil {
		t.Fatalf("租户 6 单播失败: %v", err)
	}
	if !drain(c6, time.Second) {
		t.Fatal("租户 6 自己的坐席没收到单播")
	}
	if drain(c5, 150*time.Millisecond) {
		t.Fatal("跨租户误投：租户 6 的消息被推给了租户 5 里同 ID 的坐席")
	}
}

// TestSendToAgentCtx_OnModeWithoutTenantFailsClosed on 模式下没有租户就不许发。
//
// 修前的 SendToAgent 没有 ctx，"发不发得出去"取决于有没有人碰巧注册过这个 ID。
// 现在缺租户直接报错 —— 报错比"猜一个桶"好：猜错了是数据泄漏，报错只是一行日志。
func TestSendToAgentCtx_OnModeWithoutTenantFailsClosed(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := NewHub()
	go hub.Run()
	c5 := registerAgent(t, hub, 5, "7")

	if err := hub.SendToAgentCtx(context.Background(), "7", "new_message", map[string]any{"x": 1}); err == nil {
		t.Fatal("on 模式无租户 ctx 的单播必须 fail-closed 报错")
	}
	if drain(c5, 150*time.Millisecond) {
		t.Fatal("fail-closed 之后仍有连接收到消息（等于没修）")
	}
	if err := NotifyNewSession(context.Background(), "7", map[string]any{"x": 1}); err == nil {
		t.Fatal("on 模式无租户 ctx 的 NotifyNewSession 必须报错")
	}
}

// TestBroadcastToAgentsCtx_OnModeIsolatesTenants 真调用点的那条路：
// service 层（访客开场 / 转人工 / 渠道消息入库）广播 session 详情时，只许发本租户。
func TestBroadcastToAgentsCtx_OnModeIsolatesTenants(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := NewHub()
	go hub.Run()
	c5 := registerAgent(t, hub, 5, "agentA")
	c6 := registerAgent(t, hub, 6, "agentB")

	ctx5 := tenant.WithTenantID(context.Background(), 5)
	if err := hub.BroadcastToAgentsCtx(ctx5, TypeNewSession, map[string]any{"session_id": "sess-a"}); err != nil {
		t.Fatalf("广播失败: %v", err)
	}
	if !drain(c5, time.Second) {
		t.Fatal("本租户坐席未收到广播")
	}
	if drain(c6, 150*time.Millisecond) {
		t.Fatal("跨租户泄漏：租户 6 的坐席收到了租户 5 的会话详情广播")
	}

	// on 模式无租户 ⇒ fail-closed（不许退化成全实例广播）
	if err := hub.BroadcastToAgentsCtx(context.Background(), TypeNewSession, map[string]any{"session_id": "leak"}); err == nil {
		t.Fatal("on 模式无租户 ctx 的 BroadcastToAgents 必须报错")
	}
	if drain(c5, 100*time.Millisecond) || drain(c6, 100*time.Millisecond) {
		t.Fatal("fail-closed 之后仍有人收到广播")
	}
}

// TestBroadcastToAgentsCtx_OffModeUnchanged 对照组：off 模式不限桶，发全部（逐字节等价）。
func TestBroadcastToAgentsCtx_OffModeUnchanged(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)

	hub := NewHub()
	go hub.Run()
	c1 := registerAgent(t, hub, tenant.DefaultTenantID, "agentA")
	c2 := registerAgent(t, hub, tenant.DefaultTenantID, "agentB")

	// off 模式即便 ctx 带着租户也不参与过滤（历史调用方从不传租户）
	ctx := tenant.WithTenantID(context.Background(), 9)
	if err := hub.BroadcastToAgentsCtx(ctx, TypeNewSession, map[string]any{"session_id": "s"}); err != nil {
		t.Fatalf("off 模式广播不应报错: %v", err)
	}
	for name, c := range map[string]*Client{"agentA": c1, "agentB": c2} {
		if !drain(c, time.Second) {
			t.Errorf("off 模式下 %s 应收到全实例广播", name)
		}
	}

	// 单播同样不限桶（SendToAgentCtx 在 off 模式解析为 0）
	if err := hub.SendToAgentCtx(context.Background(), "agentA", TypeNewMessage, map[string]any{"m": 1}); err != nil {
		t.Fatalf("off 模式单播不应报错: %v", err)
	}
	if !drain(c1, time.Second) {
		t.Error("off 模式下 agentA 应收到单播")
	}
}

// TestOnlineQueries_TenantScoped 在线态查询按租户：同 ID 只在本租户算在线。
func TestOnlineQueries_TenantScoped(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	hub := NewHub()
	go hub.Run()
	registerAgent(t, hub, 5, "7")
	registerAgent(t, hub, 6, "7")
	registerAgent(t, hub, 6, "8")

	ctx5 := tenant.WithTenantID(context.Background(), 5)
	ctx6 := tenant.WithTenantID(context.Background(), 6)

	if !hub.IsAgentOnline(ctx5, "7") || !hub.IsAgentOnline(ctx6, "7") {
		t.Fatal("两个租户各自的 7 号坐席都该算在线（一级桶下只会剩一个）")
	}
	if hub.IsAgentOnline(ctx5, "8") {
		t.Fatal("8 号坐席属于租户 6，租户 5 的查询不该看到它")
	}
	if n := hub.GetOnlineCount(ctx5); n != 1 {
		t.Fatalf("租户 5 在线数应为 1，实际 %d", n)
	}
	if n := hub.GetOnlineCount(ctx6); n != 2 {
		t.Fatalf("租户 6 在线数应为 2，实际 %d", n)
	}
	if got := hub.GetOnlineAgents(ctx5); len(got) != 1 || got[0] != "7" {
		t.Fatalf("租户 5 在线坐席应为 [7]，实际 %v", got)
	}

	// 无租户 ⇒ 安全侧：答"不在线"（答"在线"会让调用方把会话派给别人的坐席）
	if hub.IsAgentOnline(context.Background(), "7") {
		t.Fatal("on 模式无租户 ctx 的在线查询必须答 false（fail-closed 方向）")
	}
	if got := hub.GetOnlineAgents(context.Background()); len(got) != 0 {
		t.Fatalf("on 模式无租户 ctx 的在线列表必须为空，实际 %v", got)
	}
}
