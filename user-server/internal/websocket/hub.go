package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// Hub WebSocket连接中心
type Hub struct {
	clients     map[string]*Client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan *envelopeFrame
	mu          sync.RWMutex
	agentOnline map[string]bool
}

type envelopeFrame struct {
	agentID string
	bytes   []byte
}

// Client WebSocket客户端
// 同时支持 agent（坐席）和 visitor（访客）两种类型
type Client struct {
	hub        *Hub
	send       chan []byte
	clientType ClientType
	agentID    string
	agentName  string
	sessionID  string
	visitorID  string
	channelID  string
	// tenantID 连接归属租户（specs/001-saas-multi-tenant T026）：
	// 握手时从 JWT 租户 claim 绑定，推送按租户过滤（0=off 模式/系统广播）
	tenantID          uint
	onConnectInflight atomic.Bool
}

// TenantID 连接归属租户（推送分桶判定用）
func (c *Client) TenantID() uint { return c.tenantID }

// Message WebSocket消息
type Message struct {
	Type    string          `json:"type"`
	AgentID string          `json:"agent_id"`
	Payload json.RawMessage `json:"payload"`
}

// NewHub 创建WebSocket中心
func NewHub() *Hub {
	return &Hub{
		clients:     make(map[string]*Client),
		register:    make(chan *Client, 256),
		unregister:  make(chan *Client, 256),
		broadcast:   make(chan *envelopeFrame, 1024),
		agentOnline: make(map[string]bool),
	}
}

// Run 运行WebSocket中心
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client.agentID] = client
			h.agentOnline[client.agentID] = true
			h.mu.Unlock()
			logger.GetLogger().Info().Str("agent_id", client.agentID).Msg("agent connected")

		case client := <-h.unregister:
			h.mu.Lock()
			if existing, ok := h.clients[client.agentID]; ok {
				delete(h.clients, client.agentID)
				h.agentOnline[client.agentID] = false
				close(existing.send)
			}
			h.mu.Unlock()
			logger.GetLogger().Info().Str("agent_id", client.agentID).Msg("agent disconnected")

		case frame := <-h.broadcast:
			h.mu.RLock()
			client, ok := h.clients[frame.agentID]
			h.mu.RUnlock()
			if !ok {
				continue
			}

			select {
			case client.send <- frame.bytes:
			default:
				logger.GetLogger().Warn().Str("agent_id", frame.agentID).Msg("ws send buffer full, frame dropped")
			}

		case <-ticker.C:
			h.mu.RLock()
			clients := make([]*Client, 0, len(h.clients))
			for _, client := range h.clients {
				clients = append(clients, client)
			}
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- []byte(`{"type":"heartbeat"}`):
				default:

					logger.GetLogger().Warn().Str("agent_id", client.agentID).Msg("ws heartbeat dropped, marking offline")
					h.mu.Lock()
					if existing, ok := h.clients[client.agentID]; ok {
						delete(h.clients, client.agentID)
						h.agentOnline[client.agentID] = false
						close(existing.send)
					}
					h.mu.Unlock()
				}
			}
		}
	}
}

// Register 注册客户端
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 注销客户端
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast 广播消息
//
// 内部走 Envelope：分配全局 seq 编号，便于客户端排序 / 丢包检测。
func (h *Hub) Broadcast(message *Message) {
	if message == nil {
		return
	}
	env := MustEnvelope(NextSeq(), message.Type, message.Payload)
	bytes, err := env.MarshalBytes()
	if err != nil {
		return
	}
	h.broadcast <- &envelopeFrame{agentID: message.AgentID, bytes: bytes}
}

func (h *Hub) sendToAgentWithEnvelope(agentID string, env *Envelope) error {
	if env == nil {
		return nil
	}
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}
	h.broadcast <- &envelopeFrame{agentID: agentID, bytes: bytes}
	return nil
}

// SendToAgent 发送消息给指定客服
func (h *Hub) SendToAgent(agentID string, messageType string, payload any) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	h.Broadcast(&Message{
		Type:    messageType,
		AgentID: agentID,
		Payload: payloadBytes,
	})
	return nil
}

