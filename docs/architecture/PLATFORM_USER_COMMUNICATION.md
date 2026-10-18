# 平台端 ↔ 用户端：通信 / 协调 / 数据架构

> **状态**：v1.0 · 2026-10-18
> **事实源**：`hivemtk-platform/platform-server/internal/router/router.go`、
> `hivemtk/user-server/internal/platform/*.go`、两侧 `internal/model/*.go`
> **性质**：现状描述 + 缺陷清单。不是设计提案 —— 每条结论都指到具体代码。

---

## 一、通信拓扑：5 条通道，3 种鉴权

```text
                    ┌───────────────── hivemtk-platform ─────────────────┐
                    │  platform-server (8205)                            │
                    │                                                    │
  ① HMAC 签名  ────►│  /merchant-api/*    MerchantAuth()                 │
                    │    /merchant/register     商户注册                 │
                    │    /modules/sync          模块下发 ★               │
                    │    /module-market         模块市场 ★               │
                    │    /orders                下单/查单 ★              │
                    │    /llm/{plans,providers,wallet,wallet/txns} ★     │
                    │    /tenant/{notice,register} ★                     │
                    │    /asset-market/*        资产市场                 │
                    │                                                    │
  ② 公开        ────►│  /api/platform/*    （无鉴权）                     │
                    │    /heartbeat             心跳（3 分钟）           │
                    │    /install               安装上报 ⚠ 用户端零调用   │
                    │                                                    │
  ③ Bearer(商户)────►│  /llm-api/v1/chat/completions   LLM 代理 ★         │
                    │    Authorization: Bearer <merchant_key>            │
                    │                                                    │
  ④ JWT+超管    ────►│  /platform/*   JWTAuth+RequirePlatformAdmin+Audit  │
                    │    模块目录/授权/订单/租户/LLM/监控（人操作）      │
                    └────────────────────────────────────────────────────┘
                                        ▲
                        ┌───────────────┴───────────────┐
                        │  user-server (8204)           │
                        │  internal/platform/*.go 是**唯一**出站客户端
                        └───────────────────────────────┘
                                        ▲
  ⑤ JWT(本地)   ────────────────────────┘
    浏览器 ──► user-server /api/* （与平台无关，本地鉴权）
    浏览器 ──► user-server /api/platform/message/* 【代理】──► 平台 /platform/message/*
```

★ = 本次功能模块化改造新增；⚠ = 见 §六·问题3

### 1.1 关键性质：**纯拉取（pull-only）**

平台端**除 LLM 代理外没有任何出站调用**（实测 `http.NewRequest` 只出现在
`llm_proxy_service.go` 与 `playground_service.go`），也**没有推送机制**
（`platform_messages` 是平台端本地表，用户端不轮询它）。

一切信息流都是"用户端定时来拉"或"人点了某个页面才发"：

| 触发方式 | 周期 | 内容 |
| --- | --- | --- |
| 定时器 | **3 分钟** | 心跳（`heartbeatIntervalGetter`） |
| 定时器 | **10 分钟** | 模块下发（`moduleSyncInterval`） |
| 启动时一次 | — | 模块下发（`StartModuleSync` 首轮**同步**执行）、provider 拉取 |
| 人点页面 | — | 站内信（经 `controller/platform.go` 代理，走 JWT 管理端） |

**直接后果**：平台管理员点了「确认收款」之后，用户端要等**最长 10 分钟**才看到模块开通。
这个延迟是纯拉取模型的固有代价，不是 bug —— 但它决定了"确认后立即生效"这类需求
必须先有推送（或把同步周期调短），不能靠改业务代码解决。

### 1.2 鉴权机制与坑

