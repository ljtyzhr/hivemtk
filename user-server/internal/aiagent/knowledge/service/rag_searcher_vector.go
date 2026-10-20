package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"hivemtk-user/internal/pkg/kbrelease"
	"hivemtk-user/internal/pkg/pgvector"
)

func (s *RagSearcher) vectorSearch(ctx context.Context, productID string, query string, topK int) ([]scored, error) {
	if s.embeddingService == nil {
		return nil, fmt.Errorf("embedding service 未初始化")
	}
	cfg := s.embeddingService.DefaultConfig()

	queryVec, err := s.embeddingService.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, fmt.Errorf("TEI 编码失败: %w", err)
	}

	vecLiteral := vecToPGString(queryVec)

	var rows []chunkRow
	q := s.db.WithContext(ctx).Table("knowledge_chunks").
		Select("id, document_id, content, (1 - (embedding <=> ?::vector))::float8 AS score", vecLiteral).
		Where("embedding IS NOT NULL AND embedding_source = 'tei'")
	if productID != "" {
		q = q.Where("product_id = ?"+kbrelease.AndVisible(), productID)
	} else if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order(pgvector.OrderByEmbeddingDistance(vecLiteral)).Limit(topK)
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	pairs := make([]scored, 0, len(rows))
	for _, r := range rows {
		pairs = append(pairs, scored{row: r, score: r.Score})
	}
	return pairs, nil
}

func vecToPGString(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
