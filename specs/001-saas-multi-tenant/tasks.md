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
- [x] T006 表登记表：`internal/pkg/db/tenant_registry.go` 启动期扫描 model 注册表收集含 `TenantID` 字段的表；`tenant:"global"` tag 豁免清单按 data-model.md 全局表逐表落标（tenants/migration 记录/config_params/config_param_groups/feature_flags/平台心跳/cron_job_leases/telegram_polling_locks/backups/dictionary 等）；单测：登记表与 AutoMigrate 列表交叉校验（新增含 TenantID 模型未登记即 fail）
- [x] T007 业务表加列迁移（分批幂等，每批一个迁移文件注册执行，`ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`）：批次按 model 文件域分组（auth&system / customer&clue / message&inbox / reach&channel / sop&workflow / knowledge&rag / cards&links / billing&order / stats&monitor 共 9 批）；每批迁移自带回填校验断言
- [x] T008 唯一索引复合化迁移（幂等）：动态重建"已租户化表"上的非约束型唯一索引为 `(tenant_id, 原列...)`（排除 _pkey 与约束型 UNIQUE——后者 T027 人工审计）；**同变更内**必须把既有 `clause.OnConflict{Columns:[...]}` 显式冲突目标（实测量约 12 处，豁免表 config_params/system_config_kv/cron_job_leases 除外）同步补 `tenant_id` 列，否则索引重建后 upsert 运行时报错——保证 master 迁移后即时可运行；清单外唯一索引在 T027 审计中兜底复查
- [x] T009 JWT 与租户中间件：`internal/pkg/utils/jwt.go` claims 增加 tenant_id（签发/解析）；新增 `internal/middleware/tenant.go`——on 模式无 claim → 401，off 模式注入默认租户；`internal/router/router.go` auth 组挂载（JWT 之后）
- [x] T010 [P] Redis 键租户分域：`internal/pkg/tenant/keys.go` 键构造函数 `t{tenant}:{key}`；替换既有裸拼键点位（jwt_blacklist/jwt_revocation/ratelimit/cache 等逐点替换）；⚠️ off 模式**不加前缀、键形与现状逐字节一致**（存量吊销名单/会话键必须继续命中，否则已吊销 token 在升级窗口复活）；仅 on 模式启用前缀；单测覆盖两形态键形快照
- [x] T011 [P] uploads 租户分域：on 模式新上传落 `uploads/tenant_{id}/`；静态服务校验租户归属（跨根目录穿越拒绝）；⚠️ off 模式**读写路径与现状逐字节一致**（存量文件留在根目录直接可达，不做迁移搬移）；on 模式对存量根目录文件按只读兼容回落
- [ ] T012 GORM-only 清偿·repository 包（61 处）：`.Exec(`/`.Raw(` 全部改为 GORM API——upsert 用 `clause.OnConflict`、`FOR UPDATE SKIP LOCKED` 用 `clause.Locking`、`RETURNING` 用 `clause.Returning`、pgvector 操作符用 Where 片段+参数绑定、复杂聚合改多条 GORM 查询+Go 组装；`SELECT 1` 健康检查类改 `sqlDB.PingContext`；每完成一组跑 `go build ./... && go test ./internal/repository/...`
- [ ] T013 [P] GORM-only 清偿·aiagent 包（44 处）：同 T012 手法；跑 `go test ./internal/aiagent/...`
- [ ] T014 [P] GORM-only 清偿·其余包（28 处：monitor 7/ops 6/geo 6/service 3/browser_automation 3/router 2/pkg 1）：同 T012 手法；router/health.go 的 `SELECT 1` 探针改 `sqlDB.PingContext`；各包测试通过
- [ ] T015 门禁归零验证：`bash user-server/scripts/check-gorm-only.sh` 输出 0 违规；`make audit` 挂载生效（反向验证：临时加一处 `.Raw(` 门禁必红，验证后删除）

**Checkpoint**: 隔离执行者/登记表/迁移/身份/门禁全部就位——story 可开工

---

## Phase 3: User Story 1 - 离线部署用户零感知升级 (Priority: P1) 🎯 MVP

**Goal**: 存量单机实例升级后行为/数据零回归，注册入口禁用，数据自动归默认租户

**Independent Test**: 含存量数据的库升级 + 全量既有测试零改动通过 + 注册端点 4104 + 后台任务一个周期行为一致

