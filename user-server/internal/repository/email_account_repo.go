package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// EmailAccountRepository 邮件账号查询/配额计数仓储
type EmailAccountRepository struct {
	db *gorm.DB
}

func NewEmailAccountRepository() *EmailAccountRepository {
	return NewEmailAccountRepositoryWithDB(_db.GetDB())
}

func NewEmailAccountRepositoryWithDB(db *gorm.DB) *EmailAccountRepository {
	return &EmailAccountRepository{db: db}
}

func (r *EmailAccountRepository) GetByID(ctx context.Context, id uint) (*model.EmailAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var acc model.EmailAccount
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *EmailAccountRepository) ListActiveBelowQuota(ctx context.Context) ([]model.EmailAccount, error) {
	if r == nil || r.db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var candidates []model.EmailAccount
	err := r.db.WithContext(ctx).
		Where("status = ? AND daily_used < daily_quota", "active").
		Order("daily_used ASC").
		Find(&candidates).Error
	return candidates, err
}

// IncDailyUsed 配额计数自增（发信成功后调用；失败不阻断发信，由调用方留痕）
func (r *EmailAccountRepository) IncDailyUsed(ctx context.Context, id uint) error {
	if r == nil || r.db == nil {
		return gorm.ErrInvalidDB
	}
	return r.db.WithContext(ctx).Model(&model.EmailAccount{}).
		Where("id = ?", id).
		UpdateColumn("daily_used", gorm.Expr("daily_used + 1")).Error
}
