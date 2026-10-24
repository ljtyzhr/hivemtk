# SaaS 全链路改造检查报告：租户隔离 / 数据隔离 / 双形态耦合剥离

> 日期：2026-10-23 ｜ 范围：`user-server`（用户端，云 SaaS + 本地独立部署同一产物）+ `hivemtk-platform`（运营端）+ 两端的接缝
> 事实源：`specs/001-saas-multi-tenant/`、`docs/architecture/SaaS_MULTI_TENANT.md`、`docs/architecture/SAAS_MULTI_TENANT_EVALUATION_2026-10.md`、`docs/architecture/DEPLOYMENT_MODES_EVALUATION_2026-10.md`
> 性质：**检查 + 本轮已闭环项 + 待批准项**。本轮改动的代码都带回归用例；未改的每一项都写了"为什么不这轮改"。

---

## 0. 一句话结论

**隔离机制本体是好的，问题出在"机制之外的通道"**：凡是不经过 GORM 隔离 callback 的东西——
**进程内推送（WS 广播）、静态文件服务（/files）、进程级内存缓存（归属缓存）**——
都没有纳入租户判据。它们不在 callback 的射程内，因此 `make test-go` 全绿、
`check-gorm-only` 全绿，也没有任何一条用例会红。

本轮检查出的形态是同一个：**"记下了租户、但没有任何一处用它判定"**。

| # | 问题 | 严重度 | 状态 |
|---|---|---|---|
| P0-1 | WS `BroadcastToMerchant` 遍历全部连接广播 ⇒ on 模式下**跨租户推送载荷**（SOP 的 session/customer/content 进别人工作台） | 高（数据泄漏） | **已修 + 3 例回归** |
| P0-2 | `/files/*` 静态服务**不看路径里的租户段** ⇒ 任何拿到 URL 的已登录租户可读别人租户的文件（tasks.md T011 声称"校验租户归属"，代码里那半边从未实现） | 高（数据泄漏） | **两轮修完**：一轮的 `tenantPathAllowed` 实测在生产上走不到（见 §3 P0-2 的更正），二轮换成"on 模式只认签名凭证 +URL 不含租户段"（A1/A12），含 6 组用例 + 存量重签迁移 |
| P1-1 | 归属校验缓存的键是 `table:id`、**无租户段** ⇒ 一个租户的归属判定命中另一个租户，且其 record-not-found 语义是 fail-open | 中高 | **已修 + 2 例回归** |
| P1-2 | 无凭证公开外链（访客 `<img>`、分享页）拿不到租户归属 ⇒ 只能靠 UUID 不可枚举 | 中 | **已修**（A1）：`/files` 在 on 模式只认 `?s=` 签名凭证，缺失/篡改/过期一律 404；签名自指租户目录 ⇒ 跨租户不可表达 |
| P1-3 | 通用归属中间件 `RequireOwner*` 的 fail-open 分支（`checker==nil ⇒ 放行`、`db==nil ⇒ (0,nil)`）+ not-found 混成 500 | 中（潜在） | **已修**（语义收口 + 6 例回归，见 §3） |
| P1-4 | 平台端**商户面**（`/merchant-api`，外部主体）的商户分域**全靠手写 `Where("merchant_key = ?")`**，无框架约束、无门禁、失败模式是静默跨商户 | 中高（云端侧） | **已落地**（A3）：只收商户面 28 条路由，`internal/utils/merchantscope` 三档注入器 + 静态门禁（含"证能红"自检）+ 双商户交叉扫描五腿；运营面**有据不做**（论证见 `PLATFORM_MERCHANT_ISOLATION_EVALUATION_2026-10-24.md`） |
| P1-6 | `/llm-api/v1` 这个**第二个商户面**没有身份闸：身份取自 `Authorization: Bearer <merchant_key>`，而 `merchant_key` 在整套 HMAC 体系里是**标识符**（每次调用都以 `X-Merchant-Key` 明文出现在请求头、平台管理端列表里 `json:"key"` 直出），真正的凭据是每商户的 `api_secret` ⇒ 知道一个商户 key 就能**烧掉那个商户的 LLM 余额**，且可永久重放 | 高（云端侧） | **已落地**（A19 · 平台端仓 `23abf55`）：`/llm-api/v1` 挂 `MerchantLLMProxyAuth()`（HMAC over `method→RequestURI→ts→nonce→sha256(body)`、±300s、按 nonce 去重）+ controller 只读 ctx（Bearer 回落已删）+ 兼容窗口显式开关（默认要求签名、关掉则启动告警且放行留痕）+ 租户端同口径签名（保留 Bearer 使升级顺序自由）+ 跨仓契约向量；用例 11 例 + 真装配腿五 + 十条腿"证能红"自检。**落地时对计划稿有三处偏差，逐条记在 §3 P1-6「已落地」** |
| P1-5 | 云/本地之间**没有版本/契约握手**：老实例连新平台只能靠运行时 500 发现字段不匹配 | 中（运维） | **已落地**（A4）：契约版本 `Version=1` / 头 `X-Contract-Version`，两侧各钉一条 pin 用例；平台端 `/merchant-api`+`/llm-api/v1`+`/api/platform` 三组挂 `ContractHandshake()`，**两个方向**都回**真 HTTP 426**（`response.UpgradeRequired`）；默认 fail-closed + `PLATFORM_CONTRACT_MIN_VERSION` 显式窗口（启动告警 + 每次未声明放行留痕 + 非法值回落严格档）；租户端收敛成"唯一构造入口 + 唯一判定点"，426 经 `/api/system/info.platform_contract` 让管理员看见（`omitempty` 保证 off 模式逐字节等价）。`/platform/**` 与 `/api/auth/login` 刻意豁免并有反向防腐判据 |
| P2-1..6 | 限流键无租户维度 ✅、11 张租户表缺 tenant_id 前导索引 ✅、知识库导入路径未分域 ✅、cron 扇出无门禁 ✅、事件总线 TenantID 靠自觉 ✅、双仓库同名模型漂移 | 低—中 | 6 项中 5 项已修，余 1 项在清单 |

**"一套代码一套逻辑"的实测结论**：部署形态确实由**两个环境变量**（`TENANT_MODE` / `PLATFORM_ENABLED`）单点决定，代码里**没有**任何 `if deploy == "saas"` 式的形态分支——这一条是真的。
真正威胁"维护两套"的不是形态分支，而是**同一语义在两个通道里各写了一遍且口径不一致**（SSE 按租户过滤、WS 不过滤；落盘层做了租户前缀、服务层没校验）——本轮修的就是这一类。

---

## 1. 方法与判定口径

三条硬标准，缺一条就不算"实证"：

1. **活体代码**：结论必须能指到 `文件:行`，不接受"文档说 X"。
2. **真库内省**：表结构类判断一律对真 PG（`mtk-postgres:8202`，371 张表）查 `information_schema` / `pg_indexes`，不接受"模型里写了"。
3. **能红的用例**：修复必须伴随一条**在修复前会红**的用例。本轮 9 条用例都做了反向验证。

本轮实际执行的验证（命令与结果见 §5）。

---

## 2. 机制本体体检：实测通过项（这些不该被后面的建议推翻）

| 体检项 | 判定标准 | 实测结果 |
|---|---|---|
| 隔离执行者唯一 | 业务包不得出现手写租户过滤/裸 SQL | `check-gorm-only.sh`：**0 处** `.Exec(`/`.Raw(` |
| 登记表判据唯一 | 「是否租户表」只由**表上有没有 `tenant_id` 列**决定 | 真库 371 张表，12 张无 `tenant_id`，**恰好等于**模型侧全局豁免清单（`tenant_callback_test.go` 的反向不变量） |
| 唯一性租户化 | 租户表上的唯一索引/约束必须已复合为 `(tenant_id, …)` | 只剩 1 条非复合唯一索引：`global_uniq_system_users_email`（刻意豁免，邮箱跨租户全局唯一）。**无遗漏** |
| 写入侧不串租户 | Create 填充 / 跨租户写拒绝 / `Omit`·`Select`·`Save` 绕过 | `tenant_callback_adversarial_test.go` 覆盖 D1/D2/D3 三处历史缺陷 |
| 推送通道租户分桶 | 只能收到本租户事件 | **SSE ✅**（`sse_hub.go` Publish 按 `client.tenantID` 过滤）；**WS ✅**（P0-1 + 二期：连接两级桶 + 发送口强制带租户） |
| 后台任务租户扇出 | 周期任务按活跃租户逐个执行 | 全仓 **40+ 处** `tenant.Fanout`，覆盖 cron / SOP outbox / 告警 / RFM / churn / RAG eval / GEO / browser_automation / 邮件列表 |
| 后台任务登记门 | 新增后台任务必须显式声明"在哪扇出 / 为什么与租户无关" | `scripts/check-tenant-ctx-writes.py`：7 个注册点全部登记（fanout / tenant-free 各有据）；seam 回调首参必须 `context.Context`（这条门是 2026-10-22 一场线上静默空转之后建的） |
| 外部入口租户解析 | 渠道 webhook / bridge / Widget / 访客 WS / 邮件追踪 | 四条入口全部前置解析 + 失败拒绝 + 安全打点 |
| 离线形态零回归 | off 模式 SQL/键形/路径与历史逐字节一致 | callback 直通、Redis 键无前缀、uploads 无租户段；本轮三处修复都保留了 off 分支 |
| 启动期 fail-closed | 开关拼错 ⇒ 拒绝启动（不是静默降级） | `TENANT_MODE` 拼错 `log.Fatalf`；`PLATFORM_ENABLED` 故意反过来（只少可选功能，不许拖垮私有化部署） |

---

## 3. 缺陷清单

### P0-1（已修）WS 广播跨租户：`Hub.BroadcastToMerchant`

**证据（修前）**：`internal/websocket/hub.go:182-207`

```go
// 私域部署：单租户全广播
func (h *Hub) BroadcastToMerchant(merchantID string, ...) error {
    h.mu.RLock()
    for agentID := range h.clients { agentIDs = append(agentIDs, agentID) }  // 全部连接
```

**三个生产调用点**，payload 都带业务数据：

| 调用点 | 载荷 |
|---|---|
| `internal/service/sop_node_executors.go:165` | SOP 节点推送：`session_id` / `customer_id` / `content` / `source` |
| `internal/websocket/hub.go:307` `BroadcastAgentStatus` | 坐席在线状态（其他租户可探测人员在线情况） |
| `internal/websocket/handler.go:340` `BroadcastMessage` | 通用广播口 |

**判定**：`Client.tenantID` 在两条握手链路（坐席 `handler.go:166`、访客 `visitor_handler.go:212`）都被正确绑定，**但全仓没有任何一处读它做过滤**（`grep '\.TenantID()'` 只命中访客测试）。同包 SSE 早就做对了——**同一需求两个通道两套实现，其中一套漏了判据**。

**修法**：
- `BroadcastToMerchant(ctx, merchantID, …)` 改为 **ctx 携带租户**：on 模式只发 `client.tenantID == ctx 租户` 的连接；**ctx 无租户 ⇒ 返回 error、一条都不发**（fail-closed，绝不退化成"全实例广播"——那正是缺陷本身）。
- off 模式：**逐字节等价历史**（发全部坐席），离线单机零回归。
- 三个调用点补 ctx（`sop_node_executors.go` 的 `Execute(ctx, …)` 本来就有）。

**回归**：`internal/websocket/hub_tenant_broadcast_test.go`（3 例）
- on 模式：租户 5 的广播只有租户 5 的坐席收到（跨租户 150ms 静默）；
- on 模式无租户：返回 error 且无人收到；
- off 模式：两坐席都收到（含"ctx 带租户也不参与判定"的历史口径）。

---

### P0-2（已修）`/files` 静态服务不校验租户归属

**证据（修前）**：`internal/router/files_guard.go:48-63` —— 只判「危险扩展名」+ 防穿越，**完全不看路径**；而落盘层 `internal/storage/local.go:52-61` 在 on 模式下会把新上传写成 `tenant_{id}/…`，URL 由 `PublicURL` 原样拼出。

**判定**：`tasks.md` T011 写的是"静态服务校验租户归属（跨根目录穿越拒绝）"——**落盘半边有实现、服务半边从未实现**，声明与实现漂移，且没有任何用例会红。攻击面：URL 会经导出文件、日志、聊天转发外流，UUID 不可枚举是**唯一**防线。

**二轮修法（A1/A12，2026-10-23 定稿）**：`tenantPathAllowed(ctx, rel)` 那版**已退役**，理由见下一条更正。现行判据只有一条：**on 模式下 `/files` 必须有签名凭证，验不过一律 404**。

