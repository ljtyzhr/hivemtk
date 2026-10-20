// Package reporting 收口「报表/列表在调用方没给 days 时的默认窗口天数」这一个语义。
//
// 为什么要有它：全仓有十几处报表/列表入口各自写死 `days = 30` 兜底
// （controller 的 DefaultQuery("days","30")、service/repository 的 `days <= 0 → 30` 夹取）。
// 逐处就地接参数中心做不到——这些点位横跨 repository / controller / geo / knowledge /
// ops 等多个包，其中大半**不能反向 import internal/service**（service 反向依赖它们，
// 就地直连会成环）。所以走 provider 注入范式（与 middleware.SetAuditFlushIntervalProvider
// 同一形状）：本包是叶子包只依赖 context，装配层 internal/app 注入读参数中心的闭包。
//
// 行为零变更：provider 未注入、读到非正值、或 provider 返回错误值时，一律回落
// DefaultWindowDaysValue = 30——即接线前各处写死的字面量。
package reporting

import (
	"context"
	"sync"
)

// DefaultWindowDaysValue 报表默认窗口天数的编译期兜底。
// 与 config_params 种子 reporting/default_window_days 的 DefaultValue 严格同值，
// 改一处必须改另一处（scripts/check-config-param-defaults.py 会核对读取点兜底位）。
const DefaultWindowDaysValue = 30

// windowDaysProvider 延迟读参数中心的闭包；由 internal/app 装配层注入。
// 不捕获装配时的 ctx——provider 是每请求热读的，见包注释。
var (
	windowDaysMu     sync.RWMutex
	windowDaysReader func(ctx context.Context) int
)

// SetDefaultWindowDaysProvider 注入参数中心读取口（装配层调用一次）。
func SetDefaultWindowDaysProvider(f func(ctx context.Context) int) {
	windowDaysMu.Lock()
	windowDaysReader = f
	windowDaysMu.Unlock()
}

// DefaultWindowDays 返回报表/列表的默认窗口天数（每请求热读）。
//
// 未注入 provider（测试/未装配）或 provider 返回非正值时回落 DefaultWindowDaysValue，
// 保证「有没有这条参数行」不改变任何现有行为。
func DefaultWindowDays(ctx context.Context) int {
	windowDaysMu.RLock()
	f := windowDaysReader
	windowDaysMu.RUnlock()
	if f == nil {
		return DefaultWindowDaysValue
	}
	if v := f(ctx); v > 0 {
		return v
	}
	return DefaultWindowDaysValue
}