| 通道 | 机制 | 载荷 | 备注 |
| --- | --- | --- | --- |
| ① `/merchant-api/*` | HMAC-SHA256 | `method\npath\ntimestamp\nbody` | ±5 分钟时间戳窗口；写操作另有 5 分钟重放缓存 |
| ② `/api/platform/*` | 无 | — | 仅统计用；`install` 有 `merchant_key` 存在性校验，`heartbeat` 无 |
| ③ `/llm-api/v1/*` | Bearer = `merchant_key` | — | 不走 JWT（商户端与平台管理员令牌不是一回事） |
| ④ `/platform/*` | JWT + 回源查管理员 | — | 令牌吊销水位 `tokens_invalid_before` |

**HMAC 密钥解析链有个坑（实测踩过）**：

```text
签名密钥 := 每商户独立密钥（进程内注入）
          → 环境变量 MERCHANT_API_SECRET
          → PlatformCfg.Secret
```

而**每商户密钥在客户端构造时从 `config/.merchant_api_secret` 文件加载**，
它**优先于环境变量**。实测：环境变量配对了却持续 401，原因是那个文件里
留着 2026-09-04 的旧值。排查成本极高（两侧"看起来都配对了"）。

> 建议：该文件与 env 冲突时应**显式告警**而不是静默取前者。

---

## 二、身份模型：两套标识 + 一次修复

### 2.1 两个标识的语义

| 标识 | 生成 | 存储 | 用途 |
| --- | --- | --- | --- |
| **`merchant_key`** | `config/.merchant_key`（32 位随机） | 平台 `merchants.merchant_key` | **商业身份**：模块授权、订单、LLM 钱包、资产购买 |
| **`install_id`** | `install.lock`（`ins-<32hex>`） | 平台 `platform_installs.install_id` | **运维统计**：心跳、版本、在线 |

### 2.2 ⚠ 本轮修复：两处上报曾把两者混填

`SubmitRegistrationToPlatform` 与 `submitSelfRegisterToPlatform` 原先都写：

```go
installID := platform.GetMerchantKey()
payload := TenantRegistrationPayload{
    MerchantKey: installID,   // ✔ 正确
    InstallID:   installID,   // ✘ 错：这里应是 install.lock 的值
}
```

后果不是"多写一个字段"，而是**平台端答不出三个问题**："这个部署是谁 / 它在线吗 / 它登记了吗" ——
因为 `tenant_registrations.install_id` 存的是商户密钥，而 `platform_installs` 用的是真 install_id，
**两表之间没有可连接的键**。

已修：`currentInstallID()` 从 `install.lock` 读；读不到时返回**空串**而不是回落 merchant_key
（混填比空值更难排查：空值至少明确表示"没提供"）。由
`TestRegistrationPayloadKeepsTwoIdentitiesDistinct` 钉死。

### 2.3 遗留：`merchants.install_id` 列基本恒空

写这一列的**唯一路径**是 `/api/platform/install`，而 `ReportInstall` /
`ReportInstallDefault` 在用户端**生产代码零调用**（只在测试里出现）。
生产实际只调 `/merchant-api/merchant/register`（不写 install_id）。

实测 59 个商户：**53 个只有 merchant_key、6 个两者都有**（后者来自手工/种子）。

所以 `platform_installs` 与 `merchants` 是两个**并行且互不关联**的身份表。
修好 §2.2 之后，`tenant_registrations` 成了唯一能把两者连起来的表 ——
但它只在"开户登记"时写一次，且需要平台端主动 join。

---

## 三、数据归属矩阵

原则：**平台端是商业关系的唯一权威；用户端只存只读镜像用于离线可用。**

| 数据 | 权威在 | 用户端镜像 | 同步方式 | 冲突时 |
| --- | --- | --- | --- | --- |
| 模块目录（24 个模块 / 定价 / 依赖 / 菜单绑定） | **平台** | 否（用编译期常量 + 下发的已激活集合） | `GET /modules/sync` | 平台胜 |
| 租户已开通模块 | **平台** | `tenant_modules` | 同上 | 平台胜 |
| 订单 | **平台** | `module_orders`（只读缓存） | 随模块同步刷新 | 平台胜（本地**不跑状态机**） |
| LLM 钱包余额 | **平台**（代理转发时实时扣） | `llm_wallets`（展示镜像） | `GET /llm/wallet` | 平台胜（本地**不二次扣减**） |
| LLM 用量明细 | 平台 `llm_usage_records`（对账） | `platform_llm_usage_records`（商户自助） | dispatcher 回填 | 各记各的 |
| 商户登记 | 平台 `tenant_registrations`（监管） | `tenants`（本地留痕） | 开户时上报一次 | 本地可先落 |
| 安装/心跳 | 平台 `platform_installs` | — | 3 分钟心跳 | 平台胜 |
| 商户身份（key/secret） | **用户端生成**，平台登记 | `config/.merchant_key` | 首次注册 | 用户端胜 |

