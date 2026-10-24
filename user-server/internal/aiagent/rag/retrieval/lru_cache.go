package ragretrieval

import (
	"container/list"
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/tenant"
)

// LRUCache 线程安全的 LRU 缓存
// 用于 RAG 三级架构的 L1（高频 query 答案缓存）
type LRUCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

type lruEntry struct {
	key      string
	value    any
	expireAt time.Time
}

func NewLRUCache(capacity int, ttl time.Duration) *LRUCache {
	if capacity <= 0 {
		capacity = 100
	}
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &LRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element, capacity),
		order:    list.New(),
	}
}

func (c *LRUCache) Get(ctx context.Context, key string) (any, bool) {
	key = tenant.RedisKey(ctx, key)
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return nil, false
	}
	entry := el.Value.(*lruEntry)
	if time.Now().After(entry.expireAt) {
		c.order.Remove(el)
		delete(c.items, key)
		return nil, false
	}
	c.order.MoveToFront(el)
	return entry.value, true
}

func (c *LRUCache) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	key = tenant.RedisKey(ctx, key)
	c.mu.Lock()
	defer c.mu.Unlock()
	if ttl <= 0 {
		ttl = c.ttl
	}
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
	el := c.order.PushFront(&lruEntry{
		key:      key,
		value:    value,
		expireAt: time.Now().Add(ttl),
	})
	c.items[key] = el
	for c.order.Len() > c.capacity {
		oldest := c.order.Back()
		if oldest == nil {
			break
		}
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*lruEntry).key)
	}
}

func (c *LRUCache) Delete(ctx context.Context, key string) {
	key = tenant.RedisKey(ctx, key)
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.Remove(el)
		delete(c.items, key)
	}
}

func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element, c.capacity)
	c.order = list.New()
}

func (c *LRUCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func (c *LRUCache) Capacity() int { return c.capacity }

func (c *LRUCache) CleanupExpired() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	removed := 0
	for el := c.order.Back(); el != nil; {
		entry := el.Value.(*lruEntry)
		prev := el.Prev()
		if now.After(entry.expireAt) {
			c.order.Remove(el)
			delete(c.items, entry.key)
			removed++
		}
		el = prev
	}
	return removed
}
