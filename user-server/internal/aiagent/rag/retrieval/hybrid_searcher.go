package ragretrieval

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	"hivemtk-user/internal/pkg/kbrelease"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func makeRRFKey(docID, chunkID string) string {
	h := sha256.Sum256([]byte(docID + "\x00" + chunkID))
	return "rrf:" + base64.RawURLEncoding.EncodeToString(h[:8])
}

type tsvectorConfig struct {
	tsConfig string
	tsvCol   string
}

type HybridSearcher struct {
	db              *gorm.DB
	embeddingClient llm.EmbeddingServiceInterface
	vectorSearcher  VectorSearcher
	keywordSearcher KeywordSearcher
	reranker        RerankerInterface
	redisClient     RedisClient
	llmChatClient   LLMChatClient
	vectorWeight    float64
	keywordWeight   float64
	config          *HybridSearcherConfig
	tsvectorCfg     *tsvectorConfig
	tsvectorOnce    sync.Once
	tsvectorMu      sync.RWMutex
}

// VectorSearcher 向量检索接口
type VectorSearcher interface {
	SearchVector(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error)
}

type KeywordSearcher interface {
	SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error)
}

// HybridSearcherConfig 混合检索配置
type HybridSearcherConfig struct {
	EnableRerank     bool
	EnableHyDE       bool
	EnableMultiQuery bool
	VectorTopK       int
	KeywordTopK      int
	FinalTopK        int
	DefaultTopK      int
	CandidatePool    int
	FusedTopN        int
	RRFK             int
	EfSearch         int
	VectorWeight     float64
	KeywordWeight    float64
}

// DefaultHybridSearcherConfig 默认配置
func DefaultHybridSearcherConfig() *HybridSearcherConfig {
	return &HybridSearcherConfig{
		EnableRerank:     true,
		EnableHyDE:       false,
		EnableMultiQuery: false,
		VectorTopK:       50,
		KeywordTopK:      30,
		FinalTopK:        5,
		DefaultTopK:      5,
		CandidatePool:    100,
		FusedTopN:        20,
		RRFK:             60,
		EfSearch:         128,
		VectorWeight:     0.7,
		KeywordWeight:    0.3,
	}
}

// NewHybridSearcher 创建混合检索器
func NewHybridSearcher(db *gorm.DB, embeddingClient llm.EmbeddingServiceInterface, reranker RerankerInterface, llmChatClient LLMChatClient, redisClient RedisClient, cfg *HybridSearcherConfig) *HybridSearcher {
	if cfg == nil {
		cfg = DefaultHybridSearcherConfig()
	}
	h := &HybridSearcher{
		db:              db,
		embeddingClient: embeddingClient,
		reranker:        reranker,
		llmChatClient:   llmChatClient,
		redisClient:     redisClient,
		vectorWeight:    cfg.VectorWeight,
		keywordWeight:   cfg.KeywordWeight,
		config:          cfg,
	}
	if db != nil {
		h.vectorSearcher = NewVectorRetriever(db, embeddingClient, cfg.EfSearch)
		h.keywordSearcher = NewLexicalRetriever(db)
	}
	return h
}

// NewHybridSearcherWithInterfaces 基于接口创建（兼容旧接口）
func NewHybridSearcherWithInterfaces(vs VectorSearcher, ks KeywordSearcher, r RerankerInterface, vectorW, keywordW float64) *HybridSearcher {
	if vectorW == 0 {
		vectorW = 0.7
	}
	if keywordW == 0 {
		keywordW = 0.3
	}
	return &HybridSearcher{
		vectorSearcher:  vs,
		keywordSearcher: ks,
		reranker:        r,
		vectorWeight:    vectorW,
		keywordWeight:   keywordW,
		config:          DefaultHybridSearcherConfig(),
	}
}

// Search 全产品检索
func (h *HybridSearcher) Search(ctx context.Context, query string, topK int) ([]Chunk, error) {
	return h.SearchIndex(ctx, "", query, topK)
}

