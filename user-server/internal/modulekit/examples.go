package modulekit

import "context"

// NoopHealthModule 是一个 no-op 健康模块：启用/停用只是翻转状态，
// 健康检查永远通过。用于演示注册表的最小可用闭环，也作依赖树的根。
//
// 为什么不给它加任何真实逻辑：它的职责是「示范契约形状 + 给依赖树一个恒健康的根」。
// 真实健康能力（进程/依赖/磁盘等探针）属于各自业务域，不在本包的边界内。
type NoopHealthModule struct {
	id string
}

// NewNoopHealthModule 构造 no-op 健康模块。
func NewNoopHealthModule() *NoopHealthModule {
	return &NoopHealthModule{id: "builtin.health"}
}

// ID 返回模块唯一标识。
func (m *NoopHealthModule) ID() string { return m.id }

// Descriptor 返回能力声明。
func (m *NoopHealthModule) Descriptor() Descriptor {
	return Descriptor{
		Name:         "内建健康占位",
		Version:      "v0.1.0",
		Capabilities: []string{"health.ping"},
	}
}

// Enable 激活模块（幂等）。
func (m *NoopHealthModule) Enable(context.Context) error { return nil }

// Disable 停用模块（幂等）。
func (m *NoopHealthModule) Disable(context.Context) error { return nil }

// Health 健康检查：恒健康。
func (m *NoopHealthModule) Health(context.Context) error { return nil }

// AuthPlaceholderModule 认证相关占位模块。
//
// 它对应的是现状里那份 auth_plugin.so（导出符号 AuthPlugin，方法含
// CheckAuthorization / RefreshAuthorization / GetFirstAccessTime）想占据的位置：
// 「认证 / 授权 / License」能力点。本占位只示范「真实认证插件未来挂哪个 ID、
// 声明什么能力、依赖什么」，不实现任何认证逻辑——真实迁移留待后续（见 delivery.md）。
type AuthPlaceholderModule struct{}

// ID 返回模块唯一标识。
func (m *AuthPlaceholderModule) ID() string { return "builtin.auth" }

// Descriptor 返回能力声明：依赖内建健康模块，演示注册表的依赖校验。
func (m *AuthPlaceholderModule) Descriptor() Descriptor {
	return Descriptor{
		Name:         "认证占位",
		Version:      "v0.1.0",
		DependsOn:    []string{"builtin.health"},
		Capabilities: []string{"auth.check", "auth.license"},
	}
}

// Enable 激活模块（幂等）。
func (m *AuthPlaceholderModule) Enable(context.Context) error { return nil }

// Disable 停用模块（幂等）。
func (m *AuthPlaceholderModule) Disable(context.Context) error { return nil }

// Health 健康检查：占位恒健康。
func (m *AuthPlaceholderModule) Health(context.Context) error { return nil }

// BuiltinModules 返回本包内建示例模块，按依赖顺序排列（health 先于 auth）。
//
// 装配期可这样接线：
//
//	r := modulekit.NewRegistry()
//	for _, m := range modulekit.BuiltinModules() {
//	    if err := r.Register(m); err != nil { ... }
//	}
func BuiltinModules() []Module {
	return []Module{NewNoopHealthModule(), &AuthPlaceholderModule{}}
}
