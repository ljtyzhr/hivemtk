# Phase 0 研究：站内消息通知模块

> 结论先行：全部技术未知点已在探查真实代码与线上环境中解决，无 NEEDS CLARIFICATION 残留。

## R1. 投递方向：拉取 vs 推送

**Decision**：用户端**周期拉取**（pull），平台不主动回调。

**Rationale**：
- 生产为**单机部署**（实测 `docker ps`：一个 `mtk-user-server` + 一个 `mtk-postgres`），
  user-server **没有公网回调地址**。平台要"推"就必须先给 user-server 装一条入站通道。
- 仓内**已有同款模式且线上验证通过**：`module-sync`（模块下发同步）就是 user-server
  周期调平台 `GET /merchant-api/modules/sync?tenant_code=xxx`，HMAC 签名，
  周期 10 分钟。线上日志实测：`[module-sync] 已刷新 8 条订单状态`。
- 拉取方向天然满足"全员收到"：每个租户的 user-server 实例各自拉自己那份。

**Alternatives considered**：
- *WebSocket 推送*：`user-server/internal/websocket` 的 hub 现只服务访客与智能体，
  不接登录用户；接入需加登录态鉴权、租户路由、前端重连。工作量翻倍，
  且 FR-002 只要求 5 分钟内可见。
- *平台侧定时扫租户逐个 POST*：需要平台侧持有全部 user-server 地址 —— 同上，没有。

**决策依据**：与仓内既有、线上已验证的模式同构，风险最低。

## R2. 投递周期

**Decision**：3 分钟。

**Rationale**：
- 平台心跳上报周期为 3 分钟（`heartbeat_sender.go` 默认值，线上可读），
  说明平台↔用户端链路在 3 分钟尺度上已验证稳定。
- 拉取请求带 HMAC 签名，3 分钟 = 每天 480 次往返，对当前规模（16 租户）
  完全可接受。
- 满足 FR-002（5 分钟内可见）且留有余量。

**Alternatives considered**：1 分钟（更及时但往返翻三倍，无收益）；
10 分钟（对齐 module-sync 但逼近 FR-002 的 5 分钟红线，剔除）。

## R3. 已读语义：拆回执表

**Decision**：平台消息的已读状态放**独立回执表** `notification_reads`，
按 `(notification_id, user_id)` 唯一；`notifications.is_read` 原字段**保留不动**。

**Rationale**：
- 现有 `notifications` 表的 `user_id=0` 表示"全体用户"，`is_read` 是**消息行上的单一布尔位**。
  这意味着 A 用户点已读，B 用户看到的那条也变已读 —— 广播消息下未读角标会被
  一个人清零。这是**根本性缺陷**，加索引或改查询都解决不了。
- 平台消息 = 1 条消息 × N 个用户 ⇒ 天然需要 1:N 的已读关系。
- 本地业务通知（SLA 逾期、待归因坏例、待审核变更）语义不同：
  它们的受众是**管理员/审核人**，"任一人读过即算"是对的（避免 N 个人各点一次）。
  保持 `is_read` 原样，**不迁移存量**，零回归风险。

**Alternatives considered**：
- *给 `notifications` 加 `read_by` JSON 字段*：读面要全表扫 + JSON 过滤，
  未读数统计无法走索引，量大后必然劣化。
- *广播消息也按人独立已读（改 `is_read` 语义）*：会让既有待办 SLA 的
  "升级面向管理员全体"提醒变成 N 份待办，是**行为回归**，本次不碰。

## R4. 幂等键

**Decision**：用户端 `notifications` 新增 `source_msg_id`（平台消息 ID）+ 唯一索引
`(tenant_id, source_msg_id)`，投递用 `ON CONFLICT DO NOTHING` 语义的 upsert。

**Rationale**：
- 拉取天然可能重复：网络重试、周期重叠、多实例并发。
- `(tenant_id, source_msg_id)` 复合唯一是租户维度正确的幂等键
  （单列唯一会跨租户撞车 —— 违反 4.3b 判据）。
- 该列只对平台消息非空；本地业务通知 `source_msg_id = 0`，
  故唯一索引必须**允许 0 重复**（PG 的普通唯一索引不行）⇒ 用
  `WHERE source_msg_id > 0` 的**部分唯一索引**（`uniqueIndex:where`）。

