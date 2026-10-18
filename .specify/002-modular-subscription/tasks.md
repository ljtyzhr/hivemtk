# Tasks: 功能模块化 · 按需订阅收费改造

> **唯一施工清单**（`.specify/002-modular-subscription/tasks.md`）。
> 逐条打勾；跳过须在 Deviations 写明原因。
> 每条任务标注 **[US]** 对应 spec 的用户故事，**[FR]** 对应功能需求。
>
> **状态标注约定（2026-10-18 对账）**：`[x]` 已完成（附完成证据）；`[ ]` 未完成/部分完成（附一行状态）；
> `[~]` 方案已作废/被替代（附原因）。证据均为实际读码核实。

---

## P0 · 模块目录种子（菜单绑定的事实源）

- [~] **T001** [FR-1.3] [G1] 从 `user-server/internal/model/menu.go` 的 `SystemMenuTree` 导出全部 208 个 `menu_key`（含目录节点），落 `hivemtk-platform/platform-server/seed/module_menu_keys.txt`（生成件，带生成命令头注释）(已作废: 产物形态变更——菜单 key 事实源落 `user-server/internal/model/module_bindings.go`（24 模块 + 全量 key 归属），种子 JSON 由 `cmd/gen-module-bindings` 生成并 `//go:embed` 进 `platform-server/internal/service/module_catalog_seed.json`（E4 后磁盘文件仅作可选覆盖），未落独立 `seed/module_menu_keys.txt`)
- [~] **T002** [FR-1.1][FR-1.3] 写 `platform-server/internal/service/module_catalog_seeds.go`：24 个模块的元数据种子（key/名称/分类/默认价格分/图标/摘要/描述/排序）+ 208 个 `menu_key` → `module_key` 的完整归属映射(已作废: 未建 `module_catalog_seeds.go`；种子落在 user-server `module_bindings.go`（事实源，两端测试钉死）+ platform `module_catalog_seed.json` 内嵌)
- [x] **T003** [FR-1.2] 依赖边种子（`module_dependencies`）：按 `MODULAR_MODULES.md §4.1` 依赖图 (完成证据: platform-server/internal/repository/module.go:502 `SeedDependencies` + `module_catalog_seed.json` 依赖边；环检测 `TestAddDependencyRejectsCycle`)
- [x] **T004** [G1] 幂等 upsert：`platform-server/internal/repository/module.go` 的 `SeedCatalog`，重复执行不产生重复行 (完成证据: module_catalog_service.go:446 `Seed()` 幂等灌入；`module_seed_test.go:56 TestSeedIdempotent`；函数名落地为 Seed 非 SeedCatalog)
- [x] **T005** [G1] 测试 `module_catalog_seeds_test.go`：断言 ①208 个 key 全覆盖 ②无一个 key 归属两个模块 ③每个模块至少 1 个 key ④每个 key 的模块分类与 `MODULAR_MODULES.md §2.1` 速查表一致 ⑤基础模块 7 个齐全 (完成证据: user-server/internal/model/module_bindings_test.go（全覆盖/无重复/基础模块数）+ platform-server/internal/service/module_seed_test.go:16 `TestSeedFileCoversAllMenuKeys`)

## P1 · 平台端 · 模块 / 租户 / 订单（后端）

### P1.1 模型与仓储

- [x] **T101** [FR-1.1] `platform-server/internal/model/module_catalog.go`：`ModuleCatalog` / `ModuleDependency` / `ModuleMenuBinding` / `ModuleOption` (完成证据: model/module_catalog.go:41/81/95/114)
- [x] **T102** [FR-1.2] 依赖环检测：`repository/module.go` 的 `AddDependency` 内拓扑检测，返回 `ErrModuleDependencyCycle` (完成证据: repository/module.go:135 `AddDependency` + :21/:149 `ErrModuleDependencyCycle`)
- [x] **T103** [FR-1.5] `model/tenant_registration.go` + `model/tenant_module.go`（`tenant_modules`）(完成证据: 两模型并入 model/module_catalog.go——`TenantModule`:143、`TenantRegistration`:181；user-server 侧镜像 model/tenant_module.go:45)
- [x] **T104** [FR-2.1] `model/tenant_registration.go`（`tenant_registrations`，含 `reg_mode` / `platform_sync` / `agreement_version` / `regulatory_ack`）(完成证据: model/module_catalog.go:181-215 含全部四字段)
- [x] **T105** [FR-3.1] `model/order.go`：`Order` / `OrderItem` / `OrderStatusLog` / `PaymentRecord` / `PaymentCallback` (完成证据: model/order.go:65/136/173/199/249)
- [x] **T106** [FR-3.1] 订单状态机常量与合法迁移表（`draft → pending_payment → paid → fulfilled`，分支 `cancelled` / `rejected`）(完成证据: model/order.go:39-42 状态常量 + `TestOrderStateMachineTransitions`)
- [x] **T107** [FR-3.1] `repository/order.go`：`CreateOrder`（事务内写 order + items + status log）、`MarkPaidCAS`（`WHERE status='pending_payment'`）、`GetByOrderNo` (完成证据: repository/order.go:36 `CreateOrder`、:141 `MarkStatusCAS`（落地名）、:76 `GetByOrderNo`)
- [x] **T108** [FR-1.5] `repository/module_grant.go`：`GrantWithClosure`（事务内写 `tenant_modules` + 依赖闭包）、`Revoke`（在用检测）、`ListActiveForMerchant` (完成证据: 无独立 module_grant.go——闭包在 service/module_grant_service.go:53 `Grant`+`walkDeps`、撤销 :174；仓储 UpsertManyTenantModules/ListActiveTenantModules 在 repository/module.go:478/:436)

### P1.2 服务

- [x] **T111** [FR-1.7] `service/module_catalog_service.go`：CRUD / 发布 / 下线；`category=base` 的改价改状态一律 `ErrBaseModuleImmutable` (完成证据: module_catalog_service.go:121/151/202/212；:162/:218/:231 ErrBaseModuleImmutable)
- [x] **T112** [FR-1.2] 依赖环对外错误映射（400 `module_dependency_cycle`）(完成证据: controller/module_catalog_controller.go:41)
- [x] **T113** [FR-1.5] `service/module_grant_service.go`：下发（含闭包 + 依赖来源标注）、撤销（在用检测 400 `module_in_use` + 依赖方列表）(完成证据: module_grant_service.go:53/:174；:200 ErrModuleInUse + 依赖方列表)
- [x] **T114** [FR-1.6] 商户端同步快照：`GET /merchant-api/modules/sync` 返回激活模块 + 菜单绑定 + 定价 + 依赖闭包 (完成证据: module_catalog_service.go:659 `SyncSnapshot` + router/router.go:67 路由)
- [x] **T115** [FR-2.3] `service/tenant_registration_service.go`：登记写入 / 列表 / 详情 / 手工补同步 (完成证据: platform-server/internal/service/tenant_registration_service.go 存在且被 controller 引用)
- [x] **T116** [FR-3.3] `service/order_service.go`：`CreateOrder`（幂等：同 `order_no` 重复创建返回既有单）、**`MarkPaid(ctx, orderNo, PaymentNotice)` 唯一入账入口**（写 `payment_records` + `order_status_logs` + 触发模块开通/钱包入账）、`Cancel` / `Reject` (完成证据: order_service.go:119 `CreateOrder`、:337 `MarkPaid`、:579 `Cancel`、:600 `Reject`、:533 `fulfillLLM`)
- [x] **T117** [FR-3.3] `MarkPaid` 幂等：已 `paid` 的订单再次调用返回 `ErrOrderAlreadyPaid`（409），**不重复开通** (完成证据: order_service.go:19/:393；`TestMarkPaidOpensModules` 重复调用断言)
- [x] **T118** [FR-3.5] `service/payment_notify_service.go`：回调入口，本期返回 501 + 引导文案；回调原始报文仍写 `payment_callbacks`（`verified=false`），保证**二期只需补验签** (完成证据: controller/payment_callback_controller.go:24/:38 `Notify` 501 + 留痕；InsertCallbackIfAbsent)
- [x] **T119** [FR-3.6] 订单行快照：`item_type` / `item_key` / `snapshot` jsonb (完成证据: model/order.go:140-150)

### P1.3 控制器与路由

- [x] **T121** [US1] `controller/module_catalog_controller.go`：管理端 CRUD + 发布 + 下线 + 依赖管理 (完成证据: controller/module_catalog_controller.go + router.go:271-278)
- [x] **T122** [US1] `controller/module_grant_controller.go`：下发 / 撤销 / 批量下发 / 租户模块矩阵查询 (完成证据: 落地并入 module_catalog_controller——Grant/Revoke/Matrix 处理器，router.go:290-293)
- [x] **T123** [US2] `controller/order_controller.go`：列表 / 详情 / 确认收款 / 作废 / 拒绝 / 凭证上传 (完成证据: controller/order_controller.go + router.go:302-308)
- [x] **T124** [US3] `controller/tenant_registration_controller.go`：列表 / 详情 / 更新 / 补同步 (完成证据: controller/tenant_registration_controller.go:74 List + router.go:324-328)
- [x] **T125** [FR-2.1] 商户端开户登记：`POST /merchant-api/tenant/register`（HMAC 鉴权，必填校验）(完成证据: router.go:72 `merchantAPI.POST("/tenant/register")` + MerchantRegister；必填校验 TestRegisterTenantRequiredFields)
- [x] **T126** [FR-1.6] 商户端：`GET /merchant-api/modules/sync` (完成证据: router.go:67)
- [x] **T127** [FR-3.5] `POST /platform/payments/notify/:channel`（501 占位路由）(完成证据: router.go:315)
- [x] **T128** 路由注册进 `router/router.go`（`/platform/modules*` `/platform/tenants` `/platform/orders` 三组，全部挂 `JWTAuthMiddleware` + `RequirePlatformAdmin` + `PlatformAuditMiddleware`）(完成证据: router.go:261-315 三组均在 platform 组下自动带三中间件)
- [x] **T129** `internal/utils/db/db.go`：`migrateModels` 追加新模型 + 幂等 DDL 兜底 (完成证据: db.go:78-90 AutoMigrate 19 张新表 + :127 幂等 DDL)
- [x] **T130** 测试：`module_catalog_seeds_test.go`（T005）· `order_service_test.go`（状态机 + 幂等 + 并发 CAS）· `module_grant_test.go`（闭包 + 在用检测 + 环检测）· `tenant_registration_test.go`（必填 + 幂等 + 离线）(完成证据: 实际文件为 module_seed_test.go + module_commerce_test.go（TestMarkPaidConcurrentCASOnlyOneWins / TestAddDependencyRejectsCycle / TestGrantClosesDependenciesAndRevokeBlocks / TestRegisterTenantRequiredFields 等 20+ 用例）)

