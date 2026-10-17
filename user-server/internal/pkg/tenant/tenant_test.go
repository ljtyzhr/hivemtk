package tenant

import (
	"context"
	"testing"
)

func TestResolveMode(t *testing.T) {
	cases := []struct {
		env    string
		want   Mode
		known  bool
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
	if got := InitFromEnv(); got != ModeOn {
		t.Fatalf("InitFromEnv with on = %v, want %v", got, ModeOn)
	}
	if !IsMultiTenant() {
		t.Fatal("IsMultiTenant should be true after on init")
	}

	t.Setenv(EnvMode, "bogus")
	if got := InitFromEnv(); got != ModeOff {
		t.Fatalf("unknown value must fail-safe to off, got %v", got)
	}

	t.Setenv(EnvMode, "")
	if got := InitFromEnv(); got != ModeOff {
		t.Fatalf("empty env must default to off, got %v", got)
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

func TestFromContextNil(t *testing.T) {
	if _, ok := FromContext(nil); ok {
		t.Fatal("nil ctx must have no tenant")
	}
}
