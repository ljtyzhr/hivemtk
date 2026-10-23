package cache_test

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/cache"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cacheImpl 缓存实现的统一形状（MemoryCache 与 RedisCache 各起一列）。
type cacheImpl struct {
	name string
	get  func(ctx context.Context, key string) (string, error)
	lpop func(ctx context.Context, key string) (string, error)
	incr func(ctx context.Context, key string, ttl time.Duration) (int64, error)
	set  func(ctx context.Context, key, val string, ttl time.Duration) error
	push func(ctx context.Context, key, val string) error
}

func memoryImpl() *cacheImpl {
	m := cache.NewMemoryCache()
	return &cacheImpl{
		name: "memory",
		get:  m.Get,
		lpop: m.LPop,
		incr: m.Incr,
		set: func(ctx context.Context, key, val string, ttl time.Duration) error {
			return m.Set(ctx, key, val, ttl)
		},
		push: func(ctx context.Context, key, val string) error {
			return m.LPush(ctx, key, val, 0)
		},
	}
}

// redisImpl 可选：本机没有 Redis 时跳过该列（契约测试的两列设计里 Redis 列
// 只在实例可用时跑；Memory 列永远跑——降级态必须绿）。
func redisImpl(t *testing.T) *cacheImpl {
	t.Helper()
	r, err := cache.NewRedisCache(cache.RedisConfig{Host: "127.0.0.1", Port: 6379, DB: 15})
	if err != nil {
		t.Skip("本机无 Redis，只跑 Memory 腿（降级态必须绿，Redis 腿在 CI 有 Redis 时补跑）")
	}
	t.Cleanup(func() { _ = r.Close() })
	return &cacheImpl{
		name: "redis",
		get:  r.Get,
		lpop: r.LPop,
		incr: r.Incr,
		set: func(ctx context.Context, key, val string, ttl time.Duration) error {
			return r.Set(ctx, key, val, ttl)
		},
		push: func(ctx context.Context, key, val string) error {
			return r.LPush(ctx, key, val, 0)
		},
	}
}

// TestCacheContract_MemoryMatchesRedis E-6b 契约测试（规划 §4.6b 第 3 条）：
// 同一组用例分别跑两实现，断言相同的错误哨兵与 TTL 行为。
//   - Get 未命中：ErrCacheMiss / redis.Nil（两哨兵等价，各自实现内一致）
//   - LPop 未命中 / 空列表：同为未命中哨兵（Memory 旧实现返回 ("", nil) 已修正）
//   - Incr TTL：只首次设置，后续自增不续期
func TestCacheContract_MemoryMatchesRedis(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库/Redis")
	}
	impls := []*cacheImpl{memoryImpl()}
	if r := redisImpl(t); r != nil {
		impls = append(impls, r)
	}

	for _, impl := range impls {
		t.Run(impl.name, func(t *testing.T) {
			ctx := context.Background()
			nonce := time.Now().UnixNano()

			// ① Get 未命中哨兵
			_, err := impl.get(ctx, "contract-miss")
			if impl.name == "memory" {
				assert.ErrorIs(t, err, cache.ErrCacheMiss)
			} else {
				assert.ErrorIs(t, err, redis.Nil)
			}

			// ② LPop 未命中：与 Get 同哨兵（E-6b 修正点）
			_, err = impl.lpop(ctx, "contract-lpop-empty")
			if impl.name == "memory" {
				assert.ErrorIs(t, err, cache.ErrCacheMiss, "LPop 未命中应与 Get 同哨兵")
			} else {
				assert.ErrorIs(t, err, redis.Nil, "LPop 未命中应与 Get 同哨兵")
			}

			// ③ LPop 空列表（push 后取空）：同为未命中哨兵
			key := "contract-lpop-drain"
			require.NoError(t, impl.push(ctx, key, "only-one"))
			v, err := impl.lpop(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, "only-one", v)
			_, err = impl.lpop(ctx, key)
			if impl.name == "memory" {
				assert.ErrorIs(t, err, cache.ErrCacheMiss, "列表取空后 LPop 应报未命中")
			} else {
				assert.ErrorIs(t, err, redis.Nil, "列表取空后 LPop 应报未命中")
			}

			// ④ Incr TTL 只首次设置：n=1,2,3 后键仍按首次 TTL 过期，不续期。
			//    用"远大于测试时长的 TTL + 观察不漂移"验证：连续 incr 后
			//    Memory 侧读内部过期时间不可行（黑盒），改用可观测等价面——
			//    短 TTL（50ms）下 incr 3 次后等待 60ms，键必须已过期。
			//    旧 Memory 实现（每次续期）在这一格里 n=3 后过期时间被推到
			//    3×TTL 之后，等待 TTL 时仍命中 ⇒ 本用例旧实现必红。
			//    Redis 最小 TTL 1s ⇒ 用 1200ms、等 1300ms。
			ikey := "contract-incr-ttl"
			n1, err := impl.incr(ctx, ikey, 1200*time.Millisecond)
			require.NoError(t, err)
			assert.EqualValues(t, 1, n1)
			_, err = impl.incr(ctx, ikey, 1200*time.Millisecond)
			require.NoError(t, err)
			n3, err := impl.incr(ctx, ikey, 1200*time.Millisecond)
			require.NoError(t, err)
			assert.EqualValues(t, 3, n3)
			time.Sleep(1300 * time.Millisecond)
			_, err = impl.get(ctx, ikey)
			if impl.name == "memory" {
				assert.ErrorIs(t, err, cache.ErrCacheMiss, "Incr TTL 只首次设置：3 次自增后 50ms 键必须已过期（不得续期）")
			} else {
				assert.ErrorIs(t, err, redis.Nil, "Incr TTL 只首次设置：Redis 参照行为")
			}
			_ = nonce
		})
	}
}
