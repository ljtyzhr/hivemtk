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
//
// connections 是**两级桶**：租户 → (agentID → 连接)。
//
// 修前是一级 `map[agentID]*Client`（A14）：坐席 ID 只是数据库里的自增主键，
// 两个租户各有一个 id=7 的坐席是**正常现象**；一级 map 下后连的会把先连的顶掉，
// 于是租户 B 的坐席一上线，租户 A 的坐席就静默"消失"（收不到任何推送、
// 而且顶掉的那条连接不会被清理，谁也发现不了）。send 时只按 agentID 找，
// 命中谁全看谁先注册 —— 这不是"没做隔离"，是**误投**。
//
// 两级之后每层语义都是确定的：桶外的事（谁来发、发给谁）必须显式带租户。
type Hub struct {
	connections map[uint]map[string]*Client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan *envelopeFrame
	mu          sync.RWMutex
}

// envelopeFrame 一次下行投递。
//
// tenantID == 0 是**唯一**的"不限租户桶"取值，只在 off 模式下由发送侧产生
// （离线单机 = 单租户，全实例语义与升级前逐字节一致；见 resolveSendTenant）。
// on 模式下发送侧取不到租户就直接失败，绝不产生 tenantID==0 的帧。
type envelopeFrame struct {
	tenantID uint
	agentID  string
	bytes    []byte
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
		connections: make(map[uint]map[string]*Client),
		register:    make(chan *Client, 256),
		unregister:  make(chan *Client, 256),
		broadcast:   make(chan *envelopeFrame, 1024),
	}
}

// bucketLocked 取（必要时新建）某租户的连接桶。调用方必须持有写锁。
func (h *Hub) bucketLocked(tenantID uint) map[string]*Client {
	b, ok := h.connections[tenantID]
	if !ok {
		b = make(map[string]*Client)
		h.connections[tenantID] = b
	}
	return b
}

// dropLocked 从所属桶里摘掉连接并在桶空时回收桶。调用方必须持有写锁。
func (h *Hub) dropLocked(tenantID uint, agentID string) (*Client, bool) {
	b, ok := h.connections[tenantID]
	if !ok {
		return nil, false
	}
	existing, ok := b[agentID]
	if !ok {
		return nil, false
	}
	delete(b, agentID)
	if len(b) == 0 {
		delete(h.connections, tenantID)
	}
	return existing, true
}

// allClientsLocked 快照全部连接（含所有租户桶）。调用方必须持有读锁。
func (h *Hub) allClientsLocked() []*Client {
	n := 0
	for _, b := range h.connections {
		n += len(b)
	}
	clients := make([]*Client, 0, n)
	for _, b := range h.connections {
		for _, client := range b {
			clients = append(clients, client)
		}
	}
	return clients
}

