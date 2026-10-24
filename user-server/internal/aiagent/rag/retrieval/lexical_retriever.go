package ragretrieval

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"hivemtk-user/internal/pkg/kbrelease"
)

type LexicalRetriever struct {
	db *gorm.DB
}

// NewLexicalRetriever 创建词法召回器
func NewLexicalRetriever(db *gorm.DB) *LexicalRetriever {
	return &LexicalRetriever{db: db}
}

func (r *LexicalRetriever) Retrieve(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("bm25 retriever 未初始化")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 50
	}

	// 文本搜索配置按「库里真实存在的」依次试（见 tsconfig_probe.go）：
	// 装了 zhparser 的库走 zh_rag（中文分词更准），没装的库直接走 simple。
	cfgs := availableTSConfigs(ctx, r.db)
	for _, cfg := range cfgs {
		for _, col := range []string{"contextual_tsv", "content_tsv"} {
			if rows, err := r.tryTSQuery(ctx, productID, query, topK, col, cfg); err == nil && len(rows) > 0 {
				return rows, nil
			}
		}
	}

	return r.ilikeFallback(ctx, productID, query, topK)
}

func (r *LexicalRetriever) SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error) {
	return r.Retrieve(ctx, kbID, query, topK)
}

var allowedTSCols = map[string]string{
	"contextual_tsv": "contextual_tsv",
	"content_tsv":    "content_tsv",
}

var allowedTSConfigs = map[string]string{
	"zh_rag":  "zh_rag",
	"simple":  "simple",
	"english": "english",
}

func (r *LexicalRetriever) tryTSQuery(ctx context.Context, productID string, query string, topK int, tsvCol, tsConfig string) ([]Chunk, error) {
	safeCol, ok := allowedTSCols[tsvCol]
	if !ok {
		return nil, fmt.Errorf("invalid tsvector column: %s", tsvCol)
	}
	safeConfig, ok := allowedTSConfigs[tsConfig]
	if !ok {
		return nil, fmt.Errorf("invalid text search config: %s", tsConfig)
	}
	q := r.db.WithContext(ctx).Table("knowledge_chunks").
		Select(fmt.Sprintf("id, document_id, content, ts_rank(%s, plainto_tsquery('%s', ?))::float8 AS score", safeCol, safeConfig), query).
		Where(fmt.Sprintf("%s @@ plainto_tsquery('%s', ?)", safeCol, safeConfig), query)
	if productID != "" {
		q = q.Where("product_id = ?"+kbrelease.AndVisible(), productID)
	} else if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order("score DESC").Limit(topK)
	var rows []chunkScanRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}

func (r *LexicalRetriever) ilikeFallback(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	pattern := "%" + escaped + "%"
	q := r.db.WithContext(ctx).Table("knowledge_chunks").
		Select("id, document_id, content, 1.0::float8 AS score").
		Where("content ILIKE ?", pattern)
	if productID != "" {
		q = q.Where("product_id = ?"+kbrelease.AndVisible(), productID)
	} else if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order("id DESC").Limit(topK)
	var rows []chunkScanRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}
