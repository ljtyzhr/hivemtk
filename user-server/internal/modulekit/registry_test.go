package modulekit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// fakeModule 记录生命周期调用顺序，可按用例注入启停/健康失败。
type fakeModule struct {
	id         string
	desc       Descriptor
	mu         sync.Mutex
	calls      []string
	enableErr  error
	disableErr error
	healthErr  error
}

func newFake(id string, deps ...string) *fakeModule {
	return &fakeModule{id: id, desc: Descriptor{Name: "fake " + id, Version: "v0", DependsOn: deps}}
}

func (f *fakeModule) ID() string                   { return f.id }
func (f *fakeModule) Descriptor() Descriptor       { return f.desc }
func (f *fakeModule) Enable(context.Context) error { f.record("enable:" + f.id); return f.enableErr }
func (f *fakeModule) Disable(context.Context) error {
	f.record("disable:" + f.id)
	return f.disableErr
}
func (f *fakeModule) Health(context.Context) error { f.record("health:" + f.id); return f.healthErr }

func (f *fakeModule) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakeModule) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func TestRegisterDuplicateFailsClosed(t *testing.T) {
	r := NewRegistry()
	m := newFake("dup")

	if err := r.Register(m); err != nil {
		t.Fatalf("首次注册失败: %v", err)
	}
	err := r.Register(newFake("dup"))
	if !errors.Is(err, ErrModuleAlreadyRegistered) {
		t.Fatalf("重复注册应报 ErrModuleAlreadyRegistered，实际: %v", err)
	}
	if got := len(r.List()); got != 1 {
		t.Fatalf("重复注册后表内模块数应为 1，实际 %d", got)
	}
	if r.State("dup") != StateDisabled {
		t.Fatalf("重复注册不应改变既有状态，实际 %s", r.State("dup"))
	}
}

func TestRegisterInvalid(t *testing.T) {
	tests := []struct {
		name string
		m    Module
		want error
	}{
		{name: "nil 模块", m: nil, want: ErrModuleInvalid},
		{name: "空 ID", m: newFake(""), want: ErrModuleInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			err := r.Register(tt.m)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Register() err=%v，期望包含 %v", err, tt.want)
			}
			if len(r.List()) != 0 {
				t.Fatalf("非法注册不得留痕，List()=%v", r.List())
			}
		})
	}
}

func TestGetUnknownFailsClosed(t *testing.T) {
	r := NewRegistry()
	got, err := r.Get("nope")
	if !errors.Is(err, ErrModuleNotRegistered) {
		t.Fatalf("查未注册应报 ErrModuleNotRegistered，实际: %v", err)
	}
	if got != nil {
		t.Fatalf("查未注册不得返回模块，实际: %#v", got)
	}
}

func TestLifecycleOrder(t *testing.T) {
	r := NewRegistry()
	a := newFake("a")
	b := newFake("b")
	if err := r.Register(a); err != nil {
		t.Fatalf("注册 a: %v", err)
	}
	if err := r.Register(b); err != nil {
		t.Fatalf("注册 b: %v", err)
	}
	ctx := context.Background()

	// 未启用的模块健康检查必须被注册表拦下（不调用模块 Health）。
	if err := r.CheckHealth(ctx, "a"); !errors.Is(err, ErrModuleNotEnabled) {
		t.Fatalf("未启用健康检查应报 ErrModuleNotEnabled，实际: %v", err)
	}

	if err := r.EnableAll(ctx); err != nil {
		t.Fatalf("EnableAll: %v", err)
	}
	wantEnable := []string{"enable:a", "enable:b"}
	if got := a.callsSnapshot(); !equalStrings(got, wantEnable[:1]) {
		t.Fatalf("a 调用轨迹 = %v，期望 %v", got, wantEnable[:1])
	}
	if got := b.callsSnapshot(); !equalStrings(got, wantEnable[1:]) {
		t.Fatalf("b 调用轨迹 = %v，期望 %v", got, wantEnable[1:])
	}
	if r.State("a") != StateEnabled || r.State("b") != StateEnabled {
		t.Fatalf("启用后状态应为 enabled，实际 a=%s b=%s", r.State("a"), r.State("b"))
	}

	if err := r.CheckHealth(ctx, "a"); err != nil {
		t.Fatalf("已启用健康检查应通过: %v", err)
	}
	if got := a.callsSnapshot(); !equalStrings(got, []string{"enable:a", "health:a"}) {
		t.Fatalf("a 调用轨迹 = %v，期望 enable 先于 health", got)
	}

	// DisableAll 按注册逆序：依赖者 b 先停，a 后停。
	if err := r.DisableAll(ctx); err != nil {
		t.Fatalf("DisableAll: %v", err)
	}
	if got := a.callsSnapshot(); !equalStrings(got, []string{"enable:a", "health:a", "disable:a"}) {
		t.Fatalf("a 调用轨迹 = %v，期望 disable 在最后", got)
	}
	if r.State("a") != StateDisabled || r.State("b") != StateDisabled {
		t.Fatalf("停用后状态应为 disabled，实际 a=%s b=%s", r.State("a"), r.State("b"))
	}
}

