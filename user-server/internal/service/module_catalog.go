package service

import (
	"context"
	"encoding/json"
	"sort"
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
// ===== 租户语义（per-tenant 缓存，勿改回全局单份）=====
//  镜像行按 tenant_id 归属，而闸是进程级共享的：缓存必须是
//   map[租户]状态，一次判定只读、只刷新**当前 ctx 解析出的那一个租户**。
//  曾经是「全局一份 cache + 全局一个 bootstrapped」，于是每个请求都把
//   自己租户的行灌进同一份缓存、被下一个请求覆盖 —— 租户 2 的请求会被
//   拿租户 1 的开通集判定（反之亦然），在 gateCacheTTL 内串着判。
//
// ===== 三态语义（B-2 修订 + 空态收口为单口径）=====
//   ① 该租户的模块数据为空（首启 / 从未同步 / on 模式 2+ 租户的空镜像）
//      → **只放行基础模块**（model.BaseModuleKeys），收费模块拦。
//        判据与边界写在 IsEnabled 的 ① 注释里；与 EnabledKeys 的空态
//        返回严格同一口径（单口径是本文件的教义）。
//   ② 有数据且命中模块 → 过闸（按行 usable 判定）
//   ③ 标注缺失（新增接口忘了归模块）
//      → **放行 + 记 WARN + 进 /api/modules/uncovered-routes 巡检清单**。
//        这态发生在**路由层**：路径没进模式表 → ModuleOwnerOf 返回 ""
//        → 根本不会带着非空 key 进到 IsEnabled（见 middleware/module_gate.go）。
//        这里选「放行」而不是「拦」是刻意的：新功能上线时全站 403 是灾难，
//        而漏拦最多让某个接口多暴露一点，两害相权的取舍写在 tasks.md B-2。
//        真正的兜底是 CI 里的覆盖率测试。
// ============================================================

// gateTenantState 单个租户的闸状态快照。
// 约定：加载后**整体替换、只读共享** —— 任何方法都不原地改 m，
// 读方在锁内拿到的是一份稳定视图（锁外读 map 不会与写方交错）。
type gateTenantState struct {
	m            map[string]bool // module_key → usable
	bootstrapped bool            // 该租户镜像是否有过行（len(rows)>0）
	loadedAt     time.Time
}

// ModuleGateService 模块闸（进程级单例，见 NewModuleGateService）
type ModuleGateService struct {
	// repo 显式仓储（测试注入）。nil 表示**按次**从全局 DB 现取：
	// 单例的寿命比单个测试用例长，构造期定格会把用例结束时被还原/关闭的
	// 连接钉死在单例上（测试进程里每个用例都会换库）。
	repo repository.TenantModuleRepository

	mu      sync.RWMutex
	tenants map[uint]*gateTenantState // 租户 → 状态（进程级共享）
}

// gateCacheTTL 模块状态本地缓存有效期（按租户条目各自计时）。
// 取 60s：模块开通是一个「低频但要尽快生效」的操作 —— 平台确认后用户在
// 一分钟内看到菜单变化是可接受的，而每次请求都查库会让模块闸变成热点。
const gateCacheTTL = 60 * time.Second

// baseModuleKeySet 基础模块集（免费常开）的判定快照。
// BaseModuleKeys 每次都遍历目录构造，闸的判定是每请求热路径 —— 装配一次。
var baseModuleKeySet = func() map[string]struct{} {
	out := make(map[string]struct{}, 8)
	for _, k := range model.BaseModuleKeys() {
		out[k] = struct{}{}
	}
	return out
}()

// isBaseModuleKey 是否属于基础模块集
func isBaseModuleKey(k string) bool {
	_, ok := baseModuleKeySet[k]
	return ok
}

var (
	processGateOnce sync.Once
	processGate     *ModuleGateService
)

// NewModuleGateService 返回**进程单例**的模块闸。
//
// 为什么是单例（勿改回"每次 new 一份"）：闸被 middleware（接口闸）、菜单过滤、
// 同步收尾三处共用，缓存与失效必须是同一份。曾经每次调用各建实例，于是
// Sync 收尾那句「同步后必须让闸与菜单立刻生效」刷的是一个没人读的实例 ——
// 真正被 middleware 持有的那份照旧服务旧缓存直到 TTL 过期；Invalidate 同理
// 打不中目标。测试请用 NewModuleGateServiceWithRepo（每例全新、缓存隔离）。
func NewModuleGateService() *ModuleGateService {
	processGateOnce.Do(func() {
		processGate = &ModuleGateService{}
	})
	return processGate
}

// NewModuleGateServiceWithRepo 带显式仓储（测试注入：独立实例、独立缓存）
func NewModuleGateServiceWithRepo(repo repository.TenantModuleRepository) *ModuleGateService {
	return &ModuleGateService{repo: repo}
}

// modules 解析实际使用的仓储（显式注入优先，否则按次取全局 DB）
func (s *ModuleGateService) modules() repository.TenantModuleRepository {
	if s.repo != nil {
		return s.repo
	}
	return repository.NewTenantModuleRepository()
}

// gateTenantOf 解析本次判定所属的租户。
//
// 口径（与 tenancyidx 隔离 callback 的注入语义对齐）：
//   - SystemScope → DefaultTenantID：系统作用域没有"调用方租户"，镜像与
//     同步统一落默认租户（单商户单部署），菜单快照（ModuleEnabledFunc）
//     也走这条 —— 注意 SystemScope 可能叠加在已有绑定上（手动同步按钮的
//     请求 ctx），此时仍按默认租户视图读，不看绑定；
//   - 已绑定（TenantMiddleware 写进请求 ctx 的租户）→ 就是它；
//   - 未绑定（裸 Background、装配早期）→ DefaultTenantID：off 模式即唯一
//     租户；on 模式下这是"没有调用方身份"时最保守的可读视图 —— 绝不能
//     兜成全量 union，那会把别的租户的开通状态判给当前请求。
func gateTenantOf(ctx context.Context) uint {
	if tenant.IsSystemScope(ctx) {
		return tenant.DefaultTenantID
	}
	if id, ok := tenant.FromContext(ctx); ok {
		return id
	}
	return tenant.DefaultTenantID
}

// gateReadCtx 构造「只读该租户镜像」的读 ctx。
//
// 与今日注入行为的关系：调用方 ctx 已绑定同一租户时**原样透传**（SQL 形状
// 与修复前逐字节一致，请求取消/日志值也一并保留）；其余情形（SystemScope /
// 无绑定 / 绑了别的租户）一律换成钉死 tid 的干净 ctx —— SystemScope 会让
// scopeQueryForTenant 直接放行，读到的是**跨租户 union**，拿它当"某一个
// 租户的视图"正是本次要修的串租户来源之一（菜单快照曾这样读）。
func gateReadCtx(ctx context.Context, tid uint) context.Context {
	if !tenant.IsSystemScope(ctx) {
		if id, ok := tenant.FromContext(ctx); ok && id == tid {
			return ctx
		}
	}
	return tenant.OnlyTenant(ctx, tid)
}

// Refresh 从仓储重载**当前 ctx 所属租户**的状态到缓存。
// 其它租户的条目不受影响；全清用 Invalidate（同步/激活收尾用后者，
// 因为新行可能落在任意租户上）。
func (s *ModuleGateService) Refresh(ctx context.Context) error {
	return s.refreshTenant(ctx, gateTenantOf(ctx))
}

// refreshTenant 拉取 tid 的镜像行并整体替换其缓存条目
func (s *ModuleGateService) refreshTenant(ctx context.Context, tid uint) error {
	rows, err := s.modules().ListAll(gateReadCtx(ctx, tid))
	if err != nil {
		return err
	}
	now := time.Now()
	m := make(map[string]bool, len(rows))
	for _, r := range rows {
		m[r.ModuleKey] = model.TenantModuleUsable(r, now)
	}
	st := &gateTenantState{m: m, bootstrapped: len(rows) > 0, loadedAt: now}
	s.mu.Lock()
	if s.tenants == nil {
		s.tenants = make(map[uint]*gateTenantState, 1)
	}
	s.tenants[tid] = st
	s.mu.Unlock()
	return nil
}

// Invalidate 清掉**全部租户**的缓存条目（同步/激活收尾、下单后调用）。
// 只清单租户是不够的：激活的落点是**下单租户**，可能不是默认租户 ——
// 留下的那条陈旧条目会一直服务到它自己的 TTL 过期。
func (s *ModuleGateService) Invalidate() {
	s.mu.Lock()
	s.tenants = nil
	s.mu.Unlock()
}

// loadState 取当前 ctx 所属租户的状态快照；缺失或超过 gateCacheTTL 则刷新。
// 刷新失败沿用上一次快照（判定不因一次 DB 抖动翻转）；从未成功加载过则按
// 空租户态返回（①：基础模块放行、收费拦 —— fail-closed，绝不"放行一切"）。
func (s *ModuleGateService) loadState(ctx context.Context) gateTenantState {
	tid := gateTenantOf(ctx)
	s.mu.RLock()
	st := s.tenants[tid]
	s.mu.RUnlock()
	if st != nil && time.Since(st.loadedAt) < gateCacheTTL {
		return *st
	}
	if err := s.refreshTenant(ctx, tid); err != nil {
		logger.Warnf("[module-gate] 刷新租户 %d 模块状态失败，按上一次缓存判定: %v", tid, err)
	}
	s.mu.RLock()
	st = s.tenants[tid]
	s.mu.RUnlock()
	if st == nil {
		return gateTenantState{}
	}
	return *st
}

// IsEnabled 模块是否已开通。
//
// 这是**唯一**的判定入口：菜单过滤、接口闸、cron/工具装配全部走它，
// 避免出现「菜单过滤用了 A 口径、接口闸用了 B 口径」的不一致。
func (s *ModuleGateService) IsEnabled(ctx context.Context, moduleKey string) bool {
	if moduleKey == "" {
		return true // 未标注的路由：放行（见文件头 ③）
	}
	st := s.loadState(ctx)
	if !st.bootstrapped {
		// ① 该租户镜像为空（首启 / 从未同步 / on 模式 2+ 租户的空镜像）：
		//    **只放行基础模块**，其余一律拦。与 EnabledKeys 空态同口径。
		//
		// 口径变更记录（原为"全放行"，B-2 收口）：上一版在这里写着"租户 2+
		// 恒空落进 allow-all 是产品决策" —— 那实际掩盖了两个问题：
		//   (a) 全放行 = 未付费租户拿到**全部**付费能力，付费闸对这些租户
		//       静默失效（界面无任何异样，正是本文件开头点名的失效形态）；
		//   (b) 它与 EnabledKeys 的空态返回（只给 BaseModuleKeys）自相
		//       矛盾 —— 同一条件两个口径，违背本文件的单口径教义。
		// 收口后各场景：
		//   on 模式空镜像租户 → 拿到**可用的基础系统**（base-pack），
		//       收费模块 4301 → 市场 → 下单，收款确认后激活的是**自己租户**
		//       的行（Sync 的落点归并，见 purchaseTenants）；
		//   全新安装 → 首轮同步在 router.Setup 之前同步执行（cmd/api），同步
		//       前的空窗里基础能力照常可用、收费能力锁住，与 bootstrapOffline
		//       灌"基础模块=免费常开"是同一条产品线；
		//   off 模式**可观察差异**：同步完成之前的付费 key 由放行改为拦截。
		//       判据：离线没有付费概念，bootstrapOffline 落地的就是 base
		//       （全免费），落地后行为与旧版逐字节一致；"还没数据"时放行
		//       收费能力是 fail-open，宁可 fail-closed（该窗口在生产里几乎
		//       不可见 —— 首轮同步先于路由装配）。
		//   附核对：platform-link 属基础模块（ModuleCategoryBase），/api/modules/my
		//       与市场购买链路在空态下依然可用；市场列表/下单本就不挂闸。
		//       故**不需要** gateAlwaysAllowed 之类的旁路集合 —— 空态允许集
		//       就是 BaseModuleKeys（bootstrapOffline 灌的那批，免费常开）。
		return isBaseModuleKey(moduleKey)
	}
	v, ok := st.m[moduleKey]
	if !ok {
		// ② 该模块在本地镜像里没有记录 —— 平台还没下发它（或已被撤销）
		return false
	}
	return v
}

// EnabledKeys 已开通模块 key 列表（菜单过滤用）
func (s *ModuleGateService) EnabledKeys(ctx context.Context) map[string]struct{} {
	st := s.loadState(ctx)
	out := make(map[string]struct{}, len(st.m)+len(baseModuleKeySet))
	if !st.bootstrapped {
		// 空镜像：与 IsEnabled ① 完全同口径 —— 只有基础模块
		for k := range baseModuleKeySet {
			out[k] = struct{}{}
		}
		return out
	}
	for k, v := range st.m {
		if v {
			out[k] = struct{}{}
		}
	}
	return out
}

// ModuleEnabledFunc 返回可注入 model.FilterMenuTreeByModule 的判定函数
func (s *ModuleGateService) ModuleEnabledFunc() func(string) bool {
	// 菜单过滤是**进程级单快照**（全局单口径，不属于任何请求），装配/同步时
	// 取一次就固化进闭包。SystemScope 显式表达"系统快照"意图：读按
	// gateTenantOf 解析为 DefaultTenantID 视图（不再 union 全租户 —— 曾经的
	// SystemScope 裸读会把所有租户的行混成一锅）。off 模式直通，行为不变。
	//
	// on 模式下菜单按**默认租户**的开通集渲染是有意为之（进程级共享）：按
	// 租户分裂菜单要同时动会话/缓存/权限三处的假设，不在本闸范围；接口闸
	// （IsEnabled）才是逐请求按租户判的 —— 菜单与接口短暂不一致时以接口
	// 4301 为准，购买入口（市场/我的模块）不受菜单裁剪影响。
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

// refreshProcessGate 同步/离线引导落地后让闸与菜单**立刻**生效。
//
// Invalidate 是全租户清：本轮可能激活了下单租户的行（落点=订单归属租户），
// 只刷默认租户会把激活租户的陈旧条目留到 TTL 过期 ——用户下单成功却要等
// 60s 才开通。之后把默认租户重灌一遍，菜单快照直接拿这份新状态。
func refreshProcessGate(ctx context.Context) {
	g := NewModuleGateService()
	g.Invalidate()
	if err := g.Refresh(ctx); err != nil {
		logger.Warnf("[module-gate] 同步后刷新默认租户状态失败（其余租户靠 TTL 内自愈）: %v", err)
	}
	model.SetModuleEnabledFilter(g.ModuleEnabledFunc())
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

	// 同步/离线引导跑在请求之外（后台 runner、启动期、手动同步按钮），ctx 可能
	// 无租户绑定 —— on 模式直连会被隔离 callback fail-closed，Sync 整体报错、
	// 镜像永不落地（P1 形态：已购模块永远激活不了，闸停在空态）。挂
	// SystemScope：平台侧是单商户单部署，目录镜像与同步日志统一落默认租户
	// （tenant_id 由列默认值/GORM default 填 1，见 repository.UpsertMany 的
	// 行为注记）。**唯一例外是收费模块的落点** —— 有本地下单记录的按下单
	// 租户分流（见落点归并/purchaseTenants），否则租户 2 买的东西会激活到
	// 租户 1。off 模式 SystemScope 是直通，SQL 形状逐字节不变。
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

	// 订单状态同步（与模块同一个周期）。**必须先于镜像落库**：下面的落点
	// 归并要用「已收款」的最新状态决定付费模块落到哪个租户 —— 顺序反了，
	// 平台本轮刚确认的那单会按旧状态（pending）错过激活。
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
	// ---- 落点归并：激活必须绑**下单租户** ----
	//
	// 收费模块的激活信号以平台下发目录为准（平台 MarkPaid 后 owned 随
	// /modules/sync 下发），但镜像行必须落在**下单的那个租户**上：平台侧
	// 只有 merchant 视角，订单在用户端带着 tenant_id。若把目录行统一塞进
	// SystemScope 写（列默认值 1），租户 2 买的东西会激活到租户 1 —— 租户 2
	// 永远打不开、租户 1 反而白得。对不上本地订单的行（平台种子、迁移前的
	// 历史单）落默认租户，与旧行为一致；off 模式订单全部归默认租户 →
	// 单组直通，行为逐字节不变。
	targets := s.purchaseTenants(ctx)
	groups := make(map[uint][]*model.TenantModule, 2)
	for _, row := range rows {
		tid := tenant.DefaultTenantID
		// 基础模块行**永远**落默认租户：免费常开是全安装的底线，不随订单
		// 归属漂移（若某张历史订单的 items 里混进了基础 key，默认租户也不
		// 丢失它的 base 行；下单租户的基础包由下面的补齐分支负责）。
		if !isBaseModuleKey(row.ModuleKey) {
			if t, ok := targets[row.ModuleKey]; ok {
				tid = t
			}
		}
		groups[tid] = append(groups[tid], row)
	}
	// 非默认租户拿到任何一行，就必须同时拿到基础模块行：boot=true 之后判定
	// 按行走，缺 base 行会把 core/customer 等基础能力一并判关（空镜像给
	// 基础包、有镜像却不给基础包是自相矛盾的）。
	for tid, g := range groups {
		if tid == tenant.DefaultTenantID {
			continue
		}
		groups[tid] = append(g, baseTenantModuleRows(now)...)
	}
	tids := make([]uint, 0, len(groups))
	for tid := range groups {
		tids = append(tids, tid)
	}
	sort.Slice(tids, func(i, j int) bool { return tids[i] < tids[j] })
	for _, tid := range tids {
		uctx := ctx
		if tid != tenant.DefaultTenantID {
			// ctx 挂着 SystemScope：直接用会绕过隔离填充（行落列默认值 1），
			// 慢路径先查后写也会变跨租户 —— 换成只带该租户的干净 ctx。
			uctx = tenant.OnlyTenant(ctx, tid)
		}
		if err := s.repo.UpsertMany(uctx, groups[tid]); err != nil {
			_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
				Action: "sync", Status: "failed", Source: "platform", ErrorMsg: err.Error(),
				DurationMs: time.Since(start).Milliseconds(),
			})
			return nil, err
		}
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
	// 同步后必须让闸与菜单立刻生效，否则激活租户要等 60s 缓存过期
	refreshProcessGate(ctx)
	return &ModuleSyncResult{ActiveCount: len(snap.ActiveKeys), Source: "platform"}, nil
}

// purchaseTenants 从本地订单镜像推导「模块 key → 下单租户」。
//
// 激活的落点判据：平台只有 merchant 视角（哪个模块 owned），**谁买的是
// 用户端订单行里的 tenant_id**（下单时由请求租户经 fillCreateForTenant
// 写入并校验）。取「最近一笔已收款/已履约的模块订单」——订单镜像按
// created_at DESC 分页，首见即最新（后翻页只补尚未覆盖的 key）。
//
// 读挂 SystemScope（Sync 的 ctx）→ 跨租户全量，这正是这里需要的：要给
// **任意租户**的订单找落点。读失败按"无订单"处理 → 全部落默认租户，
// 即旧行为（宁可退化到已知行为，也不能让同步整体失败）。
func (s *ModuleCatalogService) purchaseTenants(ctx context.Context) map[string]uint {
	out := make(map[string]uint)
	page := 1
	for {
		orders, total, err := s.order.List(ctx, page, 100)
		if err != nil {
			logger.Warnf("[module-sync] 读取订单镜像失败，本轮按默认租户落镜像: %v", err)
			return out
		}
		for _, o := range orders {
			if o == nil || o.OrderType != "module" {
				continue
			}
			if o.Status != model.ModuleOrderPaid && o.Status != model.ModuleOrderFulfilled {
				continue
			}
			tid := o.TenantID
			if tid == 0 {
				// 迁移前落库的历史行没有 tenant_id 值：归默认租户（存量数据
				// 升级口径同此），绝不放行 tid=0（那会构造出无绑定 ctx →
				// on 模式 fail-closed 让整轮同步失败）。
				tid = tenant.DefaultTenantID
			}
			for _, key := range orderModuleKeys(o) {
				if _, seen := out[key]; !seen {
					out[key] = tid
				}
			}
		}
		if int64(page*100) >= total {
			break
		}
		page++
	}
	return out
}

// orderModuleKeys 订单行快照里的模块 key。
//
// 只收「类型是模块且在 24 模块目录内」的行：选项行/LLM 行的 item_key 不是
// 模块 key，误收会凭空造出不存在的开通记录（闸会因此放行没买过的模块）。
func orderModuleKeys(o *model.ModuleOrder) []string {
	if o == nil || o.Items == "" {
		return nil
	}
	var items []platform.ModuleOrderItem
	if err := json.Unmarshal([]byte(o.Items), &items); err != nil {
		return nil
	}
	var out []string
	for _, it := range items {
		if it.ItemType != "" && it.ItemType != "module" {
			continue
		}
		if model.OwnerModuleCategory(it.ItemKey) == "" {
			continue // 不在目录里 = 不是模块 key
		}
		out = append(out, it.ItemKey)
	}
	return out
}

// baseTenantModuleRows 基础模块行（免费常开）。
// bootstrapOffline 与「给非默认租户首次落行」共用 —— 保证"有行的租户
// 必然有基础行"，与空态放行 BaseModuleKeys 是同一批 key。
func baseTenantModuleRows(now time.Time) []*model.TenantModule {
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
	return rows
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
	rows := baseTenantModuleRows(time.Now())
	if err := s.repo.UpsertMany(ctx, rows); err != nil {
		return nil, err
	}
	_ = s.repo.CreateLog(ctx, &model.ModuleSyncLog{
		Action: "offline_bootstrap", Status: "success", Source: "offline_bootstrap",
		ActiveCount: len(rows), DurationMs: time.Since(start).Milliseconds(),
	})
	refreshProcessGate(ctx)
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
