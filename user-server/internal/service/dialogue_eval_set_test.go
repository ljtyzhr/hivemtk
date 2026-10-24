package service

// dialogue_eval_set_test.go P0-1 残点 5：CSV 评测集的导入→读回→导出血节对账。
//
// AC：导入 N 条 → 评测集可读回（ListEvalSets 看到条数、loadEvalSetSamples 读到样本、
// 能作为评测入口跑出 runs）→ 导出与导入字节对账（export(import(x)) == x 的规范形）。

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/aiagent/eval"
	"hivemtk-user/internal/model"
)

func buildEvalSetCSV(t *testing.T) string {
	t.Helper()
	samples := []eval.DialogueSample{
		{
			ID: "set-1", Scene: "price_inquiry",
			Turns: []eval.DialogueTurn{
				{Role: eval.DialogueTurnRoleUser, Text: "多少钱？"},
				{Role: eval.DialogueTurnRoleAssistant, Text: "4999 元。", ExpectedAction: "quote", MustContain: []string{"4999"}},
			},
			Degraded: []eval.DialogueTurn{
				{Role: eval.DialogueTurnRoleUser, Text: "多少钱？"},
				{Role: eval.DialogueTurnRoleAssistant, Text: "请稍等。", ExpectedAction: "defer"},
			},
		},
		{
			ID: "set-2", Scene: "objection_price",
			Turns: []eval.DialogueTurn{
				{Role: eval.DialogueTurnRoleUser, Text: "太贵。"},
				{Role: eval.DialogueTurnRoleAssistant, Text: "可年付优惠。"},
			},
			Degraded: []eval.DialogueTurn{
				{Role: eval.DialogueTurnRoleUser, Text: "太贵。"},
				{Role: eval.DialogueTurnRoleAssistant, Text: "嗯。"},
			},
		},
	}
	var b strings.Builder
	if err := eval.ExportDialogueCSV(&b, samples); err != nil {
		t.Fatalf("构造夹具失败: %v", err)
	}
	return b.String()
}

func TestEvalSetCSV_ImportReadBackExportByteIdentical(t *testing.T) {
	db := dialogueEvalTestDB(t)
	if err := db.AutoMigrate(&model.DialogueEvalSet{}, &model.DialogueEvalRun{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	svc := NewDialogueEvalService(nil)
	ctx := context.Background()

	in := buildEvalSetCSV(t)
	set, err := svc.ImportEvalSetCSV(ctx, "月付题集", in)
	if err != nil {
		t.Fatalf("导入失败: %v", err)
	}
	if set.SampleCount != 2 {
		t.Fatalf("SampleCount = %d want 2", set.SampleCount)
	}

	// 可读回 ①：列表
	sets, err := svc.ListEvalSets(ctx, 10)
	if err != nil || len(sets) != 1 || sets[0].ID != set.ID {
		t.Fatalf("列表读回失败: sets=%+v err=%v", sets, err)
	}
	if sets[0].Payload != "" {
		t.Error("ListEvalSets 不应返回 Payload 大字段")
	}

	// 可读回 ②：样本可加载（供 RunGoldEvaluationWithSet 使用）
	samples, err := svc.loadEvalSetSamples(ctx, set.ID)
	if err != nil || len(samples) != 2 {
		t.Fatalf("样本读回失败: n=%d err=%v", len(samples), err)
	}

	// 可读回 ③：作为评测入口能跑出 runs（基线相至少一行）
	runs, err := svc.RunGoldEvaluationWithSet(ctx, "set-run", 0, set.ID)
	if err != nil {
		t.Fatalf("按集评测失败: %v", err)
	}
	if len(runs) == 0 || runs[0].SampleCount != 2 {
		t.Fatalf("评测 runs 异常: %+v", runs)
	}

	// 字节对账：导出 == 导入（规范形恒等）
	out, exported, err := svc.ExportEvalSetCSV(ctx, set.ID)
	if err != nil {
		t.Fatalf("导出失败: %v", err)
	}
	if exported.SampleCount != 2 {
		t.Errorf("导出集条数 = %d want 2", exported.SampleCount)
	}
	if out != in {
		t.Errorf("字节对账失败：\n导入:\n%s\n导出:\n%s", in, out)
	}
}

func TestEvalSetCSV_ImportRejectsBrokenCSV(t *testing.T) {
	db := dialogueEvalTestDB(t)
	if err := db.AutoMigrate(&model.DialogueEvalSet{}); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}
	// 测试库槽位进程内复用：先清空，计数才只反映本用例。
	if err := db.Exec("DELETE FROM dialogue_eval_sets").Error; err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	svc := NewDialogueEvalService(nil)
	if _, err := svc.ImportEvalSetCSV(context.Background(), "坏集", "id,scene\nx,y\n"); err == nil {
		t.Fatal("错表头应被拒绝")
	}
	var n int64
	db.Model(&model.DialogueEvalSet{}).Count(&n)
	if n != 0 {
		t.Errorf("拒绝导入后不应落库，实际 %d 行", n)
	}
}
