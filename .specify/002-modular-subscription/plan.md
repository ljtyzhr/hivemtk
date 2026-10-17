# Implementation Plan: 功能模块化 · 按需订阅收费改造

**Spec**: `./spec.md` · **模块清单**: `docs/architecture/MODULAR_MODULES.md`
**Date**: 2026-10-17

---

## 一、总体架构

```text
┌───────────────────────── hivemtk-platform（平台端 / 唯一运营入口）─────────────────────────┐
│                                                                                            │
│  platform-web (Vue3)                                                                       │
│    ├─ 驾驶舱 / 商户 / 心跳 / 监控（既有）                                                     │
│    ├─ 功能模块：模块目录 / 定价 / 依赖 / 菜单绑定 / 下发                        ← 新增 P1     │
│    ├─ 租户：租户列表 / 开户登记详情                                        ← 新增 P1         │
│    ├─ 订单：订单列表 / 确认收款 / 作废 / 凭证                    ← 新增 P1                   │
│    ├─ LLM：Provider 配置 / 套餐 / 钱包 / 用量看板                            ← 新增 P4        │
│    └─ 资产市场（既有）                                                                       │
│                    ↓                                                                        │
│  platform-server (Go / 五层)                                                                │
│    controller → service → repository → model                                                 │
│    /platform/*        管理端（JWT + RequirePlatformAdmin + Audit）                           │
│    /merchant-api/*    商户端（HMAC）                                                         │
│    /llm-api/*         平台 LLM 代理（Bearer = merchant key）              ← 新增 P4          │
│    /platform/payments/notify/:channel   线上回调占位（501）                ← 新增 P1          │
└────────────────────────────────────────────────────────────────────────────────────────────┘
                    ▲ HMAC / Bearer                        │ 模块同步 / 订单 / 计量
                    │                                     ▼
┌───────────────────────── hivemtk（用户端）──────────────────────────────────────────────┐
│  user-web (Vue3)                                                                            │
│    ├─ 注册/开户向导（含公司/负责人/监管告知）                            ← 新增 P2            │
│    ├─ 功能模块市场：Market / Detail / MyModules / Orders               ← 新增 P3            │
│    └─ LLM：provider 管理（self/platform 区分）+ 余额条 + 成本看板      ← 新增 P4            │
│                    ↓                                                                        │
│  user-server (Go / 五层)                                                                    │
│    middleware.RequireModule(key)   模块闸（鉴权后、权限前）                ← 新增 P2            │
│    service/module/*                 模块同步 / 闸判定 / 依赖闭包            ← 新增 P2           │
│    model.TenantModule               本地镜像                                ← 新增 P2          │
│    model.ModuleOrder / LLMWallet    本地镜像                                ← 新增 P2/P4       │
│    aiagent/llm/dispatcher          双轨计价 + 实时扣减                      ← 新增 P4         │
└────────────────────────────────────────────────────────────────────────────────────────────┘
```

## 二、技术决策（ADR 形式）

### ADR-M1 · 模块闸用「中间件」而非「拆 Setup」

**决策**：在 `router.Setup()` 里给现有路由**加标注**（模块 key），由 `middleware.RequireModule(key)` 在链路上拦截。

**理由**：
- `router.Setup()` 目前注册 37 个路由文件、2000+ 条路由，拆成 24 个 `Setup` 函数会引入 (a) DI 装配顺序问题、(b) 模块间交叉引用（`ai-sales` 的接口要用 `customer` 的 service）、(c) 现有 `router_*_test.go` 大面积失效。
- 中间件方案对现有代码侵入最小：一处 `group.Use()` + 一处能力查询。
- 代价：路由注册期不做裁剪（全部注册、运行期拦截）。对本项目（单体部署、路由表 2000 条）性能可忽略。

### ADR-M2 · 平台端 `order` 单据统一三类

**决策**：`orders` 表用 `order_type` 区分 `module` / `llm` / `asset`，共用状态机与支付凭证。

**理由**：三类购买的生命周期一致（提交→确认→开通/入账），拆三套表会让平台端 UI 出现三个"订单"列表，运营要记三个入口。

### ADR-M3 · 「支付回调复用」的实现形态

**决策**：`OrderService.MarkPaid(ctx, orderNo string, notice PaymentNotice)` 是**唯一**入账函数。
- 一期线下：`source=offline_manual`，`operator=管理员ID`，管理端按钮直接调。
- 二期线上：`POST /platform/payments/notify/{channel}` 验签后构造**同一个** `PaymentNotice` 调 `MarkPaid`。

**理由**：需求明确要求"支付回调复用"。若一期线下路径自己写一套 SQL，二期接回调时必然要重写且容易漏幂等。契约先立，通道后接。

### ADR-M4 · LLM 双轨计价

