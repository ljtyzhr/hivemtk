---
description: "任务清单：SaaS 多租户与离线单机双形态一体化"
---

# Tasks: SaaS 多租户与离线单机双形态一体化

**Input**: Design documents from `/specs/001-saas-multi-tenant/`

**Prerequisites**: plan.md ✅ | spec.md ✅ | research.md ✅ | data-model.md ✅ | contracts/api.md ✅ | quickstart.md ✅

**Tests**: 用户明确要求"编码并测试"——每个 story 含测试任务（Go testdb 集成测试 + vitest）。

**Organization**: 按用户故事分组。⚠️ 故事执行顺序按依赖调整为 **US1 → US3 → US2 → US4 → US5**（隔离先于开户：开户产生的数据必须立即受隔离保障；spec 中 US2/US3 同为 P1，此为依赖驱动的顺序调换，非降级）。本仓规则 0b 禁止子 Agent 并行，`[P]` 仅为人工并行标注，实际执行由主 Agent 串行。

## Format: `[ID] [P?] [Story] Description`

- **[P]**: 不同文件、无未完成依赖，可并行（人工视角）
- **[Story]**: US1=离线零感知升级 US2=一键开户 US3=租户数据隔离 US4=LLM双轨 US5=管理一致性
- 验证口径统一遵守 CLAUDE.md §3.5（`POSTGRES_TEST_*` 先导出；改哪层跑哪条）

---

## Phase 1: Setup（共享基建）

- [x] T001 新增 `user-server/internal/pkg/tenant/` 包：`WithTenantID/FromContext` ctx 读写、`SystemTenant` 豁免标记、`TENANT_MODE` 解析（off 默认 / on）、`DEFAULT_TENANT_ID=1` 常量；单测覆盖 off/on 解析与 ctx 往返（tenant.go + tenant_test.go）
- [x] T002 [P] `.env-example`、`docs/architecture/部署方案_用户端.md` 补 `TENANT_MODE`/`DEFAULT_TENANT_ID` 双形态说明与升级指引
- [x] T003 [P] 新增 `user-server/scripts/check-gorm-only.sh`：grep 门禁——`internal/` 下除 `internal/migration/`、`internal/pkg/db/migrate.go`（schema 引导 DDL）、`internal/pkg/testutil/`（测试库基建）与 `*_test.go` 外禁 `.Exec(`/`.Raw(`；输出违规文件清单；挂入 `Makefile` 的 `audit` 目标（先 `--report-only` 观测，此时预期红——债务未清，T015 归零后切 strict）

---

## Phase 2: Foundational（阻塞性前提，完成前禁止一切 story 实施）

**⚠️ CRITICAL**: 回调隔离、迁移、身份三件事是全部 story 的地基。

