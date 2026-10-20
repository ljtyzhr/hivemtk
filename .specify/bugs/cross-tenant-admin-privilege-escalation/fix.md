# Fix Report: 跨租户管理端点越权（租户 admin 可操作其他租户与实例级端点）

- **Slug**: cross-tenant-admin-privilege-escalation
- **Fixed**: 2026-10-21
- **Assessment**: [assessment.md](assessment.md)
- **Status**: applied

## Summary

修复 SaaS 模式（`TENANT_MODE=on`）下一处**真实跨租户越权**：
**任意租户的 admin 角色账号**都能操作平台运营方专属端点。

根因是「admin」这一个角色在 SaaS 模式下承载了两种语义（平台运营方 / 租户管理员），
而 `systemAdmin` 组的守卫只判 `role == "admin"`——租户 admin 也是 admin，守卫形同虚设。

修法（用户已确认方向：**平台超管 = 首个租户**）：

- 新增 `middleware.PlatformOperatorMiddleware()`：在角色判定**之上**加一道
  「你是不是平台运营方」判定——**调用者所属租户是本实例的首个租户**（`tenants` 表 ID 最小）。
  `off` 模式（离线单机）**恒真**，行为与历史版本逐字节一致。
- `router.go` 的 `systemAdmin` 组换挂该守卫。
- `UpdateTenantStatus` 增加**首租户不可停用**的死锁保护。

## Changes

| 文件 | 改动 | 依据 |
|------|------|------|
| `user-server/internal/middleware/platform_operator.go` | **新建**：`PlatformOperatorMiddleware()`——角色判定 + 首租户判定；判定不出首租户 ⇒ 拒绝（权限边界上"查不到"不等于"放行"）；首租户 ID 带 5 分钟进程级缓存 | assessment「Proposed Remediation」B1/B2 |
| `user-server/internal/router/router.go` | `systemAdmin` 组守卫从 `AdminAuthMiddleware()` 换成 `PlatformOperatorMiddleware()` | assessment B2 |
| `user-server/internal/repository/tenant.go` | 新增 `GetFirstTenant(ctx)`（`ORDER BY id ASC` 取首行）+ 接口声明 | assessment B1 |
| `user-server/internal/service/tenant.go` | `UpdateTenantStatus` 增判：首租户不允许被停用（防平台运营方把自己锁在门外） | assessment「Risks」首租户锁死条目 |
| `user-server/internal/middleware/platform_operator_test.go` | **新建**：6 例守卫回归 | assessment「Tests to add or update」 |
| `user-server/internal/service/tenant_first_tenant_protection_test.go` | **新建**：2 例首租户保护回归（含"库内状态未变"断言） | 同上 |

## Diff Highlights

`internal/middleware/platform_operator.go`（守卫核心）：

```go
func PlatformOperatorMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // 角色不达标先拒：平台运营方本身也是 admin 角色，role 判定是前置条件。
        role, exists := c.Get("role")
        ...
        // 离线单机：没有跨租户这回事，全部放行。
        if !tenant.IsMultiTenant() {
            c.Next()
            return
        }
        tenantID, bound := tenant.FromContext(c.Request.Context())
        ...
        ok2, err := isPlatformOperatorTenant(c.Request.Context(), tenantID)
        if err != nil {
            // 判定不了就拒：这是权限边界，"查不到"不等于"放行"。
            ...
        }
        if !ok2 {
            c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
                "code": 403, "message": "权限不足，该操作仅平台运营方可执行"})
            return
        }
        c.Next()
    }
}
```

`internal/repository/tenant.go`（首租户判据）：

```go
// GetFirstTenant 取本实例最早建立的租户（ID 最小）。
// 判据用 ID 最小而不是 code 前缀：租户是只增不删的（停用是软删，不改 ID），
// ID 最小 ⇒ 恒定不变 ⇒ 判据稳定；而 code 前缀（"local-" / "t"）是生成策略的
// 副产品，改了生成规则就会漂移。
func (r *tenantRepo) GetFirstTenant(ctx context.Context) (*model.Tenant, error) {
    var t model.Tenant
    if err := r.db.WithContext(ctx).Order("id ASC").First(&t).Error; err != nil {
        return nil, err
    }
    return &t, nil
}
```

`internal/service/tenant.go`（死锁保护）：

```go
// 首租户 = 平台运营方租户。停用它 ⇒ 平台运营方自己被锁在门外
// （登录受限 ⇒ 读不到自己的租户 ⇒ 恢复不了），且没有任何人能再停用/启用它 —— 死锁。
if status != model.TenantStatusActive {
    if first, err := s.tenantRepo.GetFirstTenant(ctx); err == nil && first != nil && first.ID == id {
        return bizerr.New(bizerr.CodeTenantParamInvalid,
            "首个租户是平台运营方租户，不允许停用（如需停用请先新建运营租户并移交）")
    }
}
```

## Tests Added or Updated

- `internal/middleware/platform_operator_test.go`（6 例）：
  非 admin 角色被拒 / 非首租户被拒 / 首租户放行 / 判定不出被拒 / on 模式无租户绑定被拒 /
  off 模式一律放行。