func TestEnableDependencyMissing(t *testing.T) {
	tests := []struct {
		name    string
		dep     string
		preReg  *fakeModule // 可选：把依赖注册但保持 disabled
		wantErr error
	}{
		{name: "依赖未注册", dep: "ghost", wantErr: ErrModuleDependencyMissing},
		{name: "依赖已注册未启用", dep: "lazy", preReg: newFake("lazy"), wantErr: ErrModuleDependencyMissing},
		{name: "依赖自身", dep: "self", wantErr: ErrModuleDependencyMissing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			if tt.preReg != nil {
				_ = r.Register(tt.preReg)
			}
			m := newFake(tt.name, tt.dep)
			if err := r.Register(m); err != nil {
				t.Fatalf("Register: %v", err)
			}
			err := r.Enable(context.Background(), tt.name)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Enable() err=%v，期望包含 %v", err, tt.wantErr)
			}
			if r.State(tt.name) != StateDisabled {
				t.Fatalf("依赖未就绪时状态应保持 disabled，实际 %s", r.State(tt.name))
			}
		})
	}
}

func TestDisableWithEnabledDependents(t *testing.T) {
	r := NewRegistry()
	base := newFake("base")
	top := newFake("top", "base")
	if err := r.Register(base); err != nil {
		t.Fatalf("注册 base: %v", err)
	}
	if err := r.Register(top); err != nil {
		t.Fatalf("注册 top: %v", err)
	}
	ctx := context.Background()
	if err := r.EnableAll(ctx); err != nil {
		t.Fatalf("EnableAll: %v", err)
	}

	err := r.Disable(ctx, "base")
	if !errors.Is(err, ErrModuleHasDependents) {
		t.Fatalf("停用被依赖的 base 应报 ErrModuleHasDependents，实际: %v", err)
	}
	if r.State("base") != StateEnabled {
		t.Fatalf("被拒停用后 base 应保持 enabled，实际 %s", r.State("base"))
	}

	// 依赖者先停，再停 base 必须成功。
	if err := r.Disable(ctx, "top"); err != nil {
		t.Fatalf("停用 top: %v", err)
	}
	if err := r.Disable(ctx, "base"); err != nil {
		t.Fatalf("top 停用后 base 应可停用: %v", err)
	}
}

func TestEnableDisableIdempotent(t *testing.T) {
	r := NewRegistry()
	m := newFake("idem")
	_ = r.Register(m)
	ctx := context.Background()

	if err := r.Enable(ctx, "idem"); err != nil {
		t.Fatalf("首次 Enable: %v", err)
	}
	if err := r.Enable(ctx, "idem"); err != nil {
		t.Fatalf("重复 Enable 应幂等返回 nil，实际: %v", err)
	}
	if got := m.callsSnapshot(); !equalStrings(got, []string{"enable:idem"}) {
		t.Fatalf("幂等 Enable 不得重复调用模块，轨迹 = %v", got)
	}

	if err := r.Disable(ctx, "idem"); err != nil {
		t.Fatalf("首次 Disable: %v", err)
	}
	if err := r.Disable(ctx, "idem"); err != nil {
		t.Fatalf("重复 Disable 应幂等返回 nil，实际: %v", err)
	}
	if got := m.callsSnapshot(); !equalStrings(got, []string{"enable:idem", "disable:idem"}) {
		t.Fatalf("幂等 Disable 不得重复调用模块，轨迹 = %v", got)
	}
}

