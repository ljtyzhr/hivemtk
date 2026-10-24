package modulekit

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// 哨兵错误：调用方一律用 errors.Is 判定，禁止字符串匹配。
var (
	// ErrModuleInvalid 注册的模块不满足契约（nil、ID 为空）。
	ErrModuleInvalid = errors.New("modulekit: 模块不满足契约")
	// ErrModuleAlreadyRegistered 重复注册：同 ID 第二次 Register 必错（fail-closed）。
	ErrModuleAlreadyRegistered = errors.New("modulekit: 模块已注册")
	// ErrModuleNotRegistered 查询/启停/健康检查了未注册的 ID（fail-closed，不返回 nil）。
	ErrModuleNotRegistered = errors.New("modulekit: 模块未注册")
	// ErrModuleDependencyMissing Enable 时声明的依赖未启用（或未注册、或依赖自身）。
	ErrModuleDependencyMissing = errors.New("modulekit: 依赖模块未启用")
	// ErrModuleHasDependents Disable 时仍有已启用模块声明依赖它（fail-closed：不拆承重墙）。
	ErrModuleHasDependents = errors.New("modulekit: 仍有已启用模块依赖它")
	// ErrModuleNotEnabled 对未启用的模块做健康检查。
	ErrModuleNotEnabled = errors.New("modulekit: 模块未启用")
	// ErrPluginSymbolType plugin 查到的符号不是 Module 类型（宿主与插件包路径不一致等）。
	ErrPluginSymbolType = errors.New("modulekit: 插件符号类型不符")
)

// Registry 进程内模块注册表。
//
// # 并发语义
//
// 读（Get/List/State/Has）只取 RLock，可任意并发；Register/Enable/Disable 改状态时取
// Lock。模块方法（Enable/Disable/Health）一律在锁外调用——模块方法可能反向调用注册表，
// 持锁调用会死锁。同一模块的 Enable/Disable 用 per-entry 互斥锁串行化，避免并发启停
// 把状态踩坏。
//
// # fail-closed 总纲
//
// 凡是「拿不到/对不上」的，一律返回带哨兵的错误，绝不返回零值或静默放行：
// 重复注册报错、查未注册报错、启停未注册报错、依赖未启用报错、启停有依赖者报错。
type Registry struct {
	mu   sync.RWMutex
	seq  []*entry          // 注册顺序：保证 List/EnableAll/DisableAll 输出稳定
	byID map[string]*entry // ID → entry；判重与查找的唯一索引
}

type entry struct {
	module Module
	desc   Descriptor // Register 时快照，之后不重读模块方法
	state  ModuleState
	mu     sync.Mutex // 串行化同一模块的 Enable/Disable
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{byID: make(map[string]*entry)}
}

// Register 登记一个模块。失败（重复/契约不符）不产生任何副作用。
func (r *Registry) Register(m Module) error {
	if m == nil {
		return fmt.Errorf("%w: 传入 nil 模块", ErrModuleInvalid)
	}
	id := m.ID()
	if id == "" {
		return fmt.Errorf("%w: ID 为空", ErrModuleInvalid)
	}
	desc := m.Descriptor()

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; ok {
		return fmt.Errorf("%w: %q", ErrModuleAlreadyRegistered, id)
	}
	r.byID[id] = &entry{module: m, desc: desc, state: StateDisabled}
	r.seq = append(r.seq, r.byID[id])
	return nil
}

// RegisterFromLookup 把 Go plugin 查到的符号断言为 Module 并登记。
//
// sym 来自 plugin.Open 之后的 plugin.Lookup(PluginSymbol)。类型对不上
// （宿主与插件包路径不一致、或符号根本不是 Module）→ ErrPluginSymbolType。
func (r *Registry) RegisterFromLookup(sym any) (Module, error) {
	m, ok := sym.(Module)
	if !ok {
		return nil, fmt.Errorf("%w: 符号 %q 实际类型 %T，宿主与插件必须导入同一份 modulekit",
			ErrPluginSymbolType, PluginSymbol, sym)
	}
	if err := r.Register(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Get 返回已注册模块；未注册返回 ErrModuleNotRegistered（fail-closed，不返回 nil）。
func (r *Registry) Get(id string) (Module, error) {
	e, err := r.entry(id)
	if err != nil {
		return nil, err
	}
	return e.module, nil
}

// Has 返回模块是否已注册。
func (r *Registry) Has(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.byID[id]
	return ok
}

// List 返回按注册顺序排列的模块快照（数组是拷贝，可安全持有）。
func (r *Registry) List() []Module {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Module, 0, len(r.seq))
	for _, e := range r.seq {
		out = append(out, e.module)
	}
	return out
}

// Descriptor 返回模块在 Register 时快照的能力声明；未注册返回 ErrModuleNotRegistered。
func (r *Registry) Descriptor(id string) (Descriptor, error) {
	e, err := r.entry(id)
	if err != nil {
		return Descriptor{}, err
	}
	return e.desc, nil
}

// State 返回模块生命周期状态；未注册返回 StateUnknown。
func (r *Registry) State(id string) ModuleState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	if !ok {
		return StateUnknown
	}
	return e.state
}

