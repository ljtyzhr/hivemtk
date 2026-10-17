# Feature Specification: 功能模块化 · 按需订阅收费改造

**Feature Branch**: `002-modular-subscription`
**Status**: 已论证（v1.0）
**Baseline**: `master` @ 2026-10-17
**关联交付物**: `docs/architecture/MODULAR_MODULES.md`（任务一：模块清单论证）

---

## 一、背景与问题陈述

当前系统过于宏大：菜单注册表 **208 个节点**、数据表 **264 张**、前端视图目录 **80+ 个**、`user-web` API 模块 **110 个**。将来还会持续增加新功能。在这种体量下：

1. **用户侧**：新装用户面对 7 个顶级分组、208 个菜单，不知道该点哪里；大量功能用不上却已经写进编译产物和数据库。
2. **商业侧**：项目从纯开源需要演进为「基座开源 + 基础功能开源 + 特需插件/模块收费 + 周边（资产包 / LLM）收费」，但当前**没有任何计费、订单、授权的概念**（`user-server/docs/dev/FEATURES.md §二十四` 明确写了"❌ 定价方案 / ❌ 注册开户"）。
3. **合规侧**：`platform-server` 已有 `merchants` 表，但只按 `install_id` 统计，**不收集公司名称、负责人、电话、邮箱**，无法满足平台端的监管责任要求；用户端 `TENANT_MODE=off` 时连租户都不存在。
4. **AI 侧**：LLM 只有**用户自带 Key**一种路径（`llm_providers` 表），平台没有任何垫资能力，也没有按量计费与扣减。

## 二、目标（Goals）

| # | 目标 | 验收口径 |
| --- | --- | --- |
| G1 | 产出一份**完整且自洽**的功能模块清单（模块 × 页面 × 数据表 × 耦合 × 菜单组织 × 基础模块集） | `docs/architecture/MODULAR_MODULES.md`；208 个菜单节点有且只有一个 module 归属（测试钉死） |
| G2 | 平台端完成**功能模块管理 / 租户管理 / 模块状态控制 / 下发 / 定价**，模块分「基础模块」与「收费模块」（收费模块允许 0 元） | 平台端 6 组管理页面 + 对应 API 全部跑通 |
| G3 | 平台端具备**订单 / 功能模块 / 租户 / 资产包 / 监控**五件套 | 订单状态机可从「已提交」走到「已开通」，支付凭证与回调表预留 |
| G4 | 用户端**注册即开户**打通平台端登记，收集电话/邮箱/公司名称/负责人等监管必需信息，**离线独立部署**同样收集 | SaaS 与离线两条路径都有登记记录 |
| G5 | 用户端有**功能模块市场**（对标资产包市场），按需下单 → 提示联系本人 → 平台端确认 → 落盘开通 | 完整链路可演示 |
| G6 | 平台支持**用户自带 LLM**（只统计）与**平台提供 LLM**（预付费计量，输入/输出分别计价并实时扣减） | 余额不足时阻断调用；用量明细可查 |

## 三、非目标（Non-Goals）

| # | 非目标 | 理由 |
| --- | --- | --- |
| N1 | 本阶段**不接入任何真实支付通道** | 个人初期运行，无支付资质/收款账户。**但契约必须为线上支付预留** |
| N2 | 不做「运行时动态卸载 Go 模块」（二进制热插拔） | Go 静态编译做不到；"热插拔"的真实形态是**能力闸**（路由/菜单/工具/任务按模块放行） |
| N3 | 不做套餐包 | 会把 17 个收费模块压成 1 档，运营失去颗粒度，依赖闭包规则复杂化 |
| N4 | 不做按 feature point 计费 | 粒度过细；模块级是当前代码结构能支撑的最细粒度 |
| N5 | 不动 `user-server` 的五层架构约束、不引入 `handler` 同义词命名 | 宪法约束 |
| N6 | 不删除任何表 / 不做数据迁移 | 关模块只关"可用性"，表结构属于基座；避免升级降级丢数据 |

## 四、术语表

