// reflux collector：触发源异步沉淀 unknown_questions（T1）。
//
// 有界队列 + 后台 goroutine 消费：问答主链路只做非阻塞入队；
// 队满丢弃并计数（丢弃率日志），panic 由 SafeGo 兜底。
package reflux

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// Event 一次未知问题触发事件
type Event struct {
	Question     string  // 原始问句
	Source       string  // model.UQSource*
	SourceID     string  // signal_id / decision_id / bad_case_id / csat_id
	SessionID    string
	CustomerID   string
	AgentID      uint
	IntentType   string
	LowConfScore float64
}

// Collector 异步收集器
type Collector struct {
	db    *gorm.DB
	ch    chan Event
	wg    sync.WaitGroup
	drops int64 // 队满丢弃计数（观测丢弃率）

	mu        sync.Mutex
	clusterer *Clusterer
	enabled   bool
}

// NewCollector 构造并启动消费 goroutine。
// queueSize 有界队列容量；refluxEnabled=false 时仅建结构不消费（灰度开关）。
func NewCollector(db *gorm.DB, queueSize int, refluxEnabled bool) *Collector {
	if queueSize <= 0 {
		queueSize = 512
	}
	c := &Collector{db: db, ch: make(chan Event, queueSize), enabled: refluxEnabled}
	if refluxEnabled {
		c.wg.Add(1)
		go c.consumeLoop()
	}
	return c
}

// SetClusterer 注入归簇器（延迟装配；nil = 只沉淀不归簇）
func (c *Collector) SetClusterer(cl *Clusterer) {
	c.mu.Lock()
	c.clusterer = cl
	c.mu.Unlock()
}

// Enabled 灰度开关状态
func (c *Collector) Enabled() bool { return c.enabled }

// Stop 停止消费（等待队列排空或超时）
func (c *Collector) Stop() {
	if !c.enabled {
		return
	}
	c.enabled = false
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Println("[reflux] collector stop timeout, queue not drained")
	}
}

// Emit 非阻塞入队；队满丢弃并计数。任何调用方（问答主链路）都不应被阻塞。
func (c *Collector) Emit(ev Event) {
	if c == nil || !c.enabled || ev.Question == "" {
		return
	}
	select {
	case c.ch <- ev:
	default:
		atomic.AddInt64(&c.drops, 1)
	}
}

// Drops 丢弃计数（观测用）
func (c *Collector) Drops() int64 { return atomic.LoadInt64(&c.drops) }

func (c *Collector) consumeLoop() {
	defer c.wg.Done()
	for ev := range c.ch {
		c.processOne(ev)
	}
}

// processOne 单事件处理：upsert 沉淀 + 归簇。失败仅记日志（不重试——下一同问句事件会再来）。
func (c *Collector) processOne(ev Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hash := QuestionHash(ev.Question)
	now := time.Now()
	// upsert：命中指纹则频次+1、刷新 last_seen；否则新建 draft
	res := c.db.WithContext(ctx).Exec(`INSERT INTO unknown_questions
		(tenant_id, question_hash, question, status, source, source_id, session_id, customer_id,
		 agent_id, intent_type, low_conf_score, frequency, first_seen_at, last_seen_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (tenant_id, question_hash) DO UPDATE SET
		 frequency = unknown_questions.frequency + 1,
		 last_seen_at = EXCLUDED.last_seen_at,
		 source_id = EXCLUDED.source_id`,
		0, hash, ev.Question, model.UQStatusDraft, ev.Source, ev.SourceID, ev.SessionID,
		ev.CustomerID, ev.AgentID, ev.IntentType, ev.LowConfScore,
		1, now, now, now, now,
	)
	if res.Error != nil {
		log.Printf("[reflux] upsert unknown_question: %v (q=%q)", res.Error, ev.Question)
		return
	}

	// 归簇（trgm 相似；失败不阻塞沉淀）
	if c.clusterer != nil {
		c.mu.Lock()
		cl := c.clusterer
		c.mu.Unlock()
		if err := cl.Assign(ctx, hash, ev.Question); err != nil {
			log.Printf("[reflux] cluster assign: %v", err)
		}
	}
}

// RefluxEmitter 回流收集器最小接口（消费方依赖此接口而非 *Collector，
// 便于测试注入与 nil 关闭语义）
type RefluxEmitter interface {
	Emit(ev Event)
}