// Run 运行WebSocket中心
func (h *Hub) Run() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if prev, ok := h.dropLocked(client.tenantID, client.agentID); ok {
				// 同桶同 agentID 重连：旧连接必须显式关掉，否则它留在 writePump 里
				// 永远等不到 EOF（send 无人再写），泄漏一个 goroutine + 一条 TCP。
				close(prev.send)
			}
			h.bucketLocked(client.tenantID)[client.agentID] = client
			h.mu.Unlock()
			logger.GetLogger().Info().Str("agent_id", client.agentID).Uint("tenant_id", client.tenantID).Msg("agent connected")

		case client := <-h.unregister:
			h.mu.Lock()
			// 只摘"这条连接自己"：同 agentID 的新连接已经接管时，旧连接的
			// unregister 不能把新连接摘掉（修前的一级 map 正是这么误杀的）。
			if existing, ok := h.dropLocked(client.tenantID, client.agentID); ok && existing == client {
				close(existing.send)
			}
			h.mu.Unlock()
			logger.GetLogger().Info().Str("agent_id", client.agentID).Uint("tenant_id", client.tenantID).Msg("agent disconnected")

		case frame := <-h.broadcast:
			h.mu.RLock()
			var targets []*Client
			if frame.tenantID == 0 {
				// 不限租户桶：off 模式的全实例语义（逐字节等价于升级前）
				targets = h.allClientsLocked()
			} else if b, ok := h.connections[frame.tenantID]; ok {
				if client, ok := b[frame.agentID]; ok {
					targets = []*Client{client}
				}
			}
			h.mu.RUnlock()

			for _, client := range targets {
				select {
				case client.send <- frame.bytes:
				default:
					logger.GetLogger().Warn().Str("agent_id", frame.agentID).Uint("tenant_id", client.tenantID).Msg("ws send buffer full, frame dropped")
				}
			}

		case <-ticker.C:
			h.mu.RLock()
			clients := h.allClientsLocked()
			h.mu.RUnlock()

			for _, client := range clients {
				select {
				case client.send <- []byte(`{"type":"heartbeat"}`):
				default:

					logger.GetLogger().Warn().Str("agent_id", client.agentID).Msg("ws heartbeat dropped, marking offline")
					h.mu.Lock()
					if existing, ok := h.dropLocked(client.tenantID, client.agentID); ok && existing == client {
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

// resolveSendTenant 解析一次推送的目标租户桶。**所有** WS 下行都必须经过它。
//
//   - off 模式：返回 0（= 不限桶，全实例语义；与升级前逐字节一致，不多付令牌成本）
//   - on 模式 + ctx 有租户：返回该租户（精确落桶）
//   - on 模式 + ctx 无租户：报错 fail-closed。绝不回落到 0 —— 0 在这条链上是
//     "全部租户"，一旦回落就等于把漏配变成跨租户群发（A14 修的正是这个形状）。
func resolveSendTenant(ctx context.Context, op string) (uint, error) {
	if !tenant.IsMultiTenant() {
		return 0, nil
	}
	id, ok := tenant.FromContext(ctx)
	if !ok {
		logger.GetLogger().Warn().Str("op", op).
			Msg("[ws] on 模式下推送缺租户上下文，已拒绝（绝不退化成全实例推送）")
		return 0, fmt.Errorf("[ws] %s 缺租户上下文（TENANT_MODE=on 下 fail-closed）", op)
	}
	return id, nil
}

// enqueue 把一帧投进分发队列。tenantID=0 表示不限桶（只在 off 模式产生）。
func (h *Hub) enqueue(tenantID uint, agentID string, bytes []byte) {
	h.broadcast <- &envelopeFrame{tenantID: tenantID, agentID: agentID, bytes: bytes}
}

// SendToAgentCtx 发送消息给「ctx 租户名下」的指定客服。
//
// 消费方是坐席工作台：坐席 ID 只在租户内有意义（各租户自增），所以租户必须来自
// 调用方的 ctx，不能来自连接侧反查——反查在 on 模式下等于"猜"。
func (h *Hub) SendToAgentCtx(ctx context.Context, agentID string, messageType string, payload any) error {
	tid, err := resolveSendTenant(ctx, "SendToAgent "+messageType)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	env := MustEnvelope(NextSeq(), messageType, payloadBytes)
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}
	h.enqueue(tid, agentID, bytes)
	return nil
}

func (h *Hub) sendToAgentWithEnvelopeCtx(ctx context.Context, agentID string, env *Envelope) error {
	if env == nil {
		return nil
	}
	tid, err := resolveSendTenant(ctx, "SendToAgent "+env.Type)
	if err != nil {
		return err
	}
	bytes, err := env.MarshalBytes()
	if err != nil {
		return err
	}
	h.enqueue(tid, agentID, bytes)
	return nil
}

// BroadcastToMerchant 向「同一租户」的全部坐席连接广播。
//
// 历史形状：私域单租户部署 ⇒ 直接遍历全部连接，merchantID 参数留作接口一致性。
// 在 TENANT_MODE=on 下那条形状就是**跨租户泄漏**：租户 A 的 SOP 节点载荷
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
	tid, err := resolveSendTenant(ctx, "BroadcastToMerchant "+messageType)
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	for _, agentID := range h.agentIDsForTenant(tid) {
		env := MustEnvelope(NextSeq(), messageType, payloadBytes)
		bytes, err := env.MarshalBytes()
		if err != nil {
			continue
		}
		h.enqueue(tid, agentID, bytes)
	}
	return nil
}

// agentIDsForTenant 快照目标坐席：tid=0 表示不限租户（off 模式/全实例语义）。
//
// 只在锁内做快照；入队（broadcast channel 可能阻塞）必须在 RUnlock 之后，
// 否则 channel 打满时持 RLock 阻塞会卡死 Run 循环的 Lock → 整个 hub 死锁。
func (h *Hub) agentIDsForTenant(tid uint) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if tid == 0 {
		agentIDs := make([]string, 0, len(h.connections))
		for _, client := range h.allClientsLocked() {
			agentIDs = append(agentIDs, client.agentID)
		}
		return agentIDs
	}
	b := h.connections[tid]
	agentIDs := make([]string, 0, len(b))
	for agentID := range b {
		agentIDs = append(agentIDs, agentID)
	}
	return agentIDs
}

