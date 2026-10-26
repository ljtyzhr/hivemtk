# SaaS 多租户改造：多角度评估与「降侵入 / 降耦合」论证

> 日期：2026-10-20 ｜ 范围：`user-server` 全量 Go 代码 + 隔离机制本体（`pkg/tenant`、`pkg/tenancyidx`）
> 事实源：`specs/001-saas-multi-tenant/`、`docs/architecture/SaaS_MULTI_TENANT.md`
> 性质：**评估与提案**。第 8 节的方案需批准后走 spec-kit 施工；第 2 节的缺陷本轮已闭环。

---

## 0. 一句话结论

**隔离机制本体是对的**——它把租户语义吃进了框架层，业务代码确实做到了零手工隔离。本轮真正的问题
不是"设计错了"，而是三类：

| # | 问题 | 性质 | 状态 |
|---|---|---|---|
| 1 | 三处**边界漏洞**（对抗性探针打出） | 正确性 | **已修 + 已加常驻回归** |
| 2 | `tenant.WithSystemScope` 是**运行时标志**，任何层都能调 | 耦合 | 待批准（第 8 节） |
| 3 | 两处**事实源漂移**（文档路径、僵尸包） | 可维护性 | 已修 |

**"降侵入"的空间已经不大**（§3 有量化：真正冗余的隔离代码 ≈ 0–3 处），
**"降耦合"的重点应放在"能力耦合"**——即"谁能豁免隔离"这件事，目前靠约定而不是靠类型。

---

## 1. 隔离机制本体：做对了什么

这些是**评估时确认的优点**，后面的建议不该推翻它们：

| 优点 | 证据 |
|---|---|
| **业务零侵入** | 租户过滤/填充/拒绝全在 GORM callback（`pkg/tenancyidx/tenant_callback.go`）里；业务代码搜不到任何 `tenant_id` 拼接 |
| **单一判据** | 「这张表是不是租户表」只看**表上有没有 `tenant_id` 列**（库内省），没有第二份豁免清单 |
| **fail-closed 贯穿到底** | ctx 缺租户 ⇒ 报错；登记表未装配 ⇒ 报错；`TENANT_MODE` 拼错 ⇒ 拒绝启动（`main.go` 已补） |
| **双形态等价** | off 模式 callback 直通，SQL 与历史版本等价 |
| **唯一性租户化** | `tenancyidx.Normalize` 把唯一索引/约束原位复合为 `(tenant_id, ...)`，索引名稳定、幂等可重入 |
| **漏列有反向不变量兜底** | `TestTenantRegistry_ModelDeclarations` 要求「未声明 `TenantID` 的模型**必须**在全局豁免清单里」——新增模型漏写 `TenantID` 会红，不会静默落默认租户 |
| **数据访问单一路径** | `check-gorm-only` 门禁把裸 SQL 归零，隔离只需在一个地方正确 |

**一句话**：这套机制把"租户"从一个**散落在业务代码里的参数**，变成了**数据库里的一列 + 框架层的一条规则**。
这是正确的方向，本文的建议是在这个方向上**收口**，不是推倒。

---

## 2. 角度一 · 正确性：对抗性审计打出的三处真缺陷

### 2.1 方法

常规测试（117 例，全绿）覆盖的是"正常写法"。本轮改用**对抗性探针**：主动构造**会踩坑的 GORM 写法**，
在 `TENANT_MODE=on` + 真实 PG 上执行，然后**用系统作用域读回**核对真实落库租户。

> 关键方法论：读回**必须**用 `tenant.WithSystemScope`。用租户作用域读回会把"行落到别的租户"
> 伪装成 `record not found`——缺陷会以**假绿**通过。（本轮第一版探针就踩了这个坑，
> P4 第一次跑出 "record not found"，差点被当成通过。）

### 2.2 三处缺陷

