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
| P0-2 | `/files/*` 静态服务**不看路径里的租户段** ⇒ 任何拿到 URL 的已登录租户可读别人租户的文件（tasks.md T011 声称"校验租户归属"，代码里那半边从未实现） | 高（数据泄漏） | **已修（已绑定租户的请求）+ 4 例回归**；无凭证外链的残留缺口见 P1-2 |
| P1-1 | 归属校验缓存的键是 `table:id`、**无租户段** ⇒ 一个租户的归属判定命中另一个租户，且其 record-not-found 语义是 fail-open | 中高 | **已修 + 2 例回归** |
| P1-2 | 无凭证公开外链（访客 `<img>`、分享页）拿不到租户归属 ⇒ 只能靠 UUID 不可枚举 | 中 | 待批准（签名 URL） |
| P1-3 | 通用归属中间件 `RequireOwner*` 的 fail-open 分支（`checker==nil ⇒ 放行`、`db==nil ⇒ (0,nil)`）+ not-found 混成 500 | 中（潜在） | **已修**（语义收口 + 6 例回归，见 §3） |
| P1-4 | 平台端（运营端）的商户隔离**全靠手写 `Where("merchant_key = ?")`**，无框架约束、无门禁、失败模式是静默跨商户 | 中高（云端侧） | 待批准（设计已给） |
| P1-5 | 云/本地之间**没有版本/契约握手**：老实例连新平台只能靠运行时 500 发现字段不匹配 | 中（运维） | 待批准 |
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

**修法**：`tenantPathAllowed(ctx, rel)`
1. off 模式 ⇒ 恒放行（零回归）；
2. 首段不是 `tenant_{id}` ⇒ 放行（存量根域文件、系统级上传、附件子目录）；
3. 首段是 `tenant_{id}`：请求 ctx **已绑定租户** ⇒ 必须同租户，否则 **404**（不用 403，避免变成存在性预言机）；ctx **无绑定** ⇒ 放行（访客 `<img>` / 分享页，无凭证外链）。

**回归**：`internal/router/files_guard_tenant_test.go`（4 例，含 `tenant_51` 不能被 `tenant_5` 前缀骗过的边界）。

**残留缺口（P1-2）**：第 3 条第二种情形没收口——无凭证公开外链拿不到租户归属。要真收口只能走**签名 URL**（短时 token / 预签名），属于独立一项，见 P1-2。

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

### P1-4（待批准）平台端商户隔离无框架约束

**证据（运营端 `hivemtk-platform/platform-server`）**：
- `internal/middleware/merchant_auth.go:121` 只做 `c.Set("merchant_id", m.ID)`，**全仓 `merchant_id` 出现 1 次**——没有任何仓储层/回调层拿它过滤；
- 实际隔离靠每个 repository 方法手写：`internal/repository/order.go:190/216` `Where("merchant_key = ?", merchantKey)`；
- 单点校验是手写的：`internal/controller/order_controller.go:238` `d.Order.MerchantKey != mk → 403`（**写得对，但属于"每处都得记得写"的形状**）。

**判定**：这是两端最本质的**能力不对称**——用户端把隔离吃进框架（漏了会 fail-closed 报错），运营端把它留在业务代码里（漏了**静默跨商户**）。新增一个 repository 方法忘了 `Where`，没有任何门禁会红。

**建议**：按用户端已验证的形态复制最小集，不动业务代码：
1. `merchant_keys`/订单/资产/账单/订阅等商户域表统一加 `merchant_key` 列（多数已有）；
2. 注册一个 GORM callback，按 ctx 里的 `merchant_id`（由 `merchant_auth` 注入 ctx）自动注入 `WHERE merchant_key = ?`，`merchant-api` 组 fail-closed；
3. 加一条仿 `check-gorm-only.sh` 的门禁：`merchant-api` 组内的 repository 方法必须走这个通道。

代价：中（约 15 张表 + 一条 callback + 一条门禁）；收益：把运营端从"约定隔离"拉到"结构隔离"。

---

### P1-5（待批准）云 / 本地之间没有契约握手

**证据**：`internal/platform/client.go:491,545` 只在 install/heartbeat 载荷里**上报** `version`；平台侧没有任何 `min_version` / 兼容判定（`grep -i "compat|handshake|min_version"` 无命中）。开户登记字段不全时会 500（历史实测记录在 `DEPLOYMENT_MODES_EVALUATION_2026-10.md` §9.2）。

