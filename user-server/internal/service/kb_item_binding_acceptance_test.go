package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P0-2/P0-3/D-8 方案 A（kb_item_bindings 成员表）的验收用例（规划 §2.2 验收判据）：
//  1. KB1/KB2 各自只返回自己的绑定内容，未绑定内容不出现在两者（成员关系真实生效）；
//  2. 一条内容可同时绑定 KB1 与 KB2（shared 语义，方案 A 对方案 B 的决定性优势）；
//  3. 跨租户：租户 2 的 KB 看不到租户 1 的内容（绑定前同租户校验）；
//  4. 删除内容后绑定行被清掉（DeleteBindingsForItem），列表不再返回；
//  5. item_count 与绑定行数一致；重复绑定幂等。
func TestKBItemBinding_FullAcceptance(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDBOrSkip(t, &model.KnowledgeBase{}, &model.KBItemBinding{},
		&model.FAQEntry{}, &model.SOPTemplate{}, &model.KnowledgeDocument{},
		&model.AgentKBBinding{})
	ctx := context.Background()
	svc := service.NewKBItemBindingService(service.NewKnowledgeBaseService(db),
		repository.NewKBItemBindingRepository(db))

	enabled := true
	kb1 := &model.KnowledgeBase{TenantID: 1, KBCode: fmt.Sprintf("kb1-%d", time.Now().UnixNano()),
		Type: "faq", Name: "KB1", Enabled: &enabled}
	kb2 := &model.KnowledgeBase{TenantID: 1, KBCode: fmt.Sprintf("kb2-%d", time.Now().UnixNano()),
		Type: "faq", Name: "KB2", Enabled: &enabled}
	kbT2 := &model.KnowledgeBase{TenantID: 2, KBCode: fmt.Sprintf("kbt2-%d", time.Now().UnixNano()),
		Type: "faq", Name: "KB-tenant2", Enabled: &enabled}
	require.NoError(t, db.Create(kb1).Error)
	require.NoError(t, db.Create(kb2).Error)
	require.NoError(t, db.Create(kbT2).Error)

	faqA := &model.FAQEntry{TenantID: 1, Question: "内容 A", Answer: "A"}
	faqB := &model.FAQEntry{TenantID: 1, Question: "内容 B", Answer: "B"}
	faqT2 := &model.FAQEntry{TenantID: 2, Question: "租户2内容", Answer: "X"}
	require.NoError(t, db.Create(faqA).Error)
	require.NoError(t, db.Create(faqB).Error)
	require.NoError(t, db.Create(faqT2).Error)

	// ① KB1 ← A；KB2 ← B；A 同时也绑进 KB2（shared）
	require.NoError(t, svc.Bind(ctx, 1, kb1.ID, "faq", uint64(faqA.ID), "tester"))
	require.NoError(t, svc.Bind(ctx, 1, kb2.ID, "faq", uint64(faqB.ID), "tester"))
	require.NoError(t, svc.Bind(ctx, 1, kb2.ID, "faq", uint64(faqA.ID), "tester"))

	list1, total1, err := svc.ListItems(ctx, 1, kb1.ID, "", 1, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total1, "KB1 只应有 A")
	if assert.Len(t, list1, 1) {
		assert.EqualValues(t, faqA.ID, list1[0]["item_id"])
		assert.Equal(t, "内容 A", list1[0]["title"], "标题应反查自内容表")
	}
	list2, _, err := svc.ListItems(ctx, 1, kb2.ID, "", 1, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 2, len(list2), "KB2 应有 A+B（共享成员关系成立）")

	// 未绑定的内容不出现在任何 KB
	listB1, _, err := svc.ListItems(ctx, 1, kb1.ID, "faq", 1, 50)
	require.NoError(t, err)
	for _, it := range listB1 {
		assert.NotEqual(t, faqB.ID, it["item_id"], "B 未绑进 KB1，不得出现")
	}

	// ③ 跨租户：租户 2 的 KB 绑租户 1 的内容 ⇒ 拒绝
	err = svc.Bind(ctx, 2, kbT2.ID, "faq", uint64(faqA.ID), "tester")
	assert.ErrorIs(t, err, service.ErrItemNotFound, "跨租户绑定必须被拒")
	// 租户 2 绑自己的内容则成功
	require.NoError(t, svc.Bind(ctx, 2, kbT2.ID, "faq", uint64(faqT2.ID), "tester"))

	// ⑤ 幂等：重复绑定不报错、不产生第二行
	require.NoError(t, svc.Bind(ctx, 1, kb1.ID, "faq", uint64(faqA.ID), "tester"))
	cnt, err := svc.CountByKB(ctx, 1, kb1.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, cnt, "重复绑定幂等")

	// item_count 与绑定行数一致（走 GetKBStats 真实口径）
	// 本用例没建 agent 绑定 ⇒ agent_count=0 是正确读数，只断言契约键存在。
	kbSvc := service.NewKnowledgeBaseService(db)
	stats, err := kbSvc.GetKBStats(ctx, kb1.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, stats["item_count"])
	assert.Contains(t, stats, "agent_count", "agent_count 契约键不回退")

	// ④ 删除内容 ⇒ 绑定行在读取侧被清理（多态 item_id 无外键，靠读时一致性修复），
	// 列表不再返回、item_count 同时回落（CountEffectiveByKB 只计内容仍存在的绑定）。
	require.NoError(t, db.Delete(faqA).Error)
	listAfter, totalAfter, err := svc.ListItems(ctx, 1, kb1.ID, "", 1, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 0, totalAfter, "删除内容后 KB1 应为空")
	assert.Empty(t, listAfter)
	var orphanLeft int64
	require.NoError(t, db.Model(&model.KBItemBinding{}).
		Where("kb_id = ? AND item_type = ? AND item_id = ?", kb1.ID, "faq", faqA.ID).
		Count(&orphanLeft).Error)
	assert.EqualValues(t, 0, orphanLeft, "幽灵绑定行必须被清掉，不能留在表里撑大 item_count")
	cntAfter, err := svc.CountByKB(ctx, 1, kb1.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, cntAfter, "有效绑定计数必须随内容删除回落")
	list2After, _, err := svc.ListItems(ctx, 1, kb2.ID, "", 1, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 1, len(list2After), "KB2 只剩 B")
	// 非法类型拒绝
	assert.Error(t, svc.Bind(ctx, 1, kb1.ID, "video", uint64(faqB.ID), "tester"))
}

