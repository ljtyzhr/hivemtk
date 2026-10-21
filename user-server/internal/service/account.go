package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// AccountService 账号读取服务（依赖由装配层注入，不在服务内自建仓储）。
type AccountService struct {
	repo repository.AccountRepository
}

func NewAccountService(repo repository.AccountRepository) *AccountService {
	return &AccountService{repo: repo}
}

func (s *AccountService) GetAccount(ctx context.Context, id string) (*model.Account, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AccountService) GetAccountList(ctx context.Context) ([]*model.Account, error) {
	return s.repo.GetAccountList(ctx)
}
