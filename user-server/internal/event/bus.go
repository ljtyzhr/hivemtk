// Package event 提供进程内事件总线基础设施。
//
// 设计目标：
//   - 解耦非关键路径（操作日志、统计上报、健康度上报等旁路事件）
//   - 异步、非阻塞发布，best-effort 投递（队列满则丢弃，不阻塞主流程）
//   - 全局单例模式，与 db.GetDB() / llm.GetGlobalDispatcher() 一致
//
// 不适用场景：
//   - 关键业务路径（如 SalesEngine 主链路、订单创建等不可丢失的操作）
//   - 跨进程通信（请使用消息队列）
//
// 落地状态： 试点 OperationLog 异步化
package event

import (
	"context"
	"reflect"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// Handler 事件处理器函数签名
type Handler func(evt Event) error

// Event 事件结构
type Event struct {
	Topic     string
	Payload   any
	Timestamp time.Time
	Source    string
}

// EventBus 进程内事件总线
//
// 特性：
//   - 异步发布：Publish 立即返回，事件由后台 worker 消费
//   - best-effort：队列满时丢弃新事件并记录日志，不阻塞调用方
//   - 至少一次投递：单个 handler 失败不影响其他 handler，但失败事件不会重试
//   - 优雅关闭：Stop() 等待所有已入队事件处理完成
type EventBus struct {
	subscribers    map[string][]Handler
	mu             sync.RWMutex
	queue          chan Event
	criticalQueue  chan Event
	criticalTopics map[string]bool
	stopCh         chan struct{}
	wg             sync.WaitGroup
	stopped        bool
	stopMu         sync.Mutex
}

// New 创建事件总线
//
// 参数：
//   - workerCount: 消费者协程数（默认 2）
//   - queueSize: 事件队列容量，满了丢弃新事件（默认 1024）
func New(workerCount, queueSize int) *EventBus {
	if workerCount <= 0 {
		workerCount = 2
	}
	if queueSize <= 0 {
		queueSize = 1024
	}
	criticalWorkers := workerCount
	if criticalWorkers < 4 {
		criticalWorkers = 4
	}
	b := &EventBus{
		subscribers:    make(map[string][]Handler),
		queue:          make(chan Event, queueSize),
		criticalQueue:  make(chan Event, queueSize),
		criticalTopics: map[string]bool{TopicCustomerMessageReceived: true},
		stopCh:         make(chan struct{}),
	}
	for i := 0; i < workerCount; i++ {
		b.wg.Add(1)
		go b.worker(b.queue, i)
	}
	for i := 0; i < criticalWorkers; i++ {
		b.wg.Add(1)
		go b.worker(b.criticalQueue, workerCount+i)
	}
	return b
}

// Subscribe 订阅主题
// 注意：Subscribe 必须在 Publish 之前调用（启动阶段注册），运行时不应动态增删
func (b *EventBus) Subscribe(topic string, handler Handler) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[topic] = append(b.subscribers[topic], handler)
}

// MarkCritical 将一个 topic 标记为关键事件，路由到独立的高优先队列/worker 池。
// 关键事件（如客户消息）与旁路事件（操作日志、统计上报）隔离，避免被洪峰挤掉。
func (b *EventBus) MarkCritical(topic string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.criticalTopics == nil {
		b.criticalTopics = make(map[string]bool)
	}
	b.criticalTopics[topic] = true
}

// Publish 异步发布事件（非阻塞）
//
// 行为：
//   - 队列未满：事件入队，立即返回
//   - 队列已满：丢弃事件并记录日志，立即返回（best-effort）
//   - 总线已停止：丢弃事件并记录日志
func (b *EventBus) Publish(evt Event) {
	if b == nil {
		return
	}
	b.stopMu.Lock()
	if b.stopped {
		b.stopMu.Unlock()
		logger.Warnf("[EventBus] bus stopped, dropping event topic=%s", evt.Topic)
		return
	}
	b.stopMu.Unlock()

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}

	b.mu.RLock()
	target := b.queue
	if b.criticalTopics[evt.Topic] {
		target = b.criticalQueue
	}
	b.mu.RUnlock()
	select {
	case target <- evt:
	default:
		logger.Warnf("[EventBus] queue full, dropping event topic=%s critical=%v", evt.Topic, target == b.criticalQueue)
	}
	_ = len(b.queue)
	_ = len(b.criticalQueue)
}

func (b *EventBus) worker(q chan Event, id int) {
	defer b.wg.Done()
	for {
		select {
		case evt := <-q:
			b.dispatch(evt)
		case <-b.stopCh:
			b.drainFrom(q)
			return
		}
	}
}

func (b *EventBus) drainFrom(q chan Event) {
	for {
		select {
		case evt := <-q:
			b.dispatch(evt)
		default:
			return
		}
	}
}

func (b *EventBus) dispatch(evt Event) {
	b.mu.RLock()
	handlers := b.subscribers[evt.Topic]
	handlersCopy := make([]Handler, len(handlers))
	copy(handlersCopy, handlers)
	b.mu.RUnlock()

	for _, h := range handlersCopy {
		if err := h(evt); err != nil {
			logger.Errorf("[EventBus] handler error topic=%s err=%v", evt.Topic, err)
		}
	}
}