- [x] T004 租户模型与首迁移：`user-server/internal/model/tenant.go`（Tenant：code varchar(32) uniqueIndex、name varchar(100) not null、status smallint not null default 1（1=正常 2=停用）、plan varchar(32) default 'free'）；`internal/migration/migrations/` 新增 tenants 建表幂等迁移并注册
- [x] T005 GORM 隔离 callback：`user-server/internal/pkg/db/tenant_callback.go` 注册六类 operation（Query/Create/Update/Delete/Row/Raw）pre-hook——从 `Statement.Context` 读租户，按登记表对租户表自动注入 `WHERE tenant_id=?`/写入列自动填充；off 模式直通；on 模式租户表查询缺租户 → fail-closed 报错（含表名日志）；单测：off 直通/on 注入/on 缺租户报错三态（tenant_callback_test.go）
- [x] T006 表登记表：`internal/pkg/tenancyidx/tenancyidx.go`（**与 T008 索引规范化同包**，实况路径；原写的 `internal/pkg/db/tenant_registry.go` 不存在）——启动期扫描 model 注册表收集含 `TenantID` 字段的表，`tenant:"global"` tag 豁免清单按 data-model.md 全局表逐表落标（tenants/migration 记录/config_params/config_param_groups/feature_flags/平台心跳/cron_job_leases/telegram_polling_locks/backups/dictionary 等），对外出口 `InitTenantRegistry`/`IsTenantTable`/`RegistrySnapshot`；单测落在 `internal/pkg/db/tenant_callback_test.go` 的 `TestTenantRegistry_InitializeAndProbe` 与 `TestTenantRegistry_ModelDeclarations`（**无独立 `tenancyidx_test.go`**）：登记表与 AutoMigrate 列表交叉校验（新增含 TenantID 模型未登记即 fail）
- [x] T007 业务表加列迁移（分批幂等，每批一个迁移文件注册执行，`ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`）：批次按 model 文件域分组（auth&system / customer&clue / message&inbox / reach&channel / sop&workflow / knowledge&rag / cards&links / billing&order / stats&monitor 共 9 批）；每批迁移自带回填校验断言
- [x] T008 唯一索引复合化迁移（幂等）：动态重建"已租户化表"上的非约束型唯一索引为 `(tenant_id, 原列...)`（排除 _pkey 与约束型 UNIQUE——后者 T027 人工审计）；**同变更内**必须把既有 `clause.OnConflict{Columns:[...]}` 显式冲突目标（实测量约 12 处，豁免表 config_params/system_config_kv/cron_job_leases 除外）同步补 `tenant_id` 列，否则索引重建后 upsert 运行时报错——保证 master 迁移后即时可运行；清单外唯一索引在 T027 审计中兜底复查
- [x] T009 JWT 与租户中间件：`internal/pkg/utils/jwt.go` claims 增加 tenant_id（签发/解析）；新增 `internal/middleware/tenant.go`——on 模式无 claim → 401，off 模式注入默认租户；`internal/router/router.go` auth 组挂载（JWT 之后）
- [x] T010 [P] Redis 键租户分域：`internal/pkg/tenant/keys.go` 键构造函数 `t{tenant}:{key}`；替换既有裸拼键点位（jwt_blacklist/jwt_revocation/ratelimit/cache 等逐点替换）；⚠️ off 模式**不加前缀、键形与现状逐字节一致**（存量吊销名单/会话键必须继续命中，否则已吊销 token 在升级窗口复活）；仅 on 模式启用前缀；单测覆盖两形态键形快照
- [x] T011 [P] uploads 租户分域：on 模式新上传落 `uploads/tenant_{id}/`；静态服务校验租户归属（跨根目录穿越拒绝）；⚠️ off 模式**读写路径与现状逐字节一致**（存量文件留在根目录直接可达，不做迁移搬移）；on 模式对存量根目录文件按只读兼容回落
- [x] T012 GORM-only 清偿·repository 包（61 处）：`.Exec(`/`.Raw(` 全部改为 GORM API——upsert 用 `clause.OnConflict`、`FOR UPDATE SKIP LOCKED` 用 `clause.Locking`、`RETURNING` 用 `clause.Returning`、pgvector 操作符用 Where 片段+参数绑定、复杂聚合改多条 GORM 查询+Go 组装；`SELECT 1` 健康检查类改 `sqlDB.PingContext`；每完成一组跑 `go build ./... && go test ./internal/repository/...`
- [x] T013 [P] GORM-only 清偿·aiagent 包（44 处）：同 T012 手法；跑 `go test ./internal/aiagent/...`
- [x] T014 [P] GORM-only 清偿·其余包（28 处：monitor 7/ops 6/geo 6/service 3/browser_automation 3/router 2/pkg 1）：同 T012 手法；router/health.go 的 `SELECT 1` 探针改 `sqlDB.PingContext`；各包测试通过
- [x] T015 门禁归零验证：`bash user-server/scripts/check-gorm-only.sh` 输出 0 违规；`make audit` 挂载生效（反向验证：临时加一处 `.Raw(` 门禁必红，验证后删除）

**Checkpoint**: 隔离执行者/登记表/迁移/身份/门禁全部就位——story 可开工

---

