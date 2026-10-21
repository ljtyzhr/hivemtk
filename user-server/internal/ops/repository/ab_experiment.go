package repository

import (
	"context"
	"errors"

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

// NewABExperimentRepositoryWithDB 创建指定数据库连接的实验仓库实例（用于测试）
func NewABExperimentRepositoryWithDB(db *gorm.DB) *ABExperimentRepository {
	return &ABExperimentRepository{db: db}
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

	if err := r.db.WithContext(ctx).Model(&model.ABExperiment{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
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
	res := r.db.WithContext(ctx).Delete(&model.ABExperiment{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateStatus 更新实验状态
func (r *ABExperimentRepository) UpdateStatus(ctx context.Context, id uint, status string) error {
	res := r.db.WithContext(ctx).Model(&model.ABExperiment{}).Where("id = ?", id).Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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

// NewABVariantRepositoryWithDB 创建指定数据库连接的变体仓库实例（用于测试）
func NewABVariantRepositoryWithDB(db *gorm.DB) *ABVariantRepository {
	return &ABVariantRepository{db: db}
}

// Create 创建变体
func (r *ABVariantRepository) Create(ctx context.Context, variant *model.ABVariant) error {
	return r.db.WithContext(ctx).Create(variant).Error
}

// GetByExperiment 获取实验下的变体列表
func (r *ABVariantRepository) GetByExperiment(ctx context.Context, experimentID uint) ([]*model.ABVariant, error) {
	var variants []*model.ABVariant
	err := r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).
		Order("id ASC").Find(&variants).Error
	return variants, err
}

// DeleteByExperiment 删除实验下所有变体
func (r *ABVariantRepository) DeleteByExperiment(ctx context.Context, experimentID uint) error {
	return r.db.WithContext(ctx).Where("experiment_id = ?", experimentID).Delete(&model.ABVariant{}).Error
}

// ABConversionEventRepository 转化事件仓库
//
// 只保留读取：应用内没有写入路径，写入方法一旦存在就是不可达代码。
type ABConversionEventRepository struct {
	db *gorm.DB
}

// NewABConversionEventRepository 创建转化事件仓库实例
func NewABConversionEventRepository() *ABConversionEventRepository {
	return &ABConversionEventRepository{
		db: _db.GetDB(),
	}
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

// NewABExperimentResultRepositoryWithDB 创建指定数据库连接的实验结果仓库实例（用于测试）
func NewABExperimentResultRepositoryWithDB(db *gorm.DB) *ABExperimentResultRepository {
	return &ABExperimentResultRepository{db: db}
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
		result.CreatedAt = existing.CreatedAt
		return r.db.WithContext(ctx).Save(result).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
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
