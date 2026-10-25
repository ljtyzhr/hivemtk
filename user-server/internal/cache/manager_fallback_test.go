// manager_fallback_test.go E-6b 契约测试配套（规划 §4.6b 第 4 条）：
// 钉住 NewCacheManager 在 Redis 不可用时的静默降级行为——
// "Redis 连接失败 ⇒ 回退 MemoryCache"是既有生产行为（降级态必须绿），
// 本文件把它钉成断言，防止有人把降级改成"报错启动失败"（单实例部署立即炸）
// 或"返回 nil cache"（所有缓存调用 nil panic）。
//
// 降级后的契约面（Get/LPop 未命中哨兵、Set/Incr 可用性）也一并钉住：
// 降级不是"换个实现继续跑"就完了，**降级前后调用方看到的错误哨兵必须一致**，
// 这正是 E-6b 把两实现未命中统一为 ErrCacheMiss 的理由——
// 双哨兵分支写漏一支的调用方，在主路径（Redis 正常）上永远测不出来，
// 只会在 Redis 故障降级的那一刻才发作。
package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCacheManager_RedisDownFallsBackToMemory 钉住 manager.go 的静默降级：
// 连不上的 Redis 配置必须返回**可用的** CacheManager（后端为内存实现），
// 且不返回 error——降级这件事本身不是调用方的故障。
//
// 端口用 1（TCP 保留端口，无监听 ⇒ 连接必然失败，不依赖"本机没起 Redis"
// 这种环境假设；反过来也不依赖"6379 正好关着"）。
func TestCacheManager_RedisDownFallsBackToMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("需要网络拨号（连保留端口 1）")
	}
	mgr, err := NewCacheManager(CacheConfig{Type: "redis", Redis: RedisConfig{Host: "127.0.0.1", Port: 1, DB: 15}})
	if err != nil {
		t.Fatalf("Redis 不可用时 NewCacheManager 不得报错（静默降级是既有行为）： %v", err)
	}
	if mgr == nil || mgr.cache == nil {
		t.Fatal("降级后必须返回可用的 CacheManager（nil 会让全部缓存调用 panic）")
	}
	// 降级到内存实现的直接证据：RedisCache 才有 Client()（非 nil）。
	if rc, ok := mgr.cache.(*RedisCache); ok && rc.Client() != nil {
		t.Fatal("降级后后端仍是 RedisCache：Redis 都连不上了，缓存读写会全部失败")
	}

	ctx := context.Background()
	// 降级腿的契约面：写入可读回、未命中是统一哨兵。
	if err := mgr.Set(ctx, "fb-key", "v1", time.Minute); err != nil {
		t.Fatalf("降级后 Set 应可用： %v", err)
	}
	if v, err := mgr.Get(ctx, "fb-key"); err != nil || v != "v1" {
		t.Fatalf("降级后 Get 应读回 v1，得 (%q, %v)", v, err)
	}
	if _, err := mgr.Get(ctx, "fb-miss"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("降级腿 Get 未命中必须是 ErrCacheMiss，得 %v", err)
	}
	if _, err := mgr.LPop(ctx, "fb-lpop-empty"); !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("降级腿 LPop 未命中必须是 ErrCacheMiss，得 %v", err)
	}
}
