// dialogue_eval_service.go 对话级 gold 评测的生产接线（P0-1 / G12）。
//
// 现状：aiagent/eval 的 RunDialogueGold 运行器只有测试调用方（孤岛），本文件把它
// 接进生产：加载 gold 集（testdata 基线 + bad_case 导出送料）→ 双相评分（基线 +
// 退化对照）→ 落 dialogue_eval_runs（每相一行）。
//
// 形状对齐 rag_eval_cron.go / controller/rag_eval.go（同族先例，AC 要求逐形状复用）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"hivemtk-user/internal/aiagent/eval"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// DialogueEvalService 对话级 gold 评测服务。
type DialogueEvalService struct {
	badCase *BadCaseService
	repo    *repository.DialogueEvalRepository
}

// NewDialogueEvalService 构造。badCase 可为 nil（只用内置 gold 集）。
// 2026-10-25：落库/查询下沉 repository（L3 铁律）。
func NewDialogueEvalService(badCase *BadCaseService) *DialogueEvalService {
	return &DialogueEvalService{badCase: badCase, repo: repository.NewDialogueEvalRepository()}
}

// dialogueGoldSamples 组装本轮评测样本：内置 testdata 基线集 + bad_case 导出（送料口
// BadCaseToDialogueSample，sample.CaseID 可追溯）。bad_case 不可用只降级不报错——
// gold 基线集永远可跑，送料是增量。
func (s *DialogueEvalService) dialogueGoldSamples(ctx context.Context, badCaseLimit int) ([]eval.DialogueSample, int, error) {
	samples, err := eval.LoadDialogueGold(dialogueGoldDataPath())
	if err != nil {
		return nil, 0, fmt.Errorf("加载内置 gold 集失败: %w", err)
	}
	fedFromBadCase := 0
	if s.badCase != nil && s.badCase.Available() && badCaseLimit > 0 {
		exp, err := s.badCase.ExportEvalSet(ctx, nil, badCaseLimit)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Msg("[dialogue_eval] bad_case 导出失败，本轮只用内置 gold 集")
		} else {
			for i := range exp.Rows {
				samples = append(samples, eval.BadCaseToDialogueSample(&model.BadCase{
					ID:         exp.Rows[i].CaseID,
					Label:      exp.Rows[i].Label,
					FixLayer:   exp.Rows[i].FixLayer,
					QueryText:  exp.Rows[i].QueryText,
					AnswerText: exp.Rows[i].AnswerText,
				}))
				fedFromBadCase++
			}
		}
	}
	return samples, fedFromBadCase, nil
}

// dialogueGoldDataPath 定位内置 gold 集（testdata 相对源码，go:embed 之外的文件路径
// 取法与 rag_eval_auto 的 fixture 一致——运行时从编译期源路径推导，测试可注入覆盖）。
var dialogueGoldDataPath = func() string {
	_, file, _, _ := runtime.Caller(0)
	// 本文件在 internal/service/，gold 集在 internal/aiagent/eval/testdata/。
	return filepath.Join(filepath.Dir(file), "..", "aiagent", "eval", "testdata", "dialogue_gold.json")
}

// RunGoldEvaluation 执行一轮对话级评测：基线相 + 退化相各落一行 dialogue_eval_runs。
// 退化相要求每个样本都有 degraded transcript；bad_case 送料的样本没有（BadCaseToDialogueSample
// 不造 degraded），因此退化相只跑有 degraded 的子集（样本不足 2 个时跳过该相并记原因）。
func (s *DialogueEvalService) RunGoldEvaluation(ctx context.Context, name string, badCaseLimit int) ([]*model.DialogueEvalRun, error) {
	if name == "" {
		name = "dialogue_gold_" + time.Now().Format("20060102")
	}
	samples, fed, err := s.dialogueGoldSamples(ctx, badCaseLimit)
	if err != nil {
		return nil, err
	}
	return s.runGold(ctx, name, samples, fed)
}

