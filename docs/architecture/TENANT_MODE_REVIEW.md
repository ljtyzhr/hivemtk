# 多租户文档口径与授权策略复核（TENANT_MODE_REVIEW）

> 定位：C4 收口——核对 `specs/001-saas-multi-tenant` 与 `internal/pkg/tenant` 的实现，
> 找出「文档怎么说」与「代码怎么做」的出入，并复核 `WithSystemScope` 授权策略。
> 本文件为**新增**复核文档；对既有陈旧文档只登记勘误、不整写（避免与 lane 在途文件冲突）。
> 事实源：源码实读（2026-10-24）。

---

## 一、结论

**实现与 spec 的目标口径一致**：一套代码、`TENANT_MODE=off/on` 单开关、off 模式全部数据归
默认租户 1 且行为等价旧版、on 模式行级隔离 + fail-closed、后台任务按租户扇出、外部入口按
权威线索解析租户。**没有发现实现层面的口径错误**。

真正的问题是**四份早期文档还停留在「单商户时代」**（E4 硬约束、子索引头句、数据库深度解析、
ADR-014），与 001 的演进方向相反，属于**事实源漂移**——这正是本 lane 要修的。

---

## 二、实现口径核对表（代码 ↔ 文档）

| # | 口径项 | 代码事实 | 文档口径 | 结论 |
|---|---|---|---|---|
| 1 | 形态开关 | `tenant.EnvMode = "TENANT_MODE"`；`ResolveMode`: 空/未设/off/OFF → off，on/ON → on，**未知值 → off 并返回 error**，`InitFromEnv` 拼错即 `log.Fatalf` 拒启 | spec FR-001；plan「未知值 fail-safe 朝 off」；SAAS_PRODUCTION_CONSTRAINTS §3.2 | ✅ 一致 |
| 2 | 默认租户 | `DefaultTenantID uint = 1` | spec Assumptions「默认租户 ID 固定」；tasks T001「DEFAULT_TENANT_ID=1」 | ✅ 一致（命名 `DefaultTenantID`） |
| 3 | 隔离执行者 | `internal/pkg/db` 六类 GORM callback + `internal/pkg/tenancyidx` 表登记/索引复合化 | plan/T005/T006/T008 | ✅ 一致 |
| 4 | 入口绑定 | `middleware/tenant.go`：off 显式绑默认租户；on 无 claim → 401；JWT claim `tenant_id` | T009 | ✅ 一致 |
| 5 | 后台扇出 | `pkg/tenant/fanout.go` `Fanout`：off 单租户退化；on 枚举活跃租户逐租户回调，单租户失败出声不阻断 | research D6 写作 `TenantRunner`（`runner.go`） | ⚠️ **命名漂移**（见 §四-4） |
| 6 | Redis 键分域 | `keys.go` `RedisKey`：on + ctx 租户 → `t{id}:{key}`；off/无租户 → 裸键（键形逐字节一致） | T010 | ✅ 一致 |
| 7 | 行自证 | `lookup.go` `SingleTenantByKey`：系统作用域反查命中行租户，>1 租户 → `ErrAmbiguousTenantKey` 拒绝 | T008 唯一索引复合化、US3 行自证 | ✅ 一致 |
| 8 | 系统豁免 | `WithSystemScope` / `IsSystemScope`：migration/备份/健康检查唯一豁免通道 | tasks T001 写作 `SystemTenant` | ⚠️ **命名漂移**（见 §四-5） |
| 9 | 全局表豁免清单 | `tenancyidx` `tenant:"global"` tag：tenants/migration 记录/config_params/feature_flags/平台心跳/cron_job_leases/backups/dictionary 等 12 张 | data-model.md 全局表清单 | ✅ 一致 |
| 10 | 注册两路径 | `POST /api/tenant/register`（新开户）vs `POST /api/auth/register`（凭租户短码加入已有商户） | SAAS_PRODUCTION_CONSTRAINTS §2.2b | ✅ 一致 |

---

## 三、授权策略复核

### 3.1 谁能调 `WithSystemScope`（豁免通道）

代码里非测试调用约 **95 处**，可归为三类，口径如下：

| 允许（系统语义） | 代码证据 | 禁止（业务请求路径） |
|---|---|---|
| 装配层 / 启动期：`cmd/api/main.go` 迁移 ctx、`tenancyidx.Normalize` 内省重建索引 | main.go:448、tenancyidx.go | 任何「随 HTTP 请求进来」的业务 service 不得自行 `WithSystemScope` 去读全量 |
| 平台级扫描 / 定时对账：`geo/service/job_manager.go` 清僵尸运行、`search_probe.go`、`ops/repository/dashboard_screen.go` | job_manager.go:240 | 上表三类之外的 repository/service 私有豁免属「能力耦合」残留，见 §五 |
| 无请求上下文的凭证反查：`email_tracking.go`、`password_reset_token.go`、`short_link.go`、`sms_tracking.go`、`knowledge_merchant_token.go`（回调按 token claim 恢复租户前，须以系统作用域先反查到行） | email_tracking.go:54 | 反查到行后必须立刻 `WithTenantID` 收窄，不得带着 SystemScope 继续写业务 |

