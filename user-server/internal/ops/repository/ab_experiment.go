package repository

import (
	"context"

	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ABExperimentRepository A/B 实验仓库
type ABExperimentRepository struct {
	db *gorm.DB
}

// NewABExperimentRepository 创建 A/B 实验仓库实例
func NewABExperimentRepository() *ABExperimentRepository {
	return &ABExperimentRepository{
		db: _db.GetDB(),
	}
}

// Create 创建实验
func (r *ABExperimentRepository) Create(ctx context.Context, experiment *model.ABExperiment) error {
	return r.db.WithContext(ctx).Create(experiment).Error
}

// GetByID 根据 ID 获取实验
func (r *ABExperimentRepository) GetByID(ctx context.Context, id uint) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.WithContext(ctx).First(&experiment, id).Error
	return &experiment, err
}

// GetAll 获取所有实验列表
func (r *ABExperimentRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.ABExperiment, int64, error) {
	var experiments []*model.ABExperiment
	var total int64

	r.db.WithContext(ctx).Model(&model.ABExperiment{}).Count(&total)
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&experiments).Error

	return experiments, total, err
}

// Update 更新实验
func (r *ABExperimentRepository) Update(ctx context.Context, experiment *model.ABExperiment) error {
	return r.db.WithContext(ctx).Save(experiment).Error
}

// Delete 删除实验
func (r *ABExperimentRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.ABExperiment{}, id).Error
}

// UpdateStatus 更新实验状态
func (r *ABExperimentRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	return r.db.WithContext(ctx).Model(&model.ABExperiment{}).Where("id = ?", id).Update("status", status).Error
}

// GetRunningBySourceID 根据 sourceID 获取运行中的实验
func (r *ABExperimentRepository) GetRunningBySourceID(ctx context.Context, sourceID string) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.WithContext(ctx).Where("source_id = ? AND status = ?", sourceID, "running").First(&experiment).Error
	return &experiment, err
}

// GetLatestBySourceID 根据 sourceID 获取最新实验（不限状态，按 updated_at DESC）
func (r *ABExperimentRepository) GetLatestBySourceID(ctx context.Context, sourceID string) (*model.ABExperiment, error) {
	var experiment model.ABExperiment
	err := r.db.WithContext(ctx).Where("source_id = ?", sourceID).
		Order("updated_at DESC").
		First(&experiment).Error
	return &experiment, err
}

// ABVariantRepository A/B 变体仓库
type ABVariantRepository struct {
	db *gorm.DB
}

// NewABVariantRepository 创建 A/B 变体仓库实例
func NewABVariantRepository() *ABVariantRepository {
	return &ABVariantRepository{
		db: _db.GetDB(),
	}
}

// Create 创建变体
func (r *ABVariantRepository) Create(ctx context.Context, variant *model.ABVariant) error {
	return r.db.WithContext(ctx).Create(variant).Error
}

// GetByID 根据 ID 获取变体
func (r *ABVariantRepository) GetByID(ctx context.Context, id uint) (*model.ABVariant, error) {
	var variant model.ABVariant
	err := r.db.WithContext(ctx).First(&variant, id).Error
	return &variant, err
}

// GetByExperiment 获取实验下的变体列表
func (r *ABVariantRepository) GetByExperiment(ctx context.Context, experimentID uint) ([]*model.ABVariant, error) {
	var variants []*model.ABVariant
	err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).
		Order("id ASC").Find(&variants).Error
	return variants, err
}

// Update 更新变体
func (r *ABVariantRepository) Update(ctx context.Context, variant *model.ABVariant) error {
	return r.db.WithContext(ctx).Save(variant).Error
}

// Delete 删除变体
func (r *ABVariantRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.ABVariant{}, id).Error
}

// DeleteByExperiment 删除实验下所有变体
func (r *ABVariantRepository) DeleteByExperiment(ctx context.Context, experimentID uint) error {
	return r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).Delete(&model.ABVariant{}).Error
}

// IncrementTraffic 增加访问计数
func (r *ABVariantRepository) IncrementTraffic(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.ABVariant{}).Where("id = ?", id).
		UpdateColumn("traffic_count", gorm.Expr("traffic_count + 1")).Error
}

// IncrementConversion 增加转化计数
func (r *ABVariantRepository) IncrementConversion(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.ABVariant{}).Where("id = ?", id).
		UpdateColumn("conversion_count", gorm.Expr("conversion_count + 1")).Error
}

// ABConversionEventRepository 转化事件仓库
type ABConversionEventRepository struct {
	db *gorm.DB
}

// NewABConversionEventRepository 创建转化事件仓库实例
func NewABConversionEventRepository() *ABConversionEventRepository {
	return &ABConversionEventRepository{
		db: _db.GetDB(),
	}
}

// Create 创建转化事件
func (r *ABConversionEventRepository) Create(ctx context.Context, event *model.ABConversionEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// GetByExperiment 获取实验下的转化事件列表
func (r *ABConversionEventRepository) GetByExperiment(ctx context.Context, experimentID uint, page, pageSize int) ([]*model.ABConversionEvent, int64, error) {
	var events []*model.ABConversionEvent
	var total int64

	r.db.WithContext(ctx).Model(&model.ABConversionEvent{}).Where("experiment_id = ?", experimentID).Count(&total)
	err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&events).Error

	return events, total, err
}

// GetByVariant 获取变体下的转化事件
func (r *ABConversionEventRepository) GetByVariant(ctx context.Context, variantID uint) ([]*model.ABConversionEvent, error) {
	var events []*model.ABConversionEvent
	err := r.db.WithContext(ctx).Where("variant_id = ?", variantID).Find(&events).Error
	return events, err
}

// ABExperimentResultRepository 实验结果仓库
type ABExperimentResultRepository struct {
	db *gorm.DB
}

// NewABExperimentResultRepository 创建实验结果仓库实例
func NewABExperimentResultRepository() *ABExperimentResultRepository {
	return &ABExperimentResultRepository{
		db: _db.GetDB(),
	}
}

// Upsert 创建或更新结果
func (r *ABExperimentResultRepository) Upsert(ctx context.Context, result *model.ABExperimentResult) error {
	var existing model.ABExperimentResult
	err := r.db.WithContext(ctx).Where("experiment_id = ? AND variant_id = ?", result.ExperimentID, result.VariantID).First(&existing).Error
	if err == nil {
		result.ID = existing.ID
		// Save 会把 Selects 展开为 "*"，tenant_id 与 created_at 必然进 SET；
		// 租户归属与创建时间只能沿用库中已有行，不能由调用方传入的空值覆盖。
		result.TenantID = existing.TenantID
		return r.db.WithContext(ctx).Save(result).Error
	}
	return r.db.WithContext(ctx).Create(result).Error
}

// GetByExperiment 获取实验结果
func (r *ABExperimentResultRepository) GetByExperiment(ctx context.Context, experimentID uint) ([]*model.ABExperimentResult, error) {
	var results []*model.ABExperimentResult
	err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).Find(&results).Error
	return results, err
}

// UpdateWinner 更新获胜者
func (r *ABExperimentResultRepository) UpdateWinner(ctx context.Context, experimentID uint, winnerVariantID uint) error {
	r.db.WithContext(ctx).Model(&model.ABExperimentResult{}).Where("experiment_id = ?", experimentID).
		Update("is_winner", false)

	return r.db.WithContext(ctx).Model(&model.ABExperimentResult{}).Where("experiment_id = ? AND variant_id = ?", experimentID, winnerVariantID).
		Update("is_winner", true).Error
}
