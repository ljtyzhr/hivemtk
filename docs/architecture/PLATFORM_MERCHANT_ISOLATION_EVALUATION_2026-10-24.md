# 平台端"商户数据隔离"要不要做？——A3 的调研与论证（2026-10-24）

> 背景：SaaS 全链路审计把 A3 登记为「平台端商户隔离全靠手写 `Where("merchant_key = ?")`」，
> 严重度中高，判据写成「运营端 merchant-api 组有框架级注入 + 漏写会红的门禁」。
> 动手前必须先回答一件事：**平台端业务就是"平台管理所有商户"，它到底需不需要商户数据隔离？**
> 本文用活体代码 + 真库内省论证这个问题，并据此改写 A3 的判据与落地形态。

---

## 1. 先把"隔离"这个词拆成两种不同的东西

| | 用户端（user-server） | 平台端（platform-server） |
|---|---|---|
| 主体是谁 | 外部客户（租户），互不可见 | **运营人员（内部，可信）** + **商户（外部，不可信）** |
| 隔离轴 | `tenant_id`：客户之间的数据不能串 | 不是"平台 vs 商户"的租户隔离，而是"**商户 A vs 商户 B**" |
| 谁能合法看全部 | 只有系统级操作（迁移、后台扇出） | **运营端本来就该看全部**（这是它的业务） |
| 若照抄用户端 fail-closed | 对（租户表无 ctx 即拒） | **错**：运营端每个列表/报表/调账都会被拒 |

**结论一：把平台端当成用户端那样的多租户系统去全量 fail-closed 是错的。**
它会同时造成两件坏事：①把运营端合法跨商户查询（订单列表、钱包流水、用量报表、调账、
模块开通…）全部打成 500；②为了让它跑起来，得在 237 处手写条件上再逐一标注"我是系统级"，
而这份标注本身又会腐烂成第二套真相。收益为零、回归面最大。

**但"不需要租户级隔离"不等于"不需要商户级隔离"。** 商户是**外部主体**，它的身份只由
HMAC 签名证明；它读到的每一行都必须是自己的。这条线的两端天然不对称：

- 运营面：JWT + RBAC 管越权（已有 `middleware.ContributorJWTAuth` / admin 会话），
  商户维度对它只是**筛选条件**，不是安全边界。
- 商户面（`/merchant-api/*`，22 条路由）：商户维度是**安全边界**，身份只能来自签名。

**结论二：A3 的真实范围 = 商户面（`/merchant-api` 组）。运营面登记为"不受商户隔离约束"，
理由（威胁模型不同）写进台账，而不是含糊地"待做"。**

---

## 2. 实证：商户面今天有没有可利用的越权？

### 2.1 装配面（端点清单）

`internal/router/router.go:61` 起，`/merchant-api` 组 **22 条路由全部** `Use(middleware.MerchantAuth())`：

```
POST /merchant/register        POST /logs/report         GET  /tenant/notice
GET  /modules/sync             GET  /module-market       GET  /module-market/payment-guide
GET  /notices/list             POST /notices/report      POST /tenant/register
POST /orders                   GET  /orders              GET  /orders/:order_no
POST /orders/:order_no/pay     GET  /llm/plans           GET  /llm/providers
GET  /llm/wallet               GET  /llm/wallet/txns     + asset-market 9 条
```

`MerchantAuth` 的判据是 HMAC（`method\npath\ntimestamp\nbody`）+ ±5min 时间戳 + 写操作重放去重
+ "商户必须存在且 active"，身份落 `c.Set("merchant_key", …)`。**装配面这一格是好的。**

### 2.2 身份来源面（handler 有没有从 body/query 取商户身份）

逐个核对 22 条路由的 handler（`grep 'Query("merchant_key")' | 'GetString("merchant_key")'`）：

- 商户面 handler 全部走 `c.GetString("merchant_key")`（asset-market 走 `merchantKeyOf(c)`）。
- 出现 `ctx.Query("merchant_key")` 的只有**运营面** handler：`LLMBillingController.WalletTxns` /
  `Usage`（管理端按商户筛）、`OrderController.List` 的回落分支、`Adjust`（路径参数）。
- **今天没有"商户面从 query/body 取身份"的现成洞。**

### 2.3 取数面（repository 有没有按商户分域）

多数方法显式带条件，例如 `OrderRepository.ListOrdersByMerchant` 的行首就是
`Where("merchant_key = ?", merchantKey)`；`GetByClientOrderNo` 把 `merchant_key` 与业务唯一键
一起写进 WHERE。**抽查未见"漏条件"的实现。**

但是——散落的判定里已经能看见三种**结构性**缺口，它们今天不流血，但任何一次改动都会流血，
而且**没有任何用例会红**：

#### G1 空串 fail-open（"空 = 全部"的语义写在 repository 里）

```go
// internal/service/order_service.go:883
// List 列表（管理端 / 商户端共用，merchantKey 空表示全部）
func (s *OrderService) List(ctx, merchantKey, ...) {
    if merchantKey != "" { return s.orderRepo.ListOrdersByMerchant(...) }
    return s.orderRepo.ListOrders(...)          // ← 空串 ⇒ 全平台订单
}
// internal/repository/llm_billing.go:346 —— 同形
if merchantKey != "" { q = q.Where("merchant_key = ?", merchantKey) }
```

同一条方法被 `/merchant-api/orders`（商户面）与 `/platform/orders`（运营面）**共注册**。
安全与否完全依赖上游"控制器先取 ctx 再回落 query"的**书写顺序**：