func TestEnableAllStopsOnFirstError(t *testing.T) {
	r := NewRegistry()
	a := newFake("a")
	b := newFake("b")
	c := newFake("c")
	boom := errors.New("boom")
	b.enableErr = boom
	for _, m := range []Module{a, b, c} {
		if err := r.Register(m); err != nil {
			t.Fatalf("Register: %v", err)
		}
	}

	err := r.EnableAll(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("EnableAll 应透传 b 的启用失败，err=%v", err)
	}
	if r.State("a") != StateEnabled {
		t.Fatalf("a（先于 b）应已启用，实际 %s", r.State("a"))
	}
	if r.State("b") != StateDisabled {
		t.Fatalf("b 启用失败应保持 disabled，实际 %s", r.State("b"))
	}
	if r.State("c") != StateDisabled {
		t.Fatalf("c（后于 b）不应被启用，实际 %s", r.State("c"))
	}
}

func TestRegisterFromLookup(t *testing.T) {
	tests := []struct {
		name string
		sym  any
		want error
	}{
		{name: "符号是 Module", sym: newFake("ok"), want: nil},
		{name: "符号是字符串", sym: "not-a-module", want: ErrPluginSymbolType},
		{name: "符号是数字", sym: 42, want: ErrPluginSymbolType},
		{name: "符号是 nil", sym: nil, want: ErrPluginSymbolType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRegistry()
			got, err := r.RegisterFromLookup(tt.sym)
			if tt.want == nil {
				if err != nil {
					t.Fatalf("RegisterFromLookup() err=%v，期望成功", err)
				}
				if got == nil {
					t.Fatalf("成功路径应返回模块")
				}
				if !r.Has("ok") {
					t.Fatalf("成功路径应完成登记")
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Fatalf("RegisterFromLookup() err=%v，期望包含 %v", err, tt.want)
			}
			if got != nil {
				t.Fatalf("失败路径不得返回模块，实际 %#v", got)
			}
		})
	}
}

func TestBuiltinModulesBootstrap(t *testing.T) {
	r := NewRegistry()
	for _, m := range BuiltinModules() {
		if err := r.Register(m); err != nil {
			t.Fatalf("Register(%s): %v", m.ID(), err)
		}
	}
	if got := len(r.List()); got != 2 {
		t.Fatalf("内建模块应有两个，实际 %d", got)
	}
	// 认证占位依赖健康模块，EnableAll 必须按依赖顺序放行。
	if err := r.EnableAll(context.Background()); err != nil {
		t.Fatalf("EnableAll: %v", err)
	}
	for _, id := range []string{"builtin.health", "builtin.auth"} {
		if err := r.CheckHealth(context.Background(), id); err != nil {
			t.Fatalf("CheckHealth(%s): %v", id, err)
		}
	}
	// 认证占位仍被启用时，健康模块不可停。
	if err := r.Disable(context.Background(), "builtin.health"); !errors.Is(err, ErrModuleHasDependents) {
		t.Fatalf("停用被依赖的内建健康模块应报 ErrModuleHasDependents，实际: %v", err)
	}
	if err := r.DisableAll(context.Background()); err != nil {
		t.Fatalf("DisableAll: %v", err)
	}
}

// TestConcurrentReadSafe 并发读（Get/List/State/Has）与并发写（Register）同时进行，
// 配 `go test -race` 跑；数据面只做弱断言（不 panic、Has 与 Register 自洽）。
func TestConcurrentReadSafe(t *testing.T) {
	r := NewRegistry()
	const total = 64

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < total; j++ {
				m := newFake(fmt.Sprintf("m%d", j))
				_ = r.Register(m)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = r.List()
				_, _ = r.Get("m0")
				_ = r.State("m0")
				_ = r.Has("m63")
			}
		}()
	}
	wg.Wait()

	// 同 ID 并发注册：恰好一个成功，其余 ErrModuleAlreadyRegistered（fail-closed）。
	for _, id := range []string{"m0", "m1", "m2"} {
		if !r.Has(id) {
			t.Fatalf("%s 应已注册（并发写后至少一个 goroutine 成功）", id)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
