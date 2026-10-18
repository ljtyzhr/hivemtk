package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/utils/logger"
)

// ============================================================
// 功能模块闸（模块化改造的核心）
//
// ===== 执行顺序（三条，顺序不可换）=====
//   登录态 → **模块闸** → 角色权限
//
// 为什么在角色权限**之前**：超管是「在本系统内权限最高」，不是「买了所有模块」。
// 闸放在权限之后，超管就会成为绕过付费模块的后门 —— 而这条后门一旦存在，
// 商业化就没有意义了。
//
// ===== 三态语义（B-2 修订）=====
//   ① 模块数据为空（首启 / 从未同步过）
//      → **全放行** + WARN 一次。判据：此时若拦，用户会看到空系统且无从自救
//        （他还没开通过任何东西，但连基础模块都不给用）。
//   ② 有数据且命中模块 → 过闸
//   ③ 有数据但未命中任何模块（新增接口忘了标注）
//      → **放行 + 记 WARN + 进 /api/modules/uncovered-routes 巡检清单**。
//        这里选「放行」而不是「拦」是刻意的：新功能上线时全站 403 是灾难，
//        而漏拦最多让某个接口多暴露一点，两害相权的取舍写在 tasks.md B-2。
//        真正的兜底是 CI 里的覆盖率测试。
// ============================================================

// ModuleGateService 模块闸
type ModuleGateService struct {
	repo repository.TenantModuleRepository

	mu       sync.RWMutex
	cache    map[string]bool // module_key → enabled
	loadedAt time.Time
	// bootstrapped 标记「本地是否有可用的模块数据」
	bootstrapped bool
}

// gateCacheTTL 模块状态本地缓存有效期。
// 取 60s：模块开通是一个「低频但要尽快生效」的操作 —— 平台确认后用户在
// 一分钟内看到菜单变化是可接受的，而每次请求都查库会让模块闸变成热点。
const gateCacheTTL = 60 * time.Second

// NewModuleGateService 构造模块闸服务
func NewModuleGateService() *ModuleGateService {
	return &ModuleGateService{repo: repository.NewTenantModuleRepository()}
}

// NewModuleGateServiceWithRepo 带显式仓储（测试注入）
func NewModuleGateServiceWithRepo(repo repository.TenantModuleRepository) *ModuleGateService {
	return &ModuleGateService{repo: repo}
}

// Refresh 从仓储重载模块状态到缓存
func (s *ModuleGateService) Refresh(ctx context.Context) error {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		m[r.ModuleKey] = model.TenantModuleUsable(r, now)
	}
	s.mu.Lock()
	s.cache = m
	s.bootstrapped = len(rows) > 0
	s.loadedAt = now
	s.mu.Unlock()
	return nil
}

// Invalidate 清缓存（下单成功后 / 手动同步后调用）
func (s *ModuleGateService) Invalidate() {
	s.mu.Lock()
	s.loadedAt = time.Time{}
	s.mu.Unlock()
}

// IsEnabled 模块是否已开通。
//
// 这是**唯一**的判定入口：菜单过滤、接口闸、cron/工具装配全部走它，
// 避免出现「菜单过滤用了 A 口径、接口闸用了 B 口径」的不一致。
func (s *ModuleGateService) IsEnabled(ctx context.Context, moduleKey string) bool {
	if moduleKey == "" {
		return true // 未标注的路由：放行（见文件头 ③）
	}
	s.mu.RLock()
	fresh := time.Since(s.loadedAt) < gateCacheTTL
	m := s.cache
	boot := s.bootstrapped
	s.mu.RUnlock()

	if !fresh {
		if err := s.Refresh(ctx); err != nil {
			logger.Warnf("[module-gate] 刷新模块状态失败，按上一次缓存判定: %v", err)
		}
		s.mu.RLock()
		m = s.cache
		boot = s.bootstrapped
		s.mu.RUnlock()
	}

	if !boot {
		// ① 从未同步过：全放行。首次启动 / 离线部署都走这里。
		//
		// on 模式的边界（有意为之，勿"顺手修"）：镜像按租户归属，而平台侧是
		// 单商户单部署，同步只灌默认租户（tenant_id=1）——租户 2+ 的
		// tenant_modules 恒空，于是也落进这里（allow-all）。"每个租户各自
		// bootstrap"是产品决策，不是本次 P1 的范围。
		// 附注：缓存是进程级单份（gate 只有一份 cache），tenant 2+ 的一次刷新
		// 会把 bootstrapped 置回 false 直到下一次 TTL 过期 —— 同样属该决策范畴。
		return true
	}
	v, ok := m[moduleKey]
	if !ok {
		// ② 该模块在本地镜像里没有记录 —— 平台还没下发它（或已被撤销）
		return false
	}
	return v
}