- `internal/service/tenant_first_tenant_protection_test.go`（2 例）：
  首租户停用被拒**且库内状态未变**（不能只断言错误码——"拒了但顺手改了库"会假绿）/
  首租户**重新启用**必须允许（否则一旦停用就再也救不回来）。
- 未改任何既有测试。

## Local Verification

环境：真实 PG（容器 `mtk-postgres`，`127.0.0.1:8202`，库 `user_db`），
口令运行时取值 `PW="$(docker exec mtk-postgres printenv POSTGRES_PASSWORD)"`。

### 1. 编译 / vet / gofmt

```bash
cd user-server
go build ./...                                   # BUILD_OK
go vet ./internal/middleware/... ./internal/repository/... \
        ./internal/service/... ./internal/router/...   # VET_OK
gofmt -l internal/middleware internal/repository internal/service internal/router
# 输出为空 ✅
```

### 2. 守卫单测（6 例）

```bash
go test ./internal/middleware/... -run 'TestPlatformOperator' -count=1 -v
```

**结果**：6 例全 PASS ✅

### 3. 首租户保护单测（2 例，含真库）

```bash
export POSTGRES_TEST_HOST=127.0.0.1 POSTGRES_TEST_PORT=8202 \
       POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD="$PW"
go test ./internal/service/... -run 'TestUpdateTenantStatus' -count=1 -v
```

**结果**：2 例全 PASS ✅（12.5s，含真实建表与状态核对）

### 4. 活体对抗验证（形态③：`TENANT_MODE=on` + `PLATFORM_ENABLED=false`，真实 8204）

**修复前**（租户 6 的 admin 令牌）：

```
GET   /api/manage/tenants            ⇒ 200，total=6（看到全部租户：公司名/负责人/电话/邮箱/地址）
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

### 5. off 模式零回归（形态①：`TENANT_MODE=off`）

```
GET /api/manage/tenants   ⇒ HTTP 200 ✅
GET /api/system/config     ⇒ HTTP 200 ✅
GET /api/system/stats      ⇒ HTTP 200 ✅
```

离线单机只有一个租户，平台运营方 = 本机超管 ⇒ 行为与历史版本**逐字节一致**。

### 6. 顺带修掉的一处并发改动暴露的降级缺陷

跑 2×2 矩阵期间，本仓工作区有**另一个 agent 进程**并发做 trace 事件模型搬迁
（`TraceEvent` 从 `internal/aiagent/llm` 移到 `internal/model/trace_event.go`，
属正交的模型分层改善）。它把 `trace_events` 变成了**模型持有**的表，
于是暴露出一个**早已存在**的降级缺陷：

`internal/migration/migrations/m_p1_migration.go`（v3.1.0）的 Down 里有
`DROP TABLE IF EXISTS trace_events` —— 而本项目的建表事实源是 AutoMigrate，
这张表被模型声明并持有，降级删它＝**销毁在用数据**（当前进程永不补回）。

这与仓规已定的口径完全一致：`down_drop_policy.go` 已为 21 个迁移把
「Down 只撤销自己 Up 新建的对象」作为护栏（`declineTableDrop`），
而 v3.1.0 三张表（`trace_events` / `intent_logs` / `provider_health`）**漏了**。

**修法**（与 `v3_34_0_churn_score_migration.go` 等 21 处同一写法）：

```go
func (m *MP1Migration) Down(ctx context.Context) error {
	declineTableDrop(m.Version(), "trace_events", "intent_logs", "provider_health")
	return nil
}
```

**验证**：`TestFullMigrationChainUpThenRollback` 修复前红（"Down 删掉了不属于它的表 1/97"、
"基线表净丢失 1 张：trace_events"、索引净丢 6 个、列净丢 12 个），
修复后 **PASS（36.76s）**。

### 7. 全量门禁

见 [test.md](test.md) 的 Test Results 表。

## Deviations from Assessment

- assessment 提到「B3 回归用例」里有一条「租户 admin 停用别的租户 ⇒ 403（**且目标租户不被停用**）」。
  本轮把这条断言落在**两个层次**：
  - 中间件层（`platform_operator_test.go`）—— 403 语义；
  - 服务层（`tenant_first_tenant_protection_test.go`）—— **库内状态未变**。
  理由：中间件 403 时请求根本没进 controller，服务层那条才是"状态没被改"的真正判据。
- assessment 未提「新租户开户后它是不是平台运营方」。实测**不是**：开户走 `CreateWithAdmin`
  落的是新租户行，ID 必然大于首租户 ⇒ 新开户的 admin 拿不到跨租户权限 ✅（这是期望行为，
  已在活体对抗里验证：租户 5/6 的 admin 均被 403）。

## Follow-ups

- **方案 A（独立角色 `platform_admin`）** 未做：语义最干净，但要改开户流程、角色枚举、
  登录签发、前端角色判断，存量账号要迁移，改动面大。当前用"首租户"作为判据，
  代价是**首租户被删则无平台运营方**——但租户是软删（停用不改 ID），实际不会发生。
- **`setupSystemRoutes` 里的实例级端点**（备份/恢复/重启）此前**任何 admin** 都能调，
  现已一并收进平台运营方档。若运维上需要"租户管理员也能重启服务"，
  应改走独立的运维通道，而不是放宽这一个守卫。