| # | 触发写法 | 修复前生成的 SQL | 后果 | 修法 |
|---|---|---|---|---|
| **D1** | `Create(&m).Omit("tenant_id")`<br>`Create(&m).Select("name")` | `INSERT INTO "tenancy_probe" ("name") VALUES ($1)` | INSERT **缺 `tenant_id` 列** ⇒ 行落**列默认值 1（默认租户）** = 静默跨租户写 | 新增 `tenantIDWillBeInserted()`：按 GORM `ConvertToCreateValues` 的**同式**判据 `(ok && v) \|\| (!ok && !restricted)` 判断该列是否进 INSERT；不进而目标是租户表 ⇒ fail-closed |
| **D2** | `Save(&m)`，而 `m.TenantID` 是**零值**（结构体没加载过） | `UPDATE "tenancy_probe" SET "tenant_id"=$1,"name"=$2 WHERE ...`（`$1=0`） | 行的租户被覆写成 **0** ⇒ **行掉出本租户** | `guardUpdateTenantAssignments` 增判「本次 SET 一定会写 `tenant_id`」（`Selects` 含 `*` ⇒ 零值也写）⇒ 值不等于上下文租户即拒 |
| **D3** | `Joins("LEFT JOIN t AS x ON … WHERE …")`（join 串自带 WHERE） | `… LEFT JOIN t AS x ON x.id = … WHERE g.id = 1 AND "x"."tenant_id" = $1` | 租户条件落进 **WHERE 侧** ⇒ LEFT JOIN 退化成 INNER JOIN ⇒ **未匹配的父行被挤掉**（静默少数据） | 新增 `insertJoinOnCondition()`：把条件插进 **ON 子句**（有顶层 WHERE 时插在 WHERE 之前）；租户 id 是 ctx 里的 uint，直接内联为整数字面量，避开"往 join 串中间插占位符 ⇒ 破坏 `?` 与 args 位置对应"这个坑 |

### 2.3 顺带打过的路径（确认安全，留作回归）

子查询（`Where("id IN (?)", sub)`）被隔离 ✅ ｜ `Unscoped()` 不跳过隔离 ✅ ｜
`AllowGlobalUpdate` 的全局删除仍带租户条件 ✅ ｜ `Update/UpdateColumn` 改写 `tenant_id` 被拒 ✅
｜ 关联写入（父带子 `Create`）子行 `tenant_id` 被填充 ✅ ｜ `FirstOrCreate` 新建行被填充 ✅

### 2.4 回归落地

新增 `user-server/internal/pkg/db/tenant_callback_adversarial_test.go`（10 例，D1/D2/D3 各有专门用例 +
上述"确认安全"路径），与既有 `tenant_callback_test.go` 同目录同包。

### 2.5 根因归纳（三处同源）

三处缺陷都来自同一个认知偏差：**"回调把值填进结构体" ≠ "值进了 SQL"**。
GORM 的 `Selects`/`Omits`/`Save` 都会改变**最终进 SQL 的列集合**，而回调只看得到结构体。
⇒ 推论：凡是要靠"值一定进了 SQL"做保证的地方，都必须**对最终的列集合做判定**，不能对结构体做判定。

---

## 3. 角度二 · 侵入面：量化盘点（含一个被推翻的误指标）

### 3.1 盘点结果

| 侵入点 | 数量 | 性质 |
|---|---|---|
| `tenant.WithSystemScope(...)` 生产调用 | **82** | 平台豁免（见 §4-ii） |
| 业务代码手动补写 `TenantID` | **1**（`repository/system_config.go:63`） | 机制把负担推给业务的**典型样本** |
| `WHERE ... tenant_id ...` 手写过滤 | **18** | 其中 ≥15 处是**按指定租户操作**的平台语义（开户、租户覆盖配置、跨租户统计），**不是冗余** |
| `tenant_id` 出现在**索引/列声明** | 31 | 必要（复合索引必须以 `tenant_id` 打头） |
| 模型声明 `TenantID` | **151** 个模型文件 | 必要 |
| 库内已租户化的表 | **349 / 366** | 17 张未租户化，全部是平台/系统表（`tenants`、`config_params`、`feature_flags`、`cron_job_leases`、备份/迁移记录…） |