**决策**：
- `llm_providers` 增 `source` 列；`cost_per_1k` 拆 `cost_per_1k_input` / `cost_per_1k_output`（旧列保留作回落）。
- `splitCost` 语义**改变**：从"按 token 比例拆分单一总价"改为"分别乘单价"。
- 平台 LLM 走平台代理（`/llm-api/v1/chat/completions`），计量与扣费在平台侧权威执行，用户端只做**本地预扣镜像**用于快速失败。

**理由**：
1. 用户要求"输入和输出分别计价"，现状 `splitCost` 是把一个总价按比例劈开 —— 数学上等价于单一单价，**不满足需求**，必须改。
2. 平台代理是唯一能让余额强一致的做法：本地异步上报存在"超用后才扣"的风险。
3. 零出域铁律例外必须有：用户显式开通 + 注册协议告知（FR-4.7）。

### ADR-M5 · 模块菜单绑定放独立表，不进 `SystemMenuNode`

**决策**：`module_menu_bindings(module_key, menu_key)` 独立表。

**理由**：`SystemMenuNode` 是编译期常量；模块开通是运行期可变数据。混在一起会让"离线=基础模块全开"规则无处安放，且需要改 208 个结构体字面量。

### ADR-M6 · 模块订购按「单模块 + 依赖闭包」，不做套餐

见 `MODULAR_MODULES.md §5.1`。

### ADR-M7 · 平台 LLM 的 Key 不下发

**决策**：平台端 provider 标记 `deliver=true` 时下发**元信息**（name/model/base_url/api_type/source='platform'/plan_code），**API Key 一律不下发**。user-server 调平台代理，平台侧持有真实 Key。

**理由**：下发 Key 等于把平台垫资凭据交给商户，商户可无限量直连厂商，平台无法计量也无法止损。

## 三、目录与文件计划

### 3.1 平台端新增（hivemtk-platform）

```
platform-server/internal/
  model/
    module_catalog.go        ModuleCatalog / ModuleDependency / ModuleMenuBinding / ModuleOption
    tenant_registration.go   TenantRegistration
    order.go                 Order / OrderItem / OrderStatusLog / PaymentRecord / PaymentCallback
    llm_billing.go           LLMPlan / LLMWallet / LLMWalletTxn / LLMUsageRecord / LLMProviderBinding
  repository/
    module.go                ModuleRepository（CRUD + 依赖拓扑检测 + 闭包计算）
    tenant_registration.go   TenantRegistrationRepository
    order.go                 OrderRepository（含状态机 CAS）
    llm_billing.go           LLMWalletRepository（含行锁扣减）、LLMUsageRepository
  service/
    module_catalog_service.go    目录 CRUD / 发布 / 依赖环检测 / 下发快照
    module_grant_service.go      下发 / 撤销 / 闭包补齐 / 在用检测
    tenant_registration_service.go
    order_service.go             CreateOrder / MarkPaid / Cancel / Reject（唯一入账入口）
    payment_notify_service.go    回调入口（本期 501 占位 + 幂等表写入）
    llm_plan_service.go
    llm_wallet_service.go        Recharge / Consume / Refund / Adjust（行锁 + 流水）
    llm_usage_service.go         Record（request_id 幂等）
    llm_proxy_service.go         OpenAI 协议转发 + 计量
  controller/
    module_catalog_controller.go
    module_grant_controller.go
    tenant_registration_controller.go
    order_controller.go
    llm_billing_controller.go
    llm_proxy_controller.go
  router/
    router.go                   追加 /platform/modules* /platform/tenants /platform/orders
                                /platform/llm* /merchant-api/modules/sync /merchant-api/module-market/*
                                /llm-api/v1/chat/completions /platform/payments/notify/:channel

platform-web/src/
  api/{module.js, tenant.js, order.js, llmBilling.js}
  views/platform/module/{Catalog.vue, Editor.vue, Grant.vue}
  views/platform/tenant/{List.vue, Registration.vue}
  views/platform/order/{List.vue, Detail.vue}
  views/platform/llm/{Providers.vue, Plans.vue, Wallets.vue, Usage.vue}
  router/modules/platform.js   追加 4 个分组
```

### 3.2 用户端新增（hivemtk）