// EnabledKeys 已开通模块 key 列表（菜单过滤用）
func (s *ModuleGateService) EnabledKeys(ctx context.Context) map[string]struct{} {
	s.mu.RLock()
	m := s.cache
	fresh := time.Since(s.loadedAt) < gateCacheTTL
	boot := s.bootstrapped
	s.mu.RUnlock()

	if !fresh {
		_ = s.Refresh(ctx)
		s.mu.RLock()
		m = s.cache
		boot = s.bootstrapped
		s.mu.RUnlock()
	}
	out := make(map[string]struct{}, len(m))
	if !boot {
		// 首启：全部基础模块 + 空收费模块
		for _, k := range model.BaseModuleKeys() {
			out[k] = struct{}{}
		}
		return out
	}
	for k, v := range m {
		if v {
			out[k] = struct{}{}
		}
	}
	return out
}

// ModuleEnabledFunc 返回可注入 model.FilterMenuTreeByModule 的判定函数
func (s *ModuleGateService) ModuleEnabledFunc() func(string) bool {
	// 菜单过滤是进程级快照（全局单口径，不属于任何请求），装配/同步时取一次
	// 就固化进闭包。on 模式下裸 Background 读租户镜像会 fail-closed —— 过滤器
	// 拿不到数据 → 整棵菜单按"首启"渲染，且这里没有调用方能感知错误。
	// 挂 SystemScope 直读；off 模式直通，行为逐字节不变。
	keys := s.EnabledKeys(tenant.WithSystemScope(context.Background()))
	return func(moduleKey string) bool {
		if moduleKey == "" {
			return true
		}
		_, ok := keys[moduleKey]
		return ok
	}
}

// InstallWiring 把模块闸接到菜单过滤上（装配期调用一次）
func (s *ModuleGateService) InstallWiring(ctx context.Context) {
	model.SetModuleEnabledFilter(s.ModuleEnabledFunc())
}

// ---------------------------------------------------------- 同步

// ModuleCatalogService 模块下发与市场服务
type ModuleCatalogService struct {
	repo  repository.TenantModuleRepository
	order repository.ModuleOrderRepository
}

// NewModuleCatalogService 构造
func NewModuleCatalogService() *ModuleCatalogService {
	return &ModuleCatalogService{
		repo:  repository.NewTenantModuleRepository(),
		order: repository.NewModuleOrderRepository(),
	}
}

// NewModuleCatalogServiceWithRepos 带显式仓储（测试注入）
func NewModuleCatalogServiceWithRepos(repo repository.TenantModuleRepository, orderRepo repository.ModuleOrderRepository) *ModuleCatalogService {
	return &ModuleCatalogService{repo: repo, order: orderRepo}
}

// ModuleSyncResult 同步结果
type ModuleSyncResult struct {
	ActiveCount int    `json:"active_count"`
	Source      string `json:"source"` // platform | offline_bootstrap | cache
	Degraded    bool   `json:"degraded"`
	Message     string `json:"message"`
}

