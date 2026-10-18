package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

// 模块闸 / 离线降级 / 市场 的服务层测试。
//
// 与中间件那组互补：中间件测「拦住没有」，这里测「**闸的数据对不对**」——
// 授权记录、下发同步、离线兜底。这两件事任一错了，收费模式都会静默失效。

func newModuleDB(t *testing.T) {
	t.Helper()
	database := testutil.NewTestDB(t)

	// 这几张表是本次改造新增的，**显式 DROP 后重建**而不是交给 testutil 的
	// "签名不一致才 DROP" 判据：
	//   改过唯一索引的列集之后，AutoMigrate 只新增/修改列，**不会删掉旧索引**，
	//   于是测试库里会同时存在
	//     idx_local_tenant_module (tenant_id, tenant_slot, module_key)   ← 旧的
	//     idx_local_tenant_module (tenant_slot, module_key)              ← 新的
	//   名字相同而形状不同，旧的那条让 ON CONFLICT 报
	//   "there is no unique or exclusion constraint matching"（42P10）——
	//   错误信息完全看不出"开发库里有条形状不对的残留索引"这回事。
	// 生产库不受影响：这些表尚未随任何版本发布过，不存在旧索引残留。
	for _, m := range []any{
		&model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{},
		&model.LLMWallet{}, &model.LLMWalletTxn{}, &model.PlatformLLMUsageRecord{},
	} {
		if err := database.Migrator().DropTable(m); err != nil {
			t.Fatalf("drop %T: %v", m, err)
		}
		if err := database.AutoMigrate(m); err != nil {
			t.Fatalf("migrate %T: %v", m, err)
		}
	}

	// **必须**在用例结束时清空，不能只在开头清。
	//
	// 本仓各包的测试共享同一个测试库（testutil 的槽位库机制），而模块闸
	// 有一层「本地有无模块数据」的判定：表里有行 = 数据存在 = 未开通的付费
	// 模块会被拦。于是本文件留下的行会被 internal/router 的菜单测试读到，
	// 症状是**同一份 RBAC 代码在 CI 上绿、在本地红**，两者只差执行顺序 ——
	// 也就是最难查的那一类。
	t.Cleanup(func() {
		for _, m := range []any{
			&model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{},
			&model.LLMWallet{}, &model.LLMWalletTxn{}, &model.PlatformLLMUsageRecord{},
		} {
			if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
				t.Logf("清理 %T 失败: %v", m, err)
			}
		}
	})
	// 清空：AutoMigrate 只保证结构，表里可能留着上一个用例的行。
	// 逐表 DELETE 而不是 Truncate —— TRUNCATE 需要表外 owner 权限且不能带 CASCADE 混用。
	for _, m := range []any{
		&model.TenantModule{}, &model.ModuleSyncLog{}, &model.ModuleOrder{},
		&model.LLMWallet{}, &model.LLMWalletTxn{},
	} {
		if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("清空 %T 失败: %v", m, err)
		}
	}
}

func newGate(t *testing.T) (*ModuleGateService, repository.TenantModuleRepository) {
	t.Helper()
	repo := repository.NewTenantModuleRepositoryWithDB(rawDB(t))
	g := NewModuleGateServiceWithRepo(repo)
	t.Cleanup(func() { model.SetModuleEnabledFilter(nil) })
	return g, repo
}

// rawDB 取共享测试连接。
//
// 注意：它**不**传模型列表 —— newModuleDB 已经在用例开头做过一次
// AutoMigrate+Truncate。每个用例再传一遍会让 testutil 走
// "schema 与目标模型签名不一致" 的 DropTable 分支，把上一个用例的数据连同
// 表一起干掉，并发时还会撞跨进程咨询锁。
func rawDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t)
}

// ---------------------------------------------------------- 闸的三态