```
user-server/internal/
  model/
    tenant_module.go           TenantModule / ModuleSyncLog
    module_order.go            ModuleOrder（本地镜像）
    llm_wallet.go              LLMWallet / LLMWalletTxn
  repository/
    tenant_module.go
    module_order.go
    llm_wallet.go
  service/
    module_catalog_service.go  拉取平台目录 + 同步到本地镜像（对齐资产市场 client 形态）
    module_gate_service.go     IsEnabled / RequireEnabled / EnabledKeys
    module_grant_service.go    平台确认后拉取激活模块
    module_market_service.go   市场列表/详情/下单/订单查询
    llm_wallet_service.go      余额/预扣/扣减/充值镜像
    llm_platform_provider.go   平台 provider 拉取与路由接入
  controller/
    module_market_controller.go
    module_center_controller.go
    llm_wallet_controller.go
  middleware/
    module_gate.go             RequireModule(key) + 路由标注解析
  platform/
    module_client.go           平台模块/订单客户端（HMAC）
    llm_proxy_client.go        平台 LLM 代理客户端
  dto/
    module_market.go
    llm_billing.go

  model/menu.go                SystemMenuNode 旁新增 ModuleKey 字段（**仅标注，不参与过滤**）
  model/menu_capability_routes.go  追加模块闸覆盖的形状
  service/menu.go              FilterMenuTree 前先按模块闸过滤
  aiagent/llm/
    dispatcher.go              ProviderConfig 增 Source
    dispatcher_dispatch.go     平台 provider 走代理 + 扣减钩子
    dispatcher_observability.go splitCost 改为分别计价
    dispatcher_persistence.go  provider 行映射增 Source / Input / Output 单价

user-web/src/
  api/{moduleMarket.js, moduleCenter.js, llmBilling.js}
  views/moduleMarket/{Market.vue, Detail.vue, MyModules.vue, Orders.vue}
  views/register/{Register.vue, TenantProfile.vue}
  router/modules/moduleMarket.js
  stores/module.js             Pinia 模块态
  layout/                      未开通模块的锁定菜单灰显（不进菜单树）
```

### 3.3 迁移

| 端 | 方式 |
| --- | --- |
| 平台端 | `internal/utils/db/db.go` 的 `migrateModels` 追加 + 幂等 DDL 兜底（照抄 `api_secret` 的既有做法） |
| 用户端 | `internal/pkg/db` 的 `AllModels()` 追加；`llm_providers` 新增列用幂等 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS` |
| 菜单绑定种子 | `internal/service/module_catalog_seeds.go`，24 个模块 × 208 个 menu_key，**幂等 upsert**，并在测试里断言全覆盖且无重复 |

## 四、执行批次（可独立验证）

| 批次 | 内容 | 验证 |
| --- | --- | --- |
| **P0** | 菜单 key 全量提取 → 生成 `module_menu_bindings` 种子数据 | 单测：208 key 全覆盖、无重复、每个模块至少 1 个 key |
| **P1** | 平台端：模型 + 仓储 + 服务 + 控制器 + 路由（模块/租户/订单）+ 幂等 DDL + 测试 | `cd platform-server && go build ./... && go test ./internal/...` |
| **P2** | 平台端页面：模块/租户/订单 9 个页面 + 路由分组 | `cd platform-web && npm run build` |
| **P3** | 用户端：模块闸 + 本地镜像 + 菜单过滤 + 路由标注 + 自检测试 | `go test ./internal/middleware/... ./internal/service/...` |
| **P4** | 用户端：注册/开户改造（公司/负责人/监管）+ 平台登记上报 | `go test ./internal/service/ -run Tenant` |
| **P5** | 用户端：功能模块市场 4 个页面 + API | `cd user-web && npm run test` |
| **P6** | LLM：双轨计价 + 平台 provider + 钱包 + 代理 + 计量 | `go test ./internal/aiagent/llm/...` |
| **P7** | 收敛：208 key 归属自检、依赖环检测、离线降级、支付回调幂等 | 全量门禁 |

## 五、风险与缓解

| 风险 | 缓解 |
| --- | --- |
| 208 菜单 key 手工绑定易错 | 生成式种子 + 测试钉死（P0） |
| 模块闸插在链路中间，可能漏挂路由 | 测试断言：新增未标注路由时 `router` 包测试失败（白名单机制，未标注默认**不过闸**，并打印告警清单供人工确认） |
| 平台 LLM 代理引入出站调用，违反零出域 | 仅在用户显式开通且 provider.source='platform' 时可用；`PLATFORM_ENABLED=false` 时路由不注册 |
| `splitCost` 语义变更影响既有成本统计 | 旧列保留 + 读时回落；历史数据 `prompt_cost`/`completion_cost` 不重算（统计口径变更在文档登记） |
| 订单状态机被并发点击 | `UPDATE ... WHERE status='pending_payment'` CAS + `order_status_logs` 唯一约束 |

## 六、不做什么（再次明确）

- 不改 `AGPL` 许可、不改开源仓的代码可见性。
- 不引入第三方支付 SDK（无依赖）。
- 不做真实邮件/短信发送的支付通知（本期靠运营人工）。
- 不动 `user-server` 已有的 251 条 `menu_capability_routes` 语义，只在其后追加模块闸。