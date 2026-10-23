// email_account_repo.go 邮件账号仓储（五层 L5）
package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// 表结构由 model.EmailAccount 描述（此前这里另有一份 EmailAccountRow：同一张表两份结构体，
// repo 那份缺 tenant_id 与时间戳，与建表结构漂移且不报错）。

// EmailAccountRepository 邮件账号查询/配额计数仓储
type EmailAccountRepository struct {
	db *gorm.DB
}

// NewEmailAccountRepository 构造
// NewEmailAccountRepository 构造（无参，内部取全局 DB；对齐 customer_session.go 先例）
func NewEmailAccountRepository() *EmailAccountRepository {
	return NewEmailAccountRepositoryWithDB(_db.GetDB())
}

// NewEmailAccountRepositoryWithDB 显式注入 DB（测试/装配用）
func NewEmailAccountRepositoryWithDB(db *gorm.DB) *EmailAccountRepository {
	return &EmailAccountRepository{db: db}
}

// GetByID 按 ID 取账号；不存在返回 (nil, err)
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

// ListActiveBelowQuota 按 daily_used 升序列出有余量的 active 账号
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
