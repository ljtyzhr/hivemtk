package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBindingRepoDB(t *testing.T) (repo *repository.KBItemBindingRepository, kbID uint, faqIDs []uint64, docID uint64) {
	t.Helper()
	db := testutil.NewTestDBOrSkip(t, &model.KnowledgeBase{}, &model.KBItemBinding{}, &model.FAQEntry{},
		&model.SOPTemplate{}, &model.KnowledgeDocument{})
	enabled := true
	kb := &model.KnowledgeBase{TenantID: 1,
		KBCode: fmt.Sprintf("kbr-%d", time.Now().UnixNano()), Type: "faq", Name: "repo-fixture", Enabled: &enabled}
	require.NoError(t, db.Create(kb).Error)
	for i := 0; i < 3; i++ {
		faq := &model.FAQEntry{TenantID: 1, Question: fmt.Sprintf("Q%d", i), Answer: "A"}
		require.NoError(t, db.Create(faq).Error)
		faqIDs = append(faqIDs, uint64(faq.ID))
	}
	sop := &model.SOPTemplate{TenantID: 1, Name: "SOP-1"}
	require.NoError(t, db.Create(sop).Error)
	// rag 绑定腿需要真文档行（CountEffectiveByKB 对 rag 腿回查 knowledge_documents）
	doc := &model.KnowledgeDocument{TenantID: 1, Title: "repo-fixture-doc"}
	require.NoError(t, db.Create(doc).Error)
	return repository.NewKBItemBindingRepository(db), kb.ID, faqIDs, doc.ID
}

// TestKBItemBindingRepo_CRUDAndIdempotent 表驱动覆盖仓储 CRUD + 幂等判定。
func TestKBItemBindingRepo_CRUDAndIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	repo, kbID, faqIDs, docID := newBindingRepoDB(t)
	ctx := context.Background()

	bind := func(itemType string, itemID uint64) *model.KBItemBinding {
		return &model.KBItemBinding{TenantID: 1, KBID: kbID, ItemType: itemType, ItemID: itemID}
	}

	t.Run("Create+Exists+Count", func(t *testing.T) {
		require.NoError(t, repo.Create(ctx, bind("faq", faqIDs[0])))
		require.NoError(t, repo.Create(ctx, bind("faq", faqIDs[1])))
		require.NoError(t, repo.Create(ctx, bind("rag", docID)))

		ok, err := repo.Exists(ctx, 1, kbID, "faq", faqIDs[0])
		require.NoError(t, err)
		assert.True(t, ok, "Exists 应命中已写入绑定")
		ok, err = repo.Exists(ctx, 1, kbID, "faq", 999999999)
		require.NoError(t, err)
		assert.False(t, ok, "Exists 未写入的绑定应为 false")
		// 跨租户不可见
		ok, err = repo.Exists(ctx, 2, kbID, "faq", faqIDs[0])
		require.NoError(t, err)
		assert.False(t, ok, "租户 2 不得看到租户 1 的绑定")

		n, err := repo.CountEffectiveByKB(ctx, 1, kbID, "")
		require.NoError(t, err)
		assert.EqualValues(t, 3, n, "按类型过滤前 total=3")
		n, err = repo.CountEffectiveByKB(ctx, 1, kbID, "faq")
		require.NoError(t, err)
		assert.EqualValues(t, 2, n, "type=faq 计数=2")
		n, err = repo.CountEffectiveByKB(ctx, 1, kbID, "sop")
		require.NoError(t, err)
		assert.EqualValues(t, 0, n, "未绑 sop 计数=0（空列表是正确行为）")
	})

	t.Run("ListByKB 分页与类型过滤", func(t *testing.T) {
		list, total, err := repo.ListByKB(ctx, 1, kbID, "", 1, 2)
		require.NoError(t, err)
		assert.EqualValues(t, 3, total)
		assert.Len(t, list, 2, "pageSize=2 只应返回一页 2 行")

		list, total, err = repo.ListByKB(ctx, 1, kbID, "faq", 1, 50)
		require.NoError(t, err)
		assert.EqualValues(t, 2, total)
		for _, b := range list {
			assert.Equal(t, "faq", b.ItemType)
		}

		list, total, err = repo.ListByKB(ctx, 1, kbID, "sop", 1, 50)
		require.NoError(t, err)
		assert.EqualValues(t, 0, total)
		assert.Empty(t, list)
	})

	t.Run("Delete 解绑幂等", func(t *testing.T) {
		rows, err := repo.Delete(ctx, 1, kbID, "faq", faqIDs[0])
		require.NoError(t, err)
		assert.EqualValues(t, 1, rows, "首次解绑应删 1 行")
		// 幂等：再删同一行 rows=0 不报错
		rows, err = repo.Delete(ctx, 1, kbID, "faq", faqIDs[0])
		require.NoError(t, err)
		assert.EqualValues(t, 0, rows, "重复解绑应幂等（rows=0）")
		// 跨租户删不掉别人行
		rows, err = repo.Delete(ctx, 2, kbID, "faq", faqIDs[1])
		require.NoError(t, err)
		assert.EqualValues(t, 0, rows, "跨租户解绑不得生效")
	})

	t.Run("ItemExists 按内容表回查", func(t *testing.T) {
		cases := []struct {
			name     string
			itemType string
			itemID   uint64
			want     bool
		}{
			{"存在的 faq", "faq", faqIDs[1], true},
			{"不存在的 faq", "faq", 999999999, false},
			{"未绑定的 rag 文档", "rag", 999999999, false},
			{"非法类型", "video", 1, false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := repo.ItemExists(ctx, 1, tc.itemType, tc.itemID)
				if tc.itemType == "video" {
					assert.Error(t, err, "非法类型应返回错误")
					return
				}
				require.NoError(t, err)
				assert.Equal(t, tc.want, got)
			})
		}
	})

	t.Run("TitlesByIDs 键缺失即内容已删", func(t *testing.T) {
		titles, err := repo.TitlesByIDs(ctx, 1, "faq", []uint64{faqIDs[1], 999999999})
		require.NoError(t, err)
		assert.Contains(t, titles, faqIDs[1], "存活内容应返回标题键")
		assert.NotContains(t, titles, uint64(999999999), "已删/不存在内容键缺失")
	})

	t.Run("DeleteByIDs 按主键硬删（幽灵行清理）", func(t *testing.T) {
		list, _, err := repo.ListByKB(ctx, 1, kbID, "", 1, 50)
		require.NoError(t, err)
		require.NotEmpty(t, list)
		ids := make([]uint, 0, len(list))
		for _, b := range list {
			ids = append(ids, b.ID)
		}
		require.NoError(t, repo.DeleteByIDs(ctx, 1, ids))
		n, err := repo.CountEffectiveByKB(ctx, 1, kbID, "")
		require.NoError(t, err)
		assert.EqualValues(t, 0, n, "按主键清空后计数应为 0")
		// 跨租户 DeleteByIDs 不得生效：重新写一行，用租户 2 删
		b := bind("faq", faqIDs[2])
		require.NoError(t, repo.Create(ctx, b))
		require.NoError(t, repo.DeleteByIDs(ctx, 2, []uint{b.ID}))
		n, err = repo.CountEffectiveByKB(ctx, 1, kbID, "")
		require.NoError(t, err)
		assert.EqualValues(t, 1, n, "租户 2 按主键删不得影响租户 1 的行")
	})
}

