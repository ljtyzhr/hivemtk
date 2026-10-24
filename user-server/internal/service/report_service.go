// Package service 报表服务（T3 观测报表）。
//
// 设计文档：docs/architecture/design/T3-observability-reports.md
// 数据流：llm_routing_logs（明细）→ llm_usage_daily（日聚合）→ 报表 API；
// 平台模型成本以 platform_llm_usage_records（真分列计价）为准，
// llm_routing_logs 的 splitCost 假拆分口径只用于延迟/错误聚合。
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"hivemtk-user/internal/model"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ============================================================
// 错误分类
// ============================================================

// ClassifyError 把错误文本归入固定错误类（关键字匹配，简单可测）。
// 返回值与 model.ErrClass* 常量一致；空串归 unknown（失败但无消息）。
func ClassifyError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case low == "":
		return model.ErrClassUnknown
	case containsAny(low, "context length", "context_length", "maximum context", "too many tokens", "token limit"):
		return model.ErrClassContextLength
	case containsAny(low, "rate limit", "rate_limit", "ratelimit", "429", "too many requests", "限流"):
		return model.ErrClassRateLimit
	case containsAny(low, "timeout", "timed out", "deadline exceeded", "context deadline", "连接超时", "超时"):
		return model.ErrClassTimeout
	case containsAny(low, "unauthorized", "401", "invalid api key", "authentication", "forbidden", "403", "鉴权"):
		return model.ErrClassAuth
	case containsAny(low, "余额不足", "insufficient balance", "insufficient_llm_balance"):
		return model.ErrClassInsuffBalance
	case containsAny(low, "500", "502", "503", "504", "internal server error", "bad gateway", "service unavailable", "gateway timeout"):
		return model.ErrClassUpstream5xx
	case containsAny(low, "empty response", "empty_response", "空响应", "no content", "响应为空"):
		return model.ErrClassEmptyResponse
	default:
		return model.ErrClassUnknown
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ============================================================
// 日聚合任务
// ============================================================

// DailyAggWindow 一次聚合的输入窗口
type DailyAggWindow struct {
	From time.Time // 含
	To   time.Time // 含（按 stat_date 落桶）
}

// ReportService 报表聚合与查询
type ReportService struct {
	db *gorm.DB
}

// NewReportService 构造
func NewReportService(db *gorm.DB) *ReportService { return &ReportService{db: db} }

// advisoryLockKey 聚合任务互斥锁键（本仓多实例防并发）
const advisoryLockKey = 830083

// aggregateWindow 不加锁的窗口聚合（测试直接调用；生产路径走 AggregateDaily 加锁版）
func (s *ReportService) aggregateWindow(ctx context.Context, from, to time.Time) error {
	_, err := s.aggregateWindowLocked(ctx, from, to)
	return err
}

// AggregateDaily 把 (from, to] 窗口内的 llm_routing_logs 明细聚合 upsert 进
// llm_usage_daily。幂等：同一窗口重跑结果一致（ON CONFLICT DO UPDATE 覆盖式写入）。
//
// advisory lock 走 dbutil.TryAdvisoryLock/ReleaseAdvisoryLock（GORM 形态，
// FROM (SELECT 1) AS t 的派生表写法封在 internal/pkg/db/advisory_lock.go，避免
// GORM 无 Model 时拼出 `SELECT <expr> FROM ` 的坏 SQL）；Group 用真实列名
// （Group("1") 在 PG 上是坏的）。
func (s *ReportService) AggregateDaily(ctx context.Context, from, to time.Time) error {
	locked, err := dbutil.TryAdvisoryLock(ctx, s.db, advisoryLockKey)
	if err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	if !locked {
		return nil // 其他实例正在聚合，本轮跳过（下轮增量会覆盖）
	}
	defer func() {
		// 释放不依赖调用方 ctx（其可能已随请求结束被取消）：与 geo/job_manager
		// 同形，用短超时 Background，保证锁一定被还回去。
		rctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = dbutil.ReleaseAdvisoryLock(rctx, s.db, advisoryLockKey)
	}()

	// 明细 → 维度桶聚合。列名分组；tenant_id 取 routing_logs 自身列（迁移 v3.6.0 起有此列）
	rows, err := s.aggregateWindowLocked(ctx, from, to)
	if err != nil {
		return err
	}
	logger.Infof("[Report] aggregated %d buckets into llm_usage_daily (%s ~ %s)", rows,
		from.Format("2006-01-02 15:04"), to.Format("2006-01-02 15:04"))
	return nil
}