### 3.2 被推翻的误指标（重要）

初盘时用"源码里出现 `tenant_id` 的行数"当侵入面指标，得出 **66 处手写过滤**。
**这个指标是错的**：那 66 处里绝大多数是**索引声明**（`Columns: []clause.Column{{Name: "tenant_id"}, …}`），
不是过滤。真正落在 `Where(...)` 里的只有 **18 处**，且大部分是"针对某个**指定**租户"的平台语义
（`config_param_tenant.go` 7 处、`tenant_starter_kit.go` 4 处、`system_user.go` 2 处）。

> **方法论教训**：评估"侵入面"必须区分**同名不同义**的用法（索引列 / 过滤条件 / 归属断言），
> 否则会得出"侵入很重"的错误结论，进而推动不必要的重构。**先推翻自己的指标，再谈优化。**

### 3.3 结论

**业务侧隔离侵入已经接近下限**。继续把隔离逻辑往回调里塞（比如做"租户作用域仓储装饰器"），
收益是"看起来更整齐"，代价是 157 个域的改造量与回归面，**性价比不成立**。

---

## 4. 角度三 · 耦合面：三种耦合要分开处理

耦合不是一种东西。按**失效后果**排三档：

### (i) 数据访问耦合（business → `tenant_id` 语义）—— **已基本消除**

- 表现：业务代码自己拼 `tenant_id` 条件、自己填 `tenant_id`。
- 现状：过滤已由 callback 注入；剩下的 18 处手写过滤是"按指定租户操作"的**平台语义**，
  不是重复隔离。**这一档不需要动作。**

### (ii) 能力耦合（谁能豁免隔离）—— **本仓最该收口的一档**

- 表现：`tenant.WithSystemScope(ctx)` 是一个**任何包都能调**的函数。
- 现状：**82 处生产调用**，分布：`service` 30、`repository` 16、`pkg` 9、`geo` 8、
  `browser_automation` 5、`router` **3**、`migration` 3、`aiagent` 3、`cmd/api` 2、`platform`/`ops`/`controller` 各 1。
- **为什么它是真问题**：豁免通道一旦用错，失效模式是**静默跨租户**——不报错、不告警、
  `make test-go` 全绿。router 3 处 / controller 1 处尤其扎眼：
  **业务层的请求处理链路持有平台级能力**。
- 现状的缓解：`WithSystemScope` 目前只做"放行查询/跳过填充"，不做"改写归属"，
  真正的越权面是"看到别的租户的数据"，不是"写到别的租户"（写入侧另有守卫）。
  但这是**当前恰好如此**，不是**结构上保证**。

### (iii) 机制耦合（租户概念散落在几处）—— **已基本收敛**

- `pkg/tenant`（4 个文件 850 行）承担 5 件事：Mode 解析、ctx 绑定、SystemScope 豁免、
  后台扇出（`fanout.go`）、归属解析（`lookup.go`/`keys.go`）。**内聚尚可**，暂不动。
- `pkg/tenancyidx` 同时承担**索引规范化** + **隔离 callback** + **登记表**三件事——
  职责偏多，但依赖只有 `gorm` + `tenant`，**无环、可测**，拆分收益小于风险。
- **`internal/repository/scope` 是僵尸包**（已删）：6 个导出 helper、**全仓 0 importer**，
  按 `owner_id`/`uid` 做隔离——与真实的 `tenant_id` 隔离是**两套并行机制**（156 个 model 用 `TenantID`，
  只有 6 个用 `OwnerID`）。它的文件头还自称"数据隔离维度"，**会误导后来人以为它才是租户隔离**。
  ⇒ 已删（`git rm`），这是"降耦合"里唯一**零风险、立即见效**的一项。