// Sync 从平台拉取模块快照并落本地镜像。
//
// 离线降级（PLATFORM_ENABLED=false）：
//   - 本地没有任何模块行 → 灌入**基础模块**（免费常开）
//   - 本地已有数据 → 原样保留（不降级到"全开"，否则用户会被静默收回已购模块）
func (s *ModuleCatalogService) Sync(ctx context.Context) (*ModuleSyncResult, error) {
	start := time.Now()

	// 同步/离线引导跑在请求之外（后台 runner、启动期、手动同步按钮），
	// ctx 无租户绑定 —— on 模式直连会被隔离 callback fail-closed，于是
	// Refresh/bootstrapped 恒 false → IsEnabled 对所有模块返回 true = 付费闸
	// 静默全开（P1）。挂 SystemScope：平台侧是单商户单部署，镜像与同步日志
	// 统一落默认租户（tenant_id 由列默认值/GORM default 填 1，见
	// repository.UpsertMany 的行为注记）。off 模式 SystemScope 是直通，
	// SQL 形状逐字节不变。
	ctx = tenant.WithSystemScope(ctx)

	if !config.PlatformEnabled() {
		return s.bootstrapOffline(ctx, start)
	}
	snap, err := platform.NewModuleClient().SyncModules(ctx)
	if err != nil {
		// 平台不可达：保留上次镜像（不清空），只记 degraded 日志。
		// 清空会让用户正在用的模块突然消失 —— 那比"数据不准"严重得多。
		existing, listErr := s.repo.ListAll(ctx)
		if listErr == nil && len(existing) > 0 {
			_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
				Action: "sync", Status: "degraded", Source: "platform",
				ActiveCount: len(existing), ErrorMsg: err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			})
			return &ModuleSyncResult{
				ActiveCount: len(existing), Source: "cache", Degraded: true,
				Message: "平台暂不可达，沿用上次同步的模块状态",
			}, nil
		}
		return s.bootstrapOffline(ctx, start)
	}

	rows := make([]*model.TenantModule, 0, len(snap.ActiveKeys))
	now := time.Now()
	for _, r := range snap.Catalog {
		if !listHas(snap.ActiveKeys, r.ModuleKey) {
			continue
		}
		menuKeys, _ := json.Marshal(r.MenuKeys)
		rows = append(rows, &model.TenantModule{
			ModuleKey:   r.ModuleKey,
			Status:      model.TenantModuleActive,
			Source:      sourceOf(r.OwnedSource, r.Category, r.ModuleKey),
			Category:    r.Category,
			PriceCents:  r.PriceCents,
			MenuKeys:    string(menuKeys),
			ActivatedAt: now,
		})
	}
	if err := s.repo.UpsertMany(ctx, rows); err != nil {
		_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
			Action: "sync", Status: "failed", Source: "platform", ErrorMsg: err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
		})
		return nil, err
	}
	// 订单状态同步（与模块同一个周期）。
	//
	// 为什么必须做：module_orders 此前只在**下单时**写一次，之后永不刷新 ——
	// 于是平台确认收款后，用户端订单页会**永远显示「待确认收款」**，
	// 而「我的模块」页却已经显示该模块已开通。两页自相矛盾，
	// 用户会以为"付款了但没生效"而重复下单。
	//
	// 只刷状态、不覆盖 items/payment_guide：那两样是下单时的快照，不可变；
	// 而状态是唯一会变的东西。这也避免了为刷新去扩展平台端的列表响应。
	if n, oerr := s.syncOrderStatuses(ctx); oerr != nil {
		logger.Warnf("[module-sync] 订单状态同步失败（不影响模块下发）: %v", oerr)
	} else if n > 0 {
		logger.Infof("[module-sync] 已刷新 %d 条订单状态", n)
	}

	// 本地存在但平台已不再下发的模块要显式失效，否则撤销永远不生效
	existing, err := s.repo.ListAll(ctx)
	if err == nil {
		active := map[string]bool{}
		for _, k := range snap.ActiveKeys {
			active[k] = true
		}
		stale := make([]*model.TenantModule, 0)
		for _, e := range existing {
			if !active[e.ModuleKey] && e.Status == model.TenantModuleActive {
				c := *e
				c.Status = model.TenantModuleDisabled
				stale = append(stale, &c)
			}
		}
		if len(stale) > 0 {
			if err := s.repo.UpsertMany(ctx, stale); err != nil {
				logger.Warnf("[module] 失效陈旧模块失败: %v", err)
			}
		}
	}
	_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
		Action: "sync", Status: "success", Source: "platform",
		ActiveCount: len(snap.ActiveKeys), DurationMs: time.Since(start).Milliseconds(),
	})
	// 同步后必须让闸与菜单立刻生效，否则用户要等 60s 缓存过期
	_ = NewModuleGateService().Refresh(ctx)
	model.SetModuleEnabledFilter(NewModuleGateService().ModuleEnabledFunc())
	return &ModuleSyncResult{ActiveCount: len(snap.ActiveKeys), Source: "platform"}, nil
}