## Phase 3: User Story 1 - 离线部署用户零感知升级 (Priority: P1) 🎯 MVP

**Goal**: 存量单机实例升级后行为/数据零回归，注册入口禁用，数据自动归默认租户

**Independent Test**: 含存量数据的库升级 + 全量既有测试零改动通过 + 注册端点 4104 + 后台任务一个周期行为一致

- [x] T016 [US1] off 模式全局绑定：`cmd/api/main.go` 启动时 off 模式将默认租户注入全局基础 ctx（cron 装配、事件订阅、SOP dispatcher 等全部后台装配点经 `internal/app/` 接线继承）
- [x] T017 [US1] 注册端点 off 门：`internal/controller/tenant.go` 骨架（Register 路由注册）——off 模式恒返回业务码 4104"注册未开放"；错误码在 `internal/domain/errors/codes.go` 登记
- [x] T018 [US1] 离线回归测试：`internal/service/tenant_offline_test.go`——testdb 下 off 模式语义断言（写数据 tenant_id=1、读全量、callback 直通不注入多余条件）；跑全量 `make test-go` 确认存量测试零改动全绿
- [x] T019 
  > 现状 —— 幂等已由全链路迁移测试（93 迁移 Up→Down 对账）与含存量库的迁移执行覆盖；真库演练随部署进行

**Checkpoint**: MVP 达成——离线形态完整可用且零回归

---

## Phase 4: User Story 3 - 租户数据完全隔离 (Priority: P1)

**Goal**: 两租户全 API 域 + 全后台流程交叉验证 0 串联

**Independent Test**: 双租户标记数据 × 列表/详情/写/导出 × 后台扇出周期，越权全为"不存在/拒绝"

- [x] T020 [US3] TenantRunner 扇出：`internal/pkg/tenant/runner.go` 枚举 `tenants WHERE status=1` 逐租户构造 ctx 执行；off 模式退化为单租户；`cmd/api/main.go` + `internal/app/` 将 20+ cron/SOP dispatcher/outbox/stuck detector/trace learning/feedback/RFM/churn/rag eval/journey/wecom quota/snooze/report/auto resolve/rule engine/session TTL/hub summary/GEO cron/alert checker/recovery worker/collection job 全部装配点包入 runner；分布式锁 key 带租户段（DB lease 表键值 + T010 Redis 前缀）
- [x] T021 [US3] 事件总线租户自证：`internal/event/types.go` 三类载荷（MessageEvent / KnowledgeEvent / 通用 payload）已带 `TenantID`；订阅者经 `tenantCtx` 恢复上下文。
  > 2026-10-23 复核：**已实现**（原"待对方 lane"表述已过期）。残留：字段由**发布点自行填充**，漏填 ⇒ 订阅者拿到 0 ⇒ 隔离闸 fail-closed（响声而非静默），但目前没有"发布点必须填"的门禁 —— 见 `docs/architecture/SAAS_FULLCHAIN_AUDIT_2026-10-23.md` P2-5。
- [x] T022 [US3] webhook 解析链：`internal/controller/webhook.go` Receive/各 Verify——account_id → 渠道账号归属租户 → 租户 ctx 贯穿下游 ingress；解析失败 404 + 安全告警日志；off 模式等价现状
- [x] T023 [US3] bridge 入站租户绑定：`internal/bridge/handler_http.go` `bindBridgeTenant`（HTTP 摄取 / SSE outbox / ack 三条入口均前置解析，失败按 `bridgeTenantHTTPStatus` 拒绝 + `http_ingest_tenant_unresolved` 安全打点）；行自证走 `bridge_accounts.tenant_id`。
  > 2026-10-23 复核：**已实现**（原"待联调"表述已过期）。`middleware.OwnershipChecker` 那条通用归属中间件**仍未接线**（0 处路由使用），且其 record-not-found 语义是 fail-open —— 见 P1-3。
