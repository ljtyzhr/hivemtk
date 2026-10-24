// observability_dashboard.go 统一看板口径的**只读**仓储（D2 + B2，specs/007）。
//
// 本文件只做「把既有表读出聚合行」，不做任何判定：送达率、在线率、渠道状态、
// 转化率全部在 service 层算——这样判定口径只有 service 一处，用例可以不连库直接测。
// 全仓库层统一 `db.WithContext(ctx)`：租户隔离由既有 tenant callback 盖章，
// 本层不自己拼 tenant_id 条件（与 bridge_selector_health 仓储同口径）。
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ObservabilityDashboardRepository 统一看板只读仓储。
type ObservabilityDashboardRepository interface {
	// LLMCost 窗口内按 (provider, scenario) 聚合 llm_routing_logs。
	LLMCost(ctx context.Context, since time.Time) ([]ObsDashLLMCostRow, error)
	// Delivery 窗口内按渠道（message_hub.platform）聚合出站送达计数。
	Delivery(ctx context.Context, since time.Time) ([]ObsDashDeliveryRow, error)
	// Funnel 窗口内 message_trace 六个生命周期节点的去重 trace 计数。
	Funnel(ctx context.Context, since time.Time) ([]ObsDashFunnelStageRow, error)
	// ChannelAccounts 按渠道聚合 bridge_accounts 的账号总数与在线数。
	// 在线数用 bridgeOnlineSQLPredicate（同一谓词，见 bridge_online_predicate.go），
	// graceSeconds 由调用方从 service.BridgeOnlineGraceWindow 读好传入。
	ChannelAccounts(ctx context.Context, graceSeconds int) ([]ObsDashChannelAccountRow, error)
	// SelectorHealthRows 当前租户全部选择器健康计数（复用 B1-a 表，判定在 service）。
	SelectorHealthRows(ctx context.Context) ([]model.BridgeSelectorHealth, error)
	// DomainBanSummary domain_pool 的封禁/风险域名计数。
	DomainBanSummary(ctx context.Context) (ObsDashDomainBanSummaryRow, error)
}

// ObsDashLLMCostRow 一条 (provider, scenario) 聚合。
type ObsDashLLMCostRow struct {
	Provider         string  `gorm:"column:provider"`
	Scenario         string  `gorm:"column:scenario"`
	Calls            int64   `gorm:"column:calls"`
	SuccessCalls     int64   `gorm:"column:success_calls"`
	PromptTokens     int64   `gorm:"column:prompt_tokens"`
	CompletionTokens int64   `gorm:"column:completion_tokens"`
	Cost             float64 `gorm:"column:cost"`
	LatencyMs        int64   `gorm:"column:latency_ms"`
}

// ObsDashDeliveryRow 一条渠道送达聚合。
type ObsDashDeliveryRow struct {
	Channel   string `gorm:"column:channel"`
	Delivered int64  `gorm:"column:delivered"`
	Failed    int64  `gorm:"column:failed"`
	Inflight  int64  `gorm:"column:inflight"`
}

// ObsDashFunnelStageRow 一个生命周期节点的去重 trace 计数。
type ObsDashFunnelStageRow struct {
	Node string `gorm:"column:node"`
	Cnt  int64  `gorm:"column:cnt"`
}

// ObsDashChannelAccountRow 一条渠道账号聚合。
type ObsDashChannelAccountRow struct {
	Channel  string `gorm:"column:channel"`
	Accounts int64  `gorm:"column:accounts"`
	Online   int64  `gorm:"column:online"`
}

// ObsDashDomainBanSummaryRow domain_pool 封禁/风险域名计数。
type ObsDashDomainBanSummaryRow struct {
	Blacklisted int64 `gorm:"column:blacklisted"`
	Total       int64 `gorm:"column:total"`
}

type observabilityDashboardRepo struct {
	db *gorm.DB
}

// NewObservabilityDashboardRepository 构造仓储。db 为 nil 时返回 nil 接口——
// 与仓内其它 repository 同口径（ARC-01：未初始化守卫由调用方用 repo == nil 表达）。
func NewObservabilityDashboardRepository(db *gorm.DB) ObservabilityDashboardRepository {
	if db == nil {
		return nil
	}
	return &observabilityDashboardRepo{db: db}
}

