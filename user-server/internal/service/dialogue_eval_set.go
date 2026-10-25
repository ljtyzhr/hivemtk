package service

// dialogue_eval_set.go P0-1 残点 5：评测集 CSV 导入/导出与回读（对标 Dify 批量评测）。
//
// 与 bad_case 管道并行：bad_case 是「生产里出过的问题自动回流」，CSV 是
// 「运营手工编制的题集」。两条入口都汇进同一个评测运行（RunGoldEvaluation）。

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hivemtk-user/internal/aiagent/eval"
	"hivemtk-user/internal/model"
)

// ImportEvalSetCSV 解析 CSV 并落一行 dialogue_eval_sets。
// 任何结构错误整体拒绝（eval.ImportDialogueCSV 的语义），不做部分导入。
func (s *DialogueEvalService) ImportEvalSetCSV(ctx context.Context, name, csvText string) (*model.DialogueEvalSet, error) {
	if strings.TrimSpace(csvText) == "" {
		return nil, fmt.Errorf("CSV 内容为空")
	}
	samples, err := eval.ImportDialogueCSV(strings.NewReader(csvText))
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(samples)
	if err != nil {
		return nil, fmt.Errorf("样本序列化失败: %w", err)
	}
	if strings.TrimSpace(name) == "" {
		name = "imported_eval_set"
	}
	set := &model.DialogueEvalSet{Name: name, SampleCount: len(samples), Payload: string(payload)}
	if err := s.repo.CreateSet(ctx, set); err != nil {
		return nil, fmt.Errorf("落库失败: %w", err)
	}
	return set, nil
}

// ListEvalSets 最近的评测集（不含 Payload，列表页不拉大字段）。
func (s *DialogueEvalService) ListEvalSets(ctx context.Context, limit int) ([]model.DialogueEvalSet, error) {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	return s.repo.ListSets(ctx, limit)
}

// ExportEvalSetCSV 按规范形导出该评测集，返回 CSV 文本。
func (s *DialogueEvalService) ExportEvalSetCSV(ctx context.Context, id uint) (string, *model.DialogueEvalSet, error) {
	set, err := s.repo.GetSet(ctx, id)
	if err != nil {
		return "", nil, fmt.Errorf("评测集不存在")
	}
	var samples []eval.DialogueSample
	if err := json.Unmarshal([]byte(set.Payload), &samples); err != nil {
		return "", nil, fmt.Errorf("评测集载荷损坏: %w", err)
	}
	var buf strings.Builder
	if err := eval.ExportDialogueCSV(&buf, samples); err != nil {
		return "", nil, err
	}
	return buf.String(), set, nil
}

// loadEvalSetSamples 读回导入集的样本（RunGoldEvaluation 的 evalSetID 入口）。
func (s *DialogueEvalService) loadEvalSetSamples(ctx context.Context, id uint) ([]eval.DialogueSample, error) {
	set, err := s.repo.GetSet(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("评测集 %d 不存在", id)
	}
	var samples []eval.DialogueSample
	if err := json.Unmarshal([]byte(set.Payload), &samples); err != nil {
		return nil, fmt.Errorf("评测集载荷损坏: %w", err)
	}
	if len(samples) == 0 {
		return nil, fmt.Errorf("评测集 %d 为空", id)
	}
	return samples, nil
}