- 签名承载租户归属与过期时间：`/files/<相对路径>?s=<AES-GCM token>`，token 的明文是 `{p: 相对路径, t: 租户, e: 过期戳}`。**URL 里不再出现 `tenant_{id}` 段**（A12）——租户信息进了加密 token，不是换个位置明文摆放（base64 不是加密）。
- 跨租户访问在结构上不可表达：签名自指它自己的租户目录，路径段改一个字节即 `ErrFileURLInvalid`。凭证的三条失效通道（缺失/篡改/过期）各有断言。
- 撤销：轮换 `FILE_URL_SECRET`（未配时回落 `USER_JWT_SECRET` → `JWT_SECRET`，都未配则签名器不可用并响亮报错）。
- 签发点唯一：`storage.LocalDriver.PublicURL`（五个上传调用点都经它），消费侧零改动；`off` 模式输出与历史**逐字节一致**。
- 时效：`FILE_URL_TTL`（如 `168h`）可配，**默认不过期**——URL 会被持久化（`materials.url`、`message_hub.media_url`、卡片 `image_url`…），短时 token 会在用户翻历史时自己到期，变成"图片某天开始全裂"。过期校验在活体代码里（`TestVerifyFileURL_ExpiresWhenTTLSet` + 守卫侧 404 用例）。
- 存量 URL：`v3_80_0_file_url_sign_migration.go` 把登记的 17 个 URL 列里 `/files/tenant_<id>/…` 就地重签（幂等；off 模式零动作；缺密钥时不改写、只响亮记一条并提示重跑）。**列宽守卫**：签名形态比明文长 ~100 字符（真库 `live_code_qrs.image_url` 只有 `varchar(255)`），装不下的行跳过并逐行 WARN —— 迁移不许把可修的数据问题放大成"升级卡在一行数据上"。**漏登记体检（只读）**：登记清单是人工审过的**单值** URL 列；通用上传接口的返回值会被前端存进任意头像/封面列，漏登记的表现是"那一列历史链接全裂且无声"。故收尾再按列名形态（`url|avatar|image|cover|logo|media|icon|thumb|photo|banner|qrcode|path`）扫一遍 `information_schema`，凡未登记却仍命中 `/files/tenant_%` 的列**逐列 WARN**；这一档**永不写库** —— 多值列（JSON 数组、逗号分隔 `images`）用"整列换成一条 URL"去改就是数据销毁。

**回归**：`internal/storage/fileurl_test.go`（形态/往返/篡改/过期/无密钥/整段匹配）+ `internal/router/files_guard_tenant_test.go`（六组：无签名 404 / 签名 200 / 篡改与过期 404 / 签名绑租户 / off 逐字节等价 / 危险扩展名仍 403）+ `v3_80_0_…_test.go`（重签/幂等/off 空跑/缺密钥保留/形状边界）。

**更正（本轮最重要的发现，与 §9 台账 P1-3 那条同体例）**：上一轮 `tenantPathAllowed` 的"ctx 已绑定租户 ⇒ 必须同租户"分支**在生产上从未生效**。`/files` 是 `Setup` 里最先注册的路由，而 gin 的 `r.Use` 只对之后登记的路由生效 ⇒ 它的 handler 链上只有一个 handler，没有 JWTAuth/TenantMiddleware ⇒ 请求 ctx 里永远没有租户绑定 ⇒ 那段 `if` 恒走"无绑定 ⇒ 放行"。**实测（改前）**：带「租户 6」JWT 请求 `/files/tenant_5/x.pdf` ⇒ 200；不带 token ⇒ 200。上一轮那 4 例之所以绿，是因为它在进程里手动 `req.WithContext(tenant.WithTenantID(...))` 注入了一个生产路径上不存在的状态 —— 这正是本仓最值得记的失效模式：**判定存在，但没有任何一条生产路径能走到它**。所以本轮换判据而不是打补丁：凭证只剩签名一种，且它不依赖任何中间件链。

---

### P1-1（已修）归属校验缓存无租户段 ⇒ 跨租户命中 + fail-open

**证据**：`internal/middleware/ownership.go`
- 缓存键：`table + ":" + resourceID`（进程级全局 map）；
- `DBOwnershipChecker.GetOwnerID` 的 not-found 语义：返回 `(0, nil)`，而中间件把 `ownerID == 0` 判为「系统级资源，允许访问」。

**判定**：on 模式下同一条 `SELECT owner_id … WHERE id=?` 在不同租户 ctx 下结果不同（callback 注入 `tenant_id`）。键里不带租户 ⇒ 租户 A 查"资源 42 不存在"（0）会被缓存 5 秒，租户 B 的同一 ID 请求直接命中这条 0 ⇒ **放行**。这是**缓存把 fail-closed 变成 fail-open**。

**修法**：键加租户段（on 模式 `{tenant}|table:id`，off 模式与历史逐字节一致）；`InvalidateOwnershipCache` 改为同时清裸键与全部租户前缀键。

**回归**：`internal/middleware/ownership_cache_tenant_test.go`（2 例；第二例断言跨租户请求**必须再打一次 checker**，即缓存未跨租户命中——修前该用例红）。

---

### P1-3（✅ 已修）通用归属中间件的 fail-open 分支

**先更正审计初稿的一处错判**：初稿写"not-found ⇒ 返回 0 ⇒ 放行（fail-open）"，那读的是
`GetOwnerID` 的**注释**。活体代码是 `if err != nil { return 0, err }` ⇒ `gorm.ErrRecordNotFound`
走 error 分支、中间件据此 500，**方向是 fail-closed**。注释与实现漂移，且漂移方向相反。

真正在的 fail-open 是另外两处（都是"装配缺失等于放行"）：

| 形状 | 后果 |
|---|---|
| `RequireOwner(k,t)` / `RequireOwnerWithCache(k,t,ttl)` 两个构造器**默认传 nil checker** | 任何路由接上就是 `c.Next()` 一律放行，而它在 review 里长得像安全件 |
| `DBOwnershipChecker.GetOwnerID` 在 `db==nil` / `table==""` / `resourceID==0` 时返回 `(0, nil)` | 落进"系统级资源"分支 ⇒ 放行 |

**修法**（`internal/middleware/ownership.go`）：
1. **删除**两个 nil-checker 构造器（0 处接线，无兼容负担）；两个保留的构造器对 nil checker **构造期 panic**；
2. `ErrOwnershipNotFound` / `ErrOwnershipMisconfigured` 两个哨兵：前者 ⇒ **404**（原来 500，
   既污染告警又把"不存在"混进真故障），后者 ⇒ 500；
3. `owner_id == 0` 默认 **404**，只有显式 `WithSystemOwnedAllowed()` 才放行
   （`owner_id=0` 可能是平台内置行、也可能是漏填 owner 的脏数据，库里长得一样）；
4. 判定逻辑抽成 `ownershipVerdict`，**缓存命中路径与无缓存路径共用**（两处各写一遍正是本仓反复出事的形状）。

**回归**（`ownership_semantics_test.go` 5 例 + 改写既有 5 例）：nil checker panic、真库 not-found ⇒
`ErrOwnershipNotFound`、四出口判定矩阵（7 行）、404 也进缓存、系统级行默认拒/显式放。

**"0 处接线"的处置**：保持不接线（不在本轮给真实端点加一层未经产品确认的归属语义），
但把"接上就会放行"的陷阱拆了 —— 现在接上它只有两种结果：构造期 panic，或按 uid 正确判定。

### P1-4 平台端商户隔离无框架约束（**二轮更正范围**）

**初稿判断错在把范围写成"平台端"**：平台端的业务本身就是"平台管理所有商户"，
运营面跨商户查询不是越权而是业务（订单列表、钱包流水、用量报表、调账、模块开通…）。
照用户端那样全量 fail-closed 会同时造成两件坏事：①运营端每个列表/报表打成 500；
②为让它跑起来得在 237 处手写条件上再逐一标注"我是系统级"，标注本身会腐烂成第二套真相。
**收益为零、回归面最大。** 论证与取证（含两侧威胁模型对照表）见
`PLATFORM_MERCHANT_ISOLATION_EVALUATION_2026-10-24.md`。

**更正后的范围**：两端威胁模型不对称 ⇒ 只收**商户面**（`/merchant-api` 组，22 条路由），
运营面登记为**"有据不做"**（其越权由 admin 会话 + RBAC 管，商户维度对它是筛选条件、不是安全边界）。

**证据（`hivemtk-platform/platform-server`，活体 + 真库）**：
- `internal/middleware/merchant_auth.go:121` 只做 `c.Set("merchant_id", m.ID)`，全仓 `merchant_id` 出现 1 次
  —— 没有任何仓储/回调层拿它过滤；手写 `merchant_key` 条件 **237 处 / 20 个非测试文件**；
- 装配面这一格是好的：`router.go:61` 起 22 条路由**全部**挂 `middleware.MerchantAuth()`（HMAC + ±5min + 写操作重放去重）；
- 身份来源面今天也没有现成洞：商户面 handler 全走 `c.GetString("merchant_key")` /
  `merchantKeyOf(c)`；`ctx.Query("merchant_key")` 只出现在运营面 handler；
- 但有三处**结构性**缺口（今天不流血、任何一次改动都会流血，且**没有任何用例会红**）：
  - **G1 空串 fail-open**：`service/order_service.go:883` `if merchantKey != "" {…} else { ListOrders(...) }`
    与 `repository/llm_billing.go:346` 同形 —— 同一条方法被 `/merchant-api/orders` 与 `/platform/orders`
    共注册，安全与否只取决于 `controller/order_controller.go:54` 那两行的**书写顺序**（先 ctx 后 query）；
  - **G2 越权判定散落在 controller 的 if 里**：`OrderController.Detail` / `MerchantCreatePay` 先按订单号取行、
    再比对 `d.Order.MerchantKey != mk ⇒ 403`，而 repo 的 `GetByOrderNo` 不分商户 —— 忘写一次比对就是越权；
  - **G3 403 vs 404 可枚举**：取不到行与不给取走两条语义，订单号空间成为可枚举面（用户端 A16 那条闸取向相反：
    不存在与外租户一律 404）。

**结论**：这是两端最本质的**能力不对称**——用户端把隔离吃进框架（漏了 fail-closed 报错），
平台端把它留在业务代码里（漏了**静默跨商户**）。判据改写为（A3 改写版）：
①`/merchant-api` 组请求 ctx 带商户 scope 时，商户表查询由框架注入；②scope 为空即拒（不退化成"看全部"）；
③门禁：该组每条路由必挂 `MerchantAuth`、该组 handler 不得从 body/query 取商户身份；
④双商户交叉扫描：A 的签名读 B 的资源恒 404/403，A 用 B 的 key 下单恒拒。
注入器三档（有 scope ⇒ 注入 / scope 空 ⇒ 拒 / 无 scope ⇒ 不动）与用户端 `tenancyidx` 是
**同一执法模型的两个档位**，不是两套逻辑 —— 无 scope 那一档正是"运营面零回归"的保证。

---

### P1-6（✅ 已修 · A19）LLM 代理把**标识符**当成了**凭据**

**证据（运营端 `hivemtk-platform/platform-server`，活体）**：
- `internal/router/router.go:215` `llmAPI := r.Group("/llm-api/v1")` **不挂任何中间件**
  （代码里有一大段注释解释为什么不挂 `JWTAuthMiddleware`——那解释是对的，但结论跳了一步：
  "不能用管理员令牌"不等于"不需要身份闸"）；
- `internal/controller/llm_proxy_controller.go:79` `merchantKeyFromBearer` 从
  `Authorization: Bearer <merchant_key>` 取商户身份，**不验签、不校验时效、不绑定请求体**；
- 租户侧 `user-server/internal/platform/llm_proxy_client.go:123` 只发 Bearer，
  不带 `X-Timestamp`/`X-Signature`（同一台机器上它给 `/merchant-api/**` 的请求**是**签名的，
  `client.go` 的 `do()` 有一套完整 HMAC —— 两条通道一个签一个不签，就是"同一语义两个通道口径不一致"）。

**为什么这是真的洞，而不是"key 本来就是秘密"**：
在整套 HMAC 体系里，`merchant_key` 被设计成**标识符**，秘密是每商户的 `api_secret`：
- 每一次正常调用都以 `X-Merchant-Key: <key>` **明文**出现在请求头（`merchant_auth.go:54`）；
- 平台管理端商户列表 `model.Merchant.Key` 的 tag 是 `json:"key"`，直接下发；
- 签名载荷是 `method\npath\nts\nbody`，**key 不进签名**，所以 key 从不需要保密。

这套设计本身没问题。问题在于 `/llm-api/v1` **把标识符当成了 bearer 凭据**：
知道一个 key（不是 secret）就能以那个商户的身份调用平台模型、扣它的钱包、
并且 query 里还能自己指定 `tenant_code` ⇒ 连"另一个商户的某个租户"都能精确打成。
又因为没有任何时效/重放约束，一个抓到的请求可以**永久**重放。

**结论**：这是同一个仓里第二次出现"商户面缺身份闸"，但形态与 A3 不同 ——
A3 是"有闸但隔离靠手写"，这里是"压根没有闸"。严重度按后果定：**高**（直接烧钱，
且不需要拿到 secret）。它同时是"一套代码一套逻辑"这一轮里唯一**必须跨端联合发布**的一项。