// Enable 启用模块：
//   - 未注册 → ErrModuleNotRegistered
//   - 依赖未启用 → ErrModuleDependencyMissing
//   - 已启用 → 幂等返回 nil
//   - 模块 Enable 失败 → 包装原错误返回，状态保持 disabled
func (r *Registry) Enable(ctx context.Context, id string) error {
	e, err := r.entry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	already, err := r.ensureEnableReadyLocked(e)
	if err != nil {
		return err
	}
	if already {
		return nil // 幂等：已启用
	}
	if err := e.module.Enable(ctx); err != nil {
		return fmt.Errorf("modulekit: 启用 %q 失败: %w", id, err)
	}
	r.mu.Lock()
	e.state = StateEnabled
	r.mu.Unlock()
	return nil
}

// Disable 停用模块：
//   - 未注册 → ErrModuleNotRegistered
//   - 仍有已启用模块依赖它 → ErrModuleHasDependents
//   - 已停用 → 幂等返回 nil
//   - 模块 Disable 失败 → 包装原错误返回，状态保持 enabled
func (r *Registry) Disable(ctx context.Context, id string) error {
	e, err := r.entry(id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	r.mu.Lock()
	if e.state == StateDisabled {
		r.mu.Unlock()
		return nil
	}
	for _, other := range r.seq {
		if other == e || other.state != StateEnabled {
			continue
		}
		for _, dep := range other.desc.DependsOn {
			if dep == id {
				r.mu.Unlock()
				return fmt.Errorf("%w: %q 仍被 %q 依赖", ErrModuleHasDependents, id, other.module.ID())
			}
		}
	}
	r.mu.Unlock()

	if err := e.module.Disable(ctx); err != nil {
		return fmt.Errorf("modulekit: 停用 %q 失败: %w", id, err)
	}
	r.mu.Lock()
	e.state = StateDisabled
	r.mu.Unlock()
	return nil
}

// CheckHealth 返回模块健康状态：
//   - 未注册 → ErrModuleNotRegistered
//   - 已注册未启用 → ErrModuleNotEnabled
//   - 已启用 → 调用模块 Health(ctx)（锁外调用）
func (r *Registry) CheckHealth(ctx context.Context, id string) error {
	e, err := r.entry(id)
	if err != nil {
		return err
	}
	r.mu.RLock()
	state := e.state
	r.mu.RUnlock()
	if state != StateEnabled {
		return fmt.Errorf("%w: %q 当前状态 %s", ErrModuleNotEnabled, id, state)
	}
	return e.module.Health(ctx)
}

// EnableAll 按注册顺序启用全部模块；任一失败即停并返回该错误（fail-closed，
// 不让半启用的集合被当成「全活了」）。
func (r *Registry) EnableAll(ctx context.Context) error {
	for _, id := range r.idsSnapshot() {
		if err := r.Enable(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// DisableAll 按注册逆序停用全部模块（依赖者先停）；个别失败继续停其余的，
// 返回 errors.Join 聚合的错误（shutdown 尽力而为，不做一半停手）。
//
// 前提：注册顺序满足依赖在前（BuiltinModules 即如此），逆序即「依赖者先停」。
func (r *Registry) DisableAll(ctx context.Context) error {
	ids := r.idsSnapshot()
	var errs []error
	for i := len(ids) - 1; i >= 0; i-- {
		if err := r.Disable(ctx, ids[i]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// entry 返回 ID 对应的 entry；未注册返回 ErrModuleNotRegistered。
func (r *Registry) entry(id string) (*entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrModuleNotRegistered, id)
	}
	return e, nil
}

// idsSnapshot 返回按注册顺序排列的 ID 快照。
func (r *Registry) idsSnapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.seq))
	for _, e := range r.seq {
		ids = append(ids, e.module.ID())
	}
	return ids
}

// ensureEnableReadyLocked 在持有 entry.mu 的前提下，复核幂等与依赖。
// 返回 already=true 表示模块已启用、应跳过模块调用（幂等）。
// 锁顺序恒为 entry.mu → Registry.mu，与 Disable 一致，避免死锁。
func (r *Registry) ensureEnableReadyLocked(e *entry) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.state == StateEnabled {
		return true, nil
	}
	for _, dep := range e.desc.DependsOn {
		de, ok := r.byID[dep]
		if !ok || de.state != StateEnabled {
			return false, fmt.Errorf("%w: %q 依赖 %q 未启用", ErrModuleDependencyMissing, e.module.ID(), dep)
		}
	}
	return false, nil
}