## P2 · 平台端 · 页面

- [x] **T201** [US1] `platform-web/src/api/module.js`（目录/依赖/菜单绑定/下发/撤销）(完成证据: api/module.js:14 `moduleApi` / :30 `grantApi`)
- [~] **T202** [US2] `platform-web/src/api/order.js` (已作废: 未单独建文件；`orderApi` 并入 api/module.js:40)
- [~] **T203** [US3] `platform-web/src/api/tenant.js` (已作废: 未单独建文件；`tenantApi` 并入 api/module.js:55)
- [x] **T211** [US1] `views/platform/module/Catalog.vue`（列表：分类/状态/价格/已开通租户数）(完成证据: views/platform/module/Catalog.vue)
- [~] **T212** [US1] `views/platform/module/Editor.vue`（编辑 + 依赖多选 + 菜单绑定勾选 + 子选项 + 发布/下线）(已作废: 未单独建 Editor.vue；编辑器为 Catalog.vue 内弹窗（:101 openEditor / :121 dialog，含依赖多选与菜单绑定），菜单绑定覆盖不完整有告警 :52)
- [x] **T213** [US1] `views/platform/module/Grant.vue`（租户 × 模块矩阵，勾选即下发，列依赖）(完成证据: views/platform/module/Grant.vue)
- [x] **T214** [US2] `views/platform/order/List.vue`（状态筛选 + **待确认红点/高亮** + 超期标记）(完成证据: order/List.vue:9 `pending-hint` 待确认高亮；超期按 B-9 改为待确认时长排序)
- [~] **T215** [US2] `views/platform/order/Detail.vue`（订单行 / 凭证 / 状态时间线 / 确认收款按钮 + 二次确认）(已作废: 未单独建 Detail.vue；详情抽屉并入 order/List.vue——:73 openDetail、:119 凭证、:130 时间线、:145/:151 确认收款 + 二次确认)
- [~] **T216** [US3] `views/platform/tenant/List.vue` + `Registration.vue`（登记详情 + 监管字段 + 补同步）(已作废: 未单独建 Registration.vue；登记详情/补录/复核并入 tenant/List.vue 弹窗——:107 详情补录、:190 review、监管字段提示 :33)
- [x] **T221** `router/modules/platform.js` 追加分组：`功能模块` `租户` `订单`；`layout/Layout.vue` 的 `GROUP_ICONS` 补三个图标 (完成证据: router/modules/platform.js:70-108；Layout.vue:121-123 GROUP_ICONS 补 '功能模块'/'租户'/'LLM 计费')
- [x] **T222** `npm run build` 通过；`Layout.vue` 菜单由 `meta.group` 驱动，无硬编码漂移 (完成证据: Layout.vue:139 按 meta.group 聚合；build 通过见本文件五/六轮门禁表)

## P3 · 用户端 · 模块闸与本地镜像（后端）

- [x] **T301** [FR-5.1] `user-server/internal/model/tenant_module.go`：`TenantModule` / `ModuleSyncLog` (完成证据: model/tenant_module.go:45 / :96)
- [x] **T302** [FR-5.1] `repository/tenant_module.go`：批量 upsert / 按 module_key 查 / 查全部激活 key (完成证据: repository/tenant_module.go:20 接口定义)
- [x] **T303** [FR-5.1] `platform/module_client.go`：HMAC 客户端（对齐 `asset_market_client.go` 形态）(完成证据: platform/module_client.go:132-140 经 NewPlatformClient(merchantKey) 签名)
- [x] **T304** [FR-5.1] `service/module_catalog_service.go`：`SyncFromPlatform`（拉取 + 幂等落本地 + 日志）；平台关闭时用 `disabledClient` 降级 (完成证据: user-server/internal/service/module_catalog.go:359 `Sync()`（落地名）+ :603 `bootstrapOffline` 离线降级；platform/disabled_client.go)
- [x] **T305** [FR-5.1] 同步时机：与心跳同频（默认 10 分钟），`utils.SafeGo` + 可重入保护；启动时同步一次 (完成证据: service/module_sync_runner.go:31 10min / :48 StartModuleSync / :74 SafeGo；cmd/api/main.go:606)
- [~] **T306** [FR-5.7] 离线降级：基础模块全开 / `price=0` 付费模块自动开 / `price>0` 保持关闭 (已作废: 口径收紧——离线只开基础模块（module_catalog.go:603 bootstrapOffline + `TestOfflineBootstrapOpensBaseModulesOnly`）；「price=0 付费模块自动开」未实现)
- [x] **T307** [FR-5.3] `service/module_gate_service.go`：`IsEnabled(key)` / `EnabledKeys()` / `RequireEnabled(key)`；**带 60s 本地缓存 + 手动失效** (完成证据: module_catalog.go:225 IsEnabled / :267 EnabledKeys / :192 Invalidate；**口径已变**：空态只放行 BaseModuleKeys（IsEnabled ①），缓存改为按租户隔离 map[uint]*gateTenantState——见文末决策记录)
- [x] **T308** [FR-5.4] `middleware/module_gate.go`：`RequireModule(key)`，未开通 → 4301 + 业务码 `module_not_enabled`；执行顺序在鉴权后、权限前 (完成证据: middleware/module_gate.go:100-127；domain/errors/codes.go:31 CodeModuleNotEnabled=4301)
- [x] **T309** [FR-5.3] 路由标注：新增 `module_key` 标签机制（按路由组挂 `Use(RequireModule(k))`），**未标注路由默认不过闸但进告警清单** (完成证据: 落地为 B-1 修订的路径模式匹配——middleware/module_route_patterns.go + GuardModulePath(module_gate.go:133) + 巡检端点 router/module_routes.go:59 GET /api/modules/uncovered-routes)
- [x] **T310** [FR-5.5] `service/menu.go`：菜单树过滤前置模块闸（`FilterMenuTree` 前先按已开通模块裁剪）(完成证据: service/menu.go:60 FilterMenuTreeByModule)
- [ ] **T311** [FR-5.6] 定时任务 / Agent 工具 / EventBus 订阅按 `IsEnabled` 过滤装配 (状态: 菜单过滤与接口闸已实现；未见定时任务/Agent 工具/EventBus 订阅按 IsEnabled 过滤装配的实现——grep 全仓无命中)
- [x] **T312** [FR-5.2] `GET /api/modules/my`：已开通模块 + 依赖 + 定价 + 菜单绑定 (完成证据: router/module_routes.go:48 `auth.GET("/modules/my", ctrl.MyModules)`)
- [x] **T313** 测试：`module_gate_test.go`（超管也过闸 / 缓存 / 失效）· `menu_module_filter_test.go`（208 key 裁剪正确）· `module_sync_test.go`（幂等 / 降级 / 离线）(完成证据: middleware/module_gate_test.go（含 :228 TestGateThreadsRequestContextToChecker）+ service/module_gate_test.go（:110 空态只放基础 / :236 离线 / :272 断网不收回已购 / :393 菜单裁剪）+ model/module_bindings_test.go:233 FilterMenuTreeByModule；未建同名 menu_module_filter_test/module_sync_test，用例并入上述文件)

## P4 · 用户端 · 注册与开户改造

- [~] **T401** [FR-2.1] `model.SystemUser` 增 `company_name` / `legal_person`（负责人）/ `phone` / `email`（邮箱已有）——开户必填集合 (已作废: 落地口径变更——company_name/legal_person 落在 Tenant 模型（model/tenant.go:34/:36），SystemUser 仅 phone(:21)/email；开户必填集合在 tenants 表)
- [x] **T402** [FR-2.1] `dto`：注册/开户请求 DTO（新增 4 字段 + 协议版本 + 监管确认位）(完成证据: service/tenant.go:33-53 `RegisterTenantRequest` 含四字段 + AgreementVersion + RegulatoryAck)
- [x] **T403** [FR-2.1] `service/auth.go` + `service/tenant.go`：`RegisterTenant` / `Register` 必填校验，缺一 → 400 (完成证据: tenant.go:245 validateRegisterReq / :367 validateRegulatoryFields；TestTenantService_Register_ParamInvalid)
- [~] **T404** [FR-2.3] 离线部署：`TENANT_MODE=off` 时仍落本地 `tenants` + 本地 `tenant_registrations` 镜像（`platform_sync='skipped'`），不发任何出站请求 (已作废: 口径变更——TENANT_MODE=off 时 `/api/tenant/register` 恒 4104（tenant.go:28）；离线负责人收集改由 `/setup` init-admin 完成（InitAdminWithRegistration 落 reg_mode=offline、platform_sync=skipped，四轮 Q1）)
- [x] **T405** [FR-2.5] `platform/tenant_registration_client.go`：`POST /merchant-api/tenant/register`；失败 → `platform_sync='pending_sync'` + 登录页提示，**不回滚注册** (完成证据: 落地为 platform/module_client.go:301 `RegisterTenant` + service/tenant.go:446 失败标 failed 不回滚；状态值域为 pending/synced/failed/skipped（无 pending_sync），登录页提示未见实现)
- [x] **T406** [FR-2.5] 监管告知文案：注册页展示，`regulatory_ack` + `agreement_version` 落库 (完成证据: service/module_regulatory.go + controller/module_llm.go:50 RegulatoryNotice；Register.vue:144/:296-297 展示与提交)
- [x] **T407** `user-web/src/views/register/Register.vue`：注册页（公司名称 / 负责人 / 手机 / 邮箱 / 密码 / 协议）(完成证据: user-web/src/views/register/Register.vue 三步向导)
- [x] **T408** `views/Login.vue` 增加"注册"入口开关（受 `GET /api/public/config` 的 `enable_register` 控制，与现状一致）(完成证据: Login.vue:195 注册按钮跳 /register（L1 修复）；开关 :189 受公开配置控制)
- [x] **T409** 测试：`tenant_test.go`（SaaS 开户 + 离线降级 + 必填 + 上报失败不回滚）· `auth_register_test.go`（必填字段 + 协议确认位）(完成证据: service/tenant_test.go（TestTenantService_Register_* :73-184）+ tenant_offline_test.go + tenant_registration_test.go；未建同名 auth_register_test.go，用例并入前者)

