package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

type RagHealthRepository interface {
	CountKnowledgeChunks(ctx context.Context) (int64, error)
}

type ragHealthRepo struct {
	db *gorm.DB
}

// NewRagHealthRepository db 为 nil 时返回 nil 接口，service 以 repo == nil 守卫。
func NewRagHealthRepository(db *gorm.DB) RagHealthRepository {
	if db == nil {
		return nil
	}
	return &ragHealthRepo{db: db}
}

func (r *ragHealthRepo) CountKnowledgeChunks(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).
		Model(&model.KnowledgeChunk{}).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

var _ RagHealthRepository = (*ragHealthRepo)(nil)
