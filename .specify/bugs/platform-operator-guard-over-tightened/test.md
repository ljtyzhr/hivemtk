# Bug Test: 平台运营方守卫过度收紧——租户 admin 被误挡在自己的租户级管理端点外

- **Slug**: platform-operator-guard-over-tightened
- **Tested**: 2026-10-21
- **Fix**: [fix.md](fix.md)
- **Result**: verified
- **Status**: verified

## Verification Commands

### 1. 编译 / vet / gofmt

```bash
cd user-server
go build ./...
go vet ./internal/router/...
gofmt -l internal/router
```

**结果**：全部干净 ✅

### 2. 门禁单测（2 例）

```bash
go test ./internal/router/... -run 'TestPlatformOperatorScope' -count=1 -v
```

**结果**：2 例全 PASS ✅

| # | 用例 | 覆盖 |
|---|------|------|
| 1 | `TestPlatformOperatorScopeEndpointOwnership` | 逐端点断言归属档位：租户级端点不得在平台档、平台级端点不得在租户档 |
| 2 | `TestPlatformOperatorScopeNoDuplicateRegistration` | 同一端点不得同时注册在两档 |

### 3. 活体复验：租户 admin 恢复访问自己的租户级端点

形态③（`TENANT_MODE=on` + `PLATFORM_ENABLED=false`），真实 8204，租户 6 的 admin 令牌：

| 端点 | 修复前 | 修复后 | 期望 |
|---|---|---|---|
| `GET /api/system/config` | 403 | **200** ✅ | 200 |
| `GET /api/agent/tool-integrations` | 403 | **200** ✅ | 200 |
| `GET /api/agent/settings` | 403 | **200** ✅ | 200 |
| `GET /api/obs/config` | 403 | **200** ✅ | 200 |
| `GET /api/manage/config-params` | 403 | **200** ✅ | 200 |
| `GET /api/manage/config-params/groups` | 403 | **200** ✅ | 200 |
| `GET /api/admin/config` | 403 | **200** ✅ | 200 |
| `GET /api/modules/sync-logs` | 403 | **200** ✅ | 200 |
| `GET /api/sso/config` | — | **200** ✅ | 200 |

### 4. 活体复验：越权面仍然堵着

同一令牌（租户 6 的 admin）：

```
GET   /api/manage/tenants                    → 403 ✅
PATCH /api/manage/tenants/5/status           → 403 ✅（停用别的租户仍被拒）
GET   /api/system/stats                      → 403 ✅
GET   /api/system/backup                     → 403 ✅
GET   /api/modules/uncovered-routes          → 403 ✅
POST  /api/system/restart                    → 403 ✅
PUT   /api/manage/dictionaries/:parent/items → 403 ✅
```

平台运营方（首租户 tenant_id=1）正向路径不受影响：

```
GET /api/manage/tenants            → 200，total=6 ✅
PUT /api/manage/dictionaries/:parent/items → 200 ✅
GET /api/sso/config                 → 200 ✅
```

### 5. 租户级写仍受隔离保护

`POST /api/system/config`（租户 6 的 admin 改自己站点名）⇒ 200，响应 `tenant_id=6`；
库内 `system_config` 核对：**租户 5 的行未变**，只有租户 6 的行被改。

⇒ 「租户 admin 能写自己那份」与「写不到别人那份」同时成立。

### 6. **反向验证**（门禁必须能红）

把 5 个租户级端点临时挪到 `systemAdmin` 档后重跑：

```
--- FAIL: TestPlatformOperatorScopeEndpointOwnership
    租户级端点 /api/system/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/agent/tool-integrations 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/agent/settings 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/obs/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
    租户级端点 /api/admin/config 误挂在平台运营方档 ⇒ 租户 admin 会被 403
--- FAIL: TestPlatformOperatorScopeNoDuplicateRegistration
```

恢复修复后重新变绿 ✅

### 7. 全量门禁

```bash
make audit          # AUDIT_RC=0
make lint           # 0 issues
make vet            # VET_RC=0
go test ./... -count=1 -p 1 -timeout 1800s
```

## Test Results

| 检查项 | 结果 |
|--------|------|
| `go build` / `go vet` / `gofmt` | ✅ 干净 |
| `TestPlatformOperatorScope*` 2 例 | ✅ |
| **反向验证**（挪错档位 ⇒ 必须红） | ✅ 红了，恢复后绿 |
| 租户级端点 9 条恢复 200 | ✅ |
| 越权向量 7 条仍 403 | ✅ |
| 平台运营方正向路径不受影响 | ✅ |
| 租户级写的隔离保护（写不到别人那份） | ✅ |
| `make audit` | ✅ AUDIT_RC=0 |
| `make lint` | ✅ 0 issues |
| `go test ./...` 全量 | ✅ 0 FAIL |

## Conclusion

上一轮修复的**方向是对的**（跨租户越权确实堵住了），但**施加范围错了**：
守卫按"路由组"施加，而权限边界按"数据域"划分，导致 8 条租户级管理端点被误伤成 403。

本轮把 admin 档按**端点操作的数据落在哪张表**拆成两档：

- **租户档**（普通 admin 守卫）：`system_config` / `system_config_kv` / `obs_config` /
  `module_sync_logs` / `tenant_config_params` / `sso.providers` —— 隔离 callback 已限域，
  租户 admin 本就该读写自己那份。
- **平台运营方档**（`PlatformOperatorMiddleware`）：`tenants` 租户清单 / 停用租户 /
  备份恢复 / 重启 / 实例统计 / 进程内路由巡检 / 字典写侧。

**教训（已写进门禁注释）**：平台运营方守卫是"降敌意"方向——
套错档位会**误伤合法功能**；漏套会**漏放越权**。两个方向代价不对称，
但都必须按**数据域**判，不能按"这路由组看起来像管理端"判。
且这类回归**编译照过、常规单测照绿**，只有拿真实租户令牌打活体端点才看得见 ——
所以门禁必须做**反向验证**（挪错档位 ⇒ 必须红），否则它只是一张摆设的清单。