## P5 · 用户端 · 功能模块市场

- [x] **T501** [FR-6.3] `model/module_order.go` + `repository/module_order.go`：本地订单镜像（`order_no` / `status` / `total_amount_cents` / `items`）(完成证据: 落地并入——ModuleOrder 在 model/tenant_module.go:126；ModuleOrderRepository 在 repository/tenant_module.go:195)
- [x] **T502** [FR-6.1] `service/module_market_service.go`：列表（分组 + 卡片 + 已开通标记）/ 详情（页面清单 + 依赖闭包 + 定价）(完成证据: 落地为 service/module_market.go:64 `MarketList` / :94 `MarketDetail`)
- [x] **T503** [FR-6.3] 下单：本地建单（`pending_payment`）+ 调平台建单（幂等：本地 `client_order_no` 复用）(完成证据: module_market.go:162-199 ClientOrderNo 幂等；`TestCreateOrderIdempotentByClientOrderNo`)
- [x] **T504** [FR-6.3] **付款指引文案**由后端下发（`payment_guide` 字段），文案含联系方式（`site_contact_config`）(完成证据: platform order_service.go:270 `PaymentGuide`（site_contact 配置驱动）；user 端 module_client.go:72/178)
- [x] **T505** [FR-6.6] 平台未连接时返回明确空态（`platform_enabled=false` → `{"code":0,"data":{"list":[],"platform_enabled":false}}`）(完成证据: module_market.go:60/:175 PlatformEnabled 字段)
- [x] **T506** [FR-6.1] `GET /api/module-market/list` · `/detail/:key` · `POST /order` · `GET /orders` · `GET /orders/:order_no` (完成证据: router/module_routes.go:37-47 全部五个端点)
- [x] **T511** [US4] `user-web/src/views/moduleMarket/Market.vue`：**对标 `assetMarket/Market.vue`** —— 顶部筛选（分组 Tab）/ 卡片网格（封面占位 + 标题 + 分组 tag + 一句话价值 + 依赖提示 + 定价 + 已开通徽标）(完成证据: views/moduleMarket/Market.vue；4301 highlight 定位 :319-464)
- [x] **T512** [US4] `views/moduleMarket/Detail.vue`：左详情右**包含页面清单**（菜单 key → 路径 → 标题，用户能看清"买了能干什么"）+ 依赖闭包 + 定价 + 订阅按钮 (完成证据: Detail.vue:254 marketModuleDetail（L6 修复）+ 匿名 401 内联登录门 :260 + 登录回跳 :273)
- [x] **T513** [US4] `views/moduleMarket/MyModules.vue`：已开通模块卡片（来源：购买/依赖/基础）+ 到期日 + `LLM` 余额条 (完成证据: MyModules.vue:543-546 llmWallet/llmPlans（L5 修复，_silent）；sourceOf 按来源区分 E6)
- [x] **T514** [US4] `views/moduleMarket/Orders.vue`：订单列表 + 状态时间线 + **付款指引卡片**（联系本人）(完成证据: Orders.vue:29/:106/:229 付款指引)
- [x] **T515** 下单二次确认弹窗：列出**依赖闭包明细**（哪些免费自动带、哪些需另购）+ 总价 + 协议勾选 (完成证据: Market.vue:188 二次确认 + :349 依赖闭包拆分)
- [x] **T516** `api/moduleMarket.js` + `router/modules/moduleMarket.js` + 菜单 `system.moduleCenter` (完成证据: api/moduleMarket.js 全函数；router/modules/moduleMarket.js:5-32；菜单 moduleCenter 三节点（L3 修复）+ Detail 页 :20)
- [x] **T517** 测试：`module_market_test.go`（列表 / 详情 / 下单幂等 / 空态）(完成证据: 未建同名文件；用例为 service/module_gate_test.go:299-358（TestMarketListMarksBaseAsFreeAndPaidAsPayable / TestMarketDetailSplitsDependencyClosure 等）+ module_commerce_test.go:304 下单幂等)

## P6 · LLM 双轨与计量

### P6.1 计价口径纠正（前置，必须先做）

- [x] **T601** [FR-7.1] `llm_providers` 增列：`source`(self\|platform) / `platform_plan_code` / `cost_per_1k_input` / `cost_per_1k_output`；旧 `cost_per_1k` 保留作回落；幂等 `ALTER TABLE` (完成证据: model/llm_provider.go:29-39 四列 + 旧列保留；migration v3.62.0)
- [x] **T602** [FR-7.2] `dispatcher_observability.go` `splitCost` 语义改为**分别乘单价**；保留旧行为为 `splitCostRatioLegacy`（仅历史数据回填用，不在热路径）(完成证据: dispatcher_observability.go:257 `computeCost` 分别计价（新热路径）；splitCost :247 保留为已算总价的展示拆分；未建 splitCostRatioLegacy，旧行为以 splitCost 形态保留)
- [x] **T603** [FR-7.1] `dispatcher.go ProviderConfig` / `dispatcher_persistence.go` 行映射同步增字段 (完成证据: dispatcher.go:52-57 ProviderConfig 增 CostPer1kInput/Output + Source + 生效口径方法 :669-706)
- [x] **T604** 测试：`split_cost_test.go`（分别计价 / 单价缺失回落旧列 / 零 token 边界）(完成证据: aiagent/llm/split_cost_test.go)

### P6.2 平台端 LLM

- [x] **T611** [FR-4.1] `platform-server/internal/model/llm_billing.go`：`LLMPlan` / `LLMWallet` / `LLMWalletTxn` / `LLMUsageRecord` / `LLMProviderBinding` (完成证据: model/llm_billing.go:39/:112/:146/:181/:83)
- [x] **T612** [FR-4.2] `repository/llm_billing.go`：`Recharge` / `Consume`（`SELECT ... FOR UPDATE` 行锁）/ `GetWallet` / `ListTxns` (完成证据: repository/llm_billing.go:57/:112/:523 FOR UPDATE/:35/:253)
- [x] **T613** [FR-4.4] `repository/llm_usage.go`：`RecordUsage`（`request_id` 唯一索引幂等，冲突则跳过扣减）(完成证据: 落地并入 repository/llm_billing.go:339 `InsertUsageIfAbsent` + :347 HasUsage；无独立 llm_usage.go；`TestRecordUsageIdempotentByRequestID`)
- [x] **T614** [FR-4.4] `service/llm_wallet_service.go`：充值（挂订单）/ 扣减 / 退款 / 人工调账（每次写流水，`balance_*_after` 落快照）(完成证据: 落地为 service/llm_billing_service.go + repo Refund(:166)/Adjust(:209)；流水快照 model.WalletTxn* + balance_*_after)
- [x] **T615** [FR-4.1] `service/llm_plan_service.go`：套餐 CRUD / 上架下架 (完成证据: repo CreatePlan/UpdatePlan/ListPlans/DeletePlan（llm_billing.go:299-329）+ controller ListPlans 等；并入 llm_billing_service)
- [x] **T616** [FR-4.5] 平台端 Provider 配置：`deliver` 标记 + 真实 Key 保管（**Key 永不下发**）(完成证据: model/llm_billing.go:96 Deliver；platform-web Providers.vue:74-76 Key 只写不回显 + :93 只下发元信息)
- [x] **T617** [FR-4.7] `service/llm_proxy_service.go`：OpenAI 协议转发（`POST /llm-api/v1/chat/completions`）→ 调真实厂商 → 计量扣减 → 响应带 `X-Llm-Usage` (完成证据: service/llm_proxy_service.go + llm_proxy_test.go:58 TestProxyForwardsAndChargesSeparatePrices)
- [x] **T618** [FR-4.7] `controller/llm_proxy_controller.go` + 路由（Bearer = merchant key，**不在 `JWTAuthMiddleware` 组**）(完成证据: controller/llm_proxy_controller.go + router.go:183-186 独立 /llm-api/v1 组)
- [x] **T619** `controller/llm_billing_controller.go`：套餐 / 钱包 / 充值 / 调账 / 用量看板（管理端）(完成证据: controller/llm_billing_controller.go + router.go:337-347)
- [~] **T620** 商户端：`POST /merchant-api/llm/plans`（套餐列表）/ `POST /merchant-api/llm/subscribe`（下单）/ `GET /merchant-api/llm/wallet`（余额）(已作废: 落地为 GET /merchant-api/llm/plans|providers|wallet|wallet/txns（router.go:80-83）；subscribe 无独立端点——LLM 套餐下单并入 POST /merchant-api/orders（order_service.go:533 fulfillLLM）)
- [x] **T621** 测试：`llm_wallet_test.go`（并发扣减不超扣 / 幂等 / 余额不足拒绝）· `llm_usage_test.go`（`request_id` 幂等）· `llm_proxy_test.go` (完成证据: 未建同名 wallet/usage 文件——用例在 module_commerce_test.go:483/:522/:581（TestWalletConcurrentConsumeNeverOverdraw 等）+ llm_proxy_test.go 全套)

