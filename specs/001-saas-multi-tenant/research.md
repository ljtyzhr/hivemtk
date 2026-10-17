# Phase 0 Research: 架构决策记录

**Feature**: 001-saas-multi-tenant | **Date**: 2026-10-17

所有决策均已在头脑风暴阶段与用户完成论证并拍板。此处记录决策、依据与被否备选。

## D1 租户数据隔离物理形态

- **Decision**: 共享库 + 行级 `tenant_id` 隔离（所有业务表同库共存，按租户列过滤）。
- **Rationale**: 245 张表规模下迁移与运维成本最低；符合用户"基于租户 ID 隔离"的明确要求；跨租户统计、平台运营查询天然可行；备份/监控单库一套。
- **Alternatives rejected**:
  - schema-per-tenant：245 表 × N 租户的迁移矩阵、search_path 连接池管理、跨租户运营统计复杂化。
  - DB-per-tenant：SaaS 运维（备份/监控/迁移）成本最高，连接数爆炸风险。

## D2 隔离执行机制（不入侵业务代码）

- **Decision**: GORM 全局 callback（Query/Create/Update/Delete/Row/Raw 六类 operation 的 pre-hook）按"租户表登记表"自动注入 `WHERE tenant_id=?` / 自动填充写入列；登记表由模型 `TenantID` 字段启动期自动收集，显式 `tenant:"global"` tag 豁免全局表。
- **Rationale**: 业务代码零改动（满足铁律）；单点执行、单点审计；off 模式 callback 直通等价现状。
- **配套终态（用户拍板）**: 全仓数据访问 GORM-only——业务包 `.Exec()`/`.Raw()` 历史债务（约 293 处，migration 包 72 处 DDL 豁免）本次彻底清偿为 GORM API（`clause.OnConflict`/`clause.Locking`/`clause.Returning`/Where 片段参数绑定均可原生表达已抽样证实）；CI 门禁脚本永久阻止回归。
- **Alternatives rejected**:
  - RawBypass 白名单 helper（允许裸 SQL 携带标记放行）：用户明确否决——门禁放行是债务转移，不是清零。
  - PG RLS（数据库层行级安全）：兜底最强但连接池/事务作用域复杂；列为二期可选加固。
  - Repo 层手动 Scopes：纯人肉、与"不入侵"冲突。

## D3 SaaS 开户入口

- **Decision**: user-server 内置自助注册（`TENANT_MODE=on` 时启用）：注册即建租户+管理员+默认配置，事务原子；离线模式端点禁用。
- **Rationale**: 引流转化最短路径；平台端（hivemtk-platform）计费/下发可后续叠加，接口留扩展位。
- **Alternatives rejected**: 平台端开户下发（依赖平台端先行开发，阻塞引流目标）；两者都要（首期工作量不必要）。

## D4 LLM 双轨

- **Decision**: `llm_providers` 增加 `tenant_id` 维度（0=平台公共池）；租户解析顺序：租户自有 provider → 平台池 → 既有 failover/熔断链。SaaS 环境预置平台池，新租户零配置可用。
- **Rationale**: 完全复用既有 dispatcher/LoadProvidersFromDB/failover 机制，零新链路；离线模式 provider 全在平台池（tenant_id=0），行为等价现状。
- **Alternatives rejected**: 平台 LLM 网关代理（新协议+计费回调，且与零出域默认原则冲突）；砍掉平台 LLM（与"一键开户低门槛"冲突）。

## D5 无租户上下文场景的解析链

- **Decision**: 每类外部入口一条权威解析链，解析失败 fail-closed（拒绝+告警，绝不落默认租户）：

| 入口 | 权威线索 | 现状证据 |
|------|---------|---------|
| JWT API | claim.tenant_id | middleware/jwt.go 扩展 |
| 渠道 webhook | URL `/:channel/:account_id` | controller/webhook.go:69 现状即此形状，协议零改动 |
| bridge SSE/HTTP | X-Bridge-Token | bridge_account → tenant |
| 访客 Widget | X-Chat-App-Key / Channel-Id | router.go:138 三头机制，channel → tenant |
| Telegram long-polling | per-account 循环 | account → tenant，每轮构造 ctx |
| 邮件/短信回调 | 收发件账号配置 | 账号 → tenant |
| cron/SOP/队列/延时任务 | **行自证**：待办行写入时由 callback 自动携带 tenant_id，消费端读行取租户 | delayed_outbound/sop_timer 等待办表全部进登记表 |
| 事件总线 | payload 结构体强制增加 TenantID 字段 | event 包编译期保障 |
| WS/SSE | 连接建立时鉴权绑定租户 | 推送分桶 |
| migration/seed/health/备份 | SystemTenant 显式豁免 | 白名单路径 |

- **Rationale**: "不猜"原则——每条链可独立审计；行自证使异步链路不依赖环境上下文。
- **Alternatives rejected**: 全局默认租户兜底（会把漏配变成静默串库，方向性错误）。

## D6 双形态开关

- **Decision**: `TENANT_MODE` 环境变量（`off` 默认 | `on`）。off：全部请求/任务强制绑定 `DEFAULT_TENANT_ID=1`（固定值），callback 等价直通，注册端点禁用；on：全机制生效。既有实例升级零配置。
- **Rationale**: 离线版"一套代码、默认租户维持全部逻辑"由开关天然满足；存量测试零改动全绿是硬验收。

## D7 后台任务租户扇出

- **Decision**: `TenantRunner` 装配组件统一包装 20+ cron/SOP dispatcher/事件订阅/恢复 worker：枚举活跃租户 → 逐租户构造带租户 ctx 执行；分布式锁（Redis/DB lease）key 加 `t{tenant}` 前缀。off 模式退化为单租户（默认租户）。
- **Rationale**: 单点改造覆盖全部后台路径，避免 20+ 任务各自处理租户维度。

## D8 唯一索引复合化

- **Decision**: 既有业务唯一索引（如 system_users.username、各平台卡密/短链 code 等）重建为 `(tenant_id, 原列)` 复合唯一；用户名跨租户可并存，登录歧义由"邮箱全局唯一 + 单机模式登录方式不变"消解。
- **Rationale**: 行级隔离下唯一性必须租户维度；迁移分批幂等。

## D9 表登记分类原则

- **Decision**: 三类——①租户表（默认，凡有 TenantID 字段自动登记）；②全局表（显式 `tenant:"global"`：tenants 自身、migration 记录、config_params 参数中心首期保持实例级、feature_flags、平台心跳元数据、系统字典等，预计 10-20 张，tasks 阶段逐表盘点定案）；③系统操作豁免（migration DDL、备份恢复、健康检查以 SystemTenant 身份运行）。
- **Rationale**: 默认收窄、显式豁免，漏登记的风险方向是"多余限制可被发现"，不是"数据泄漏"。

## D10 缓存与衍生资源

- **Decision**: Redis 键统一 `t{tenant}:` 前缀（键构造函数单点封装，禁裸拼）；uploads 落 `uploads/tenant_{id}/`；导出/报表文件路径同样分域；文件静态服务校验租户归属。
- **Rationale**: 衍生资源是行级隔离之外最易漏的串域通道。

## 未决事项

无。所有 NEEDS CLARIFICATION 已在头脑风暴阶段由用户拍板（隔离形态/开户入口/LLM 实现三项）。
