package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// UserMFARepository 用户 MFA 仓储接口（封装 user_mfa 表 CRUD，service 层不直接摸 DB）。
type UserMFARepository interface {
	GetByUserID(ctx context.Context, userID uint) (*model.UserMFA, error)
	Create(ctx context.Context, mfa *model.UserMFA) error
	Save(ctx context.Context, mfa *model.UserMFA) error
	UpdateBackupCodes(ctx context.Context, userID uint, backupCodesJSON string) error
	UpdateLastUsed(ctx context.Context, userID uint, code string, now interface{}) error
}

type userMFARepository struct {
	db *gorm.DB
}

// NewUserMFARepository 创建 UserMFA 仓储
func NewUserMFARepository() UserMFARepository {
	return &userMFARepository{db: _db.GetDB()}
}

// NewUserMFARepositoryWithDB 通过 *gorm.DB 创建 UserMFA 仓储（测试 / router 装配用）
func NewUserMFARepositoryWithDB(gormDB *gorm.DB) UserMFARepository {
	return &userMFARepository{db: gormDB}
}

func (r *userMFARepository) GetByUserID(ctx context.Context, userID uint) (*model.UserMFA, error) {
	var mfa model.UserMFA
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&mfa).Error; err != nil {
		return nil, err
	}
	return &mfa, nil
}

func (r *userMFARepository) Create(ctx context.Context, mfa *model.UserMFA) error {
	return r.db.WithContext(ctx).Create(mfa).Error
}

// Save 整行回写 MFA 配置。
//
// 走 db.SaveWholeRow：裸 Save 命中 0 行会回退成 INSERT。调用方都是
// GetByUserID 后改字段再 Save，若行在窗口期被删，裸 Save 会把
// 「已解绑用户」的 MFA 配置按原 id 插回来。
func (r *userMFARepository) Save(ctx context.Context, mfa *model.UserMFA) error {
	if mfa == nil || mfa.ID == 0 {
		return errors.New("user_mfa: 缺主键，不能整行回写")
	}
	return _db.SaveWholeRow(r.db.WithContext(ctx), mfa)
}

func (r *userMFARepository) UpdateBackupCodes(ctx context.Context, userID uint, backupCodesJSON string) error {
	res := r.db.WithContext(ctx).Model(&model.UserMFA{}).
		Where("user_id = ?", userID).
		Update("backup_codes", backupCodesJSON)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("user_mfa 记录不存在,无法更新 backup_codes")
	}
	return nil
}

func (r *userMFARepository) UpdateLastUsed(ctx context.Context, userID uint, code string, now interface{}) error {
	// 0 行 ⇒ ErrRecordNotFound：行在窗口期消失说明 MFA 已被并发解绑，
	// 审计层面不该静默吞掉。
	return _db.EnsureAffected(r.db.WithContext(ctx).Model(&model.UserMFA{}).
		Where("user_id = ?", userID).
		Updates(map[string]interface{}{
			"last_used_code": code,
			"last_used_at":   now,
		}))
}