### 3.1 ⚠ 同名表在两个库结构完全不同

实测：

```text
platform_db.tenant_modules:
  id, merchant_key, module_key, status, source, order_id, options,
  activated_at, expires_at, created_by, created_at, updated_at

user_db.tenant_modules:
  tenant_id, id, module_key, tenant_slot, status, source, category,
  price_cents, menu_keys, activated_at, expires_at, created_at, updated_at
```

`llm_wallets` / `llm_wallet_txns` / `llm_usage_records` 同样如此。
**同名、不同库、不同结构**。这在排查时是个陷阱：看到 SQL 里的 `tenant_modules`
必须先确认连的是哪个库。建议用户端镜像表加 `mirror_` 前缀，或至少在两处模型
注释里互相点名（目前只在用户端注释里说明了差异）。

---

## 四、功能模块数据流（完整生命周期）

```text
① 平台运营初始化目录
   user-server 生成 → hivemtk-platform/.../config/module_catalog_seed.json
     （24 模块 + 212 菜单节点 + 依赖边；由 cmd/gen-module-bindings 导出）
   → 平台端 Seed()：//go:embed 内嵌兜底，磁盘文件为可选覆盖
   → module_catalog / module_dependencies / module_menu_bindings 落库

② 用户端启动
   StartModuleSync()（在 router.Setup **之前**，首轮同步执行）
     → GET /merchant-api/modules/sync（HMAC）
     → 落 tenant_modules（本地镜像）+ module_sync_logs
     → ModuleGateService.Refresh() → 装进 module 包的全局判定函数
     → router.Setup() 挂 GuardModulePath（鉴权后、权限前）

③ 用户下单
   user-web → POST /api/module-market/order（本地 JWT）
     → user-server 生成 client_order_no（幂等键）
     → POST /merchant-api/orders（HMAC）
     → 平台 CreateOrder：状态 pending_payment，返回 payment_guide
   → 本地 module_orders 落一条只读缓存
   → 前端提示"请联系平台运营付款"

④ 平台确认收款（人）
   POST /platform/orders/:no/confirm-paid（JWT+超管）
     → MarkPaid(order_no, PaymentNotice{source: offline_manual})   ← 唯一入账入口
       ├─ CAS pending_payment → paid
       ├─ payment_records 一条
       ├─ fulfillModules：Grant(闭包) → tenant_modules（平台权威表）
       └─ CAS paid → fulfilled
   ★ 用户端**不会立刻知道**，等下一次模块同步（≤10 分钟）

⑤ 用户端感知
   10 分钟后的模块同步 → tenant_modules 本地镜像更新 → 菜单闸放行
```

**闸的三态**（`middleware/module_gate.go`）：

| 态 | 判据 | 行为 |
| --- | --- | --- |
| ① 本地无模块数据 | 首启 / 从未同步成功 | **全放行** + WARN 一次 |
| ② 有数据且命中模块 | `tenant_modules` 有该 key 且 active 且未过期 | 放行 |
| ③ 有数据但未命中 | 新增接口忘了标注 | **放行** + 记入 `/api/modules/uncovered-routes` 巡检 |

态①是刻意的：判"拦"会让首启用户看到空系统且无从自救。**这也是为什么
"同步任务没被调度"会成为致命缺陷** —— 表恒空 = 恒处态① = 付费模块全放行。

### 4.1 路由归属用「路径模式表」而不是逐条标注