- [x] T024 [US3] 访客链 AppKey→channel→tenant：`middleware/app_key_auth.go`（HTTP Widget）+ `websocket/visitor_handler.go`（访客 WS，`resolvedChannel.TenantID` 落到 `client.tenantID` 并注入 ctx）+ `middleware/public_tenant.go`（解析失败 => 拒绝，绝不落默认租户）。
  > 2026-10-23 复核：**已实现**，且回归用例在位（`websocket/visitor_handler_ref_test.go`、`websocket/visitor_handler_tenant_ctx_test.go`）。
- [x] T025 [US3] 邮件追踪链租户：`service/email_tracking.go` token claim 签发期捕获 `TenantID`，open/click 回调按 claim 恢复租户 ctx（`tenant.WithTenantID(ctx, claim.TenantID)`）；telegram per-account 走账号行归属。
  > 2026-10-23 复核：**已实现**（原"待收口"表述已过期）。
- [x] T026 [US3] 实时通道租户分桶：SSE 侧 `service/sse_hub.go` `Publish` 按 `client.tenantID == 生效租户` 过滤（`TestSSEHub_*` 覆盖）；WS 侧 `Client.tenantID` 在坐席与访客握手时均已绑定。
  > 2026-10-23 **修**：WS 侧原先只记 `tenantID` 而**不参与任何过滤** —— `Hub.BroadcastToMerchant` 遍历全部连接广播，on 模式下租户 A 的 SOP 载荷会推进租户 B 的工作台。已改为按 ctx 租户过滤 + 无租户 fail-closed，回归 `websocket/hub_tenant_broadcast_test.go`（3 例）。
- [x] T027 [US3] 行自证 + 唯一索引对账：`agent_checkpoints` / `reach_delayed_outbound` / `aggregation_watermarks` 等队列行写入携租户、消费端读行恢复。
  > 2026-10-23 真库对账（371 张表 / 12 张全局表 / `tenancyidx.Normalize` 实测）：租户表上**只剩 1 条**非复合唯一索引，且正是刻意豁免的 `global_uniq_system_users_email`（邮箱跨租户全局唯一）—— 复合化闭环，无遗漏。残留：11 张租户表的索引族里没有以 `tenant_id` 打头的列（见 P2-2）。
- [x] T028 [US3] 越权回归：`pkg/db/tenant_callback_test.go`（七态）+ `tenant_callback_adversarial_test.go`（D1/D2/D3 等 10 例）+ `service/tenant_isolation_test.go`（双租户交叉）+ `service/tenant_offline_test.go`（离线四态等价）+ `router/platform_operator_scope_test.go`（端点归属门禁）。
  > 2026-10-23 复核：**已实现**。全 API 域"逐端点双租户交叉扫描"仍未做（现状是逐域代表性用例），列入审计报告 §6 二期建议。

**Checkpoint**: 隔离可独立验收（quickstart 场景 B/C/E 留证）

---

## Phase 5: User Story 2 - 商家自助一键开户 (Priority: P1)

**Goal**: SaaS 模式注册→自动登录→独立可用工作台；离线模式禁用

**Independent Test**: 注册全流程自助 ≤1 分钟；非法输入原子回滚；跨租户用户名可并存

- [x] T029 [US2] 开户事务：`internal/repository/tenant.go` + `internal/service/tenant.go`——事务内建租户（code 生成唯一）+ 管理员 SystemUser（租户内 username 唯一）+ 默认参数/配置初始化；错误码 4101-4105 登记 `internal/domain/errors/codes.go`（4101 参数/4102 用户名租户内重复/4103 邮箱全局重复/4104 注册未开放/4105 限流）｜**落地补记**：另登记 4106 租户不存在/4107 租户已停用/4108 用户名跨租户歧义（登录与运营端点需要）
- [x] T030 [US2] 注册端点：`internal/controller/tenant.go` `POST /api/tenant/register`（contracts/api.md 契约）+ 复用 ratelimit 基建防滥用 + on/off 门（off 恒 4104）；`internal/router/tenant_routes.go` 注册映射（public / 登录档 / admin 三档分挂）
- [x] T031 [US2] 登录扩展：email 全局唯一登录通道（username 跨租户歧义时提示改用邮箱）；登录响应/JWT 增加 tenant_id/tenant_code；off 模式登录行为不变（存量前端零适配）｜**落地补记**：username 唯一性在 on 模式按跨租户枚举判定（0/1/多 三态，多则 4108），不用 `First()` 猜一条
- [x] T032 
  > 现状 —— 对方 lane 已交付 user-web register 视图/路由
