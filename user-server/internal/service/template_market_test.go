package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/repository"
)

// stubMarketRepo 测试用仓储：可注入目录与错误。
type stubMarketRepo struct {
	entries []repository.TemplateMarketEntry
	err     error
}

func (s *stubMarketRepo) All(ctx context.Context) ([]repository.TemplateMarketEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

func (s *stubMarketRepo) Get(ctx context.Context, id string) (*repository.TemplateMarketEntry, error) {
	if s.err != nil {
		return nil, s.err
	}
	for i := range s.entries {
		if s.entries[i].ID == id {
			return &s.entries[i], nil
		}
	}
	return nil, nil
}

// realMarketService 用真实进程内目录构造的服务（目录种子即演示数据源）。
func realMarketService() *TemplateMarketService {
	return NewTemplateMarketServiceWithRepo(repository.NewMemoryTemplateMarketRepository())
}

func TestTemplateMarketService_List_Filters(t *testing.T) {
	ctx := context.Background()
	svc := realMarketService()

	tests := []struct {
		name string
		q    MarketQuery
		want func(got []MarketItem) bool
	}{
		{
			name: "零值查询返回全量目录",
			q:    MarketQuery{},
			want: func(got []MarketItem) bool { return len(got) == 13 },
		},
		{
			name: "按 kind 筛出技能",
			q:    MarketQuery{Kind: MarketKindSkill},
			want: func(got []MarketItem) bool {
				return len(got) == 4 && allMatch(got, func(it MarketItem) bool { return it.Kind == MarketKindSkill })
			},
		},
		{
			name: "按 kind 筛出模板",
			q:    MarketQuery{Kind: MarketKindTemplate},
			want: func(got []MarketItem) bool {
				return len(got) == 9 && allMatch(got, func(it MarketItem) bool { return it.Kind == MarketKindTemplate })
			},
		},
		{
			name: "按分类精确过滤 channel_card",
			q:    MarketQuery{Category: "channel_card"},
			want: func(got []MarketItem) bool {
				return len(got) == 5 && allMatch(got, func(it MarketItem) bool { return it.Category == "channel_card" })
			},
		},
		{
			name: "关键词忽略大小写命中名称",
			q:    MarketQuery{Keyword: "SOP"},
			want: func(got []MarketItem) bool {
				return len(got) == 1 && got[0].ID == "sop-reply"
			},
		},
		{
			name: "关键词命中 tags",
			q:    MarketQuery{Keyword: "mcp"},
			want: func(got []MarketItem) bool {
				return len(got) == 1 && got[0].ID == "skill-mcp-tool"
			},
		},
		{
			name: "kind+category 叠加过滤",
			q:    MarketQuery{Kind: MarketKindSkill, Category: "mcp"},
			want: func(got []MarketItem) bool {
				return len(got) == 1 && got[0].ID == "skill-mcp-tool"
			},
		},
		{
			name: "不存在的分类返回空列表（非 nil，前端好渲染空态）",
			q:    MarketQuery{Category: "no_such"},
			want: func(got []MarketItem) bool { return got != nil && len(got) == 0 },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.List(ctx, tt.q)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if !tt.want(got) {
				t.Fatalf("List(%+v) = %d 条, 断言失败", tt.q, len(got))
			}
		})
	}
}

func TestTemplateMarketService_List_StableOrder(t *testing.T) {
	ctx := context.Background()
	svc := realMarketService()

	a, err := svc.List(ctx, MarketQuery{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.List(ctx, MarketQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("两次 List 长度不一致: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			t.Fatalf("第 %d 条顺序不稳定: %s vs %s", i, a[i].ID, b[i].ID)
		}
	}
}

func TestTemplateMarketService_Get_Found(t *testing.T) {
	ctx := context.Background()
	svc := realMarketService()

	got, err := svc.Get(ctx, "card-douyin")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() 应返回条目")
	}
	if got.ID != "card-douyin" || got.Kind != MarketKindTemplate || got.Name == "" ||
		got.Category == "" || got.Summary == "" || got.Source == "" || got.Availability == "" {
		t.Fatalf("元数据契约字段不完整: %+v", got)
	}
}

func TestTemplateMarketService_Get_UnknownID_FailClosed(t *testing.T) {
	ctx := context.Background()
	svc := realMarketService()

	tests := []struct {
		name string
		id   string
	}{
		{name: "未知 ID", id: "no-such-entry"},
		{name: "空 ID", id: ""},
		{name: "空白 ID", id: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.Get(ctx, tt.id)
			if !errors.Is(err, ErrMarketEntryNotFound) {
				t.Fatalf("Get(%q) error = %v, 期望 errors.Is(ErrMarketEntryNotFound)", tt.id, err)
			}
			if got != nil {
				t.Fatalf("Get(%q) 应返回 nil 条目, got %+v", tt.id, got)
			}
		})
	}
}

func TestTemplateMarketService_Categories_DedupedSorted(t *testing.T) {
	ctx := context.Background()
	svc := realMarketService()

	got, err := svc.Categories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("Categories() 不应为空")
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("分类未升序或重复: %q >= %q", got[i-1], got[i])
		}
	}
	// 目录里必然包含这两个分类。
	seen := map[string]bool{}
	for _, c := range got {
		seen[c] = true
	}
	for _, want := range []string{"channel_card", "mcp"} {
		if !seen[want] {
			t.Fatalf("Categories() 缺少 %q, got %v", want, got)
		}
	}
}

func TestTemplateMarketService_RepositoryError_Propagates(t *testing.T) {
	ctx := context.Background()
	boom := errors.New("目录数据源不可用")
	svc := NewTemplateMarketServiceWithRepo(&stubMarketRepo{err: boom})

	if _, err := svc.List(ctx, MarketQuery{}); !errors.Is(err, boom) {
		t.Fatalf("List() 应透传仓储错误, got %v", err)
	}
	if _, err := svc.Categories(ctx); !errors.Is(err, boom) {
		t.Fatalf("Categories() 应透传仓储错误, got %v", err)
	}
	if _, err := svc.Get(ctx, "any"); !errors.Is(err, boom) {
		t.Fatalf("Get() 应透传仓储错误, got %v", err)
	}
}

func allMatch(items []MarketItem, pred func(MarketItem) bool) bool {
	for _, it := range items {
		if !pred(it) {
			return false
		}
	}
	return true
}
