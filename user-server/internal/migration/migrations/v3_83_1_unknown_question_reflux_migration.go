// Package migrations 迁移集合。
package migrations

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/migration"

	"github.com/lib/pq"
	"gorm.io/gorm"
)

// UnknownQuestionRefluxMigration v3.83.0（T1 低置信度回流）：
//  1. 新建三表：unknown_questions / unknown_question_clusters / unknown_question_review_logs
//     （模型含 TenantID 列 ⇒ 自动纳入 tenancyidx callback 隔离）；
//  2. pg_trgm 扩展 + unknown_questions.question 的 GIN trigram 索引（相似归簇用）。
//
// 幂等：AutoMigrate 建表/加列；扩展与索引用 IF NOT EXISTS。
// 语义编号：036 unknown_questions、037 unknown_question_clusters、038 unknown_question_review_logs。
type UnknownQuestionRefluxMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*UnknownQuestionRefluxMigration)(nil)

// NewUnknownQuestionRefluxMigration 构造
func NewUnknownQuestionRefluxMigration(db *gorm.DB) *UnknownQuestionRefluxMigration {
	return &UnknownQuestionRefluxMigration{db: db}
}

func (m *UnknownQuestionRefluxMigration) Version() string { return "v3.83.1" }

func (m *UnknownQuestionRefluxMigration) Name() string {
	return "unknown_questions 三表 + trgm 索引"
}

func (m *UnknownQuestionRefluxMigration) Description() string {
	return "T1 低置信度回流：unknown_questions / unknown_question_clusters / " +
		"unknown_question_review_logs 三表 + pg_trgm 扩展 + question GIN trigram 索引"
}

func (m *UnknownQuestionRefluxMigration) Up(ctx context.Context) error {
	if err := m.db.WithContext(ctx).AutoMigrate(
		&modelUnknownQuestion{}, &modelUnknownQuestionCluster{}, &modelUnknownQuestionReviewLog{},
	); err != nil {
		return fmt.Errorf("automigrate unknown_question tables: %w", err)
	}
	for _, stmt := range []string{
		`CREATE EXTENSION IF NOT EXISTS pg_trgm`,
		`CREATE INDEX IF NOT EXISTS idx_uq_question_trgm ON unknown_questions USING gin (question gin_trgm_ops)`,
	} {
		if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("exec [%s]: %w", stmt, err)
		}
	}
	return nil
}

func (m *UnknownQuestionRefluxMigration) Down(ctx context.Context) error {
	if err := m.db.WithContext(ctx).Exec(
		`DROP INDEX IF EXISTS idx_uq_question_trgm`).Error; err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Exec(
		`DROP TABLE IF EXISTS unknown_question_review_logs, unknown_question_clusters, unknown_questions CASCADE`).Error; err != nil {
		return err
	}
	// 不 drop pg_trgm 扩展：可能被其他表使用
	return nil
}

// 迁移侧最小模型（与 model 包同形状；GORM 默认蛇形列名与模型一致，
// 指纹/状态机索引由 uniqueIndex/index tag 生成）
type modelUnknownQuestion struct {
	ID             int64      `gorm:"primaryKey;autoIncrement"`
	TenantID       uint       `gorm:"not null;default:1;index"`
	QuestionHash   string     `gorm:"column:question_hash;size:64;not null;uniqueIndex:uk_uq_tenant_hash,priority:2"`
	Question       string     `gorm:"column:question;type:text;not null"`
	ClusterID      int64      `gorm:"column:cluster_id;default:0;index:idx_uq_cluster"`
	Status         string     `gorm:"column:status;size:32;not null;default:'draft';index:idx_uq_tenant_status,priority:2"`
	Source         string     `gorm:"column:source;size:32;not null"`
	SourceID       string     `gorm:"column:source_id;size:64;default:''"`
	SessionID      string     `gorm:"column:session_id;size:128;default:''"`
	CustomerID     string     `gorm:"column:customer_id;size:64;default:''"`
	AgentID        uint       `gorm:"column:agent_id;default:0;index:idx_uq_tenant_status,priority:3"`
	IntentType     string     `gorm:"column:intent_type;size:64;default:''"`
	LowConfScore   float64    `gorm:"column:low_conf_score;type:decimal(5,4);default:0"`
	Frequency      int        `gorm:"column:frequency;not null;default:1"`
	FirstSeenAt    time.Time  `gorm:"column:first_seen_at;not null"`
	LastSeenAt     time.Time  `gorm:"column:last_seen_at;not null;index:idx_uq_last_seen"`
	MergedTargetID int64      `gorm:"column:merged_target_id;default:0"`
	AdoptedFaqID   uint       `gorm:"column:adopted_faq_id;default:0"`
	ReviewedBy     string     `gorm:"column:reviewed_by;size:64;default:''"`
	ReviewedAt     *time.Time `gorm:"column:reviewed_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
}

func (modelUnknownQuestion) TableName() string { return "unknown_questions" }

type modelUnknownQuestionCluster struct {
	ID              int64          `gorm:"primaryKey;autoIncrement"`
	TenantID        uint           `gorm:"not null;default:1;index"`
	ClusterHash     string         `gorm:"column:cluster_hash;size:64;not null;uniqueIndex:uk_uqc_tenant_hash,priority:2"`
	Title           string         `gorm:"column:title;size:255;not null"`
	AgentID         uint           `gorm:"column:agent_id;default:0;index"`
	Status          string         `gorm:"column:status;size:32;not null;default:'pending_review';index:idx_uqc_status"`
	QuestionCount   int            `gorm:"column:question_count;not null;default:0"`
	TotalFrequency  int            `gorm:"column:total_frequency;not null;default:0"`
	SampleQuestions pq.StringArray `gorm:"column:sample_questions;type:text[];default:'{}'"`
	AdoptedFaqID    uint           `gorm:"column:adopted_faq_id;default:0"`
	ReviewedBy      string         `gorm:"column:reviewed_by;size:64;default:''"`
	ReviewedAt      *time.Time     `gorm:"column:reviewed_at"`
	CreatedAt       time.Time      `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time      `gorm:"column:updated_at;not null"`
}

func (modelUnknownQuestionCluster) TableName() string { return "unknown_question_clusters" }

type modelUnknownQuestionReviewLog struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	TenantID   uint      `gorm:"not null;default:1;index"`
	TargetType string    `gorm:"column:target_type;size:16;not null;index:idx_uqrl_target,priority:2"`
	TargetID   int64     `gorm:"column:target_id;not null;index:idx_uqrl_target,priority:3"`
	Action     string    `gorm:"column:action;size:32;not null"`
	FromStatus string    `gorm:"column:from_status;size:32;default:''"`
	ToStatus   string    `gorm:"column:to_status;size:32;default:''"`
	OperatorID string    `gorm:"column:operator_id;size:64;not null"`
	Detail     string    `gorm:"column:detail;type:text;default:''"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;index:idx_uqrl_target,priority:4"`
}

func (modelUnknownQuestionReviewLog) TableName() string { return "unknown_question_review_logs" }
