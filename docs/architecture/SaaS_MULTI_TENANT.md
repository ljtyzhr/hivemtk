# SaaS 多租户架构与运维（specs/001-saas-multi-tenant T041）

**状态**：落地版（隔离框架层 + GORM-only + 双形态基建全量交付）
**设计事实源**：`specs/001-saas-multi-tenant/`（spec/plan/research/data-model/contracts/quickstart/tasks）

## 一、双形态总览

| 形态 | TENANT_MODE | 行为 |
|------|-------------|------|
| **离线单机**（默认） | `off`（或不设置） | 全部数据归默认租户 1；隔离 callback 直通（SQL 逐字节等价历史版本）；租户注册关闭（4104）；存量实例升级零配置 |
| **SaaS 多租户** | `on` | 开放自助开户；行级隔离自动注入；后台任务按活跃租户扇出；外部入口按解析链定位租户，失败即拒 |

同一份代码产物，行为由环境变量单点决定；未知值一律 fail-safe 朝 off。

## 二、租户上下文与解析链

**贯通方式**：入口中间件/解析链把租户写入 `context` → GORM callback 从 `Statement.Context` 自动注入 → 后台任务经 TenantRunner/行自证构造。

| 入口 | 权威线索 | 绑定点 |
|------|---------|--------|
| JWT API | claim.tenant_id | `middleware/tenant.go`（auth 组） |
| 渠道 webhook | URL `/:channel/:account_id` | `controller/webhook.go` → bridge_accounts |
| bridge 入站 | 账号归属（校验返回租户） | `bridge/handler_http.go` |
| 访客 Widget | X-Chat-App-Key → channel | `middleware/app_key_auth.go` |
| 访客 WS | channel_id → channel | `websocket/visitor_handler.go`（窄接口注入） |
| 坐席 WS | JWT 租户 claim | `websocket/handler.go` |
| 邮件追踪 | token claim（签发时捕获） | `service/email_tracking.go` |
| 后台周期任务 | 活跃租户枚举扇出 | `pkg/tenant/fanout.go`（main.go 接线） |
| SOP outbox/timer | 行自证（timer.tenant_id） | `sop_outbox_dispatcher.go` / dispatchTask.TenantID |
| 事件总线 | payload.TenantID | `event/types.go` + 订阅者 `tenantCtx` |
| 系统级（migration/seed/backup） | SystemScope 显式豁免 | `pkg/tenant.SystemScope` |

**铁律**：`on` 模式下租户表访问 ctx 缺租户 ⇒ fail-closed 报错（绝不落默认租户）；解析失败 ⇒ 拒绝 + 安全日志。

## 三、隔离机制（框架层，业务代码零手工隔离）

1. **GORM callback**（`pkg/db/tenant_callback.go`，六类 operation）：on 模式自动注入 `WHERE tenant_id=?` / Create 自动填充 / 跨租户写拒绝 / Raw-Exec 守卫；off 直通。
2. **登记表**（`pkg/tenancyidx`，数据库内省）：有 tenant_id 列的表即租户表；全局表（tenants/参数中心/功能开关/租约等）显式豁免清单。
3. **唯一性租户化**：`tenancyidx.Normalize` 原位复合化全部非约束型唯一索引与约束型 UNIQUE 为 `(tenant_id, 原列...)`；挂 AutoMigrate 尾部 + testdb 钩子。
4. **GORM-only 门禁**（`scripts/check-gorm-only.sh`）：业务包 `.Exec(`/`.Raw(` 归零（133 处历史债务已清偿）；migration DDL / db.migrate / testutil / pgvector 运维单点豁免；`make audit` strict 挡回归。

## 四、后台任务租户化

- `tenant.Fanout(ctx, db, fn)`：off 单次默认租户（等价现状）；on 枚举 `tenants WHERE status=1` 逐租户回调，单租户失败不阻断。
- 接线：main.go 8 个闭包 cron + RFM/Churn/RagEval/Journey/WeComQuota cron；SOP dispatch 走 dispatchTask.TenantID 行自证。
- 行自证原则：一切待办/队列/追踪行（SOP timer、checkpoint、delayed_outbound、追踪 token…）写入时携带 tenant_id，消费端读行恢复上下文。

## 五、运维手册

### 升级（离线实例）
```bash
git pull && make build && make restart
# 不设置 TENANT_MODE 即 off；迁移自动补列归默认租户，幂等可重入（T019 真库演练已验）
```

### 切 SaaS 模式
```bash
# .env
TENANT_MODE=on
make restart
# 登录页开放 /api/tenant/register；已有实例默认租户 1 的数据继续可用
```

### 租户停用/启用（平台运营）
```bash
PATCH /api/manage/tenants/{id}  {"status": 2}   # 停用：登录受限、后台任务跳过、数据保留
```

### 验证门（每次变更后）
```bash
make audit && make lint && make vet && make test-go  # test-go 需 POSTGRES_TEST_* 凭据
bash user-server/scripts/check-gorm-only.sh           # 裸 SQL 归零门禁
```

### 越权回归测试
- `pkg/db/tenant_callback_test.go`：七态（off 直通/on 注入/fail-closed/行自证/系统豁用/跨租户写拒/Raw 守卫）
- `service/tenant_isolation_test.go`：双租户交叉列表/直查/更新/删除/渠道域 0 泄漏
- `service/tenant_offline_test.go`：离线四态等价断言

## 六、相关文档

- 规格与决策：`specs/001-saas-multi-tenant/`（research.md D1-D10 含否决理由）
- 部署形态：`docs/architecture/部署方案_用户端.md` §十一
- 环境变量：`.env-example`（TENANT_MODE/DEFAULT_TENANT_ID）