// TestKBItemBindingRepo_CountEffectiveExcludesDeadItems：软删内容后
// CountEffectiveByKB 不再计入（只数"内容仍存在"的绑定）。
func TestKBItemBindingRepo_CountEffectiveExcludesDeadItems(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDBOrSkip(t, &model.KnowledgeBase{}, &model.KBItemBinding{}, &model.FAQEntry{})
	ctx := context.Background()
	repo := repository.NewKBItemBindingRepository(db)

	enabled := true
	kb := &model.KnowledgeBase{TenantID: 1,
		KBCode: fmt.Sprintf("kbd-%d", time.Now().UnixNano()), Type: "faq", Name: "dead-item", Enabled: &enabled}
	require.NoError(t, db.Create(kb).Error)
	faq := &model.FAQEntry{TenantID: 1, Question: "会被删的内容", Answer: "A"}
	require.NoError(t, db.Create(faq).Error)
	require.NoError(t, repo.Create(ctx, &model.KBItemBinding{
		TenantID: 1, KBID: kb.ID, ItemType: "faq", ItemID: uint64(faq.ID)}))

	n, err := repo.CountEffectiveByKB(ctx, 1, kb.ID, "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "删除前计数=1")

	require.NoError(t, db.Delete(faq).Error) // 软删（FAQEntry 带 DeletedAt）
	n, err = repo.CountEffectiveByKB(ctx, 1, kb.ID, "")
	require.NoError(t, err)
	assert.EqualValues(t, 0, n, "软删内容后有效计数应为 0（绑定行虽在但内容已不在）")
}
