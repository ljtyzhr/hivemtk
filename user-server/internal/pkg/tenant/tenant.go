// Package tenant 提供租户上下文与双形态（SaaS 多租户 / 离线单机）模式解析。
//
// 设计依据: specs/001-saas-multi-tenant/（research.md D5/D6）
//   - TENANT_MODE 环境变量决定形态: off（默认，离线单机）| on（SaaS 多租户）
//   - 离线模式一切数据归属 DefaultTenantID，行为与历史版本等价
//   - 租户身份经由 context 贯通入口 → GORM callback → 后台任务（行自证）
//   - SystemScope 是 migration/备份/健康检查等系统操作的唯一豁免通道
//
// 使用方式:
//
//	import "hivemtk-user/internal/pkg/tenant"
//
//	ctx = tenant.WithTenantID(ctx, 42)
//	if id, ok := tenant.FromContext(ctx); ok { ... }
//	if tenant.IsSystemScope(ctx) { /* 系统操作，豁免隔离 */ }
package tenant

import (
	"context"
	"log"
	"os"
	"strings"
	"sync/atomic"
)

// Mode 部署形态
type Mode string

const (
	// ModeOff 离线单机模式（默认）：全部数据归属默认租户，行为等价历史版本
	ModeOff Mode = "off"
	// ModeOn SaaS 多租户模式：租户注册/隔离/扇出全机制生效
	ModeOn Mode = "on"
)

// EnvMode 形态开关的环境变量名
const EnvMode = "TENANT_MODE"

// DefaultTenantID 默认租户：离线单机部署全部数据归属；存量数据升级后归入此租户
const DefaultTenantID uint = 1

// ResolveMode 解析 TENANT_MODE 环境变量值。
// 返回 (模式, 是否为已知值)：空串/未设置/off/OFF → off+true；on/ON → on+true；
// 其他值 fail-safe 朝向 off（未知配置绝不打开多租户），第二个返回值为 false 供调用方告警。
// 大小写不敏感，容忍首尾空白。
func ResolveMode(env string) (Mode, bool) {
	v := strings.ToLower(strings.TrimSpace(env))
	switch v {
	case "":
		return ModeOff, true
	case string(ModeOff):
		return ModeOff, true
	case string(ModeOn):
		return ModeOn, true
	default:
		return ModeOff, false
	}
}

// currentMode 进程级模式。atomic.Value 保证并发读安全；
// 启动期 InitFromEnv 写一次，测试可经 SetMode 显式注入。
var currentMode atomic.Value

// InitFromEnv 从环境变量初始化进程级模式，返回生效模式。
// 未知值打告警日志（fail-safe 已朝 off）。
func InitFromEnv() Mode {
	m, known := ResolveMode(os.Getenv(EnvMode))
	if !known {
		log.Printf("[tenant] TENANT_MODE=%q 不是已知值（off|on），已按 off（离线单机）处理", os.Getenv(EnvMode))
	}
	SetMode(m)
	return m
}

// SetMode 显式设置进程级模式（启动装配与测试注入用）
func SetMode(m Mode) { currentMode.Store(m) }

// CurrentMode 生效的部署形态（未初始化时按 off）
func CurrentMode() Mode {
	if v, ok := currentMode.Load().(Mode); ok {
		return v
	}
	return ModeOff
}

// IsMultiTenant 是否处于 SaaS 多租户模式
func IsMultiTenant() bool { return CurrentMode() == ModeOn }

type ctxKey struct{}

type sysKey struct{}

// WithTenantID 将租户绑定进 ctx。
// id<=0 视为无效绑定（0 在数据库列语义中是"平台公共池"，永远不是上下文租户），
// 返回原 ctx 以便链式调用安全。
func WithTenantID(ctx context.Context, id uint) context.Context {
	if id == 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, id)
}

// WithSystemScope 标记系统级操作（migration/备份/健康检查等）。
// 系统作用域是隔离 callback 的唯一豁免通道，只允许装配层与系统入口使用。
func WithSystemScope(ctx context.Context) context.Context {
	return context.WithValue(ctx, sysKey{}, true)
}

// IsSystemScope 当前 ctx 是否为系统级操作
func IsSystemScope(ctx context.Context) bool {
	v, ok := ctx.Value(sysKey{}).(bool)
	return ok && v
}

// FromContext 读取 ctx 绑定的租户。
// ok=false 表示无租户绑定——多租户模式下调用方（GORM callback）必须 fail-closed 拒绝，
// 绝不允许回落到默认租户（那会把漏配变成静默串库）。
func FromContext(ctx context.Context) (uint, bool) {
	if ctx == nil {
		return 0, false
	}
	id, ok := ctx.Value(ctxKey{}).(uint)
	if !ok || id == 0 {
		return 0, false
	}
	return id, true
}

// ContextWithDefault 将默认租户绑定进 ctx。
// 离线模式的中间件与后台扇出退化路径用它——"默认租户"是显式绑定，
// 与"无绑定"严格区分（后者在 on 模式必须报错）。
func ContextWithDefault(ctx context.Context) context.Context {
	return WithTenantID(ctx, DefaultTenantID)
}
