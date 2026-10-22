package tenant

import (
	"context"
	"testing"
)

func TestResolveMode(t *testing.T) {
	cases := []struct {
		env   string
		want  Mode
		known bool
	}{
		{"", ModeOff, true},
		{"off", ModeOff, true},
		{"OFF", ModeOff, true},
		{"  off  ", ModeOff, true},
		{"on", ModeOn, true},
		{"ON", ModeOn, true},
		{" On ", ModeOn, true},
		{"bogus", ModeOff, false},
		{"1", ModeOff, false},
	}
	for _, c := range cases {
		got, known := ResolveMode(c.env)
		if got != c.want || known != c.known {
			t.Errorf("ResolveMode(%q) = (%v,%v), want (%v,%v)", c.env, got, known, c.want, c.known)
		}
	}
}

func TestInitFromEnv(t *testing.T) {
	t.Setenv(EnvMode, "on")
	if got, err := InitFromEnv(); err != nil || got != ModeOn {
		t.Fatalf("InitFromEnv with on = (%v,%v), want (%v,nil)", got, err, ModeOn)
	}
	if !IsMultiTenant() {
		t.Fatal("IsMultiTenant should be true after on init")
	}

	// 未知值必须报错并保持 off：静默降级会让 SaaS 以单机形态读写同一租户
	t.Setenv(EnvMode, "bogus")
	if got, err := InitFromEnv(); err == nil || got != ModeOff {
		t.Fatalf("unknown value must fail with off mode, got (%v,%v)", got, err)
	}
	if IsMultiTenant() {
		t.Fatal("unknown value must not enable multi-tenant mode")
	}

	t.Setenv(EnvMode, "")
	if got, err := InitFromEnv(); err != nil || got != ModeOff {
		t.Fatalf("empty env must default to off, got (%v,%v)", got, err)
	}
	if IsMultiTenant() {
		t.Fatal("IsMultiTenant should be false in off mode")
	}
}

func TestSetModeAndCurrentModeDefault(t *testing.T) {
	SetMode(ModeOn)
	if CurrentMode() != ModeOn {
		t.Fatal("CurrentMode should echo SetMode")
	}
	t.Cleanup(func() { SetMode(ModeOff) })
}

func TestContextRoundTrip(t *testing.T) {
	ctx := context.Background()

	if _, ok := FromContext(ctx); ok {
		t.Fatal("plain ctx must have no tenant")
	}

	bound := WithTenantID(ctx, 42)
	got, ok := FromContext(bound)
	if !ok || got != 42 {
		t.Fatalf("FromContext = (%v,%v), want (42,true)", got, ok)
	}

	// 0/负值不是合法租户绑定：返回原 ctx，无租户
	if WithTenantID(ctx, 0) != ctx {
		t.Fatal("WithTenantID(0) must return original ctx")
	}
	if _, ok := FromContext(WithTenantID(ctx, 0)); ok {
		t.Fatal("tenant 0 must not be bindable")
	}
}

func TestSystemScope(t *testing.T) {
	ctx := context.Background()
	if IsSystemScope(ctx) {
		t.Fatal("plain ctx must not be system scope")
	}
	if !IsSystemScope(WithSystemScope(ctx)) {
		t.Fatal("WithSystemScope must be detectable")
	}
	// 系统作用域与租户绑定正交
	sys := WithSystemScope(WithTenantID(ctx, 7))
	id, ok := FromContext(sys)
	if !ok || id != 7 {
		t.Fatalf("system scope must preserve tenant binding, got (%v,%v)", id, ok)
	}
}

func TestContextWithDefault(t *testing.T) {
	got, ok := FromContext(ContextWithDefault(context.Background()))
	if !ok || got != DefaultTenantID {
		t.Fatalf("ContextWithDefault = (%v,%v), want (%v,true)", got, ok, DefaultTenantID)
	}
}

