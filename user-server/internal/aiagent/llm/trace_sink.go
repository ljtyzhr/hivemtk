package llm

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// DBTraceSink 把 InMemoryTraceBus 派发的 TraceEvent 异步批量落库到 trace_events 表。
//
// 设计要点：
//   - 订阅者模式：实现 TraceEventSubscriber 接口，OnEvent 非阻塞写入自有缓冲。
//   - 双缓冲隔离：业务发布走 InMemoryTraceBus(1024) → DBTraceSink 自有缓冲(2048) → 批量落库，
//     即便 DB 慢也能 backpressure 到 sink 层，绝不阻塞业务主链路（与 tracing.MessageTrace 同思路）。
//   - 批量插入：每 500ms 或缓冲达到 128 时落一次，减小 DB 写入放大。
//   - 优雅退出：Stop 时排空缓冲并做最后一次落库，进程重启不丢尾段事件。
//   - 失败可观测：批量失败记 Error 日志（含 span_id/trace_id/批次大小），不静默吞错。
//   - 零依赖耦合：不依赖任何业务包；可独立注入 main.go 启动序列。
type DBTraceSink struct {
	db     *gorm.DB
	buffer chan model.TraceEvent

	stopCh   chan struct{}
	stopOnce sync.Once
	doneCh   chan struct{}

	dropped  int64
	inserted int64
}

const (
	DefaultDBSinkBufferSize   = 2048
	dbSinkBatchSize           = 128
	dbSinkFlushPeriod         = 500 * time.Millisecond
	DefaultDBSinkStopDeadline = 3 * time.Second
)

var dbSinkStopDeadlineProvider func() time.Duration

// SetDBSinkStopDeadlineProvider 注入 Trace DB Sink 停止等待上限；传 nil 视为不注入。
func SetDBSinkStopDeadlineProvider(fn func() time.Duration) {
	dbSinkStopDeadlineProvider = fn
}

// DBSinkStopDeadline 生效的停止等待上限。非正值一律回落兜底——0 会让
// Stop() 一进来就判定超时并强退，缓冲里的尾段 trace 事件全部丢掉。
func DBSinkStopDeadline() time.Duration {
	if p := dbSinkStopDeadlineProvider; p != nil {
		if d := p(); d > 0 {
			return d
		}
	}
	return DefaultDBSinkStopDeadline
}

var dbSinkBufferSizeProvider func() int

// SetDBSinkBufferSizeProvider 注入 Trace DB Sink 缓冲队列容量；传 nil 视为不注入。
func SetDBSinkBufferSizeProvider(fn func() int) { dbSinkBufferSizeProvider = fn }

// DBSinkBufferSize 生效的缓冲队列容量。非正值一律回落兜底——0 意味着 unbuffered
// channel，OnEvent 的非阻塞投递在高频 trace 下会大面积走 default 分支丢事件，
// 而丢弃只累加计数器、不报错，表现为"追踪数据莫名少了一截"。
func DBSinkBufferSize() int {
	if p := dbSinkBufferSizeProvider; p != nil {
		if n := p(); n > 0 {
			return n
		}
	}
	return DefaultDBSinkBufferSize
}

// NewDBTraceSink 构造 DB 订阅者；db 允许为空（此时事件将被丢弃并计数）。
func NewDBTraceSink(db *gorm.DB) *DBTraceSink {
	return &DBTraceSink{
		db:     db,
		buffer: make(chan model.TraceEvent, DBSinkBufferSize()),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start 启动后台批量落库 worker。
// 必须在 InitGlobalTraceBus 之后调用；调用方负责在进程退出时 Stop()。
func (s *DBTraceSink) Start() {
	go s.flushLoop()
	logger.GetLogger().Info().
		Int("buffer", DBSinkBufferSize()).
		Int("batch", dbSinkBatchSize).
		Dur("period", dbSinkFlushPeriod).
		Msg("[DBTraceSink] started")
}

// OnEvent 实现 TraceEventSubscriber。非阻塞：缓冲满则丢弃并计数。
func (s *DBTraceSink) OnEvent(event model.TraceEvent) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case s.buffer <- event:
	default:
		atomic.AddInt64(&s.dropped, 1)
	}
}

// Stats 返回插入/丢弃计数（运维/健康巡检用）。
func (s *DBTraceSink) Stats() (inserted, dropped int64) {
	return atomic.LoadInt64(&s.inserted), atomic.LoadInt64(&s.dropped)
}

// Stop 优雅停止：排空缓冲并做最后一次批量落库，进程退出不丢尾段事件。
// 幂等；多次调用安全。
func (s *DBTraceSink) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		select {
		case <-s.doneCh:
		case <-time.After(DBSinkStopDeadline()):
			logger.GetLogger().Warn().Msg("[DBTraceSink] stop deadline exceeded, forcing exit")
		}
	})
}

func (s *DBTraceSink) flushLoop() {
	defer close(s.doneCh)

	ticker := time.NewTicker(dbSinkFlushPeriod)
	defer ticker.Stop()

	batch := make([]model.TraceEvent, 0, dbSinkBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if s.db == nil {
			batch = batch[:0]
			return
		}
		rows := make(map[uint][]map[string]any)
		for _, e := range batch {
			rows[e.TenantID] = append(rows[e.TenantID], traceEventToRow(e))
		}
		for tenantID, group := range rows {
			if tenantID == 0 {
				if tenant.IsMultiTenant() {
					// 公开路由没有租户归属：多租户下 map 落库必被拒绝，逐批报错只会
					// 刷屏——跳过并计数（绝不借默认租户蒙混落库）。
					atomic.AddInt64(&s.dropped, int64(len(group)))
					logger.GetLogger().Debug().Int("batch", len(group)).Msg("[DBTraceSink] skip unbound events")
					continue
				}
				tenantID = tenant.DefaultTenantID
			}
			ctx := tenant.WithTenantID(context.Background(), tenantID)
			if err := s.db.WithContext(ctx).Table("trace_events").Create(group).Error; err != nil {
				logger.GetLogger().Error().
					Err(err).
					Int("batch", len(group)).
					Uint("tenant", tenantID).
					Msg("[DBTraceSink] batch insert failed")
			} else {
				atomic.AddInt64(&s.inserted, int64(len(group)))
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case ev := <-s.buffer:
			batch = append(batch, ev)
			if len(batch) >= dbSinkBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-s.stopCh:
			for {
				select {
				case ev := <-s.buffer:
					batch = append(batch, ev)
					if len(batch) >= dbSinkBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

func traceEventToRow(e model.TraceEvent) map[string]any {
	row := map[string]any{
		"trace_id":    e.TraceID,
		"span_id":     e.SpanID,
		"kind":        string(e.Kind),
		"service":     e.Service,
		"operation":   e.Operation,
		"duration_ms": e.DurationMs,
		"status":      e.Status,
		"timestamp":   e.Timestamp,
	}
	// map 形态绕过模型 Schema，隔离 callback 无从按上下文填 tenant_id；
	// 归属未知时不写这一列（多租户下由 callback fail-closed，不静默落默认租户）。
	if e.TenantID != 0 {
		row["tenant_id"] = e.TenantID
	}
	if e.ParentSpanID != "" {
		row["parent_span_id"] = e.ParentSpanID
	}
	row["metadata"] = MarshalMetadata(e.Metadata)
	return row
}
