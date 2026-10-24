package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// LLMUsageRecordsMigration 创建 llm_usage_records 表
type LLMUsageRecordsMigration struct {
	db *gorm.DB
}

func NewLLMUsageRecordsMigration(db *gorm.DB) *LLMUsageRecordsMigration {
	return &LLMUsageRecordsMigration{db: db}
}

func (m *LLMUsageRecordsMigration) Version() string { return "v3.4.0" }

func (m *LLMUsageRecordsMigration) Name() string { return "创建 llm_usage_records 表" }

func (m *LLMUsageRecordsMigration) Description() string {
	return "创建 llm_usage_records 表（id, total_tokens, cost, created_at），支撑 AI 生产力 LLM 成本统计"
}

func (m *LLMUsageRecordsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS llm_usage_records (
			id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			total_tokens bigint      NOT NULL DEFAULT 0,
			cost         numeric(14,4) NOT NULL DEFAULT 0,
			created_at   timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_records_created_at ON llm_usage_records (created_at)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("创建 llm_usage_records 失败: %w", err)
		}
	}
	return nil
}

func (m *LLMUsageRecordsMigration) Down(ctx context.Context) error {
	// 该表同时被 ops/ai_productivity 的聚合查询与后续模块化改造使用，
	// 共享测试库/存量库场景下它可能先于本迁移存在——Down 只撤销自己新建的对象，
	// 其余按 declineTableDrop 口径放弃（降级不销毁在用对象）。
	declineTableDrop(m.Version(), "llm_usage_records")
	return nil
}

var _ migration.Migration = (*LLMUsageRecordsMigration)(nil)