// SearchIndex 带 productID 过滤的检索
func (h *HybridSearcher) SearchIndex(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	if topK <= 0 {
		topK = h.config.DefaultTopK
	}
	if topK <= 0 {
		topK = 5
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	vectorStart := time.Now()
	queryVec, err := h.embedQuery(ctx, query)
	if err != nil {

		logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] embedding query failed, degrade to keyword-only search")
		queryVec = nil
	}
	vectorMs := time.Since(vectorStart).Milliseconds()

	vecResults, kwResults := make([]Chunk, 0), make([]Chunk, 0)

	vecTopK := h.config.VectorTopK
	if vecTopK <= 0 {
		vecTopK = topK * 10
		if vecTopK < 50 {
			vecTopK = 50
		}
	}
	if queryVec != nil {
		if h.vectorSearcher != nil {
			if res, err := h.vectorSearcher.SearchVector(ctx, productID, queryVec, vecTopK); err == nil {
				vecResults = res
			} else {
				logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] vector search failed, continuing with empty results")
			}
		} else if h.db != nil {
			if res, err := h.vectorSearchPG(ctx, productID, queryVec, vecTopK); err == nil {
				vecResults = res
			} else {
				logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] vectorSearchPG failed, continuing with empty results")
			}
		}
	}

	keywordStart := time.Now()
	kwTopK := h.config.KeywordTopK
	if kwTopK <= 0 {
		kwTopK = topK * 6
		if kwTopK < 30 {
			kwTopK = 30
		}
	}
	if h.keywordSearcher != nil {
		if res, err := h.keywordSearcher.SearchKeyword(ctx, productID, query, kwTopK); err == nil {
			kwResults = res
		} else {
			logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] keyword search failed, continuing with empty results")
		}
	} else if h.db != nil {
		if res, err := h.keywordSearchPG(ctx, productID, query, kwTopK); err == nil {
			kwResults = res
		} else {
			logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] keywordSearchPG failed, continuing with empty results")
		}
	}
	bm25Ms := time.Since(keywordStart).Milliseconds()

	profile := ResolveQueryWeightProfile(query, h.vectorWeight, h.keywordWeight)
	fused := h.reciprocalRankFusion(vecResults, kwResults, profile.VectorWeight, profile.KeywordWeight)

	rerankCount := 0
	if h.config.EnableRerank && h.reranker != nil && len(fused) > topK {
		rerankTopN := h.config.FusedTopN
		if rerankTopN <= 0 {
			rerankTopN = topK * 4
		}
		if rerankTopN > len(fused) {
			rerankTopN = len(fused)
		}
		rerankPool := fused[:rerankTopN]
		reranked, rerr := h.reranker.Rerank(ctx, query, toRerankDocs(rerankPool))

		if rerr == nil {
			if len(reranked) == 0 {
				logger.Infof("[Hybrid] rerank 全部文档低于阈值(floor=%.1f)，视为知识不足，降级使用原始融合结果", rerankScoreFloor)
			} else {
				fused = applyRerank(fused, reranked)
				rerankCount = len(reranked)
			}
		} else {
			logger.Warnf("[Hybrid] rerank 失败，降级使用原始融合结果: %v", rerr)
		}
	}

	if rerankCount == 0 && len(fused) > 0 {
		normalizeRRFScores(fused)
	}

	if h.config.CandidatePool > 0 && len(fused) > h.config.CandidatePool {
		fused = fused[:h.config.CandidatePool]
	}

	// FinalTopK 是**上限**，不是对调用方 topK 的覆盖。
	// 写成覆盖时，默认配置（FinalTopK=5）会让 topK=1..4 的请求全部拿到 5 条：
	// 实测 RagSearcher.Search(query, 3) 在默认配置下回了 4 条（种子只有 4 条）。
	// 调用方用 topK 控制进 prompt 的分片数与计费（如 knowledge_merchant_playground.go
	// 直接透传 req.TopK、tooluse/knowledge_tools.go 透传模型给的 topK），
	// 多回来的每一条都是静默的预算超支。
	finalK := topK
	if h.config.FinalTopK > 0 && h.config.FinalTopK < finalK {
		finalK = h.config.FinalTopK
	}
	if len(fused) > finalK {
		fused = fused[:finalK]
	}

	h.logSearch(ctx, query, productID, finalK, len(vecResults), len(kwResults), len(fused), rerankCount, vectorMs, bm25Ms)

	return fused, nil
}