**修法（已定，登记为 A19）**：
1. 平台侧给 `/llm-api/v1` 挂一个专用验签中间件：`ts` ±300s + `HMAC-SHA256(api_secret, method\npath\nts\nSHA256(body))`
   ⇒ 无签名/篡改/过期/换 key 一律 401；复用 `MerchantAuth` 的密钥解析链（文件/环境变量/全局回退），
   但**不复用它的写操作重放去重**（LLM 调用是幂等语义的读，客户端重试会带同一 ts，去重会把正常重试打成 401）。
2. 租户侧 `llm_proxy_client` 补签名：密钥取同一个来源（把现在散在 `Client.merchantSecret` /
   `.merchant_api_secret` 文件 / env 三处的解析收成一个包级 `GetMerchantSecret()`，作为唯一事实源）。
3. 兼容窗口必须**显式**：`PLATFORM_LLM_PROXY_REQUIRE_SIGNATURE`（默认 `true`）。
   老租户部署（升级了平台、还没升级 user-server）会 401 —— 这是**要暴露的**，
   所以开关名写成"要求签名"而不是"允许未签名"：默认安全，降级需要动配置并在启动期打**响亮告警**
   （口径同 A13：不静默降级）。
4. 判据：无签名/篡改/过期/换 key/重放（过期 ts）各一条用例恒拒；签名正确 200 且扣的是**自己**的钱包；
   别人的 key + 自己的签名 ⇒ 401（证明 key 与 secret 必须配对）；`PLATFORM_ENABLED=false` 时整组不注册（零回归）。

**为什么不在本轮直接改**：它需要与 user-server 同版本发布（否则老部署的 LLM 代理会断），
属于 A17 那一类"有部署时序"的项；代码改动本身不大，但要与 `PLATFORM_LLM_PROXY_REQUIRE_SIGNATURE`
的默认值、告警文案、以及"哪个版本开始强制"一起落。

**已落地（平台端仓 `23abf55`；上面的"修法"是计划稿，落地时有三处偏差，逐条记）**：

- **签名口径比计划稿多两项：`RequestURI` 而非 `path`，以及 `nonce`**。
  实际负载 `= "llm-proxy-v1\n" + method + "\n" + RequestURI + "\n" + ts + "\n" + nonce + "\n" + hex(sha256(body))`。
  ①签 `path` 是错的：`provider / scenario / tenant_code` 全在 query 里（body 必须是 OpenAI 协议原文，
  塞不进去），不签 query 等于允许把一个合法请求的 `tenant_code` 换成**同商户另一个租户**——账记到别人
  头上而签名仍然"正确"，这正是本项要堵的东西之一。②`nonce` 解决的是"按什么去重"这个问题：
  签名输入里没有随机量 ⇒ 同秒同 prompt 的两次**合法**批量调用会算出**完全相同**的签名。
  所以计划稿第 1 条那句"不复用写操作重放去重"落地成**按 `(商户, "llm:"+nonce)` 去重**，而不是"完全不去重"：
  键带 `llm:` 前缀与 `MerchantAuth` 共用 `replayCache` 但不共享命名空间（复用签名值当键会误杀同秒批量调用，
  误杀的后果不是"少一次调用"而是让人把闸关掉，比没有闸更糟）。
- **计划稿第 4 条的"`PLATFORM_ENABLED=false` 时整组不注册（零回归）"没有做，也不该做**。
  `/llm-api/v1` 在本项之前就是**无条件**注册的（`PLATFORM_ENABLED` 管的是 user-server 的平台链，
  不是平台端自己的组），凭空加一个条件注册反而是新行为。零回归的口径改为：注册形状不变、只加中间件，
  并由平台端全量回归（`env -u LLM_ENC_KEY go test -p 1 ./...` 全绿）与"签名正确必须穿过闸"的正对照腿兜住。
- **身份解析从"多一层回落"收成"只有一处"**：`LLMProxyController` 删掉 `merchantKeyFromBearer` 的
  Bearer 回落。计划稿只说要"验签"，没提这条回落本身也是洞——中间件挂上之后它**不可达**，但留着它就是
  第二个身份来源：哪天有人把 `Use(MerchantLLMProxyAuth())` 摘掉，症状会从"全部 401（一眼看见）"退化成
  "看起来正常但谁都能冒充（看不见）"。同理，窗口期的未签名分支**仍然**校验商户存在且 active
  （与 `MerchantAuth` 的免签路径同口径），否则"窗口期"里随便编一个 key 就能拿到 ctx 里的商户 scope。
- **判据侧两个坑（都已写进用例注释，因为它们是"负例能红、正对照恒绿"的典型）**：
  ①反向腿最初只断言"业务码非 0/200"，而把 `Use(MerchantLLMProxyAuth())` 整行摘掉后 controller 仍会 401
  （取不到 ctx 身份）⇒ **闸没了用例全绿**；改成判**拒绝文案出自闸**（"缺少商户密钥"/"缺少请求签名"/
  "签名错误"）才真的钉住"拒绝来自验签"。
  ②`response.Error` 是 **HTTP 200 + 信封 code**，正对照用 `w.Code == 200` 判"通过"会**恒绿**——
  而正对照一旦恒绿，它保护的负例就全是假的。正对照一律看信封业务码。
- **租户端**：`GetMerchantSecret()` 收成唯一事实源（文件 → env → `PlatformCfg.Secret`），
  `llm_proxy_client` 补发 `X-Merchant-Key`/`X-Timestamp`/`X-Nonce`/`X-Signature` 并**保留** `Bearer`
  （新客户端仍能对着只认 Bearer 的旧平台工作 ⇒ 升级顺序自由），跨仓契约向量 `3ff32cbd…a676` 两侧各钉一份。
  一处自我纠正：中途把 secret 做成过包级 `atomic.Value`，实测当场把 `TestSignSecretPrecedence` 打红
  （进程级可变全局让用例互相污染，且"谁在什么时候覆盖了它"不可追）⇒ 回退为**文件**作为跨消费者共享通道。
- **门禁**：`check-merchant-scope.py` 从"一个组"改成清单 `GATED_GROUPS`（否则"第二个商户面"永远在判据之外，
  这正是本项的成因），新增闸 2b（llm 面 handler 不得自读身份头）、闸 3b 收成 `SCOPE_INJECTORS` 白名单 +
  逐文件 `c.Set ↔ WithScope` 一一对应 + **每个注入点必须含 `hmac.New(`**（写进 ctx 的身份必须来自一次
  真实验签，否则它只是"相信请求头"）；自检六腿 → 十腿。

### P1-5（✅ 已修 · A4）云 / 本地之间没有契约握手

**证据**：`internal/platform/client.go:491,545` 只在 install/heartbeat 载荷里**上报** `version`；平台侧没有任何 `min_version` / 兼容判定（`grep -i "compat|handshake|min_version"` 无命中）。开户登记字段不全时会 500（历史实测记录在 `DEPLOYMENT_MODES_EVALUATION_2026-10.md` §9.2）。

**判定**：独立部署的实例生命周期远长于云端。老实例连新平台时，字段缺失/新增只能靠**运行时报错**发现，且失败常被 `logger.Warn` 吞掉（`internal/platform/sync.go:96` 明确写"可忽略"）。这正好是"一套逻辑跑两种部署"最容易出事的地方。

**建议**：平台在 `/merchant-api/*` 加一道 `X-Client-Version` 校验（低于 `min_supported` ⇒ 明确 426 + 升级指引），用户端把该响应呈现为管理端可见的告警，而不是只进日志。

**已落地（三处对建议的修正，逐条记）**：

- **口径从"`X-Client-Version` 校验"改成"契约版本 + 双向"**。建议稿只写了"低于 min ⇒ 426"一个方向，
  但独立部署还有一个真实形态：**实例自己先升了级**（它不归平台统一发布）。新字段平台读不懂时的
  后果与"老实例连新平台"同样是**静默**的——"存进去但永远不生效"。所以 `declared > Version` 也回 426，
  且两种方向文案不同（运维动作是"升租户"还是"升平台"完全不一样）。
- **"明确 426"必须是真 HTTP 426**，不能是 `response.Error(c, 426, …)`（本仓的 `Error` 写 HTTP 200 +
  信封码）。理由就是本项自己的判据：要"一眼认出"的是**旧客户端**，而旧客户端恰恰是那个读不懂新信封的
  一方；把语义塞进它不认识的地方等于没给。新增 `response.UpgradeRequired`，HTTP 与信封**都**是 426。
- **挂载面比建议稿宽，也比"全都挂"窄**：`/merchant-api`、`/llm-api/v1`、`/api/platform` 三组
  （唯一消费者是 user-server）。**刻意豁免** `/platform/**` 与 `/api/auth/login` —— 它们是平台自身
  Web 前端（platform-web）与租户端共用的前缀，浏览器没有"契约版本"这回事，加了会把平台前端打挂。
  豁免写进门禁的 `CONTRACT_EXEMPT` 并**反向判据**（谁给豁免前缀挂闸就红），否则下一个人"顺手统一"就打死前端。
- **默认 fail-closed，窗口显式**：`MinSupported()` 不设环境变量时 = `Version`。若默认放行未声明版本，
  这道闸在默认配置下永远走不到 —— 判据存在但没有任何生产路径能触发，是本仓最贵的失效模式。
  要开窗就 `PLATFORM_CONTRACT_MIN_VERSION=0`：启动期响亮告警 + **每次**未声明放行记日志。
  **非法版本头一律当 0（不是当最新版）**、**非法开关值一律回落 `Version`**：写错的开关/垃圾头
  不能变成"全放行"。
- **租户端按"唯一执行者"收**：`newPlatformRequest` 是每一次出站的唯一构造入口（7 处 `http.NewRequest`
  全替换），`noteContractVerdict` 是唯一判定点。刻意不给构造器留 `if guarded` 开关——那会多出一份
  要在两处同步的清单；多带一个没人读的头代价是零。
- **第三档（非 426 非 200 不动）与"只有受闸路径的 200 才能清告警"是这轮最容易写错的地方**：
  `/health`、登录、`/platform/**`、贡献者接口都没挂闸，它们的 200 只说明"平台活着"。一次周期性探针
  把 426 告警清掉 ⇒ 真问题被静默掩盖，而且掩盖得很有规律，排查时几乎不可能想到是探针干的。
  这份"受闸前缀"清单是租户侧对账单，用例钉住（少登记 ⇒ 426 记不下来；多登记 ⇒ 普通 200 清掉
  别处的 426，两种都静默）。
- **426 要进管理员可见的状态**（建议稿这一条照做并加强了）：`/api/system/info.platform_contract`
  （`service.SystemInfo` 加 `*contract.Status` + `omitempty`）。指针 + `omitempty` 是**硬约束**：
  从未被拒时整个键不出现 ⇒ `off` 模式响应体逐字节等价（去掉 `omitempty` 或"无脑填空结构"两种写法
  都被用例当场打红）。它也不在 `publicSystemInfo` 白名单里 ⇒ 匿名看不到"这台机器接的平台是哪一版"。
- **一例白盒走错门（自己抓到）**：`TestNonGuardedSuccessDoesNotClearWarning` 第一版走
  `CheckConnection`，而那条路径**根本不调**记账函数 ⇒ 把"不判受闸路径"这个变异完全测不出来
  （变异后仍全绿）。改成打在决策点 `noteContractVerdict` 上并配受闸路径的正对照才真的钉住语义。
  这是"用例跑错了门"的典型：**它绿着，但它什么都没测**。
- **顺带修掉两处会被 426 替枪的假绿**：`merchant_isolation_scan_test.go` 腿一/腿五不带头时，
  "无凭证拿不到业务成功"在**身份闸被摘掉**后依然全绿（426 挡住了）；`TestRequestBodyTooLarge`
  不带头时 `body.code>=400` 让它因"版本头没带"而假绿。两处均已补 `X-Contract-Version`。

---

### P2（列入清单，本轮不改）