// aggregateWindowLocked 实际聚合逻辑（不加锁），返回桶数
func (s *ReportService) aggregateWindowLocked(ctx context.Context, from, to time.Time) (int, error) {
	rows := []struct {
		TenantID         uint    `gorm:"column:tenant_id"`
		StatDate         string  `gorm:"column:stat_date"`
		Provider         string  `gorm:"column:provider"`
		Model            string  `gorm:"column:model"`
		Scenario         string  `gorm:"column:scenario"`
		Channel          string  `gorm:"column:channel"`
		Source           string  `gorm:"column:source"`
		ErrorClass       string  `gorm:"column:error_class"`
		CallCount        int64   `gorm:"column:call_count"`
		ErrorCount       int64   `gorm:"column:error_count"`
		CacheHitCount    int64   `gorm:"column:cache_hit_count"`
		PromptTokens     int64   `gorm:"column:prompt_tokens"`
		CompletionTokens int64   `gorm:"column:completion_tokens"`
		CostCents        float64 `gorm:"column:cost_cents"`
		LatencySumMs     int64   `gorm:"column:latency_sum_ms"`
		SumSquaresLat    int64   `gorm:"column:sum_squares_latency_ms"`
	}{}
	err := s.db.WithContext(ctx).Table("llm_routing_logs").
		Select(`
			tenant_id,
			to_char(created_at, 'YYYY-MM-DD') AS stat_date,
			provider,
			model,
			scenario,
			COALESCE(channel, '') AS channel,
			COALESCE(source, '') AS source,
			CASE WHEN success THEN '' ELSE COALESCE(NULLIF(error_class, ''), 'unknown') END AS error_class,
			COUNT(*) AS call_count,
			SUM(CASE WHEN success THEN 0 ELSE 1 END) AS error_count,
			SUM(CASE WHEN from_cache THEN 1 ELSE 0 END) AS cache_hit_count,
			SUM(prompt_tokens) AS prompt_tokens,
			SUM(completion_tokens) AS completion_tokens,
			SUM(CASE WHEN total_tokens > 0 THEN cost ELSE 0 END) * 100 AS cost_cents,
			SUM(latency_ms) AS latency_sum_ms,
			SUM(latency_ms * latency_ms) AS sum_squares_latency_ms`).
		Where("created_at >= ? AND created_at < ?", from, to).
		Group(`tenant_id, stat_date, provider, model, scenario, channel, source,
			CASE WHEN success THEN '' ELSE COALESCE(NULLIF(error_class, ''), 'unknown') END`).
		Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("aggregate routing_logs: %w", err)
	}

	for _, r := range rows {
		costCents := int64(r.CostCents + 0.5)
		avg := int64(0)
		if r.CallCount > 0 {
			avg = r.LatencySumMs / r.CallCount
		}
		// 覆盖式 upsert：冲突目标 = 迁移建的 uk_llm_usage_daily_dims 复合唯一索引
		//（列序严格一致——42P10 教训）。同一窗口重跑时整行覆盖，保证幂等。
		// 用 clause.OnConflict 表达（GORM 链式 API，禁止 .Exec 直连）。
		row := model.LLMUsageDaily{
			TenantID:         r.TenantID,
			StatDate:         r.StatDate,
			Provider:         r.Provider,
			Model:            r.Model,
			Scenario:         r.Scenario,
			Channel:          r.Channel,
			Source:           r.Source,
			ErrorClass:       r.ErrorClass,
			CallCount:        r.CallCount,
			ErrorCount:       r.ErrorCount,
			CacheHitCount:    r.CacheHitCount,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalCostCents:   costCents,
			LatencySumMs:     r.LatencySumMs,
			P50LatencyMs:     avg,
			P95LatencyMs:     avg,
			SumSquaresLat:    r.SumSquaresLat,
			UpdatedAt:        time.Now(),
		}
		if err := s.db.WithContext(ctx).
			Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "tenant_id"}, {Name: "stat_date"}, {Name: "provider"},
					{Name: "model"}, {Name: "scenario"}, {Name: "channel"},
					{Name: "source"}, {Name: "error_class"},
				},
				DoUpdates: clause.AssignmentColumns([]string{
					"call_count", "error_count", "cache_hit_count", "prompt_tokens",
					"completion_tokens", "total_cost_cents", "latency_sum_ms",
					"p50_latency_ms", "p95_latency_ms", "sum_squares_latency_ms", "updated_at",
				}),
			}).
			Create(&row).Error; err != nil {
			return 0, fmt.Errorf("upsert llm_usage_daily: %w", err)
		}
	}
	return len(rows), nil
}

// AggregateRecent 增量聚合最近 10 分钟（覆盖乱序写入），供定时任务调用
func (s *ReportService) AggregateRecent(ctx context.Context) error {
	now := time.Now()
	return s.AggregateDaily(ctx, now.Add(-10*time.Minute), now)
}

