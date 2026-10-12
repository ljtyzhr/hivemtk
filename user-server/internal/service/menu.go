package service

import (
	"context"
	"fmt"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

// MenuService 菜单权限服务（RBAC 菜单分配的业务层）
//
// 规则：
//   - admin 恒返回全量菜单，不落库、不可被分配
//   - customer_service/staff 按 role_menu_permissions 表过滤
//   - 首次全表为空时按注册表 Roles 词表自动 seed 一次默认分配
type MenuService struct {
	repo repository.RoleMenuPermissionRepository
}

// NewMenuService 构造
func NewMenuService() *MenuService {
	return &MenuService{repo: repository.NewRoleMenuPermissionRepository()}
}

// NewMenuServiceWithRepo 注入 repository（便于测试）
func NewMenuServiceWithRepo(repo repository.RoleMenuPermissionRepository) *MenuService {
	return &MenuService{repo: repo}
}

// GetAllMenuTree 返回完整注册表（管理端 el-tree 展示用）
func (s *MenuService) GetAllMenuTree() []model.SystemMenuNode {
	return model.SystemMenuTree
}

// GetUserMenuTree 当前登录用户可见菜单树
//
// 错误：角色非法 → ErrInvalidInput
func (s *MenuService) GetUserMenuTree(ctx context.Context, role string) ([]model.SystemMenuNode, error) {
	if role == model.SystemRoleCodeAdmin {
		return model.SystemMenuTree, nil
	}
	if !model.IsValidRole(role) {
		return nil, fmt.Errorf("角色不存在: %w", ErrInvalidInput)
	}
	keys, err := s.ensureSeededAndGetKeys(ctx, role)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return model.FilterMenuTree(model.SystemMenuTree, set), nil
}

// GetRoleMenuKeys 某角色已分配的菜单 key 列表（admin 返回全量）
//
// 错误：角色非法 → ErrInvalidInput
func (s *MenuService) GetRoleMenuKeys(ctx context.Context, role string) ([]string, error) {
	if role == model.SystemRoleCodeAdmin {
		return model.FlattenMenuKeys(model.SystemMenuTree), nil
	}
	if !model.IsValidRole(role) {
		return nil, fmt.Errorf("角色不存在: %w", ErrInvalidInput)
	}
	return s.ensureSeededAndGetKeys(ctx, role)
}

// UpdateRoleMenuKeys 覆盖式保存某角色的菜单分配
//
// 业务规则：
//   - admin 不可被分配（恒全量）→ ErrInvalidInput
//   - 角色 code 非法 → ErrInvalidInput
//   - 任一 menu key 不在注册表 → ErrInvalidInput
//   - keys 为空 = 清空该角色全部菜单（合法，前端应二次确认）
func (s *MenuService) UpdateRoleMenuKeys(ctx context.Context, role string, keys []string) error {
	if role == model.SystemRoleCodeAdmin {
		return fmt.Errorf("超管拥有全部菜单权限，无需分配: %w", ErrInvalidInput)
	}
	if !model.IsValidRole(role) {
		return fmt.Errorf("角色不存在: %w", ErrInvalidInput)
	}
	for _, k := range keys {
		if !model.ExistsMenuKey(k) {
			return fmt.Errorf("菜单不存在: %s: %w", k, ErrInvalidInput)
		}
	}
	return s.repo.ReplaceRoleKeys(ctx, role, keys)
}

// ensureSeededAndGetKeys 取角色分配 keys；全表为空时先 seed 一次默认分配
func (s *MenuService) ensureSeededAndGetKeys(ctx context.Context, role string) ([]string, error) {
	n, err := s.repo.CountAll(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		s.seedDefaults(ctx)
	}
	return s.repo.ListKeysByRole(ctx, role)
}

// seedDefaults 首空表为 customer_service/staff 写入默认分配（admin 不落库）
//
// seed 失败不阻断读取：并发 seed 撞唯一索引时以已写入行为准。
func (s *MenuService) seedDefaults(ctx context.Context) {
	for _, role := range []string{model.SystemRoleCodeCustomerService, model.SystemRoleCodeStaff} {
		keys := model.DefaultMenuKeys(role)
		rows := make([]*model.RoleMenuPermission, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, &model.RoleMenuPermission{RoleCode: role, MenuKey: k})
		}
		if err := s.repo.BulkInsert(ctx, rows); err != nil {
			logger.Warnf("[MenuService] seed 默认菜单分配失败 role=%s: %v", role, err)
		}
	}
}
