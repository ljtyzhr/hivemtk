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

1. **GORM callback**（`pkg/tenancyidx/tenant_callback.go`，六类 operation）：on 模式自动注入 `WHERE tenant_id=?` / Create 自动填充 / 跨租户写拒绝 / Raw-Exec 守卫；off 直通。
2. **登记表**（`pkg/tenancyidx`，数据库内省）：**唯一判据是「表上有没有 tenant_id 列」**，没有第二份豁免清单——漏加列的表在内省里就是非租户表、全量放行。所以「这张表是不是租户表」要在模型 Tag 与迁移两处对账，而不是在某个清单里登记一次。配置类六张表的归属见 §三.5。
3. **唯一性租户化**：`tenancyidx.Normalize` 原位复合化全部非约束型唯一索引与约束型 UNIQUE 为 `(tenant_id, 原列...)`；挂 AutoMigrate 尾部 + testdb 钩子。
4. **GORM-only 门禁**（`user-server/scripts/check-gorm-only.sh`，由 `Makefile` 的 audit 链以 strict 调用）：业务包 `.Exec(`/`.Raw(` 归零（133 处历史债务已清偿）；豁免以「DDL 生命周期同类」为单位整文件或整目录放行（migration 目录、schema 引导与归一化、自省类、pgvector 运维、测试夹具），**清单与每条例外的理由写在脚本头部注释里**，本文件不复制第二份。

### 三.5 配置数据的租户归属（R5，2026-10-20）

SaaS 化后"配置"不是一张表的事：同一份配置值可能属于平台、也可能属于商户。下面这张表是**当前形状的逐表现测**（判据=模型 Tag 里有没有 `TenantID`），改任何一张表都要回来更新它——内判登记表只看列，列加错方向等于整张表跨租户放行。

| 表 | 归属 | 说明 |
|---|---|---|
| `config_params` | 平台（实例级） | 参数中心的**定义目录 + 平台默认值**，无 tenant_id：种子随版本增删，逐租户复制定义会让"漏一个租户"读成代码兜底值 |
| `config_param_audit_logs` | 平台 | 平台默认值的变更审计 |
| `tenant_config_params` | 租户 | 只存「这个租户改过哪几键」，未覆盖的键回落 `config_params.value`；唯一键 `(tenant_id, param_group, param_key)` |
| `tenant_config_param_audit_logs` | 租户 | 同上，带 tenant_id |
| `system_config` | 租户 | 站点运营配置（站点名/注册开关/上传上限），`uni_system_config_tenant` 一租户一行 |
| `system_config_kv` | 租户 | 主键复合化为 `(tenant_id, key)`（迁移 v3.65.0）；租户业务配置与凭证包（`agent.tool_integrations`、`sso.providers`） |
| `platform_config_kv` | 平台 | 从 `system_config_kv` 拆出的平台接入令牌（桥接/MCP/回调验签/宿主令牌），key 单列主键、实例一份 |
| `feature_flags` + 两张日志表 | 租户 | 旧的 `key` 单列唯一索引必须删掉才允许跨租户同名开关（迁移 v3.66.0） |

**凭证类不落参数中心**：`tenant_config_params.param_value` 是明文列（只有审计行走 `dbencrypt`），把 client_secret/API key 写进去等于新增一个明文凭证面。凭证走 `system_config_kv` + `internal/secrets`（`enc:v1:` AES-GCM，MASTER_KEY），先例是 `agent.tool_integrations`，本轮新增的是 `sso.providers`（见 `docs/CONFIG_DB_DRIVEN_PLAN.md` §10 R5-4）。

### 三.6 不进 GORM callback 的三条通道（2026-10-23 补）

隔离 callback 的射程是**数据访问**。凡是绕开它把数据带出去的东西，都必须自己有租户判据——
否则 `make audit` / `make test-go` / `check-gorm-only` 全绿，缺陷照样在。实测三条：

| 通道 | 判据位置 | 口径 |
|---|---|---|
| 实时推送（SaaS） | `service/sse_hub.go` `Publish` / `DashboardSSE` 快照 | 只发给 `client.tenantID == ctx 租户` 的连接 |
| 实时推送（WS 坐席） | `websocket/hub.go` `BroadcastToMerchant` | 同上；ctx 缺租户 ⇒ **拒绝执行**（fail-closed），绝不退化成全实例广播 |
| 静态文件（`/files/*`） | `router/files_guard.go` `tenantPathAllowed` | on 模式下 `tenant_{id}/` 段与请求 ctx 租户不一致 ⇒ 404；无凭证公开外链（访客 `<img>`）仍放行 |
| 进程级内存缓存 | `middleware/ownership.go` `ownershipCacheKey` | 缓存键带租户段；否则一个租户的查询结果会命中另一个租户的判定（且该处 not-found 语义是 fail-open） |

> 共同失效模式：**"值被记下来了，但没有任何一处用它判定"**（`Client.tenantID` 在两条握手链路都绑好了，
> 全仓却只有测试在读它）。评估这类改动时，判据不是"有没有赋值"，而是"有没有一处 `if` 用它"。
> 完整清单与待办见 [SAAS_FULLCHAIN_AUDIT_2026-10-23.md](SAAS_FULLCHAIN_AUDIT_2026-10-23.md)。

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