func (r *observabilityDashboardRepo) LLMCost(ctx context.Context, since time.Time) ([]ObsDashLLMCostRow, error) {
	var rows []ObsDashLLMCostRow
	err := r.db.WithContext(ctx).Table("llm_routing_logs").
		Select("provider, scenario, COUNT(*) AS calls, "+
			"COALESCE(SUM(CASE WHEN success THEN 1 ELSE 0 END),0) AS success_calls, "+
			"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, "+
			"COALESCE(SUM(completion_tokens),0) AS completion_tokens, "+
			"COALESCE(SUM(cost),0) AS cost, "+
			"COALESCE(SUM(latency_ms),0) AS latency_ms").
		Where("created_at >= ?", since).
		Group("provider, scenario").
		Order("provider asc, scenario asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *observabilityDashboardRepo) Delivery(ctx context.Context, since time.Time) ([]ObsDashDeliveryRow, error) {
	var rows []ObsDashDeliveryRow
	// 送达率的分母是「已了结」行（delivered + failed）；pending/acked 是「在途」，
	// 塞进分母会把刚发出的消息算成「未送达」压低送达率——所以单列 inflight 展示。
	err := r.db.WithContext(ctx).Table("message_hub").
		Select("platform AS channel, "+
			"COALESCE(SUM(CASE WHEN status='delivered' THEN 1 ELSE 0 END),0) AS delivered, "+
			"COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0) AS failed, "+
			"COALESCE(SUM(CASE WHEN status NOT IN ('delivered','failed') THEN 1 ELSE 0 END),0) AS inflight").
		Where("direction = 'outbound' AND created_at >= ?", since).
		Group("platform").
		Order("platform asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *observabilityDashboardRepo) Funnel(ctx context.Context, since time.Time) ([]ObsDashFunnelStageRow, error) {
	var rows []ObsDashFunnelStageRow
	// 六个生命周期节点固定存在于 tracing.nodeOrder；agent_turn / tool_call 的 node 值
	// 不同（挂在 ai_dispatch 之下），天然被 node IN(...) 排除，无需再滤 span_kind。
	err := r.db.WithContext(ctx).Table("message_trace").
		Select("node, COUNT(DISTINCT trace_id) AS cnt").
		Where("node IN ('ingest','ai_dispatch','outbound_enqueue','inbox_sync','downlink_fetch','delivered_ack')").
		Where("created_at >= ?", since).
		Group("node").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *observabilityDashboardRepo) ChannelAccounts(ctx context.Context, graceSeconds int) ([]ObsDashChannelAccountRow, error) {
	var rows []ObsDashChannelAccountRow
	// 在线判定必须与其它三个读侧（渠道总览/主动触达/离线回扫）同一条 predicate：
	// 各写一份 SQL 会量出互相矛盾的「在线数」。占位符顺序 (offline, graceSeconds)。
	err := r.db.WithContext(ctx).Table("bridge_accounts").
		Select("channel, COUNT(*) AS accounts, "+
			"COALESCE(SUM(CASE WHEN "+bridgeOnlineSQLPredicate+" THEN 1 ELSE 0 END),0) AS online",
			bridgeStatusOffline, graceSeconds).
		Group("channel").
		Order("channel asc").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *observabilityDashboardRepo) SelectorHealthRows(ctx context.Context) ([]model.BridgeSelectorHealth, error) {
	var rows []model.BridgeSelectorHealth
	err := r.db.WithContext(ctx).
		Order("channel asc, field asc").
		Limit(BridgeSelectorHealthListLimit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *observabilityDashboardRepo) DomainBanSummary(ctx context.Context) (ObsDashDomainBanSummaryRow, error) {
	var row ObsDashDomainBanSummaryRow
	err := r.db.WithContext(ctx).Table("domain_pool").
		Select("COALESCE(SUM(CASE WHEN on_blacklist THEN 1 ELSE 0 END),0) AS blacklisted, COUNT(*) AS total").
		Scan(&row).Error
	if err != nil {
		return ObsDashDomainBanSummaryRow{}, err
	}
	return row, nil
}
