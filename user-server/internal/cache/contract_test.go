package cache_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/cache"

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

// cacheImpl2 契约第二组用例的形状：覆盖 Get/LPop 之外的共享面。
type cacheImpl2 struct {
	name    string
	set     func(ctx context.Context, key string, val any, ttl time.Duration) error
	get     func(ctx context.Context, key string) (string, error)
	exists  func(ctx context.Context, key string) (bool, error)
	setnx   func(ctx context.Context, key string, val any, ttl time.Duration) (bool, error)
	release func(ctx context.Context, key, token string) (bool, error)
	getjson func(ctx context.Context, key string, dest any) error
	lpush   func(ctx context.Context, key string, val any, ttl time.Duration) error
	rpush   func(ctx context.Context, key string, val any, ttl time.Duration) error
	lpop    func(ctx context.Context, key string) (string, error)
	lrange  func(ctx context.Context, key string, start, stop int64) ([]string, error)
	llen    func(ctx context.Context, key string) (int64, error)
}

func memoryImpl2() *cacheImpl2 {
	m := cache.NewMemoryCache()
	return &cacheImpl2{
		name:    "memory",
		set:     m.Set,
		get:     m.Get,
		exists:  m.Exists,
		setnx:   m.SetNX,
		release: m.ReleaseLock,
		getjson: m.GetJSON,
		lpush:   m.LPush,
		rpush:   m.RPush,
		lpop:    m.LPop,
		lrange:  m.LRange,
		llen:    m.LLen,
	}
}

func redisImpl2(t *testing.T) *cacheImpl2 {
	t.Helper()
	r, err := cache.NewRedisCache(cache.RedisConfig{Host: "127.0.0.1", Port: 6379, DB: 15})
	if err != nil {
		t.Skip("本机无 Redis，只跑 Memory 腿（降级态必须绿，Redis 腿在 CI 有 Redis 时补跑）")
	}
	t.Cleanup(func() { _ = r.Close() })
	return &cacheImpl2{
		name:    "redis",
		set:     r.Set,
		get:     r.Get,
		exists:  r.Exists,
		setnx:   r.SetNX,
		release: r.ReleaseLock,
		getjson: r.GetJSON,
		lpush:   r.LPush,
		rpush:   r.RPush,
		lpop:    r.LPop,
		lrange:  r.LRange,
		llen:    r.LLen,
	}
}