```go
// internal/controller/order_controller.go:54
merchantKey := ctx.GetString("merchant_key")
if merchantKey == "" { merchantKey = ctx.Query("merchant_key") }  // 管理端可按商户筛
```

把这四行调换顺序、或给这条 handler 再挂一个不验签的组、或中间件改名 —— 商户立刻能看全平台订单，
**且没有任何断言会红**。这就是"判定存在，但没有任何一条生产路径能保证它"的翻版。

#### G2 越权判定散落在 controller 的 if 里

`OrderController.Detail` / `MerchantCreatePay` 都是"先按订单号取行，再在 controller 里比对
`d.Order.MerchantKey != mk` ⇒ 403"。repo 层的 `GetByOrderNo` 是**不区分商户**的。忘写一次比对就是越权。

#### G3 归属判定只有"取数后比对"，没有"取不到即 404"的统一口径

商户 A 请求 B 的订单号时，走的是"能取到行 → 再比对 → 403"。403 与 404 的语义差异让
订单号空间成为可枚举面（A16 用户端那条闸给出的是相反的取向：不存在与外租户一律 404）。

---

## 3. 结论：A3 的判据要改写（不做的部分也要有据）

原判据「运营端 merchant-api 组有框架级注入 + 漏写会红的门禁」把范围写成"运营端"，方向偏了。
改写为：

> **A3（改写版）**：`/merchant-api` 组（外部商户面）的商户分域必须由**身份来源**唯一决定，
> 且"漏写/写反"必须是一条会红的用例：
> ①注入：该组的请求 ctx 带商户 scope 时，商户表查询由框架统一注入 `merchant_key = ?`，
> 不依赖每个 repository 方法自觉带参（G1 的结构性修法）；
> ②拒空：该组的 scope 为空即拒（不能退化成"看全部"）；
> ③门禁：该组每条路由必挂 `MerchantAuth`；该组 handler 不得从 body/query 取商户身份；
> ④行为：双商户交叉扫描，A 的签名读 B 的订单/钱包/购买记录恒为 404/403，A 用 B 的 key 下单恒为拒。
>
> **明确不做**：运营面不加 fail-closed、不要求逐处 `SystemScope` 标注 —— 运营端跨商户是业务本身，
> 其越权由 admin 会话 + RBAC 管；这一条登记为"有据不做"，理由即本文 §1。

---

## 4. 为什么"注入只在有 scope 时发生"就够（而不是再做一套 tenancyidx）

用户端的 `pkg/tenancyidx` 是 **fail-closed**：租户表无 ctx 即拒。平台端不能照抄，因为
运营面的大量查询**本来就没有商户 scope 且合法**。所以平台端的注入器必须是：

```
ctx 有商户 scope → 商户表查询强制加 merchant_key = ?（无视 repository 有没有自己带）
ctx 无商户 scope → 一律不动（运营面行为逐字节不变，零回归）
scope 存在但为空 → 拒（fail-closed，只对商户面生效）
```

这三条合起来才同时满足"商户面不会漏"与"运营面零回归"。它与用户端是**同一执法模型的两个档位**
（同一个念头：分域判据不许靠每个调用点自觉），不是两套逻辑 —— 这一点与
"云端运营版 + 本地独立部署版一套代码一套逻辑"是同一个原则：**同一个决策点，收成一个执行者**。

---

## 5. 落地清单（本轮实施）

| 项 | 位置 | 判据 |
|---|---|---|
| 商户表登记表 | `internal/utils/merchantscope/registry.go` | 真库内省（`information_schema` 里带 `merchant_key` 列的基表）与代码内冻结清单**双向相等**，多一张/少一张即红 |
| 注入器（GORM plugin） | `internal/utils/merchantscope/plugin.go` | Query/Row/Update/Delete 前置钩子：有 scope ⇒ 注入；scope 空 ⇒ 拒；无 scope ⇒ 不动 |
| 装配 | `internal/utils/db/db.go` + `internal/middleware/merchant_auth.go` | 中间件把商户 scope 放进 `c.Request.Context()`；DB 注册 plugin |
| 空串口径收口（G1） | `service/order_service.go`、`repository/llm_billing.go` | 商户面路径不再存在"空 = 全部"的分支（改由 scope 决定） |
| 门禁 | `scripts/check-merchant-scope.sh` | ①`/merchant-api` 组每条路由必挂 `MerchantAuth`；②该组 handler 不得出现 `Query("merchant_key")`/`Param("merchant_key")`；③`merchant_key` 不得从请求体绑进商户面 input |
| 行为用例 | `internal/router/merchant_isolation_scan_test.go` | 双商户真库扫描：A 签名读 B 的资源恒 404/403；A 试图以 B 的 key 下单恒拒 |

---

## 6. 附：本文用到的取证命令（可复跑）

```bash
cd hivemtk-platform/platform-server
grep -rn 'merchant_key' --include="*.go" internal/ | grep -v _test | wc -l          # 237
grep -rn 'if merchantKey != ""' --include="*.go" internal/repository/ internal/service/ | grep -v _test
grep -rn 'GetString("merchant_key")\|Query("merchant_key")' --include="*.go" internal/controller/
sed -n '61,120p' internal/router/router.go                                          # merchant-api 组 22 条
psql -h 127.0.0.1 -p 8202 -U admin -d platform_db -Atc \
  "select table_name from information_schema.columns where column_name='merchant_key' and table_schema='public' order by 1"
```