// TestModuleGateEmptyBootAllowsOnlyBaseModules 空态（!bootstrapped）→ 只放行基础模块。
//
// 这是 B-2 收口后的空态单口径（原来那条 TestGateFirstBootAllowsEverything
// 断言的是"全放行"，已被证否）：
//   - 基础 key 放行：空系统仍是**可用的系统**（base-pack），首启/离线引导
//     落地前不卡人；
//   - 收费 key 拦：空镜像给到全部付费能力 = 付费闸 fail-open —— on 模式
//     2+ 租户的空镜像正是踩的这条；
//   - 空 key 放行：未标注路由（文件头 ③）。
// 与 EnabledKeys 的空态返回必须逐 key 一致（单口径是 module_catalog.go 的教义）。
func TestModuleGateEmptyBootAllowsOnlyBaseModules(t *testing.T) {
	newModuleDB(t)
	gate, _ := newGate(t)
	ctx := context.Background()

	// 前置自证：本地确实没有任何模块数据（首启 / 从未同步 / 空镜像租户）
	if rows, err := gate.modules().ListAll(ctx); err != nil || len(rows) != 0 {
		t.Fatalf("前置不成立：应读到 0 行空镜像，实际 rows=%d err=%v", len(rows), err)
	}

	for _, k := range model.BaseModuleKeys() {
		if !gate.IsEnabled(ctx, k) {
			t.Errorf("空态时基础模块 %s 应放行（基础能力不能被空镜像卡死）", k)
		}
	}
	for _, k := range model.PaidModuleKeys() {
		if gate.IsEnabled(ctx, k) {
			t.Errorf("空态时收费模块 %s 被放行：付费闸 fail-open（B-2 收口后应拦）", k)
		}
	}
	if !gate.IsEnabled(ctx, "") {
		t.Error("空 moduleKey（未标注路由）应放行（文件头 ③）")
	}

	// EnabledKeys 同口径逐 key 对齐
	keys := gate.EnabledKeys(ctx)
	for _, k := range model.BaseModuleKeys() {
		if _, ok := keys[k]; !ok {
			t.Errorf("空态 EnabledKeys 缺基础模块 %s（与 IsEnabled 口径不一致）", k)
		}
	}
	for _, k := range model.PaidModuleKeys() {
		if _, ok := keys[k]; ok {
			t.Errorf("空态 EnabledKeys 泄漏收费模块 %s（与 IsEnabled 口径不一致）", k)
		}
	}
}

func TestGateBlocksUnpurchasedPaidModule(t *testing.T) {
	newModuleDB(t)
	gate, repo := newGate(t)
	ctx := context.Background()

	// 只开通基础模块
	var rows []*model.TenantModule
	for _, k := range model.BaseModuleKeys() {
		menu, _ := json.Marshal(model.ModuleMenuBindings[k])
		rows = append(rows, &model.TenantModule{
			ModuleKey: k, Status: model.TenantModuleActive,
			Source: model.TenantModuleSourceDefaultBase, MenuKeys: string(menu),
			ActivatedAt: time.Now(),
		})
	}
	if err := repo.UpsertMany(ctx, rows); err != nil {
		t.Fatal(err)
	}
	gate.Invalidate()
	gate.Refresh(ctx)

	// 基础模块放行
	for _, k := range model.BaseModuleKeys() {
		if !gate.IsEnabled(ctx, k) {
			t.Errorf("基础模块 %s 应放行", k)
		}
	}
	// 收费模块一律拦 —— 这是整个商业模式的落地点
	for _, k := range model.PaidModuleKeys() {
		if gate.IsEnabled(ctx, k) {
			t.Errorf("未开通的收费模块 %s 被放行：付费模块白送", k)
		}
	}
}

