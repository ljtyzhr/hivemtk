package websocket

import (
	"context"
	"strconv"
)

// 消息类型常量（统一管理）
const (
	TypeNewSession    = "new_session"
	TypeNewMessage    = "new_message"
	TypeSessionUpdate = "session_update"

	TypeWelcome     = "welcome"
	TypeMessage     = "message"
	TypeAgentJoined = "agent_joined"
	TypeAITyping    = "ai_typing"
	TypeError       = "error"
	TypePong        = "pong"
)

// SendToAgent 发送消息给指定客服（坐席 ID 为 uint）
//
// 内部走 Envelope：分配全局 seq 编号，便于客户端排序 / 丢包检测。
// 不自动 track ACK（agent 端仅用 seq 做排序；ack 跟踪仅 visitor 端）。
// ctx 必带租户：坐席 ID 只在租户内有意义（on 模式无租户 ⇒ fail-closed，见 resolveSendTenant）。
func SendToAgent(ctx context.Context, messageType string, payload any, agentID uint) error {
	agentIDStr := strconv.FormatUint(uint64(agentID), 10)
	env := MustEnvelope(NextSeq(), messageType, payload)
	return GetHub().sendToAgentWithEnvelopeCtx(ctx, agentIDStr, env)
}

// SendToVisitor 发送消息给指定会话的访客
//
// 内部走 Envelope：分配全局 seq + 自动 Track ACK，
// 客户端发 `{"type":"ack","seq":N}` 后由 GlobalPendingAck().Ack 清理。
// 通过 sessionID 路由到对应的访客 WebSocket 连接。
//
// 鲁棒性约定：
//   - 走 MustEnvelope 分配 seq，与 visitor_handler.onConnect / hub.Broadcast 对齐
//   - 自动 GlobalPendingAck().Track，确保重连后可通过 PendingSince 拉取未确认
//   - 客户端断开时由 visitor_handler.readPump 的 defer Drop 清理
func SendToVisitor(messageType string, payload any, sessionID string) error {
	return GetHub().SendToVisitor(sessionID, messageType, payload)
}

// BroadcastToAll 广播给所有客户端（坐席 + 访客）
//
// ctx 必带租户：on 模式下只发本租户（on 无租户 ⇒ fail-closed）。
func BroadcastToAll(ctx context.Context, messageType string, payload any) error {
	return GetHub().BroadcastToAllCtx(ctx, messageType, payload)
}

// BroadcastToAgents 广播给所有坐席（排除访客）
//
// ctx 必带租户：on 模式下只发本租户（on 无租户 ⇒ fail-closed）。
// 租户过滤是硬性语义：不加过滤时，访客会话开场/转人工时 service 层广播的
// session 详情会推给全实例所有坐席，租户 B 的工作台会弹出租户 A 的会话。
func BroadcastToAgents(ctx context.Context, messageType string, payload any) error {
	return GetHub().BroadcastToAgentsCtx(ctx, messageType, payload)
}

// SendToVisitor 发送消息给指定会话的访客
//
// 走 Envelope：分配 seq + Track ACK；
// 客户端通过 `{"type":"ack","seq":N}` 确认收到。
func (h *Hub) SendToVisitor(sessionID, messageType string, payload any) error {
	if sessionID == "" {
		return nil
	}

	visitor := getVisitorClient(sessionID)
	if visitor == nil {
		return nil
	}

	env := MustEnvelope(NextSeq(), messageType, payload)
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}
	GlobalPendingAck().Track(sessionID, env.Seq)

	select {
	case visitor.send <- bytes:
	default:
	}
	return nil
}

// BroadcastToAllCtx 广播给 ctx 租户下的所有客户端（off 模式 = 全实例，语义不变）
//
// 走 Envelope 统一分配 seq：保证全站消息序号单调递增。
// agent 端不自动 Track ACK（agent 端仅用 seq 做排序，不维护待 ACK 队列）。
// visitor 端不自动 Track ACK（仅 SendToVisitor 单播走 ACK 跟踪，广播消息的 ACK
// 由 visitor_handler.onConnect 的 offline_messages 路径统一处理）。
func (h *Hub) BroadcastToAllCtx(ctx context.Context, messageType string, payload any) error {
	tid, err := resolveSendTenant(ctx, "BroadcastToAll "+messageType)
	if err != nil {
		return err
	}
	env := MustEnvelope(NextSeq(), messageType, payload)
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}

	h.mu.RLock()
	pool := h.allClientsLocked()
	if tid != 0 {
		pool = make([]*Client, 0, len(h.connections[tid]))
		for _, client := range h.connections[tid] {
			pool = append(pool, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range pool {
		select {
		case client.send <- bytes:
		default:
		}
	}

	// 访客连接不在 connections 里（见 visitor_registry.go），访客侧已由 sessionID 维度
	// 自带隔离（SendToVisitor 按 session 路由），这里不重复按租户过滤——但 on 模式下
	// 无租户的调用已在上面 fail-closed 掉，不会有人从这里"顺路"广播给所有访客。
	visitorClientsMu.RLock()
	for _, client := range visitorClients {
		select {
		case client.send <- bytes:
		default:
		}
	}
	visitorClientsMu.RUnlock()

	return nil
}

// BroadcastToAgentsCtx 广播给 ctx 租户下的所有坐席（不含访客）
//
// 走 Envelope 统一分配 seq：保证全站消息序号单调递增。
func (h *Hub) BroadcastToAgentsCtx(ctx context.Context, messageType string, payload any) error {
	tid, err := resolveSendTenant(ctx, "BroadcastToAgents "+messageType)
	if err != nil {
		return err
	}
	env := MustEnvelope(NextSeq(), messageType, payload)
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}

	h.mu.RLock()
	pool := h.allClientsLocked()
	if tid != 0 {
		pool = make([]*Client, 0, len(h.connections[tid]))
		for _, client := range h.connections[tid] {
			pool = append(pool, client)
		}
	}
	h.mu.RUnlock()

	for _, client := range pool {
		if client.IsVisitor() {
			continue
		}
		select {
		case client.send <- bytes:
		default:
		}
	}
	return nil
}