| 术语 | 定义 |
| --- | --- |
| **功能模块（Module）** | 可独立订阅、独立关闭、独立定价的一组「菜单子树 + 数据表簇 + 业务闭环」 |
| **基础模块（Base）** | 免费且常开的模块。关掉它用户无法完成最小任务 |
| **收费模块（Paid）** | 需开通的模块，`price_cents >= 0`。`=0` 表示需开通但不收费 |
| **依赖闭包** | 开通某模块时必须一并开通的模块集合 |
| **模块闸（Module Gate）** | 鉴权之后、权限判定之前执行的"本模块是否已开通"检查，与角色无关 |
| **租户（Tenant）** | 平台端的经营主体；用户端 `TENANT_MODE=on` 时也落本地 `tenants` 表 |
| **商户（Merchant）** | 平台端既有概念，按 `install_id` 标识的一个部署实例 |
| **订单（Order）** | 模块订阅 / LLM 套餐 / 资产包三类购买的统一单据 |
| **LLM 钱包（Wallet）** | 租户的 LLM 预付余额，输入/输出 token 分列记账 |

## 五、用户故事与验收场景

### US1 · 平台运营（任务二）

> 作为平台运营，我要在平台端登记一个功能模块（含定价、依赖、菜单绑定），并把它下发给指定租户。

**验收场景**：
1. **创建模块**：填写 key/名称/分类/价格/依赖/子选项 → 成功后模块出现在列表，`status=draft`。
2. **依赖环拒绝**：给 A 加依赖 B，同时给 B 加依赖 A → 返回 400 `module_dependency_cycle`，不落库。
3. **上架**：`draft → published` 后该模块才出现在用户端市场。
4. **下发**：给租户 T 下发模块 X（X 依赖 Y）→ T 的 `tenant_modules` 同时出现 X 与 Y，Y 标记 `source=dependency`。
5. **关闭被依赖模块**：试图关闭 Y → 400 `module_in_use`，返回依赖方列表。

### US2 · 平台运营 · 订单确认（任务二/四/五）

> 作为平台运营，我要在平台端看到待确认订单，在收到线下付款后点击"确认收款"。

**验收场景**：
1. 用户下单 → 订单 `status=pending_payment`，`payment_method=offline_manual`。
2. 平台端订单列表出现该单，红点提示。
3. 平台端点"确认收款" → 订单 `→ paid`，模块订阅置 `active`，日志落 `order_status_logs`。
4. **幂等**：同一订单点两次确认 → 第二次返回 409 `order_already_paid`，不重复开通。

### US3 · 新用户注册（任务三）

> 作为新用户，我要在 user-web 注册并完成开户，同时把公司信息登记到平台。

**验收场景**：
1. **SaaS 路径**：`TENANT_MODE=on` 时，注册页要求 公司名称 / 负责人姓名 / 手机 / 邮箱 / 密码 / 协议勾选（含监管责任告知）。
2. 注册成功后：本地建 `tenants` + 超管 `system_users`，并 `POST /merchant-api/tenant/register` 上报平台；平台端 `tenant_registrations` 出现记录，`merchants` 出现对应商户。
3. **离线路径**：`TENANT_MODE=off` 时，不请求平台，但**仍收集同样的公司/负责人信息**并落本地 `tenants` 行（`platform_sync=skipped`）。
4. **平台不可达**：注册成功但上报失败 → 标记 `pending_sync`，登录页提示"平台登记待同步"，**不回滚注册**。
5. **必填校验**：缺公司名称或负责人 → 400。

### US4 · 功能模块市场（任务四）

> 作为用户，我要在"功能模块市场"浏览可订阅模块，一键下单。

**验收场景**：
1. 市场页按分组（智能体/触达/IM/分析/其他）展示卡片，含名称、图标、一句话价值、依赖提示、定价。
2. 已开通模块显示 `已开通` 徽标且按钮禁用。
3. `price=0` 的模块按钮文案为 `开通（免费）`；`price>0` 为 `订阅 · ¥2980/年`。
4. 点订阅 → 二次确认弹窗列出**依赖闭包**与总价 → 提交 → 订单创建。
5. 成功页**明确提示**："订单已提交，请联系平台运营（微信/邮件）完成付款；付款确认后模块自动开通。"
6. 我的订单页可见订单状态流转。