// RunDailyRecalc 日终重算前一天（00:10 调度调用）
func (s *ReportService) RunDailyRecalc(ctx context.Context) error {
	dayStart := time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	return s.AggregateDaily(ctx, dayStart, dayStart.Add(24*time.Hour))
}

// ============================================================
// 查询侧（报表 API）
// ============================================================

// ReportTotals 总览 KPI
type ReportTotals struct {
	CallCount        int64   `json:"call_count"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	CacheHitRate     float64 `json:"cache_hit_rate"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	InputCostCents   int64   `json:"input_cost_cents"`
	OutputCostCents  int64   `json:"output_cost_cents"`
	TotalCostCents   int64   `json:"total_cost_cents"`
	AvgLatencyMs     int64   `json:"avg_latency_ms"`
	P50LatencyMs     int64   `json:"p50_latency_ms"`
	P95LatencyMs     int64   `json:"p95_latency_ms"`
}

// Overview 聚合表（天粒度）+ 当日实时兜底。
// 成本口径：llm_routing_logs.cost 单位为元（浮点），聚合时 ×100 入 cents；
// 平台模型真分列成本本期以 routing_logs 成本口径出数，platform 表对账接口后置。
func (s *ReportService) Overview(ctx context.Context, from, to time.Time, channel, source string) (*ReportTotals, error) {
	out := &ReportTotals{}
	row := struct {
		CallCount        int64 `gorm:"column:call_count"`
		ErrorCount       int64 `gorm:"column:error_count"`
		CacheHitCount    int64 `gorm:"column:cache_hit_count"`
		PromptTokens     int64 `gorm:"column:prompt_tokens"`
		CompletionTokens int64 `gorm:"column:completion_tokens"`
		TotalCostCents   int64 `gorm:"column:total_cost_cents"`
		LatencySumMs     int64 `gorm:"column:latency_sum_ms"`
	}{}
	q := s.db.WithContext(ctx).Table("llm_usage_daily").
		Select(`COALESCE(SUM(call_count),0) AS call_count,
			COALESCE(SUM(error_count),0) AS error_count,
			COALESCE(SUM(cache_hit_count),0) AS cache_hit_count,
			COALESCE(SUM(prompt_tokens),0) AS prompt_tokens,
			COALESCE(SUM(completion_tokens),0) AS completion_tokens,
			COALESCE(SUM(total_cost_cents),0) AS total_cost_cents,
			COALESCE(SUM(latency_sum_ms),0) AS latency_sum_ms`).
		Where("stat_date >= ? AND stat_date < ?", from.Format("2006-01-02"), to.Format("2006-01-02"))
	q = applyChannelSource(q, channel, source)
	if err := q.Scan(&row).Error; err != nil {
		return nil, err
	}
	out.CallCount = row.CallCount
	out.ErrorCount = row.ErrorCount
	out.PromptTokens = row.PromptTokens
	out.CompletionTokens = row.CompletionTokens
	out.TotalCostCents = row.TotalCostCents
	if row.CallCount > 0 {
		out.ErrorRate = float64(row.ErrorCount) / float64(row.CallCount)
		out.CacheHitRate = float64(row.CacheHitCount) / float64(row.CallCount)
		out.AvgLatencyMs = row.LatencySumMs / row.CallCount
	}
	// P50/P95 当日实时查明细（聚合表存的是 AVG，P95 精确值在明细侧）
	var lat struct {
		P50 int64 `gorm:"column:p50"`
		P95 int64 `gorm:"column:p95"`
	}
	if err := s.db.WithContext(ctx).Table("llm_routing_logs").
		Select(`COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY latency_ms),0) AS p50,
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms),0) AS p95`).
		Where("created_at >= ? AND created_at < ?", from, to).
		Scan(&lat).Error; err == nil {
		out.P50LatencyMs = lat.P50
		out.P95LatencyMs = lat.P95
	}
	return out, nil
}