### P6.3 用户端 LLM

- [x] **T631** [FR-7.3] `model/llm_wallet.go` + `repository/llm_wallet.go`：本地余额镜像 + 流水 (完成证据: 落地并入——LLMWallet/LLMWalletTxn 在 model/tenant_module.go:158/:173；repository/llm_wallet.go 独立存在)
- [~] **T632** [FR-7.3] `service/llm_wallet_service.go`：`Balance()` / `Precheck(scenario)` / `Deduct(promptTokens, completionTokens)` / `TopupMirror` (已作废: 本地预扣设计作废——本地钱包为只读镜像（service/llm_wallet.go:54 Wallet / :122 Txns / :95 Plans），预检与扣减权威在平台代理侧（llm_proxy 预检 + 计量），见 LLM_BILLING_FLOW.md)
- [~] **T633** [FR-7.1] `platform/llm_provider_client.go`：拉取平台下发 provider 元信息（**无 Key**），落 `llm_providers(source='platform')` (已作废: 未建独立 client——provider 元信息经 platform/module_client.go:275 `Providers()` 拉取，落库走既有 provider 持久化)
- [x] **T634** [FR-7.3] `platform/llm_proxy_client.go`：调用平台代理 (完成证据: platform/llm_proxy_client.go 存在)
- [x] **T635** [FR-7.3] `dispatcher_dispatch.go`：`source='platform'` 的 provider 走代理路径；返回后按分别计价扣减本地镜像并上报计量 (完成证据: llm.go:483 callProvider 分叉到 callPlatformProxy（llm_platform_proxy.go:55）；dispatcher_dispatch.go:315 usage sink 回填；R1/R2 修复)
- [x] **T636** [FR-7.4] 余额不足 → 返回 `llm_balance_insufficient`；有降级 provider 则降级 (完成证据: domain/errors/codes.go:98 CodeLLMBalanceInsufficient=4401；llm_routing.go:149；R8 修复)
- [x] **T637** [FR-7.5] `source='self'` 路径**零改动**（不扣钱包、不上报计量）(完成证据: llm_platform_proxy_test.go:393 TestSelfProvidersUnaffectedWhenPlatformUnconfigured + :189 TestUsageSinkFiresOnlyForPlatformProvider)
- [x] **T638** `GET /api/llm/wallet` + `GET /api/llm/wallet/txns` + `GET /api/llm/plans` (完成证据: router/module_routes.go:50-52 auth 组三端点 + controller/module_llm.go:61/:72/:84)
- [x] **T641** [US5] `user-web`：LLM 页面增 `source` 选择（自带 Key / 平台套餐）+ 平台套餐购买弹窗 + **余额条**（顶部低余额告警）(完成证据: llmRouting/List.vue:58-63 source 列（自带 Key/平台垫资）+ 分列成本；余额卡片与套餐购买弹窗落在 MyModules.vue:543-546（L5 修复落点）)
- [~] **T642** `CostDashboard.vue` 增"输入/输出分列"成本展示（呼应口径纠正）(已作废: CostDashboard.vue 未改（仍只展示 total_cost，:117-122）；分列成本列落在 llmRouting/List.vue:112（L7 修复），口径纠正目标已达成但落点不同)
- [~] **T643** 测试：`llm_wallet_deduct_test.go`（扣减 / 不足 / 并发）· `dispatcher_platform_source_test.go` (已作废: 未建同名文件；等价用例为 llm_platform_proxy_test.go（:117 余额不足可辨识 / :251 TestDispatchPlatformProviderEndToEnd）+ module_commerce_test.go 钱包并发用例 + split_cost_test.go)

## P7 · 平台端 LLM 页面

- [~] **T701** [US5] `platform-web/src/api/llmBilling.js` (已作废: 未单独建文件；`llmApi` 并入 api/module.js:65)
- [x] **T711** `views/platform/llm/Providers.vue`（含 `deliver` 开关 + Key 只写不回显）(完成证据: llm/Providers.vue:12 Key 永不下发告警 + :74 只写不回显)
- [x] **T712** `views/platform/llm/Plans.vue`（套餐：token 数 / 输入单价 / 输出单价 / 价格 / 有效期）(完成证据: llm/Plans.vue)
- [x] **T713** `views/platform/llm/Wallets.vue`（钱包列表 / 充值 / 调账 / 流水抽屉）(完成证据: llm/Wallets.vue:165/:182/:204)
- [x] **T714** `views/platform/llm/Usage.vue`（用量看板：按租户/模型/场景，输入输出分列）(完成证据: llm/Usage.vue:105)
- [x] **T715** 路由分组 `LLM` + Layout 图标 (完成证据: router/modules/platform.js:94-108 'LLM 计费' 分组；Layout.vue:123 GROUP_ICONS)

## P8 · 收敛与门禁

- [x] **T801** [G1] 208 个 menu_key 归属自检测试进 CI（`make test-go` 覆盖）(完成证据: model/module_bindings_test.go + module_seed_test.go:16 TestSeedFileCoversAllMenuKeys，随 make test-go)
- [x] **T802** [FR-5.3] 路由标注覆盖率检查：未标注路由清单打印 + 人工确认表 (完成证据: middleware/module_gate.go:69 UncoveredModuleRoutes + :87 noteUncovered；巡检端点 router/module_routes.go:59 GET /api/modules/uncovered-routes；独立"人工确认表"文档未见，以巡检端点为准)
- [x] **T803** 依赖环检测端到端测试（管理端建边 → 400）(完成证据: module_commerce_test.go:75 TestAddDependencyRejectsCycle + :113 TestSetDependenciesRejectsSwapCycle)
- [x] **T804** 订单并发确认 CAS 测试（两个请求同时确认 → 恰好一个成功）(完成证据: module_commerce_test.go:390 TestMarkPaidConcurrentCASOnlyOneWins)
- [x] **T805** 离线降级端到端测试（`PLATFORM_ENABLED=false` → 零出站请求 + 基础模块全开 + 付费模块锁住）(完成证据: module_gate_test.go:236 TestOfflineBootstrapOpensBaseModulesOnly + platform/zero_egress_test.go)
- [x] **T806** 支付回调幂等测试（同一 `callback_id` 两次 → 只处理一次）(完成证据: module_commerce_test.go:744 TestPaymentCallbackIdempotent)
- [x] **T807** 平台 LLM 端到端测试（下单 → 确认 → 钱包入账 → 调用 → 扣减 → 余额不足阻断）(完成证据: llm_platform_proxy_test.go:251 TestDispatchPlatformProviderEndToEnd + module_commerce_test.go:338 TestMarkPaidOpensModules / :522 TestWalletConcurrentConsumeNeverOverdraw)
- [ ] **T808** 文档更新：`docs/architecture/MODULAR_MODULES.md` 补"实现后实测"列；`user-server/docs/dev/FEATURES.md §二十四` 更新（"定价方案/注册开户已下线" → 已实现）(状态: MODULAR_MODULES.md 已补「二之二、数据表归属总表」（Q6）；但「实现后实测」列未见，且 FEATURES.md §二十四:361-362 仍写「定价方案 ❌ / 注册开户 ❌ 已下线」——与现实现矛盾，未更新)
- [x] **T809** 门禁全绿：`make audit` `make lint` `make vet` `make test-go` + `user-web npm run test` + `platform-web npm run build` (完成证据: 本文件五/六轮门禁表——user-server gofmt/build/vet 全绿、platform-server go test 全绿、user-web 632/632、platform-web build 通过；本次对账仅改 .specify 文档未重跑代码门禁)

---

## Deviations（跳过项与原因）

> 逐条状态与证据已内联到各任务行（完成证据 / 状态 / 已作废）。仍未完成项：
>
> - **T311**（定时任务/Agent 工具/EventBus 按 IsEnabled 过滤装配）：未实现，全仓 grep 无命中。
> - **T808**（文档收尾）：FEATURES.md §二十四 仍写「定价方案/注册开户已下线」，与现实现矛盾；MODULAR_MODULES.md「实现后实测」列未补。
>
> 16 条 `[~]` 任务为方案被后续审计轮替代（文件合并、落点迁移、口径收紧），原因见各条注记。

---

## 执行顺序约束

```text
T001 → T002 → T003 → T004 → T005        （P0 种子，前置一切）
T101…T130（P1 后端）→ T201…T222（P2 页面）
T301…T313（P3 模块闸）→ T401…T409（P4 注册）
T501…T506 → T511…T517（P5 市场）
T601…T604（P6.1 口径）→ T611…T621（P6.2 平台）→ T631…T643（P6.3 用户端）→ T701…T715（P7 页面）
P8 收敛
```
---

## 二次论证结论（Brainstorm 修订，2026-10-17）

