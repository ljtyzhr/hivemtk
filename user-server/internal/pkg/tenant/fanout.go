package tenant

import (
	"context"
	"fmt"
	"sync/atomic"

	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// 后台任务租户扇出（specs/001-saas-multi-tenant T020）
//
// 全部非 UI 自动流程（cron/SOP/队列消费）统一经 Fanout 执行：
//   - off 模式：单次回调，ctx 绑默认租户（等价既有行为）
//   - on 模式：枚举活跃租户（tenants.status=1）逐租户回调，单租户失败不阻断其余
//
// 回调内的一切 GORM 访问经 WithContext(ctx) 自动获得租户隔离（T005 callback）。

var activeTenantLoader atomic.Value // 存 func(ctx) ([]uint, error)，装配层注入（避免 pkg/tenant 依赖 db 包）

// SetActiveTenantLoader 注入活跃租户枚举器（db.AutoMigrate 后由装配层注册）
func SetActiveTenantLoader(fn func(ctx context.Context) ([]uint, error)) {
	activeTenantLoader.Store(fn)
}

func loadActiveTenants(ctx context.Context) ([]uint, bool) {
	fn, ok := activeTenantLoader.Load().(func(ctx context.Context) ([]uint, error))
	if !ok || fn == nil {
		return nil, false
	}
	ids, err := fn(ctx)
	if err != nil {
		return nil, false
	}
	return ids, true
}

// Fanout 以租户维度执行 fn，汇总各租户返回值。
//
// 返回值语义：fn 的第一个返回值跨租户求和；任一租户失败记录后继续，
// 全部失败时返回最后一个错误。off 模式退化为单默认租户。
func Fanout(ctx context.Context, db *gorm.DB, fn func(ctx context.Context, tenantID uint) (int64, error)) (int64, error) {
	if fn == nil {
		return 0, nil
	}
	if !IsMultiTenant() {
		return fn(ContextWithDefault(ctx), DefaultTenantID)
	}

	ids, ok := loadActiveTenants(ctx)
	if !ok {
		// 枚举器缺失（漏装配）：fail-closed——on 模式下绝不静默跑全表
		return 0, fmt.Errorf("[tenant] Fanout 枚举器未装配，拒绝后台任务执行")
	}

	var total int64
	var lastErr error
	ran := 0
	for _, id := range ids {
		select {
		case <-ctx.Done():
			return total, ctx.Err()
		default:
		}
		n, err := fn(WithTenantID(ctx, id), id)
		if err != nil {
			lastErr = fmt.Errorf("租户 %d: %w", id, err)
			// 单租户失败不阻断其余租户（对外语义不变，见函数注释），但**必须出声**：
			// lastErr 只在"全部失败"时才返回给调用方，部分失败若只记在内存里就是
			// 一格彻底的静默吞错（二审 P2-4：生产上表现为某租户的任务长期不跑，
			// 日志里一行痕迹都没有）。
			logger.Warnf("[tenant] Fanout 租户 %d 执行失败（本轮继续其余租户）: %v", id, err)
			continue
		}
		total += n
		ran++
	}
	if ran == 0 && lastErr != nil && len(ids) > 0 {
		return total, lastErr
	}
	return total, nil
}

// ResetRegistryForTest 清空装配期注入的租户加载器，仅供测试使用。
//
// 为什么需要它：activeTenantLoader 是**进程级 atomic.Value**，一旦某个用例
// 注入了 loader，后续用例即使 SetActiveTenantLoader(nil) 也会踩到
// "atomic.Value 存过不同具体类型再存 nil 会 panic" 这条坑；
// 而 fanout 的默认行为必须回到"单默认租户"，否则用例之间会互相污染且
// 症状看起来像 fanout 本身有 bug。
func ResetRegistryForTest() {
	activeTenantLoader.Store(func(ctx context.Context) ([]uint, error) { return nil, nil })
}
