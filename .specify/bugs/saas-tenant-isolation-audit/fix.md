# Fix Report: SaaS 多租户隔离对抗审计（D1/D2/D3 + 门禁与文档漂移）

- **Slug**: saas-tenant-isolation-audit
- **Fixed**: 2026-10-20
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

修复 `TENANT_MODE=on` 下 GORM callback 层的三处**静默**租户越界，并修掉随对抗检查一起暴露的门禁与文档漂移：

- **D1 缺列写入跨租户**：`Create` 时 `Omit("tenant_id")` / `Select("name")` 让 GORM 的 `ConvertToCreateValues` 不把 `tenant_id` 放进 INSERT，回调填进内存的值根本没进 SQL ⇒ 行落**列默认值 1（默认租户）**。现按 GORM 同式判据判定"本次 INSERT 是否真的带 `tenant_id` 列"，不带即**拒绝执行**。
- **D2 零值 Save 改写行租户**：`Save` 一个没加载过的结构体（`TenantID` 零值）时，GORM 的 `ConvertToAssignments` 仍会写 `tenant_id` 列（`Selects` 含 `*`），值 `0` 把行**移出本租户**。现增判"本次 SET 一定写 `tenant_id`"，值不等于上下文租户即拒。
- **D3 LEFT JOIN 退化为 INNER JOIN**：join 串自带顶层 WHERE 时，租户条件被追加到串末尾 ⇒ 落进 WHERE 侧 ⇒ 未匹配的父行被挤掉。现把条件插进 **ON 子句**（`insertJoinOnCondition`），并只认括号深度 0、不在字符串字面量里的顶层 WHERE。
- **门禁漂移**：`scripts/check-env-coverage.py` 用 `rglob` 全盘扫描且无视 `.gitignore`，把本地调试脚手架当生产代码判红 ⇒ `make audit` 红。
- **事实源漂移**：文档实现路径写的是 `pkg/db/tenant_callback.go`，实际已迁到 `pkg/tenancyidx/tenant_callback.go`；另有僵尸包 `internal/repository/scope/`（全仓无 import，与 `tenancyidx` 职责重复）。

## Changes

| 文件 | 改动 | 依据 |
|------|------|------|
| `user-server/internal/pkg/tenancyidx/tenant_callback.go` | 新增 `tenantIDWillBeInserted`（D1 闸）、`insertJoinOnCondition` + `topLevelKeywordIndex` + `isIdentChar`（D3 注入位置）、`guardUpdateTenantAssignments` 增判"本次 SET 一定写 tenant_id"（D2 闸） | assessment「Proposed Remediation」 |
| `user-server/internal/pkg/db/tenant_callback_adversarial_test.go` | 新增对抗回归 10 例（D1×2 / D2×1 / D3×1 / 安全路径×6） | assessment「Tests to add or update」 |
| `user-server/internal/repository/scope/tenant.go` | 删除（僵尸包：全仓无 import） | assessment「Suspected Code Paths」 |
| `user-server/internal/repository/scope/tenant_test.go` | 删除（同上） | 同上 |
| `docs/architecture/SaaS_MULTI_TENANT.md` | 实现路径漂移修正（`pkg/db/tenant_callback.go` → `pkg/tenancyidx/tenant_callback.go`） | assessment「Suspected Code Paths」 |
| `docs/architecture/SAAS_MULTI_TENANT_EVALUATION_2026-10.md` | 新增：多角度评估（机制/缺陷/侵入面/耦合/性能/事实源/方案矩阵/三阶段） | 用户指令第 2 段 |
| `scripts/check-env-coverage.py` | 新增 `git_ignored(paths)`，用 `git check-ignore --stdin` 排除被忽略路径 | assessment「Proposed Remediation」 |

## Diff Highlights

`user-server/internal/pkg/tenancyidx/tenant_callback.go`：

