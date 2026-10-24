package repository

import (
	"context"
	"testing"
)

// TestMarketCatalogSeed_Integrity 钉住目录种子的契约完整性：
// ID 唯一、kind/availability 值域合法、元数据字段非空、Source 可追溯。
// 加条目时必须让本条测试继续绿，否则前端筛选器会出现「同 ID 两张卡」这类脏数据。
func TestMarketCatalogSeed_Integrity(t *testing.T) {
	entries := marketCatalogSeed()

	validKind := map[string]bool{EntryKindTemplate: true, EntryKindSkill: true}
	validAvail := map[string]bool{
		AvailabilityBuiltin: true, AvailabilityNeedsModule: true, AvailabilityComingSoon: true,
	}

	seen := make(map[string]bool, len(entries))
	for i, e := range entries {
		if e.ID == "" {
			t.Fatalf("第 %d 条 ID 为空", i)
		}
		if seen[e.ID] {
			t.Fatalf("ID 重复: %s", e.ID)
		}
		seen[e.ID] = true

		if !validKind[e.Kind] {
			t.Fatalf("%s: 非法 kind %q", e.ID, e.Kind)
		}
		if !validAvail[e.Availability] {
			t.Fatalf("%s: 非法 availability %q", e.ID, e.Availability)
		}
		if e.Name == "" || e.Category == "" || e.Summary == "" || e.Source == "" {
			t.Fatalf("%s: 元数据字段不完整: %+v", e.ID, e)
		}
	}
	// 目录至少要同时覆盖模板与技能两类，否则「技能市场」是空话。
	hasTemplate, hasSkill := false, false
	for _, e := range entries {
		if e.Kind == EntryKindTemplate {
			hasTemplate = true
		}
		if e.Kind == EntryKindSkill {
			hasSkill = true
		}
	}
	if !hasTemplate || !hasSkill {
		t.Fatalf("目录必须同时含模板与技能: template=%v skill=%v", hasTemplate, hasSkill)
	}
}

func TestMemoryTemplateMarketRepository_All_ReturnsCopy(t *testing.T) {
	repo := NewMemoryTemplateMarketRepository()

	got, err := repo.All(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("All() 不应为空")
	}
	// 拿到的是副本：调用方改写结果不得污染目录。
	got[0].Name = "被篡改"
	again, _ := repo.All(context.Background())
	if again[0].Name == "被篡改" {
		t.Fatal("All() 应返回目录副本，调用方改写不应污染数据源")
	}
}

func TestMemoryTemplateMarketRepository_Get(t *testing.T) {
	repo := NewMemoryTemplateMarketRepository()
	ctx := context.Background()

	t.Run("命中", func(t *testing.T) {
		got, err := repo.Get(ctx, "card-douyin")
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || got.ID != "card-douyin" {
			t.Fatalf("Get(card-douyin) = %+v", got)
		}
	})
	t.Run("未命中返回 nil,nil", func(t *testing.T) {
		got, err := repo.Get(ctx, "no-such")
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("未命中应返回 nil, got %+v", got)
		}
	})
}