---

## 5. 角度四 · 性能

| 维度 | 现状 | 判断 |
|---|---|---|
| 每次查询开销 | callback 多注入一条 `WHERE tenant_id = ?`；登记表是 `map` 查表，O(1) | 可忽略 |
| 索引可用性 | 唯一索引已复合为 `(tenant_id, 原列…)` ⇒ 租户过滤走**索引前缀** | 良好；租户维度过滤反而比全表扫更快 |
| 非租户表 | 17 张平台表不注入条件 | 良好 |
| 回调热点 | 每次 Query/Update/Delete/Row/Create 都要过 6 个 callback 之一 | 可接受；未做 profile，**不建议在无数据前优化** |
| 写放大 | Create 多一次 `field.Set`（反射） | 可忽略 |

**结论**：性能不是当前矛盾，不构成重构理由。

---

## 6. 角度五 · 可运维性与事实源漂移

| 问题 | 证据 | 状态 |
|---|---|---|
| 文档路径漂移 | `docs/architecture/SaaS_MULTI_TENANT.md` §三 写 callback 在 `pkg/db/tenant_callback.go`，实际在 `pkg/tenancyidx/tenant_callback.go` | 待修（小） |
| 配置表归属表未随迁移更新 | 本地库 366 张表里 `system_config_kv`/`config_params` 仍无 `tenant_id`（本地库停在 R5 迁移 v3.65.0 之前），而文档 §三.5 已按 R5 口径描述 | 文档为准；**模型侧才是判据**（`TenantID` 声明），库列由迁移补齐 |
| 豁免清单是"第二份真相" | 全局豁免表同时存在于：模型 Tag、迁移、`TestTenantRegistry_ModelDeclarations` 的 `globalTables`、文档 §三.5 | 已有反向不变量测试兜底，**暂不改** |

---

## 7. 降侵入 / 降耦合的四个方案（取舍矩阵）

| 方案 | 侵入面 | 耦合面 | 成本 | 风险 | 判断 |
|---|---|---|---|---|---|
| **A. Postgres RLS**（隔离下沉到 DB，`ENABLE ROW LEVEL SECURITY` + `current_setting('app.tenant_id')`） | **降到 0**：业务代码彻底不知道租户，连 ctx 绑定都不需要 | 与 PG 特性绑定，离线形态要靠 `current_setting` 默认值兜 | **高**：349 张表全部改策略 + 迁移 | **高**（见下） | ❌ 本轮不做 |
| **B. 能力类型化**（把 `WithSystemScope` 换成不可伪造的 `platform.Scope` 类型，只有平台包能构造，DI 发放） | 业务侧**减少**（82 处收敛） | **把"运行时约定"换成"编译期约束"** | 中：82 处调用点 + 一个新包 | 低（行为等价） | ✅ **推荐主线** |
| **C. 租户作用域仓储装饰器**（`repo.ForTenant(ctx)`） | 业务侧减少（ctx 传递 + 过滤显式化） | Repository 与 tenant 强绑定 | **高**：157 个域 | 中 | ❌ 性价比不成立（§3.3） |
| **D. ctx 类型化**（业务层拿 `tenant.Ctx` 而非裸 `context.Context`） | 中性 | 编译期可见"这个 ctx 带不带租户" | 中高：入口/出口签名改动面大 | 中 | ⏸ 备选，等 B 落地后再评估 |

### 为什么 A（RLS）现在不做——三条硬理由

1. **连接池与事务语义冲突**：RLS 需要每条连接知道当前租户（`SET LOCAL` 必须在事务内）。
   本仓**大量写路径不开事务** ⇒ 要么给每条路径套事务（改锁行为、改错误语义），
   要么用 `SET` + 归还前还原（连接池归还时机不可控）。
2. **表 owner 默认绕过 RLS**：要真正生效必须 `FORCE ROW LEVEL SECURITY` 或改用非 owner 角色，
   这会牵动迁移/备份/运维账号体系。
