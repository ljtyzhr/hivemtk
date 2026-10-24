package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// KnowledgeWeightMigration 知识库权重迁移
type KnowledgeWeightMigration struct {
	db *gorm.DB
}

func NewKnowledgeWeightMigration(db *gorm.DB) *KnowledgeWeightMigration {
	return &KnowledgeWeightMigration{db: db}
}

func (m *KnowledgeWeightMigration) Version() string { return "v3.13.0" }

func (m *KnowledgeWeightMigration) Name() string {
	return "知识库自学习 - knowledge_chunks.weight 列"
}

func (m *KnowledgeWeightMigration) Description() string {
	return "v3.13.0：为 knowledge_chunks 新增 weight 双精度列（默认 1.0），作为检索排名第二依据，支持自学习权重调整"
}

func (m *KnowledgeWeightMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS weight double precision NOT NULL DEFAULT 1`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("knowledge_weight 执行失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

func (m *KnowledgeWeightMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	declineColumnDrop(m.Version(), "knowledge_chunks.weight")
	return nil
}

var _ migration.Migration = (*KnowledgeWeightMigration)(nil)