**判定**：独立部署的实例生命周期远长于云端。老实例连新平台时，字段缺失/新增只能靠**运行时报错**发现，且失败常被 `logger.Warn` 吞掉（`internal/platform/sync.go:96` 明确写"可忽略"）。这正好是"一套逻辑跑两种部署"最容易出事的地方。

**建议**：平台在 `/merchant-api/*` 加一道 `X-Client-Version` 校验（低于 `min_supported` ⇒ 明确 426 + 升级指引），用户端把该响应呈现为管理端可见的告警，而不是只进日志。

---

### P2（列入清单，本轮不改）

| # | 问题 | 证据 | 为什么不这轮改 |
|---|---|---|---|
| **P2-1**（✅ 已修） | 限流/配额键与桶**无租户维度** | 真库/活体代码：`RateLimitMiddleware` 与 `PathQuotaMiddleware` 都挂在 JWT **之前**，键只有 `c.ClientIP()`（外加 apikey 变体）。**不需要**改挂载顺序：限流中间件自行验签解析一次令牌取 `claims.TenantID`（不写 ctx，避免给未鉴权请求塞租户绑定）。on 模式键 = `t<tenant>:<clientKey>`，off 模式前缀恒空串（键形逐字节不变，且不付令牌解析成本）；无令牌/坏令牌/历史令牌回落 IP 键 —— 这一档是**显式登记的语义**（未登录流量本无租户），后果仍只剩"未登录噪声邻居"。监控读数口 `GetRateLimitStatus` 走同一键函数，读写不会指向两个桶 |
| **P2-2**（✅ 已修） | 11 张租户表的索引族里没有以 `tenant_id` 打头的列 ⇒ 隔离条件退化为过滤而非索引查找 | 真库查询（判据**只认前导列**，`(id, tenant_id)` 不算）：`bridge_metrics`、`email_lists`、`email_smtps`、`fts_test`、`intent_logs`、`livecode_click_log`、`llm_routing_audit`、`llm_usage_records`、`obs_configs`、`qr_code_click_log`、`script_recommends` | 补 `v3.79.0` 迁移（幂等、Up 后自复核、Down 只删自建索引、清单写死不扫全库）；**真库实测缺口 11 → 0**。漏因值得记：这些表走手写 `CREATE TABLE` 而非 AutoMigrate，模型的 `index` tag 对它们**不生效**，"模型说建了索引、库里没有"长期共存 |
| **P2-3**（✅ 已修） | 知识库导入文件**未按租户分域**（绕过 `LocalDriver`，直接 `uploads/knowledge-base|knowledge|knowledge-text|knowledge-url/…`） | `internal/aiagent/knowledge/service/knowledge_base_import.go:75`、`knowledge_service_import.go:48/113/177`（共 4 条通路） | 分域原先只做在 `LocalDriver.UploadReader` 里 ⇒"上传口分域了、知识库导入口没分域"，同一个 uploads 根下两种布局并存。修法：`storage.TenantPathPrefix(ctx)` 导出为**唯一**前缀函数（驱动内部同款改为调它），4 条通路各自 `filepath.Join(base, prefix, …)`。off 模式前缀为空串 ⇒ 路径逐字节不变；存量根域文件在 on 模式下仍可读（`/files` 守卫只对 `tenant_*` 首段做归属校验） |
| **P2-4**（✅ 已修） | 后台任务扇出**已有门禁**，但扫描面只覆盖 `internal/pkg/cron/` | `scripts/check-tenant-ctx-writes.py` 新增 `SCHEDULER_DIRS`（GEO/browser_automation），任务键改用 `@<路径>::<所在函数>`，登记表补 2 行；注册点 7 → **9**，自检 9 枚控制全 PASS | 不改两个调度器自己的 `AddTask` 形态（那是不必要的小重构）：**判据只需"注册点必须做一次归属决策"**，键取法随形态变即可 |
| **P2-5**（✅ 已修） | 事件总线 `TenantID` 由**发布点自行填充**（4 处各写一遍） | 新增 `event.PublishCtx`/`PublishEventCtx` + `StampTenantFromCtx`（反射盖章，非 0 不覆盖）；3 处迁移；新增门禁 `scripts/check-event-publish-tenant.py` 禁止裸 `event.Publish(`；on 模式下 `TenantID=0` 打响声 | 执行者收成唯一一个 + 门禁，才不依赖"下一个人记得写" |
| **P2-6** | 两端同名实体各写一份模型（`ConfigParam`/`ConfigParamAuditLog` 等），schema 靠人对齐 | `hivemtk-platform/platform-server/internal/model/config_param.go` vs `user-server/internal/model/config_param.go` | 两端是两个独立库、两条演进节奏，"共一份代码"未必划算；建议只对**跨端传递的结构**（开户登记、心跳、订单、模块下发）做单一事实源 |
| **P3** | vendor 域名硬编码为默认值（`internal/config/platform.go:33` `DefaultCloudURL = https://work.xapptool.cn`）✅ 已修、`uploads` URL 暴露租户 id、`SetTrustedProxies` 只信私网 ⇒ 反代不在私网时 `ClientIP` 恒为反代 IP（全站共用一个限流桶）✅ 已修 | 见左 | P3-1（`DefaultCloudURL` 回落）：新增 `CloudURLSource` 来源标签 + 启动期 WARN，并把 provider 缺省实参从「同一个常量」改成空串（否则「没配」与「配了同一个值」不可区分，builtin 这一档永远看不到）。P3-3（可信反代）：`middleware.ConfigureTrustedProxies` 成唯一装配入口，名单可由 env `TRUSTED_PROXIES` 覆盖（非法条目整体报错而非静默丢弃），并挂观测中间件在「带 XFF 但来源不受信」时响亮一行（同对端去重 + 累计计数）。P3-2（`uploads` URL 暴露租户 id）与 P1-2 签名 URL 同源，未清 |


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
| T028 越权回归 | 待集成轮 | 已有 5 组；"逐端点双租户交叉扫描"仍未做 → 保留为待办 |
| T019 迁移真库演练 | 随部署进行 | 未变（仍需一次含存量数据的升级演练） |

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
| **无凭证外链签名化**（P1-2） | `/files` 的访客场景 | 预签名/短时 token；本地存储驱动已留 `SignUploadURL` 接口位，目前是占位报错 |
| **平台端商户隔离上框架**（P1-4） | 两端能力不对称 | 推荐；与用户端 §7 方案 B（能力类型化）同源 |
| **Postgres RLS** | `SAAS_MULTI_TENANT_EVALUATION_2026-10.md` §7 已否（连接池/事务语义、表 owner 绕过、`tenant_id=0` 与 `1` 并存无权威表） | 维持"本轮不做"，但仍是侵入面的理论下限；若做，先补"哪些行该是 `tenant_id=0`"的权威表 |
| **形态矩阵契约门禁**（方案 E）✅ 已落地 | `DEPLOYMENT_MODES_EVALUATION_2026-10.md` §6 | `internal/app/deployment_matrix_contract_test.go`：四格 × 六条行为（出站/租户上下文/自助开户/初始化态/后台扇出/隔离 callback）= 24 条断言，全部走活体组件。两个轴**正交**是核心断言之一：租户那五条在两列数值必须相同，谁在平台开关里顺手加租户判定就会红。**牙齿**：临时让 off 模式也看 claim、让 `PlatformURLFor` 在关闭态照样返回地址 ⇒ ①②③ 三格全红 |
| **逐端点双租户交叉扫描**（T028 尾巴） | 现有覆盖是"逐域代表用例" | 建议做成表驱动扫描：路由清单 × 双租户身份 × 读/写，断言恒为"不存在/403" |

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
| `user-server/internal/service/{session_assignment,customer_session,customer_session_routing,customer_session_takeover,chat_visitor,inbox_ingress_persist}.go` | 13 处 WS 推送调用点补 ctx |
| `user-server/internal/router/files_guard.go` | 新增 `tenantPathAllowed` + 租户段正则；跨租户 404 |
| `user-server/internal/router/files_guard_tenant_test.go` | 新增：/files 租户归属四态 + 边界（4 例） |
| `user-server/internal/middleware/ownership.go` | 缓存键加租户段；失效函数按前缀清；not-found 语义问题留档（P1-3） |
| `user-server/internal/middleware/ownership_cache_tenant_test.go` | 新增：键形 + 跨租户不共享缓存（2 例） |
| `user-server/internal/app/content_guard_wiring.go` | 并发作业留下的 `envInt` 重名编译红（改名为 `envIntWithFallback`） |
| `specs/001-saas-multi-tenant/tasks.md` | T021/T023/T024/T025/T026/T027/T028 状态按实测改写（消除第二份真相） |
| `docs/architecture/SAAS_FULLCHAIN_AUDIT_2026-10-23.md` | 本文件 |