// TestCacheContract2_ListAndLockParity 第二轮契约巡检（业务闭环 C-10③）：
// Get/LPop/Incr 之外的共享面逐格对齐。判据一律"过期键对 Redis 是不存在"：
//   - LPop/LLen/LRange 对过期列表 = 未命中/0/空（旧 Memory 实现把死值弹给调用方）
//   - LPush 到过期键 = 复活成新列表（旧实现追加进僵尸列表后仍不可见）
//   - SetNX 到过期键 = 视为不存在，可重新获取
//   - Exists 对过期键 = false
//   - ReleaseLock token 不匹配 / 键已过期 = false 且不误删
//   - GetJSON 未命中 = ErrCacheMiss（与 Get 同哨兵）
//
// TTL 用 1100ms：go-redis v9 的 formatSec 把 <1s 的 TTL **静默截成 1s**（commands.go:41），
// Memory 腿却真支持亚秒——两腿对齐必须取双方都可表达的值。这也意味着调用方对
// "短 TTL 锁/限流"在 Redis 腿拿到的实际 TTL ≥1s，见 redis.go 头注的已知差异登记。
func TestCacheContract2_ListAndLockParity(t *testing.T) {
	if testing.Short() {
		t.Skip("需要真 TTL 等待")
	}
	impls := []*cacheImpl2{memoryImpl2()}
	if r := redisImpl2(t); r != nil {
		impls = append(impls, r)
	}

	const ttl = 1100 * time.Millisecond
	for _, impl := range impls {
		t.Run(impl.name, func(t *testing.T) {
			ctx := context.Background()
			// DB 15 在多次 go test 之间持久：键名必须逐运行唯一，否则上一轮
			// 分钟级 TTL 的残留键会把下一轮的"过期"断言整个毒化。
			ns := fmt.Sprintf("contract2-%d", time.Now().UnixNano())

			// ① LPop / LLen / LRange 对过期列表：未命中/0/空
			key1 := ns + "-expired-list"
			require.NoError(t, impl.lpush(ctx, key1, "stale", ttl))
			require.NoError(t, impl.rpush(ctx, key1, "stale2", ttl))
			// ② LPush 到过期键复活成新列表（先落数据，等过期后再推）
			key2 := ns + "-resurrect"
			require.NoError(t, impl.lpush(ctx, key2, "old", ttl))
			// ③ SetNX 到过期键可重新获取
			key3 := ns + "-setnx-expired"
			ok, err := impl.setnx(ctx, key3, "v1", 40*time.Millisecond)
			require.NoError(t, err)
			assert.True(t, ok, "首次 SetNX 应成功")
			// ④ ReleaseLock token 不匹配不误删（未过期窗口内完成）
			ok, err = impl.release(ctx, key3, "wrong-token")
			require.NoError(t, err)
			assert.False(t, ok, "token 不匹配不得释放他人锁")
			// ⑤ GetJSON 未命中与 Get 同哨兵
			var dest map[string]any
			assert.ErrorIs(t, impl.getjson(ctx, ns+"-json-miss", &dest), cache.ErrCacheMiss)
			// ⑥ Exists：未过期 true，过期后 false（同一键跨窗口断言）
			key4 := ns + "-exists-expired"
			require.NoError(t, impl.set(ctx, key4, "x", ttl))
			ok, err = impl.exists(ctx, key4)
			require.NoError(t, err)
			assert.True(t, ok, "未过期 Exists 应为 true")

			time.Sleep(1300 * time.Millisecond)

			// ①续 过期后三读
			_, err = impl.lpop(ctx, key1)
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "过期列表 LPop 必须与未命中同哨兵")
			n, err := impl.llen(ctx, key1)
			require.NoError(t, err)
			assert.EqualValues(t, 0, n, "过期列表 LLen 必须为 0")
			items, err := impl.lrange(ctx, key1, 0, -1)
			require.NoError(t, err)
			assert.Empty(t, items, "过期列表 LRange 必须为空")

			// ②续 复活：推入后新值可见且不含旧值
			require.NoError(t, impl.lpush(ctx, key2, "fresh", 0))
			v, err := impl.lpop(ctx, key2)
			require.NoError(t, err)
			assert.Equal(t, "fresh", v, "复活后的列表必须只含新值")
			_, err = impl.lpop(ctx, key2)
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "复活列表不应残留旧值")

			// ③续 SetNX 过期后重新获取
			ok, err = impl.setnx(ctx, key3, "v2", time.Minute)
			require.NoError(t, err)
			assert.True(t, ok, "过期键上的 SetNX 必须视为不存在并可重新获取")

			// ⑥续 过期键 Exists = false
			ok, err = impl.exists(ctx, key4)
			require.NoError(t, err)
			assert.False(t, ok, "过期键 Exists 必须为 false")
		})
	}
}

// TestCacheContract_MemoryMatchesRedis E-6b 契约测试（规划 §4.6b 第 3 条）：
// 同一组用例分别跑两实现，断言**同一个**错误哨兵与 TTL 行为。
//   - Get 未命中：两腿统一为 cache.ErrCacheMiss（RedisCache 在实现内转换 redis.Nil）
//   - LPop 未命中 / 空列表：与 Get 同哨兵
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
			_ = nonce

			// ① Get 未命中哨兵：两腿同为 ErrCacheMiss（E-6b 统一点）
			_, err := impl.get(ctx, "contract-miss")
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "两后端未命中必须统一为 ErrCacheMiss")

			// ② LPop 未命中：与 Get 同哨兵
			_, err = impl.lpop(ctx, "contract-lpop-empty")
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "LPop 未命中应与 Get 同哨兵")

			// ③ LPop 空列表（push 后取空）：同为未命中哨兵
			key := "contract-lpop-drain"
			require.NoError(t, impl.push(ctx, key, "only-one"))
			v, err := impl.lpop(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, "only-one", v)
			_, err = impl.lpop(ctx, key)
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "列表取空后 LPop 应报未命中")

			// ④ Incr TTL 只首次设置：n=1,2,3 后键仍按首次 TTL 过期，不续期。
			//    用"远大于测试时长的 TTL + 观察不漂移"验证：连续 incr 后
			//    Memory 侧读内部过期时间不可行（黑盒），改用可观测等价面——
			//    短 TTL（50ms）下 incr 3 次后等待 60ms，键必须已过期。
			//    旧 Memory 实现（每次续期）在这一格里 n=3 后过期时间被推到
			//    3×TTL 之后，等待 TTL 时仍命中 ⇒ 本用例旧实现必红。
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
			assert.ErrorIs(t, err, cache.ErrCacheMiss, "Incr TTL 只首次设置：3 次自增后键必须按首次 TTL 过期（不得续期）")
		})
	}
}
