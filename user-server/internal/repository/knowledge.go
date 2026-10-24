package repository

import (
	"context"
	"strings"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type KnowledgeRepository struct {
	db *gorm.DB
}

func NewKnowledgeRepository() *KnowledgeRepository {
	return &KnowledgeRepository{db: _db.GetDB()}
}

func NewKnowledgeRepositoryWithDB(db *gorm.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

// Search 基于 LIKE 的全文搜索（生产环境应替换为向量检索）
func (r *KnowledgeRepository) Search(ctx context.Context, query string, topK int) ([]*model.KnowledgeHit, error) {
	var hits []*model.KnowledgeHit

	err := r.db.WithContext(ctx).Table("knowledge_articles").
		Select("id, title, content, 1.0 as score, 'article' as source, category_id").
		Where("title LIKE ? OR content LIKE ?", "%"+query+"%", "%"+query+"%").
		Order("score DESC").
		Limit(topK).
		Scan(&hits).Error

	if err != nil {
		if strings.Contains(err.Error(), "doesn't exist") {
			return []*model.KnowledgeHit{}, nil
		}
		return nil, err
	}

	return hits, nil
}