| # | 原方案 | 问题 | 修订 |
| --- | --- | --- | --- |
| B-1 | 路由「按组挂 `Use(RequireModule(k))`」 | 2000+ 条路由散在 37 个文件，跨模块交叉引用严重（`ai-sales` 的接口要用 `customer` 的 service），逐组挂会漏挂且无法测试覆盖 | **改为按路径模式匹配**，形态完全复刻仓内既有的 `model/menu_capability_routes.go`（251 条能力面）。生成件 `module_route_patterns.go`，由 `scripts/menu-module-audit.py --emit-go` 产出，与菜单绑定同源 |
| B-2 | 「未标注路由默认不过闸」 | 关模块后仍能调接口 = 收费模块被免费用，是**商业模式漏洞** | 三态：①**模块数据为空**（首启/离线）→ 全放行 + WARN 一次（保可用）②有数据且命中模式 → 过闸 ③有数据但未命中模式 → **放行 + 记 WARN + 暴露 `GET /api/modules/uncovered-routes` 巡检**，并在 CI 里让新增未覆盖路由变红。**[2026-10-18 作废①：空态「全放行」改为只放行 `model.BaseModuleKeys()`（与菜单同口径），见文末决策记录]** |
| B-3 | `ModuleCatalog` 只在平台端 | `SystemMenuTree` 的归属判断只有 user-server 能做，平台端拿不到「这个菜单 key 叫什么、是不是目录」 | 归属表**以 user-server 为事实源**（`internal/model/module_bindings.go`），脚本导出 JSON 给平台端做 seed，两端各有一条测试钉死 208 全覆盖 |
| B-4 | `module_order` 本地镜像表 | 一期没有支付、订单状态由平台单方面权威，本地镜像只是缓存 | 保留但降级为**只读缓存**（列表/详情读本地，状态以平台同步为准），不做本地状态机，避免两套状态打架 |
| B-5 | 平台 LLM 代理是唯一正确做法 | 确实如此，但它是本轮最大风险面（OpenAI 协议转发 + 流式 + 计量） | 代理实现放 P6.2 末尾，**先做计价口径纠正（T601-T604）与钱包（P6.2 前半）**，代理独立成一个可验证子任务；代理不可用时 `source='platform'` 的 provider 自动降级为 `disabled`，不影响自带 Key 路径 |
| B-6 | 注册改造直接改 `RegisterTenantRequest` | 工作区已存在**未提交**的 `internal/{controller,service}/tenant.go` + `tenant_routes.go`（前一轮遗留） | 在其上**增量扩展**（加 4 个字段 + 校验 + 上报），不重写；改动前先 `git stash list` 与 diff 核对归属 |
| B-7 | 基础模块 7 个 | `chatChannelList`（客服 Web Widget 渠道）原归 `inbox`，但它是 `embed-sdk` 外部站点的凭据来源，关掉 `inbox` 会让客户网站上的客服窗挂掉 | 归 `core`（不可关闭）。同理 `systemMaterialLibrary` 归 `core` |
| B-8 | `im-intl` 6 个渠道合一 | 用户将来可能只想买 WhatsApp 不买 TG | `module_options` 表已预留（`option_key` + `price_delta_cents`），一期 UI 勾选子渠道不单独定价，二期打开 delta 即可，**不改数据模型** |
| B-9 | 订单超期高亮 | 「超期」在本期无意义（无自动超时） | 改为「待确认时长」排序 + 平台端首页待办卡片计数 |
| B-10 | 收费模块 `price>0` 离线锁住 | 用户会投诉「我买了却用不了」 | 补一条：**平台确认后可导出模块授权文件**，用户离线部署时导入（`module_license.json`，含签名）。本期只做**数据结构 + 导入导出 API**，不做签名算法（留 `signature` 字段占位） |

---

## 实施记录（2026-10-17 一轮落地）

| 批次 | 状态 | 交付 |
| --- | --- | --- |
| P0 种子 | ✅ | `user-server/internal/model/module_bindings.go`（24 模块元数据 + 208 key 归属）· `cmd/gen-module-bindings`（导出 JSON）· `module_bindings_test.go`（全覆盖/无重复/无环/基础模块数）· `platform-server/seed/module_menu_bindings.json` |
| P1 平台端后端 | ✅ | `model/{module_catalog,order,llm_billing}.go` · `repository/{module,order,llm_billing,tenant_registration}.go` · `service/{module_catalog,module_grant,order,tenant_registration,llm_billing,llm_proxy}_service.go` · `controller/{module_catalog,order,tenant_registration,llm_billing,llm_proxy,payment_callback}_controller.go` · `router.go` 新增 8 组路由 + `/llm-api/v1/chat/completions` · `utils/db/db.go` 19 张新表（AutoMigrate + 幂等 DDL 双保险） |
| P1 测试 | ✅ | 17 条业务用例 + 4 条 seed 用例 + 6 条代理用例（真实 PostgreSQL）：依赖环检测 / 在用检测 / 基础模块不可变 / 下单幂等 / **并发确认 CAS** / **并发扣减不超扣** / 分别计价 / 分别开通过期 / 协议版本不可改写 / 回调幂等 |
| P2 平台端页面 | ✅ | `api/module.js` · `views/platform/module/{Catalog,Grant}.vue` · `views/platform/order/List.vue` · `views/platform/tenant/List.vue` · `views/platform/llm/{Plans,Wallets,Usage,Providers}.vue` · `router/modules/platform.js` + `Layout.vue` GROUP_ICONS · `npm run build` 通过 |
| P3 用户端模块闸 | ✅ | `model/tenant_module.go` · `repository/{tenant_module,llm_wallet}.go` · `service/module_catalog_service.go`（三态 + 离线兜底）· `middleware/{module_gate,module_route_patterns,module_route_supplement}.go` · `router/module_routes.go` + `module_gate_adapter.go` · `service/menu.go` 双闸顺序 · `cmd/gen-module-route-patterns` |
| P3 测试 | ✅ | `middleware/module_gate_test.go`（三态/购买入口不被拦/未被覆盖记巡检/模式表引用真实模块）+ `service/module_gate_service_test.go`（首启全放行/未购付费模块拦截/撤销即失效/过期即失效/**离线只开基础**/**断网不收回已购**/菜单裁剪/市场三档口径） |
| P4 注册开户 | ✅ | `model/tenant.go` 增 8 个登记字段 · `service/tenant.go`（校验 + 上报 + 补同步 + 登记状态）· `service/auth.go` 自助注册同口径 · `repository/tenant.go UpdateRegistration` · `controller/tenant.go`（告知/状态/补同步）· `router/tenant_routes.go`（public 档告知 + admin 补同步）· `views/register/Register.vue`（三步向导）· 16 条校验用例 |
| P5 功能模块市场 | ✅ | `api/moduleMarket.js` · `views/moduleMarket/{Market,Detail,MyModules,Orders}.vue` · `views/register/Register.vue` · `router/modules/moduleMarket.js` + `router/index.js` 模块名/别名登记 · `npm run test` 632 用例全绿 |
| P6.1 计价口径纠正 | ✅ | `model/llm_provider.go` 增 4 列 · `aiagent/llm/dispatcher.go` ProviderConfig 增 4 字段 + 5 个生效口径方法 · `dispatcher_observability.go` 新增 `computeCost`（分别计价）· `dispatcher_dispatch.go` 热路径改用它 · `migration v3.62.0` · `split_cost_test.go` 9 条用例 |
| P6.2 代理客户端 | ✅ | `platform/llm_proxy_client.go`（Bearer=merchant key、上下文走 query 不污染 OpenAI 报文、usage 从响应头取）· `model/llm_provider.go` 增 `source`/`plan_code` · 平台端 `llm_proxy_service.go` 侧已完成 |
| P8 收敛 | ✅ | `gofmt -l` 干净 · `go vet ./internal/...` 干净 · `go build ./...` 通过 · 前端 632 用例全绿 · 平台端全量测试全绿 |

### 实施中发现并当轮修复的存量缺陷（规则 2）

| # | 缺陷 | 后果 | 修复 |
| --- | --- | --- | --- |
| 1 | `rag/retrieval/pgvector_session.go` 声明 `package retrieval`（同目录其余 59 个文件是 `ragretrieval`） | **全仓编译不过** | 改为 `package ragretrieval` |
| 2 | `rag_searcher_vector.go` 用了 `clause.Expr` 但未 import | 该包编译不过 | 补 `gorm.io/gorm/clause` |
| 3 | `TenantGlobalEmailUniqueMigration` 与 `AggregationWatermarkPKMigration` 版本号都是 `v3.60.0` | 迁移注册**启动即 panic** | 邮箱唯一索引迁到 `v3.61.0` |
| 4 | `internal/pkg/tenant/fanout_test.go` 引用不存在的 `ResetRegistryForTest` | `go vet` 不过，且 loader 是进程级状态会污染后续用例 | 在 `fanout.go` 补该函数 |
| 5 | `platform-server` `OrderItem/OrderStatusLog/PaymentRecord/PaymentCallback` 缺 `BeforeCreate` 补 UUID | 第二次写入即撞主键（订单确认时日志刷 duplicate key） | 四个模型各补 `BeforeCreate` |
| 6 | `MarkStatusCAS` 有 `fields` 时**不写 status** | 订单永远停在 `pending_payment`，重复确认反复写凭证，CAS 测试照"更新成功"口径断言而全绿 | status 无条件进更新集 |
| 7 | `AddDependency` 靠捕获唯一冲突实现幂等 | PG 失败语句使事务 aborted，重复加边变成 `commit unexpectedly resulted in rollback` | 改用 `ON CONFLICT DO NOTHING` |
| 8 | `usageCondition` 把条件与值混在一个 `[]any` 里传给 `Where` | 用量统计可能静默返回 0 行（与"这段时间没调用"不可区分） | 改为 `(sql, args)` 明确形态 |
| 9 | `llm_proxy.parseUsage` 只判"解析成功"，usage 全 0 时返回 0 | 厂商省略 usage 时计量静默变 0，商户用了算力不扣费 | 判据改为"三个值全 0 才回落估算" |
| 10 | `RequireModule("")` 走查判定 → 返回 false | 漏传一次 moduleKey 就把那批路由全关掉 | 空 key 在查判定**之前**短路放行 |
| 11 | `GORM` 忽略匿名字段，`TenantModule._ uint` 列从未建出 | 每条 upsert 报 `column tenant_slot does not exist` | 改为具名 `TenantSlot` 字段 |
| 12 | 改唯一索引列集后 AutoMigrate 不删旧索引 | 开发库残留形状不同的同名索引，`ON CONFLICT` 报 42P10 | 测试库显式 DROP 后重建（生产库无此问题：表尚未发布） |
| 13 | `monitoring_service_test.go` 的 API 日志按 `now` 前后推 60 分钟播种 | 每天有两个时段断言恒假（23:00 后跨午夜、00:00 后基准落在未来），表现为「本地好、CI 也过、某时段整片红」 | 锚定「当天 00:00 起往前铺 60 分钟」，与运行时刻无关；`GetTodayCount` 只有 `>= 起点` 无上界，故严格落在今天内即恒成立 |

