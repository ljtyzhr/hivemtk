package repository

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// C-11 失效锚语义对：TouchAllEnabled 必须抬 updated_at（内容变更判陈答案缓存），
// UpdateVersionCanary 必须不抬（版本切换保旧命名空间缓存行作回滚）。
// 两个方向各钉一条腿——谁漂了，rag_answer_cache 要么对内容变更脏读、要么在版本
// 回滚时把旧缓存整批删空退化成"重灌数据"。
func TestKnowledgeBaseTouchSemantics(t *testing.T) {
	database := testutil.NewTestDB(t, &model.KnowledgeBase{})
	repo := NewKnowledgeBaseRepository(database)
	ctx := context.Background()

	onFlag, offFlag := true, false
	old := time.Now().Add(-48 * time.Hour)
	enabled := &model.KnowledgeBase{
		KBCode: "kb-touch-on", Type: model.KnowledgeBaseTypeFAQ,
		Name: "启用库", OwnerType: model.KnowledgeBaseOwnerPrivate, Enabled: &onFlag,
	}
	disabled := &model.KnowledgeBase{
		KBCode: "kb-touch-off", Type: model.KnowledgeBaseTypeFAQ,
		Name: "停用库", OwnerType: model.KnowledgeBaseOwnerPrivate, Enabled: &offFlag,
	}
	if err := repo.Create(ctx, enabled); err != nil {
		t.Fatalf("预置启用 KB: %v", err)
	}
	if err := repo.Create(ctx, disabled); err != nil {
		t.Fatalf("预置停用 KB: %v", err)
	}
	// Create 会写 now；把两行冻结到 48h 前，让"有没有被抬"可判。
	if err := database.Model(&model.KnowledgeBase{}).
		Where("id IN ?", []uint{enabled.ID, disabled.ID}).
		UpdateColumn("updated_at", old).Error; err != nil {
		t.Fatalf("冻结 updated_at: %v", err)
	}

	if err := repo.TouchAllEnabled(ctx); err != nil {
		t.Fatalf("TouchAllEnabled 不该失败: %v", err)
	}

	var on, off model.KnowledgeBase
	if err := database.First(&on, enabled.ID).Error; err != nil {
		t.Fatalf("回读启用 KB: %v", err)
	}
	if err := database.First(&off, disabled.ID).Error; err != nil {
		t.Fatalf("回读停用 KB: %v", err)
	}
	if !on.UpdatedAt.After(old) {
		t.Fatalf("启用 KB 的 updated_at 未被抬起（%v）——内容变更后答案缓存永久失明", on.UpdatedAt)
	}
	if !off.UpdatedAt.Before(time.Now().Add(-24 * time.Hour)) {
		t.Fatalf("停用 KB 的 updated_at 不该被抬（%v）——它不参与检索，抬了是空转", off.UpdatedAt)
	}

	// 版本切换的"刻意不抬"：UpdateColumns 跳过自动时间戳，抬了会把另一版本的
	// 缓存行整批删空，回滚从"换个指针"退化成"重灌数据"。
	before := on.UpdatedAt
	time.Sleep(20 * time.Millisecond)
	if err := repo.UpdateVersionCanary(ctx, enabled.ID, 7, true, 10); err != nil {
		t.Fatalf("UpdateVersionCanary: %v", err)
	}
	var after model.KnowledgeBase
	if err := database.First(&after, enabled.ID).Error; err != nil {
		t.Fatalf("回读版本切换后的 KB: %v", err)
	}
	if !after.UpdatedAt.Equal(before) {
		t.Fatalf("版本切换抬了 updated_at（%v → %v）：旧命名空间缓存行会被整批删空", before, after.UpdatedAt)
	}
	if after.Version != 7 {
		t.Fatalf("版本未写入：version = %d，期望 7", after.Version)
	}
}