func (h *HybridSearcher) logSearch(ctx context.Context, query, productID string, topK, vectorCount, bm25Count, fusedCount, rerankCount int, vectorMs, bm25Ms int64) {
	if h.db == nil {
		return
	}
	logCtx := tenant.BackgroundWithTenant(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Warnf("[Hybrid] logSearch panic recovered: %v\n%s", r, debug.Stack())
			}
		}()
		if err := h.db.WithContext(logCtx).Table("knowledge_search_logs").
			Create(map[string]any{
				"query":             query,
				"product_id":        gorm.Expr("NULLIF(?::text, '')::bigint", productID),
				"top_k":             topK,
				"vector_count":      vectorCount,
				"bm25_count":        bm25Count,
				"fused_count":       fusedCount,
				"rerank_count":      rerankCount,
				"vector_latency_ms": vectorMs,
				"bm25_latency_ms":   bm25Ms,
			}).Error; err != nil {
			logger.Warnf("[Hybrid] logSearch 写入 knowledge_search_logs 失败: %v", err)
		}
	}()
}

func (h *HybridSearcher) embedQuery(ctx context.Context, query string) ([]float32, error) {
	if h.embeddingClient == nil {
		return nil, fmt.Errorf("embedding client not configured")
	}
	cfg := h.embeddingClient.DefaultConfig()
	vec, err := h.embeddingClient.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, err
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("empty embedding returned")
	}
	return vec, nil
}

func (h *HybridSearcher) vectorSearchPG(ctx context.Context, productID string, queryVec []float32, topK int) ([]Chunk, error) {
	if h.db == nil {
		return nil, fmt.Errorf("db not configured")
	}
	vecStr := vecToPGString(queryVec)
	q := h.db.WithContext(ctx).Table("knowledge_chunks").
		Select("id, document_id, content, embedding <=> ?::vector AS score", vecStr).
		Where("embedding IS NOT NULL AND embedding_source = 'tei'")
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	// 闸门片段不带绑定参数（AndVisible 不追加占位符），链式追加位置安全
	if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order(clause.Expr{SQL: "embedding <=> ?::vector", Vars: []any{vecStr}}).Limit(topK)

	var rows []chunkScanRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}

func (h *HybridSearcher) detectTSVectorConfig() {
	for _, tsConfig := range []string{"zh_rag", "simple"} {
		for _, tsvCol := range []string{"contextual_tsv", "content_tsv"} {
			var ok int
			if err := h.db.Table("knowledge_chunks").
				Select("1").
				Where(fmt.Sprintf("%s @@ plainto_tsquery('test')", tsvCol)).
				Limit(1).
				Scan(&ok).Error; err == nil {
				h.tsvectorMu.Lock()
				h.tsvectorCfg = &tsvectorConfig{tsConfig: tsConfig, tsvCol: tsvCol}
				h.tsvectorMu.Unlock()
				return
			}
		}
	}
}

func (h *HybridSearcher) tsvectorSearch(ctx context.Context, tsConfig, tsvCol, productID, query string, topK int) ([]Chunk, error) {
	q := h.db.WithContext(ctx).Table("knowledge_chunks").
		Select(fmt.Sprintf("id, document_id, content, ts_rank(%s, plainto_tsquery(?, ?))::float8 AS score", tsvCol),
			tsConfig, query).
		Where(fmt.Sprintf("%s @@ plainto_tsquery(?, ?)", tsvCol), tsConfig, query)
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order("score DESC").Limit(topK)

	var rows []chunkScanRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}