| # | 问题 | 证据 | 为什么不这轮改 |
|---|---|---|---|
| **P2-1**（✅ 已修） | 限流/配额键与桶**无租户维度** | 真库/活体代码：`RateLimitMiddleware` 与 `PathQuotaMiddleware` 都挂在 JWT **之前**，键只有 `c.ClientIP()`（外加 apikey 变体）。**不需要**改挂载顺序：限流中间件自行验签解析一次令牌取 `claims.TenantID`（不写 ctx，避免给未鉴权请求塞租户绑定）。on 模式键 = `t<tenant>:<clientKey>`，off 模式前缀恒空串（键形逐字节不变，且不付令牌解析成本）；无令牌/坏令牌/历史令牌回落 IP 键 —— 这一档是**显式登记的语义**（未登录流量本无租户），后果仍只剩"未登录噪声邻居"。监控读数口 `GetRateLimitStatus` 走同一键函数，读写不会指向两个桶 |
| **P2-2**（✅ 已修） | 11 张租户表的索引族里没有以 `tenant_id` 打头的列 ⇒ 隔离条件退化为过滤而非索引查找 | 真库查询（判据**只认前导列**，`(id, tenant_id)` 不算）：`bridge_metrics`、`email_lists`、`email_smtps`、`fts_test`、`intent_logs`、`livecode_click_log`、`llm_routing_audit`、`llm_usage_records`、`obs_configs`、`qr_code_click_log`、`script_recommends` | 补 `v3.79.0` 迁移（幂等、Up 后自复核、Down 只删自建索引、清单写死不扫全库）；**真库实测缺口 11 → 0**。漏因值得记：这些表走手写 `CREATE TABLE` 而非 AutoMigrate，模型的 `index` tag 对它们**不生效**，"模型说建了索引、库里没有"长期共存 |
| **P2-3**（✅ 已修） | 知识库导入文件**未按租户分域**（绕过 `LocalDriver`，直接 `uploads/knowledge-base|knowledge|knowledge-text|knowledge-url/…`） | `internal/aiagent/knowledge/service/knowledge_base_import.go:75`、`knowledge_service_import.go:48/113/177`（共 4 条通路） | 分域原先只做在 `LocalDriver.UploadReader` 里 ⇒"上传口分域了、知识库导入口没分域"，同一个 uploads 根下两种布局并存。修法：`storage.TenantPathPrefix(ctx)` 导出为**唯一**前缀函数（驱动内部同款改为调它），4 条通路各自 `filepath.Join(base, prefix, …)`。off 模式前缀为空串 ⇒ 路径逐字节不变；存量根域文件在 on 模式下仍可读（`/files` 守卫只对 `tenant_*` 首段做归属校验） |
| **P2-4**（✅ 已修） | 后台任务扇出**已有门禁**，但扫描面只覆盖 `internal/pkg/cron/` | `scripts/check-tenant-ctx-writes.py` 新增 `SCHEDULER_DIRS`（GEO/browser_automation），任务键改用 `@<路径>::<所在函数>`，登记表补 2 行；注册点 7 → **9**，自检 9 枚控制全 PASS | 不改两个调度器自己的 `AddTask` 形态（那是不必要的小重构）：**判据只需"注册点必须做一次归属决策"**，键取法随形态变即可 |
| **P2-5**（✅ 已修） | 事件总线 `TenantID` 由**发布点自行填充**（4 处各写一遍） | 新增 `event.PublishCtx`/`PublishEventCtx` + `StampTenantFromCtx`（反射盖章，非 0 不覆盖）；3 处迁移；新增门禁 `scripts/check-event-publish-tenant.py` 禁止裸 `event.Publish(`；on 模式下 `TenantID=0` 打响声 | 执行者收成唯一一个 + 门禁，才不依赖"下一个人记得写" |
| **P2-6** | 两端同名实体各写一份模型（`ConfigParam`/`ConfigParamAuditLog` 等），schema 靠人对齐 | `hivemtk-platform/platform-server/internal/model/config_param.go` vs `user-server/internal/model/config_param.go` | **已落地**（A10，解法改了）：不做"共一份代码"（两端是两个独立库、两条演进节奏，共包会把发布节奏绑在一起），改成**形状清单 + 方向性比对** —— 平台侧把跨端 DTO（开户登记/心跳/订单/模块下发，11 类型）反射成 `docs/contracts/cross-repo-wire.json`，租户侧吐自己那份，`check-wire-contract.py` 判三条：类型集相等 / 租户字段 ⊆ 平台字段 / 平台 `required` ⊆ 租户。`ConfigParam` 这类**各自库内**的同名模型照旧各写一份（谁也不会读对面的那张表） |
| **P3** | vendor 域名硬编码为默认值（`internal/config/platform.go:33` `DefaultCloudURL = https://work.xapptool.cn`）✅ 已修、`uploads` URL 暴露租户 id、`SetTrustedProxies` 只信私网 ⇒ 反代不在私网时 `ClientIP` 恒为反代 IP（全站共用一个限流桶）✅ 已修 | 见左 | P3-1（`DefaultCloudURL` 回落）：新增 `CloudURLSource` 来源标签 + 启动期 WARN，并把 provider 缺省实参从「同一个常量」改成空串（否则「没配」与「配了同一个值」不可区分，builtin 这一档永远看不到）。P3-3（可信反代）：`middleware.ConfigureTrustedProxies` 成唯一装配入口，名单可由 env `TRUSTED_PROXIES` 覆盖（非法条目整体报错而非静默丢弃），并挂观测中间件在「带 XFF 但来源不受信」时响亮一行（同对端去重 + 累计计数）。P3-2（`uploads` URL 暴露租户 id）✅ 与 P1-2 同源、一并清掉（A12）：URL 形态改为 `?s=` 签名，租户段从路径里消失 |


### P0-1 二期（已修）WS 连接侧两级桶 + 发送口强制带租户

发送侧过滤（上一节）只解决了"`BroadcastToMerchant` 发给谁"，连接侧仍然是
**一级 `map[agentID]*Client`**，于是还剩两个洞：

1. **同 agentID 跨租户互相顶掉**。坐席 ID 只是各租户库里的自增主键，"两个租户各有一个
   id=7 的坐席"是正常现象。一级 map 下后连的把先连的覆盖掉：先连那条连接既不报错、
   也不再收到任何推送，而且不会被清理（既没人 close 它，也没人知道它存在）。
2. **其余所有发送口都没有租户维度**：`SendToAgent` / `NotifyNewSession` /
   `NotifyNewMessage` / `NotifySessionUpdate` / `BroadcastToAgents` / `BroadcastToAll`。
   单播按 agentID 找 ⇒ 命中谁看谁先注册，**投给了别人**；`BroadcastToAgents` 直接遍历
   全部连接 ⇒ `chat_visitor.go:269/492`（访客开场、关键词转人工）与
   `inbox_ingress_persist.go:120`（渠道消息入库）会把 session 详情推进**所有租户**的工作台。
   修前 `Client.tenantID` 在两侧握手里都绑好了、全仓只有测试在读它——本仓那个最典型的
   失效形状（"值被记下来了，但没有任何一处 `if` 用它判定"）的又一处落点。

**修法**：
- `Hub.connections map[uint]map[string]*Client`（租户 → agentID → 连接），
  `envelopeFrame{tenantID, agentID, bytes}`；`agentOnline` 这张与连接表一一对应的
  影子表**删除**（它唯一的作用是"再记一遍在线态"，删掉后在线态=桶里有没有这条连接，
  不可能不同步）。
- 新增 `resolveSendTenant(ctx, op)` 作为**唯一**的发送侧租户解析：off 模式返回 0
  （= 不限桶，全实例语义逐字节等价历史）；on 模式取 ctx 租户；on 模式无租户 ⇒ 报错
  fail-closed。**绝不回落 0**——0 在这条链上的含义是"全部租户"。
- 所有发送口改为 ctx 形态并走同一个解析：`SendToAgentCtx` / `sendToAgentWithEnvelopeCtx` /
  `BroadcastToAgentsCtx` / `BroadcastToAllCtx` / `BroadcastToMerchant`；包的 `Notify*`
  与 `BroadcastToAgents`/`BroadcastToAll` 同步加 ctx。**没有留下任何"不带租户也能发"的口子**。
- 在线态查询同口径：`IsAgentOnline(ctx,…)` / `GetOnlineAgents(ctx)` / `GetOnlineCount(ctx)`，
  on 模式无租户答"不在线/空/0"（答"在线"会让调用方把会话派给别人的坐席）；
  顺手删掉 `GetOnlineAgents(merchantID string)` 那个 `_ = merchantID` 的假参数。
- 13 个 service 层调用点补 ctx（`session_assignment.go` ×3、`customer_session.go` ×2、
  `customer_session_routing.go` ×2、`customer_session_takeover.go`、`chat_visitor.go` ×4、
  `inbox_ingress_persist.go`）——这些函数本来就都有 ctx，说明修前缺的从来不是"拿不到租户"，
  而是"没人要求它"。

**回归**：`internal/websocket/hub_tenant_buckets_test.go`（5 例，含"同 agentID 跨租户"
这条一级桶下必红的用例）；既有 `hub_tenant_broadcast_test.go` 三态用例随桶结构更新
（`clientOf(hub, tenantID, agentID)`——只说 agentID 的查询本身就是缺陷形状）。
**牙齿**：把 `resolveSendTenant` 临时改成恒 `return 0`（模拟修前无租户维度）⇒
6 个 on 模式用例全红（"跨租户误投/跨租户泄漏/必须 fail-closed"），恢复即绿。

**顺带修掉的既有缺陷**（同一处代码、不改就是留两颗雷）：
- 同桶同 agentID 重连时旧连接没人 `close`（goroutine + TCP 泄漏）；
- 旧连接的 `Unregister` 会把**新**连接从桶里摘掉并关掉它的 send（重连即被误杀）。
  现在只摘"这条连接自己"（`existing == client`）。

---

## 4. 事实源漂移修正（本轮一并做）

`specs/001-saas-multi-tenant/tasks.md` 里 8 项标着「现状 —— 待对方 lane / 待集成轮」，但代码里**已经实现**。**声明与实现不一致本身就是"维护两套真相"**——比缺陷更危险，因为它会让下一个人按过期状态重复劳动或误判覆盖度。

| 任务 | 原表述 | 实测判定 |
|---|---|---|
| T021 事件总线 | 待对方 lane 补 `payload.TenantID` | 已实现（`event/types.go` 三类载荷） |
| T023 bridge 归属 | 待联调 | 已实现（`bindBridgeTenant` 前置三条入口） |
| T024 访客链 | 待前端联调 | 已实现（AppKey / 访客 WS / public_tenant 全链） |
| T025 邮件/telegram | 待收口 | 已实现（token claim 捕获租户） |
| T026 WS/SSE 分桶 | 待集成轮 | SSE 已实现；WS 已修（P0-1 发送侧过滤 + 二期连接侧两级桶/发送口带租户） |
| T027 唯一索引对账 | 并入二期 | **真库实测已闭环**（仅剩刻意豁免的 `global_uniq_*`） |
| T028 越权回归 | 待集成轮 | 已有 5 组；"逐端点双租户交叉扫描"**已补**（见 §6 末行与 §7：`cross_tenant_scan_test.go`，三条腿 + 三枚牙齿） |
| T019 迁移真库演练 | 随部署进行 | **已落地（A17 + 头脑风暴加固）**：`tenant_upgrade_rehearsal_test.go` 在真库上造"升级前形态"表 + 存量行，走"升级 → 重入 → 回滚 → 再升级"；判据来自 `information_schema` 与行内容；**正对照**是一条 `tenant_id=2` 的行在重入后仍是 2（抓"回填归 1"这类数据破坏写法）；`Down` 是显式 no-op 并被断言"回滚不销毁在用对象"；对抗性复盘补两洞：形态错列（varchar）不再被静默跳过（枚举口径收紧为 bigint/NOT NULL + Up 后复核 `verifyColumns` 报错中断 —— PG 15.18 实测 `IF NOT EXISTS` 对已存在列是纯 no-op）、生产注册点不得携带 `withTables` 覆盖清单（`TestTenantColumnBatchRegistration`）；七条变异逐一验红 |

---

## 5. 验证口径（本轮实跑）

