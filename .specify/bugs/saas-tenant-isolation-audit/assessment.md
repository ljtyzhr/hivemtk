# Bug Assessment: SaaS 多租户隔离对抗审计（D1/D2/D3 + 门禁与文档漂移）

- **Slug**: saas-tenant-isolation-audit
- **Created**: 2026-10-20
- **Source**: pasted text（用户指令「前置任务一」第 2、3 段：多角度评估审核 saas 多租户逻辑 / 对抗性检查审核测试整个 saas 多租户改造，发现问题立即修复）
- **Verdict**: valid
- **Severity**: high

## Report (verbatim or summarized)

用户指令（m00003 原文要点）：

> 前置任务一：1 拉取云端最新代码 合并到本地；2 开启头脑风暴 多角度评估审核本系统现有代码和结构 saas 多租户 逻辑 优化、论证 如何减少代码层面入侵、如何减少代码层面耦合；3 对抗性 检查、审核、测试 整个 saas多租户改造 发现问题立即修复

本轮交付前的现状：`TENANT_MODE=on` 隔离机制由 GORM callback 层（`user-server/internal/pkg/tenancyidx/tenant_callback.go`）实现，
既有 117 例常规测试全绿。多角度评估结论见 `docs/architecture/SAAS_MULTI_TENANT_EVALUATION_2026-10.md`。

## Symptom

在 `TENANT_MODE=on` + 真实 PostgreSQL 上用**对抗性写法**触发 GORM callback 时，出现三种**静默**越界：
写入落到错误租户（列默认值）、行的租户被改写成 0（行掉出本租户）、LEFT JOIN 退化成 INNER JOIN（未匹配父行被挤掉）。
三者都不报错、不 panic，正常用例全绿 ⇒ 属"测试覆盖不到的静默缺陷"。

## Reproduction

前置：`TENANT_MODE=on`、真实 PG（`user-server/internal/pkg/db` 包用 testutil 建进程级测试库）、探针表 `tenancy_probe` 带 `tenant_id` 列。

1. D1：`db.Create(&m).Omit("tenant_id")` 或 `db.Create(&m).Select("name")`
   → 生成 `INSERT INTO "tenancy_probe" ("name") VALUES ($1)`，**缺 `tenant_id` 列** ⇒ 行落列默认值 **1**（默认租户）。
   发起方 ctx 租户为 91 ⇒ 静默跨租户写。
2. D2：`db.Save(&m)` 且 `m.TenantID` 为**零值**（结构体未加载过）
   → 生成 `UPDATE "tenancy_probe" SET "tenant_id"=$1,"name"=$2 WHERE …`（`$1=0`）⇒ 行租户被覆写成 **0** ⇒ 行掉出本租户。
3. D3：`db.Joins("LEFT JOIN t AS x ON … WHERE …")`（join 串自带 WHERE）
   → 租户条件落进 **WHERE 侧**：`… LEFT JOIN t AS x ON x.id = … WHERE g.id = 1 AND "x"."tenant_id" = $1`
   ⇒ LEFT JOIN 退化为 INNER JOIN ⇒ **未匹配的父行被挤掉**（静默少数据）。

读回验证必须用**系统作用域**（`tenant.WithSystemScope`）；用租户作用域读回会把"行落到别的租户"伪装成 `record not found`（本轮第一版探针即踩此坑，产出假绿）。

## Suspected Code Paths

- `user-server/internal/pkg/tenancyidx/tenant_callback.go` — Create/Update 回调只对**结构体**判定"值已填入"，未对**最终进 SQL 的列集合**判定 ⇒ D1、D2。
- `user-server/internal/pkg/tenancyidx/tenant_callback.go`（join 处理分支）— 租户条件一律追加到 join 串末尾，未识别 join 串自带顶层 WHERE ⇒ D3。
- `user-server/internal/repository/scope/tenant.go` — 僵尸包：全仓无任何 import（`grep -rn "repository/scope" --include="*.go" user-server/` 为空），与 `tenancyidx` 职责重复 ⇒ 事实源漂移。
- `docs/architecture/SaaS_MULTI_TENANT.md:37` — 事实源漂移：路径写作 `pkg/db/tenant_callback.go`，实际实现已迁到 `pkg/tenancyidx/tenant_callback.go`。
- `scripts/check-env-coverage.py` — 配置面可发现性门用 `rglob` 全盘扫描且**无视 .gitignore**，把本地调试脚手架（`user-server/tmp/probe/fresh/main.go` 读 `PROBE_DSN`/`PROBE_DB`/`PROBE_ADMIN_DSN`）当生产代码判红 ⇒ `make audit` 红。

## Root Cause Hypothesis

三处同源（置信度：**high**）：**"回调把值填进结构体" ≠ "值进了 SQL"**。
GORM 的 `Selects` / `Omits` / `Save` 都会改变**最终进 SQL 的列集合**，而 callback 只看得到结构体。
⇒ 凡是要靠"值一定进了 SQL"做保证的地方，都必须**对最终的列集合做判定**，不能对结构体做判定。
D3 同源的另一面：注入点选错（WHERE 侧 vs ON 侧）时，条件"加上了"但语义被改写 ⇒ 必须按**语义位置**注入。