// BroadcastToMerchant 向「同一租户」的全部坐席连接广播。
//
// 历史形状：私域单租户部署 ⇒ 直接遍历全部连接，merchantID 参数留作接口一致性。
// 在 TENANT_MODE=on 下这条形状就是**跨租户泄漏**：租户 A 的 SOP 节点载荷
// （session_id / customer_id / content）会被推进租户 B 的工作台。同包的 SSE
// 通道（service/sse_hub.go Publish）早已按租户过滤，WS 这一侧却只把
// client.tenantID **记下来**、没有任何一处用它过滤——正是"只有注释在守"的形状。
//
// 现行口径（与 SSE 同构）：
//   - off 模式：与历史逐字节一致 —— 发全部坐席（单租户全广播语义不变）
//   - on 模式：只发 ctx 租户名下的连接；ctx 缺租户 ⇒ 拒绝执行（fail-closed），
//     绝不退化成"全实例广播"——那是本条要修的缺陷本身
func (h *Hub) BroadcastToMerchant(ctx context.Context, merchantID string, messageType string, payload any) error {
	_ = merchantID // 私域语义：merchant 维度不参与过滤（租户维度才是隔离判据）
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	tid := uint(0)
	if tenant.IsMultiTenant() {
		id, ok := tenant.FromContext(ctx)
		if !ok {
			logger.GetLogger().Warn().
				Str("message_type", messageType).
				Msg("[ws] on 模式下广播缺租户上下文，已拒绝（绝不退化成全实例广播）")
			return fmt.Errorf("[ws] 广播 %q 缺租户上下文（TENANT_MODE=on 下 fail-closed）", messageType)
		}
		tid = id
	}

	for _, agentID := range h.agentIDsForTenant(tid) {
		h.Broadcast(&Message{
			Type:    messageType,
			AgentID: agentID,
			Payload: payloadBytes,
		})
	}
	return nil
}

// agentIDsForTenant 快照目标坐席：tid=0 表示不限租户（off 模式/全实例语义）。
//
// 只在锁内做快照；Broadcast 的入队（broadcast channel 可能阻塞）必须在 RUnlock
// 之后，否则 channel 打满时持 RLock 阻塞会卡死 Run 循环的 Lock → 整个 hub 死锁
// （与既有 BroadcastToMerchant 同一坑，不是新引入的）。
func (h *Hub) agentIDsForTenant(tid uint) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	agentIDs := make([]string, 0, len(h.clients))
	for agentID, client := range h.clients {
		if tid != 0 && client.tenantID != tid {
			continue
		}
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs
}

// IsAgentOnline 检查客服是否在线
func (h *Hub) IsAgentOnline(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.agentOnline[agentID]
}

// GetOnlineAgents 获取在线客服列表
// 私域部署：忽略 merchantID 过滤，返回所有在线 agent
func (h *Hub) GetOnlineAgents(merchantID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var agents []string
	for agentID, client := range h.clients {
		if client.IsVisitor() {
			continue
		}
		_ = merchantID
		agents = append(agents, agentID)
	}
	return agents
}

// GetOnlineCount 获取在线客服数量
func (h *Hub) GetOnlineCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// NewClient 创建客服坐席客户端（保持向后兼容）
// 推荐使用 NewAgentClient 显式指定类型
func NewWSClient(hub *Hub, agentID, agentName string) *Client {
	return NewAgentClient(hub, agentID, agentName)
}

// NewAgentClient 创建客服坐席客户端
func NewAgentClient(hub *Hub, agentID, agentName string) *Client {
	return &Client{
		hub:        hub,
		send:       make(chan []byte, 256),
		clientType: ClientTypeAgent,
		agentID:    agentID,
		agentName:  agentName,
	}
}

// NewVisitorClient 创建访客客户端
func NewVisitorClient(hub *Hub, sessionID, visitorID, channelID string) *Client {
	return &Client{
		hub:        hub,
		send:       make(chan []byte, 256),
		clientType: ClientTypeVisitor,
		sessionID:  sessionID,
		visitorID:  visitorID,
		channelID:  channelID,
	}
}

// IsVisitor 是否访客客户端
func (c *Client) IsVisitor() bool {
	return c.clientType == ClientTypeVisitor
}

// SendChan 返回客户端下行消息通道（导出，供跨包集成测试读取真实推送）
func (c *Client) SendChan() <-chan []byte {
	return c.send
}

var globalHub *Hub
var once sync.Once

// GetHub 获取全局WebSocket中心
func GetHub() *Hub {
	once.Do(func() {
		globalHub = NewHub()
		go globalHub.Run()
	})
	return globalHub
}

// NotifyNewSession 通知新会话
func NotifyNewSession(agentID string, sessionData any) error {
	return GetHub().SendToAgent(agentID, "new_session", sessionData)
}

// NotifyNewMessage 通知新消息
func NotifyNewMessage(agentID string, messageData any) error {
	return GetHub().SendToAgent(agentID, "new_message", messageData)
}

// NotifySessionUpdate 通知会话状态更新
func NotifySessionUpdate(agentID string, sessionData any) error {
	return GetHub().SendToAgent(agentID, "session_update", sessionData)
}

// BroadcastAgentStatus 广播客服状态变更（按 ctx 租户分域，见 BroadcastToMerchant）
func BroadcastAgentStatus(ctx context.Context, statusData any) error {
	return GetHub().BroadcastToMerchant(ctx, "", "agent_status", statusData)
}
