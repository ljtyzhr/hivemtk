// dashboard_template.go 数据大屏模板仓储（R39-K8）。
package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type DashboardTemplateRepository struct {
	db *gorm.DB
}

func NewDashboardTemplateRepository() *DashboardTemplateRepository {
	return &DashboardTemplateRepository{db: _db.GetDB()}
}

func (r *DashboardTemplateRepository) List(ctx context.Context, status string) ([]*model.DashboardTemplate, error) {
	q := r.db.WithContext(ctx).Model(&model.DashboardTemplate{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []*model.DashboardTemplate
	err := q.Order("id DESC").Find(&list).Error
	return list, err
}

func (r *DashboardTemplateRepository) GetByID(ctx context.Context, id uint) (*model.DashboardTemplate, error) {
	var tpl model.DashboardTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *DashboardTemplateRepository) Create(ctx context.Context, tpl *model.DashboardTemplate) error {
	return r.db.WithContext(ctx).Create(tpl).Error
}

// Update 按 map 更新；0 行 ⇒ 模板不存在（db.EnsureAffected）。
func (r *DashboardTemplateRepository) Update(ctx context.Context, id uint, cols map[string]any) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.DashboardTemplate{}).
		Where("id = ?", id).
		Updates(cols))
}

func (r *DashboardTemplateRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.DashboardTemplate{}, id))
}

func (r *DashboardTemplateRepository) IncrementCloneCount(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.DashboardTemplate{}).
		Where("id = ?", id).
		UpdateColumn("clone_count", gorm.Expr("clone_count + 1")))
}

func (r *DashboardTemplateRepository) UpdateRating(ctx context.Context, id uint, score float64) (float64, error) {
	var tpl model.DashboardTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return 0, err
	}
	newCount := tpl.RatingCount + 1
	newRating := (tpl.Rating*float64(tpl.RatingCount) + score) / float64(newCount)
	err := _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.DashboardTemplate{}).
		Where("id = ?", id).
		Updates(map[string]any{"rating": newRating, "rating_count": newCount}))
	return newRating, err
}
