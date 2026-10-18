package service

import (
	"context"
	"time"

	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

// 模块同步后台任务（模块化改造 · T305）
//
// ============================================================
// 为什么必须真的有这个任务
//
// 模块闸（middleware.GuardModulePath）在「本地有模块数据」时才拦未开通的模块；
// 而本地数据只有一个来源：平台的下发同步。这个任务**不存在**时，
// tenant_modules 表恒空 → 闸永远处于「首启全放行」态 →
// 付费模块对所有人放行，而界面上看不出任何差别（菜单都在、页面能开、数据都对）。
//
// 也就是说：闸的代码全部正确、测试全部通过、而商业模式整体失效。
// 这类失效没有任何单点测试能发现 —— 只有"这条数据谁来写"被钉住才行。
// ============================================================

// moduleSyncInterval 模块下发同步周期。
//
// 取 10 分钟的判据：模块开通是一个"低频但要尽快生效"的操作 —— 平台确认收款后
// 用户最多等 10 分钟就看到菜单变化，这个延迟用户能接受；而更短的周期会把
// 一次 HMAC 往返放大到每天 144+ 次，对一个目前由个人运营的后台没有意义。
const moduleSyncInterval = 10 * time.Minute

// moduleSyncRunner 模块同步循环。抽出成结构体是为了让 Stop 有确切归属，
// 且便于单测用短周期驱动它（否则测试要么真等 10 分钟，要么只能测单次调用）。
type moduleSyncRunner struct {
	cancel    context.CancelFunc
	done      chan struct{}
	firstDone chan struct{}
}

// StartModuleSync 启动模块同步后台任务。
//
// 首轮**同步执行**（不是丢进 goroutine）：模块闸的第一次判定必须基于真实数据。
// 丢进异步的话，router.Setup 装上闸之后到首轮同步完成之间存在一个窗口，
// 窗口内所有人全放行 —— 而那恰好是服务刚起来的时刻，也是并发最高的时刻。
//
// 返回的 runner 可被 StopModuleSync 停掉（测试与优雅退出用）。
func StartModuleSync(parent context.Context) *moduleSyncRunner {
	ctx, cancel := context.WithCancel(parent)
	// 同步循环在请求之外（Background 派生），on 模式下租户 callback 会
	// fail-closed 拒绝读写 tenant_modules → 恒空 → 闸退化成全放行（P1）。
	// 挂 SystemScope：镜像统一落默认租户（单商户单部署）。
	// ModuleCatalogService.Sync 内部还会再包一层，重复包裹无害 ——
	// 两处各自独立成立，将来谁只改了其中一处都不会让另一处回退。
	ctx = tenant.WithSystemScope(ctx)
	r := &moduleSyncRunner{
		cancel:    cancel,
		done:      make(chan struct{}),
		firstDone: make(chan struct{}),
	}
	svc := NewModuleCatalogService()

	// 首轮同步：失败不阻断启动 —— 平台不可达是常态（离线部署），
	// 此时离线兜底会把基础模块写进去，模块闸照常工作。
	res, err := svc.Sync(ctx)
	if err != nil {
		logger.Warnf("[module-sync] 首轮同步失败（按离线兜底处理）: %v", err)
	} else {
		logger.Infof("[module-sync] 首轮同步完成：%d 个模块（来源 %s）",
			res.ActiveCount, res.Source)
	}
	close(r.firstDone)

	utils.SafeGo(ctx, "module.sync.loop", func(ctx context.Context) {
		defer close(r.done)
		ticker := time.NewTicker(moduleSyncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 同步里已经处理了"平台不可达 → 沿用上次镜像"，
				// 这里不需要再判一次可达性。
				if _, err := svc.Sync(ctx); err != nil {
					logger.Warnf("[module-sync] 周期同步失败: %v", err)
				}
			}
		}
	})

	logger.Infof("[module-sync] 模块同步已启动，周期 %s", moduleSyncInterval)
	return r
}

// Stop 停止同步循环并等待退出
func (r *moduleSyncRunner) Stop() {
	if r == nil {
		return
	}
	r.cancel()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		logger.Warn("[module-sync] 停止等待超时（3s），协程可能仍在收尾")
	}
}

// WaitFirstSync 等待首轮同步完成（测试用）
func (r *moduleSyncRunner) WaitFirstSync() { <-r.firstDone }
