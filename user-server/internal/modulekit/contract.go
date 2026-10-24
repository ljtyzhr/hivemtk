package modulekit

import (
	"context"
	"fmt"
)

// PluginSymbol 是 Go plugin 必须导出的符号名。
//
// 宿主侧约定：plugin.Open(path) 之后 plugin.Lookup(PluginSymbol)，把返回的
// interface{} 交给 Registry.RegisterFromLookup 断言并登记。插件侧约定：
//
//	var Module modulekit.Module = impl{}
//
// 详见 doc.go 的「两种实现如何挂进同一张表」。
const PluginSymbol = "Module"

// Module 是模块契约：任何内建模块或 Go plugin 导出的符号都必须实现它。
//
// 实现约定：
//   - ID() 与 Descriptor() 是静态声明，注册表在 Register 时快照，之后不重读；
//   - Enable / Disable 必须幂等（重复调用返回 nil）；
//   - Health 只对已启用的模块有意义；未启用由注册表兜底返回 ErrModuleNotEnabled；
//   - 三个生命周期方法都必须可并发调用（健康巡检与启停可能同时打到同一模块）。
type Module interface {
	// ID 返回模块唯一标识。注册表按它判重、按它查未注册，全库唯一事实源。
	ID() string
	// Descriptor 返回能力声明（名称/版本/依赖/能力点），不参与判重。
	Descriptor() Descriptor
	// Enable 激活模块。必须幂等。
	Enable(ctx context.Context) error
	// Disable 停用模块。必须幂等。
	Disable(ctx context.Context) error
	// Health 返回模块健康状态：健康返回 nil，否则返回具体错误。
	Health(ctx context.Context) error
}

// Descriptor 能力声明：注册表靠它做依赖校验，运维靠它知道「这模块是什么」。
type Descriptor struct {
	// Name 人类可读名（展示用，不参与判重）。
	Name string
	// Version 模块自身版本（如 "v0.1.0"）。
	Version string
	// DependsOn 依赖的模块 ID 列表：Enable 时要求它们已启用。
	DependsOn []string
	// Capabilities 声明提供的能力点（自由标签，如 "auth.check"）。
	Capabilities []string
}

// ModuleState 模块生命周期状态。
type ModuleState int

const (
	// StateUnknown 未注册（State 对未注册 ID 的返回值）。
	StateUnknown ModuleState = iota
	// StateDisabled 已注册但未启用（Register 之后的初始状态）。
	StateDisabled
	// StateEnabled 已启用。
	StateEnabled
)

// String 状态的可读表示（日志/观测用）。
func (s ModuleState) String() string {
	switch s {
	case StateUnknown:
		return "unknown"
	case StateDisabled:
		return "disabled"
	case StateEnabled:
		return "enabled"
	default:
		return fmt.Sprintf("ModuleState(%d)", int(s))
	}
}