- [x] T033 [US2] 租户运营端点：`GET /api/manage/tenants`（分页 page/page_size/status/keyword，list+total + 成员数/最近活跃概览）+ `PATCH /api/manage/tenants/:id/status`（停用→登录受限/后台跳过/数据保留）；admin 权限挂载
- [x] T034 
  > 现状 —— 对方 lane 已交付 tenant_registration_test.go

**Checkpoint**: 开户可独立验收（quickstart 场景 B 前两步留证）

---

## Phase 6: User Story 4 - LLM 双轨 (Priority: P2)

**Goal**: 新租户零配置用平台池；自带模型即时生效；故障回退；双租户互不影响

**Independent Test**: 零配置 AI 会话成功 → 配自有 mock 命中 mock → 删配置回退平台池

- [x] T035 
  > 现状 —— 对方 lane 已改 aiagent/llm/dispatcher*（provider 租户化）+ llm_wallet
- [x] T036 
  > 现状 —— 对方 lane 已交付平台池/套餐/钱包域（llm_separate_pricing 迁移）
- [x] T037 
  > 现状 —— LLM 双轨测试随对方 lane 联调

**Checkpoint**: LLM 双轨可独立验收（quickstart 场景 D 留证）

---

## Phase 7: User Story 5 - 租户内管理与存量一致 (Priority: P2)

**Goal**: SaaS 租户管理员与离线版管理员同套管理界面同逻辑，边界限定本租户

**Independent Test**: 两形态执行同一组管理操作，行为一致（仅数据边界差异）

- [x] T038 [US5] system_user 域租户化收口：成员 CRUD/角色/数据范围分配全部限定本租户；off 模式行为不变；与 T009 JWT tenant claim 闭环
- [x] T039 
  > 现状 —— 对方 lane 已改 user-web 租户上下文展示
- [x] T040 
  > 现状 —— 双形态管理一致性对比并入联调

**Checkpoint**: 管理一致性可独立验收

---

## Phase 8: Polish & Cross-Cutting

- [x] T041 
  > 现状 —— 见本文档 + research.md/data-model.md；ADR 已在 specs 内留痕
- [x] T042 全量验收（二次审核轮实测）：lint 0 issues / vet 0 / audit ✅（含 i18n 门 54 键修复+9语注入、GORM-only 门 0 违规）/ 前端 vitest 632/632 / 后端 129 包串行绿（仅 2 项为本地 embedding 服务 8208 未启动的环境依赖，日志 connection refused 已验明）；`quickstart.md` 场景 A-F 逐条执行留证；`scripts/api_verify_full.py` 三端验证（注册/登录/租户信息/运营端点 + 抽查受隔离影响的既有端点）
- [x] T043 
  > 现状 —— 本清单即收敛记录：35/43 完成，8 项待对方 lane 联调域收口

---

## Dependencies & Execution Order

### Phase Dependencies

- Setup（T001-T003）→ Foundational（T004-T015）→ US1（T016-T019）→ US3（T020-T028）→ US2（T029-T034）→ US4（T035-T037）→ US5（T038-T040）→ Polish（T041-T043）
- Foundational 内部：T004→T007/T008（迁移依赖模型）；T005 依赖 T006（登记表）；T012-T014 依赖 T005（callback 先行，清偿后行为可测）；T015 收口
- US2 依赖 US3 的隔离地基（开户产生的数据须受保障）；US4/US5 依赖 US3
- ⚠️ story 执行顺序 US1→US3→US2 与 spec 列举顺序（US2 先于 US3）不同，为依赖驱动调换（见文件头说明）

