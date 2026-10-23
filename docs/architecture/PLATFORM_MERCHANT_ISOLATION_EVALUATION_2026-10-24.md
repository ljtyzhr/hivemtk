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

### 5.1 落地结果与三处偏差（回填，平台端仓提交 `3c4ba07`）

| 原计划 | 实际落地 | 偏差原因 |
|---|---|---|
| 商户面 22 条路由 | **28 条**（含子组 `assetMarket := merchantAPI.Group("/asset-market")`） | 上文 §2.1 的 22 条是手数 `merchantAPI.*` 直接注册的；子组继承父组中间件，也是商户面。门禁按**路线树**（组 + 子组前缀拼接）枚举，才数得全——手数漏掉子组正是"装配面靠人记"的老毛病 |
| G1：「商户面路径不再存在"空 = 全部"的分支」 | **分支保留**，改由 scope 决定它看到什么 | 原计划要删分支，实施时发现删不得：同一条方法被运营面共注册，删了运营面就废了。真正的问题不是"分支存在"，而是"谁能让它跑起来"。现在的形状是：`if merchantKey == ""` 保留，商户面 ctx 有 scope ⇒ 框架强制 `AND merchant_key = ?` ⇒ 走进这条分支也只看到自己。**这里不加条件才是对的**，加条件会让运营面 500。两处站点已补注释指向框架收口，判据是 `order_service_merchant_scope_test.go` 里故意传空 key |
| 门禁 `scripts/check-merchant-scope.sh`，三条：挂 `MerchantAuth` / 不取 query / 不绑 body | `scripts/check-merchant-scope.py`，三道闸：**装配面**（先挂中间件再注册 + 不得绕出组 + 子组继承）/ **身份来源**（query/path/form 取名，共享 handler 白名单双向防腐）/ **接线锁**（注册点 ×2、身份写入↔scope 一一对应、`WithScope` 唯一）；另有 `check-merchant-scope.test.sh` 六条腿证它**能红** | ①"不绑 body"这一条**静态判不了**（要解析 struct tag + 判断哪条路由用哪个 DTO），硬做会写成一层脆弱的字符串匹配；它由运行时腿三（body 填别人 key 的下单用例）覆盖，这里如实标注而不是假装有。②实施中真正撞到的三类回归是"路由绕出组""某分支漏注入 scope""`Register` 被注释掉"，所以闸口改成对准它们；门禁本身必须证明能红，否则它只是一段会 `exit 0` 的代码 |

**用例清单（实际落地，11 + 5 + 3 例）**：
- `merchantscope/plugin_test.go` 8 例：三档语义、AND fail-closed（拿 B 的 key 查恒空）、Update/Delete 按别人 id 0 行、Create 填空/拒外、裸 SQL 自证闸、身份表豁免、**无 scope 时 DryRun SQL 与未注册连接逐字节相同**；
- `merchantscope/registry_test.go` 3 例：独立 schema 真库内省双向、ast 扫模型源码双向、豁免须有理由；
- `router/merchant_isolation_scan_test.go` 5 腿：28 条路由无凭证不成功、探针证明注入在真 HTTP 路径可达、5 条对抗式 query、body 填别人 key 下单、运营面 admin 仍看全两个商户；
- `service/order_service_merchant_scope_test.go` 3 例：G1 收口（订单 / 钱包流水 / 空 scope 必拒）。

**落地过程被双向判据抓出的两处真错**（这就是为什么不用 grep 推断清单）：
1. `revenue_records` **没有** `merchant_key` 列 —— 凭印象写进清单 ⇒ 真库内省立刻红；
2. `merchants` 的列是 `gorm:"column:merchant_key" json:"key"` 声明的 —— 只扫 `json:"merchant_key"` 会把它漏掉，
   而它恰恰是唯一需要**豁免**的表（身份表：那一列就是登录凭据）。

### 5.2 门禁后续扩到“清单”（A19，平台端仓 `23abf55`）

本轮 A3 的门禁有一个**结构性缺口**，被 A19 直接撞上：判据口径写死成 `MERCHANT_GROUP = "/merchant-api"`，
于是同仓第二个商户面 `/llm-api/v1` 从头到尾都在**判据视野之外**——它不是“闸被绕过”，而是“没人知道该有闸”。

- `GATED_GROUPS` 取代单个常量：`("/merchant-api", "MerchantAuth()")`、`("/llm-api/v1", "MerchantLLMProxyAuth()")`。
  每新增一条对外商户通道必须在这里登记它要求的中间件，否则不在判据里。**这是“清单化”而不是“多写一条 if”**：
  前者要求的是“登记不全会红”，后者只要求“这条路径上有个写着 `MerchantAuth` 的字符串”。
- 新增闸 2b：`/llm-api/v1` 组下 handler 不得自己从请求里取身份（`Authorization` / `X-Merchant-Key` /
  `merchant_key`）。这一条与闸 2 同源但**判据不同**：闸 2 判“不得从 query/path/form 取名”，
  2b 判“不得读凭据头”——A19 的原始形态正是后者。
- 闸 3b 从“只认 `merchant_auth.go`”改成 `SCOPE_INJECTORS` 白名单：每个登记文件内
  `c.Set("merchant_key")` 与 `merchantscope.WithScope(` **逐文件一一对应**，且文件里必须含 `hmac.New(`
  ——写进 ctx 的商户身份必须来自一次**真实**验签，否则它只是“相信请求头”。白名单之外的生产代码
  既不许注入 scope 也不许写身份（唯一执行者口径不变）。
- 自检六条腿 → 十条腿（新增四条对准本轮四道新判据），仍要求“注入违规 ⇒ 必须红 ⇒ 还原后 md5 全等且回绿”。

### 5.3 门禁再加一道：跨端契约版本闸（A4）

同一份门禁文件随后又长出闸 4（`CONTRACT_GUARDED` / `CONTRACT_EXEMPT`），这次不是"多一条 if"，
而是把 **A19 那个成因本身**变成判据：清单化之后，"某个对外通道没人知道该挂什么"这件事必须能被红灯照出来。

- `CONTRACT_GUARDED` = `("/merchant-api", "/llm-api/v1", "/api/platform")`：三个**唯一消费者是租户端**
  的前缀，每组必须恰好一个组入口、必须在注册路由**之前**挂 `ContractHandshake()`、不得把路由直接
  注册在根引擎上绕出去。
- `CONTRACT_EXEMPT` = `/platform` 与 `/api/auth/login`，且**反向判据**：谁给豁免前缀挂上闸就红。
  理由写进常量旁边而不是另写一份文档 —— 因为下一个人"顺手统一一下"时只会看到代码。
  这两处豁免不是偏好：`/platform/**` 是平台自身 Web 前端（platform-web）在调，
  `/api/auth/login` 是浏览器登录与租户端取 JWT 共用的同一个端点，浏览器没有"契约版本"这回事。
- 自检十条腿 → 十二条腿（⑪ 摘 `/merchant-api` 的 `ContractHandshake`、⑫ 给豁免的 `/platform` 挂闸）。
  ⑪ 是本轮唯一"新加一条腿时顺手发现老腿文案要改"的地方：摘掉组上唯一的 `Use` 之后报错是
  `没有 ContractHandshake()（实际: …）`，所以 ①⑦ 的期望文案要跟着从"没有挂任何中间件"改掉。

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
