# Bug Test: 跨租户管理端点越权（租户 admin 可操作其他租户与实例级端点）

- **Slug**: cross-tenant-admin-privilege-escalation
- **Tested**: 2026-10-21
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 编译 / vet / gofmt

```bash
cd user-server
go build ./...                                   # BUILD_OK
go vet ./internal/middleware/... ./internal/repository/... \
        ./internal/service/... ./internal/router/...   # VET_OK
gofmt -l internal/middleware internal/repository internal/service internal/router
# 输出为空 ✅
```

**结果**：全部干净 ✅

### 2. 守卫单测（6 例）

```bash
go test ./internal/middleware/... -run 'TestPlatformOperator' -count=1 -v
```

**结果**：6 例全 PASS ✅

| # | 用例 | 覆盖 |
|---|------|------|
| 1 | `TestPlatformOperatorMiddlewareRejectsNonAdminRole` | 非 admin 角色（user/staff/customer_service/空）⇒ 401/403 |
| 2 | `TestPlatformOperatorMiddlewareRejectsNonFirstTenant` | admin 但非首租户 ⇒ 403 |
| 3 | `TestPlatformOperatorMiddlewareAllowsFirstTenant` | admin 且是首租户 ⇒ 放行 |
| 4 | `TestPlatformOperatorMiddlewareRejectsWhenUnresolved` | 判定不出首租户 ⇒ 500 拒绝（不放行） |
| 5 | `TestPlatformOperatorMiddlewareRejectsMissingTenantBinding` | on 模式 ctx 无租户绑定 ⇒ 拒绝 |
| 6 | `TestPlatformOperatorMiddlewareOfflineModeAlwaysAllows` | **off 模式一律放行**（离线单机零回归） |

### 3. 首租户保护单测（2 例，含真库）

```bash
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/service/... -run 'TestUpdateTenantStatus' -count=1 -v
```

**结果**：2 例全 PASS ✅

| # | 用例 | 覆盖 |
|---|------|------|
| 7 | `TestUpdateTenantStatusRejectsDisablingFirstTenant` | 首租户停用被拒 **且库内状态未变**（不只断言错误码）＋ 普通租户仍可正常停用 |
| 8 | `TestUpdateTenantStatusAllowsReenablingFirstTenant` | 首租户**重新启用**必须允许（否则一旦停用再也救不回来） |

### 4. 活体对抗验证（形态③：`TENANT_MODE=on` + `PLATFORM_ENABLED=false`，真实 8204）

**修复前**（租户 6 的 admin 令牌）：

```
GET   /api/manage/tenants            ⇒ 200，total=6（看到全部租户）
PATCH /api/manage/tenants/5/status   ⇒ 200，{"status":2,"tenant_id":5}（停用了租户 5）
POST  /api/system/restart             ⇒ 200
```

**修复后**（同一令牌）：

```
GET   /api/manage/tenants            ⇒ 403 {"code":403,"message":"权限不足，该操作仅平台运营方可执行"}
PATCH /api/manage/tenants/5/status   ⇒ 403 同上
POST  /api/system/restart             ⇒ 403 同上
```

**库内状态核对**（停用被拒 ⇒ 库内必须没变）：

```sql
SELECT id,code,status FROM tenants ORDER BY id;
-- 1|tei6385|1  2|tc4h9aw|1  3|t4fpsxz|1  4|ted3cbt|1  5|tjvdvvu|1  6|tke98t6|1
-- 租户 5 仍是 active ✅（修复前它是 2）
```

**平台运营方正向路径**（首租户 tenant_id=1 的 admin 令牌）：

```
GET   /api/manage/tenants            ⇒ 200，total=6（全部租户可见 ✅）
PATCH /api/manage/tenants/5/status   ⇒ 200，{"status":2,"tenant_id":5}（可停用普通租户 ✅）
PATCH /api/manage/tenants/1/status   ⇒ 4101 "首个租户是平台运营方租户，不允许停用" ✅（死锁保护）
```

### 5. 形态④（`TENANT_MODE=on` + `PLATFORM_ENABLED=true` + 线上平台域名）复验

租户 7 的 admin（自助开户建的账号）：

```
GET /api/manage/tenants ⇒ 403 {"code":403,"message":"权限不足，该操作仅平台运营方可执行"}
```

⇒ 修复在 `on+true` 下**同样生效**，不是只在 `on+false` 成立。

### 6. off 模式零回归（形态①：`TENANT_MODE=off`）

```
GET /api/manage/tenants   ⇒ HTTP 200 ✅
GET /api/system/config     ⇒ HTTP 200 ✅
GET /api/system/stats      ⇒ HTTP 200 ✅
```

离线单机只有一个租户，平台运营方 = 本机超管 ⇒ 行为与历史版本**逐字节一致**。

### 7. 全量门禁

```bash
make audit          # AUDIT_RC=0
make lint           # 4 条红，均为 HEAD 已存在的既有债（见下）
make vet            # VET_RC=0
go test ./... -count=1 -p 1 -timeout 1800s
```

`make lint` 的 4 条红**在 HEAD 上一模一样存在**（逐条 `git show HEAD:<file>` 核对过），
不属本任务范围（不为绿灯改别人的文件）：

| 位置 | 类别 |
|---|---|
| `internal/service/customer_session_cron.go:95` | errcheck |
| `internal/bridge/sse.go:540` | ineffassign |
| `internal/middleware/rate_quota.go:240` | unused |
| `internal/service/reach_send_pipeline_quiethours.go:34` | unused |

## Test Results

| 检查项 | 结果 |
|--------|------|
| `go build` / `go vet` / `gofmt -l` | ✅ 干净 |
| `TestPlatformOperator*` 6 例 | ✅ |
| `TestUpdateTenantStatus*` 2 例（含真库） | ✅ |
| 租户 admin 读跨租户清单 ⇒ 403 | ✅ |
| 租户 admin 停用别的租户 ⇒ 403 **且库内状态未变** | ✅ |
| 租户 admin 调 `POST /api/system/restart` ⇒ 403 | ✅ |
| 平台运营方（首租户）读全部租户 ⇒ 200 | ✅ |
| 平台运营方停用普通租户 ⇒ 200 | ✅ |
| 平台运营方停用首租户 ⇒ 4101 拒绝（死锁保护） | ✅ |
| 首租户重新启用 ⇒ 允许 | ✅ |
| off 模式 admin 端点 ⇒ 与历史一致（200） | ✅ |
| 形态④（on+true）租户 admin ⇒ 403 | ✅ |
| `make audit` | ✅ AUDIT_RC=0 |
| `make lint` | 4 条红（HEAD 既有债，非本任务引入） |
| `go test ./...` 全量 | ✅ 0 FAIL |

## Conclusion

跨租户越权已从"**任何租户 admin 都能操作平台运营方端点**"收敛为
"**只有首租户（平台运营方）的管理员能操作**"，且三处越权向量（读跨租户清单 /
停用别的租户 / 实例级重启）全部转为 403。

关键点：
- **没有削弱行级隔离机制**——隔离 callback 一直是对的，泄露只在全局表与实例级端点；
- **判据是稳定的**（首租户 ID 最小，租户只增不删），不是"code 前缀"这类会漂移的派生量；
- **不把"查不到"当"放行"**——判定不出首租户时拒绝而非放行；
- **死锁防护**：首租户不允许被停用（否则平台运营方把自己锁在门外且无人能救），
  但允许重新启用；
- **off 模式零回归**：离线单机只有一个租户，平台运营方 = 本机超管，行为逐字节不变。
