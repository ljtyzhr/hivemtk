// custom_tool.go OpenAPI 自定义工具仓储（P1-2，2026-10-25 自 service 层下沉）。
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// CustomToolRepository custom_tools 表 CRUD。
type CustomToolRepository struct {
	db *gorm.DB
}

// NewCustomToolRepository 默认构造（全局句柄，仓库既有惯例）。
func NewCustomToolRepository() *CustomToolRepository {
	return &CustomToolRepository{db: GetDB()}
}

// FindByName 按名字查；不存在返回 gorm.ErrRecordNotFound。
func (r *CustomToolRepository) FindByName(ctx context.Context, name string) (*model.CustomTool, error) {
	var row model.CustomTool
	err := r.db.WithContext(ctx).Where("name = ?", name).First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Create 落库。
func (r *CustomToolRepository) Create(ctx context.Context, rec *model.CustomTool) error {
	return r.db.WithContext(ctx).Create(rec).Error
}

// UpdateToolCount 回写工具数（导入注册成功后）。
func (r *CustomToolRepository) UpdateToolCount(ctx context.Context, id uint, n int) error {
	return r.db.WithContext(ctx).Model(&model.CustomTool{}).Where("id = ?", id).
		Update("tool_count", n).Error
}

// List 全量列表（租户隔离由 tenancyidx 全局 callback 负责，不手写过滤）。
func (r *CustomToolRepository) List(ctx context.Context) ([]*model.CustomTool, error) {
	var out []*model.CustomTool
	err := r.db.WithContext(ctx).Order("id DESC").Find(&out).Error
	return out, err
}

// Delete 先确认存在再删（保留 service 原先「不存在即报错」语义）。
func (r *CustomToolRepository) Delete(ctx context.Context, id uint) error {
	var rec model.CustomTool
	if err := r.db.WithContext(ctx).First(&rec, id).Error; err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&model.CustomTool{}, id).Error
}
