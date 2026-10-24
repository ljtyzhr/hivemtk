package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
)

// RoleMenuPermissionRepository 角色↔菜单分配仓储接口。
// admin 恒全量不落库；仅 customer_service/staff 两档存行。
type RoleMenuPermissionRepository interface {
	ListKeysByRole(ctx context.Context, role string) ([]string, error)
	ReplaceRoleKeys(ctx context.Context, role string, keys []string) error
	CountAll(ctx context.Context) (int64, error)
	BulkInsert(ctx context.Context, rows []*model.RoleMenuPermission) error
}

type roleMenuRepo struct {
	db *gorm.DB
}

func NewRoleMenuPermissionRepository() RoleMenuPermissionRepository {
	return &roleMenuRepo{db: db.GetDB()}
}

func (r *roleMenuRepo) ListKeysByRole(ctx context.Context, role string) ([]string, error) {
	var keys []string
	if err := r.db.WithContext(ctx).
		Model(&model.RoleMenuPermission{}).
		Where("role_code = ?", role).
		Order("menu_key ASC").
		Pluck("menu_key", &keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// ReplaceRoleKeys 事务内清空该角色旧行再批量插入新 keys（去重、去空）
func (r *roleMenuRepo) ReplaceRoleKeys(ctx context.Context, role string, keys []string) error {
	seen := make(map[string]struct{}, len(keys))
	uniq := make([]string, 0, len(keys))
	for _, k := range keys {
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, k)
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_code = ?", role).Delete(&model.RoleMenuPermission{}).Error; err != nil {
			return err
		}
		if len(uniq) == 0 {
			return nil
		}
		rows := make([]*model.RoleMenuPermission, 0, len(uniq))
		for _, k := range uniq {
			rows = append(rows, &model.RoleMenuPermission{RoleCode: role, MenuKey: k})
		}
		return tx.Create(rows).Error
	})
}

func (r *roleMenuRepo) CountAll(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.RoleMenuPermission{}).Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (r *roleMenuRepo) BulkInsert(ctx context.Context, rows []*model.RoleMenuPermission) error {
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(rows).Error
}