---

## 二次审核轮（2026-10-18 复审与修复）

复审方式：逐条对照需求原文 + 读实现 + 对每条链路写端到端测试。
**结论：第一轮有 7 处实质缺口，其中 2 处是会直接导致功能不可用的断链。**

| # | 缺口 | 严重度 | 现象 | 修复 |
| --- | --- | --- | --- | --- |
| R1 | `source='platform'` 的 provider **从未接入代理**：`dispatcher` 仍直连厂商，而平台按 ADR-M7 **不下发 Key**，所以该 provider 的 APIKey 恒空 | **阻断** | 用户买了 LLM 套餐 → 模型可选 → 一调用必 401 | `LLMConfig.Source/ProviderName` + `LLMService.callProvider` 单点分叉到 `callPlatformProxy`；`main.go` 装配 |
| R2 | 本地无任何用量落库、无余额镜像 | **阻断** | 平台在扣钱，商户本地看不到任何账 | `llm_usage_records` → `platform_llm_usage_records`（见 R4）+ 可注入 sink + `main.go` 装配 |
| R3 | 平台未配置时 `platform` provider 仍进候选 | 高 | 离线部署后每次 AI 调用都发一次注定失败的请求，用户看到「AI 一直报错」，排查方向被完全带偏 | 候选筛选加 `IsPlatformProxyConfigured()` 门，改为走 fallback |
| R4 | 新表 `llm_usage_records` **与迁移 v3.4.0 的既有表同名**（既有表主键是 uuid、列是 total_tokens/cost，语义是 AI 产能聚合） | **严重（静默）** | AutoMigrate 会按模型把主键改成 bigint，历史行 uuid 失效，而 AI 产能看板不会报任何错 | 改名 `platform_llm_usage_records`，三张表分工写进模型注释；由全链路迁移测试兜住 |
| R5 | 前端不识别业务码 4301 | 中 | 深链到未开通模块只弹「操作失败」，用户不知道要去哪买 | `request.js` 统一拦截 4301：警告 + 3s 跳转市场并带 `?highlight=`；市场页滚动定位并高亮该卡 |
| R6 | 平台代理预检给输出写死 1024 token 下限 | 中 | `output_tokens < 1024` 的小规格套餐**一次都调不通**，商户付了钱却用不了，而日志只是「余额不足」 | 预检只判输入 + 输出余额 > 0；补 `TestProxySmallOutputPlanStillWorks` |
| R7 | 厂商只回 prompt/completion 不回 total_tokens 时，真实用量被估算值覆盖 | 中（计费） | 分列用量丢失 → 按估算扣费 → 商户账单与厂商账单对不上 | 三层判据：total 有则用 / total 无但分列有则**信任分列** / 全无才估算 |

### 复审中发现的既有代码缺陷（同轮修复）

| # | 缺陷 | 后果 |
| --- | --- | --- |
| R8 | `IsBalanceInsufficient` 定义后**无人调用** | 402 余额不足以通用错误冒泡，用户拿不到「去充值」的引导；已接 `bizerr.CodeLLMBalanceInsufficient`(4401) |
| R9 | 模块闸用例往**共享测试库**留行且只在开头清 | `internal/router` 的菜单测试被污染 → 同一份 RBAC 代码「CI 绿、本地红」；已加 `t.Cleanup` |
| R10 | RBAC 菜单测试未隔离模块闸，且「清闸」写在 `Setup()` 之前会被重新装上 | 已改为 `Setup()` 之后清，并补 `TestAdminAlsoSeesModuleGate` 钉死「超管也过闸」 |
| R11 | 继承来的 `tenant_test.go` 夹具未含监管四项 | 开户必填生效后夹具失效；已补四项（**不放宽校验**） |
| R12 | 平台端监控测试的播种时刻随运行时刻漂移 | 先按「锚今天」修 → 又打破 1 小时滚动窗口的兄弟用例；最终改为「锚最近 60 分钟 + 按播种时刻算期望值」，两侧都不再依赖运行时刻 |

### 复审补测（14 条）

- 代理分流：平台走代理 / 自带 Key **绝不**被代理接管 / 余额不足可辨识 / 未配置快速失败 / 错误体截断
- **端到端**：`dispatch → 代理 → 用量回填`（跨模块连线，单点测试测不出）；自带 Key 不产生平台用量事件
- 平台未配置时降级到自带 Key；自带 Key 完全不受影响
- 小规格套餐可调用；输入/输出分别计价的端到端数值
- admin 也受模块闸约束；菜单测试与模块闸隔离

### 复审后的门禁

| 项 | 结果 |
| --- | --- |
| user-server `gofmt -l` / `go build ./...` / `go vet ./internal/...` | 干净 |
| user-server 受影响包测试（model / middleware / aiagent/llm / router / migration×2 / platform / pkg/db / controller / repository） | 全绿 |
| platform-server `gofmt -l` / `go vet ./...` / `go test ./...` | 全绿 |
| user-web `npm run test` | 632/632 |
| platform-web `npm run build` | 通过 |

**已知红（不属于本任务，按 CLAUDE.md 规则 2 如实记录不去改别人的文件）**：
`internal/service` 的 4 条 `TestSettleOrphan*` / `TestRunOnce_CallsOrphanSettlement`。
隔离验证：把工作区里他人未提交的 `internal/repository/bridge_offline_replay_repo.go`
（Raw SQL → GORM query builder 的重构）单独 stash 后，这 4 条全绿 —— 与本次改造无关。

---

## 三次审核轮（2026-10-18 集成断链专项）

审核方式换成**集成视角**：不看单模块对不对，只问「用户从入口到结果的每一步，
数据是谁写的、路由谁注册的、前端调的那个函数后端有没有、前端有没有真的调它」。

**结论：7 处断链，其中 3 处是「功能等于不存在」。**

| # | 断链 | 严重度 | 用户实际会遇到什么 | 修复 |
| --- | --- | --- | --- | --- |
| **L1** | **登录页仍走内联注册表单**，只提交 用户名/密码/邮箱；而后端已把 公司名/负责人/电话/邮箱 定为开户必填 | **功能不存在** | 走登录页注册的**所有人**都被 4304 拒绝，而后端一条错都不报（我只是加了独立注册页，没把入口指过去） | 登录页注册按钮改为跳 `/register`；同步删掉被拖累残留的 `loginForm.email`；更新受影响的既有用例 |
| **L2** | **模块同步 `Sync()` 生产从未被调度**（只在测试里调用） | **商业模式整体失效** | `tenant_modules` 恒空 → 模块闸永远处在「首启全放行」态 → 付费模块对所有人开放，界面上毫无异样。闸的代码全对、单测全过、而闸没生效 | 新增 `service.StartModuleSync`（首轮**同步**执行 + 10 分钟周期），`main.go` 在 `router.Setup` 之前启动 |
| **L3** | **功能模块市场在侧栏里没有任何入口**：路由没有 group、菜单注册表里也没有节点 | **功能不存在** | 需求四的核心功能只能手敲 URL 或从报错跳进来 | 菜单树加 `moduleCenterGroup` 三节点（system 顶级直接子位），绑到**基础模块** platform-link；同步 9 语言 i18n |
| **L4** | `UpdateModel` 不保留 `source` | **破坏性** | 商户在 UI 上把平台模型点一下「停用」，它就降级成 `self`（Key 为空）→ 此后每次调用必 401，而用户只是点了个开关 | Update 一律沿用旧 `source`/`plan_code`/分列单价；Create 强制 `self`（拒绝商户伪造 platform） |
| **L5** | `/api/llm/wallet`、`/api/llm/plans`、`/api/llm/recharge` **前端零调用** | 高 | 计量做了一整套，却没有任何人看得见余额 —— 「实时扣减」对用户不可感知 | 我的模块页加余额卡片 + 套餐购买弹窗；补 api 层三个函数 |
| **L6** | `Detail.vue` 用**列表**接口而非详情接口 + 用裸 `fetch('/api/menus')` | 中 | 「详细说明」与依赖闭包两段永远显示占位文案；菜单清单栏永远只有裸 key（裸 fetch 不带 Authorization，恒 401 而 catch 是空的） | 改用 `marketModuleDetail` + `api/menu` 的 `getUserMenus`；后端详情响应补 `platform_enabled` |
| **L7** | provider 列表 DTO 不含 `source` 与分列单价 | 中 | 前端无法区分「自带 Key 不计费」与「平台垫资按量计费」—— 需求五的核心区分在 UI 上不可见 | DTO 补 7 个字段；LLM 页加「来源」列与输入/输出分列成本列 |

### 连带修掉的存量缺陷

