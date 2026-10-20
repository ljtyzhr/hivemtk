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
	"fmt"
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
// 未知值返回 error 且保持 off：拼错的 TENANT_MODE 绝不能静默降级为单机形态
// （那会让 SaaS 部署把所有租户写进默认租户，且没有任何报错）。
func InitFromEnv() (Mode, error) {
	raw := os.Getenv(EnvMode)
	m, known := ResolveMode(raw)
	if !known {
		SetMode(ModeOff)
		return ModeOff, fmt.Errorf("TENANT_MODE=%q 不是已知值（off|on），拒绝以单机形态启动 SaaS 服务", raw)
	}
	SetMode(m)
	return m, nil
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

// detachedCtx 保留父 ctx 的取消与截止时间，但丢弃**全部** value。
type detachedCtx struct{ context.Context }

// Value 恒 nil：连同 SystemScope 与既有租户绑定一并剥掉。
func (detachedCtx) Value(any) any { return nil }

// BackgroundWithTenant 为后台工作派生新 ctx：不继承父 ctx 的取消/截止
// （父 ctx 通常随请求结束而取消），但保留其租户归属——异步写库仍落在本租户。
// 父 ctx 无绑定时：off 模式显式绑定默认租户；on 模式保持无绑定，
// 由隔离 callback 对后续访问 fail-closed（绝不静默回落默认租户）。
func BackgroundWithTenant(ctx context.Context) context.Context {
	bg := context.Background()
	if id, ok := FromContext(ctx); ok {
		return WithTenantID(bg, id)
	}
	if !IsMultiTenant() {
		return WithTenantID(bg, DefaultTenantID)
	}
	return bg
}

// OnlyTenant 构造「只携带指定租户」的 ctx：保留父 ctx 的取消/截止语义，
// 剥离其余全部 value（含 SystemScope 与既有绑定），再显式绑定 id。
//
// 为什么需要它：有些读写必须**钉死**在一个确定的租户上，而调用方 ctx 可能
// 带着 SystemScope —— 隔离 callback 见到 SystemScope 会放行查询（读到的是
// 跨租户 union）且跳过 Create 的 tenant_id 填充（写落列默认值 1），两者都会
// 把别的租户的数据混进本次结果；而直接换 context.Background() 又会丢掉取消
// 语义（后台任务退出时进行中的读不跟着断）。本 helper 两者都不牺牲。
//
// id 无效（0）时按 WithTenantID 原样返回（视为无绑定，on 模式由隔离
// callback fail-closed 兜底，绝不回落默认租户）。
func OnlyTenant(ctx context.Context, id uint) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return WithTenantID(detachedCtx{ctx}, id)
}