```bash
# 0) 测试库凭据（仓库 .env 的密码是占位符，必须取容器真值）
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD=<mtk-postgres 容器口令>

cd user-server
go build ./...                                             # ✅
bash scripts/check-gorm-only.sh                            # ✅ 0 处 .Exec(/.Raw(
go test ./internal/pkg/tenant/... ./internal/pkg/tenancyidx/...   # ✅
go test ./internal/websocket/ ./internal/middleware/ ./internal/router/ -count=1  # ✅ 三包全绿

# A1/A12/A18 轮（签名 URL + 存量重签 + 附件分域）
go test ./internal/storage/ ./internal/pkg/mail/ ./internal/email/... \
        ./internal/router/ ./internal/migration/... ./internal/pkg/cron/ -count=1   # ✅ 全绿
go test ./internal/content/... ./internal/browser_automation/service/ -count=1      # ✅ 上传调用点未受影响
bash scripts/check-gorm-only.sh                            # ✅ 0 处（迁移走 GORM API + SystemScope）

# A19/A20 轮（/llm-api/v1 验签闸 + 干净环境必红）
go test ./internal/platform/ -count=1                      # ✅ 全绿（含新增 4 例签名用例）
# 平台端（独立仓）
env -u LLM_ENC_KEY go test -p 1 ./... -count=1             # ✅ 全绿（A20 判据：不 source .env）
python3 scripts/check-merchant-scope.py                    # ✅ 通过（现覆盖 /merchant-api + /llm-api/v1）
bash scripts/check-merchant-scope.test.sh                  # ✅ 十条腿「先绿后红再绿」
make llm-proxy-auth-gate                                   # ✅ 11 例 + 真装配腿五 + 十条腿「改坏即红」

# A17 轮（含存量数据的升级演练）
cd user-server && go test ./internal/migration/migrations/ -run 'TestTenantUpgradeRehearsal|TestTenantColumnBatch' -count=1 -v  # ✅ 3 例（真库；含头脑风暴补的形态错列腿与注册点腿）
cd user-server && go test ./internal/migration/... -count=1                                      # ✅ 两包全绿（migrations 30s）

# A10 轮（双仓库跨端线上形状清单）
cd user-server && go test ./internal/pkg/wirecontract/ -count=1   # ✅ 4 例（反射⇄清单）
cd .. && make wire-contract                                       # ✅ 反射用例 + 跨仓比对（11 类型/11 比对）
# 平台端（独立仓）
cd platform-server && make wire-contract                          # ✅ 3 例（11 类型全覆盖）

# A4 轮（云/本地契约版本握手）
go test ./internal/platform/ ./internal/pkg/contract/ ./internal/pkg/shared/service/ -count=1  # ✅ 全绿
# 平台端（独立仓）
go test ./internal/contract/ ./internal/middleware/ -run 'TestContract|TestParse|TestMinSupported' -count=1  # ✅ 9 例
go test ./internal/router/ -run TestContractHandshakeEnforcedOnRealEngine -count=1             # ✅ 真装配腿六
python3 scripts/check-merchant-scope.py                   # ✅ 通过（+ CONTRACT_GUARDED 3 组 / CONTRACT_EXEMPT 2 前缀）
bash scripts/check-merchant-scope.test.sh                  # ✅ 十二条腿「先绿后红再绿」
# 变异手验（每条都必须红；④ 第一次没红，见 §3 P1-5 末尾）
#   ① newPlatformRequest 不设版本头 ② doRetry 426 不记账 ③ 只记不清 ④ 不判受闸路径 ⑤ 心跳 200 不清
#   ⓐ 删 merchantAPI.Use(middleware.ContractHandshake()) ⇒ 腿六红（"GET /merchant-api/asset-market/assets 未声明契约版本竟未被 426"）

# 1) 真库内省（判定"哪些表是租户表"的唯一事实源）
psql -h 127.0.0.1 -p 8202 -U admin -d user_db -Atc \
  "select count(*) from information_schema.tables where table_schema='public' and table_type='BASE TABLE'"   # 371
# 无 tenant_id 的表 ⇒ 12 张，与模型侧全局豁免清单逐表一致
# 租户表上非复合唯一索引 ⇒ 1 条（global_uniq_system_users_email，刻意豁免）
```

**环境注记**：本机 `mtk-postgres` 的口令与仓库 `.env` 的占位符**不一致**，不导出 `POSTGRES_TEST_*`
会得到整片 `SQLSTATE 28P01`——那是环境失败，不是代码缺陷（与 CLAUDE.md §3.5 同一类坑）。

**本轮顺带修的编译红**：工作区有第二个 agent 进程并发改 `internal/app` / `internal/service`，
期间留下 `envInt` 重复声明（`content_guard_wiring.go` 与 `tool_circuit_breaker_wiring.go` 两个同名不同签名）。
按"发现即修"把前者的局部 helper 改名为 `envIntWithFallback`（5 处调用点，语义不变），编译恢复。
并发作业仍在进行的 `internal/contentguard/**` 不在本轮范围内。

---

## 6. 二期建议（承接已有评估，不重复论证）

| 项 | 依据 | 判断 |
|---|---|---|
| **无凭证外链签名化**（P1-2 / A1 / A12）✅ 已落地 | `/files` 的访客场景 + URL 暴露租户 id | `internal/storage/fileurl.go`：AES-GCM 签名 token（租户 + 相对路径 + 过期），URL 路径不含租户段；`/files` 守卫只认签名（缺失/篡改/过期 ⇒ 404）；`FILE_URL_TTL` 可配（默认不过期，因 URL 会被持久化），撤销靠轮换 `FILE_URL_SECRET`；存量 17 列由 `v3_80_0` 一次性重签。**牙齿**：守卫"验不过就放行" ⇒ 4 条 404 用例红；`PublicURL` 照发明文 ⇒ 形态与签名用例红 |
| **平台端商户面隔离上框架**（P1-4 / A3）✅ 已落地 | 两端能力不对称；运营面跨商户是业务本身 | `hivemtk-platform/platform-server`（独立仓，提交 `3c4ba07`）：`internal/utils/merchantscope` 是唯一执行者，三档语义只有**一个判定点**（`scopeTarget`）——有商户 scope ⇒ 六个 callback 强制 `AND merchant_key = ?`；有 scope 但为空 ⇒ 拒；无 scope ⇒ **一行 SQL 都不动**（运营面零回归，有 DryRun 逐字节用例）。注入的是 **AND** 不是替换：漏写条件的仓储只剩框架这一条（依然正确），写反了（拿 B 的 key 查）恒空，失败方向是"查不到"而非"查到别人的"。注册表由**真库内省 + 模型源码 ast** 双向夹住（落地时各抓到一处真错：`revenue_records` 根本没有 `merchant_key` 列、`merchants` 用 `column:merchant_key` 声明）。门禁 `scripts/check-merchant-scope.py` 三道闸（装配面/身份来源/接线锁，判据跑在**去注释后**的源码上），`scripts/check-merchant-scope.test.sh` 六条腿证它**能红**；接进 `make merchant-scope-gate` 与 `platform-ci.yml`。**牙齿**：摘 `testutil` 的 `Register` 或摘 `merchant_auth.go` 的一处 `WithScope` ⇒ 探针腿（`TestMerchantScopeReachesRepositoryThroughRealRequest`）红；给非共享 handler 加一句 `ctx.Query("merchant_key")` ⇒ 门禁闸二红 |
| **`/llm-api/v1` 验签闸**（P1-6 / A19）✅ 已落地 | 同一仓里第二个商户面，形态是"压根没有身份闸"（与 A3 的"有闸但隔离靠手写"不同） | `hivemtk-platform/platform-server`（独立仓，提交 `23abf55`）：`internal/middleware.MerchantLLMProxyAuth()` 挂在 `/llm-api/v1` 组上 —— HMAC-SHA256 over `"llm-proxy-v1\n"+method+"\n"+RequestURI+"\n"+ts+"\n"+nonce+"\n"+hex(sha256(body))`、`ts`±300s、按 `(商户,"llm:"+nonce)` 去重。三处口径**必须**与 `/merchant-api` 不同且已逐条论证：签 `RequestURI` 不签 `path`（`provider/scenario/tenant_code` 全在 query 里，不签就等于允许换 `tenant_code` 把账记到同商户另一个租户头上）；签 body 的 sha256 而非 body（最大 8MB）；带 nonce（签名输入无随机量，同秒同 prompt 的**合法**批量调用签名相同，照抄"按签名去重"会误杀，误杀的后果是有人把闸关掉）。controller 删掉 Bearer 回落只读 ctx（留着它 = 闸一摘就静默变"谁都能冒充"）。兼容窗口 `PLATFORM_LLM_PROXY_REQUIRE_SIGNATURE`（默认 true，只认环境变量；关掉则启动告警 + 每次放行留痕，且**窗口期仍校验商户存在且 active**）。门禁从"一个组"改成清单 `GATED_GROUPS` + 闸 2b（llm 面 handler 不得自读身份头）+ 闸 3b 逐文件 `c.Set ↔ WithScope` 一一对应且注入点必须含 `hmac.New(`。**牙齿**：`scripts/check-llm-proxy-auth.test.sh` 十条腿（逐条改坏 ⇒ 对应用例必红 ⇒ md5 还原回绿）；真装配腿五要求**拒绝出自闸**（判文案不判状态码——摘掉 `Use(...)` 后 controller 也会 401）；正对照一律看**信封业务码**（`response.Error` 是 HTTP 200 + code，用 `w.Code==200` 判通过会恒绿） |
| **Postgres RLS** | `SAAS_MULTI_TENANT_EVALUATION_2026-10.md` §7 已否（连接池/事务语义、表 owner 绕过、`tenant_id=0` 与 `1` 并存无权威表） | 维持"本轮不做"，但仍是侵入面的理论下限；若做，先补"哪些行该是 `tenant_id=0`"的权威表 |
| **跨端线上形状清单**（P2-6 / A10）✅ 已落地 | 两端各写一套同名结构、靠人对齐；Go 编译器看不见另一侧 | **不共代码，共形状**：平台侧（权威方）`internal/wirecontract` 把四条通道 11 个 DTO 从**活体 struct** 反射成 `docs/contracts/cross-repo-wire.json`；租户侧只对**跨端传递**的结构（开户登记/心跳/订单/模块下发）做单一事实源（`ConfigParam` 这类各自库内的同名模型刻意不收）。跨仓比对**方向性**：类型集相等 + 租户字段 ⊆ 平台字段 + 平台 `binding:"required"` ⊆ 租户；相等会造永久红，而永久红的判据等于没有判据。两侧清单落进**同一命名空间**（租户生成时把嵌套 Go 名改写成平台名）⇒ 配对=键相等，不存在会漏行的映射表。只比 **wire 口径**不比 Go 类型（`SyncedAt` 的 `time.Time`↔`string` 否则永久红），另加一条"两份清单 `version` 相等"（两仓反射器口径分叉的最低成本判据）。**牙齿**：八条变异全红（json tag 改名/口径不一致/类型集缺口/必填反向/平台侧改名/删 rename 登记/退出码三档/清单版本不一致）+ 正对照原样绿；单仓 CI 显式 `--allow-missing-platform` 且**响亮打印**"跨仓那一半证不了" |
| **跨端契约版本握手**（P1-5 / A4）✅ 已落地 | 独立部署的实例生命周期远长于云端；平台与 user-server 两仓各自发布 | 版本号 `Version=1` + 头 `X-Contract-Version`，两侧各钉一条 pin 用例（跨仓协议号，单方面 +1 会让对面全量 426）。平台端三组（`/merchant-api`、`/llm-api/v1`、`/api/platform`）挂 `ContractHandshake()`，**两个方向**都回**真 HTTP 426**（`response.UpgradeRequired`）——"高于平台实现"这一方向是建议稿漏掉的独立部署真实形态（实例自己先升了级，后果同样是静默的"存进去但永远不生效"）。默认 fail-closed，`PLATFORM_CONTRACT_MIN_VERSION` 是唯一开窗方式（启动告警 + 每次未声明放行留痕 + 非法值回落严格档 + 非法版本头当 0 而不是当最新版）。租户端收成"唯一构造入口 `newPlatformRequest` + 唯一判定点 `noteContractVerdict`"，426 经 `/api/system/info.platform_contract` 变成管理员可见状态（`omitempty` 保证 `off` 模式逐字节等价），且**只有受闸路径的 200 才能清告警**（`/health`/登录/`/platform/**` 的 200 只说明平台活着）。`/platform/**` 与 `/api/auth/login` 刻意豁免并有反向防腐判据。**牙齿**：四个变异逐一验红（不设版本头 / 426 不记账 / 只记不清 / 不判受闸路径），删 `merchantAPI.Use(middleware.ContractHandshake())` ⇒ 真装配腿六红；门禁十二条腿 |
| **形态矩阵契约门禁**（方案 E）✅ 已落地 | `DEPLOYMENT_MODES_EVALUATION_2026-10.md` §6 | `internal/app/deployment_matrix_contract_test.go`：四格 × 六条行为（出站/租户上下文/自助开户/初始化态/后台扇出/隔离 callback）= 24 条断言，全部走活体组件。两个轴**正交**是核心断言之一：租户那五条在两列数值必须相同，谁在平台开关里顺手加租户判定就会红。**牙齿**：临时让 off 模式也看 claim、让 `PlatformURLFor` 在关闭态照样返回地址 ⇒ ①②③ 三格全红 |
| **逐端点双租户交叉扫描**（T028 尾巴）✅ 已落地 | 现有覆盖是"逐域代表用例"，731 条带参数端点无一穷尽判据 | `internal/router/cross_tenant_scan_test.go` 三条互补的腿：①匿名参数路由 32 条必须能说出"租户/行靠什么定位"（链上 `Public*Tenant`/`AppKeyResolve` 自动认 16 条 + 台账登记 16 条，双向校验）；②按行自证类用未知键探测 16 次，不得 200；③真库双租户真扫 6 条不同取数风格端点。**关键口径纠正**：正对照从"状态码 200"升级为"200 且响应体含播种行 marker"——实测 `/api/email/smtp/:id` 两租户都回 200 + `{"code":4301,"message":"该功能模块未开通…"}`，只判状态码是假绿（模块闸挡在取数之前，行根本没读出来）。**牙齿**：`GetByID` 走 `tenant.WithSystemScope`（真绕过 callback；`Unscoped()` 咬不动）⇒ 闸三红；摘 `/share/douyin/:id` 的 `PublicIDTenant` ⇒ 闸一红；`renderCardChatPage` 取不到行回 200 ⇒ 闸二红 |
| **on 模式邮件附件静默丢弃**（A18，本轮顺带审计出） | 修 A1/A12 时顺手查"谁把 `/files/…` 当磁盘路径用" | 附件在 on 模式分域落盘到 `{baseDir}/tenant_{id}/attachments/…`，而解析器只认平根 ⇒ **每一条**附件都 Lstat 不中、静默丢弃（一条 WARN），邮件状态仍写"已发送"，无用例会红。修法：附件根按 URL 里的签名改选（`mail.WithAttachmentDir` + `storage.TenantDirFromFileURL`），并收成唯一构造点 `storage.LocalAttachmentResolver()`（单封投递与营销群发共用；两处各拼一遍正是成因）；验不过不回落猜平根同名文件。**牙齿**：修前 `on 模式附件没解析出来（附件会被静默丢弃）：got=[]` |

