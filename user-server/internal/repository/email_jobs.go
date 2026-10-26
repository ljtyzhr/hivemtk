package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailJobsRepository interface {
	Create(ctx context.Context, jobs *model.EmailJobs) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.EmailJobs, error)
	List(ctx context.Context, page int, pageSize int) ([]*model.EmailJobs, int64, error)
	Update(ctx context.Context, jobs *model.EmailJobs) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type emailJobsRepo struct {
	db *gorm.DB
}

func NewEmailJobsRepository() EmailJobsRepository {
	return &emailJobsRepo{db: _db.GetDB()}
}

func (r *emailJobsRepo) Create(ctx context.Context, jobs *model.EmailJobs) error {
	return r.db.WithContext(ctx).Create(jobs).Error
}

func (r *emailJobsRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailJobs, error) {
	var jobs model.EmailJobs
	if err := r.db.WithContext(ctx).First(&jobs, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &jobs, nil
}

func (r *emailJobsRepo) List(ctx context.Context, page int, pageSize int) ([]*model.EmailJobs, int64, error) {
	var jobsLists []*model.EmailJobs
	var total int64
	db := r.db.WithContext(ctx)
	err := db.Model(&model.EmailJobs{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = db.Offset((page - 1) * pageSize).Limit(pageSize).Order("created_at DESC").Find(&jobsLists).Error
	if err != nil {
		return nil, 0, err
	}
	return jobsLists, total, err
}

func (r *emailJobsRepo) Update(ctx context.Context, jobs *model.EmailJobs) error {
	return saveWholeRow(r.db.WithContext(ctx), jobs)
}

func (r *emailJobsRepo) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&model.EmailJobs{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