## Proposed Remediation

**Preferred**:

- D1：新增 `tenantIDWillBeInserted(stmt)`，按 GORM `ConvertToCreateValues`（gorm v1.31.2 `statement.go:715`）的**同式**判据 `(ok && v) || (!ok && !restricted)` 判定该列是否真的进 INSERT；不进而目标是租户表 ⇒ **fail-closed** 拒绝。
- D2：`guardUpdateTenantAssignments` 增判「本次 SET 一定会写 `tenant_id`」（`Selects` 含 `*` ⇒ 零值也写）⇒ 值不等于上下文租户即拒。
- D3：新增 `insertJoinOnCondition(joinSQL, cond)`，把条件插进 **ON 子句**（join 串自带顶层 WHERE 时插在 WHERE 之前）；只认括号深度 0 且不在字符串字面量里的顶层 WHERE（`topLevelKeywordIndex` / `isIdentChar`）；租户 id 取自 ctx 的 uint，**内联为整数字面量**，避开"往 join 串中间插占位符 ⇒ 破坏 `?` 与 args 位置对应"这个坑。
- 僵尸包：删除 `user-server/internal/repository/scope/`（无引用，删除面零风险）。
- 文档：修正 `docs/architecture/SaaS_MULTI_TENANT.md` 的实现路径漂移。
- 门禁：`scripts/check-env-coverage.py` 新增 `git_ignored(paths)`，用 `git check-ignore --stdin`（用 git 自己的忽略语义，不手写黑名单）排除被忽略路径。

**Alternatives** (optional):

- D1/D2 的另一条路是"要求业务侧禁止 `Omit("tenant_id")`/`Save` 零值"——把隔离责任推给调用方，与"框架层隔离、业务代码零手工隔离"的既有设计相悖，故不取。
- D3 的另一条路是改写整条 join 串为子查询——侵入面大、语义风险高，故不取。

**Files likely to change**:

- `user-server/internal/pkg/tenancyidx/tenant_callback.go`
- `user-server/internal/pkg/db/tenant_callback_adversarial_test.go`（新增）
- `user-server/internal/repository/scope/tenant.go`（删除）
- `user-server/internal/repository/scope/tenant_test.go`（删除）
- `docs/architecture/SaaS_MULTI_TENANT.md`
- `docs/architecture/SAAS_MULTI_TENANT_EVALUATION_2026-10.md`（新增）
- `scripts/check-env-coverage.py`

**Tests to add or update**:

- 对抗回归 10 例（D1×2、D2×1、D3×1 + 确认安全路径 6：子查询隔离 / `Unscoped` 不跳过 / `AllowGlobalUpdate` 仍带租户条件 / `UpdateColumn` 改 `tenant_id` 被拒 / 关联写入子行被填充 / `FirstOrCreate` 被填充）。

## Risks & Considerations

- **门禁性能**：`git_ignored` 必须**先算成变量**再过滤（写在列表推导的 `if` 条件里会让每个元素起一次 git 子进程 ⇒ 1943 次进程 ⇒ 门从 2.9s 变 6 分钟）。
- **既有 lint 债**：`make lint` 的 4 条红（`internal/middleware/rate_quota.go` unused `stop()`、`internal/bridge/sse.go:540` ineffassign、`internal/service/customer_session_cron.go` errcheck、`internal/service/reach_send_pipeline_quiethours.go:34` unused `nextDayFirstSendHour`）经 `git grep HEAD` 逐条核对**在 HEAD 上一模一样存在** ⇒ 本轮改动前就是红的，不属本任务范围（不为绿灯改别人的文件）。
- **环境失败 ≠ 代码 bug**：`SQLSTATE 28P01`（测试库口令）与 embedding 服务不可达（`dial tcp 127.0.0.1:8208: connect: connection refused`）都是环境依赖缺失，不得当缺陷去"修"。
- **SQL 注入面**：D3 注入的租户 id 来自 ctx 里的 `uint`，非外部输入，内联为整数字面量是安全的；`insertJoinOnCondition` 不接受外部字符串。
- **回归风险**：D3 改注入位置会改变既有 join 查询的 SQL 形态，需保证"未匹配父行保留"这一语义，并让既有 join 用例不退化。

## Open Questions

- [NEEDS CLARIFICATION: 方案 B 三阶段（B1 不可伪造平台作用域类型 / B2 82 处 `WithSystemScope` 调用点逐一定类 / B3 加门禁纳入 `make audit`）需要用户批准后才动结构；本轮只交付 D1/D2/D3 与门禁、文档修正。]
- [NEEDS CLARIFICATION: 82 处 `WithSystemScope` 调用点的定类工作量与排期，是否纳入下一轮。]