`middleware/module_route_patterns.go`（**生成件**，224 条）：由
`model/menu_capability_routes.go`（人工审过的 251 条「路径 → 菜单key」）
接上 `model/module_bindings.go`（菜单key → 模块）两跳推导。
`module_route_supplement.go`（8 条手写）补生成件覆盖不到的纯控制面。

口径：**只有归属唯一时才登记**；一条路径被多个模块的菜单共用时**不登记**
（走态③巡检），避免给收费模块开后门。

---

## 五、商户信息数据流

```text
① 开户（三条入口，同一套必填口径）
   a) SaaS 自助开户   POST /api/tenant/register
   b) 单机自助注册    POST /api/auth/register
   c) 离线初始化向导  POST /api/system/init-admin     ← 离线部署唯一入口
   四项必填：公司名称 / 负责人 / 电话 / 邮箱（需求点名的监管最小集）
   + 协议版本留痕 + 监管确认位（**事后不可改**）

② 本地落库
   租户行：tenants{company_name, legal_person, phone, email, license_no,
                  address, reg_mode, platform_sync, agreement_version, regulatory_ack}
   - SaaS   → platform_sync = pending → 上报成功 synced / 失败 failed
   - 离线   → platform_sync = skipped（**不发任何出站请求**）

③ 上报平台
   POST /merchant-api/tenant/register（HMAC）
   → tenant_registrations（平台端监管留痕，含 merchant_key + install_id）

④ 负责人初始化
   超管本人 real_name / phone 回填（登录后自己的资料页就能看到，不必另找入口）
```

`platform_sync` 三态语义：`skipped` **不是错误** —— 离线部署本来就不上报，
标成 `failed` 会在平台端堆一批无法处理的红单。

---

## 六、发现的架构问题（按严重度）

| # | 问题 | 影响 | 状态 |
| --- | --- | --- | --- |
| 1 | 两处上报把 `install_id` 填成 `merchant_key` | 平台端无法关联登记与心跳 | **已修** + 回归测试 |
| 2 | 纯拉取模型，模块开通最长延迟 10 分钟 | 「确认后立即生效」类需求无法满足 | 待决策（见下） |
| 3 | `/api/platform/install` 用户端零调用；`merchants.install_id` 恒空 | 两套身份表互不关联 | 待决策 |
| 4 | `tenant_modules` 等 4 张表两端**同名不同构** | 排查时选错库 | 待决策（建议改名） |
| 5 | HMAC 密钥文件静默优先于环境变量 | 实测持续 401，两侧"看起来都配对了" | 待决策（建议冲突告警） |
| 6 | 用户端 `tenant_module` 的 `MenuKeys` 列存了但几乎不用 | 冗余：闸用的是编译期 `ModuleMenuBindings` | 已知，无害 |

### 关于问题 2/3 的判断

这两条**不建议现在改**，理由是它们涉及产品决策而非缺陷：

- **问题 2**：缩短同步周期（如 1 分钟）或加推送都能解决，但代价不同
  （前者线性增加 HMAC 往返，后者要建长连接）。当前"个人初期运行"量级下
  10 分钟可接受，且**前端已经把付款指引写清楚了**（"确认收款后权益自动开通"），
  用户不会以为没成功。
- **问题 3**：`/api/platform/install` 保留有价值（它是"按 install_id upsert 商户"
  的语义出口，未来若要按安装实例而非商户管理时用得上）。但 `merchants.install_id`
  恒空这件事应该**在列注释里写明**，否则下一个人会以为它是有效的关联键。

---

## 七、一句话总结

**平台端是商业关系的唯一权威，用户端只存只读镜像并定时拉取。**
通信是**纯拉取**的（平台从不主动联系用户端，除响应式 LLM 代理），
身份有**两套标识**（商业身份 `merchant_key` / 运维身份 `install_id`），
前者决定一切授权与计费，后者只用于统计 —— 二者**不可混用**，混用会让平台端
失去把"登记 / 在线 / 授权"三件事关联起来的能力。