package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ChurnPredictionRepository 流失预测仓库
type ChurnPredictionRepository struct {
	db *gorm.DB
}

// NewChurnPredictionRepository 创建流失预测仓库实例
func NewChurnPredictionRepository() *ChurnPredictionRepository {
	return &ChurnPredictionRepository{
		db: _db.GetDB(),
	}
}

// Upsert 创建或更新流失预测
//
// 不用 Save：Save 会把整个结构体写回，包括调用方没填的 tenant_id（=0），
// 那是「更新不可改写 tenant_id」闸门的直接触发点（on 模式整条保存 500），
// 而且行会带着 tenant_id=0 落库 —— 那不属于任何租户，任何租户都读不到它
// （离线单机档下 tenant_id=1 的行也读不到，等于这批预测对所有人隐身）。
// tenant_id 由隔离 callback 在 INSERT 时填、在 UPDATE 时不允许改；
// updated_at 交给 GORM 的 autoUpdateTime。
func (r *ChurnPredictionRepository) Upsert(ctx context.Context, prediction *model.ChurnPrediction) error {
	var existing model.ChurnPrediction
	err := r.db.WithContext(ctx).Where("user_id = ?", prediction.UserID).First(&existing).Error
	if err == nil {
		prediction.ID = existing.ID
		res := r.db.WithContext(ctx).Model(&model.ChurnPrediction{}).
			Where("id = ?", existing.ID).
			Select("user_id", "churn_score", "churn_risk", "risk_factors",
				"last_activity_at", "last_purchase_at", "days_since_active",
				"purchase_freq", "average_order_value").
			Updates(prediction)
		if res.Error != nil {
			return res.Error
		}
		// 把内存里的租户归属同步成这一行真实的归属：调用方传进来的结构体
		// tenant_id 是零值（它没义务知道行归属），但把零值留在结构体里，
		// 下游若再拿它去写别的表，就会被「写入 tenant_id 与上下文租户不一致」
		// 那道闸拒掉 —— 而那不是调用方的错，是这里没把事实回填。
		prediction.TenantID = existing.TenantID
		return nil
	}
	return r.db.WithContext(ctx).Create(prediction).Error
}

// GetByID 根据 ID 获取流失预测
func (r *ChurnPredictionRepository) GetByID(ctx context.Context, id uint) (*model.ChurnPrediction, error) {
	var prediction model.ChurnPrediction
	err := r.db.WithContext(ctx).First(&prediction, id).Error
	return &prediction, err
}

// GetAll 获取所有流失预测列表
func (r *ChurnPredictionRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.ChurnPrediction, int64, error) {
	var predictions []*model.ChurnPrediction
	var total int64

	q := r.db.WithContext(ctx).Model(&model.ChurnPrediction{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.
		Order("churn_score DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&predictions).Error

	return predictions, total, err
}

// GetByRisk 根据风险等级获取流失预测
func (r *ChurnPredictionRepository) GetByRisk(ctx context.Context, riskLevel string, page, pageSize int) ([]*model.ChurnPrediction, int64, error) {
	var predictions []*model.ChurnPrediction
	var total int64

	q := r.db.WithContext(ctx).Model(&model.ChurnPrediction{}).Where("churn_risk = ?", riskLevel)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("churn_score DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&predictions).Error

	return predictions, total, err
}

// GetByUserID 根据用户 ID 获取流失预测
func (r *ChurnPredictionRepository) GetByUserID(ctx context.Context, userID string) (*model.ChurnPrediction, error) {
	var prediction model.ChurnPrediction
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&prediction).Error
	return &prediction, err
}

// GetHighRiskUsers 获取高风险用户列表
func (r *ChurnPredictionRepository) GetHighRiskUsers(ctx context.Context, limit int) ([]*model.ChurnPrediction, error) {
	var predictions []*model.ChurnPrediction
	err := r.db.WithContext(ctx).Where("churn_risk IN ?", []string{"high", "critical"}).
		Order("churn_score DESC").
		Limit(limit).
		Find(&predictions).Error
	return predictions, err
}

// ChurnWarningRepository 流失预警仓库
type ChurnWarningRepository struct {
	db *gorm.DB
}

// NewChurnWarningRepository 创建流失预警仓库实例
func NewChurnWarningRepository() *ChurnWarningRepository {
	return &ChurnWarningRepository{
		db: _db.GetDB(),
	}
}

// Create 创建流失预警
func (r *ChurnWarningRepository) Create(ctx context.Context, warning *model.ChurnWarning) error {
	return r.db.WithContext(ctx).Create(warning).Error
}

// GetByID 根据 ID 获取流失预警
func (r *ChurnWarningRepository) GetByID(ctx context.Context, id uint) (*model.ChurnWarning, error) {
	var warning model.ChurnWarning
	err := r.db.WithContext(ctx).First(&warning, id).Error
	return &warning, err
}

// GetAll 获取所有流失预警列表
func (r *ChurnWarningRepository) GetAll(ctx context.Context, page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	var warnings []*model.ChurnWarning
	var total int64

	q := r.db.WithContext(ctx).Model(&model.ChurnWarning{})
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&warnings).Error

	return warnings, total, err
}

// GetUnhandled 获取未处理的流失预警
func (r *ChurnWarningRepository) GetUnhandled(ctx context.Context, page, pageSize int) ([]*model.ChurnWarning, int64, error) {
	var warnings []*model.ChurnWarning
	var total int64

	q := r.db.WithContext(ctx).Model(&model.ChurnWarning{}).Where("is_handled = ?", false)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&warnings).Error

	return warnings, total, err
}

// Update 更新流失预警
func (r *ChurnWarningRepository) Update(ctx context.Context, warning *model.ChurnWarning) error {
	return r.db.WithContext(ctx).Save(warning).Error
}

// MarkHandled 标记为已处理
func (r *ChurnWarningRepository) MarkHandled(ctx context.Context, id uint, handledBy uint, note string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.ChurnWarning{}).Where("id = ?", id).Updates(map[string]any{
		"is_handled":   true,
		"handled_at":   now,
		"handled_by":   handledBy,
		"handled_note": note,
		"updated_at":   now,
	}).Error
}