- [ ] T016 [US1] off 模式全局绑定：`cmd/api/main.go` 启动时 off 模式将默认租户注入全局基础 ctx（cron 装配、事件订阅、SOP dispatcher 等全部后台装配点经 `internal/app/` 接线继承）
- [ ] T017 [US1] 注册端点 off 门：`internal/controller/tenant.go` 骨架（Register 路由注册）——off 模式恒返回业务码 4104"注册未开放"；错误码在 `internal/domain/errors/codes.go` 登记
- [ ] T018 [US1] 离线回归测试：`internal/service/tenant_offline_test.go`——testdb 下 off 模式语义断言（写数据 tenant_id=1、读全量、callback 直通不注入多余条件）；跑全量 `make test-go` 确认存量测试零改动全绿
- [ ] T019 [US1] 升级路径演练：本地起含存量数据的 Postgres（8202），执行 059+ 迁移序列，验证幂等（重复执行不炸）与中断续跑（每批独立事务）；quickstart 场景 A 逐条留证

**Checkpoint**: MVP 达成——离线形态完整可用且零回归

---

## Phase 4: User Story 3 - 租户数据完全隔离 (Priority: P1)

**Goal**: 两租户全 API 域 + 全后台流程交叉验证 0 串联

**Independent Test**: 双租户标记数据 × 列表/详情/写/导出 × 后台扇出周期，越权全为"不存在/拒绝"

- [ ] T020 [US3] TenantRunner 扇出：`internal/pkg/tenant/runner.go` 枚举 `tenants WHERE status=1` 逐租户构造 ctx 执行；off 模式退化为单租户；`cmd/api/main.go` + `internal/app/` 将 20+ cron/SOP dispatcher/outbox/stuck detector/trace learning/feedback/RFM/churn/rag eval/journey/wecom quota/snooze/report/auto resolve/rule engine/session TTL/hub summary/GEO cron/alert checker/recovery worker/collection job 全部装配点包入 runner；分布式锁 key 带租户段（DB lease 表键值 + T010 Redis 前缀）
- [ ] T021 [US3] 事件总线租户化：`internal/event/` payload 结构体增加 TenantID 字段（编译期强制）；发布点从 ctx 取租户填充；订阅者以 payload.TenantID 构造 ctx（禁止读环境）
- [ ] T022 [US3] webhook 解析链：`internal/controller/webhook.go` Receive/各 Verify——account_id → 渠道账号归属租户 → 租户 ctx 贯穿下游 ingress；解析失败 404 + 安全告警日志；off 模式等价现状
- [ ] T023 [US3] bridge 解析链：X-Bridge-Token → bridge_account → tenant；`internal/router/router.go` RegisterOwnershipChecker 升级为 (tenant_id, user_id) 双维校验；SSE 出站按租户 claim
- [ ] T024 [US3] 访客链：AppKey/Channel-Id → chat_channel → tenant；visitor token 签发与校验携带租户；`internal/controller/chat_public.go` 全链路租户 ctx
- [ ] T025 [US3] 轮询与回调链：telegram long-polling per-account 循环构造租户 ctx；邮件收发回调按账号配置解析租户
- [ ] T026 [US3] WS/SSE 分桶：`internal/websocket/` 连接注册时绑定租户；推送按租户过滤；dashboard SSE hub 主题按租户隔离
- [ ] T027 [US3] 行自证与索引审计：待办表（sop_timer/sop outbox/sop execution/delayed_outbound/approval_request/agent_checkpoint/human_task/reach 任务/导入导出）集成测试——写入行自动携带 tenant_id、消费端读行取租户执行；全库唯一索引与 data-model.md 复合化清单对账（漏网唯一索引补 T008 批次）
- [ ] T028 [US3] 越权测试套件：`internal/service/tenant_isolation_test.go`——双租户 fixture × 每业务域（customer/clue/inbox/reach/sop/kb/cards/links/bill/quote/dashboard…）交叉列表/详情/更新/删除/导出断言 0 泄漏；伪造跨租户资源 ID 表现为 404/403；后台流程产出归属断言

**Checkpoint**: 隔离可独立验收（quickstart 场景 B/C/E 留证）

---

## Phase 5: User Story 2 - 商家自助一键开户 (Priority: P1)

**Goal**: SaaS 模式注册→自动登录→独立可用工作台；离线模式禁用

**Independent Test**: 注册全流程自助 ≤1 分钟；非法输入原子回滚；跨租户用户名可并存

