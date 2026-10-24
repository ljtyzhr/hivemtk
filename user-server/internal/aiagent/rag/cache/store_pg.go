package ragcache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/pgvector"
	"hivemtk-user/internal/pkg/utils/logger"
)

// Store 语义缓存存储接口（Repository 层抽象，便于单测 mock）
type Store interface {
	// GetExact Tier1 精确键：kb_id + prompt_version + query 向量逐位相等
	GetExact(ctx context.Context, kbID, promptVersion string, vec []float32) (*Entry, error)

	// GetSemantic Tier2 语义层：cosine >= minSimilarity 的最近邻一条
	GetSemantic(ctx context.Context, kbID, promptVersion string, vec []float32, minSimilarity float64) (*Entry, error)

	// Put 写入缓存条目
	Put(ctx context.Context, e *Entry) error

	// Delete 按 id 删除（kb 更新失效时调用）
	Delete(ctx context.Context, id uint64) error
}

// KBMetaReader 知识库元信息读取接口（命中前校验 kb 更新时间）
type KBMetaReader interface {
	// GetKBUpdatedAt 读取 knowledge_bases.updated_at
	GetKBUpdatedAt(ctx context.Context, kbID string) (time.Time, error)
}

type pgKBMetaReader struct {
	db *gorm.DB
}

func (r *pgKBMetaReader) GetKBUpdatedAt(ctx context.Context, kbID string) (time.Time, error) {
	if r == nil || r.db == nil {
		return time.Time{}, errors.New("kb meta reader 未初始化")
	}
	var updated time.Time
	err := r.db.WithContext(ctx).
		Table("knowledge_bases").
		Select("updated_at").
		Where("id = ?", kbID).
		Scan(&updated).Error
	if err != nil {
		return time.Time{}, err
	}
	if updated.IsZero() {
		return time.Time{}, fmt.Errorf("知识库不存在或 updated_at 为空: kb_id=%s", kbID)
	}
	return updated, nil
}

// PGAnswerCacheStore 基于 GORM + pgvector 的存储实现（表 rag_answer_cache，
// 建表走 internal/pkg/db.allModels()）
type PGAnswerCacheStore struct {
	db *gorm.DB
}

// NewPGAnswerCacheStore 创建 pgvector 存储实现
func NewPGAnswerCacheStore(db *gorm.DB) *PGAnswerCacheStore {
	return &PGAnswerCacheStore{db: db}
}

// NewPGKBMetaReader 创建基于 GORM 的 KB 元信息读取器
func NewPGKBMetaReader(db *gorm.DB) KBMetaReader {
	return &pgKBMetaReader{db: db}
}

// GetExact 实现 Store.Tier1：pgvector 相等运算符做逐位精确匹配。
// 同一归一化 query 文本经确定性 embedding 得到相同向量 → 命中同一行。
func (s *PGAnswerCacheStore) GetExact(ctx context.Context, kbID, promptVersion string, vec []float32) (*Entry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("answer cache store 未初始化")
	}
	var row model.RAGAnswerCache
	err := s.db.WithContext(ctx).Table("rag_answer_cache").
		Select("id, kb_id, prompt_version, query_vector::text AS query_vector, answer, created_at, kb_updated_at").
		Where("kb_id = ? AND prompt_version = ? AND query_vector = ?::vector", kbID, promptVersion, vecToLiteral(vec)).
		Order("created_at DESC").Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	return rowToEntry(&row)
}

// GetSemantic 实现 Store.Tier2：<=> 余弦距离取阈值内最近邻
func (s *PGAnswerCacheStore) GetSemantic(ctx context.Context, kbID, promptVersion string, vec []float32, minSimilarity float64) (*Entry, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("answer cache store 未初始化")
	}
	literal := vecToLiteral(vec)
	var row struct {
		model.RAGAnswerCache
		Similarity float64 `gorm:"column:similarity"`
	}
	err := s.db.WithContext(ctx).Table("rag_answer_cache").
		Select("id, kb_id, prompt_version, query_vector::text AS query_vector, answer, created_at, kb_updated_at, "+
			"1 - (query_vector <=> ?::vector) AS similarity", literal).
		Where("kb_id = ? AND prompt_version = ?", kbID, promptVersion).
		Where("1 - (query_vector <=> ?::vector) >= ?", literal, minSimilarity).
		Order(pgvector.OrderByQueryVectorDistance(literal)).
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil
	}
	e, err := rowToEntry(&row.RAGAnswerCache)
	if err != nil {
		return nil, err
	}
	logger.Debugf("[ragcache] semantic hit similarity=%.4f threshold=%.2f", row.Similarity, minSimilarity)
	return e, nil
}

// Put 实现 Store 写入
func (s *PGAnswerCacheStore) Put(ctx context.Context, e *Entry) error {
	if s == nil || s.db == nil {
		return errors.New("answer cache store 未初始化")
	}
	if e.KBID == "" || e.PromptVersion == "" || len(e.QueryVector) == 0 || e.Answer == "" {
		return errors.New("ragcache put: 缺少必填字段")
	}
	return s.db.WithContext(ctx).Table("rag_answer_cache").
		Create(map[string]any{
			"kb_id":          e.KBID,
			"prompt_version": e.PromptVersion,
			"query_vector":   gorm.Expr("?::vector", vecToLiteral(e.QueryVector)),
			"answer":         e.Answer,
			"created_at":     time.Now(),
			"kb_updated_at":  e.KBUpdatedAt,
		}).Error
}

// Delete 实现按 id 删除
func (s *PGAnswerCacheStore) Delete(ctx context.Context, id uint64) error {
	if s == nil || s.db == nil {
		return errors.New("answer cache store 未初始化")
	}
	return s.db.WithContext(ctx).Table("rag_answer_cache").
		Where("id = ?", id).
		Delete(nil).Error
}

func rowToEntry(row *model.RAGAnswerCache) (*Entry, error) {
	vec, err := parseVectorLiteral(row.QueryVector)
	if err != nil {
		return nil, fmt.Errorf("解析 query_vector 失败: %w", err)
	}
	return &Entry{
		ID:            row.ID,
		KBID:          row.KBID,
		PromptVersion: row.PromptVersion,
		QueryVector:   vec,
		Answer:        row.Answer,
		CreatedAt:     row.CreatedAt,
		KBUpdatedAt:   row.KBUpdatedAt,
	}, nil
}