// RunGoldEvaluationWithSet 对导入的评测集（dialogue_eval_sets）跑一轮：与 bad_case
// 送料并行的那条入口。evalSetID 为 0 时等价于 RunGoldEvaluation。
func (s *DialogueEvalService) RunGoldEvaluationWithSet(ctx context.Context, name string, badCaseLimit int, evalSetID uint) ([]*model.DialogueEvalRun, error) {
	if evalSetID == 0 {
		return s.RunGoldEvaluation(ctx, name, badCaseLimit)
	}
	if name == "" {
		name = "dialogue_set_" + time.Now().Format("20060102")
	}
	samples, err := s.loadEvalSetSamples(ctx, evalSetID)
	if err != nil {
		return nil, err
	}
	fed := 0
	if s.badCase != nil && s.badCase.Available() && badCaseLimit > 0 {
		exp, err := s.badCase.ExportEvalSet(ctx, nil, badCaseLimit)
		if err != nil {
			logger.Ctx(ctx).Warn().Err(err).Msg("[dialogue_eval] bad_case 导出失败，本轮只用导入集")
		} else {
			for i := range exp.Rows {
				samples = append(samples, eval.BadCaseToDialogueSample(&model.BadCase{
					ID:         exp.Rows[i].CaseID,
					Label:      exp.Rows[i].Label,
					FixLayer:   exp.Rows[i].FixLayer,
					QueryText:  exp.Rows[i].QueryText,
					AnswerText: exp.Rows[i].AnswerText,
				}))
				fed++
			}
		}
	}
	return s.runGold(ctx, name, samples, fed)
}

// runGold 双相评分并落库（两个入口的公共后半段）。
func (s *DialogueEvalService) runGold(ctx context.Context, name string, samples []eval.DialogueSample, fed int) ([]*model.DialogueEvalRun, error) {
	if len(samples) == 0 {
		return nil, fmt.Errorf("dialogue gold: 样本集为空")
	}
	chrf := eval.NewChrFEvaluator()
	runs := make([]*model.DialogueEvalRun, 0, 2)

	// 基线相
	baseline, err := eval.RunDialogueGold(chrf, samples, false)
	if err != nil {
		return nil, fmt.Errorf("基线相失败: %w", err)
	}
	runs = append(runs, s.saveRun(ctx, name, "baseline", fed, baseline))

	// 退化相（只跑有 degraded 的样本——bad_case 送料样本无 degraded 是形状事实，
	// 不是数据腐烂；有 degraded 的样本数 ≥2 才有统计意义）
	degraded := make([]eval.DialogueSample, 0, len(samples))
	for _, sm := range samples {
		if len(sm.Degraded) > 0 {
			degraded = append(degraded, sm)
		}
	}
	if len(degraded) >= 2 {
		deg, err := eval.RunDialogueGold(chrf, degraded, true)
		if err != nil {
			return nil, fmt.Errorf("退化相失败: %w", err)
		}
		runs = append(runs, s.saveRun(ctx, name, "degraded", 0, deg))
	} else {
		logger.Ctx(ctx).Info().Int("degraded_count", len(degraded)).
			Msg("[dialogue_eval] 有 degraded 的样本不足 2 个，本轮跳过退化相")
	}
	return runs, nil
}

// saveRun 落一行。phase 写进 Name（表无 phase 列，Name 是唯一区分位——与
// gold-baseline-<日期> / gold-degraded-<日期> 的既有惯例一致）。
func (s *DialogueEvalService) saveRun(ctx context.Context, name, phase string, fed int, res eval.DialogueGoldResult) *model.DialogueEvalRun {
	details, _ := json.Marshal(res.Samples)
	run := &model.DialogueEvalRun{
		Name:            fmt.Sprintf("%s-%s", name, phase),
		SampleCount:     res.SampleCount,
		CompositeAvg:    res.CompositeAvg,
		KeyCoverageAvg:  res.KeyCoverageAvg,
		ActionMatchRate: res.ActionMatchRate,
		IntentHitRate:   res.IntentHitRate,
		ChrfAvg:         res.ChrfAvg,
		Details:         string(details),
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("name", run.Name).Msg("[dialogue_eval] 落库失败")
		return run
	}
	logger.Ctx(ctx).Info().
		Str("name", run.Name).
		Int("samples", run.SampleCount).
		Float64("composite", run.CompositeAvg).
		Msg("[dialogue_eval] gold run 已落库")
	return run
}

// ListDialogueEvalRuns 按时间倒序列出运行（controller List 用）。
func (s *DialogueEvalService) ListDialogueEvalRuns(ctx context.Context, limit int) ([]*model.DialogueEvalRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListRuns(ctx, limit)
}

// GetDialogueEvalRun 单条详情。
func (s *DialogueEvalService) GetDialogueEvalRun(ctx context.Context, id uint) (*model.DialogueEvalRun, error) {
	return s.repo.GetRun(ctx, id)
}
