package repository

import (
	"context"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// accountListLimit 账号列表返回上限：列表接口无分页参数，不封顶时会把整表读进内存。
const accountListLimit = 200

type AccountRepository interface {
	GetByID(ctx context.Context, id string) (*model.Account, error)
	GetAccountList(ctx context.Context) ([]*model.Account, error)
}

type accountRepo struct {
	db *gorm.DB
}

// NewAccountRepositoryWithDB 创建账号仓库实例（显式注入 db，兼容测试与五层架构调用方）
func NewAccountRepositoryWithDB(db *gorm.DB) AccountRepository {
	return &accountRepo{db: db}
}

func (r *accountRepo) GetByID(ctx context.Context, id string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).First(&account, "id = ?", id).Error
	return &account, err
}

func (r *accountRepo) GetAccountList(ctx context.Context) ([]*model.Account, error) {
	var accounts []*model.Account
	err := r.db.WithContext(ctx).Order("create_time DESC").Limit(accountListLimit).Find(&accounts).Error
	return accounts, err
}
