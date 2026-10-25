package service

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"

	"hivemtk-user/internal/repository"
)

// newDictionaryDBFixture 起一个带字典两表的真库。
func newDictionaryDBFixture(t *testing.T) (*DictionaryService, *gorm.DB) {
	t.Helper()
	database := testutil.NewTestDBOrSkip(t, &model.Dictionary{}, &model.DictionaryTransition{})
	if database == nil {
		t.Skip("no test DB")
	}
	return NewDictionaryService(repository.NewDictionaryRepository(database)), database
}

// TestDictionaryDBSeedableAndReadBack 真库路径：建表 → Upsert 落库 → 读回 DB 版。
// 这一条钉的是「种子可落库」这个管理端主流程（种子本身不可改，落地后由 DB 优先）。
func TestDictionaryDBSeedableAndReadBack(t *testing.T) {
	svc, _ := newDictionaryDBFixture(t)
	ctx := context.Background()

	for _, d := range []*model.Dictionary{
		{ParentCode: "demo_status", Code: "open", Name: "开", SortOrder: 1, Enabled: true},
		{ParentCode: "demo_status", Code: "done", Name: "完成", SortOrder: 2, Enabled: true},
	} {
		if err := svc.UpsertItem(ctx, d, 1); err != nil {
			t.Fatalf("UpsertItem(%s) err=%v", d.Code, err)
		}
	}
	if err := svc.ReplaceTransitions(ctx, "demo_status", []model.DictionaryTransition{
		{FromCode: "open", ToCode: "done", Description: "完成它"},
	}, 1); err != nil {
		t.Fatalf("ReplaceTransitions err=%v", err)
	}

	items, trans, err := svc.GetDictionary(ctx, "demo_status")
	if err != nil {
		t.Fatalf("GetDictionary err=%v", err)
	}
	if len(items) != 2 {
		t.Fatalf("应有 2 项, got %d", len(items))
	}
	if items[0].Enabled != true || items[0].Name != "开" {
		t.Errorf("落库/读回不一致: %+v", items[0])
	}
	if len(trans) != 1 || trans[0].ToCode != "done" {
		t.Errorf("跃迁落库/读回不一致: %+v", trans)
	}
}

// TestDictionaryDBRepoNilSafe 仓储没装 DB 时返回哨兵错而不是 panic
// （与 0.4/0.5/3.2 三处 NilDB 修复同一口径）。
func TestDictionaryDBRepoNilSafe(t *testing.T) {
	svc := NewDictionaryService(nil)
	ctx := context.Background()
	if _, _, err := svc.GetDictionary(ctx, "approval_status"); err != nil {
		// DB 不可用但种子可用时回落种子是允许的；这里只要求不 panic。
		t.Logf("GetDictionary with nil db err=%v（种子回落）", err)
	}
	if _, err := svc.ListParents(ctx); err == nil {
		t.Log("ListParents with nil db 未报错（可能走了种子分支）")
	}
	if err := svc.UpsertItem(ctx, &model.Dictionary{
		ParentCode: "x", Code: "a", Name: "A",
	}, 1); err == nil {
		t.Error("nil db 时 UpsertItem 应返回哨兵错")
	}
}
