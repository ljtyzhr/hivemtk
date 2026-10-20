package ragretrieval

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/tenant"
)

// 检索缓存是跨请求复用的派生数据：同键在不同租户下必须互不可见，
// 且 on 模式下不允许写入无租户归属的裸键。
func TestRetrievalCaches_TenantIsolation(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	ctxA := tenant.WithTenantID(context.Background(), 11)
	ctxB := tenant.WithTenantID(context.Background(), 22)

	shared := cache.NewMemoryCache()
	defer shared.Close()

	backends := map[string]CacheInterface{
		"in_memory":    NewInMemoryCache(),
		"redis_backed": NewRedisBackedCache(shared),
	}
	for name, c := range backends {
		c.Set(ctxA, "same-key", "A", time.Minute)
		c.Set(ctxB, "same-key", "B", time.Minute)

		if v, ok := c.Get(ctxA, "same-key"); !ok || v != "A" {
			t.Fatalf("[%s] 租户 A 读到 %v,%v，期望 A", name, v, ok)
		}
		if v, ok := c.Get(ctxB, "same-key"); !ok || v != "B" {
			t.Fatalf("[%s] 租户 B 读到 %v,%v，期望 B（疑似串租户）", name, v, ok)
		}
		if _, ok := c.Get(context.Background(), "same-key"); ok {
			t.Fatalf("[%s] 无租户 ctx 读到了租户私有键", name)
		}
	}

	// LRU 是三级检索的 L1 热缓存，热路径同样必须按租户分域。
	lru := NewLRUCache(8, time.Minute)
	lru.Set(ctxA, "same-key", "A", time.Minute)
	lru.Set(ctxB, "same-key", "B", time.Minute)
	if v, ok := lru.Get(ctxA, "same-key"); !ok || v != "A" {
		t.Fatalf("LRU 租户 A 读到 %v,%v，期望 A", v, ok)
	}
	if v, ok := lru.Get(ctxB, "same-key"); !ok || v != "B" {
		t.Fatalf("LRU 租户 B 读到 %v,%v，期望 B（疑似串租户）", v, ok)
	}
}

// Search 必须在读缓存之前先校验知识库归属：命中缓存也不得绕过 storage
// 的租户作用域，否则猜中他人 kbID 即可回放他人缓存结果。
func TestRagSearch_ValidatesKBBeforeCacheHit(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	storage := NewMockStorage()
	memCache := NewInMemoryCache()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, memCache, &RetrievalConfig{
		MaxQueryLength: 1000, DefaultTopK: 5, DefaultSimilarityThreshold: 0.3, MinSimilarityThreshold: 0.1,
		MaxChunkSize: 1000, DefaultChunkOverlap: 100, MaxDocLength: 10000, CacheTTL: time.Minute,
	})
	ctx := context.Background()

	if err := storage.SaveKnowledgeBase(ctx, KnowledgeBaseInfo{ID: "kb_guard", OwnerID: "u1"}); err != nil {
		t.Fatalf("SaveKnowledgeBase: %v", err)
	}
	if err := service.IndexDocuments(ctx, "kb_guard", []Document{{ID: "d1", Content: "归属校验文档"}}); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	params := SearchParams{TopK: 5, SimilarityThreshold: 0.3}
	results, err := service.Search(ctx, "kb_guard", "归属", params)
	if err != nil {
		t.Fatalf("首次 Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("首次 Search 无结果，缓存路径未被覆盖")
	}

	cacheKey := fmt.Sprintf("search:%s:%s:%v", "kb_guard", "归属", params)
	if _, ok := memCache.Get(ctx, cacheKey); !ok {
		t.Fatal("首次 Search 未写入缓存")
	}

	// 模拟知识库被删除（或不属于当前租户）：即使缓存里有结果也必须报错。
	delete(storage.knowledgeBases, "kb_guard")
	if _, err := service.Search(ctx, "kb_guard", "归属", params); err == nil {
		t.Fatal("知识库不存在时仍从缓存返回结果，绕过了归属校验")
	}
}

// on 模式下检索缓存键必须带租户前缀，不得落裸键。
func TestRagSearch_CacheKeysScopedPerTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	storage := NewMockStorage()
	memCache := NewInMemoryCache()
	service := NewRagRetrievalService(&constantVectorizer{dimension: 128}, NewMockIndexManager(), storage, memCache, &RetrievalConfig{
		MaxQueryLength: 1000, DefaultTopK: 5, DefaultSimilarityThreshold: 0.3, MinSimilarityThreshold: 0.1,
		MaxChunkSize: 1000, DefaultChunkOverlap: 100, MaxDocLength: 10000, CacheTTL: time.Minute,
	})

	ctxA := tenant.WithTenantID(context.Background(), 11)
	if err := storage.SaveKnowledgeBase(ctxA, KnowledgeBaseInfo{ID: "kb_scope", OwnerID: "u1"}); err != nil {
		t.Fatalf("SaveKnowledgeBase: %v", err)
	}
	if err := service.IndexDocuments(ctxA, "kb_scope", []Document{{ID: "d1", Content: "作用域文档"}}); err != nil {
		t.Fatalf("IndexDocuments: %v", err)
	}

	params := SearchParams{TopK: 5, SimilarityThreshold: 0.3}
	if _, err := service.Search(ctxA, "kb_scope", "作用域", params); err != nil {
		t.Fatalf("Search: %v", err)
	}

	cacheKey := fmt.Sprintf("search:%s:%s:%v", "kb_scope", "作用域", params)
	if _, ok := memCache.Get(ctxA, cacheKey); !ok {
		t.Fatal("未按租户前缀写入缓存键")
	}
	if _, ok := memCache.Get(context.Background(), cacheKey); ok {
		t.Fatal("on 模式下写入了无租户归属的缓存键")
	}
}
