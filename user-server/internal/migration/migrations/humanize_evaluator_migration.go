package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// HumanizeEvaluatorMigration 拟人度评估器迁移 v2.9.0
type HumanizeEvaluatorMigration struct {
	db *gorm.DB
}

func NewHumanizeEvaluatorMigration(db *gorm.DB) *HumanizeEvaluatorMigration {
	return &HumanizeEvaluatorMigration{db: db}
}

func (m *HumanizeEvaluatorMigration) Version() string { return "v2.9.1" }

func (m *HumanizeEvaluatorMigration) Name() string { return "拟人度评估器（4 张表）" }

func (m *HumanizeEvaluatorMigration) Description() string {
	return "创建 humanize_scores / humanize_dimensions / champion_baselines / champion_phrases 4 张表"
}

func (m *HumanizeEvaluatorMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.createHumanizeScores(ctx); err != nil {
		return fmt.Errorf("create humanize_scores 失败: %w", err)
	}

	if err := m.createHumanizeDimensions(ctx); err != nil {
		return fmt.Errorf("create humanize_dimensions 失败: %w", err)
	}

	if err := m.createChampionBaselines(ctx); err != nil {
		return fmt.Errorf("create champion_baselines 失败: %w", err)
	}

	if err := m.createChampionPhrases(ctx); err != nil {
		return fmt.Errorf("create champion_phrases 失败: %w", err)
	}

	return nil
}

func (m *HumanizeEvaluatorMigration) createHumanizeScores(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS humanize_scores (
			id BIGSERIAL PRIMARY KEY,
			score_id VARCHAR(64) NOT NULL UNIQUE,
			session_id VARCHAR(128) NOT NULL,
			customer_id VARCHAR(128) NOT NULL,
			message_id VARCHAR(128) DEFAULT '',
			persona VARCHAR(128) DEFAULT '',
			industry VARCHAR(64) DEFAULT '',
			platform VARCHAR(32) DEFAULT '',
			intent VARCHAR(32) DEFAULT '',
			customer_message TEXT DEFAULT '',
			ai_reply TEXT NOT NULL,
			final_reply TEXT DEFAULT '',
			evaluator_type VARCHAR(16) NOT NULL DEFAULT 'rule',
			sample_strategy VARCHAR(24) NOT NULL DEFAULT 'full',
			naturalness DECIMAL(4,3) NOT NULL,
			conciseness DECIMAL(4,3) NOT NULL,
			empathy DECIMAL(4,3) NOT NULL,
			professionalism DECIMAL(4,3) NOT NULL,
			persuasiveness DECIMAL(4,3) NOT NULL,
			total_score DECIMAL(4,3) NOT NULL,
			threshold DECIMAL(4,3) NOT NULL DEFAULT 0.850,
			distance_to_champion DECIMAL(5,4) DEFAULT 0,
			passed BOOLEAN NOT NULL DEFAULT FALSE,
			attempt_count INT NOT NULL DEFAULT 1,
			llm_model VARCHAR(64) DEFAULT '',
			llm_latency_ms INT DEFAULT 0,
			reason_json JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_humanize_session ON humanize_scores(session_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_humanize_persona ON humanize_scores(persona, industry, intent)`,
		`CREATE INDEX IF NOT EXISTS idx_humanize_score ON humanize_scores(total_score)`,
	}
	return execAllHumanize(ctx, m.db, stmts)
}

func (m *HumanizeEvaluatorMigration) createHumanizeDimensions(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS humanize_dimensions (
			id BIGSERIAL PRIMARY KEY,
			score_id VARCHAR(64) NOT NULL,
			dimension VARCHAR(32) NOT NULL,
			score DECIMAL(4,3) NOT NULL,
			weight DECIMAL(4,3) NOT NULL,
			reason TEXT DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hd_score ON humanize_dimensions(score_id, dimension)`,
	}
	return execAllHumanize(ctx, m.db, stmts)
}

func (m *HumanizeEvaluatorMigration) createChampionBaselines(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS champion_baselines (
			id BIGSERIAL PRIMARY KEY,
			persona VARCHAR(128) NOT NULL,
			industry VARCHAR(64) NOT NULL,
			intent VARCHAR(32) NOT NULL,
			naturalness DECIMAL(4,3) NOT NULL,
			conciseness DECIMAL(4,3) NOT NULL,
			empathy DECIMAL(4,3) NOT NULL,
			professionalism DECIMAL(4,3) NOT NULL,
			persuasiveness DECIMAL(4,3) NOT NULL,
			sample_count INT NOT NULL,
			sample_stddev DECIMAL(4,3) DEFAULT 0,
			period_start TIMESTAMPTZ,
			period_end TIMESTAMPTZ,
			version INT NOT NULL DEFAULT 1,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_champion_pii ON champion_baselines(persona, industry, intent, version DESC, enabled)`,
	}
	return execAllHumanize(ctx, m.db, stmts)
}

func (m *HumanizeEvaluatorMigration) createChampionPhrases(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS champion_phrases (
			id BIGSERIAL PRIMARY KEY,
			baseline_id BIGINT NOT NULL,
			phrase VARCHAR(64) NOT NULL,
			tfidf_score DECIMAL(8,5) NOT NULL,
			tf INT NOT NULL,
			df INT NOT NULL,
			phrase_type VARCHAR(16) NOT NULL DEFAULT 'general',
			rank INT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_cp_baseline ON champion_phrases(baseline_id, rank)`,
	}
	return execAllHumanize(ctx, m.db, stmts)
}

// Down 回滚（删除新表）
//
// 注意：
// 不删除 low_quality_samples（与 共享）
//   - 4 张新表可安全删除
func (m *HumanizeEvaluatorMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	declineTableDrop(m.Version(), "champion_phrases", "champion_baselines",
		"humanize_dimensions", "humanize_scores")
	return nil
}

func execAllHumanize(ctx context.Context, db *gorm.DB, stmts []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

var _ migration.Migration = (*HumanizeEvaluatorMigration)(nil)
