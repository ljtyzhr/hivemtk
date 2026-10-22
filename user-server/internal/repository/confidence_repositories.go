package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
)

// ConfidenceSignalRepository 5 维信号快照读写
type ConfidenceSignalRepository struct {
	db *gorm.DB
}

// NewConfidenceSignalRepositoryWithDB 用装配点注入的连接创建信号快照仓库
func NewConfidenceSignalRepositoryWithDB(db *gorm.DB) *ConfidenceSignalRepository {
	return &ConfidenceSignalRepository{db: db}
}

// Create 创建信号快照
func (r *ConfidenceSignalRepository) Create(ctx context.Context, s *model.ConfidenceSignal) error {
	return r.db.WithContext(ctx).Create(s).Error
}

// List 分页查询置信度信号（管理面）
func (r *ConfidenceSignalRepository) List(ctx context.Context, sessionID string, page, pageSize int) ([]model.ConfidenceSignal, int64, error) {
	page, pageSize = normalizeConfidencePaging(page, pageSize)
	applyFilter := func(tx *gorm.DB) *gorm.DB {
		if sessionID != "" {
			tx = tx.Where("session_id = ?", sessionID)
		}
		return tx
	}

	var total int64
	if err := applyFilter(r.db.WithContext(ctx).Model(&model.ConfidenceSignal{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []model.ConfidenceSignal
	if err := applyFilter(r.db.WithContext(ctx).Model(&model.ConfidenceSignal{})).
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetByID 按主键查询置信度信号（管理面详情）
func (r *ConfidenceSignalRepository) GetByID(ctx context.Context, id string) (*model.ConfidenceSignal, error) {
	var s model.ConfidenceSignal
	if err := r.db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// ConfidenceBandStat 置信度决策带聚合结果
type ConfidenceBandStat struct {
	DecisionBand string `json:"decision_band"`
	Count        int64  `json:"count"`
}

// StatsByBand 按决策带聚合统计（管理面）
func (r *ConfidenceSignalRepository) StatsByBand(ctx context.Context, since time.Time) ([]ConfidenceBandStat, error) {
	var rows []ConfidenceBandStat
	if err := r.db.WithContext(ctx).
		Model(&model.ConfidenceSignal{}).
		Select("decision_band, COUNT(*) as count").
		Where("created_at > ?", since).
		Group("decision_band").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ConfidenceCalibrationRepository 校准参数历史
type ConfidenceCalibrationRepository struct {
	db *gorm.DB
}

// NewConfidenceCalibrationRepositoryWithDB 用装配点注入的连接创建校准参数仓库
func NewConfidenceCalibrationRepositoryWithDB(db *gorm.DB) *ConfidenceCalibrationRepository {
	return &ConfidenceCalibrationRepository{db: db}
}

// SaveActive 保存校准结果并标记为 active（同 signal_type 其他记录置 inactive）
func (r *ConfidenceCalibrationRepository) SaveActive(ctx context.Context, c *model.ConfidenceCalibration) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ConfidenceCalibration{}).
			Where("signal_type = ? AND is_active = true", c.SignalType).
			Update("is_active", false).Error; err != nil {
			return err
		}
		c.IsActive = true
		return tx.Create(c).Error
	})
}

// GetActive 获取指定 signal_type 的当前 active 记录
func (r *ConfidenceCalibrationRepository) GetActive(ctx context.Context, signalType string) (*model.ConfidenceCalibration, error) {
	var c model.ConfidenceCalibration
	err := r.db.WithContext(ctx).
		Where("signal_type = ? AND is_active = true", signalType).
		Order("created_at DESC, id DESC").
		First(&c).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List 分页查询置信度校准记录（管理面）
func (r *ConfidenceCalibrationRepository) List(ctx context.Context, signalType string, page, pageSize int) ([]model.ConfidenceCalibration, int64, error) {
	page, pageSize = normalizeConfidencePaging(page, pageSize)
	applyFilter := func(tx *gorm.DB) *gorm.DB {
		if signalType != "" {
			tx = tx.Where("signal_type = ?", signalType)
		}
		return tx
	}

	var total int64
	if err := applyFilter(r.db.WithContext(ctx).Model(&model.ConfidenceCalibration{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var list []model.ConfidenceCalibration
	if err := applyFilter(r.db.WithContext(ctx).Model(&model.ConfidenceCalibration{})).
		Order("created_at DESC, id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// ThresholdPolicyRepository 动态阈值策略
type ThresholdPolicyRepository struct {
	db *gorm.DB
}

// NewThresholdPolicyRepositoryWithDB 用装配点注入的连接创建阈值策略仓库
func NewThresholdPolicyRepositoryWithDB(db *gorm.DB) *ThresholdPolicyRepository {
	return &ThresholdPolicyRepository{db: db}
}

// SaveActive 保存策略并置为唯一 active：同意图的旧版本先下线，整个过程同事务。
// ID 为 0 时插入，否则按主键更新；更新命中 0 行报 ErrRecordNotFound（不走 Save 的补插兜底）。
func (r *ThresholdPolicyRepository) SaveActive(ctx context.Context, p *model.ThresholdPolicy) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ThresholdPolicy{}).
			Where("intent_type = ? AND is_active = true", p.IntentType).
			Update("is_active", false).Error; err != nil {
			return err
		}
		p.IsActive = true
		if p.ID == 0 {
			return tx.Create(p).Error
		}
		res := tx.Model(&model.ThresholdPolicy{}).
			Where("id = ?", p.ID).
			Updates(map[string]any{
				"intent_type":               p.IntentType,
				"base_threshold":            p.BaseThreshold,
				"customer_level_weight":     p.CustomerLevelWeight,
				"timeslot_weight":           p.TimeslotWeight,
				"agent_availability_weight": p.AgentAvailabilityWeight,
				"band_handoff_upper":        p.BandHandoffUpper,
				"band_fallback_upper":       p.BandFallbackUpper,
				"band_review_upper":         p.BandReviewUpper,
				"review_sla_seconds":        p.ReviewSLASeconds,
				"is_active":                 true,
				"version":                   p.Version,
				"updated_at":                p.UpdatedAt,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// GetByPolicyID 按业务主键查询策略（不区分 active，供 Upsert 判存在）
func (r *ThresholdPolicyRepository) GetByPolicyID(ctx context.Context, policyID string) (*model.ThresholdPolicy, error) {
	var p model.ThresholdPolicy
	if err := r.db.WithContext(ctx).Where("policy_id = ?", policyID).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// ListActive 列出所有 active 策略
func (r *ThresholdPolicyRepository) ListActive(ctx context.Context) ([]model.ThresholdPolicy, error) {
	var list []model.ThresholdPolicy
	err := r.db.WithContext(ctx).
		Where("is_active = true").
		Order("intent_type ASC, version DESC").
		Find(&list).Error
	return list, err
}

// normalizeConfidencePaging 兜底分页：page<1 会算出负 Offset（PG 直接报错），
// pageSize<1 在 PG 里等于不设上限 —— 两者都必须归一。
func normalizeConfidencePaging(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}