- [ ] T029 [US2] 开户事务：`internal/repository/tenant.go` + `internal/service/tenant.go`——事务内建租户（code 生成唯一）+ 管理员 SystemUser（租户内 username 唯一）+ 默认参数/配置初始化；错误码 4101-4105 登记 `internal/domain/errors/codes.go`（4101 参数/4102 用户名租户内重复/4103 邮箱全局重复/4104 注册未开放/4105 限流）
- [ ] T030 [US2] 注册端点：`internal/controller/tenant.go` `POST /api/tenant/register`（contracts/api.md 契约）+ 复用 ratelimit 基建防滥用 + on/off 门（off 恒 4104）；`internal/router/` 注册映射
- [ ] T031 [US2] 登录扩展：email 全局唯一登录通道（username 跨租户歧义时提示改用邮箱）；登录响应/JWT 增加 tenant_id/tenant_code；off 模式登录行为不变（存量前端零适配）
- [ ] T032 [US2] 前端注册页：`user-web/src/views/tenant/Register.vue` + `src/api/tenant.js` + `src/router/modules/` 路由（SaaS 显/off 隐）+ i18n 四语言包同步
- [ ] T033 [US2] 租户运营端点：`GET /api/manage/tenants`（分页 page/page_size/status/keyword，list+total）+ `PATCH /api/manage/tenants/:id/status`（停用→登录受限/后台跳过/数据保留）；admin 权限挂载
- [ ] T034 [US2] 注册链路测试：成功（租户+管理员+默认配置齐活）/参数错/用户名租户内重/邮箱全局重/限流/off 4104；vitest 注册页表单校验

**Checkpoint**: 开户可独立验收（quickstart 场景 B 前两步留证）

---

## Phase 6: User Story 4 - LLM 双轨 (Priority: P2)

**Goal**: 新租户零配置用平台池；自带模型即时生效；故障回退；双租户互不影响

**Independent Test**: 零配置 AI 会话成功 → 配自有 mock 命中 mock → 删配置回退平台池

- [ ] T035 [US4] provider 租户化：`llm_providers` 加列迁移（tenant_id default 0=平台池）；`internal/aiagent/llm/dispatcher_persistence.go` 加载改为按 ctx 租户 → 平台池兜底；调用侧（agent/rag/意图识别）ctx 贯通
- [ ] T036 [US4] 平台池 seed 与继承：SaaS 模式启动 seed 平台池 provider（env 驱动，零出域不破坏）；新租户默认继承（无自有配置即平台池）；off 模式全部走 tenant_id=0 等价现状
- [ ] T037 [US4] LLM 双轨测试：零配置平台池命中/自有配置 100% 指向 mock/删除回退/双租户并发互不影响；熔断降级不跨租户

**Checkpoint**: LLM 双轨可独立验收（quickstart 场景 D 留证）

---

## Phase 7: User Story 5 - 租户内管理与存量一致 (Priority: P2)

**Goal**: SaaS 租户管理员与离线版管理员同套管理界面同逻辑，边界限定本租户

**Independent Test**: 两形态执行同一组管理操作，行为一致（仅数据边界差异）

- [ ] T038 [US5] system_user 域租户化收口：成员 CRUD/角色/数据范围分配全部限定本租户；off 模式行为不变；与 T009 JWT tenant claim 闭环
- [ ] T039 [US5] 前端租户上下文：`user-web` 用户态携带 tenant 信息、SaaS 显示租户标识、off 隐藏全部租户 UI；i18n 四语言
- [ ] T040 [US5] 管理一致性测试：双形态同一组管理操作对比断言（创建成员/分配角色/改参数）；租户内成员隔离边界测试

**Checkpoint**: 管理一致性可独立验收

---

## Phase 8: Polish & Cross-Cutting

- [ ] T041 [P] 文档：`docs/architecture/SaaS_MULTI_TENANT.md`（架构决策+租户解析链+运维手册：双形态部署/升级/回滚）+ README 双形态说明 + ADR 记录 D1-D10
- [ ] T042 全量验收：`make audit` + `make lint` + `make vet` + `make test-go` 全绿；`quickstart.md` 场景 A-F 逐条执行留证；`scripts/api_verify_full.py` 三端验证（注册/登录/租户信息/运营端点 + 抽查受隔离影响的既有端点）
- [ ] T043 converge 收敛：speckit-converge 复盘 tasks 遗漏项，追加并回实现直至 Converged

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

## Notes

- [P] = 人工并行标注；本仓禁子 Agent，实际由主 Agent 串行执行
- off 模式等价现状是贯穿性硬约束：任何任务若破坏 off 回归（T018）立即当轮修复
- 契约细节以 contracts/api.md 为准；表分类以 data-model.md 为准；决策依据以 research.md 为准
