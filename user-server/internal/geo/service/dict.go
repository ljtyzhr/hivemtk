package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// DictService GEO 通用配置字典：DB 优先 + 缺行自动播种缺省值 + DB 故障 fail-open。
//
// cache 是包级共享而非实例字段：读侧缓存的边界是「进程」，不是「某个 service 实例」。
// 管理端（router 用 gormDB 注入的实例）与业务读路径（globalDict 单例）本就是两个实例，
// 各持一份缓存时 PUT /geo/dicts 只能失效自己那份，业务侧继续读单例里永不过期的旧值
// ⇒ 管理员改字典在进程重启前对线上行为不可见（实测：字典写入 1 条探针模板后，
// 长尾组合接口仍返回旧的 23 条）。共享一份缓存把「写一次即全进程可见」变成结构保证。
type DictService struct {
	repo repository.GeoDictRepository
}

// dictCache 全进程共享的字典读缓存，键为 dictCacheKey(category, key)，值为 string
var dictCache sync.Map

func NewDictService(repo repository.GeoDictRepository) *DictService {
	return &DictService{repo: repo}
}

func NewDictServiceDefault() *DictService {
	return NewDictService(repository.NewGeoDictRepository())
}

var (
	globalDictService *DictService
	globalDictOnce    sync.Once
)

func globalDict() *DictService {
	globalDictOnce.Do(func() {
		globalDictService = NewDictServiceDefault()
	})
	return globalDictService
}

// dictCacheKey 缓存键必须带租户：字典是租户表，同名 category/key 在不同租户
// 可以是不同的值；不带租户会让进程内缓存把 A 租户的配置发给 B 租户。
func dictCacheKey(ctx context.Context, category, key string) string {
	tenantID := tenant.DefaultTenantID
	if id, ok := tenant.FromContext(ctx); ok {
		tenantID = id
	}
	return fmt.Sprintf("%d\x00%s\x00%s", tenantID, category, key)
}

// GetString 取字典值；DB 无行则播种 fallback 行（best-effort）后返回 fallback；
// DB 故障直接返回 fallback（fail-open，保证业务不断）。
func (s *DictService) GetString(ctx context.Context, category, key, fallback string) string {
	ck := dictCacheKey(ctx, category, key)
	if v, ok := dictCache.Load(ck); ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	d, err := s.repo.GetAny(ctx, category, key)
	if err == nil && d != nil {
		if !d.Active {
			// 已停用：回缺省但不播种（否则停用会被复活）；Set 重启会清缓存
			dictCache.Store(ck, fallback)
			return fallback
		}
		dictCache.Store(ck, d.Value)
		return d.Value
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fallback
	}
	// 缺行：播种缺省值（幂等 upsert，失败不阻塞）
	_ = s.repo.Upsert(ctx, &model.GeoDict{Category: category, Key: key, Value: fallback, Active: true})
	dictCache.Store(ck, fallback)
	return fallback
}

// GetJSON 取字典 JSON 值并反序列化到 out；DB 无行/解析失败时用 fallback 反序列化。
func (s *DictService) GetJSON(ctx context.Context, category, key, fallback string, out any) error {
	raw := s.GetString(ctx, category, key, fallback)
	if err := json.Unmarshal([]byte(raw), out); err != nil {
		if raw != fallback {
			return json.Unmarshal([]byte(fallback), out)
		}
		return err
	}
	return nil
}

// Set 写入字典值并失效缓存（管理端维护入口）。失效的是包级共享缓存，
// 因此任何实例（含业务读路径的单例）的下一跳都会重新读库。
func (s *DictService) Set(ctx context.Context, category, key, value, remark string, active bool, sort int) error {
	if err := s.repo.Upsert(ctx, &model.GeoDict{
		Category: category, Key: key, Value: value,
		Remark: remark, Active: active, Sort: sort,
	}); err != nil {
		return err
	}
	dictCache.Delete(dictCacheKey(ctx, category, key))
	return nil
}

// List 按类目列出条目（管理端查看入口）
func (s *DictService) List(ctx context.Context, category string) ([]*model.GeoDict, error) {
	return s.repo.ListByCategory(ctx, category)
}

// Delete 删除条目并失效缓存
func (s *DictService) Delete(ctx context.Context, category, key string) error {
	if err := s.repo.Delete(ctx, category, key); err != nil {
		return err
	}
	dictCache.Delete(dictCacheKey(ctx, category, key))
	return nil
}

// DictValue 带调用方 ctx 读取字典（DB 优先 + 自动播种 + fail-open）。
// ctx 必须带租户：否则 on 模式读不到本租户的值（fail-open 成缺省值）、
// 缺行播种也写不进去。
func DictValue(ctx context.Context, category, key, fallback string) string {
	return globalDict().GetString(ctx, category, key, fallback)
}

// DictJSON 带调用方 ctx 读取字典 JSON 值到 out。
func DictJSON(ctx context.Context, category, key, fallback string, out any) error {
	return globalDict().GetJSON(ctx, category, key, fallback, out)
}

// DictInvalidate 失效共享缓存（测试/运维用；直改库后的补偿入口，正常写路径走 Set）。
// 不带租户：把所有租户下同 category/key 的缓存一并清掉，宁多清不漏清。
func DictInvalidate(category, key string) {
	suffix := "\x00" + category + "\x00" + key
	dictCache.Range(func(k, _ any) bool {
		if s, ok := k.(string); ok && strings.HasSuffix(s, suffix) {
			dictCache.Delete(k)
		}
		return true
	})
}
