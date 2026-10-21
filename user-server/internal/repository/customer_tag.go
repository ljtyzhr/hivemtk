package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// CustomerTagRepository defines the interface for customer tag data access
type CustomerTagRepository interface {
	Create(ctx context.Context, tag *model.CustomerTag) error
	GetByID(ctx context.Context, id string) (*model.CustomerTag, error)
	ListByMerchant(ctx context.Context) ([]*model.CustomerTag, error)
	ListAutoTags(ctx context.Context) ([]*model.CustomerTag, error)
	Update(ctx context.Context, tag *model.CustomerTag) error
	Delete(ctx context.Context, id string) error
}

type customerTagRepository struct{}

// NewCustomerTagRepository creates a new CustomerTagRepository instance
func NewCustomerTagRepository() CustomerTagRepository {
	return &customerTagRepository{}
}

func (r *customerTagRepository) Create(ctx context.Context, tag *model.CustomerTag) error {
	return _db.GetDB().WithContext(ctx).Create(tag).Error
}

func (r *customerTagRepository) GetByID(ctx context.Context, id string) (*model.CustomerTag, error) {
	var tag model.CustomerTag
	if err := _db.GetDB().WithContext(ctx).First(&tag, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &tag, nil
}

func (r *customerTagRepository) ListByMerchant(ctx context.Context) ([]*model.CustomerTag, error) {
	var tags []*model.CustomerTag

	if err := _db.GetDB().WithContext(ctx).
		Order("category, name").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

func (r *customerTagRepository) ListAutoTags(ctx context.Context) ([]*model.CustomerTag, error) {
	var tags []*model.CustomerTag

	if err := _db.GetDB().WithContext(ctx).
		Where("source = ?", model.TagSourceAuto).
		Order("category, name").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

func (r *customerTagRepository) Delete(ctx context.Context, id string) error {
	return _db.GetDB().WithContext(ctx).Delete(&model.CustomerTag{}, "id = ?", id).Error
}

func (r *customerTagRepository) Update(ctx context.Context, tag *model.CustomerTag) error {
	return _db.GetDB().WithContext(ctx).Save(tag).Error
}
