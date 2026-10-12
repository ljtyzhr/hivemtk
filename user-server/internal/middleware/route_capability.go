// 能力面守卫的运行时件：角色 → 已授予的菜单 key 集合 的查询与缓存。
//
// 判据本体在 model/menu_capability.go（生成件）+ 这里的授予查询，
// 由 RoutePolicyGuard 在"仅超管表"之后顺带执行——同一道守卫、同一个位置，
// 免得再挂一个中间件、再让常驻门多认一个守卫名。
//
// 缓存口径（ deliberate ）：
//   - TTL 60s：菜单授权是低频管理动作，60s 的传播窗比每次请求一趟库划算；
//   - 管理员改分配后主动 Invalidate（controller.MenuController.AdminUpdateRoleMenus
//     保存成功后调；service 不能 import 本包，middleware→service 已有依赖方向）；
//     所以界面点完即生效，TTL 只是兜住别的进程实例；
//   - 查询失败时**用旧值**（stale-while-error）：库抖动不该把坐席全数拦在门外，
//     但一次都没有查成功过（无旧值）时按"无授予"处理 ⇒ 拦，向拒的方向收。
package middleware

import (
	"context"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// MenuGrantProvider 返回某角色被授予的菜单 key 集合。
// 装配期由 router.Setup 注入 service 的实现；测试可注入假实现。
type MenuGrantProvider func(ctx context.Context, role string) (map[string]struct{}, error)

// capabilityDeniedMsg 能力面拒绝文案。刻意区别于"需要超管权限"：
// 这一档不是角色不够高，是**该角色没被授予这个功能**，管理员在菜单授权里能开。
const capabilityDeniedMsg = "权限不足，该功能需先被授予对应菜单"

const menuGrantTTL = 60 * time.Second

type menuGrantEntry struct {
	keys     map[string]struct{}
	cachedAt time.Time
}

var (
	menuGrantMu       sync.RWMutex
	menuGrantProvider MenuGrantProvider
	menuGrantCache    = map[string]menuGrantEntry{}
	// menuGrantNow 是可替换的时钟：用例要验 TTL 到期，不该真等 60 秒。
	menuGrantNow = time.Now
)

// SetMenuGrantProvider 注入授予查询；传 nil 撤掉（撤掉后能力面按"无授予"拦）。
func SetMenuGrantProvider(p MenuGrantProvider) {
	menuGrantMu.Lock()
	defer menuGrantMu.Unlock()
	menuGrantProvider = p
	menuGrantCache = map[string]menuGrantEntry{}
}

// InvalidateMenuGrants 丢掉某角色（role 为空则全部）的缓存。
func InvalidateMenuGrants(role string) {
	menuGrantMu.Lock()
	defer menuGrantMu.Unlock()
	if role == "" {
		menuGrantCache = map[string]menuGrantEntry{}
		return
	}
	delete(menuGrantCache, role)
}

// grantedMenuKeys 取角色的授予集合，第三个返回值说明"这次有没有可信读数"。
func grantedMenuKeys(ctx context.Context, role string) (map[string]struct{}, bool, error) {
	menuGrantMu.RLock()
	provider := menuGrantProvider
	entry, hasEntry := menuGrantCache[role]
	menuGrantMu.RUnlock()

	if provider == nil {
		return nil, false, nil
	}
	if hasEntry && menuGrantNow().Sub(entry.cachedAt) < menuGrantTTL {
		return entry.keys, true, nil
	}
	keys, err := provider(ctx, role)
	if err != nil {
		if hasEntry {
			logger.Warnf("能力面授予查询失败，沿用上次读数 role=%s: %v", role, err)
			return entry.keys, true, err
		}
		return nil, false, err
	}
	menuGrantMu.Lock()
	menuGrantCache[role] = menuGrantEntry{keys: keys, cachedAt: menuGrantNow()}
	menuGrantMu.Unlock()
	return keys, true, nil
}

// CapabilityAllows 判定"这条路径所属能力"对给定角色是否放行。
//
// 返回 (pathInCapabilityTable, allowed)。表外的路径由调用方照旧放行。
func CapabilityAllows(ctx context.Context, role, path string) (bool, bool) {
	owners := model.CapabilityOwnersFor(path)
	if len(owners) == 0 {
		return false, true
	}
	if role == "admin" {
		return true, true
	}
	keys, ok, err := grantedMenuKeys(ctx, role)
	if err != nil || !ok {
		if err != nil {
			logger.Warnf("能力面授予不可用，按无授予拦下 role=%s path=%s: %v", role, path, err)
		} else {
			logger.Warnf("能力面守卫没拿到授予查询（装配缺 SetMenuGrantProvider），按无授予拦下 role=%s path=%s",
				role, path)
		}
		return true, false
	}
	for _, owner := range owners {
		for _, mk := range owner.MenuKeys {
			if _, hit := keys[mk]; hit {
				return true, true
			}
		}
	}
	return true, false
}

// CapabilityOwnerKeysFor 供门与导出件用：某路径命中的能力规则及其归属菜单 key。
func CapabilityOwnerKeysFor(path string) []string {
	owners := model.CapabilityOwnersFor(path)
	if len(owners) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	out := []string{}
	for _, o := range owners {
		for _, k := range o.MenuKeys {
			if _, dup := set[k]; dup {
				continue
			}
			set[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}
