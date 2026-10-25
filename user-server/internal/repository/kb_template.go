// kb_template.go 知识库模板仓储（R39-K7）。
package repository

import (
	"context"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type KbTemplateRepository struct {
	db *gorm.DB
}

func NewKbTemplateRepository() *KbTemplateRepository {
	return &KbTemplateRepository{db: _db.GetDB()}
}

func (r *KbTemplateRepository) List(ctx context.Context, category, status string) ([]*model.KbTemplate, error) {
	q := r.db.WithContext(ctx).Model(&model.KbTemplate{})
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []*model.KbTemplate
	err := q.Order("id DESC").Find(&list).Error
	return list, err
}

func (r *KbTemplateRepository) GetByID(ctx context.Context, id uint) (*model.KbTemplate, error) {
	var tpl model.KbTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return nil, err
	}
	return &tpl, nil
}

func (r *KbTemplateRepository) Create(ctx context.Context, tpl *model.KbTemplate) error {
	return r.db.WithContext(ctx).Create(tpl).Error
}

// Update 按 map 更新，0 值字段得以清空；0 行 ⇒ 模板不存在（db.EnsureAffected）。
func (r *KbTemplateRepository) Update(ctx context.Context, id uint, cols map[string]any) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.KbTemplate{}).
		Where("id = ?", id).
		Updates(cols))
}

func (r *KbTemplateRepository) Delete(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).Delete(&model.KbTemplate{}, id))
}

// IncrementApplyCount apply 成功后计数；0 行 ⇒ 模板在计数前被删（幂等收尾不报错）。
func (r *KbTemplateRepository) IncrementApplyCount(ctx context.Context, id uint) error {
	return _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.KbTemplate{}).
		Where("id = ?", id).
		UpdateColumn("apply_count", gorm.Expr("apply_count + 1")))
}

// UpdateRating 运行均值：new = (rating*count + score) / (count+1)，与
// content 模板市场"逐次平均"口径不同——这里保留了真实计数，均值不被顺序影响。
func (r *KbTemplateRepository) UpdateRating(ctx context.Context, id uint, score float64) (float64, error) {
	var tpl model.KbTemplate
	if err := r.db.WithContext(ctx).First(&tpl, id).Error; err != nil {
		return 0, err
	}
	newCount := tpl.RatingCount + 1
	newRating := (tpl.Rating*float64(tpl.RatingCount) + score) / float64(newCount)
	err := _db.EnsureAffected(r.db.WithContext(ctx).
		Model(&model.KbTemplate{}).
		Where("id = ?", id).
		Updates(map[string]any{"rating": newRating, "rating_count": newCount}))
	return newRating, err
}
