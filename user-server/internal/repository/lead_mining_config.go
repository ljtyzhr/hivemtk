package repository

import (
	"context"
	"errors"
	"time"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LeadMiningConfigRepository 线索发掘配置仓储（单例行）
type LeadMiningConfigRepository interface {
	GetSingleton(ctx context.Context) (*model.LeadMiningConfig, error)
	Save(ctx context.Context, cfg *model.LeadMiningConfig) error
}

type leadMiningConfigRepo struct {
	db *gorm.DB
}

func NewLeadMiningConfigRepository() LeadMiningConfigRepository {
	return &leadMiningConfigRepo{db: _db.GetDB()}
}

// singletonID 单例行主键：取 id = tenant_id。id 是全局主键，写死 1 会让
// 第二个租户保存时撞主键 500；取 tenant_id 既保持各租户一行、又不必新增唯一索引，
// 且单租户（off 档）仍是 id=1，与历史行兼容。
func (r *leadMiningConfigRepo) singletonID(ctx context.Context) int64 {
	if id, ok := tenant.FromContext(ctx); ok && id > 0 {
		return int64(id)
	}
	return 1
}

func (r *leadMiningConfigRepo) GetSingleton(ctx context.Context) (*model.LeadMiningConfig, error) {
	id := r.singletonID(ctx)
	var c model.LeadMiningConfig
	err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &model.LeadMiningConfig{ID: id, MinIntentScore: 50}, nil
		}
		return nil, err
	}
	return &c, nil
}

// Save 落单例配置。用 ON CONFLICT (id) DO UPDATE 一条语句完成，而不是
// 「先 First 再 Create/Updates」：并发保存时预检通过后落的那条会撞主键 500，
// 且 Updates 分支命中 0 行只体现在 RowsAffected 上，调用方以为存进去了。
func (r *leadMiningConfigRepo) Save(ctx context.Context, cfg *model.LeadMiningConfig) error {
	if cfg == nil {
		return errors.New("配置为空")
	}
	cfg.ID = r.singletonID(ctx)
	cfg.UpdatedAt = time.Now().Unix()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled", "keywords", "tags", "requirement",
			"channels", "min_intent_score", "model", "updated_at",
		}),
	}).Create(cfg).Error
}
