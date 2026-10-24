// Package model LLM 用量日聚合表（T3 观测报表）。
//
// 设计文档：docs/architecture/design/T3-observability-reports.md
//
// 读侧：报表 API / 看板默认读本表（天粒度），当日未聚合数据由 API 实时查
// llm_routing_logs 兜底；写入侧：聚合任务增量 upsert（冲突目标 = 迁移建的
// uk_llm_usage_daily_dims 复合唯一索引，列序严格一致——42P10 教训）。
// 模型有意不用 uniqueIndex 标签（同 v3.71.0 / geo_citation_snapshots 教训），
// 索引形状以迁移显式 SQL 为准。
package model

import "time"

// LLMUsageDaily LLM 用量日聚合（维度：租户×天×provider×model×scenario×channel×source×error_class）
type LLMUsageDaily struct {
	ID int64 `gorm:"primaryKey;autoIncrement" json:"id"`

	// TenantID 租户列：隔离靠 GORM callback，判据是表有无 tenant_id 列
	TenantID uint `gorm:"not null;default:1;index:idx_llm_usage_daily_tenant_date,priority:1" json:"tenant_id"`
	// StatDate 统计日 '2026-10-24'（本地时区日）
	StatDate string `gorm:"type:varchar(10);not null;index:idx_llm_usage_daily_tenant_date,priority:2" json:"stat_date"`
	Provider string `gorm:"type:varchar(64);not null;default:''" json:"provider"`
	Model    string `gorm:"type:varchar(128);not null;default:'';index:idx_llm_usage_daily_model" json:"model"`
	Scenario string `gorm:"type:varchar(64);not null;default:''" json:"scenario"`
	// Channel 渠道：web(官网)/app(用户端)/extension(扩展)/api(开放 API)；''=未知档
	Channel string `gorm:"type:varchar(30);not null;default:''" json:"channel"`
	// Source 计费来源：self(自有)/platform(平台垫资)
	Source string `gorm:"type:varchar(16);not null;default:''" json:"source"`
	// ErrorClass 错误分类；成功行恒为 ''
	ErrorClass string `gorm:"type:varchar(32);not null;default:''" json:"error_class"`

	CallCount        int64 `gorm:"not null;default:0" json:"call_count"`
	ErrorCount       int64 `gorm:"not null;default:0" json:"error_count"`
	CacheHitCount    int64 `gorm:"not null;default:0" json:"cache_hit_count"`
	PromptTokens     int64 `gorm:"not null;default:0" json:"prompt_tokens"`
	CompletionTokens int64 `gorm:"not null;default:0" json:"completion_tokens"`
	InputCostCents   int64 `gorm:"not null;default:0" json:"input_cost_cents"`
	OutputCostCents  int64 `gorm:"not null;default:0" json:"output_cost_cents"`
	TotalCostCents   int64 `gorm:"not null;default:0" json:"total_cost_cents"`
	// LatencySumMs / SumSquaresLat 供二次聚合 AVG（含 variance 推导）
	LatencySumMs  int64 `gorm:"not null;default:0" json:"latency_sum_ms"`
	P50LatencyMs  int64 `gorm:"not null;default:0" json:"p50_latency_ms"`
	P95LatencyMs  int64 `gorm:"not null;default:0" json:"p95_latency_ms"`
	SumSquaresLat int64 `gorm:"column:sum_squares_latency_ms;not null;default:0" json:"sum_squares_latency_ms"`

	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 表名
func (LLMUsageDaily) TableName() string { return "llm_usage_daily" }

// ErrClass 错误分类常量（与 service.ClassifyError 输出一致）
const (
	ErrClassTimeout       = "timeout"
	ErrClassRateLimit     = "rate_limit"
	ErrClassAuth          = "auth"
	ErrClassUpstream5xx   = "upstream_5xx"
	ErrClassContextLength = "context_length"
	ErrClassEmptyResponse = "empty_response"
	ErrClassInsuffBalance = "insufficient_balance"
	ErrClassUnknown       = "unknown"
)