// bootstrapOffline 离线兜底：基础模块全开
func (s *ModuleCatalogService) bootstrapOffline(ctx context.Context, start time.Time) (*ModuleSyncResult, error) {
	existing, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
			Action: "sync", Status: "success", Source: "cache",
			ActiveCount: len(existing), DurationMs: time.Since(start).Milliseconds(),
		})
		return &ModuleSyncResult{ActiveCount: len(existing), Source: "cache"}, nil
	}
	now := time.Now()
	rows := make([]*model.TenantModule, 0, 8)
	for _, k := range model.BaseModuleKeys() {
		menuKeys, _ := json.Marshal(model.ModuleMenuBindings[k])
		rows = append(rows, &model.TenantModule{
			ModuleKey:   k,
			Status:      model.TenantModuleActive,
			Source:      model.TenantModuleSourceDefaultBase,
			Category:    model.ModuleCategoryBase,
			MenuKeys:    string(menuKeys),
			ActivatedAt: now,
		})
	}
	if err := s.repo.UpsertMany(ctx, rows); err != nil {
		return nil, err
	}
	_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
		Action: "offline_bootstrap", Status: "success", Source: "offline_bootstrap",
		ActiveCount: len(rows), DurationMs: time.Since(start).Milliseconds(),
	})
	_ = NewModuleGateService().Refresh(ctx)
	model.SetModuleEnabledFilter(NewModuleGateService().ModuleEnabledFunc())
	return &ModuleSyncResult{
		ActiveCount: len(rows), Source: "offline_bootstrap",
		Message: "未连接平台，已按基础模块开通全部免费能力",
	}, nil
}

// sourceOf 决定本地镜像记录的开通来源。
//
// 曾经写成「ownedStatus 非空即 purchase」，而 ownedStatus 传的是**状态**
// （active/expired/disabled），于是**所有模块都被记成"用户购买"** ——
// 基础能力（core/customer/inbox…）在「我的模块」页显示成花钱买的，
// 用户会问"我没买过的东西为什么显示已购买"。
//
// 判据改为按平台下发的 OwnedSource 直取；缺省时按模块分类回落：
// 基础模块 → default_base，收费模块 → purchase（保守：不要凭空说用户买了）。
func sourceOf(ownedSource, category, moduleKey string) string {
	if ownedSource != "" {
		return ownedSource
	}
	if category == model.ModuleCategoryBase {
		return model.TenantModuleSourceDefaultBase
	}
	if model.OwnerModuleCategory(moduleKey) == model.ModuleCategoryBase {
		return model.TenantModuleSourceDefaultBase
	}
	return model.TenantModuleSourcePurchase
}

