// sales_cockpit_repo.go 驾驶舱聚合查询仓储。
package repository

import (
	"context"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

type SalesCockpitRepository struct {
	db *gorm.DB
}

// NewSalesCockpitRepository 构造（无参，内部取全局 DB）。
func NewSalesCockpitRepository() *SalesCockpitRepository {
	return NewSalesCockpitRepositoryWithDB(_db.GetDB())
}

func NewSalesCockpitRepositoryWithDB(db *gorm.DB) *SalesCockpitRepository {
	return &SalesCockpitRepository{db: db}
}

// CountWhere 任意表条件计数（表名与条件由调用方传入，仅内部聚合使用，无用户输入拼接）
func (r *SalesCockpitRepository) CountWhere(ctx context.Context, table, cond string, args ...any) int64 {
	if r.db == nil {
		return 0
	}
	var n int64
	if err := r.db.WithContext(ctx).Table(table).Where(cond, args...).Count(&n).Error; err != nil {
		return 0
	}
	return n
}

// LLMRouteStats 驾驶舱：LLM 路由调用分布。
func (r *SalesCockpitRepository) LLMRouteStats(ctx context.Context, since string) []map[string]any {
	if r.db == nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if err := r.db.WithContext(ctx).Table("llm_routing_logs").
		Select("scenario, provider, COUNT(*) AS calls, "+
			"COALESCE(AVG(latency_ms),0) AS avg_latency, COALESCE(SUM(cost),0) AS total_cost").
		Where("created_at >= ?", since).
		Group("scenario, provider").
		Order("calls DESC").Limit(10).
		Scan(&out).Error; err != nil {
		return []map[string]any{}
	}
	return out
}

// ChannelHealth 驾驶舱：平台账号健康分布
func (r *SalesCockpitRepository) ChannelHealth(ctx context.Context) []map[string]any {
	if r.db == nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if err := r.db.WithContext(ctx).Table("platform_accounts").
		Select("platform, status, COUNT(*) AS cnt").
		Group("platform, status").
		Order("cnt DESC").Limit(30).
		Scan(&out).Error; err != nil {
		return []map[string]any{}
	}
	return out
}

// IntentDistribution 驾驶舱：细粒度意图分布
func (r *SalesCockpitRepository) IntentDistribution(ctx context.Context, since time.Time) []map[string]any {
	if r.db == nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if err := r.db.WithContext(ctx).Table("intent_records").
		Select("intent_type AS intent, COUNT(*) AS cnt").
		Where("created_at >= ? AND source = 'fine_grained'", since).
		Group("intent_type").
		Order("cnt DESC").Limit(12).
		Scan(&out).Error; err != nil {
		return []map[string]any{}
	}
	return out
}

// TopTools 驾驶舱：message_trace 按 node 分组的调用次数
func (r *SalesCockpitRepository) TopTools(ctx context.Context, since time.Time) []map[string]any {
	if r.db == nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if err := r.db.WithContext(ctx).Table("message_trace").
		Select("node AS tool_name, COUNT(*) AS calls").
		Where("created_at >= ?", since).
		Group("node").
		Order("calls DESC").Limit(10).
		Scan(&out).Error; err != nil {
		return []map[string]any{}
	}
	return out
}
