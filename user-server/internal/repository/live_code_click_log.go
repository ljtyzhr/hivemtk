package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

type LiveCodeClickLogRepository interface {
	CreateLiveCodeClick(ctx context.Context, log *model.LiveCodeClickLog) error
	CreateQRCodeClick(ctx context.Context, log *model.QRCodeClickLog) error
}

type liveCodeClickLogRepository struct {
	db *gorm.DB
}

func NewLiveCodeClickLogRepository(db *gorm.DB) LiveCodeClickLogRepository {
	return &liveCodeClickLogRepository{db: db}
}

func (r *liveCodeClickLogRepository) CreateLiveCodeClick(ctx context.Context, log *model.LiveCodeClickLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

func (r *liveCodeClickLogRepository) CreateQRCodeClick(ctx context.Context, log *model.QRCodeClickLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}
