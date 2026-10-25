package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/aiagent/eval"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
)

// dialogueEvalTestDB 测试库底座（testutil.NewTestDB，AutoMigrate 由调用方指定模型）。
func dialogueEvalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t)
}

// TestRunGoldEvaluation_WritesBothPhasesAndDegradedDrops P0-1 核心 AC：
// ①基线+退化两相各落一行；②退化相 composite 显著低于基线相（沿用 gold 框架 AC③ 自证形状）。
func TestRunGoldEvaluation_WritesBothPhasesAndDegradedDrops(t *testing.T) {
	db := dialogueEvalTestDB(t)
	db.AutoMigrate(&model.DialogueEvalRun{})

	svc := NewDialogueEvalService(nil) // bad_case 不装配：只用内置 gold 集
	runs, err := svc.RunGoldEvaluation(context.Background(), "ut_gold", 0)
	if err != nil {
		t.Fatalf("RunGoldEvaluation: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("期望基线+退化两行，实际 %d 行", len(runs))
	}
	if !strings.HasSuffix(runs[0].Name, "-baseline") || !strings.HasSuffix(runs[1].Name, "-degraded") {
		t.Errorf("Name 应带相位后缀: %q / %q", runs[0].Name, runs[1].Name)
	}
	if runs[1].CompositeAvg >= runs[0].CompositeAvg {
		t.Errorf("退化相 composite(%f) 应显著低于基线相(%f)", runs[1].CompositeAvg, runs[0].CompositeAvg)
	}
}

// TestRunGoldEvaluation_BadCaseFeed 送料管道：bad_case 导出行转 gold 样本，CaseID 可追溯。
func TestRunGoldEvaluation_BadCaseFeed(t *testing.T) {
	db := dialogueEvalTestDB(t)
	db.AutoMigrate(&model.DialogueEvalRun{}, &model.BadCase{})

	// 直接构造 service 层级的 fake bad_case 服务不可行（repo 依赖 DB）——
	// 这里单测送料函数本身：BadCaseToDialogueSample 产出的样本无 degraded，
	// 只应进基线相、不进退化相（形状事实锁定）。
	s := eval.BadCaseToDialogueSample(&model.BadCase{
		ID: "bc_ut_1", Label: "wrong_answer",
		QueryText: "怎么退款？", AnswerText: "联系客服处理。",
	})
	if len(s.Degraded) != 0 {
		t.Fatal("bad_case 送料样本不应有 degraded")
	}
	if s.CaseID != "bc_ut_1" || s.ID != "badcase-bc_ut_1" {
		t.Errorf("CaseID/ID 可追溯性破坏: %q / %q", s.CaseID, s.ID)
	}
}

// TestRunGoldEvaluation_GoldSetExpansionGate AC：gold 集 ≥50 条。
// 读同一 JSON（生产路径 dialogueGoldDataPath），失败=扩集未达标。
func TestRunGoldEvaluation_GoldSetExpansionGate(t *testing.T) {
	samples, err := eval.LoadDialogueGold(dialogueGoldDataPath())
	if err != nil {
		t.Fatalf("加载 gold 集: %v", err)
	}
	if len(samples) < 50 {
		t.Fatalf("gold 集 %d 条 < 50（P0-1 扩集 AC 未达标）", len(samples))
	}
}

// TestRunGoldEvaluation_IntentHitRate P0-1 AC：intent_hit 维度入 Details 且进汇总。
func TestRunGoldEvaluation_IntentHitRate(t *testing.T) {
	// 带期望意图的小样本直接跑运行器（不依赖 DB）：
	chrf := eval.NewChrFEvaluator()
	samples := []eval.DialogueSample{{
		ID: "it-1", Scene: "quote",
		Turns: []eval.DialogueTurn{
			{Role: eval.DialogueTurnRoleUser, Text: "多少钱"},
			{Role: eval.DialogueTurnRoleAssistant, Text: "4999 元", ExpectedAction: "quote", ExpectedIntent: "price_inquiry"},
		},
		Degraded: []eval.DialogueTurn{
			{Role: eval.DialogueTurnRoleUser, Text: "多少钱"},
			{Role: eval.DialogueTurnRoleAssistant, Text: "您好", ExpectedAction: "greet", ExpectedIntent: "greeting"},
		},
	}}
	base, err := eval.RunDialogueGold(chrf, samples, false)
	if err != nil {
		t.Fatal(err)
	}
	if base.IntentHitRate != 1.0 {
		t.Errorf("基线 intent 命中应为 1.0，实际 %f", base.IntentHitRate)
	}
	deg, err := eval.RunDialogueGold(chrf, samples, true)
	if err != nil {
		t.Fatal(err)
	}
	if deg.IntentHitRate != 0 {
		t.Errorf("退化 intent 应 0 命中，实际 %f", deg.IntentHitRate)
	}
	// Details 序列化含 intent 字段
	raw, _ := json.Marshal(base.Samples)
	if !strings.Contains(string(raw), "intent_hit") {
		t.Error("Details JSON 应含 intent_hit 字段")
	}
}

// TestDialogueGoldDataPath 路径推导指向真实文件。
func TestDialogueGoldDataPath(t *testing.T) {
	p := dialogueGoldDataPath()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("gold 集 %s 不可读: %v", p, err)
	}
	if filepath.Base(p) != "dialogue_gold.json" {
		t.Errorf("文件名漂移: %s", p)
	}
}
