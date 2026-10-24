package llm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// TraceContext 全链路追踪上下文
type TraceContext struct {
	traceID      string
	spanID       string
	parentSpanID string
	mu           sync.RWMutex
}

// NewTraceContext 创建新的追踪上下文
// traceID 为空时生成 UUIDv7（按时间排序，便于时序查询）
func NewTraceContext(traceID, parentSpanID string) *TraceContext {
	if traceID == "" {
		traceID = generateTraceID()
	}
	return &TraceContext{
		traceID:      traceID,
		spanID:       generateSpanID(),
		parentSpanID: parentSpanID,
	}
}

// TraceID 返回 trace_id
func (tc *TraceContext) TraceID() string {
	if tc == nil {
		return ""
	}
	return tc.traceID
}

// SpanID 返回 span_id
func (tc *TraceContext) SpanID() string {
	if tc == nil {
		return ""
	}
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.spanID
}

// ParentSpanID 返回 parent_span_id
func (tc *TraceContext) ParentSpanID() string {
	if tc == nil {
		return ""
	}
	return tc.parentSpanID
}

// SetSpanIDFor 显式设置本 span_id（用于 trace middleware 注入 W3C 子 span）。
// 防御：tc 为 nil 时不 panic。
func (tc *TraceContext) SetSpanIDFor(spanID string) {
	if tc == nil {
		return
	}
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.spanID = spanID
}

// GenerateSpanIDStatic 包级便捷函数：生成新 span_id（16 hex）。
// 用于 trace middleware 等需要同步获取 span_id 而不构造完整 TraceContext 的场景。
func GenerateSpanIDStatic() string {
	return generateSpanID()
}

// GenerateTraceIDStatic 包级便捷函数：生成新 trace_id（32 hex，W3C 规范）。
// 用于 trace middleware 等需要同步获取 trace_id 而不构造完整 TraceContext 的场景。
func GenerateTraceIDStatic() string {
	return generateTraceID()
}

// ChildSpan 创建子 span（trace_id 不变，parent_span_id = 当前 span_id）
func (tc *TraceContext) ChildSpan() *TraceContext {
	if tc == nil {
		return NewTraceContext("", "")
	}
	tc.mu.RLock()
	parent := tc.spanID
	traceID := tc.traceID
	tc.mu.RUnlock()
	return &TraceContext{
		traceID:      traceID,
		spanID:       generateSpanID(),
		parentSpanID: parent,
	}
}

// InjectContext 将 trace_id 注入 context（与 logger.WithTraceID 兼容）
func (tc *TraceContext) InjectContext(ctx context.Context) context.Context {
	if tc == nil || ctx == nil {
		return ctx
	}
	return logger.WithTraceID(ctx, tc.traceID)
}

func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {

		return uuid.New().String()
	}
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {

		id := uuid.New().String()
		return id[:8] + id[9:13]
	}
	return hex.EncodeToString(b)
}

// TraceEventPublisher 追踪事件发布器接口
type TraceEventPublisher interface {
	Publish(event model.TraceEvent)
}

// TraceEventSubscriber 追踪事件订阅器接口
type TraceEventSubscriber interface {
	OnEvent(event model.TraceEvent)
}

// InMemoryTraceBus 进程内追踪事件总线（默认实现）
// 支持多订阅者，事件缓冲区 1024，背压时丢弃事件并记日志
type InMemoryTraceBus struct {
	mu          sync.RWMutex
	subscribers []TraceEventSubscriber
	eventQueue  chan model.TraceEvent
	stopped     atomic.Bool
}

// NewInMemoryTraceBus 创建进程内追踪事件总线
func NewInMemoryTraceBus() *InMemoryTraceBus {
	bus := &InMemoryTraceBus{
		eventQueue: make(chan model.TraceEvent, 1024),
	}
	go bus.dispatch()
	return bus
}

// Subscribe 订阅追踪事件
func (b *InMemoryTraceBus) Subscribe(sub TraceEventSubscriber) {
	if sub == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, sub)
}

// Publish 发布事件（非阻塞，缓冲区满时丢弃）
func (b *InMemoryTraceBus) Publish(event model.TraceEvent) {
	if b.stopped.Load() {
		return
	}
	select {
	case b.eventQueue <- event:
	default:
		logger.Warnf("[TraceBus] event queue full, dropping event trace_id=%s span_id=%s", event.TraceID, event.SpanID)
	}
}

// Stop 停止事件总线
func (b *InMemoryTraceBus) Stop() {
	if b.stopped.CompareAndSwap(false, true) {
		close(b.eventQueue)
	}
}

func (b *InMemoryTraceBus) dispatch() {
	for event := range b.eventQueue {
		b.mu.RLock()
		subs := make([]TraceEventSubscriber, len(b.subscribers))
		copy(subs, b.subscribers)
		b.mu.RUnlock()
		for _, sub := range subs {
			func() {
				defer func() {
					if r := recover(); r != nil {
						logger.Errorf("[TraceBus] subscriber panic: %v\n%s", r, debug.Stack())
					}
				}()
				sub.OnEvent(event)
			}()
		}
	}
}

var (
	globalTraceBus     *InMemoryTraceBus
	globalTraceBusOnce sync.Once
)

// InitGlobalTraceBus 初始化全局追踪事件总线
func InitGlobalTraceBus() *InMemoryTraceBus {
	globalTraceBusOnce.Do(func() {
		globalTraceBus = NewInMemoryTraceBus()
	})
	return globalTraceBus
}

// GetGlobalTraceBus 获取全局追踪事件总线（懒初始化，并发安全）。
// 不能"先裸读 globalTraceBus、为 nil 才去 Init"：那次读与 Once 闭包内的写之间
// 无同步，`-race` 会判成数据竞争。统一走 Once.Do：既保证只建一个 bus，也让
// 返回值对每个调用方都是已发布的值（happens-after）。
func GetGlobalTraceBus() *InMemoryTraceBus {
	globalTraceBusOnce.Do(func() {
		globalTraceBus = NewInMemoryTraceBus()
	})
	return globalTraceBus
}

// PublishTraceEvent 发布追踪事件（便捷方法）
// 自动设置 timestamp；trace_id/span_id 缺失时自动生成
func PublishTraceEvent(event model.TraceEvent) {
	if event.TraceID == "" {
		event.TraceID = generateTraceID()
	}
	if event.SpanID == "" {
		event.SpanID = generateSpanID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	bus := GetGlobalTraceBus()
	if bus != nil {
		bus.Publish(event)
	}
}

// PublishLLMCall 发布 LLM 调用事件
func PublishLLMCall(tenantID uint, traceID, spanID, parentSpanID, provider, modelName string, durationMs int64, status string, metadata map[string]any) {
	PublishTraceEvent(model.TraceEvent{
		TenantID:     tenantID,
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpanID,
		Kind:         model.TraceSpanKindLLMCall,
		Service:      "llm",
		Operation:    "chat_completion",
		DurationMs:   durationMs,
		Status:       status,
		Metadata: mergeMetadata(metadata, map[string]any{
			"provider": provider,
			"model":    modelName,
		}),
	})
}

func mergeMetadata(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// MarshalMetadata 序列化 metadata 为 JSON 字符串（便于持久化）
func MarshalMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return "{}"
	}
	b, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// UnmarshalMetadata 反序列化 metadata
func UnmarshalMetadata(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}
