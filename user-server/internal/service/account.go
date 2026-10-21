package service

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

type AccountService struct {
	repo repository.AccountRepository
}

func NewAccountService() *AccountService {
	return &AccountService{repo: repository.NewAccountRepository()}
}

func (s *AccountService) GetAccount(ctx context.Context, id string) (*model.Account, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *AccountService) GetAccountList(ctx context.Context) ([]*model.Account, error) {
	return s.repo.GetAccountList(ctx)
}