func (h *HybridSearcher) keywordSearchPG(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	if h.db == nil {
		return nil, fmt.Errorf("db not configured")
	}

	h.tsvectorOnce.Do(func() {
		h.detectTSVectorConfig()
	})

	h.tsvectorMu.RLock()
	cfg := h.tsvectorCfg
	h.tsvectorMu.RUnlock()
	if cfg != nil {
		rows, err := h.tsvectorSearch(ctx, cfg.tsConfig, cfg.tsvCol, productID, query, topK)
		if err == nil {
			return rows, nil
		}

		h.tsvectorMu.Lock()
		h.tsvectorCfg = nil
		h.tsvectorMu.Unlock()
	}

	for _, tsConfig := range []string{"zh_rag", "simple"} {
		for _, tsvCol := range []string{"contextual_tsv", "content_tsv"} {
			if rows, err := h.tsvectorSearch(ctx, tsConfig, tsvCol, productID, query, topK); err == nil && len(rows) > 0 {
				return rows, nil
			}
		}
	}
	return h.keywordSearchPGFallback(ctx, productID, query, topK)
}

func (h *HybridSearcher) keywordSearchPGFallback(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	pattern := "%" + escaped + "%"
	q := h.db.WithContext(ctx).Table("knowledge_chunks").
		Select("id, document_id, content, 1.0::float8 AS score").
		Where("content ILIKE ?", pattern)
	if productID != "" {
		q = q.Where("product_id = ?", productID)
	}
	if vis := kbrelease.AndVisible(); vis != "" {
		q = q.Where(strings.TrimPrefix(vis, " AND "))
	}
	q = q.Order("id DESC").Limit(topK)

	var rows []chunkScanRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}

func (h *HybridSearcher) reciprocalRankFusion(vecResults, kwResults []Chunk, vecW, kwW float64) []Chunk {
	k := h.config.RRFK
	if k <= 0 {
		k = 60
	}
	scores := make(map[string]float64)
	chunkMap := make(map[string]Chunk)

	for rank, c := range vecResults {
		key := makeRRFKey(c.DocumentID, c.ID)
		scores[key] += vecW / float64(k+rank+1)
		chunkMap[key] = c
	}

	for rank, c := range kwResults {
		key := makeRRFKey(c.DocumentID, c.ID)
		scores[key] += kwW / float64(k+rank+1)
		if _, exists := chunkMap[key]; !exists {
			chunkMap[key] = c
		}
	}

	type kv struct {
		key   string
		score float64
	}
	pairs := make([]kv, 0, len(scores))
	for k, v := range scores {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		// pairs 由 scores(map) 摊出，只比 score 时同分块次的先后取自 map 迭代序（每轮随机）；
		// 下游按顺序截 Top-K 当 LLM 上下文 ⇒ 同一个查询两次能取到不同的召回片段。
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].key < pairs[j].key
	})

	result := make([]Chunk, 0, len(pairs))
	for _, p := range pairs {
		if c, ok := chunkMap[p.key]; ok {
			c.Score = p.score
			result = append(result, c)
		}
	}
	return result
}

func normalizeRRFScores(chunks []Chunk) {
	if len(chunks) == 0 {
		return
	}
	max, min := chunks[0].Score, chunks[0].Score
	for _, c := range chunks[1:] {
		if c.Score > max {
			max = c.Score
		}
		if c.Score < min {
			min = c.Score
		}
	}
	span := max - min
	for i := range chunks {
		if span <= 0 {
			chunks[i].Score = 1.0
			continue
		}
		chunks[i].Score = (chunks[i].Score - min) / span
	}
}