### US5 · 自带 LLM vs 平台 LLM（任务五）

> 作为用户，我可以用自己的 API Key（免费，只统计），也可以买平台 LLM 套餐（预付费，按量扣减）。

**验收场景**：
1. 自带 Key：`llm_providers.source='self'` → 调用照常，`llm_routing_logs` 记录用量成本，**不扣任何钱包**。
2. 平台 LLM：平台端配置 `llm_plans`（token 数 + 输入单价 + 输出单价）→ 用户下单 → 确认 → 钱包余额入账。
3. 平台端把 provider 下发给用户端（`source='platform'`）→ 用户端路由优先选它。
4. **实时扣减**：每次 dispatch 后按 `prompt_tokens × input_price` + `completion_tokens × output_price` 扣减本地钱包，余额落 `llm_wallet_txns`。
5. **余额不足**：预扣判定失败 → 该次调用不发起（或降级到自带 Key），返回明确错误码 `llm_balance_insufficient`。
6. **口径纠正**：输入/输出**分别计价**，不再按 token 比例拆分总价（现状 `splitCost` 是假拆分，必须改）。

## 六、功能需求

### FR-1 平台端 · 模块目录与下发

| 编号 | 需求 |
| --- | --- |
| FR-1.1 | `module_catalog`：`module_key`(uk) / `name` / `category`(base\|paid) / `price_cents` / `billing_cycle` / `status`(draft\|published\|offline) / `icon` / `summary` / `description` / `sort_order` / `options`(jsonb) |
| FR-1.2 | `module_dependencies`：`module_key` + `depends_on_key`，写入时拓扑检测环 |
| FR-1.3 | `module_menu_bindings`：`module_key` + `menu_key`(uk) |
| FR-1.4 | `module_options`：`module_key` + `option_key` + `option_name` + `price_delta_cents`（为将来细拆预留） |
| FR-1.5 | `tenant_modules`：`merchant_key` + `module_key` + `status`(active\|expired\|disabled) + `source`(purchase\|dependency\|manual\|default_base) + `order_id` + `activated_at` + `expires_at` |
| FR-1.6 | 平台端下发接口 `POST /merchant-api/modules/sync` 返回该商户全部已激活模块（含依赖闭包结果、菜单绑定、定价） |
| FR-1.7 | 基础模块**不可删除、不可下线、不可改价**（`category=base` 的写操作一律 400） |

### FR-2 平台端 · 租户与开户登记

| 编号 | 需求 |
| --- | --- |
| FR-2.1 | `tenant_registrations`：`company_name`(必填) / `legal_person`(负责人，必填) / `phone`(必填) / `email`(必填) / `license_no` / `address` / `merchant_key` / `install_id` / `reg_mode`(saas\|offline) / `platform_sync`(pending\|synced\|failed\|skipped) / `agreement_version` / `regulatory_ack`(bool) |
| FR-2.2 | 用户端注册接口必须校验上述必填项；缺一 → 400 |
| FR-2.3 | 离线部署（`PLATFORM_ENABLED=false`）时，登记只落本地 `tenants` + 本地 `tenant_registrations` 镜像，`platform_sync=skipped` |
| FR-2.4 | 平台端 `GET/PUT /platform/tenants` 可查可改，可手工触发补同步 |
| FR-2.5 | 监管责任：注册页与平台端均展示告知文本，`regulatory_ack=true` 且记录 `agreement_version` |

### FR-3 平台端 · 订单与支付（预留线上）