| # | 缺陷 | 处置 |
| --- | --- | --- |
| L8 | `internal/websocket/visitor_handler.go` 用了 `tenant.` 但没 import（他人未提交的改动） | 补 import（编译阻断，不改逻辑） |
| L9 | 平台端 seed 用例把菜单节点总数写死 208 | 改为不写死：真正的判据是「覆盖数 == 文件自报总数 && 重复绑定 == 0」。写死会让「加个菜单节点」变成仪式，迟早被「顺手改数字」绕过 |

### 三轮审核的共同教训（值得写进项目规范）

前两轮我都以「单模块是否正确」为判据，结果连续两轮各漏一处**连线**。
真正能兜住这类缺陷的只有两种断言：

1. **端到端跨模块用例**（`TestDispatchPlatformProviderEndToEnd`：dispatch → 代理 → 用量回填）
2. **「这条数据/这个入口是谁写的」断言**
   - `TestStartModuleSyncBootstrapsBaseModules`：钉住后台循环真的会落数据
   - `TestModuleMarketAlwaysReachable`：钉住购买入口在菜单注册表里且归属基础模块

单点测试（Sync 对不对、闸拦不拦、路由在不在、页面渲不渲染）**全部可以在
"两者之间没人连线"的情况下同时变绿** —— 这是本次三处「功能等于不存在」缺陷的共同成因。

### 门禁

| 项 | 结果 |
| --- | --- |
| user-server gofmt / go build / go vet（涉我改动包） | 干净 |
| user-server service（745s）/ controller / repository / pkg/db / migration×2 / model / middleware / aiagent/llm / router / platform / pkg/tenant | **全绿** |
| platform-server gofmt / go vet / go test ./... | 全绿 |
| user-web npm run test（632）+ build + i18n 检查 | 全绿 |
| platform-web npm run build | 通过 |

---

## 四次审核轮（2026-10-18 需求回读复核）

起因：同一份需求第三次送达，说明前三轮交付仍有未覆盖之处。
本轮做法是**逐字回读需求原文**，把每个动词/名词落到具体实现位置，核对是否真的存在。

| # | 需求原文片段 | 复核结果 |
| --- | --- | --- |
| 「注意离线独立部署情况，**也要有负责人初始化**，也要收集信息」 | ❌ **漏做** | 我改的是注册页（`/api/auth/register`）与自助开户；而**离线用户只走 `/setup` 初始化向导**，那里把公司/负责人/电话/邮箱全部标为「选填」，`InitAdminRequest` 也未强制 —— 需求唯一点名的人群，恰恰是唯一绕过监管收集的人群 |
| 「整理当前所有的功能模块…以及**数据表**」 | ⚠️ **覆盖不足** | 实测 270 张表里 **32 张无主**（`wechat_accounts` / `webhook_events` / `notifications` / `bridge_accounts` 等真实业务表）。表无主的危害不是文档不好看：没人知道它随哪个模块开关，关模块后仍可能被定时任务写入 |
| 「功能模块市场**要论证合理UI，组织架构**」 | ❌ **缺交付物** | 只交了代码，没交论证文档 |
| 「完整的流程**看情况论证补全**」 | ❌ **缺交付物** | 同上 |
| 「菜单组织」 | ⚠️ 部分 | 页面归属完整；但市场入口缺菜单节点（第三轮已补） |

### 本轮修复

| # | 修复 | 测试 |
| --- | --- | --- |
| Q1 | `/api/system/init-admin` 四项改必填；新增 `InitAdminWithRegistration`：建超管 + 回填负责人姓名/电话 + 落一条 `reg_mode=offline` 的 `tenants` 登记（`platform_sync=skipped`）。登记失败**不回滚超管**——用户已能登录，为一条留痕把人挡在门外代价更大 | `ValidateOfflineRegistration` 口径测试；`auth_test.go` 夹具补齐四项 |
| Q2 | `/setup` 向导改为必填公司/负责人/电话/邮箱 + 监管告知勾选 + 完成页展示登记结果 | 前端 632 用例 + build |
| Q3 | 新增 `internal/model/module_tables.go`：**表 → 模块**归属登记表（304 条），把文档清单变成可执行数据 | `TestEveryModelTableHasModuleOwner`（270/270 零孤儿）、`TestNoTableBelongsToTwoModules`、`TestPaidModuleHasTables` |
| Q4 | 新增 `docs/architecture/MODULE_MARKET_UI.md`：入口位置四选一论证、四页职责、三档价格口径、依赖闭包两段展示、4301 全站引导、与资产市场的复用/差异 | — |
| Q5 | 新增 `docs/architecture/LLM_BILLING_FLOW.md`：双轨分工、零出域例外三条件、主时序与二期回调时序、幂等三道防线、分别计价与"按比例拆分"的区别、余额三不变量、错误语义、provider 生命周期 | — |
| Q6 | `MODULAR_MODULES.md` 补「二之二、数据表归属总表」节 | — |

### 连带

- `config_param_guard_test.go` 的 D12 守卫是**文本匹配**，把表归属登记表里出现的 `system_config_kv` 字样当成裸 SQL 直查。加白名单并注明"这是文本匹配无法区分表名登记与裸 SQL 的误报"。

### 门禁

| 项 | 结果 |
| --- | --- |
| user-server gofmt / go build / go vet（涉我改动包） | 干净 |
| user-server service / controller / repository / migration×2 / model / middleware / aiagent/llm / router / platform / websocket | **全绿** |
| platform-server gofmt / go test ./... | 全绿 |
| user-web npm test（632）+ build + i18n | 全绿 |
| platform-web npm run build | 通过 |

### 四轮的共同教训

前三轮我把"需求"理解成"我打算实现的那几条"，第四轮才发现**需求原文里点名的
"离线独立部署"路径我根本没碰**，而**"以及数据表"我默认成了文档里写几句**。
教训：逐字回读需求里**带限定词的从句**（"注意…"、"也要…"、"以及…"），它们标出的
恰恰是最容易被跳过的范围边界。

---

## 五次审核轮（2026-10-18 真实启动验证）

起因：前四轮都是**静态审查 + 单测**。本轮改为**把系统真跑起来**，端到端打 API。
结论：**静态审查与单测全部无法覆盖的 6 处缺陷，其中 2 处会让整个收费模式静默失效。**

### 关键方法差异

单测跑在**干净测试库**上（测试自己 DropTable 重建），而真实部署跑在**比代码老的库**上。
"索引形状漂移""配置加载时序""迁移体系不执行"这三类问题**只在后者出现** ——
这就是为什么前四轮全绿却仍有致命缺陷。

| # | 缺陷 | 现象 | 根因 | 修复 |
| --- | --- | --- | --- | --- |
| **E1** | **平台配置加载时序错误** | `[llm-provider] 拉取平台 provider 失败: 平台配置未初始化` —— 平台 provider 永远拉不到 | 我把 `SyncPlatformProviders` 放在第 303 行，而 `LoadPlatform` 在第 336 行。**代码写在配置就绪之前** | 移到 `LoadPlatform` 之后，并在拿到 provider 后重载一次路由表 |
| **E2** | **模块下发 42P10，闸恒放行** | `[module-sync] 首轮同步失败: there is no unique or exclusion constraint matching the ON CONFLICT specification`。同步失败 → `tenant_modules` 恒空 → **付费模块对所有人开放** | ① `tenant_modules` 的唯一索引是**旧形状** `(tenant_id, tenant_slot, module_key)`，而代码按 `(tenant_slot, module_key)` 做 ON CONFLICT；② AutoMigrate **不删除形状已变的索引** | 新增 `internal/pkg/moduleidx`：随每次启动幂等规范化索引形状（紧跟 AutoMigrate，与既有 `tenancyidx.Normalize` 同形） |
| **E3** | **迁移体系根本不执行迁移** | 我写的 `v3.63.0` 迁移在 `migration_records` 里显示 `completed`，但**索引形状一点没变** | `main.go` 调用 `ExecuteUpgrade(ctx, "v1.0.0", "v1.0.0")` —— from/to 相同，不跑任何迁移。写进迁移 = 写进一个永远不跑的地方 | 删除该迁移文件（避免"看着像修了"的假象），修复落到 `moduleidx`；把这条写进 `moduleidx` 的文件头注释 |
| **E4** | **seed 数据从未真正落地** | `POST /platform/module-catalog/seed` 报 `open seed/module_menu_bindings.json: not a directory`。**平台端从来没有模块目录** | ① `-out ../hivemtk-platform/...` 在 `user-server` 下执行时解析成 `hivemtk/hivemtk-platform/…`（多一层），**而命令照样打印"已生成"**；② 仓库里 `platform-server/seed` 是一个未入库的构建残留**二进制**，同名冲突 | 生成器改为**向上找工作区根**（认 `hivemtk` + `hivemtk-platform` 两个子目录）而不是猜层级，并加**写后存在性断言**；目录改 `config/`；`Seed()` 改**编译期内嵌**（`//go:embed`），磁盘文件降级为可选覆盖 |
| **E5** | **启动 panic，服务起不来** | `panic: ERROR: default for column "tenant_id" cannot be cast automatically to type bigint` | GEO 模块模型已把 `tenant_id` 改成 `uint`，而遗留库里该列仍是 `varchar`（default `''`）。`42804` 不在可容忍清单 → `migrate.go` 直接 panic | 把该错误加入 `isTolerableMigrateError`。理由：**一个环境的历史 schema 漂移不该让整个进程起不来** —— panic 的伤害远大于漂移本身 |
| **E6** | **「我的模块」把所有模块记成"用户购买"** | 基础能力（core/customer/inbox…）在页面上显示为用户花钱买的 | `sourceOf(ownedStatus)` 收到的是**状态**（active/expired）而非**来源**，于是恒返回 `purchase` | 平台端下发 `owned_source`；`sourceOf` 按模块分类回落（基础→`default_base`） |

