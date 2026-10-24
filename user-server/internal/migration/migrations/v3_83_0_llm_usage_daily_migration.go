// Package migrations 数据库迁移集合。
package migrations

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// LLMUsageDailyMigration v3.83.0（T3 观测报表）：
//  1. 新建日聚合表 llm_usage_daily（维度复合唯一索引，供报表 API 与看板读取，
//     与明细表量级解耦）；
//  2. 明细表 llm_routing_logs / platform_llm_usage_records 补 channel、error_class 列
//     （渠道与错误分类维度）；
//  3. 存量失败行 error_class 回填 'unknown'（success=false 且 error_msg 非空）。
//
// 幂等：建表/加列用 AutoMigrate（幂等），索引用 DO block 判存，回填带条件。
// 注意：复合唯一索引必须用显式 SQL 建，upsert 冲突目标与列序严格一致
// （savePlatformProviders 42P10 前车之鉴；模型有意不用 uniqueIndex 标签）。
type LLMUsageDailyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*LLMUsageDailyMigration)(nil)

// NewLLMUsageDailyMigration 构造 v3.83.0 迁移
func NewLLMUsageDailyMigration(db *gorm.DB) *LLMUsageDailyMigration {
	return &LLMUsageDailyMigration{db: db}
}

func (m *LLMUsageDailyMigration) Version() string { return "v3.83.0" }

func (m *LLMUsageDailyMigration) Name() string {
	return "llm_usage_daily + 渠道/错误分类列"
}

func (m *LLMUsageDailyMigration) Description() string {
	return "T3 观测报表：新建 llm_usage_daily 日聚合表（复合唯一索引）；" +
		"llm_routing_logs / platform_llm_usage_records 补 channel、error_class 列；" +
		"存量失败行 error_class 回填 unknown"
}

func (m *LLMUsageDailyMigration) Up(ctx context.Context) error {
	// 1) 日聚合表（模型定义见 model/llm_usage_daily.go；AutoMigrate 幂等建表/加列）
	if err := m.db.WithContext(ctx).AutoMigrate(&modelLLMUsageDaily{}); err != nil {
		return fmt.Errorf("automigrate llm_usage_daily: %w", err)
	}
	// 复合唯一索引：显式 SQL，判存后创建（列序即 upsert 冲突目标列序）
	const idxName = "uk_llm_usage_daily_dims"
	var idxCount int64
	if err := m.db.WithContext(ctx).Raw(
		`SELECT count(*) FROM pg_indexes WHERE indexname = ?`, idxName).Scan(&idxCount).Error; err != nil {
		return fmt.Errorf("check index %s: %w", idxName, err)
	}
	if idxCount == 0 {
		if err := m.db.WithContext(ctx).Exec(
			`CREATE UNIQUE INDEX ` + idxName + ` ON llm_usage_daily
			(tenant_id, stat_date, provider, model, scenario, channel, source, error_class)`).Error; err != nil {
			return fmt.Errorf("create unique index %s: %w", idxName, err)
		}
	}
	// 报表常用过滤维度
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_daily_tenant_date ON llm_usage_daily (tenant_id, stat_date)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_daily_model ON llm_usage_daily (model)`,
	} {
		if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// 2) 明细表补列（AutoMigrate 幂等加列）
	if err := m.db.WithContext(ctx).AutoMigrate(&modelLLMRoutingLog{}, &modelPlatformLLMUsageRecord{}); err != nil {
		return fmt.Errorf("automigrate detail tables: %w", err)
	}

	// 3) 存量失败行 error_class 回填（幂等条件）
	if err := m.db.WithContext(ctx).Exec(
		`UPDATE llm_routing_logs SET error_class = 'unknown'
		 WHERE success = false AND (error_class IS NULL OR error_class = '') AND error_msg <> ''`).Error; err != nil {
		return fmt.Errorf("backfill routing_logs error_class: %w", err)
	}
	return nil
}

func (m *LLMUsageDailyMigration) Down(ctx context.Context) error {
	// Down 只删本迁移新建的形状，回填数据不还原（无源可还原）
	if err := m.db.WithContext(ctx).Exec(`DROP INDEX IF EXISTS uk_llm_usage_daily_dims`).Error; err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Exec(`DROP INDEX IF EXISTS idx_llm_usage_daily_tenant_date`).Error; err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Exec(`DROP INDEX IF EXISTS idx_llm_usage_daily_model`).Error; err != nil {
		return err
	}
	return m.db.WithContext(ctx).Migrator().DropTable("llm_usage_daily")
}

// modelLLMUsageDaily 迁移侧最小模型（与 model.LLMUsageDaily 同形状，避免循环依赖）。
type modelLLMUsageDaily struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`
	// TenantID 租户列：本仓库隔离靠 GORM callback，判据是表有无 tenant_id 列
	TenantID         uint      `gorm:"not null;default:1;index:idx_llm_usage_daily_tenant_date,priority:1"`
	StatDate         string    `gorm:"type:varchar(10);not null;index:idx_llm_usage_daily_tenant_date,priority:2"`
	Provider         string    `gorm:"type:varchar(64);not null;default:''"`
	Model            string    `gorm:"type:varchar(128);not null;default:'';index:idx_llm_usage_daily_model"`
	Scenario         string    `gorm:"type:varchar(64);not null;default:''"`
	Channel          string    `gorm:"type:varchar(30);not null;default:''"`
	Source           string    `gorm:"type:varchar(16);not null;default:''"`
	ErrorClass       string    `gorm:"type:varchar(32);not null;default:''"`
	CallCount        int64     `gorm:"not null;default:0"`
	ErrorCount       int64     `gorm:"not null;default:0"`
	CacheHitCount    int64     `gorm:"not null;default:0"`
	PromptTokens     int64     `gorm:"not null;default:0"`
	CompletionTokens int64     `gorm:"not null;default:0"`
	InputCostCents   int64     `gorm:"not null;default:0"`
	OutputCostCents  int64     `gorm:"not null;default:0"`
	TotalCostCents   int64     `gorm:"not null;default:0"`
	LatencySumMs     int64     `gorm:"not null;default:0"`
	P50LatencyMs     int64     `gorm:"not null;default:0"`
	P95LatencyMs     int64     `gorm:"not null;default:0"`
	SumSquaresLat    int64     `gorm:"not null;default:0"`
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`
}

func (modelLLMUsageDaily) TableName() string { return "llm_usage_daily" }

// 明细表迁移侧最小模型：仅需让 AutoMigrate 补 channel / error_class 两列
type modelLLMRoutingLog struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Channel    string `gorm:"type:varchar(30);not null;default:''"`
	ErrorClass string `gorm:"type:varchar(32);not null;default:''"`
}

func (modelLLMRoutingLog) TableName() string { return "llm_routing_logs" }

type modelPlatformLLMUsageRecord struct {
	ID         int64  `gorm:"primaryKey;autoIncrement"`
	Channel    string `gorm:"type:varchar(30);not null;default:''"`
	ErrorClass string `gorm:"type:varchar(32);not null;default:''"`
}

func (modelPlatformLLMUsageRecord) TableName() string { return "platform_llm_usage_records" }
