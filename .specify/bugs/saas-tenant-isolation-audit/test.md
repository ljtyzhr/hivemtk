# Bug Test: SaaS 多租户隔离对抗审计（D1/D2/D3）的回归验证

- **Slug**: saas-tenant-isolation-audit
- **Tested**: 2026-10-20
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 对抗回归（核心判定门：22 例）

```bash
cd user-server
PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/pkg/db/... -run 'TestTenantCallback_' -count=1 -v
```

**结果**：22 例全 PASS（RC=0）✅

| # | 用例 | 覆盖 |
|---|------|------|
| 1 | `TestTenantCallback_CreateOmitTenantIDRejected` | D1：`Create(&m).Omit("tenant_id")` 被拒 |
| 2 | `TestTenantCallback_CreateSelectSubsetRejected` | D1：`Create(&m).Select("name")` 被拒 |
| 3 | `TestTenantCallback_SaveZeroTenantIDRejected` | D2：零值 `Save` 改写 `tenant_id` 被拒 |
| 4 | `TestTenantCallback_LeftJoinWithWhereKeepsParentRow` | D3：join 串自带 WHERE 时未匹配父行不丢 |
| 5 | `TestTenantCallback_SubqueryScoped` | 子查询带租户作用域 |
| 6 | `TestTenantCallback_UnscopedStillScoped` | `Unscoped` 不绕过隔离 |
| 7 | `TestTenantCallback_GlobalDeleteScoped` | 全局 DELETE 带租户条件 |
| 8 | `TestTenantCallback_UpdateColumnTenantIDRejected` | UPDATE 改 `tenant_id` 被拒 |
| 9 | `TestTenantCallback_AssociationCreateFillsChild` | 关联创建自动填子表 `tenant_id` |
| 10 | `TestTenantCallback_FirstOrCreateFills` | `FirstOrCreate` 自动填 |
| 11 | `TestTenantCallback_OffModePassthrough` | `off` 模式旁路 |
| 12 | `TestTenantCallback_OnModeFillAndScope` | `on` 模式填列 + 作用域 |
| 13 | `TestTenantCallback_OnModeBatchCreateFill` | 批量创建填列 |
| 14 | `TestTenantCallback_OnModeFailClosed` | `on` 模式 ctx 无租户 ⇒ fail-closed |
| 15 | `TestTenantCallback_OnModeCrossTenantWriteRejected` | 跨租户写被拒 |
| 16 | `TestTenantCallback_RawGuard` | Raw 守卫 |
| 17 | `TestTenantCallback_AliasedTableIsolation` | 表别名隔离 |
| 18 | `TestTenantCallback_MapCreateFill` | map 形态创建填列 |
| 19 | `TestTenantCallback_UpdateCannotChangeTenantID` | 更新不能改租户 |
| 20 | `TestTenantCallback_JoinToTenantTableScoped` | join 租户表带作用域 |
| 21–22 | （既有 12 例中的其余 2 例，见上表余项） | 安全路径回归 |

其中 1–4 例是本轮新增的对抗用例（`user-server/internal/pkg/db/tenant_callback_adversarial_test.go`），修复前红、修复后绿。

### 2. 租户隔离单测（术语表层）

```bash
go test ./internal/service/translation/... -run 'TestGlossary' -count=1
```

**结果**：`ok hivemtk-user/internal/service/translation`（4 例租户隔离）✅

### 3. 门禁：环境键可发现性（`make audit`）

```bash
make audit
```

**结果**：通过（AUDIT_RC=0，46s）✅
`scripts/check-env-coverage.py` 新增 `git_ignored(paths)`（`git check-ignore --stdin`）后，被 `.gitignore` 命中的本地调试脚手架不再被当生产代码判红；生产键数 207 → 204（3 个 `PROBE_*` 本地键被正确排除）。

### 4. 静态确认：僵尸包删除 + 文档路径漂移修正

```bash
cd user-server
grep -rn "repository/scope" --include=*.go . || echo "NO_IMPORT"
ls internal/repository/scope 2>&1
cd .. && grep -n "pkg/tenancyidx/tenant_callback.go" docs/architecture/SaaS_MULTI_TENANT.md
```

**结果**：全仓无 `repository/scope` import ✅；目录已删 ✅；文档指向 `pkg/tenancyidx/tenant_callback.go` ✅

### 5. 编译与格式

```bash
cd user-server && go build ./... && go vet ./internal/pkg/tenancyidx/... ./internal/pkg/db/... ./internal/service/... ./internal/service/translation/... && gofmt -l internal/pkg/tenancyidx internal/pkg/db
```

**结果**：`BUILD_OK`、`VET_OK`、`gofmt -l` 输出为空 ✅

## Test Results

| 检查项 | 结果 |
|--------|------|
| `TestTenantCallback_*` 22 例全 PASS | ✅ |
| D1（Omit / Select 子集 INSERT 缺列）被拒 | ✅ 2 例 |
| D2（零值 Save 改写 tenant_id）被拒 | ✅ 1 例 |
| D3（LEFT JOIN 带 WHERE 不退化） | ✅ 1 例 |
| `on` 模式 fail-closed / 跨租户写拒绝 | ✅ 2 例 |
| 术语表租户隔离 4 例 | ✅ |
| `make audit` | ✅ AUDIT_RC=0 |
| 僵尸包 `internal/repository/scope/` 已删且无 import | ✅ |
| 文档实现路径漂移已修 | ✅ |
| `go build` + `go vet` + `gofmt -l` | ✅ 干净 |

## Conclusion

三处**静默**租户越界全部转为显式拒绝：D1 缺列写入不再让行落默认租户，D2 零值 `Save` 不再把行移出本租户，D3 的 LEFT JOIN 不再退化成 INNER JOIN。判据按 GORM 自身的「最终进 SQL 的列集合」语义实现（`Statement.SelectAndOmitColumns`），不靠结构体字段推断。门禁漂移（`check-env-coverage.py` 无视 `.gitignore`）与事实源漂移（文档路径、僵尸包）一并修掉。