| 编号 | 需求 |
| --- | --- |
| FR-3.1 | `orders`：`order_no`(uk) / `merchant_key` / `order_type`(module\|llm\|asset) / `status` / `total_amount_cents` / `currency` / `payment_method`(offline_manual\|wechat\|alipay\|stripe…) / `channel`(web\|api) / `contact` / `remark` / `pay_serial_no` / `paid_at` / `confirmed_by` / `confirmed_at` / `cancel_reason` |
| FR-3.2 | 订单状态机：`pending_payment → paid → fulfilled`，分支 `cancelled` / `rejected`。非法迁移返回 409 |
| FR-3.3 | **支付回调复用**：`OrderService.MarkPaid(orderNo, PaymentNotice)` 是唯一入账入口；线下确认与未来的线上回调**调用同一个函数**，只有 `source` 不同（`offline_manual` vs `channel_notify`）。回调幂等键：`payment_callbacks.callback_id` 唯一索引 |
| FR-3.4 | `payment_records`：凭证（方式/流水/金额/凭证图片/操作人） |
| FR-3.5 | `payment_callbacks`：异步回调原始报文（`channel` / `callback_id` / `payload` / `raw_headers` / `verified` / `processed_at`），本期只建表 + 提供 `POST /platform/payments/notify/{channel}` 的**关闭态占位**（返回 501，不实现验签） |
| FR-3.6 | `order_items`：订单行（`item_type` / `item_key` / `item_name` / `unit_price_cents` / `quantity` / `amount_cents` / `snapshot` jsonb） |

### FR-4 平台端 · LLM 套餐与计量

| 编号 | 需求 |
| --- | --- |
| FR-4.1 | `llm_plans`：`plan_code`(uk) / `name` / `input_tokens` / `output_tokens` / `input_price_per_1k_cents` / `output_price_per_1k_cents` / `price_cents` / `valid_days` / `status` |
| FR-4.2 | `llm_wallets`：`merchant_key`(pk) / `input_tokens_remaining` / `output_tokens_remaining` / `total_recharged` / `total_consumed_input` / `total_consumed_output` / `low_balance_threshold` |
| FR-4.3 | `llm_wallet_txns`：`id` / `merchant_key` / `order_id` / `delta_input` / `delta_output` / `balance_input_after` / `balance_output_after` / `reason`(recharge\|consume\|refund\|adjust) / `ref_type` / `ref_id` / `operator` |
| FR-4.4 | `llm_usage_records`：`request_id`(uk，幂等) / `merchant_key` / `provider` / `model` / `scenario` / `prompt_tokens` / `completion_tokens` / `input_cost_cents` / `output_cost_cents` / `total_cost_cents` / `trace_id` / `occurred_at` |
| FR-4.5 | 平台端管理页面：LLM Provider 配置（含 `platform` 标记与下发生效）、套餐管理、钱包管理（充值/查账/流水）、用量看板 |
| FR-4.6 | Provider 下发：平台端 provider 标记 `deliver=true` 时，随模块同步接口一起下发给用户端（**API Key 不下发**，用户端调用走平台代理，见 FR-4.7） |
| FR-4.7 | 平台提供 LLM 的调用链：user-server → `POST /llm-api/v1/chat/completions`（平台代理，Bearer=merchant key）→ 平台转发到真实厂商 → 平台侧计量扣费并返回 `X-Llm-Usage` 头。**零出域例外**：用户显式开通平台 LLM 且注册协议已告知 |

### FR-5 用户端 · 模块闸

| 编号 | 需求 |
| --- | --- |
| FR-5.1 | `tenant_modules`（本地镜像）+ `module_sync_logs`；启动时与心跳同频拉取（默认 10 分钟），失败降级用本地镜像 |
| FR-5.2 | `GET /api/modules/my`：返回已开通模块、依赖闭包、定价、菜单绑定 |
| FR-5.3 | **模块闸中间件** `middleware.RequireModule(key)`：路由注册时按模块分组标注，鉴权后执行；未开通 → HTTP 403 + 业务码 `module_not_enabled` |
| FR-5.4 | 闸的执行顺序：**登录态 → 模块闸 → 角色权限**。超管也过闸 |
| FR-5.5 | 菜单树过滤：`FilterMenuTree(SystemMenuTree, 角色授权 ∩ 已开通模块覆盖)` |
| FR-5.6 | 定时任务、Agent 工具、WebSocket/EventBus 订阅按 `IsEnabled(moduleKey)` 过滤 |
| FR-5.7 | 离线部署（`PLATFORM_ENABLED=false`）：基础模块全开；`price=0` 付费模块自动开；`price>0` 付费模块保持关闭 |
| FR-5.8 | **自检**：`module_menu_bindings` 的 208 个 menu_key 必须有且只有一个 module 归属（测试断言） |

### FR-6 用户端 · 功能模块市场

