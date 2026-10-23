package repository

import (
	"context"
	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type EmailSmtpRepository interface {
	Create(ctx context.Context, emailSmtp *model.EmailSmtp) error
	GetByID(ctx context.Context, id string) (*model.EmailSmtp, error)
	GetEmailSmtpList(ctx context.Context) ([]*model.EmailSmtp, error)
	Update(ctx context.Context, emailSmtp *model.EmailSmtp) error
	Delete(ctx context.Context, id string) error
}

type emailSmtpRepo struct {
	db *gorm.DB
}

func NewEmailSmtpRepository() EmailSmtpRepository {
	return &emailSmtpRepo{db: _db.GetDB()}
}

func (r *emailSmtpRepo) Create(ctx context.Context, emailSmtp *model.EmailSmtp) error {
	return r.db.WithContext(ctx).Create(emailSmtp).Error
}

func (r *emailSmtpRepo) Update(ctx context.Context, emailSmtp *model.EmailSmtp) error {
	res := r.db.WithContext(ctx).Save(emailSmtp)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 整行 Save 命中 0 行不报错：记录已删、或 id 属于别的租户时，
		// 上层会把"什么都没改"当成保存成功。这里显式翻成不存在。
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *emailSmtpRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.EmailSmtp{}).Error
}

func (r *emailSmtpRepo) GetByID(ctx context.Context, id string) (*model.EmailSmtp, error) {
	var emailSmtp model.EmailSmtp
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&emailSmtp).Error
	return &emailSmtp, err
}

func (r *emailSmtpRepo) GetEmailSmtpList(ctx context.Context) ([]*model.EmailSmtp, error) {
	var emailSmtp []*model.EmailSmtp
	err := r.db.WithContext(ctx).Find(&emailSmtp).Error
	return emailSmtp, err
}