// ChurnModelConfigRepository 流失模型配置仓库
type ChurnModelConfigRepository struct {
	db *gorm.DB
}

// NewChurnModelConfigRepository 创建流失模型配置仓库实例
func NewChurnModelConfigRepository() *ChurnModelConfigRepository {
	return &ChurnModelConfigRepository{
		db: _db.GetDB(),
	}
}

// GetCurrent 获取当前模型配置
func (r *ChurnModelConfigRepository) GetCurrent(ctx context.Context) (*model.ChurnModelConfig, error) {
	var config model.ChurnModelConfig
	err := r.db.WithContext(ctx).First(&config).Error
	return &config, err
}

// Upsert 创建或更新模型配置
func (r *ChurnModelConfigRepository) Upsert(ctx context.Context, config *model.ChurnModelConfig) error {
	var existing model.ChurnModelConfig
	err := r.db.WithContext(ctx).Where("id > ?", 0).First(&existing).Error
	if err == nil {
		config.ID = existing.ID
		// 不用 Save：Save 会把整个结构体写回，包括请求体里没带的 tenant_id（=0），
		// 那是「更新不可改写 tenant_id」闸门的直接触发点（on 模式整条保存 500）。
		// tenant_id 由隔离 callback 在 INSERT 时填、在 UPDATE 时不允许改。
		res := r.db.WithContext(ctx).Model(&model.ChurnModelConfig{}).
			Where("id = ?", existing.ID).
			Select("inactive_days_weight", "purchase_freq_weight", "order_value_weight",
				"engagement_weight", "inactive_threshold", "purchase_threshold",
				"high_risk_score", "critical_risk_score").
			Updates(config)
		return res.Error
	}
	return r.db.WithContext(ctx).Create(config).Error
}

// UpdateCalcTime 更新最后计算时间
func (r *ChurnModelConfigRepository) UpdateCalcTime(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.ChurnModelConfig{}).
		Where("id > ?", 0).
		Update("last_calculated_at", time.Now()).Error
}

// ChurnStatisticsRepository 流失统计仓库
type ChurnStatisticsRepository struct {
	db *gorm.DB
}

// NewChurnStatisticsRepository 创建流失统计仓库实例
func NewChurnStatisticsRepository() *ChurnStatisticsRepository {
	return &ChurnStatisticsRepository{
		db: _db.GetDB(),
	}
}

// Create 创建流失统计
func (r *ChurnStatisticsRepository) Create(ctx context.Context, stats *model.ChurnStatistics) error {
	return r.db.WithContext(ctx).Create(stats).Error
}

// GetAll 获取所有流失统计
func (r *ChurnStatisticsRepository) GetAll(ctx context.Context, startDate, endDate string) ([]*model.ChurnStatistics, error) {
	var stats []*model.ChurnStatistics
	err := r.db.WithContext(ctx).Where("stat_date BETWEEN ? AND ?", startDate, endDate).
		Order("stat_date DESC").
		Find(&stats).Error
	return stats, err
}

// GetLatest 获取最新的流失统计
func (r *ChurnStatisticsRepository) GetLatest(ctx context.Context) (*model.ChurnStatistics, error) {
	var stats model.ChurnStatistics
	err := r.db.WithContext(ctx).
		Order("stat_date DESC").
		First(&stats).Error
	return &stats, err
}

// NewChurnPredictionRepositoryWithDB 创建指定数据库连接的流失预测仓库实例（用于测试）
func NewChurnPredictionRepositoryWithDB(db *gorm.DB) *ChurnPredictionRepository {
	return &ChurnPredictionRepository{db: db}
}

// NewChurnWarningRepositoryWithDB 创建指定数据库连接的流失预警仓库实例（用于测试）
func NewChurnWarningRepositoryWithDB(db *gorm.DB) *ChurnWarningRepository {
	return &ChurnWarningRepository{db: db}
}

// NewChurnModelConfigRepositoryWithDB 创建指定数据库连接的流失模型配置仓库实例（用于测试）
func NewChurnModelConfigRepositoryWithDB(db *gorm.DB) *ChurnModelConfigRepository {
	return &ChurnModelConfigRepository{db: db}
}

// NewChurnStatisticsRepositoryWithDB 创建指定数据库连接的流失统计仓库实例（用于测试）
func NewChurnStatisticsRepositoryWithDB(db *gorm.DB) *ChurnStatisticsRepository {
	return &ChurnStatisticsRepository{db: db}
}
