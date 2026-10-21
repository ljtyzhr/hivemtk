package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/content/model"

	"gorm.io/gorm"
)

type MaterialCategoryRepository interface {
	Create(ctx context.Context, category *model.MaterialCategory) error
	GetByID(ctx context.Context, id string) (*model.MaterialCategory, error)
	GetList(ctx context.Context, licenseID string, parentID string, materialType string, page int, limit int) ([]*model.MaterialCategory, int64, error)
	Update(ctx context.Context, category *model.MaterialCategory) error
	Delete(ctx context.Context, id string) error
	GetTree(ctx context.Context, licenseID string, materialType string) ([]*model.MaterialCategory, error)
	UpdateMaterialCount(ctx context.Context, categoryID string) error
}

type materialCategoryRepo struct {
	db *gorm.DB
}

// NewMaterialCategoryRepository 由装配点注入句柄：仓储不自己摸全局 db.GetDB()。
func NewMaterialCategoryRepository(db *gorm.DB) MaterialCategoryRepository {
	return &materialCategoryRepo{db: db}
}

func (r *materialCategoryRepo) Create(ctx context.Context, category *model.MaterialCategory) error {
	return r.db.WithContext(ctx).Create(category).Error
}

// GetByID err 非空时返回 nil（与素材腿同一口径）。
func (r *materialCategoryRepo) GetByID(ctx context.Context, id string) (*model.MaterialCategory, error) {
	var category model.MaterialCategory
	if err := r.db.WithContext(ctx).Preload("Parent").Where("id = ?", id).First(&category).Error; err != nil {
		return nil, err
	}
	return &category, nil
}

func (r *materialCategoryRepo) GetList(ctx context.Context, licenseID string, parentID string, materialType string, page int, limit int) ([]*model.MaterialCategory, int64, error) {
	var categories []*model.MaterialCategory
	var total int64
	page, limit = normalizeMaterialPaging(page, limit)

	cond := func() *gorm.DB {
		query := r.db.WithContext(ctx).Model(&model.MaterialCategory{}).Where("license_id = ?", licenseID)
		if parentID != "" {
			query = query.Where("parent_id = ?", parentID)
		}
		if materialType != "" {
			query = query.Where("type = ?", materialType)
		}
		return query
	}

	if err := cond().Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := cond().Preload("Parent").Offset((page - 1) * limit).Limit(limit).Order("sort ASC, created_at DESC").Find(&categories).Error
	return categories, total, err
}

// Update 只覆写本行可编辑列，归属列与主键、创建时间一起排除（理由见素材腿同名方法）。
func (r *materialCategoryRepo) Update(ctx context.Context, category *model.MaterialCategory) error {
	res := r.db.WithContext(ctx).Model(category).
		Select("*").Omit("id", "created_at", "tenant_id", "license_id", "user_id").Updates(category)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMaterialCategoryMissing
	}
	return nil
}

// Delete 先自证行在本租户内可寻址，再挡掉「还有子分类/还有素材」两种非空场景。
// 命中 0 行（并发被别处删掉）交出缺失错误，而不是静默成功。
func (r *materialCategoryRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.GetByID(ctx, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrMaterialCategoryMissing
		}
		return err
	}

	// ⚠️ 这条不变量只有 Go 里这一处守：schema 里 materials.category_id / parent_id 上没有外键
	// 约束（变异 M14 验证：裸删返回成功、子分类与素材变成孤儿行）。所以「非空即拒」不能省，
	// 也不能像改前那样伪造一个 gorm.ErrForeignKeyViolated 上抛——那条文案不含「不存在」，
	// 出口层只能给 500，运维看不出是「分类非空」。
	var childCount int64
	if err := r.db.WithContext(ctx).Model(&model.MaterialCategory{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		return err
	}
	if childCount > 0 {
		return ErrMaterialCategoryNotEmpty
	}

	var materialCount int64
	if err := r.db.WithContext(ctx).Model(&model.Material{}).Where("category_id = ?", id).Count(&materialCount).Error; err != nil {
		return err
	}
	if materialCount > 0 {
		return ErrMaterialCategoryNotEmpty
	}

	res := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.MaterialCategory{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrMaterialCategoryMissing
	}
	return nil
}

func (r *materialCategoryRepo) GetTree(ctx context.Context, licenseID string, materialType string) ([]*model.MaterialCategory, error) {
	var categories []*model.MaterialCategory

	query := r.db.WithContext(ctx).Where("license_id = ?", licenseID)
	if materialType != "" {
		query = query.Where("type = ?", materialType)
	}

	if err := query.Preload("Children").Order("sort ASC, created_at DESC").Find(&categories).Error; err != nil {
		return nil, err
	}

	var rootCategories []*model.MaterialCategory
	for _, category := range categories {
		if category.ParentID == nil {
			rootCategories = append(rootCategories, category)
		}
	}

	return rootCategories, nil
}

func (r *materialCategoryRepo) UpdateMaterialCount(ctx context.Context, categoryID string) error {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Material{}).Where("category_id = ?", categoryID).Count(&count).Error; err != nil {
		return err
	}

	return r.db.WithContext(ctx).Model(&model.MaterialCategory{}).Where("id = ?", categoryID).Update("material_count", count).Error
}
