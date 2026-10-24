package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"hivemtk-user/internal/repository"
)

// 模板/技能市场（F2 最小可用版）服务层。
//
// 职责边界：本服务是既有「内容模板 / SOP / 对接模板 / MCP / 智能路由」之上的
// **目录层**——只读浏览 + 元数据 + 分类。它不渲染任何模板内容、不改任何
// 既有实现的内容语义；要拿内容，按目录项 Source 去找那个事实源。
// 数据源是进程内注册的只读目录（repository.TemplateMarketRepository），不连 DB。

// ErrMarketEntryNotFound 目录中查无此条目（fail-closed 哨兵）。
// 上层据此映射 404 语义；不要把它包成 CodeInternal——「查无」是客户端可预期结果。
var ErrMarketEntryNotFound = errors.New("模板/技能不在市场目录中")

// MarketKind 与 MarketAvailability 常量直接复用仓储层的契约常量，
// 避免 service/repository 两份枚举漂移（单一事实源）。
const (
	MarketKindTemplate = repository.EntryKindTemplate
	MarketKindSkill    = repository.EntryKindSkill
)

// MarketItem 市场条目（API 面向的元数据契约）。
// 与 repository.TemplateMarketEntry 一一对应，字段名就是前后端契约。
type MarketItem struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Name         string   `json:"name"`
	Category     string   `json:"category"`
	Summary      string   `json:"summary"`
	Source       string   `json:"source"`
	Availability string   `json:"availability"`
	Tags         []string `json:"tags,omitempty"`
}

// MarketQuery 目录查询条件。零值 = 不过滤，返回全量。
type MarketQuery struct {
	Kind     string // 为空不筛；否则精确匹配 template|skill
	Category string // 为空不筛；否则精确匹配
	Keyword  string // 为空不筛；否则在 name/summary/tags 里做子串匹配（忽略大小写）
}

// TemplateMarketService 市场目录服务。
type TemplateMarketService struct {
	repo repository.TemplateMarketRepository
}

// NewTemplateMarketService 构造（默认进程内目录仓储）。
func NewTemplateMarketService() *TemplateMarketService {
	return &TemplateMarketService{repo: repository.NewMemoryTemplateMarketRepository()}
}

// NewTemplateMarketServiceWithRepo 带显式仓储（测试注入）。
func NewTemplateMarketServiceWithRepo(repo repository.TemplateMarketRepository) *TemplateMarketService {
	return &TemplateMarketService{repo: repo}
}

// List 目录查询。过滤顺序固定：kind → category → keyword，结果保持目录稳定序。
func (s *TemplateMarketService) List(ctx context.Context, q MarketQuery) ([]MarketItem, error) {
	entries, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]MarketItem, 0, len(entries))
	for _, e := range entries {
		if !matchMarket(e, q) {
			continue
		}
		items = append(items, toMarketItem(e))
	}
	return items, nil
}

// Categories 返回目录中出现的全部分类（去重、升序），供前端做筛选器。
func (s *TemplateMarketService) Categories(ctx context.Context) ([]string, error) {
	entries, err := s.repo.All(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		set[e.Category] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}

// Get 取单条详情。未知 ID 一律 fail-closed 返回 ErrMarketEntryNotFound，
// 不把「查无」翻译成 500——空 ID 与乱敲 ID 同样视为查无。
func (s *TemplateMarketService) Get(ctx context.Context, id string) (*MarketItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrMarketEntryNotFound
	}
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, ErrMarketEntryNotFound
	}
	item := toMarketItem(*e)
	return &item, nil
}

func toMarketItem(e repository.TemplateMarketEntry) MarketItem {
	tags := make([]string, len(e.Tags))
	copy(tags, e.Tags)
	return MarketItem{
		ID:           e.ID,
		Kind:         e.Kind,
		Name:         e.Name,
		Category:     e.Category,
		Summary:      e.Summary,
		Source:       e.Source,
		Availability: e.Availability,
		Tags:         tags,
	}
}

func matchMarket(e repository.TemplateMarketEntry, q MarketQuery) bool {
	if q.Kind != "" && e.Kind != q.Kind {
		return false
	}
	if q.Category != "" && e.Category != q.Category {
		return false
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		hay := strings.ToLower(e.Name + " " + e.Summary + " " + strings.Join(e.Tags, " "))
		if !strings.Contains(hay, strings.ToLower(kw)) {
			return false
		}
	}
	return true
}