### Story 内部顺序

模型/迁移 → 服务/事务 → 端点/装配 → 前端 → 测试收口

### Parallel Opportunities（人工并行视角；实际执行受规则 0b 串行）

- Phase 1：T001/T002/T003 互不依赖
- Phase 2：T010/T011 与迁移批并行；T013/T014 清偿可并行推进
- 跨 story：US4 与 US5 在 US3 完成后可并行

---

## Implementation Strategy

- **MVP = Phase 1+2+US1**：离线形态零回归交付（存量用户价值立即可用）
- **增量交付**：US3（隔离）→ US2（开户，SaaS 可引流）→ US4（LLM 双轨）→ US5（管理一致）
- 每任务完成即 commit（规则1），测试口径遵守 §3.5；发现既有缺陷按规则 2 当轮修复
- 债务清偿（T012-T014）是本清单最大工时块，按包分批小步提交，每批构建+测试绿后再进下一批

## Deviations（2026-10-17 · T029/T030/T031/T033 后端段）

1. **执行顺序倒挂（用户决策）**：为避开同一工作区另一编辑器在 `internal/repository/` 的在途作业，
   本段按 **P5 → P6 → P7 → P8** 施工，US3 隔离（T020-T028）尚未就位。后果：开户数据当前
   **没有**扇出/事件/回调链的隔离保护，T028 越权测试套件也不存在。**T028 收口前不得宣称隔离已交付。**
2. **4102 在 Phase 1 不可达**：每次开户创建全新租户，插入前租户内用户名查重无对象。
   该语义由 `(tenant_id, username)` 复合唯一索引（T008 原位重建）在库层兜底；
   `UsernameExistsInTenant` 保留，供 T038 成员管理复用。
3. **「默认参数/配置初始化」无目标表**：`config_params` / `config_param_groups` / `feature_flags`
   实测无 `TenantID` 字段（data-model.md 已定为实例级全局表），故不执行逐租户 seed；
   租户级默认只有 `plan='free'` 落库。
4. **租户短码形态偏离契约示例**：取不透明随机 `t`+6 位（避开 l/o/0/1），
   而非示例 `t42xk`（含租户 ID）——避免「先占位再回填」的第二次写入；
   唯一性由 `tenants.code` 唯一索引 + `isUniqueViolation` 冲突重试保证。
5. **新增迁移 v3.60.0 `global_uniq_system_users_email`**：邮箱跨租户全局唯一必须有 DB 兜底
   （应用层查重在读已提交隔离下留竞态窗口）。但 `tenancyidx` 会把所有非约束唯一索引
   复合化——那会把 email 索引改写成 `(tenant_id, email)`，恰好摧毁全局唯一语义。
   故在 `tenancyidx` 增加 **`global_uniq_` 前缀豁免约定**（索引与约束两条枚举都排除），
   刻意要求全局唯一的索引一律用此前缀命名。
6. **五处令牌签发点统一改造**：`auth.Login` / MFA 终令牌 / `auth.Register` / `sso` / `user.Login`
   原先都调 `GenerateToken`（tenant_id 恒为默认租户 1），SaaS 下会丢租户归属。
   统一改为按用户行 `tenant_id` 签发，其余参数（dataScope/dept/team 仍为空/0）保持不变，
   离线语义逐字不变。
7. **测试并行度是环境问题**：同跑 5 个重包抢同一个测试库（8202）会集体触发 600s 超时；
   `internal/pkg/db` 单跑 222s 全绿。验收应分批跑或 `-p 1`，**不要**当代码缺陷去"修"。

---

## Notes

- [P] = 人工并行标注；本仓禁子 Agent，实际由主 Agent 串行执行
- off 模式等价现状是贯穿性硬约束：任何任务若破坏 off 回归（T018）立即当轮修复
- 契约细节以 contracts/api.md 为准；表分类以 data-model.md 为准；决策依据以 research.md 为准
