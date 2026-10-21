package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoKnowledgeDocumentRepository GEO 知识库文档仓储接口
type GeoKnowledgeDocumentRepository interface {
	Create(ctx context.Context, doc *model.GeoKnowledgeDocument) error
	Update(ctx context.Context, doc *model.GeoKnowledgeDocument) error
	GetByID(ctx context.Context, id string) (*model.GeoKnowledgeDocument, error)
	GetList(ctx context.Context) ([]*model.GeoKnowledgeDocument, error)
	Delete(ctx context.Context, id string) error
}

type geoKnowledgeDocRepo struct {
	db *gorm.DB
}

func NewGeoKnowledgeDocumentRepository() GeoKnowledgeDocumentRepository {
	return &geoKnowledgeDocRepo{db: _db.GetDB()}
}

// NewGeoKnowledgeDocumentRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewGeoKnowledgeDocumentRepositoryWithDB(db *gorm.DB) GeoKnowledgeDocumentRepository {
	return &geoKnowledgeDocRepo{db: db}
}

func (r *geoKnowledgeDocRepo) Create(ctx context.Context, doc *model.GeoKnowledgeDocument) error {
	return r.db.WithContext(ctx).Create(doc).Error
}

func (r *geoKnowledgeDocRepo) Update(ctx context.Context, doc *model.GeoKnowledgeDocument) error {
	res := r.db.WithContext(ctx).Model(&model.GeoKnowledgeDocument{}).
		Where("id = ?", doc.ID).
		Updates(map[string]any{
			"title":      doc.Title,
			"content":    doc.Content,
			"doc_type":   doc.DocType,
			"metadata":   doc.Metadata,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return res.Error
	}
	// 0 行说明文档已删除或不属于本租户：不能当作保存成功。
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *geoKnowledgeDocRepo) GetByID(ctx context.Context, id string) (*model.GeoKnowledgeDocument, error) {
	var doc model.GeoKnowledgeDocument
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&doc).Error
	return &doc, err
}

// GetList 返回最近的文档（上限 1000，避免全表读进内存；检索/列表都走这里）。
func (r *geoKnowledgeDocRepo) GetList(ctx context.Context) ([]*model.GeoKnowledgeDocument, error) {
	var docs []*model.GeoKnowledgeDocument
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(1000).Find(&docs).Error
	return docs, err
}

func (r *geoKnowledgeDocRepo) Delete(ctx context.Context, id string) error {
	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.GeoKnowledgeDocument{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