### 3.2 `WithTenantID` 的 id=0 语义

`WithTenantID(ctx, 0)` **返回原 ctx（无效绑定）**——0 在数据库列语义是「平台公共池」，永远不是上下文租户。
`FromContext` 读到 0 视为无绑定。on 模式下无绑定由 GORM callback fail-closed（报「需要租户上下文」），
**绝不回落默认租户**（回落 = 静默串库）。`OnlyTenant(ctx, 0)` 同样视为无绑定。

### 3.3 fail-closed 三条判据

1. **读**：on 模式租户表查询缺租户 ctx → callback 拒绝（含表名日志）。
2. **写**：`WriteTenantID` 在 on 模式无绑定时返回 0，调用方必须丢弃并出声。
3. **后台**：`Fanout` 枚举器未装配 → 拒绝执行；`BackgroundWithTenant` 在 on 模式无绑定保持无绑定，由 callback 兜底。
   2026-10-22 修：`BackgroundWithTenant` 必须一并保留 `SystemScope`，否则 SSE 出站认领（要 UPDATE message_hub）被拒。

---

## 四、文档口径勘误清单

| # | 文件 | 陈旧表述 | 事实 | 处置 |
|---|---|---|---|---|
| 1 | `hivemtk/docs/DEV_DOCS_INDEX.md` E4 | 「私域数据隔离：独立部署单实例数据物理隔离，**业务表无需多租户字段**」 | 001 后 245 张业务表全部带 `tenant_id`（`ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1`），仅 12 张全局表豁免；隔离为「共享库 + 行级 tenant_id」 | **已修**（本 lane，E4 表述 + 权威来源挂本文件） |
| 2 | `hivemtk/docs/INDEX.md` 头句 | 「单商户本地部署（私域模式）」 | 现为双形态：off（默认，单租户私域）/ on（SaaS 多租户） | **已修**（本 lane） |
| 3 | `hivemtk/docs/architecture/DATABASE_SCHEMA_DEEP_DIVE.md` §3.1 / §X3 / 行 2161 | 「单租户：无 merchant_id 字段」「X3 单商户 ⇒ 无租户列」 | 001 已给全部业务表加 `tenant_id` 并复合化唯一索引（≠ 早期 merchant_id 方案，两套历史互不冲突，但「无租户列」结论已失效） | **只登记**（本文件即勘误记录；该大体量文件可能 lane 在途，不整写） |
| 4 | `specs/001-saas-multi-tenant/research.md` D6 | 设计组件名 `TenantRunner`（`runner.go`） | 实现为 `pkg/tenant/fanout.go` 的 `Fanout` | 记录漂移，不追改历史 spec；后续引用以代码名为准 |
| 5 | `specs/001-saas-multi-tenant/tasks.md` T001 | `SystemTenant` 豁免标记、`DEFAULT_TENANT_ID=1` 常量 | 实现为 `WithSystemScope/IsSystemScope`、`DefaultTenantID` | 同上（命名漂移，不影响语义） |

---

## 五、遗留与建议（授权策略侧）

1. **`WithSystemScope` 是运行时标志、任何层都能调**（约 95 处非测试调用）。这是
   [SAAS_MULTI_TENANT_EVALUATION_2026-10.md](SAAS_MULTI_TENANT_EVALUATION_2026-10.md) §8 已论证的
   「能力耦合」问题，方案待批准后走 spec-kit 施工，本 lane 不重复展开。
2. **事件总线「发布点必须填 TenantID」无门禁**（SAAS_FULLCHAIN_AUDIT_2026-10-23.md P2-5）：
   漏填 → 订阅者拿 0 → fail-closed 出声，但不静默；建议补一条静态检查。
3. **越权回归仍缺「全 API 域逐端点双租户交叉扫描」**（tasks T028 注）：现状为逐域代表性用例，
   建议列入后续审计二期。
4. 索引复合化后仍有 **11 张租户表的索引族不以 tenant_id 打头**（P2-2），查询模式需复核是否命中复合索引。

## 六、相关文档

| 文档 | 关系 |
|---|---|
| [`SaaS_MULTI_TENANT.md`](SaaS_MULTI_TENANT.md) | 落地版架构与运维（权威） |
| [`../handbook/SAAS_PRODUCTION_CONSTRAINTS.md`](../handbook/SAAS_PRODUCTION_CONSTRAINTS.md) | 生产联调约束与教训（每条对应一次真实翻车） |
| [`../../specs/001-saas-multi-tenant/`](../../specs/001-saas-multi-tenant/) | spec 施工源 |
| [`SAAS_MULTI_TENANT_EVALUATION_2026-10.md`](SAAS_MULTI_TENANT_EVALUATION_2026-10.md) | 「降侵入/降耦合」评估 |
| [`DEPLOYMENT_MODES_EVALUATION_2026-10.md`](DEPLOYMENT_MODES_EVALUATION_2026-10.md) | 部署形态 2×2 矩阵 |
