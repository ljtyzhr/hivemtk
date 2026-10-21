package repository

import (
	"context"
	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type AccountRepository interface {
	GetByID(ctx context.Context, id string) (*model.Account, error)
	GetAccountList(ctx context.Context) ([]*model.Account, error)
}

type accountRepo struct {
	db *gorm.DB
}

func NewAccountRepository() AccountRepository {
	return &accountRepo{db: _db.GetDB()}
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
	err := r.db.WithContext(ctx).Find(&accounts).Error
	return accounts, err
}