3. **存量数据形态不干净**：`tenant_id=0`（平台公共池）与 `tenant_id=1`（默认租户）并存，
   策略要与之对齐，而"哪些行该是 0"本身还没有一张权威表。

**但 A 是"侵入面"的理论下限**——如果未来要重新评估，它是唯一能把业务侧租户语义归零的路径。

---

## 8. 推荐：方案 B 的最小切片（分阶段，每阶段可独立验证）

| 阶段 | 内容 | 验收 |
|---|---|---|
| **B1** | 新增 `internal/platform/scope`（或等价包）：定义**不可伪造**的平台作用域类型（unexported 字段 + 构造只在平台包内）；`tenant.WithSystemScope` 降级为**仅平台包可调用**（把 `WithSystemScope` 移出 `pkg/tenant`，或用 lint 护栏禁止业务包 import） | `make lint` 护栏生效；82 处调用点逐一迁移 |
| **B2** | 把 82 处调用点按"平台语义"归类：真平台操作（migration/备份/健康检查/跨租户运营）留在平台层；**误用**（router 3 / controller 1 / service 里非平台语义的）改走正确路径 | 每处调用点有归类结论；`WithSystemScope` 生产调用数显著下降 |
| **B3** | 加**门禁**：静态检查禁止 `internal/{router,controller,service}` 直接调用豁免通道（仿 `check-gorm-only` 的形态） | 门禁纳入 `make audit` |

**为什么先做 B 而不是 A/D**：B 的失效模式从"静默串数据"变成"编译不过"，**成本可控、行为等价、可分阶段**；
A 要动 349 张表 + 事务语义；D 要动所有入口出口签名。C 不解决任何真问题（§3.3）。

---

## 9. 本轮已交付 vs 待批准

### 已交付（本轮闭环，已进回归）

| 项 | 位置 |
|---|---|
| D1/D2/D3 三处缺陷修复 | `user-server/internal/pkg/tenancyidx/tenant_callback.go` |
| 对抗性回归用例 10 例 | `user-server/internal/pkg/db/tenant_callback_adversarial_test.go` |
| 僵尸包清除 | 删除 `user-server/internal/repository/scope/`（0 importer） |
| 文档路径漂移 | `docs/architecture/SaaS_MULTI_TENANT.md` §三 callback 路径改为 `pkg/tenancyidx/tenant_callback.go` |
| 门禁口径修正 | `scripts/check-env-coverage.py`：扫描面排除 **gitignore 命中的本地文件**（此前把本地脚手架 `user-server/tmp/probe/fresh/main.go` 的 `PROBE_*` 三个键当生产代码判红） |

### 待批准（本文第 8 节）

方案 B 的三个阶段。**未批准前不动结构**——按仓规 3（spec-kit）走 spec → plan → tasks → implement → converge。

---

## 10. 附：本轮执行的命令与判定口径

```bash
# 测试库凭据（本地容器真值与仓库 .env 占位符不一致，见下）
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_USER=admin \
       POSTGRES_TEST_PASSWORD=<容器 mtk-postgres 的 POSTGRES_PASSWORD>

cd user-server
go build ./...                      # 通过
go vet ./internal/pkg/tenancyidx/... ./internal/pkg/db/...   # 通过
go test ./internal/pkg/db/... -run 'TestTenantCallback_'     # 22 passed
go test ./...                       # 全量套件
```

**环境注记**：仓库 `.env` 的 `POSTGRES_PASSWORD` 是占位符 `<见本机 .env 的 POSTGRES_PASSWORD>!`，
与本机容器 `mtk-postgres` 的实际口令**不一致**；直接按 `.env` 导出变量跑测试会得到
`SQLSTATE 28P01` 全量红——那是**环境失败不是代码 bug**（与 CLAUDE.md 3.5 同一类坑，只是这次是
**密码值**不一致而非变量名不一致）。
