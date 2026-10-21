// dict_tenant_cache_test.go 字典缓存的租户隔离契约。
//
// 缓存是进程级共享的，键必须带 tenant_id：否则 A 租户先读、B 租户后读同名
// (category, key) 时会直接命中 A 的值，把别家配置发给 B。
package service

import (
	"context"
	"sync"
	"testing"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

type tenantDictRepo struct {
	mu   sync.Mutex
	rows map[uint]map[string]*model.GeoDict
}

func newTenantDictRepo() *tenantDictRepo {
	return &tenantDictRepo{rows: map[uint]map[string]*model.GeoDict{}}
}

func (r *tenantDictRepo) ck(category, key string) string { return category + "\x00" + key }

func (r *tenantDictRepo) tenantOf(ctx context.Context) uint {
	if id, ok := tenant.FromContext(ctx); ok {
		return id
	}
	return tenant.DefaultTenantID
}

func (r *tenantDictRepo) GetAny(ctx context.Context, category, key string) (*model.GeoDict, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.rows[r.tenantOf(ctx)][r.ck(category, key)]; ok {
		return d, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *tenantDictRepo) ListByCategory(ctx context.Context, category string) ([]*model.GeoDict, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*model.GeoDict
	for _, d := range r.rows[r.tenantOf(ctx)] {
		if d.Category == category {
			list = append(list, d)
		}
	}
	return list, nil
}

func (r *tenantDictRepo) Upsert(ctx context.Context, d *model.GeoDict) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tid := r.tenantOf(ctx)
	d.TenantID = tid
	if r.rows[tid] == nil {
		r.rows[tid] = map[string]*model.GeoDict{}
	}
	cp := *d
	r.rows[tid][r.ck(d.Category, d.Key)] = &cp
	return nil
}

func (r *tenantDictRepo) Delete(ctx context.Context, category, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	tid := r.tenantOf(ctx)
	if _, ok := r.rows[tid][r.ck(category, key)]; !ok {
		return gorm.ErrRecordNotFound
	}
	delete(r.rows[tid], r.ck(category, key))
	return nil
}

const dictTenantTestCategory = "dict_tenant_cache_test"

func TestDictService_CacheIsTenantScoped(t *testing.T) {
	const key = "k"
	repo := newTenantDictRepo()
	svc := NewDictService(repo)
	ctxA := tenant.WithTenantID(context.Background(), 271)
	ctxB := tenant.WithTenantID(context.Background(), 272)
	t.Cleanup(func() { DictInvalidate(dictTenantTestCategory, key) })

	if err := svc.Set(ctxA, dictTenantTestCategory, key, "A", "", true, 0); err != nil {
		t.Fatalf("租户 A 写入失败: %v", err)
	}
	if err := svc.Set(ctxB, dictTenantTestCategory, key, "B", "", true, 0); err != nil {
		t.Fatalf("租户 B 写入失败: %v", err)
	}

	// 先读 A 把缓存灌热，再读 B：缓存键不带租户时 B 会命中 A 的值。
	if got := svc.GetString(ctxA, dictTenantTestCategory, key, "fallbackA"); got != "A" {
		t.Fatalf("租户 A 读到 %q", got)
	}
	if got := svc.GetString(ctxB, dictTenantTestCategory, key, "fallbackB"); got != "B" {
		t.Fatalf("租户 B 读到 %q（缓存跨租户串值）", got)
	}

	if err := svc.Set(ctxA, dictTenantTestCategory, key, "A2", "", true, 0); err != nil {
		t.Fatalf("租户 A 二次写入失败: %v", err)
	}
	if got := svc.GetString(ctxA, dictTenantTestCategory, key, "fallbackA"); got != "A2" {
		t.Fatalf("租户 A 写入后仍读到 %q", got)
	}
	if got := svc.GetString(ctxB, dictTenantTestCategory, key, "fallbackB"); got != "B" {
		t.Fatalf("租户 A 写入污染了租户 B 的值: %q", got)
	}
}
