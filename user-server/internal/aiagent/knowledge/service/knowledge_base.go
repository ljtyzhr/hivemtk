// Package service 知识库子域 —— 核心 CRUD
package service

import (
	"context"
	"fmt"
	"hivemtk-user/internal/aiagent/knowledge/model"
	knowledgeRepo "hivemtk-user/internal/aiagent/knowledge/repository"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/etl"
	chunkModel "hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"os"

	"gorm.io/gorm"
)

// KnowledgeBaseService 知识库服务
type KnowledgeBaseService struct {
	db        *gorm.DB
	docs      *knowledgeRepo.KBDocumentRepository
	processor *etl.DocumentProcessor
	indexer   ragretrieval.IndexManagerInterface
	vector    *ragretrieval.Vectorizer

	// qaSplit 可选的 QA 拆分服务（上传文档可选 QA 增强；未装配=nil）。
	qaSplit *QASplitService
	// qaEnqueue 投递口替身（仅测试注入；生产走 qaSplit.EnqueueDocument）。
	qaEnqueue func(docID, kbID uint)
}

// NewKnowledgeBaseService 创建知识库服务
// 默认 EmbeddingDim 维(bge-m3,本地 TEI 真实 embedding)
func NewKnowledgeBaseService() *KnowledgeBaseService {
	return &KnowledgeBaseService{
		db:        db.GetDB(),
		docs:      knowledgeRepo.NewKBDocumentRepository(db.GetDB()),
		processor: etl.NewDocumentProcessor(nil),
		indexer:   ragretrieval.NewInMemoryIndexManager(EmbeddingDim()),
		vector:    ragretrieval.NewVectorizer(EmbeddingDim(), nil),
	}
}

// NewKnowledgeBaseServiceWithDB 创建带 DB 连接的知识库服务(用于测试)
func NewKnowledgeBaseServiceWithDB(gdb *gorm.DB) *KnowledgeBaseService {
	return &KnowledgeBaseService{
		db:        gdb,
		docs:      knowledgeRepo.NewKBDocumentRepository(gdb),
		processor: etl.NewDocumentProcessor(nil),
		indexer:   ragretrieval.NewInMemoryIndexManager(EmbeddingDim()),
		vector:    ragretrieval.NewVectorizer(EmbeddingDim(), nil),
	}
}

// ListDocuments 列出文档
func (s *KnowledgeBaseService) ListDocuments(ctx context.Context, page, pageSize int) ([]*model.KBDocument, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = DefaultPageSize
	}

	return s.docs.List(ctx, page, pageSize)
}

// GetDocument 获取文档
func (s *KnowledgeBaseService) GetDocument(ctx context.Context, id uint) (*model.KBDocument, error) {
	return s.docs.GetByID(ctx, id)
}

func (s *KnowledgeBaseService) DeleteDocument(ctx context.Context, id uint) error {
	var doc model.KBDocument
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Where("id = ?", id).First(&doc).Error; err != nil {
			return err
		}

		if err := tx.WithContext(ctx).Where("document_id = ?", id).
			Delete(&chunkModel.KnowledgeChunk{}).Error; err != nil {
			return fmt.Errorf("清理文档分段失败: %w", err)
		}
		return tx.WithContext(ctx).Where("id = ?", id).Delete(&model.KBDocument{}).Error
	})
	if err != nil {
		return err
	}
	if doc.FilePath != "" {
		_ = os.Remove(doc.FilePath)
	}
	_ = s.indexer.DropIndex(ctx, fmt.Sprintf("kb_%d", doc.ID))
	return nil
}