### 端到端验证（真实双服务 + 真实双库）

| 验证项 | 结果 |
| --- | --- |
| 平台端 seed 目录 | 24 个模块 / 212 菜单节点 / 0 重复 |
| 商户 HMAC 下单（geo） | `¥2980` = `298000` 分，状态 `pending_payment` |
| 0 元模块下单 | 金额 `0`，仍走订单流程（不是"免费直开"） |
| 平台端确认收款 | `fulfilled`，`modules:["geo"]` |
| **基础模块自动开通** | 确认前 `tenant_modules` 已有 7 条（`default_base`） |
| **依赖闭包** | geo 开通时自动带上 `knowledge` 等 |
| 重复确认收款（幂等） | 返回成功，`payment_records` **仍为 1 条** |
| 依赖环拒绝 | `409 模块依赖存在环` |
| 基础模块改价拒绝 | `403 基础模块不可修改` |
| 撤销被依赖模块 | `409 …仍被已开通的 [asset geo] 依赖` |
| **用户端模块闸（菜单）** | admin 只看到 **96/212** 节点；geo 菜单在，**ai-sales 菜单全部消失** |
| **用户端模块闸（接口）** | 未开通的 ai-sales API → **`4301`**；已开通 geo → 落到业务层；基础模块 → `code 0` |
| **离线开户登记** | 四项必填校验 + `tenants` 行落库（`reg_mode=offline` / `platform_sync=skipped`）+ 超管 `real_name`/`phone` 回填 |
| 全新安装 AutoMigrate | 6 张新表随基座正常建出 |

### 新增回归测试

- `internal/service/offline_registration_test.go`：离线开户登记（**判据是"租户行真的写出来了"**，不是"校验函数返回 nil"）
- `internal/pkg/moduleidx`：索引形状规范化（随启动幂等执行）

### 已如实记录的既有问题（不属本任务，未改他人文件）

| 项 | 说明 |
| --- | --- |
| `internal/pkg/testutil/testdb.go` 新增 `import db` | 构成环 `model(测试) → testutil → db → model`，阻断 `go test ./internal/model/`。**他人未提交改动**。隔离验证（临时还原该文件）后 `internal/model` 测试通过，已恢复其文件 |
| 遗留库 GEO 表 `tenant_id` 为 varchar | 历史 schema 漂移，非本任务引入。已通过"容忍 + 不 panic"解除启动阻断 |

### 六轮审核的总教训

前四轮我以"单模块是否正确 + 需求是否逐条覆盖"为判据，全部通过，但**系统仍然跑不通**。
本轮把服务真拉起来，一次性暴露 6 处 —— 其中 E2/E3 是**同一类问题的两个层次**：
"代码写对了，但它运行在一个比代码老的环境里"。

**判据必须包含"对既有环境成立"，而不只是"对新建环境成立"。**

---

## 六次审核轮（2026-10-18 通信与身份模型专项）

按要求梳理「平台端 ↔ 用户端」的通信、协调、身份与数据归属，产出一份代码级架构文档，
并在梳理过程中发现并修掉 1 处实质缺陷。

**交付物**：`docs/architecture/PLATFORM_USER_COMMUNICATION.md`
（5 条通道 × 3 种鉴权、纯拉取模型、身份模型、数据归属矩阵、模块与商户两条完整数据流、
6 条架构问题清单）

### 本轮发现的缺陷

| # | 缺陷 | 后果 | 修复 |
| --- | --- | --- | --- |
| **F1** | `SubmitRegistrationToPlatform` 与 `submitSelfRegisterToPlatform` **都把 `install_id` 填成 `merchant_key`** | 平台端 `tenant_registrations.install_id` 存的是商户密钥，而 `platform_installs` 用的是真 install_id —— **两表没有可连接的键**，"这个部署是谁 / 它在线吗 / 它登记了吗"三个问题在平台上答不出来 | 新增 `currentInstallID()` 从 `install.lock` 读；读不到返回**空串**而非回落 merchant_key（混填比空值更难排查）。由 `TestRegistrationPayloadKeepsTwoIdentitiesDistinct` 钉死 |

### 本轮补充的防误解注释（不改变行为）

| 位置 | 说明 |
| --- | --- |
| 平台 `model.Merchant.InstallID` | 标注该列**基本恒空**（写它的唯一路径 `/api/platform/install` 在用户端生产零调用），**不可**当关联键 |
| 用户 `model.TenantModule` | 标注**平台端有同名表但结构完全不同**，列出两侧列清单与语义差别 |

### 梳理中确认为「现状而非缺陷」的三条

| 项 | 判断 |
| --- | --- |
| 纯拉取模型 → 模块开通最长延迟 10 分钟 | 产品取舍，非缺陷。前端付款指引已写明"确认收款后权益自动开通"，用户不会误判失败。缩短周期或加推送是另一个决策 |
| `/api/platform/install` 零调用 | 保留有价值（"按 install_id upsert 商户"的语义出口），但 `merchants.install_id` 恒空这件事已在列注释写明 |
| 两端 4 张表同名不同构 | 有意的镜像命名（一眼看出是同一概念的两侧），已在用户端模型注释里互指 |

---

## 2026-10-18 全链路检查决策记录

本节记录全链路审计（含 9c11a888、a28b7c27 及其后工作区改动）中**改变了既有方案口径**的决策。
任务行内的证据以读码核实为准；本节为决策汇总，供后续实现与复审对照。

| 决策 | 理由 | 代码落点 |
| --- | --- | --- |
| **闸空态口径：allow-all → 只放行 `model.BaseModuleKeys()`** | 空态（首启/离线/同步未完成）全放行 = 付费闸在最脆弱窗口整体失效，与菜单过滤口径不一致；只放基础模块与菜单同口径，堵 on 模式付费闸失效 | user-server/internal/service/module_catalog.go `IsEnabled` ① 注释与实现（:38/:225）；`TestModuleGateEmptyBootAllowsOnlyBaseModules` |
| **闸缓存：进程单份 → 按租户隔离（map[uint]*gateTenantState）** | 进程单份缓存会把租户 A 的开通状态喂给 B（串租户），多租户下是越权 | module_catalog.go:54 `gateTenantState` / :68 tenants map / :135 `gateTenantOf`；`TestModuleGateTwoTenantCacheIsolation` |
| **激活归属：付费模块行落下单租户（purchaseTenants 路由）** | 同步按 order 激活时若不绑定下单租户，租户 2 的购买会激活到租户 1（白得模块 / 钱照扣） | module_catalog.go:521 `purchaseTenants`；`TestModuleSyncActivatesPaidModuleOnOrderingTenant` |
| **on 模式 init-admin：先建租户行 → 超管显式归属** | on 模式下 `/setup` 只建超管会让超管悬空在默认租户上，租户登记与超管归属断链 | service/auth_init_admin_onmode_test.go `TestInitAdminWithRegistrationOnModeBindsTenant` |
| **init-complete 接线：localStorage-only → POST /api/system/init-complete** | 只翻前端标记则后端安装锁未落，App 每次启动都强推 `/setup` | user-web/src/utils/initHelper.js:22-31（markInitializationComplete POST，_silent）；router/admin_routes.go:65 端点 |
| **市场匿名浏览：requiresAuth:false + 详情 401 内联登录门 + 登录回跳** | 购买入口必须匿名可逛（橱窗），但详情接口在鉴权档——匿名撞 401 时给内联登录引导而非白屏；登录后回跳原详情页 | user-web router/modules/moduleMarket.js:12/:20 requiresAuth:false；Detail.vue:260 401/403 内联门 + :273 login?redirect= 回跳 |
| **llm 闸：保留 wallet/plans→llm-cloud（激活走市场，无死锁）**；过期"不能拦"注释已改 | wallet/plans 挂 llm-cloud 闸、下单入口（/api/llm/recharge + 市场）不过闸 → 未订阅 4301 引导去市场，购买链路永远开着，不存在死锁 | router/module_routes.go:19-22 注释 + :50-53；middleware/module_route_supplement.go 归属 |
| **协议版本：后端 tenant.go 单一字面量，前端/告知端点派生** | 两端各写版本号必然漂移；单一来源 + 告知端点下发，前端提交时取自告知响应 | user-server/internal/service/tenant.go:360 `serviceCurrentAgreementVersion = "2026-10-17.v1"`；前端 InitSetup.vue:358 与 Register.vue:297 从 /api/tenant/regulatory-notice 取 |
| **MFA/邮箱/租户代码/主动续期：登录链路四件套** | MFA 二步验证、邮箱登录、tenant_code 通道、令牌 15 分钟窗口主动单飞续期，构成完整登录链路 | Login.vue:109/:246 MFA（need_mfa→POST /api/auth/mfa/verify）+ :57 tenant_code；request.js:177 15min 窗口 / :181 单飞锁 / :222 refresh-token；users.js:34 |
| **无 e2e 的处置：已补链路关键回归；浏览器级 e2e 列为后续基建项** | 六轮审计证明单测全绿仍会断链；已补跨模块断言：`TestInitAdminWithRegistrationOnModeBindsTenant`、`TestModuleGateTwoTenantCacheIsolation`、`TestModuleSyncActivatesPaidModuleOnOrderingTenant`、`TestGateThreadsRequestContextToChecker`、`TestModuleGateEmptyBootAllowsOnlyBaseModules`、`TestModuleSyncRefreshesOrderStatus`；浏览器级 e2e（Playwright 等）未建，列为后续基建 | user-server/internal/service/、internal/middleware/module_gate_test.go:228 |

### 本节对账统计

- 全部任务 119 条：**101 条 `[x]`**（附完成证据，均读码核实）、**16 条 `[~]`**（方案被替代，附原因）、**2 条 `[ ]`**（T311、T808 未完成，附状态）。