**Alternatives considered**：用 `link` 做键（不唯一，平台消息可能 link 相同）。

## R5. 跨租户写面：如何把消息落到正确的租户

**Decision**：写库用 `tenant.OnlyTenant(ctx, tid)`（只带该租户的干净 ctx），
**不用** `SystemScope`。

**Rationale**：
- 隔离在 GORM callback 层，判据是"表有没有 `tenant_id` 列" ⇒ `notifications`
  有 `tenant_id`，是租户表 ⇒ callback 会自动注入 `WHERE tenant_id=?` 与填列。
- `SystemScope` 会**跳过填列** ⇒ 行落进列默认值 1（默认租户）。
  仓内 `module_catalog.go:526` 有明确注释记载这个坑：
  "ctx 挂着 SystemScope：直接用会绕过隔离填充（行落列默认值 1），慢路径先查后写
  也会变跨租户 —— 换成只带该租户的干净 ctx"。
- 本仓 memory 亦记有 `feedback-gin-context-loses-tenant`：控制器传
  `ctx.Request.Context()`（不带租户）在 on 模式会 fail-closed。

**Alternatives considered**：
- *手动 `Select("tenant_id")` 强写*：违反 callback 的 fail-closed 设计，
  且 4.3d 判据要求"写路径必须能填出 tenant_id"，靠 callback 填才是正解。

## R6. 平台侧如何按租户过滤

**Decision**：平台 `platform_messages` 扩 `tenant_scope` 列（`all` 或逗号分隔的
tenant_code 列表），拉取端点按 `tenant_code` 匹配。

**Rationale**：
- 平台端**没有 `tenant_id` 列也没有租户隔离 callback** —— 它是全局表，
  租户维度靠 `tenant_registrations.tenant_code` 这条既有链路。
- `platform_messages.targets` 列**已存在**，但 `message_service.go` 只做
  `strings.Join(targets, ",")` 落库、**从不被读取** ⇒ 复用它即可，
  不必新增列。`targets` 空 = 全体（与既有注释口径一致：
  "targets 为空表示面向全部商户"）。
- 因此**不新增 `tenant_scope` 列**，只把 `targets` 用起来 —— 少一列少一处漂移。

**修正记录**：初稿曾计划新增 `tenant_scope`，探查后确认 `targets` 语义完全够用
（空 = 全体），改为复用并让 service 真正消费它。

## R7. `level` 列同样是死列

**Decision**：不新增 level 列，复用 `platform_messages.level`。

**Rationale**：`message_service.go:38` 硬编码 `Level: model.MessageLevelInfo`，
即 `Send` 收了 `msgType` 但 `level` 从不取用户输入 ⇒ **现有 `level` 列恒为 info**。
本次让 `level` 真正生效即可。

**修正记录**：初稿曾计划新增 level 列，探查后确认列已存在、只是没接线。

## R8. 平台→用户端的接口形态

**Decision**：平台新增 `GET /merchant-api/notices/list?tenant_code=xxx&since=xxx`，
走既有 `MerchantAuth` 中间件（HMAC-SHA256，签名 PATH **不含 query string**）。

**Rationale**：
- 复用 `merchant-api` 组（`router.go:61-62` 已挂 `MerchantAuth()`）——
  这是仓内既有的商户侧通道，签名规则、secret 取值（每商户独立 `api_secret`
  优先、回落全局）全部现成，**不新造鉴权**。
- 签名 PATH 不含 query 是硬约束（memory `reference-prod-server-access` 实测：
  含了就 401）。因此 `tenant_code` / `since` 走 **query**，但**签名时剥离**。
  既有 `module_client.go:213` 正是这么做的：
  `path := "/merchant-api/modules/sync?tenant_code=" + url.QueryEscape(tenantCode)`。
- `since` 用**发布时间戳**而非自增 ID：发布时间跨库可比，且 ID 在平台库自增、
  用户端无从判断"是否已拉过"。改用 `published_at > since` 最直白。

**Alternatives considered**：
- *用平台消息 ID 做游标*：平台库 ID 单调但用户端要记住"已拉到第几号"，
  与"平台在两次拉取之间插入新消息"存在空洞风险；时间戳无此问题。

