package translation

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

// 本文件守住 Glossary 实体本轮修掉的两条多租户缺陷：
//
//  1. 缓存键不带租户 ⇒ 租户 A 的术语视图被喂给租户 B 的 LLM 输出后置校准。
//  2. 术语 CRUD 之后调 cache.Clear 失效缓存，而 Redis 后端的 Clear 是
//     FLUSHDB（internal/cache/redis.go:201）⇒ 一个租户管理员保存一条术语，
//     把整台实例所有租户、所有模块的缓存一起掀了。
//
// 两条都不是"以后再说"的事：第 1 条是客户直接看到别家品牌名，
// 第 2 条是跨租户跨模块的服务降级。

// fakeCache 记录 Delete/Clear 调用，并持有一份可读写的键值对。
//
// 只实现本用例用到的方法；其余方法靠嵌入的 nil 接口（调用即 panic），
// 免得用例"恰好没碰到"某个方法就以为它是对的。
type fakeCache struct {
	cache.Cache
	mu      sync.Mutex
	store   map[string]string
	deleted []string
	cleared int
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: map[string]string{}}
}

func (f *fakeCache) Delete(_ context.Context, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, key)
	delete(f.store, key)
	return nil
}

func (f *fakeCache) Clear(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleared++
	f.store = map[string]string{}
	return nil
}

func (f *fakeCache) GetJSON(_ context.Context, key string, dest any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, ok := f.store[key]
	if !ok {
		return context.Canceled // 任何非 nil error 都算"未命中"
	}
	return json.Unmarshal([]byte(raw), dest)
}

func (f *fakeCache) SetJSON(_ context.Context, key string, value any, _ time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f.store[key] = string(raw)
	return nil
}

func (f *fakeCache) keys() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.store))
	for k, v := range f.store {
		out[k] = v
	}
	return out
}

// fakeGlossaryRepo 返回固定的一份术语表。
type fakeGlossaryRepo struct {
	terms []*model.Glossary
}

func (r *fakeGlossaryRepo) ListActive(_ context.Context) ([]*model.Glossary, error) {
	return r.terms, nil
}
func (r *fakeGlossaryRepo) GetByTermID(_ context.Context, _ string) (*model.Glossary, error) {
	return nil, nil
}
func (r *fakeGlossaryRepo) Create(_ context.Context, _ *model.Glossary) error { return nil }
func (r *fakeGlossaryRepo) Update(_ context.Context, _ *model.Glossary) error { return nil }
func (r *fakeGlossaryRepo) Delete(_ context.Context, _ string) error          { return nil }
func (r *fakeGlossaryRepo) ListByCategory(_ context.Context, _ string) ([]*model.Glossary, error) {
	return r.terms, nil
}
func (r *fakeGlossaryRepo) ListAll(_ context.Context, _, _ string, _, _ int) ([]*model.Glossary, int64, error) {
	return r.terms, int64(len(r.terms)), nil
}

func ctxWithTenant(id uint) context.Context {
	return tenant.WithTenantID(context.Background(), id)
}

// TestGlossaryCacheKey_IsolatesTenants 键必须带租户维度。
func TestGlossaryCacheKey_IsolatesTenants(t *testing.T) {
	k1 := glossaryCacheKey(ctxWithTenant(1), "en")
	k2 := glossaryCacheKey(ctxWithTenant(2), "en")
	if k1 == k2 {
		t.Fatalf("两个租户拿到同一个缓存键 %q：术语视图会跨租户复用", k1)
	}
	if !strings.Contains(k1, ":1:") || !strings.Contains(k2, ":2:") {
		t.Fatalf("缓存键里看不到租户号：k1=%q k2=%q", k1, k2)
	}
	if !strings.HasPrefix(k1, glossaryCacheKeyPrefix) {
		t.Fatalf("缓存键前缀不对：%q", k1)
	}
}

// TestGlossaryInvalidateAll_DoesNotFlushWholeCache 失效缓存不得调 cache.Clear。
//
// 这条是本轮修掉的最高危项：Redis 后端 Clear = FLUSHDB。
func TestGlossaryInvalidateAll_DoesNotFlushWholeCache(t *testing.T) {
	fc := newFakeCache()
	svc := NewGlossaryService(&fakeGlossaryRepo{}, fc)

	svc.InvalidateAll(ctxWithTenant(1))

	if fc.cleared != 0 {
		t.Fatalf("术语缓存失效调了 cache.Clear %d 次：Redis 后端那是 FLUSHDB，会掀掉整台实例所有租户所有模块的缓存", fc.cleared)
	}
	if len(fc.deleted) == 0 {
		t.Fatal("术语缓存失效一个键都没删：改完术语后旧视图会一直命中")
	}
	for _, k := range fc.deleted {
		if !strings.HasPrefix(k, glossaryCacheKeyPrefix+"1:") {
			t.Errorf("删了别的租户的缓存键 %q", k)
		}
	}
}

// TestGlossaryInvalidateAll_OnlyTouchesOwnTenant 失效只作用于本租户。
func TestGlossaryInvalidateAll_OnlyTouchesOwnTenant(t *testing.T) {
	fc := newFakeCache()
	other := glossaryCacheKey(ctxWithTenant(2), "en")
	fc.SetJSON(context.Background(), other, GlossaryView{Lang: "en"}, time.Hour)

	svc := NewGlossaryService(&fakeGlossaryRepo{}, fc)
	svc.InvalidateAll(ctxWithTenant(1))

	if _, ok := fc.keys()[other]; !ok {
		t.Fatalf("租户 2 的术语缓存被租户 1 的术语变更清掉了（键 %q）", other)
	}
}

// TestGlossaryLoadByLang_DoesNotReuseOtherTenantCache 租户 B 不得命中租户 A 的缓存。
func TestGlossaryLoadByLang_DoesNotReuseOtherTenantCache(t *testing.T) {
	fc := newFakeCache()
	repo := &fakeGlossaryRepo{terms: []*model.Glossary{
		{TermID: "brand-a", Category: "brand", Status: "active",
			Translations: model.JSONMap{"zh": "品牌甲", "en": "Brand A"}},
	}}
	svc := NewGlossaryService(repo, fc)

	viewA, err := svc.LoadByLang(ctxWithTenant(1), "en")
	if err != nil {
		t.Fatalf("租户 1 加载术语失败: %v", err)
	}
	if viewA.Mappings["品牌甲"] != "Brand A" {
		t.Fatalf("租户 1 术语视图不对: %+v", viewA.Mappings)
	}

	// 租户 2 的 ctx：必须走库（拿到自己的术语表），不能复用租户 1 的缓存。
	repo2 := &fakeGlossaryRepo{terms: []*model.Glossary{
		{TermID: "brand-b", Category: "brand", Status: "active",
			Translations: model.JSONMap{"zh": "品牌乙", "en": "Brand B"}},
	}}
	svc2 := NewGlossaryService(repo2, fc)
	viewB, err := svc2.LoadByLang(ctxWithTenant(2), "en")
	if err != nil {
		t.Fatalf("租户 2 加载术语失败: %v", err)
	}
	if _, leaked := viewB.Mappings["品牌甲"]; leaked {
		t.Fatalf("租户 2 的术语视图里出现了租户 1 的术语 品牌甲：%+v", viewB.Mappings)
	}
	if viewB.Mappings["品牌乙"] != "Brand B" {
		t.Fatalf("租户 2 术语视图不对: %+v", viewB.Mappings)
	}
}
