package repository

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

type HumanizeLowQualitySampleCollector struct{}

func NewHumanizeLowQualitySampleCollector() *HumanizeLowQualitySampleCollector {
	return &HumanizeLowQualitySampleCollector{}
}

// Collect 持久化低质样本（sample 由 service 层构建，含序列化字段）
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