## R9. 平台侧如何知道有哪些租户可选（发布页的目标选择）

**Decision**：发布页从平台已有的 `GET /platform/tenants` 取租户列表
（`tenant_registrations` 表，含 `tenant_code` / `company_name` / `status`）。

**Rationale**：该端点已存在且已被 platform-web 的租户管理页使用
（`views/platform/tenant/List.vue`），无需新建租户查询接口。
发布页直接复用，避免第二套租户列表口径。

## R10. 前端铃铛角标为何恒为 0

**Decision**：在 `Layout.vue` 里给 `unreadCount` 接上轮询。

**Rationale**：`Layout.vue:241` 声明 `const unreadCount = ref(0)`，全仓 grep
`unreadCount` **只有声明没有赋值** ⇒ 角标恒 0，从未工作过。
同文件 `geoAlertCount` 是现成的可用范式（`Layout.vue:250` 赋值，
定时器轮询 `getGeoAlertsUnreadCount`）⇒ 照抄这个形状即可，不发明新模式。

## R11. 用户端同步器挂在哪

**Decision**：新建独立 runner `notice_sync_runner.go`，**不挂进 module-sync**。

**Rationale**：
- module-sync 周期 10 分钟（对模块开通足够），通知要求 5 分钟内 ⇒ 周期不同。
- module-sync 的失败语义是"平台不可达 → 离线兜底灌基础模块"，与通知
  "平台不可达 → 保留上次已投递"冲突 ⇒ 混在一起会互相污染降级逻辑。
- 独立 runner 的首轮**同步执行**（与 module-sync 同）：否则启动后到首轮完成
  之间存在窗口。首轮失败不阻断启动（平台不可达是离线部署的常态）。

## R12. 平台不可达时的降级

**Decision**：保留上次已投递的消息，不清空、不报错中断。

**Rationale**：module-sync 已有同款判定（"清空会让用户正在用的模块突然消失
—— 那比'数据不准'严重得多"）。通知同理：清空会让运营方发的公告凭空消失，
用户会认为系统故障。

## R13. 存量 `notifications` 表的迁移方式

**Decision**：走**仓内既有的 Go 版本化迁移**（`internal/migration/migrations/*.go`
的 AutoMigrate），不写 `migrations/*.sql`。

**Rationale**：
- 仓内 model-list.md 的判据明确写了三处登记口径：`db.allModels()` /
  `RegisterExtraModels` / **Go 版本化迁移**。SQL 文件那条路已被标注为
  "不逐一列举"的遗留，新增功能应走 Go 迁移。
- Go 迁移随代码一起版本化 ⇒ 不存在 `deploy.sh rsync` 与 `Dockerfile COPY`
  两处同步问题（平台端 db.go 的注释正是这么抱怨 SQL 文件的）。
- `notifications` 已在 `internal/pkg/db/migrate.go:313` 的 `allModels()` 中登记
  ⇒ 加列只需 AutoMigrate 能加；新表 `notification_reads` 需登记进 `allModels()`。

**平台端**：`platform_messages` 扩列同样走 AutoMigrate
（`internal/utils/db/db.go` 的 `migrateModels`），但该处有历史教训——
"AutoMigrate 失败被 warning 静默吞掉" ⇒ **必须同时加幂等 DDL 兜底**
（`ALTER TABLE ... ADD COLUMN IF NOT EXISTS`），这是 db.go 里既有的双保险做法。

## R14. 测试策略

**Decision**：
- Go 单测：表驱动 + 真实 PG 测试库（`POSTGRES_TEST_*` 环境变量），
  重点覆盖幂等去重、跨租户隔离、按人已读三个点。
- 跨租户隔离测试必须**用两个不同租户**的种子（memory `project-tenant-isolation-mechanism`
  明确：同键两租户各一行才照得出跨租户）。
- 前端：Vitest。
- 端到端：`scripts/api_verify_full.py` + quickstart.md 的手工验证。

**Why**：本次的核心风险全在隔离与幂等，单测必须打在这些点上；
而"发布线上"这一步只能靠真实环境验证。

## 未解决项

无。全部技术未知点已由实探代码与线上环境解决。
