package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// HumanizeLowQualitySampleCollector 低质样本收集器
//
// 仅负责持久化，业务逻辑（序列化、校验 sampleType）由 service 层完成
type HumanizeLowQualitySampleCollector struct{}

// NewHumanizeLowQualitySampleCollector 构造
func NewHumanizeLowQualitySampleCollector() *HumanizeLowQualitySampleCollector {
	return &HumanizeLowQualitySampleCollector{}
}

// Collect 持久化低质样本到 low_quality_samples 表
// sample 由 service 层构建（含序列化后的 DimensionScores / CandidateReplies）
func (c *HumanizeLowQualitySampleCollector) Collect(
	ctx context.Context,
	sample *model.LowQualitySample,
) error {
	if sample == nil {
		return nil
	}
	if err := db.GetDB().WithContext(ctx).Create(sample).Error; err != nil {
		return fmt.Errorf("save low quality sample: %w", err)
	}
	return nil
}