// TimeseriesPoint 时间序列点
type TimeseriesPoint struct {
	StatDate         string  `json:"date"`
	CallCount        int64   `json:"call_count"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalCostCents   int64   `json:"total_cost_cents"`
	AvgLatencyMs     int64   `json:"avg_latency_ms"`
}

// Timeseries 按天序列（列名分组，禁 Group("1")）
func (s *ReportService) Timeseries(ctx context.Context, from, to time.Time, model_, scenario, channel, source string) ([]TimeseriesPoint, error) {
	rows := []TimeseriesPoint{}
	q := s.db.WithContext(ctx).Table("llm_usage_daily").
		Select(`stat_date,
			SUM(call_count) AS call_count,
			SUM(error_count) AS error_count,
			SUM(prompt_tokens) AS prompt_tokens,
			SUM(completion_tokens) AS completion_tokens,
			SUM(total_cost_cents) AS total_cost_cents,
			CASE WHEN SUM(call_count) > 0 THEN SUM(latency_sum_ms)/SUM(call_count) ELSE 0 END AS avg_latency_ms`).
		Where("stat_date >= ? AND stat_date < ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Group("stat_date").Order("stat_date")
	q = applyDims(q, model_, scenario)
	q = applyChannelSource(q, channel, source)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].CallCount > 0 {
			rows[i].ErrorRate = float64(rows[i].ErrorCount) / float64(rows[i].CallCount)
		}
	}
	return rows, nil
}

// BreakdownRow 维度分解行
type BreakdownRow struct {
	Dim              string  `json:"dim"`
	CallCount        int64   `json:"call_count"`
	ErrorCount       int64   `json:"error_count"`
	ErrorRate        float64 `json:"error_rate"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalCostCents   int64   `json:"total_cost_cents"`
	P95LatencyMs     int64   `json:"p95_latency_ms"`
}

var breakdownCols = map[string]string{
	"model":       "model",
	"scenario":    "scenario",
	"channel":     "channel",
	"error_class": "error_class",
	"provider":    "provider",
}

// Breakdown 单维度分解；by 必须在白名单内（防注入）
func (s *ReportService) Breakdown(ctx context.Context, from, to time.Time, by, model_, scenario, channel, source string) ([]BreakdownRow, error) {
	col, ok := breakdownCols[by]
	if !ok {
		return nil, fmt.Errorf("by 参数仅支持 model|scenario|channel|error_class|provider")
	}
	rows := []BreakdownRow{}
	q := s.db.WithContext(ctx).Table("llm_usage_daily").
		Select(col+` AS dim,
			SUM(call_count) AS call_count,
			SUM(error_count) AS error_count,
			SUM(completion_tokens) AS completion_tokens,
			SUM(total_cost_cents) AS total_cost_cents,
			COALESCE(MAX(p95_latency_ms),0) AS p95_latency_ms`).
		Where("stat_date >= ? AND stat_date < ?", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Group(col).Order("call_count DESC")
	q = applyDims(q, model_, scenario)
	q = applyChannelSource(q, channel, source)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].CallCount > 0 {
			rows[i].ErrorRate = float64(rows[i].ErrorCount) / float64(rows[i].CallCount)
		}
		if rows[i].Dim == "" {
			rows[i].Dim = "(未知)"
		}
	}
	return rows, nil
}

// ErrorRow 错误明细行
type ErrorRow struct {
	TraceID    string    `json:"trace_id"`
	Scenario   string    `json:"scenario"`
	Provider   string    `json:"provider"`
	Model      string    `json:"model"`
	ErrorClass string    `json:"error_class"`
	ErrorMsg   string    `json:"error_msg"`
	LatencyMs  int64     `json:"latency_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// ErrorReport 错误类聚合 + 失败明细（明细表直查，量小）
func (s *ReportService) ErrorReport(ctx context.Context, from, to time.Time, errorClass string, page, pageSize int) (map[string]any, error) {
	agg := []BreakdownRow{}
	aq := s.db.WithContext(ctx).Table("llm_usage_daily").
		Select(`error_class AS dim, SUM(error_count) AS call_count`).
		Where("stat_date >= ? AND stat_date < ? AND error_count > 0", from.Format("2006-01-02"), to.Format("2006-01-02")).
		Group("error_class")
	if err := aq.Scan(&agg).Error; err != nil {
		return nil, err
	}
	dq := s.db.WithContext(ctx).Table("llm_routing_logs").
		Select(`trace_id, scenario, provider, model,
			COALESCE(NULLIF(error_class,''),'unknown') AS error_class,
			LEFT(COALESCE(error_msg,''),500) AS error_msg,
			latency_ms, created_at`).
		Where("success = false AND created_at >= ? AND created_at < ?", from, to)
	if errorClass != "" {
		dq = dq.Where("COALESCE(NULLIF(error_class,''),'unknown') = ?", errorClass)
	}
	var total int64
	if err := dq.Count(&total).Error; err != nil {
		return nil, err
	}
	rows := []ErrorRow{}
	if err := dq.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return map[string]any{"by_class": agg, "total": total, "rows": rows,
		"page": page, "page_size": pageSize}, nil
}

func applyDims(q *gorm.DB, model_, scenario string) *gorm.DB {
	if model_ != "" {
		q = q.Where("model = ?", model_)
	}
	if scenario != "" {
		q = q.Where("scenario = ?", scenario)
	}
	return q
}

func applyChannelSource(q *gorm.DB, channel, source string) *gorm.DB {
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	if source != "" {
		q = q.Where("source = ?", source)
	}
	return q
}
