package ragcache

import "time"

// CacheTier 缓存命中层级
type CacheTier string

const (
	// TierExact Tier1 精确键命中（query 向量逐位相等）
	TierExact CacheTier = "exact"

	// TierSemantic Tier2 语义层命中（cosine >= 阈值）
	TierSemantic CacheTier = "semantic"

	// TierMiss 未命中（调用方回源检索+生成）
	TierMiss CacheTier = "miss"
)

// DefaultSemanticThreshold Tier2 语义相似度阈值
// RT-2 契约：0.95 起步，只调紧不放松。
const DefaultSemanticThreshold = 0.95

// Entry 缓存条目（向量以 []float32 表示，屏蔽 pgvector 序列化细节）
type Entry struct {
	ID            uint64
	KBID          string
	PromptVersion string
	QueryVector   []float32
	Answer        string
	CreatedAt     time.Time
	KBUpdatedAt   time.Time
}

// LookupRequest 缓存查询请求
type LookupRequest struct {
	KBID          string
	PromptVersion string
	QueryVector   []float32
}

// LookupResult 缓存查询结果
type LookupResult struct {
	Tier       CacheTier
	Answer     string
	Similarity float64
}
