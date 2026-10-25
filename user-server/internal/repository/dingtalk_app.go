package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type DingTalkAppRepository interface {
	Create(ctx context.Context, acc *model.DingTalkAppAccount) error
	FindByID(ctx context.Context, id uint) (*model.DingTalkAppAccount, error)
	Update(ctx context.Context, acc *model.DingTalkAppAccount) error
	ListAll(ctx context.Context) ([]model.DingTalkAppAccount, error)
	DeleteByID(ctx context.Context, id uint) error
}

type dingTalkAppRepo struct {
	db *gorm.DB
}

// NewDingTalkAppRepositoryDefault 默认构造（全局句柄）。
func NewDingTalkAppRepositoryDefault() DingTalkAppRepository {
	return NewDingTalkAppRepository(GetDB())
}

func NewDingTalkAppRepository(db *gorm.DB) DingTalkAppRepository {
	return &dingTalkAppRepo{db: db}
}

func (r *dingTalkAppRepo) Create(ctx context.Context, acc *model.DingTalkAppAccount) error {
	return r.db.WithContext(ctx).Create(acc).Error
}

func (r *dingTalkAppRepo) FindByID(ctx context.Context, id uint) (*model.DingTalkAppAccount, error) {
	var acc model.DingTalkAppAccount
	if err := r.db.WithContext(ctx).First(&acc, id).Error; err != nil {
		return nil, err
	}
	return &acc, nil
}

func (r *dingTalkAppRepo) Update(ctx context.Context, acc *model.DingTalkAppAccount) error {
	return r.db.WithContext(ctx).Model(&model.DingTalkAppAccount{}).Where("id = ?", acc.ID).
		Updates(map[string]interface{}{
			"account_name":    acc.AccountName,
			"app_key":         acc.AppKey,
			"app_secret":      acc.AppSecret,
			"agent_id":        acc.AgentID,
			"token":           acc.Token,
			"aes_key":         acc.AESKey,
			"inbound_enabled": acc.InboundEnabled,
			"ai_agent_id":     acc.AIAgentID,
			"status":          acc.Status,
		}).Error
}

func (r *dingTalkAppRepo) ListAll(ctx context.Context) ([]model.DingTalkAppAccount, error) {
	var list []model.DingTalkAppAccount
	// 应用账号是租户级配置（个位数），但接口无分页参数，仍给一个上限挡异常数据量。
	if err := r.db.WithContext(ctx).Order("id desc").Limit(200).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *dingTalkAppRepo) DeleteByID(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.DingTalkAppAccount{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		// 命中 0 行 = 记录不存在或不属于本租户，不回"删除成功"。
		return gorm.ErrRecordNotFound
	}
	return nil
}
