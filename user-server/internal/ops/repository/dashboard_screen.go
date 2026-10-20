package repository

import (
	"context"

	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"

	"gorm.io/gorm"
)

// DashboardScreenRepository 数据大屏仓库
type DashboardScreenRepository struct {
	db *gorm.DB
}

// NewDashboardScreenRepository 创建数据大屏仓库实例
func NewDashboardScreenRepository() *DashboardScreenRepository {
	return &DashboardScreenRepository{
		db: _db.GetDB(),
	}
}

// Create 创建大屏
func (r *DashboardScreenRepository) Create(screen *model.DashboardScreen) error {
	return r.db.Create(screen).Error
}

// GetByID 根据 ID 获取大屏
func (r *DashboardScreenRepository) GetByID(id uint) (*model.DashboardScreen, error) {
	var screen model.DashboardScreen
	err := r.db.First(&screen, id).Error
	return &screen, err
}

// GetAll 获取所有大屏列表
func (r *DashboardScreenRepository) GetAll(page, pageSize int) ([]*model.DashboardScreen, int64, error) {
	var screens []*model.DashboardScreen
	var total int64

	r.db.Model(&model.DashboardScreen{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&screens).Error

	return screens, total, err
}

// GetByCode 根据编码获取大屏
func (r *DashboardScreenRepository) GetByCode(code string) (*model.DashboardScreen, error) {
	var screen model.DashboardScreen

	err := r.db.Where("code = ? AND is_public = ?", code, true).First(&screen).Error
	return &screen, err
}

// GetPublicByCodeCtx 匿名公开访问：code 由分享链接携带，先系统作用域反查
// 唯一租户，再钉死在该租户内读取并自增浏览量。同码多租户时拒猜（返回未找到）。
func (r *DashboardScreenRepository) GetPublicByCodeCtx(ctx context.Context, code string) (*model.DashboardScreen, error) {
	var ids []uint
	if err := r.db.WithContext(tenant.WithSystemScope(ctx)).
		Model(&model.DashboardScreen{}).
		Distinct("tenant_id").
		Where("code = ? AND is_public = ?", code, true).
		Limit(2).
		Pluck("tenant_id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) != 1 || ids[0] == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	tctx := tenant.OnlyTenant(ctx, ids[0])
	var screen model.DashboardScreen
	if err := r.db.WithContext(tctx).
		Where("code = ? AND is_public = ?", code, true).
		First(&screen).Error; err != nil {
		return nil, err
	}
	if err := r.db.WithContext(tctx).Model(&model.DashboardScreen{}).
		Where("id = ?", screen.ID).
		Update("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}
	return &screen, nil
}

// Update 更新大屏
func (r *DashboardScreenRepository) Update(screen *model.DashboardScreen) error {
	return r.db.Save(screen).Error
}

// Delete 删除大屏
func (r *DashboardScreenRepository) Delete(id uint) error {
	return r.db.Delete(&model.DashboardScreen{}, id).Error
}

// IncrementViewCount 增加访问次数
func (r *DashboardScreenRepository) IncrementViewCount(id uint) error {
	return r.db.Model(&model.DashboardScreen{}).Where("id = ?", id).Update("view_count", gorm.Expr("view_count + 1")).Error
}

// DashboardWidgetRepository 大屏 Widget 仓库
type DashboardWidgetRepository struct {
	db *gorm.DB
}

// NewDashboardWidgetRepository 创建大屏 Widget 仓库实例
func NewDashboardWidgetRepository() *DashboardWidgetRepository {
	return &DashboardWidgetRepository{
		db: _db.GetDB(),
	}
}

// Create 创建 Widget
func (r *DashboardWidgetRepository) Create(widget *model.DashboardWidget) error {
	return r.db.Create(widget).Error
}

// GetByScreenID 根据大屏 ID 获取 Widgets
func (r *DashboardWidgetRepository) GetByScreenID(screenID uint) ([]*model.DashboardWidget, error) {
	var widgets []*model.DashboardWidget
	err := r.db.Where("screen_id = ?", screenID).Order("sort_order, y, x").Find(&widgets).Error
	return widgets, err
}

// GetByScreenIDCtx 公开访问路径的 widgets 读取（必须带已绑定租户的 ctx）
func (r *DashboardWidgetRepository) GetByScreenIDCtx(ctx context.Context, screenID uint) ([]*model.DashboardWidget, error) {
	var widgets []*model.DashboardWidget
	err := r.db.WithContext(ctx).Where("screen_id = ?", screenID).Order("sort_order, y, x").Find(&widgets).Error
	return widgets, err
}

// DeleteByScreenID 删除大屏下所有 Widgets
func (r *DashboardWidgetRepository) DeleteByScreenID(screenID uint) error {
	return r.db.Where("screen_id = ?", screenID).Delete(&model.DashboardWidget{}).Error
}