| 编号 | 需求 |
| --- | --- |
| FR-6.1 | `GET /api/module-market/list`：分组 + 卡片（名称/图标/摘要/依赖提示/定价/已购标记） |
| FR-6.2 | `GET /api/module-market/detail/:key`：详情 + 包含页面清单 + 依赖闭包 + 定价 |
| FR-6.3 | `POST /api/module-market/order`：创建订单（本地镜像 + 调平台），返回 `order_no` + 付款指引文案 |
| FR-6.4 | `GET /api/module-market/orders`、`GET /api/module-market/orders/:order_no` |
| FR-6.5 | 前端：`Market.vue`（对标 `assetMarket/Market.vue`）、`Detail.vue`、`MyModules.vue`、`Orders.vue`；菜单挂在 `system.moduleCenter` |
| FR-6.6 | 未连平台时市场页显示"平台未连接"空态，不报错 |

### FR-7 用户端 · LLM 双轨

| 编号 | 需求 |
| --- | --- |
| FR-7.1 | `llm_providers` 新增 `source`(self\|platform) 与 `platform_plan_code`；`CostPer1k` 拆为 `CostPer1kInput` / `CostPer1kOutput`（迁移保留旧列，读时回落） |
| FR-7.2 | **计费口径纠正**：`splitCost` 由"按 token 比例拆总价"改为"分别乘单价"，见 `dispatcher_observability.go:229` |
| FR-7.3 | 平台 LLM 的调用在 `dispatcher_dispatch.go` 收尾处扣减本地钱包镜像 + 上报平台计量记录（`request_id` 幂等） |
| FR-7.4 | 余额不足 → 预检阻断，返回 `llm_balance_insufficient`；若配置了降级 provider 则降级 |
| FR-7.5 | 自带 Key 路径**零改动**：不扣钱包、不上报计量（除已有的 `llm_routing_logs` 统计） |

## 七、数据模型变更

见 `docs/architecture/MODULAR_MODULES.md §六`。用户端沿用 `internal/pkg/db.AllModels()` 自动迁移机制；平台端在 `internal/utils/db/db.go` 的 `migrateModels` + 幂等 DDL 兜底。

## 八、非功能约束

| 约束 | 口径 |
| --- | --- |
| 架构 | 严格五层 `Router→Controller→Service→Repository→Model`；Router 只做映射，禁内联闭包 |
| API | `/api/{domain}/{resource}`（用户端）、`/api/manage/{resource}`（管理端）、平台端沿用 `/platform/*` + `/merchant-api/*` |
| 响应 | user-server：`{"code":0,"data":...,"message":"ok"}`；platform-server：`{code,msg,data}`（历史口径，勿改） |
| 零出域 | 默认路径**不得**产生外部网络调用；平台 LLM 代理是唯一显式例外（用户开通 + 协议告知） |
| 离线 | `PLATFORM_ENABLED=false` 时不得有任何出站请求；所有平台交互面降级为本地镜像/明确空态 |
| 金额 | 一律用整数分（`*_cents`），禁止 float 存金额 |
| 测试 | Go：`go build ./...` + `go test ./internal/<包>/...`（需 `POSTGRES_TEST_*`）；前端：`npm run test`；提交前 `make audit` `make lint` `make vet` `make test-go` |

## 九、开放问题与决策

| 问题 | 决策 | 理由 |
| --- | --- | --- |
| 模块解锁是"下载代码"还是"授权位"？ | **授权位**。代码全在开源仓，收费对象是订阅与配额 | 离线用户仍拥有全部代码，符合 AGPL 精神与"基座开源"定位 |
| 用户自带 Key 收费吗？ | 不收 | 收费对象是平台垫付的算力 |
| 平台关闭时付费模块怎么办？ | `price=0` 自动开；`price>0` 锁住并明示 | 否则定价失效 |
| 依赖自动补齐会不会"买一送一堆"？ | 会，但只补**基础 + 免费**依赖；付费依赖必须用户显式勾选并在确认弹窗列出 | 避免意外收费 |
| 一期订单会不会永远卡在待确认？ | 会，靠运营。UI 上超期高亮 + 平台端红点 | 无支付通道时唯一可控做法 |