---

## 7. 本轮改动清单（可直接 review）

| 文件 | 改动 |
|---|---|
| `user-server/internal/websocket/hub.go` | `BroadcastToMerchant` 改为 ctx + 租户过滤；新增 `agentIDsForTenant`；`BroadcastAgentStatus` 补 ctx |
| `user-server/internal/websocket/handler.go` | `BroadcastMessage` 补 ctx |
| `user-server/internal/service/sop_node_executors.go` | SOP 推送传 ctx |
| `user-server/internal/websocket/hub_tenant_broadcast_test.go` | 新增：WS 广播三态回归（3 例） |
| `user-server/internal/websocket/hub.go`/`notify.go`/`handler.go`（二期） | `connections[tenantID][agentID]` 两级桶；删 `agentOnline`；`resolveSendTenant` 为唯一租户解析；所有发送口/在线态查询改 ctx 形态 |
| `user-server/internal/websocket/hub_tenant_buckets_test.go` | 新增：两级桶 + 发送口 fail-closed + 在线态按租户（5 例） |
| `user-server/internal/app/deployment_matrix_contract_test.go` | 新增：2×2 部署形态矩阵契约（四格 × 六条行为） |
| `user-server/internal/router/cross_tenant_scan_test.go` | 新增：A16 逐端点双租户交叉扫描三条腿（匿名参数路由机制闸 32 条 / 未知键闸 16 次 / 真扫 6 条，正对照要求 200 且含 marker） |
| `user-server/internal/storage/fileurl.go` | 新增（A1/A12）：AES-GCM 文件 URL 签名器（`SignStoredPath`/`VerifyFileURL`/`TenantDir`/`TenantDirFromFileURL`），token 明文 `{p,t,e}`，域分离 `hivemtk/file-url/v1`，密钥 `FILE_URL_SECRET`→`USER_JWT_SECRET`→`JWT_SECRET`，`FILE_URL_TTL` 默认不过期 |
| `user-server/internal/storage/fileurl_test.go` | 新增：形态（无租户段）/往返/篡改三态+换密钥/过期/无签名器/整段匹配边界 |
| `user-server/internal/storage/local.go` | `PublicURL` 在 on 模式输出 `/files/<相对路径>?s=<token>`（off 逐字节不变）；`TenantPathPrefix` 保持唯一分域函数 |
| `user-server/internal/router/files_guard.go` | 整文件重写：删除 `tenantPathAllowed`/`tenantFileSegmentRe`，on 模式**只认签名**（失败 404，无"验不过就放行"分支）；文件头写入退役记录与实测复现 |
| `user-server/internal/router/files_guard_tenant_test.go` | 六组契约用例（无签名 404 含 ctx 注入 / 签名 200 / 篡改与过期 404 / 签名自指租户 / off 等价 / 危险扩展名仍 403） |
| `user-server/internal/router/files_guard_setup_test.go` | 新增：走**真装配 `Setup`** 的四条（钉住"ctx 注入能变绿、生产路径走不到"这一失效模式） |
| `user-server/internal/migration/migrations/v3_80_0_file_url_sign_migration.go` | 新增：17 个 URL 列的存量 `/files/tenant_<id>/…` 就地重签（幂等 / off 零动作 / 缺密钥只 WARN / Down 无动作） |
| `user-server/internal/migration/migrations/v3_80_0_file_url_sign_migration_test.go` | 新增：重签形状与可验签 / 幂等 / off 空跑 / 缺密钥保留 / 形状边界 |
| `user-server/internal/migration/migrations/initial_schema.go` | 注册 `v3.80.0` |
| `user-server/internal/pkg/mail/attachment.go` | 新增 `WithAttachmentDir` 接缝（A18）：附件根按录入值改选 |
| `user-server/internal/storage/attachment_source.go` | 新增 `LocalAttachmentResolver()`（唯一构造点）：附件根按签名里的租户改选 |
| `user-server/internal/email/service/email_send.go` | 兜底解析器改走 `storage.LocalAttachmentResolver()` |
| `user-server/internal/pkg/cron/emaillistcron.go`（+`_test.go`） | 群发侧同样改走唯一构造点；静态装配判据同步升级 |
| `user-server/internal/email/service/email_attachments_tenant_test.go` | 新增（A18）：on 解析到租户树 / on 无签名不许回落 / off 逐字节等价 |
| `user-server/internal/service/{session_assignment,customer_session,customer_session_routing,customer_session_takeover,chat_visitor,inbox_ingress_persist}.go` | 13 处 WS 推送调用点补 ctx |
| `user-server/internal/router/files_guard.go` | 新增 `tenantPathAllowed` + 租户段正则；跨租户 404 |
| `user-server/internal/router/files_guard_tenant_test.go` | 新增：/files 租户归属四态 + 边界（4 例） |
| `user-server/internal/middleware/ownership.go` | 缓存键加租户段；失效函数按前缀清；not-found 语义问题留档（P1-3） |
| `user-server/internal/middleware/ownership_cache_tenant_test.go` | 新增：键形 + 跨租户不共享缓存（2 例） |
| `user-server/internal/app/content_guard_wiring.go` | 并发作业留下的 `envInt` 重名编译红（改名为 `envIntWithFallback`） |
| `specs/001-saas-multi-tenant/tasks.md` | T021/T023/T024/T025/T026/T027/T028 状态按实测改写（消除第二份真相） |
| `docs/architecture/SAAS_FULLCHAIN_AUDIT_2026-10-23.md` | 本文件 |

### 7.1 平台端（独立仓 `hivemtk-platform`，提交 `3c4ba07`）

| 文件 | 改动 |
|---|---|
| `platform-server/internal/utils/merchantscope/registry.go` | 新增（A3）：17 张商户分域表冻结清单 + `merchants` 豁免（身份表，带理由）；`NormalizeIdent` 归一化（库名前缀/引号/大小写） |
| `platform-server/internal/utils/merchantscope/scope.go` | 新增：私有 ctxKey + `WithScope`/`FromContext`/`IsMerchantRequest`；空身份也记成"商户面"（否则身份解析失败会静默退化成"看全部"） |
| `platform-server/internal/utils/merchantscope/plugin.go` | 新增：六个 GORM callback（Query/Row/Update/Delete/Create/Raw），`scopeTarget` 是**唯一**的三档判定点；Create 填/拒；裸 SQL 自证闸 |
| `platform-server/internal/utils/merchantscope/plugin_test.go` | 新增：8 例（三档 / AND fail-closed / Update+Delete 0 行 / Create 填与拒外 / 裸 SQL 闸 / 豁免 / DryRun 逐字节等价） |
| `platform-server/internal/utils/merchantscope/registry_test.go` | 新增：3 例（独立 schema 真库内省双向 / ast 扫模型源码双向 / 豁免须有理由） |
| `platform-server/internal/utils/db/db.go` | `Init` 在 `gorm.Open` 之后、`AutoMigrate` 之前 `merchantscope.Register(db)` |
| `platform-server/internal/testutil/testdb.go` | 测试连接同样注册（同款执法模型；不注册时相关用例的绿是假的） |
| `platform-server/internal/middleware/merchant_auth.go` | 两条通过分支各注入一次 scope（与 `c.Set("merchant_key")` 一一对应） |
| `platform-server/internal/router/merchant_isolation_scan_test.go` | 新增：五腿（28 条路由无凭证不成功 / 探针证明注入在真 HTTP 路径可达 / 5 条对抗式 query+body / 运营面 admin 仍看全） |
| `platform-server/internal/service/order_service_merchant_scope_test.go` | 新增：G1 收口 3 例（故意传空 key 走"空 = 全部"分支，断言只看自己；运营面同一调用仍看全部） |
| `platform-server/internal/service/order_service.go` / `internal/repository/llm_billing.go` | G1 两处站点补注释指向框架收口（**这里不加条件**才是对的） |
| `platform-server/scripts/check-merchant-scope.py` | 新增：三道闸（装配面 / 身份来源 / 接线锁），判据跑在去注释后的源码上 |
| `platform-server/scripts/check-merchant-scope.test.sh` | 新增：六条腿证明上面那道闸**能红**（注入违规⇒必须红⇒还原后 md5 全等且回绿） |
| `platform-server/Makefile` + `.github/workflows/platform-ci.yml` | 新增 `make merchant-scope-gate`（并挂到 `lint` 前置）与 CI 步骤 |

### 7.2 A19 / A20 轮（平台端仓 `23abf55`；租户端见下表）

| 文件 | 改动 |
|---|---|
| `user-server/internal/platform/client.go` | 新增 `GetMerchantSecret()`（**唯一事实源**：文件 `.merchant_api_secret` → env `MERCHANT_API_SECRET` → `config.PlatformCfg.Secret`，与原 `sign` 行为逐字一致，只是收成一处）；`sign()` 改用它。`SetMerchantSecret` 保持**实例**状态（不做包级全局——实测会污染 `TestSignSecretPrecedence`，且"谁覆盖了它"不可追） |
| `user-server/internal/platform/llm_proxy_client.go` | 新增 `LLMProxySignature`（与平台侧逐字同口径）/`newLLMProxyNonce`/`signLLMProxyRequest`；发 `X-Merchant-Key`+`X-Timestamp`+`X-Nonce`+`X-Signature`，**保留** `Authorization: Bearer`（新客户端可对旧平台工作 ⇒ 升级顺序自由）；无密钥不阻断、只告警一次 |
| `user-server/internal/platform/llm_proxy_sign_test.go` | 新增（4 例）：跨仓契约向量 `3ff32cbd…a676` / 签名头可验（且签名覆盖含 query 的 URI）/ 文件档优先于 env / 无密钥照发不阻断 |
| `platform-server/internal/middleware/merchant_llm_auth.go` | 新增（A19）：`MerchantLLMProxyAuth()` / `LLMProxyRequireSignature()` / `LLMProxySignature()`；商户行只查一次；失败分支对"key 存在/不存在"同码同文案（消状态码预言机）；兼容窗口仍校验商户存在且 active 并留审计日志；nonce 去重键带 `llm:` 前缀 |
| `platform-server/internal/middleware/merchant_llm_auth_test.go` | 新增（11 例 + 契约向量）：无签名 / 只带 Bearer / 篡改 body / 篡改 query / 过期 ts（±301s）/ nonce 重放 / 错 secret / 未知商户 / 停用商户 / 兼容窗口 / 同秒同 body 不同 nonce 两次都放行；每条负例配正对照（判信封业务码） |
| `platform-server/internal/controller/llm_proxy_controller.go` | 删 `merchantKeyFromBearer` 及其 Bearer 回落；身份只读 `ctx.GetString("merchant_key")`，取不到即报"身份闸没挂" |
| `platform-server/internal/router/router.go` | `/llm-api/v1` 挂 `MerchantLLMProxyAuth()`；注释块补"成立条件第 4 条：请求带商户签名" |
| `platform-server/internal/router/merchant_isolation_scan_test.go` | 新增腿五（A19）：走 `router.Setup` 真引擎，无凭证 / 只带 Bearer / 错 secret 的拒绝**必须出自闸**（按文案判），并配签名正确的正对照 |
| `platform-server/cmd/api/main.go` | 启动期：`PLATFORM_LLM_PROXY_REQUIRE_SIGNATURE=false` 时响亮告警（口径同 A13：不静默降级） |
| `platform-server/scripts/check-llm-proxy-auth.test.sh` | 新增：十条腿逐条改坏中间件 ⇒ 对应用例必须红 ⇒ md5 还原回绿再跑必须绿（一道从未红过的判据与没有判据是同一件事） |
| `platform-server/scripts/check-merchant-scope.py` | `MERCHANT_GROUP` 常量 → `GATED_GROUPS` 清单（`/merchant-api`→`MerchantAuth()`、`/llm-api/v1`→`MerchantLLMProxyAuth()`）；新增闸 2b（llm 面 handler 不得自读 `Authorization`/`X-Merchant-Key`/`merchant_key`）；闸 3b 改为 `SCOPE_INJECTORS` 白名单 + 逐文件 `c.Set ↔ WithScope` 一一对应 + 注入点必须含 `hmac.New(` + 白名单外禁写身份 |
| `platform-server/scripts/check-merchant-scope.test.sh` | 六条腿 → 十条腿（新增：摘 `/llm-api/v1` 的闸 / llm handler 自读 Authorization / 删 `merchant_llm_auth.go` 的 `WithScope` / 去掉注入点里的 `hmac.New`） |
| `platform-server/Makefile` + `.github/workflows/platform-ci.yml` | 新增 `make llm-proxy-auth-gate`（用例 + 自证）与 CI 步骤（`platform-server-test` 第 6 步） |
| `platform-server/internal/service/llm_proxy_test.go` | A20：`setupProxy` 里 `t.Setenv("LLM_ENC_KEY", …)` 自带密钥（不用 Makefile 注占位——那只覆盖 `make test`，CI 直接跑 `go test ./...` 仍会红），判据 `env -u LLM_ENC_KEY go test -p 1 ./...` 全绿 |