// TestBackgroundWithTenantPreservesSystemScope BackgroundWithTenant 必须连
// SystemScope 一起搬过去。
//
// 回归背景（2026-10-22 线上）：平台级后台任务（延迟出站派发
// dispatchDueDelayedOutbound）拿的是 tenant.WithSystemScope(ctx) —— 那条豁免通道
// 是它们写租户表的唯一凭据。BackgroundWithTenant 此前只搬租户、把 SystemScope 丢了，
// 派生出来的 ctx 变成"无租户、无豁免"，隔离 callback 一律 fail-closed 拒绝，
// 现场是 "[SSE] 出站认领报错，放弃本次推送"（认领要 UPDATE message_hub）。
//
// 丢豁免比不丢更坏：调用方以为豁免还在，写库全被拒。
func TestBackgroundWithTenantPreservesSystemScope(t *testing.T) {
	prev := CurrentMode()
	SetMode(ModeOn)
	t.Cleanup(func() { SetMode(prev) })

	// 1) 父 ctx 带租户 + 系统作用域 ⇒ 两者都保留
	base := WithSystemScope(WithTenantID(context.Background(), 7))
	got := BackgroundWithTenant(base)
	if id, ok := FromContext(got); !ok || id != 7 {
		t.Errorf("租户绑定未保留：(%v,%v), want (7,true)", id, ok)
	}
	if !IsSystemScope(got) {
		t.Error("SystemScope 未保留：平台级后台任务派生 ctx 后失去豁免，写租户表会被 fail-closed 拒绝")
	}

	// 2) 父 ctx 只有系统作用域（无租户绑定）⇒ 豁免仍须保留，租户保持无绑定
	//    （on 模式下绝不回落默认租户）
	sysOnly := WithSystemScope(context.Background())
	got2 := BackgroundWithTenant(sysOnly)
	if !IsSystemScope(got2) {
		t.Error("父 ctx 仅带 SystemScope 时，派生 ctx 必须保留豁免")
	}
	if id, ok := FromContext(got2); ok {
		t.Errorf("on 模式下无租户绑定的父 ctx 派生后不得回落默认租户，实际绑到 %d", id)
	}

	// 3) 普通 ctx ⇒ 派生后既无租户也无豁免
	plain := BackgroundWithTenant(context.Background())
	if IsSystemScope(plain) {
		t.Error("普通 ctx 派生后不应带 SystemScope")
	}
	if _, ok := FromContext(plain); ok {
		t.Error("on 模式下普通 ctx 派生后不得绑定默认租户")
	}
}

func TestFromContextNil(t *testing.T) {
	if _, ok := FromContext(nil); ok {
		t.Fatal("nil ctx must have no tenant")
	}
}

// TestOnlyTenantStripsValuesKeepsCancel OnlyTenant：剥离全部 value（含
// SystemScope 与既有绑定），保留取消语义 —— 闸/激活把读写钉在确定租户上时
// 依赖这两点（SystemScope 会让隔离 callback 放行 → 读变跨租户 union、
// 写落列默认值 1 = 串租户）。
func TestOnlyTenantStripsValuesKeepsCancel(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	sys := WithSystemScope(WithTenantID(base, 7))

	only := OnlyTenant(sys, 9)
	if id, ok := FromContext(only); !ok || id != 9 {
		t.Fatalf("OnlyTenant 应只保留绑定 9，实际 ok=%v id=%d", ok, id)
	}
	if IsSystemScope(only) {
		t.Fatal("OnlyTenant 必须剥离 SystemScope")
	}

	// 取消必须穿透（同步循环退出时进行中的读要跟着断）
	cancel()
	select {
	case <-only.Done():
	default:
		t.Fatal("父 ctx 的取消未穿透 OnlyTenant")
	}

	// 无效 id → 无绑定（on 模式由隔离 callback fail-closed 兜底）
	if id, ok := FromContext(OnlyTenant(context.Background(), 0)); ok {
		t.Fatalf("id=0 应视为无绑定，实际 id=%d", id)
	}
}