```go
// D1：INSERT 必须真的带上 tenant_id 列。
if isTenant && !tenantIDWillBeInserted(stmt) {
    tenantGuardFail(db, table, "create：本次 INSERT 不含 tenant_id 列（Omit/Select 掉了），"+
        "行会落列默认值 1（默认租户）= 跨租户写，拒绝执行")
    return
}

// 与 GORM ConvertToCreateValues 的入选判据同式：(ok && v) || (!ok && !restricted)
func tenantIDWillBeInserted(stmt *gorm.Statement) bool {
    if stmt == nil {
        return false
    }
    selected, restricted := stmt.SelectAndOmitColumns(true, false)
    v, ok := selected["tenant_id"]
    return (ok && v) || (!ok && !restricted)
}

// D3：条件必须落进 ON 子句；租户 id 取自 ctx 的 uint，内联成整数字面量。
j.Name = insertJoinOnCondition(j.Name,
    fmt.Sprintf(` AND %s.%s = %d`, quoteIdent(q), quoteIdent("tenant_id"), id))

func insertJoinOnCondition(joinSQL, cond string) string {
    idx := topLevelKeywordIndex(joinSQL, "where")
    if idx < 0 {
        return joinSQL + cond
    }
    return joinSQL[:idx] + cond + " " + joinSQL[idx:]
}

// D2：本次 SET 到底会不会写 tenant_id 列？
selected, restricted := stmt.SelectAndOmitColumns(false, true)
alwaysWritten, inMap := selected["tenant_id"]
switch {
case inMap && alwaysWritten:
case !inMap && !restricted:
    if !zero {
        if err := reject(v); err != nil {
            return err
        }
    }
}
```

`scripts/check-env-coverage.py`：

```python
def git_ignored(paths: list[str]) -> set[str]:
    """用 git 自己的忽略语义挑出被忽略路径（不手写黑名单）。"""
    ...
# 必须先算成变量再过滤：写在推导式的 if 条件里会让每个元素起一次 git 子进程
```

## Tests Added or Updated

- 新增 `user-server/internal/pkg/db/tenant_callback_adversarial_test.go`（10 例，对抗写法 + 安全路径回归）。
- 新增 `user-server/internal/service/translation/glossary_tenant_test.go`（4 例，术语缓存按租户隔离）。
- 未改任何既有测试。

## Local Verification

环境：真实 PG（容器 `mtk-postgres`，`127.0.0.1:8202`，库 `user_db`），口令运行时取值
`PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"`，
`export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"`。

### 1. 对抗回归（核心判定门）

```bash
cd user-server
go test ./internal/pkg/db/... -run 'TestTenantCallback_' -count=1 -v
```

**结果**：22 例全 PASS（RC=0）✅ —— 修复前 D1/D2/D3 三条对抗路径红。

### 2. 租户隔离（术语表层）

```bash
go test ./internal/service/translation/... -run 'TestGlossary' -count=1
```

**结果**：`ok hivemtk-user/internal/service/translation`（4 例）✅

### 3. 门禁：`make audit`

```bash
make audit
```

**结果**：通过（AUDIT_RC=0，46s）✅
`scripts/check-env-coverage.py` 新增 `git_ignored(paths)` 后，被 `.gitignore` 命中的本地脚手架不再被当生产代码判红；
生产键 207 → 204（排除 3 个 `PROBE_*` 本地键）。

### 4. 静态确认：僵尸包删除 + 文档路径漂移修正

```bash
cd user-server
grep -rn "repository/scope" --include=*.go . || echo "NO_IMPORT"
ls internal/repository/scope 2>&1
cd .. && grep -n "pkg/tenancyidx/tenant_callback.go" docs/architecture/SaaS_MULTI_TENANT.md
```

**结果**：全仓无 `repository/scope` import ✅；目录已删 ✅；文档路径已指向 `pkg/tenancyidx/tenant_callback.go` ✅

### 5. 编译与格式

```bash
cd user-server
go build ./... && go vet ./internal/pkg/tenancyidx/... ./internal/pkg/db/... \
  ./internal/service/... ./internal/service/translation/...
gofmt -l internal/pkg/tenancyidx internal/pkg/db
```

**结果**：`BUILD_OK`、`VET_OK`、`gofmt -l` 输出为空 ✅

### 6. 全量套件

```bash
cd user-server
go test ./... -count=1 -p 1 -timeout 900s
```

见 [test.md](test.md) 的 Test Results 表。

## Deviations from Assessment

- assessment 列出的 `user-server/internal/pkg/db/migrate.go`、`internal/model/glossary.go`、`internal/model/module_tables.go`、`internal/app/sales_engine_factory.go`、`cmd/api/main.go`、`scripts/check-i18n-coverage.py`、`scripts/check-deploy-claims.py`、`scripts/prod_e2e_verify.sh` 与 9 个语言包、3 个清单文档（`docs/router-list.md`、`go-files-list.md`、`user-server/docs/architecture/model-list.md`）**不在本轮 SaaS 隔离修复范围内**——它们是上一轮在途工作（i18n 门修正、语言包瘦身、租户模式装配）的产物，本轮只做"发现即修"时顺带核对了它们对 `make audit` 的影响，未改其语义。

## Follow-ups

- 方案 B 三阶段（B1 不可伪造平台作用域类型 / B2 82 处 `WithSystemScope` 调用点逐一定类 / B3 门禁纳入 `make audit`）需用户批准后才动结构，本轮未动。
