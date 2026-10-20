// trace_event.go 全链路追踪事件模型（表 trace_events）。
package model

import "time"

// TraceSpanKind span 类型，即 trace_events.kind 列的值域。
type TraceSpanKind string

const (
	TraceSpanKindLLMCall  TraceSpanKind = "llm_call"
	TraceSpanKindToolCall TraceSpanKind = "tool_call"
	TraceSpanKindDBOp     TraceSpanKind = "db_op"
	TraceSpanKindLog      TraceSpanKind = "log"
)

// TraceEvent 一条 span 事件：同一 trace_id 下的多行按 parent_span_id 组成 span 树。
type TraceEvent struct {
	TenantID uint `gorm:"not null;default:1;index" json:"tenant_id"`

	ID           uint64         `json:"id" gorm:"primaryKey;autoIncrement"`
	TraceID      string         `json:"trace_id" gorm:"type:varchar(64);not null;index"`
	SpanID       string         `json:"span_id" gorm:"type:varchar(64);not null;uniqueIndex"`
	ParentSpanID string         `json:"parent_span_id,omitempty" gorm:"type:varchar(64);index"`
	Kind         TraceSpanKind  `json:"kind" gorm:"type:varchar(32);not null;index"`
	Service      string         `json:"service" gorm:"type:varchar(64);not null"`
	Operation    string         `json:"operation" gorm:"type:varchar(128);not null"`
	DurationMs   int64          `json:"duration_ms" gorm:"default:0"`
	Status       string         `json:"status" gorm:"type:varchar(16);default:'ok'"`
	Metadata     map[string]any `json:"metadata,omitempty" gorm:"type:text;serializer:json"`
	Timestamp    time.Time      `json:"timestamp" gorm:"index"`
}

// TableName GORM 表名
func (TraceEvent) TableName() string { return "trace_events" }