// Stop 优雅关闭：等待所有已入队事件处理完成
// 可重复调用（幂等）
func (b *EventBus) Stop() {
	if b == nil {
		return
	}
	b.stopMu.Lock()
	defer b.stopMu.Unlock()
	if b.stopped {
		return
	}
	b.stopped = true
	close(b.stopCh)
	b.wg.Wait()
}

// HasSubscribers 检查指定主题是否有订阅者（用于测试和调试）
func (b *EventBus) HasSubscribers(topic string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[topic]) > 0
}

var (
	globalBus *EventBus
	globalMu  sync.RWMutex
)

// SetGlobalBus 设置全局事件总线（main.go 启动阶段调用一次）
func SetGlobalBus(b *EventBus) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalBus = b
}

// GetGlobalBus 获取全局事件总线（未初始化返回 nil）
func GetGlobalBus() *EventBus {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalBus
}

// Publish 全局快捷发布函数
// 未初始化时为 no-op，调用方可安全使用
//
// ⚠️ 租户域事件请改用 `PublishCtx`（发布点不再各自拷贝 `TenantID`）。
// 保留本函数只为"确实与租户无关"的事件；`scripts/check-event-publish-tenant.py`
// 禁止在 `internal/event` 之外直接调用它。
func Publish(topic string, payload any) {
	b := GetGlobalBus()
	if b == nil {
		return
	}
	warnIfTenantMissing(topic, payload)
	b.Publish(Event{
		Topic:   topic,
		Payload: payload,
	})
}

// PublishCtx 带 ctx 的发布口：**租户由本函数统一盖章**（T021 的收口）。
//
// 语义：ctx 里有租户 ⇒ 写进 payload 的 `TenantID` 字段（payload 自己已填的非 0 值不覆盖，
// 以便"行自证"的发布点钉死行归属）；ctx 里没有租户 ⇒ 原样透传，由 warnIfTenantMissing
// 在 on 模式下打出响声。
//
// 为什么不用"每个发布点自己写 `TenantID: tenantIDFromCtx(ctx)`"：
// 那是 40+ 处各写一遍的形状，漏一处就是订阅者拿到 0 —— 而 0 在 on 模式下要么 fail-closed
// 空转、要么落默认租户。执行者唯一 + 门禁，才不依赖"下一个人记得写"。
func PublishCtx(ctx context.Context, topic string, payload any) {
	Publish(topic, StampTenantFromCtx(ctx, payload))
}

// PublishEventCtx 同 PublishCtx，但入参是完整 Event（用于需要 Source/Timestamp 的发布点）。
func PublishEventCtx(ctx context.Context, evt Event) {
	b := GetGlobalBus()
	if b == nil {
		return
	}
	evt.Payload = StampTenantFromCtx(ctx, evt.Payload)
	warnIfTenantMissing(evt.Topic, evt.Payload)
	b.Publish(evt)
}

// StampTenantFromCtx 把 ctx 租户写进 payload 的 `TenantID` 字段；无该字段/已填/无租户则原样返回。
//
// 用反射而不是接口，是为了覆盖"已有 20+ 种载荷结构、且大都是值类型"的现状：
// 值类型会被复制成可寻址副本再改字段，调用方拿到的仍是原值（不产生副作用）。
func StampTenantFromCtx(ctx context.Context, payload any) any {
	if ctx == nil || payload == nil {
		return payload
	}
	tid, ok := tenant.FromContext(ctx)
	if !ok || tid == 0 {
		return payload
	}
	v := reflect.ValueOf(payload)
	if !v.IsValid() {
		return payload
	}
	target := v
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return payload
		}
		target = v.Elem()
	case reflect.Struct:
		// 值类型：复制一份可寻址副本，改副本，传副本
		cp := reflect.New(v.Type()).Elem()
		cp.Set(v)
		target = cp
	default:
		return payload
	}
	if target.Kind() != reflect.Struct {
		return payload
	}
	f := target.FieldByName("TenantID")
	if !f.IsValid() || !f.CanSet() || f.Kind() != reflect.Uint {
		return payload
	}
	if f.Uint() != 0 {
		return payload
	}
	f.SetUint(uint64(tid))
	if v.Kind() == reflect.Struct {
		return target.Interface()
	}
	return payload
}

// warnIfTenantMissing on 模式下"载荷带 TenantID 字段但值是 0" ⇒ 打响声。
//
// 这是"漏填"唯一能在运行期被看见的地方：不阻断发布（旁路事件丢了比晚到更糟），
// 但让 `[event]` 这行出现在日志里，而不是等订阅者静默空转。
func warnIfTenantMissing(topic string, payload any) {
	if !tenant.IsMultiTenant() || payload == nil {
		return
	}
	v := reflect.ValueOf(payload)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	f := v.FieldByName("TenantID")
	if !f.IsValid() || f.Kind() != reflect.Uint {
		return // 该载荷不含租户维度：与本门无关
	}
	if f.Uint() == 0 {
		logger.GetLogger().Warn().
			Str("topic", topic).
			Msg("[event] on 模式下发布的载荷 TenantID=0（发布点未带租户 ctx）——订阅者会 fail-closed 空转")
	}
}

// StopGlobal 停止全局事件总线（main.go 退出时调用）
func StopGlobal() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalBus != nil {
		globalBus.Stop()
		globalBus = nil
	}
}
