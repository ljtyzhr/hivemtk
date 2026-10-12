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

// GrantProvider 给能力面守卫用的"角色 → 已授予菜单 key 集合"查询。
//
// 与 GetRoleMenuKeys 的差别只有两点，两点都是守卫这条链路要的：
//   - 返回集合而不是切片（守卫是 O(1) 命中判断，每条请求都跑）；
//   - 首空表**不落库**：守卫跑在读到空表的那一刻，写库不是它的职责，
//     直接按 DefaultMenuKeys 的出厂分配判，保证首屏菜单和首屏接口档位一致。
//
// 非法/空角色返回空集合：能力面按"无授予"拦（向拒的方向收）。
func (s *MenuService) GrantProvider(ctx context.Context, role string) (map[string]struct{}, error) {
	if role == model.SystemRoleCodeAdmin {
		return nil, nil // 守卫对 admin 短路，不走到这里
	}
	if !model.IsValidRole(role) {
		return map[string]struct{}{}, nil
	}
	n, err := s.repo.CountAll(ctx)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		keys := model.DefaultMenuKeys(role)
		set := make(map[string]struct{}, len(keys))
		for _, k := range keys {
			set[k] = struct{}{}
		}
		return set, nil
	}
	list, err := s.repo.ListKeysByRole(ctx, role)
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(list))
	for _, k := range list {
		set[k] = struct{}{}
	}
	return set, nil
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