### 7.3 A4 轮（契约版本握手；平台端独立仓 + 本仓同批）

| 文件 | 改动 |
|---|---|
| `platform-server/internal/contract/contract.go` | 新增：契约版本**唯一事实源** —— `HeaderName="X-Contract-Version"` / `Version=1` / `MinVersionEnv="PLATFORM_CONTRACT_MIN_VERSION"` / `MinSupported()`（默认 = `Version` 即 fail-closed，非法值回落 `Version`）/ `Parse()`（缺失/空/非法/负数一律当 0，**不能**读成最新版，否则任何人可跳过闸） |
| `platform-server/internal/contract/contract_test.go` | 新增（3 例）：版本字面量 pin / `Parse` 六档 / `MinSupported` 默认从严 + 显式开窗 + 非法值回落 |
| `platform-server/internal/middleware/contract_handshake.go` | 新增：`ContractHandshake()` —— 两个方向 426、文案分方向、`contractWarned` 按版本只喊一次、窗口期未声明放行**留痕**、ctx 写 `contract_version` |
| `platform-server/internal/middleware/contract_handshake_test.go` | 新增（6 例，每条负例配正对照）：未声明 426 / 当前版本放行且 ctx 有版本号 / 高于平台 426 且指向"升平台" / 非法头 426 / 窗口期放行且留痕且不被伪装成最新版 / 非法开关回落严格档 |
| `platform-server/internal/utils/response/response.go` | 新增 `UpgradeRequired`（**真 HTTP 426** + 信封 426）。不用 `Error(c,426,…)` —— 那是 HTTP 200 + 信封码，而"一眼认出"的主体是读不懂新信封的旧客户端 |
| `platform-server/internal/router/router.go` | `merchantAPI`/`llmAPI`/`platformPublic` 三组各挂 `ContractHandshake()`（写在 `MerchantAuth`/`MerchantLLMProxyAuth` 之前）+ 一段注释说明**两个豁免前缀**（`/platform/**`、`/api/auth/login`）的理由：它们各有两种消费者，加要求会打挂平台自身前端 |
| `platform-server/cmd/api/main.go` | 启动期：窗口开着就响亮告警（含"失败会伪装成业务错误"），默认档打一条 info |
| `platform-server/internal/router/merchant_isolation_scan_test.go` | 腿一/腿五及 `merchantCall` 补 `contract.HeaderName`（否则 426 替身份闸挡枪、断言假绿）；新增腿六 `TestContractHandshakeEnforcedOnRealEngine`（受闸三前缀无头恒 426 + 声明的正对照不得 426 + 豁免前缀不得被挂闸 + "一条豁免都没扫到"也算失败） |
| `platform-server/tests/api/api_test.go` | `TestRequestBodyTooLarge` 改用 `http.NewRequest` + `X-Contract-Version: 1`（否则 426 让 `code>=400` 假绿） |
| `platform-server/scripts/check-merchant-scope.py` | 新增 `CONTRACT_GUARDED`/`CONTRACT_EXEMPT` + `check_contract_guard()` 闸 4：组唯一 / 先挂闸再注册路由 / 不得绕出组 / 豁免侧反向防腐 |
| `platform-server/scripts/check-merchant-scope.test.sh` | 十条腿 → **十二条腿**（⑪ 摘 `/merchant-api` 的 `ContractHandshake`、⑫ 给豁免的 `/platform` 挂闸）；①⑦ 期望文案改为 `没有 MerchantAuth()`/`没有 MerchantLLMProxyAuth()`（摘掉唯一 `Use` 后组上已无第二条 `Use`） |
| `user-server/internal/pkg/contract/contract.go` | 新增：租户端版本唯一事实源（`HeaderName`/`Version=1`/`HeaderValue()`）+ "平台要求本机升级"状态（`RecordUpgradeRequired`/`ClearUpgradeRequired`/`Status`/`UpgradeRequired()`）。用互斥量而非 atomic：存的是 struct，原子换指针会让读侧拿到半新半旧的一行字 |
| `user-server/internal/platform/contract_header.go` | 新增：`newPlatformRequest`（**出站唯一构造入口**，无条件打版本头，不留 `if guarded` 开关）+ `contractGuardedPrefixes` 对账单 + `noteContractVerdict`（**唯一判定点**：426 记录 / 200 清除 / 其它不动，且只有受闸路径的 200 有清除资格） |
| `user-server/internal/platform/client.go` | 登录 / `doRetry` / `CheckConnection` / `ReportInstall` / `ReportHeartbeat` 五处构造改走 `newPlatformRequest`；`doRetry` 与两个上报的非 200 分支补 `noteContractVerdict` |
| `user-server/internal/platform/llm_proxy_client.go` | 构造改走 `newPlatformRequest`；收到响应后补 `noteContractVerdict`（这条通道最先被平台升级打到，失败表现是"模型调用全报错"） |
| `user-server/internal/platform/contributor_client.go` | 同款改造（该前缀未挂闸，走同一入口只是不留第二份清单） |
| `user-server/internal/pkg/shared/service/system_stats.go` | `SystemInfo` 加 `PlatformContract *contract.Status json:"platform_contract,omitempty"`，仅在被拒时填充 ⇒ `off` 模式响应体逐字节等价；不进 `publicSystemInfo` 白名单 |
| `user-server/internal/pkg/contract/contract_test.go` | 新增（4 例）：版本 pin / 记录→可见→清除的状态机 / 重复上报不堆文案但累加次数 / 原因被最新一条覆盖（两个方向的文案动作不同） |
| `user-server/internal/platform/contract_header_test.go` | 新增（5 例）：五个真实入口逐个断言带版本头 / 426 记录且原因取自平台信封 + 200 清除的正对照 / 豁免路径 200 不得清告警 + 受闸路径 200 必须清的正对照 / 受闸前缀清单对账 / 心跳 200 也必须能清（它是“只在主通道清”最容易漏的一处，漏了之后告警永不再更新） |
| `user-server/internal/pkg/shared/service/system_stats_test.go` | 加一例：没被拒过时 JSON 里**连键都不能有**（`omitempty` + 指针，去 `omitempty` 与"无脑填空结构"两种写法实测都会红） |

### 7.4 A10 轮（跨端线上形状清单；平台端独立仓 + 本仓同批）

| 文件 | 改动 |
|---|---|
| `platform-server/internal/wirecontract/wirecontract.go` | 新增：`Describe`/`DescribeWithRenames` 把活体 struct 反射成形状清单（`Manifest`/`Struct`/`Field`）；wire 口径只到 string/integer/number/boolean/array/object/any（`time.Time`→string、`json.RawMessage`/`[]byte`→any、指针记 `Nullable`）；内嵌 struct 就地摊平（`encoding/json` 就是这么做的）；`renames` 让租户侧把嵌套 Go 名改写成规范名 |
| `platform-server/internal/wirecontract/manifest_test.go` | 新增（3 例）：清单与代码一致（改了 DTO 不重生成即红）/ 四条通道根类型齐全（`platformRoots()` 11 项）/ 无未命名（大驼峰）线上字段 |
| `platform-server/docs/contracts/cross-repo-wire.json` | 新增：权威侧清单（11 类型，生成物） |
| `platform-server/docs/contracts/README.md` | 新增：角色（权威方）/ 反射而非手写 / 三条判据 / **这里不跑跨仓比对**的边界 |
| `platform-server/Makefile` | 新增 `make wire-contract`（只跑本仓反射用例） |
| `platform-server/docs/dev/CONVENTIONS.md` | 新增 §6.2.5：契约闸管"不兼容"，管不了"没声明的漂移"；清单只记 wire 口径；新增跨端结构两仓各登记一条 |
| `user-server/internal/pkg/wirecontract/wirecontract.go` | 新增：同款反射器（租户视角注释）+ `canonical()` 改名 |
| `user-server/internal/pkg/wirecontract/manifest_test.go` | 新增（4 例）：清单与代码一致 / 四条通道根类型齐全（键用**平台类型名**）/ **清单里不许出现自家 Go 名的键**（`tenantRenames` 漏登记在单仓就红）/ 无未命名线上字段 |
| `user-server/docs/contracts/cross-repo-wire.json` | 新增：租户侧清单（11 类型，键=平台名） |
| `user-server/docs/contracts/README.md` | 新增：11 组类型对照表 / 三条判据 / **合法子集逐条理由** / 两处"看起来不一致但不该改"（`SyncedAt`、`json.RawMessage`）/ 门禁边界 |
| `user-server/scripts/check-wire-contract.py` | 新增：跨仓方向性比对（类型集相等 + 租户 ⊆ 平台 + 平台 required ⊆ 租户）；退出码 0/1/2 三档；`--allow-missing-platform` 单仓降级且**响亮打印**；`ONE_SIDED` 是唯一放行口子且要求写理由 |
| `Makefile` | 新增 `make wire-contract`（反射用例 + 跨仓比对） |
| `.github/workflows/wire-contract.yml` | 新增：单仓 CI（Go 反射用例 + `--allow-missing-platform` 降级）；`paths:` 含 `user-server/internal/platform/**` —— 反射的输入就是那些 struct |

---

## 8. 双部署起服实测复盘（SaaS 8411 / 独立 8412 + 真平台 8205）

起服方式：同一份代码、同一个二进制，只换 `TENANT_MODE` 与 `PLATFORM_ENABLED`；平台端用
`https://manageapi.xapptool.cn` 的本地等价物（8205，参数中心 `platform.api_host` 优先于 env，
实测生效地址就是 8205 —— 这条优先级本身也在这轮被验证了一次）。

| 观测点 | SaaS（8411） | 独立（8412） | 结论 |
|---|---|---|---|
| `init-status` | `INITIALIZED` | `INITIALIZED` | 两形态都能完成初始化 |
| `edition` | `saas` | `opensource` | 轴一 |
| `POST /api/tenant/register` | 准入（走到参数校验 4101） | `4104 当前为离线部署模式，注册未开放` | 轴二正交 |
| 平台商户注册 | `POST /merchant-api/merchant/register` → 200，拿到 key/secret | 不上报 | 平台链只在 SaaS 侧 |
| 心跳 | `上报心跳成功`（平台侧 `/api/platform/heartbeat` 200） | 不打点 | 同上 |
| 模块下发 | `modules/sync` 200，8 个基础模块 `active` + `trial` 订阅段 | 空数据但不报错 | 平台关态 fail-empty，不是 fail-crash |

### 8.1 P0：`message_hub` 唯一索引的形状**从未真正升级过**

现场读数（升级前）：`CREATE UNIQUE INDEX uni_message_hub_platform_msg_conv ON message_hub (tenant_id, platform, msg_id, conversation_id)` —— 缺 `direction`，
即 D-2 方案③想要的"跨方向同文本可共存"在真实库上一直没落地。

根因在判据而不是 DDL：`indexShapeMatches` 拿库里的列清单**原样**跟 `want` 比。SaaS 归一化
（`tenancyidx.Normalize`）会把租户表上的唯一索引原位复合为 `(tenant_id, …)`，于是
`exact`（四列）永不命中、`old_shape`（三列）也永不命中 ⇒ 恒落 `other` 分支 ⇒ 那里只有
`CREATE ... IF NOT EXISTS`，名字已在场 ⇒ **空转**。三层都"没做错"，合起来就是"库里还是旧形状，
没人管"。修法是把租户前缀在比较前剥掉（单点），并让升级 DDL 显式带上 `tenant_id`，
避免重建后又被 Normalize 二次复合。

**牙齿**：重启后同一实例日志从 `已就绪`（幂等）之前的 `已按 D-2 升级为 (tenant_id, platform, msg_id, conversation_id, direction)`，
库内 `indexdef` 实测由四列变五列。`message_hub_unique_batchm5_test.go` 里那句逐字镜像的
Info 文案同步更新 —— 它是"钩子走到了 ready 分支"的唯一证据，不跟着改就会静默失联。

### 8.2 P0：迁移的 `Down` 会删掉"不是自己建的"对象

两处，同一类错：

- `v3.79.0`（tenant 前导索引）按**名字** `DROP INDEX`。它 `Up` 时只给"确实缺"的表补索引，
  而那些本来就有前导索引的表，其索引名与本迁移的命名规则**同形**（`idx_<表>_tenant_id`）⇒
  Down 把不属于它的在用索引删掉，降级后的实例长期 seq scan。实测被
  `a_full_chain_migration_test.go` 抓出 `livecode_click_log` / `qr_code_click_log` 两枚。
