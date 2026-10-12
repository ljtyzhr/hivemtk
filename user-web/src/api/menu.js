// menu.js 菜单权限 API（RBAC）
//
// 用户端：GET /api/menus —— 当前登录用户可见菜单树（JWT role 决定）
// 管理端（RequireAdmin）：
//   GET  /api/system/menus              完整菜单注册表（el-tree 数据源）
//   GET  /api/system/roles/:code/menus  某角色已分配 menu_keys
//   PUT  /api/system/roles/:code/menus  覆盖保存 { menu_keys: [...] }
import { http } from '@/utils/request'

// 当前用户可见菜单树 → { tree, all }
export function getUserMenus() {
  return http.get('/api/menus')
}

// 完整菜单注册表（管理端分配界面）
export function getAllMenus() {
  return http.get('/api/system/menus')
}

// 某角色已分配的菜单 key
export function getRoleMenuKeys(code) {
  return http.get(`/api/system/roles/${code}/menus`)
}

// 覆盖式保存某角色的菜单分配
export function updateRoleMenuKeys(code, menuKeys) {
  return http.put(`/api/system/roles/${code}/menus`, { menu_keys: menuKeys })
}

export const menuApi = {
  getUserMenus,
  getAllMenus,
  getRoleMenuKeys,
  updateRoleMenuKeys
}

export default menuApi