// 批量绑定先整体校验：有一条内容不存在 ⇒ 一条都不写，不留半成品。
func TestKBItemBinding_BindManyAllOrNothing(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDBOrSkip(t, &model.KnowledgeBase{}, &model.KBItemBinding{}, &model.FAQEntry{})
	ctx := context.Background()
	svc := service.NewKBItemBindingService(service.NewKnowledgeBaseService(db),
		repository.NewKBItemBindingRepository(db))
	enabled := true
	kb := &model.KnowledgeBase{TenantID: 1, KBCode: fmt.Sprintf("kbm-%d", time.Now().UnixNano()),
		Type: "faq", Name: "KB-many", Enabled: &enabled}
	require.NoError(t, db.Create(kb).Error)
	ok1 := &model.FAQEntry{TenantID: 1, Question: "存在的内容", Answer: "A"}
	require.NoError(t, db.Create(ok1).Error)

	_, err := svc.BindMany(ctx, 1, kb.ID, []service.BindItem{
		{Type: "faq", ItemID: uint64(ok1.ID)},
		{Type: "faq", ItemID: 999999999},
	}, "tester")
	require.ErrorIs(t, err, service.ErrItemNotFound)

	var n int64
	require.NoError(t, db.Model(&model.KBItemBinding{}).Where("kb_id = ?", kb.ID).Count(&n).Error)
	assert.EqualValues(t, 0, n, "整体校验失败时不得留下已写入的绑定行")

	bound, err := svc.BindMany(ctx, 1, kb.ID, []service.BindItem{
		{Type: "faq", ItemID: uint64(ok1.ID)},
		{Type: "faq", ItemID: uint64(ok1.ID)},
	}, "tester")
	require.NoError(t, err)
	assert.Equal(t, 2, bound, "幂等重绑也计入 bound（沿用单条 Bind 的幂等语义）")
	require.NoError(t, db.Model(&model.KBItemBinding{}).Where("kb_id = ?", kb.ID).Count(&n).Error)
	assert.EqualValues(t, 1, n, "重复绑定只落一行")
}