- `v3.26.0`（触达域两张表）按名字 `DROP TABLE`，而这两张表在**每个真实部署**里都由
  AutoMigrate 持有（model 挂在 `RegisterExtraModels`）。一次 `POST /api/migration/rollback`
  就是：现网正在写的合规审计日志 + 延迟出站队列（丢一行 = 一条客户回复永不再发）一起消失，
  且 AutoMigrate 只在启动时跑 ⇒ 回滚后的进程**永不自动补回**。

均改为 `declineIndexDrop` / `declineTableDrop`（`down_drop_policy.go` 的既有口径：多留一个对象
不破坏任何功能，删掉一个在用对象则不可恢复），用例从"Down 后应被删掉"翻成"降级不销毁在用对象，
且 Down 之后再 Up 仍幂等"——红因里写清两个世界，避免后来人看见红就删断言。

**仍待收口（P2）**：`llm_routing_logs` / `rag_hybrid` / `v3_22_3` / `v3_33_0` 四处的 Down 仍直接
`DROP TABLE`，需逐个判"表是 AutoMigrate 持有还是本迁移独占"后再决定；`v3_16/v3_17/v3_19/unmultitenant`
是"drop 迁移"，删表是它的本职，不动。

### 8.3 P1：`v3.26.0` 建表 DDL 缺 model 的 5 列（纯链恢复的库读不了）

E1 策略判定给 `reach_compliance_log` 加了 `decision / reason / region / industry / policy_scope`，
但建表 DDL 没跟 —— 文件头注释自己写着"模型加字段而这里漏了，用例会当场红"，这轮就红了。
纯迁移链恢复出来的库第一次读写即 `column does not exist`（现网靠 AutoMigrate 掩盖着）。
补：新建库走 `CREATE TABLE` 带列，存量库走 `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`（可重入），
并按 model tag 补齐四条索引。**注意**这轮只补 DDL 同步，不改 AutoMigrate 仍是建表事实源这件事。

### 8.4 P1：平台配置按 CWD 解析 —— 起服目录决定平台链是否活着

从仓库根起 `user-server` 时，`config/platform.yaml` 读不到 ⇒ `PlatformCfg=nil` ⇒
商户注册、心跳、资产市场、模块下发**全部静默降级**，日志只有一行 error（`平台配置加载失败…`）。
`PLATFORM_ENABLED` 默认 false，所以这条路径平时不显形，一旦真去连平台就必然踩到。

`config/` 是随二进制一起分发的目录，部署时 CWD 与二进制目录不同是常态（systemd
`WorkingDirectory`、桌面端双击、容器换 WORKDIR）。故：默认相对路径读不到时按**二进制所在目录**
再找一次；`PLATFORM_CONFIG_PATH` 显式指定或调用方传了具体路径时**不回落**——显式路径找不到就
该报错，"猜一份配置来读"比直接报错更难查。两例回归（回落生效 / 边界不回落）。

### 8.5 P1（环境面）：平台端 SQL 迁移 032–035 是**手工**的，漏一条就在运行时炸

新商户首次 `modules/sync` 返回 `平台业务码 400 … SQLSTATE 42P10 there is no unique or
exclusion constraint matching the ON CONFLICT specification`。根因：平台 `tenant_modules`
的唯一索引还是旧的 `(merchant_key, module_key)`，而代码
`UpsertTenantModule/UpsertManyTenantModules` 走 `ON CONFLICT (merchant_key, tenant_code, module_key)`。
`hivemtk-platform/migrations/034_tenant_modules_tenant_code.sql` 就是干这事的，但**没被应用**，
且仓库里没有任何自动化引用这几份 SQL（Makefile/scripts/deploy 全无）。

**为什么此前一直是假绿**：`EnsureBaseModules` 只对"还没激活的基础模块"造行，存量商户
`rows` 为空 ⇒ 提前 return ⇒ 那条 SQL 根本不发。只有**新注册商户**第一次 sync 才会带上行，
于是这个洞只在"拉新"时暴露——正是最不该出错的时刻。手工应用 034 后 `modules/sync` 200，
基础模块 8 个 `active`、订阅段 `trial`。

**建议**：平台端起服时把 `migrations/*.sql` 纳入版本化执行（或至少启动自检"模型 tag 声明的
唯一索引 vs 库内实际"，与该用户端 `indexShapeMatches` 同源），避免"代码在跑新形状、库还是旧形状"。
本轮只做环境修复 + 留档，不改平台端执行机制（跨仓，且要连带部署脚本）。

### 8.6 本轮改动清单

| 文件 | 改动 |
|---|---|
| `user-server/internal/pkg/db/migrate.go` | `indexShapeMatches` 比较前剥租户前缀；升级/兜底 DDL 带 `tenant_id`；日志文案同步 |
| `user-server/internal/pkg/db/message_hub_unique_batchm5_test.go` | 逐字镜像的 ready 文案更新 + 注释说明"这是唯一证据" |
| `user-server/internal/migration/migrations/v3_79_0_tenant_leading_index_migration.go` | `Down` 改 `declineIndexDrop` |
| `user-server/internal/migration/migrations/v3_79_0_tenant_leading_index_migration_test.go` | Down 契约翻转为"不销毁在用对象"+ Down→Up 幂等 |
| `user-server/internal/migration/migrations/v3_26_0_reach_tables_migration.go` | 补 E1 五列 + 四条索引（新建 + 存量 ADD COLUMN IF NOT EXISTS）；`Down` 改 `declineTableDrop` |
| `user-server/internal/migration/migrations/v3_26_0_reach_tables_migration_test.go` | Down 契约翻转；Up/Down 幂等 |
| `user-server/internal/config/platform.go` | 默认路径读不到时按二进制目录回落（显式路径不回落） |
| `user-server/internal/config/platform_test.go` | 新增 2 例：回落生效 / 显式路径不回落 |
| `hivemtk-platform/migrations/034_tenant_modules_tenant_code.sql` | 环境面：应用于本地 `platform_db`（未改文件） |
| `model-list.md` | #206 `kb_item_bindings` 补审行 |

## 9. 任务一续审：服务层「自持 gorm」清零 + L-4 归档装配 + 绑定写事务下沉（2026-10-24）

### 9.1 三条全仓扫描（用来定位，不代替逐实体审）

| 扫描 | 命令口径 | 结果 |
|---|---|---|
| 服务层直发查询 | `internal/service/*.go` 中出现 `db.WithContext/Raw/Model/Create/...` | 收口前 3 个文件：`tenant_starter_kit.go`(7)、`memory_decay.go`(4)、`agent_kb_binding.go`(2)；收口后 **0** |
| 服务/控制器裸 SQL（门 2） | `.Raw(`/`.Exec(` | 0（历史已清） |
| 控制器持 gorm（门 1） | `internal/controller/*.go` 导入 `gorm.io/gorm` | 0 |

`agent_kb_binding.go` 那 2 处不是查询，而是 `db.WithContext(ctx).Transaction(...)`：事务边界。
但事务边界在 service 的直接后果是「同一条业务规则写了两份」——有 db 走事务、无 db 退化裸写，
退化那条**静默丢掉原子性**（先删后建中途失败 ⇒ 绑定凭空消失）。所以按缺陷处理，不是纯风格。

### 9.2 三处改动

**a) 开户赠送（`tenant_starter_kit.go`）**：三张表（`system_config` / `ai_agents` / `asset_bundles`）
的读写进新增 `repository/starter_kit.go`（`CountSiteConfig`/`CreateSiteConfig`/`CountAgentByCode`/
`FindAgentByCode`/`CreateAgent`/`CountBundleByAssetID`/`CreateBundle`）。单开一个仓储而不是往三个
既有仓储各加方法：这是**一个流程**（开户赠送），拆三处会让"开户时到底碰了哪几张表"读不出来；
且 `SystemConfigRepository`/`AssetBundleRepository` 是 interface，各加两三个方法要连带补所有 fake。
归属靠行上显式 `tenant_id`（注册 ctx 无租户绑定，走 `tenant.WithSystemScope` 豁免通道，该通道下
callback 不填列 ⇒ 每个方法都把 tenantID 作显式条件）。

**b) L-4 记忆衰减归档（`memory_decay.go`）**：原状是**僵尸**——实现完整、9 个用例齐全，
但 `StartMemoryDecayLoop`/`RunMemoryDecayOnce` 在全仓**零生产调用方**，兄弟作业
（RFM/Churn/旅程沉睡/RAG 评测/企微配额）都在 `cmd/api/main.go` 装配，唯独它没有。
后果不是"功能缺失"而是"数据只进不出"：`memory_items` 与 `customer_long_term_memory` 的
`invalid_at` 永远不写，L2 事实无界增长。修法：

- 新增 `repository/memory_decay.go`（读 → 判 → 软失效，租户由 ctx 承载）；
- `RunMemoryDecayJob` 降为**单租户单元**（收 `*repository.MemoryDecayRepository`），
  新增 `RunMemoryDecayAllTenants` 经 `tenant.Fanout` 逐租户；
- `cmd/api/main.go` 装配 `StartMemoryDecayLoop(ctx, db, time.Hour)`。

为什么不一把梭全表扫：两张都是租户表，on 模式下隔离 callback 只认 ctx 里的租户，
用 `context.Background()` 会 fail-closed（一行读不到），而"读不到"与"没数据可归档"在统计上
完全一样（`scanned=0`）——这正是它漏装这么久没人发现的原因；用 `SystemScope` 强扫则是拿
后台任务当万能钥匙。`Fanout` 两头都不碰：off 给默认租户 ctx、on 逐租户给 ctx、单租户失败不阻断
其余租户、枚举器缺失 fail-closed 报错（该枚举器在 `db.AutoMigrate` 里注册，生产有装配）。

**c) 绑定写事务下沉（`agent_kb_binding.go` + `repository/agent_kb_binding.go`）**：新增
`BindOverwrite`/`ReplaceByAgent`/`BatchBindOverwrite`，各自单一实现且常驻事务；service 只剩
入参校验与错误文案，不再持 `*gorm.DB`，`txBeginner` 与非事务退化路径一并删除。
`kb_type` 一律读知识库真实类型（写死 `faq` 会让按类型过滤整批漏掉 rag/sop 库——本轮一并去掉
"kbRepo 为 nil 时默认 faq"这条兜底，它只会静默写错列）。
错误用带 id 的自定义类型 `KnowledgeBaseNotFoundError`（`Is` 指向 `ErrKnowledgeBaseNotFound`）：
`fmt.Errorf("知识库 %d 不存在: %w", id, Err...)` 拼出来的文案是"知识库 7 不存在: 知识库不存在"，
同一句话说两遍；只写 id 不 wrap，调用方就再也无法用 `errors.Is` 分支。

### 9.3 新增回归用例（12 例）

| 文件 | 用例数 | 锁住的性质 |
|---|---|---|
| `internal/repository/memory_decay_tenant_test.go` | 3 | 读侧只看得见本租户；软失效 UPDATE 混入他租户 id 也改不动那一行；无租户 ctx 报错（不退化成全表） |
| `internal/service/memory_decay_fanout_test.go` | 3 | 生产同款 `context.Background()` 入口下两租户各被归档且不串；枚举器缺失必须报错而非静默 `archived=0`；干跑不落 `invalid_at` |
| `internal/repository/agent_kb_binding_tx_test.go` | 6 | 坏 kb 整体回滚（既有绑定原样保留）；空列表清空；覆盖语义与优先级；kb_type 取真实类型；批量全或无 |

### 9.4 测试库踩到的两个坑（留档，避免下次再查）

1. `customer_long_term_memory.embedding` 是 `vector(1024)`，GORM 会把零值 `""` 写进去 ⇒
   `invalid input syntax for type vector: ""`。预置数据要 `Omit("Embedding")` 让列为 NULL
   （生产写入通路 `memory_system.go:591` 一定带向量，所以这不是线上缺陷）。
2. 复用带主键的 dest 变量再 `First(&dest, 新id)`，GORM 会把旧主键当附加条件 ⇒ 表现为
   `record not found`（而不是读错行）。物理回查每次用新变量。

### 9.5 未决（需产品/跨仓决策，不在审计范围内单方面拍定）

- `sop_state_memories`：写入口 `L3SaveSOPState → SaveSOPState` **只有测试调用方**，
  而 `BuildPrompt` 在生产照读该表 ⇒ prompt 里「L3 SOP 状态」段恒为空。是设计未落地
  （model 注释 D-6 已记录，并据此判定 NOT NULL 迁移无存量风险）。二选一：接三处 SOP 推进点
  （`sop.go` 390/490、`sop_dispatcher.go` 578）持久化，或删表并删掉 prompt 中的 L3 段。
- `hivemtk-platform/migrations/*.sql` 未纳入平台端起服执行（见 §8.5 建议，跨仓 + 部署脚本）。