func listHas(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// MyModuleView 「我的模块」视图项
type MyModuleView struct {
	ModuleKey  string     `json:"module_key"`
	Name       string     `json:"name"`
	Category   string     `json:"category"`
	PriceCents int64      `json:"price_cents"`
	Icon       string     `json:"icon"`
	Summary    string     `json:"summary"`
	DependsOn  []string   `json:"depends_on"`
	MenuKeys   []string   `json:"menu_keys"`
	Owned      bool       `json:"owned"`
	Source     string     `json:"source"`
	Status     string     `json:"status"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// MyModules 返回「我的模块」：已开通 + 未开通（全部目录）
//
// 一次返回两侧，前端「我的模块」页既能显示已购也能引导未购，不必两次请求。
func (s *ModuleCatalogService) MyModules(ctx context.Context) ([]MyModuleView, error) {
	rows, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	owned := map[string]*model.TenantModule{}
	for _, r := range rows {
		owned[r.ModuleKey] = r
	}

	// 目录来自编译期注册的 ModuleCatalogMeta（离线也拿得到）
	out := make([]MyModuleView, 0, len(model.ModuleCatalogMeta))
	for _, meta := range model.ModuleCatalogMeta {
		mk := meta.Key
		v := MyModuleView{
			ModuleKey:  mk,
			Name:       meta.Name,
			Category:   meta.Category,
			PriceCents: meta.PriceCents,
			Icon:       meta.Icon,
			Summary:    meta.Summary,
			DependsOn:  meta.DependsOn,
			MenuKeys:   model.ModuleMenuBindings[mk],
			Owned:      false,
		}
		if tm, ok := owned[mk]; ok {
			v.Owned = model.TenantModuleUsable(tm, now)
			v.Source = tm.Source
			v.Status = tm.Status
			v.ExpiresAt = tm.ExpiresAt
			if v.PriceCents == 0 {
				v.PriceCents = tm.PriceCents
			}
			// 菜单键**优先用编译期绑定**，本地镜像列只作兜底。
			//
			// 镜像里的 menu_keys 是平台下发回来的，而平台那份又是由本仓
			// cmd/gen-module-bindings 从 ModuleMenuBindings 导出的 ——
			// 也就是"把编译期常量绕一圈发回来"。用它当权威有两个坏处：
			//   ① 离线且从未同步成功时它是空的，而编译期绑定始终可用；
			//   ② 两端版本不一致时（用户端升级了、平台还没重新 seed），
			//      它会显示过期清单，而本地那份是对的。
			// 只有 llm-cloud 这类**没有菜单页**的模块，编译期绑定本就为空，
			// 此时才需要镜像兜底。
			if len(v.MenuKeys) == 0 {
				v.MenuKeys = repository.MenuKeysOf(tm)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// SyncLogs 同步日志
func (s *ModuleCatalogService) SyncLogs(ctx context.Context, limit int) ([]*model.ModuleSyncLog, error) {
	return s.repo.ListLogs(ctx, limit)
}

// syncOrderStatuses 从平台拉取本商户订单列表，刷新本地镜像的**状态**。
//
// 只更新 status / paid_at：items 与 payment_guide 是下单时的快照，不可变。
// 返回更新的条数。
func (s *ModuleCatalogService) syncOrderStatuses(ctx context.Context) (int, error) {
	rows, _, err := platform.NewModuleClient().ListOrders(ctx, "", 1, 200)
	if err != nil {
		return 0, err
	}
	updated := 0
	now := time.Now()
	for _, r := range rows {
		orderNo := mapStr(r, "order_no")
		status := mapStr(r, "status")
		if orderNo == "" || status == "" {
			continue
		}
		local, err := s.order.GetByOrderNo(ctx, orderNo)
		if err != nil || local == nil {
			continue // 本地没有的订单不新建：列表只以本地已下的单为准
		}
		if local.Status == status && local.PaidAt != nil {
			continue
		}
		local.Status = status
		local.UpdatedAt = now
		if status == model.ModuleOrderPaid || status == model.ModuleOrderFulfilled {
			if local.PaidAt == nil {
				local.PaidAt = &now
			}
		}
		if err := s.order.Upsert(ctx, local); err != nil {
			logger.Warnf("[module-sync] 刷新订单 %s 状态失败: %v", orderNo, err)
			continue
		}
		updated++
	}
	return updated, nil
}

// mapStr 从平台返回的 map 里安全取字符串字段
func mapStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