func TestGateRevokedModuleBlockedAgain(t *testing.T) {
	newModuleDB(t)
	gate, repo := newGate(t)
	ctx := context.Background()
	if err := repo.UpsertMany(ctx, []*model.TenantModule{
		{ModuleKey: model.ModuleGEO, Status: model.TenantModuleActive, ActivatedAt: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	gate.Invalidate()
	if err := gate.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if !gate.IsEnabled(ctx, model.ModuleGEO) {
		t.Fatal("已开通的 geo 应放行")
	}
	// 撤销
	row, _ := repo.Get(ctx, model.ModuleGEO)
	row.Status = model.TenantModuleDisabled
	if err := repo.UpsertMany(ctx, []*model.TenantModule{row}); err != nil {
		t.Fatal(err)
	}
	gate.Invalidate()
	gate.Refresh(ctx)
	if gate.IsEnabled(ctx, model.ModuleGEO) {
		t.Fatal("已撤销的模块仍放行：撤销不生效等于没有撤销")
	}
}

func TestGateExpiredModuleBlocked(t *testing.T) {
	newModuleDB(t)
	gate, repo := newGate(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	if err := repo.UpsertMany(ctx, []*model.TenantModule{
		{ModuleKey: "exp", Status: model.TenantModuleActive, ActivatedAt: past, ExpiresAt: &past},
		{ModuleKey: "live", Status: model.TenantModuleActive, ActivatedAt: past, ExpiresAt: &future},
	}); err != nil {
		t.Fatal(err)
	}
	gate.Invalidate()
	gate.Refresh(ctx)
	if gate.IsEnabled(ctx, "exp") {
		t.Error("已过期模块应被拦（到期即失效）")
	}
	if !gate.IsEnabled(ctx, "live") {
		t.Error("未到期的模块应放行")
	}
}

// ---------------------------------------------------------- 离线降级

func TestOfflineBootstrapOpensBaseModulesOnly(t *testing.T) {
	newModuleDB(t)
	// 关掉平台
	t.Setenv("PLATFORM_ENABLED", "false")

	svc := NewModuleCatalogServiceWithRepos(
		repository.NewTenantModuleRepositoryWithDB(rawDB(t)),
		nil,
	)
	ctx := context.Background()
	res, err := svc.Sync(ctx)
	if err != nil {
		t.Fatalf("离线 Sync 应成功（不产生任何出站请求）: %v", err)
	}
	if res.Source != "offline_bootstrap" {
		t.Errorf("来源应为 offline_bootstrap，实际 %q", res.Source)
	}
	if res.ActiveCount != len(model.BaseModuleKeys()) {
		t.Errorf("离线应开通全部 %d 个基础模块，实际 %d", len(model.BaseModuleKeys()), res.ActiveCount)
	}

	gate := NewModuleGateServiceWithRepo(repository.NewTenantModuleRepositoryWithDB(rawDB(t)))
	gate.Invalidate()
	gate.Refresh(ctx)
	for _, k := range model.BaseModuleKeys() {
		if !gate.IsEnabled(ctx, k) {
			t.Errorf("离线部署时基础模块 %s 应放行（免费用户必须可用）", k)
		}
	}
	for _, k := range model.PaidModuleKeys() {
		if gate.IsEnabled(ctx, k) {
			t.Errorf("离线部署时收费模块 %s 被放行：定价被绕过", k)
		}
	}
}

func TestOfflineSyncDoesNotDowngradeExistingGrants(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")

	repo := repository.NewTenantModuleRepositoryWithDB(rawDB(t))
	ctx := context.Background()
	// 模拟"用户买过 geo，然后断网"
	if err := repo.UpsertMany(ctx, []*model.TenantModule{
		{ModuleKey: model.ModuleGEO, Status: model.TenantModuleActive,
			Source: model.TenantModuleSourcePurchase, ActivatedAt: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewModuleCatalogServiceWithRepos(repo, nil)
	if _, err := svc.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	gate := NewModuleGateServiceWithRepo(repo)
	gate.Invalidate()
	gate.Refresh(ctx)
	if !gate.IsEnabled(ctx, model.ModuleGEO) {
		t.Fatal("离线兜底把用户已购买的模块收回了：断一次网就失去已购能力")
	}
}

// ---------------------------------------------------------- 市场

func TestMarketListMarksBaseAsFreeAndPaidAsPayable(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	svc := NewModuleCatalogServiceWithRepos(
		repository.NewTenantModuleRepositoryWithDB(rawDB(t)), nil)
	ctx := context.Background()

	list, enabled, err := svc.MarketList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Error("PLATFORM_ENABLED=false 时 platform_enabled 应为 false")
	}
	byKey := map[string]ModuleMarketItem{}
	for _, m := range list {
		byKey[m.ModuleKey] = m
	}
	if len(byKey) != len(model.ModuleCatalogMeta) {
		t.Errorf("市场应列出全部 %d 个模块，实际 %d", len(model.ModuleCatalogMeta), len(byKey))
	}
	if !byKey[model.ModuleCore].Free {
		t.Error("core 应标记为 free")
	}
	if byKey[model.ModuleGEO].Free {
		t.Error("geo 是收费模块，不应标记 free")
	}
	if byKey[model.ModuleGEO].FreeToEnable {
		t.Error("geo 有价，不应标记 free_to_enable")
	}
	if !byKey[model.ModuleLLMCloud].FreeToEnable {
		t.Error("llm-cloud 定价为 0：应标记「需开通但不收钱」")
	}
}

func TestMarketListMarksOwnedFromLocalMirror(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	repo := repository.NewTenantModuleRepositoryWithDB(rawDB(t))
	ctx := context.Background()
	if err := repo.UpsertMany(ctx, []*model.TenantModule{
		{ModuleKey: model.ModuleGEO, Status: model.TenantModuleActive,
			Source: model.TenantModuleSourcePurchase, ActivatedAt: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewModuleCatalogServiceWithRepos(repo, nil)
	list, _, err := svc.MarketList(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range list {
		want := m.ModuleKey == model.ModuleGEO
		if m.Owned != want {
			t.Errorf("%s Owned=%v，期望 %v", m.ModuleKey, m.Owned, want)
		}
	}
}

func TestMarketDetailSplitsDependencyClosure(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	svc := NewModuleCatalogServiceWithRepos(
		repository.NewTenantModuleRepositoryWithDB(rawDB(t)), nil)
	ctx := context.Background()

	d, err := svc.MarketDetail(ctx, model.ModuleAISales)
	if err != nil {
		t.Fatal(err)
	}
	// ai-sales 依赖 core/customer/inbox/llm-self（基础）+ ai-agent（收费）
	if len(d.AutoEnabled) == 0 {
		t.Error("应自动开通的基础依赖为空")
	}
	if len(d.PayableDependencies) == 0 {
		t.Error("应列出需另购的收费依赖（不列出会导致用户买完发现用不了）")
	}
	for _, k := range d.PayableDependencies {
		if model.OwnerModuleCategory(k) == model.ModuleCategoryBase {
			t.Errorf("基础依赖 %s 不该出现在「需另购」里", k)
		}
	}
	for _, k := range d.AutoEnabled {
		if model.OwnerModuleCategory(k) != model.ModuleCategoryBase {
			t.Errorf("收费依赖 %s 不该出现在「自动开通」里（意外收费）", k)
		}
	}
	if _, err := svc.MarketDetail(ctx, "no-such-module"); !errors.Is(err, ErrModuleNotInCatalog) {
		t.Errorf("未知模块应返回 ErrModuleNotInCatalog，实际 %v", err)
	}
}

// ---------------------------------------------------------- 菜单过滤

func TestMenuTreeFilteredByClosedModule(t *testing.T) {
	newModuleDB(t)
	repo := repository.NewTenantModuleRepositoryWithDB(rawDB(t))
	ctx := context.Background()
	// 只开通基础模块（本地已有数据 → 闸生效）
	if err := repo.UpsertMany(ctx, baseModuleRows()); err != nil {
		t.Fatal(err)
	}
	gate := NewModuleGateServiceWithRepo(repo)
	gate.Invalidate()
	gate.Refresh(ctx)
	model.SetModuleEnabledFilter(gate.ModuleEnabledFunc())

	tree := model.FilterMenuTreeByModule(model.SystemMenuTree, model.ModuleEnabledFilter)

	// geo 已关闭 → 它的目录节点必须消失
	if findMenuKey(tree, "geoTools") >= 0 {
		t.Error("geo 模块已关闭，但 geoTools 目录仍在菜单树里（用户点进去会撞 403）")
	}
	// 基础模块的节点必须还在
	for _, k := range []string{"systemUser", "customer360", "knowledgeManagement"} {
		if findMenuKey(tree, k) < 0 {
			t.Errorf("基础模块节点 %s 不应被过滤掉", k)
		}
	}
}

func baseModuleRows() []*model.TenantModule {
	now := time.Now()
	out := make([]*model.TenantModule, 0, 8)
	for _, k := range model.BaseModuleKeys() {
		menu, _ := json.Marshal(model.ModuleMenuBindings[k])
		out = append(out, &model.TenantModule{
			ModuleKey: k, Status: model.TenantModuleActive,
			Source: model.TenantModuleSourceDefaultBase, MenuKeys: string(menu),
			ActivatedAt: now,
		})
	}
	return out
}

func findMenuKey(nodes []model.SystemMenuNode, key string) int {
	for i, n := range nodes {
		if n.Key == key {
			return i
		}
		if idx := findMenuKey(n.Children, key); idx >= 0 {
			return idx
		}
	}
	return -1
}

// ---------------------------------------------------------- 监管告知

func TestRegulatoryNoticeFallbackWhenPlatformDisabled(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	svc := NewModuleCatalogServiceWithRepos(
		repository.NewTenantModuleRepositoryWithDB(rawDB(t)), nil)
	n := svc.RegulatoryNotice(context.Background())
	if n.Notice == "" {
		t.Fatal("离线部署也必须能拿到监管告知：否则离线用户根本没法开户")
	}
	if n.AgreementVersion == "" {
		t.Fatal("协议版本不能为空（它是「用户当时同意的是哪一版」的唯一凭据）")
	}
	need := map[string]bool{}
	for _, f := range n.RequiredFields {
		need[f] = true
	}
	for _, must := range []string{"company_name", "legal_person", "phone", "email"} {
		if !need[must] {
			t.Errorf("必填项清单缺 %s（需求原文明确要求这四项）", must)
		}
	}
}

func TestPaymentGuideNeverEmpty(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	svc := NewModuleCatalogServiceWithRepos(
		repository.NewTenantModuleRepositoryWithDB(rawDB(t)), nil)
	g, err := svc.PaymentGuide(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if g == "" {
		t.Fatal("付款指引不能为空：空串等于「订单已提交但没人知道找谁付款」")
	}
}

var (
	_ = os.Getenv
	_ = config.PlatformEnabled
)

// TestStartModuleSyncBootstrapsBaseModules 生产链路的最后一环。
//
// 为什么这条必须用真实 runner 而不只是测 Sync()：
// 事故形态是「Sync 写对了、闸也写对了、两者之间没人连线」——
// tenant_modules 恒空 → 闸永远处在"首启全放行"态 → 付费模块对所有人开放，
// 而界面、菜单、API 全都正常。**没有任何单点测试能发现它**：
// Sync 的用例通过、闸的用例通过、菜单的用例通过。
// 所以这里钉的是"后台循环真的会跑起来并落数据"这件事本身。
func TestStartModuleSyncBootstrapsBaseModules(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false") // 离线：应兜底开通全部基础模块

	r := StartModuleSync(context.Background())
	defer r.Stop()
	r.WaitFirstSync()

	keys, _, err := func() ([]string, int64, error) {
		repo := repository.NewTenantModuleRepositoryWithDB(rawDB(t))
		rows, err := repo.ListActiveKeys(context.Background(), time.Now())
		return rows, int64(len(rows)), err
	}()
	if err != nil {
		t.Fatalf("读取本地模块镜像失败: %v", err)
	}
	if len(keys) < len(model.BaseModuleKeys()) {
		t.Fatalf("同步未落数据：期望至少 %d 个基础模块，实际 %d —— "+
			"这意味着模块闸永远处于「首启全放行」态，付费模块全部白送",
			len(model.BaseModuleKeys()), len(keys))
	}

	// 闸此刻必须真的开始拦（这是本条用例真正的判据）
	gate := NewModuleGateServiceWithRepo(repository.NewTenantModuleRepositoryWithDB(rawDB(t)))
	gate.Invalidate()
	if err := gate.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gate.IsEnabled(context.Background(), model.ModuleGEO) {
		t.Error("同步完成后未开通的收费模块仍放行：闸未生效")
	}
	if !gate.IsEnabled(context.Background(), model.ModuleCore) {
		t.Error("同步完成后基础模块应放行")
	}
}

// TestModuleSyncRunnerStopIsClean 停得掉（优雅退出与测试依赖它）
func TestModuleSyncRunnerStopIsClean(t *testing.T) {
	newModuleDB(t)
	t.Setenv("PLATFORM_ENABLED", "false")
	r := StartModuleSync(context.Background())
	r.WaitFirstSync()
	done := make(chan struct{})
	go func() { r.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop 未在 5s 内返回：优雅退出会被拖住")
	}
	// 二次 Stop 不得 panic
	r.Stop()
}

// TestModuleGateOnModeSyncAndTenantScopedRead P1 回归：on 模式闸不许静默全开。
//
// 修复前的失效链（两段分别钉住两端）：
//
//	① 同步/引导在请求之外跑（后台 runner、手动同步按钮），ctx 无租户绑定，
//	   ListAll/UpsertMany 被隔离 callback fail-closed → Sync 报错 →
//	   tenant_modules 恒空 → bootstrapped=false → IsEnabled 对**所有**模块
//	   返回 true —— 付费闸整体失效，且界面无任何异样；
//	② 请求路径上闸必须能带着租户 ctx 读到镜像（middleware 现在把
//	   c.Request.Context() 递进 adapter）：基础模块放行、未开通收费模块拦截。
//
// 注意：必须经**带模型**的 NewTestDB 建连接 —— 只有那条路径会注册隔离
// callback 与租户登记表；不传模型的连接在 on 模式下等于没开隔离，用例会假绿。
func TestModuleGateOnModeSyncAndTenantScopedRead(t *testing.T) {
	withTenantMode(t, tenant.ModeOn)
	database := testutil.NewTestDB(t, &model.TenantModule{}, &model.ModuleSyncLog{})
	t.Setenv("PLATFORM_ENABLED", "false") // 离线兜底：不发出站请求，直接灌基础模块

	repo := repository.NewTenantModuleRepositoryWithDB(database)
	svc := NewModuleCatalogServiceWithRepos(repo, nil)

	// 前置自证：隔离 callback 在本连接上必须真的在拦 —— 否则①②在修复前也全绿（假绿）
	if _, err := repo.ListAll(context.Background()); err == nil {
		t.Fatal("无租户绑定的 tenant_modules 读未被 fail-closed：on 模式隔离未生效，本用例测不到 P1")
	}

	// ① 后台形态的同步：无租户绑定也必须成功（修复前 fail-closed）
	res, err := svc.Sync(context.Background())
	if err != nil {
		t.Fatalf("on 模式离线同步应成功（修复前 fail-closed → 闸全放行）: %v", err)
	}
	if res.Source != "offline_bootstrap" {
		t.Fatalf("来源应为 offline_bootstrap，实际 %q", res.Source)
	}

	// 镜像行必须落默认租户，否则下一步的租户 ctx 读一条也看不见
	sysCtx := tenant.WithSystemScope(context.Background())
	rows, err := repo.ListAll(sysCtx)
	if err != nil {
		t.Fatalf("读镜像失败: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("同步未落镜像行：tenant_modules 恒空 = 闸永远在「首启全放行」态")
	}
	for _, r := range rows {
		if r.TenantID != tenant.DefaultTenantID {
			t.Fatalf("镜像行 %s 落在 tenant_id=%d，应为默认租户 %d",
				r.ModuleKey, r.TenantID, tenant.DefaultTenantID)
		}
	}

	// ② 请求路径形态：租户 ctx 直连闸（修复前 adapter 用 Background，等价于这里的裸读）
	gate := NewModuleGateServiceWithRepo(repo)
	gate.Invalidate()
	reqCtx := tenant.WithTenantID(context.Background(), tenant.DefaultTenantID)
	if err := gate.Refresh(reqCtx); err != nil {
		t.Fatalf("租户 ctx 刷新镜像不应 fail-closed: %v", err)
	}
	if !gate.IsEnabled(reqCtx, model.ModuleCore) {
		t.Error("基础模块应放行")
	}
	if gate.IsEnabled(reqCtx, model.ModuleGEO) {
		t.Error("未开通的收费模块被放行：on 模式闸静默全开（P1 回归）")
	}

	// 收尾：清镜像行（此刻 mode 仍是 on，无绑定删除会被 fail-closed → 挂 SystemScope）
	t.Cleanup(func() {
		for _, m := range []any{&model.TenantModule{}, &model.ModuleSyncLog{}} {
			if err := database.Session(&gorm.Session{AllowGlobalUpdate: true}).
				WithContext(tenant.WithSystemScope(context.Background())).
				Delete(m).Error; err != nil {
				t.Logf("清理 %T 失败: %v", m, err)
			}
		}
	})
}