// IsAgentOnline 检查「ctx 租户名下」的客服是否在线。
//
// 返回 false 有两种情况，且都是安全的：该租户下没有这条连接 / on 模式下 ctx 无租户
// （读不到租户时宁可答"不在线"——答"在线"会让调用方把一个会话派给别人的坐席）。
func (h *Hub) IsAgentOnline(ctx context.Context, agentID string) bool {
	tid, err := resolveSendTenant(ctx, "IsAgentOnline")
	if err != nil {
		return false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if tid == 0 {
		for _, b := range h.connections {
			if _, ok := b[agentID]; ok {
				return true
			}
		}
		return false
	}
	_, ok := h.connections[tid][agentID]
	return ok
}

// GetOnlineAgents 获取「ctx 租户名下」的在线客服列表。
//
// 原来的 merchantID 形参一直是 `_ = merchantID`（写而不用）：merchant 维度在私域
// 单租户里恒等于"全部"，留着一个不参与判定的参数只会让人以为它参与过滤。
// 现在换成 ctx 租户，签名与语义一致。
func (h *Hub) GetOnlineAgents(ctx context.Context) []string {
	tid, err := resolveSendTenant(ctx, "GetOnlineAgents")
	if err != nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	var pool []*Client
	if tid == 0 {
		pool = h.allClientsLocked()
	} else {
		pool = make([]*Client, 0, len(h.connections[tid]))
		for _, client := range h.connections[tid] {
			pool = append(pool, client)
		}
	}

	agents := make([]string, 0, len(pool))
	for _, client := range pool {
		if client.IsVisitor() {
			continue
		}
		agents = append(agents, client.agentID)
	}
	return agents
}

// GetOnlineCount 获取「ctx 租户名下」的在线连接数（off 模式 = 全部，语义不变）。
func (h *Hub) GetOnlineCount(ctx context.Context) int {
	tid, err := resolveSendTenant(ctx, "GetOnlineCount")
	if err != nil {
		return 0
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if tid == 0 {
		return len(h.allClientsLocked())
	}
	return len(h.connections[tid])
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

// NotifyNewSession 通知新会话（ctx 必带租户，见 resolveSendTenant）
func NotifyNewSession(ctx context.Context, agentID string, sessionData any) error {
	return GetHub().SendToAgentCtx(ctx, agentID, "new_session", sessionData)
}

// NotifyNewMessage 通知新消息（ctx 必带租户，见 resolveSendTenant）
func NotifyNewMessage(ctx context.Context, agentID string, messageData any) error {
	return GetHub().SendToAgentCtx(ctx, agentID, "new_message", messageData)
}

// NotifySessionUpdate 通知会话状态更新（ctx 必带租户，见 resolveSendTenant）
func NotifySessionUpdate(ctx context.Context, agentID string, sessionData any) error {
	return GetHub().SendToAgentCtx(ctx, agentID, "session_update", sessionData)
}

// BroadcastAgentStatus 广播客服状态变更（按 ctx 租户分域，见 BroadcastToMerchant）
func BroadcastAgentStatus(ctx context.Context, statusData any) error {
	return GetHub().BroadcastToMerchant(ctx, "", "agent_status", statusData)
}
