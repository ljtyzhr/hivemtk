# Phase 1 Data Model: 租户数据模型与表登记分类

**Feature**: 001-saas-multi-tenant | **Date**: 2026-10-17

## 新增实体

### Tenant（租户）

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| id | uint | PK | 租户标识（即数据归属值） |
| code | varchar(32) | uniqueIndex | 租户短码（回调 URL/展示用），全局唯一 |
| name | varchar(100) | not null | 企业/团队名 |
| status | smallint | not null default 1 | 1=正常 2=停用（预留欠费/封禁扩展） |
| plan | varchar(32) | default 'free' | 套餐（预留，本期不承载计费逻辑） |
| created_at / updated_at | timestamptz | | |

状态机：`正常 ⇄ 停用`（停用：登录受限、后台任务跳过、数据保留）。

### SystemUser（既有，变更）

| 变更 | 说明 |
|------|------|
| + tenant_id uint | not null default 1；唯一索引 `(tenant_id, username)` 复合化 |
| JWT claims + tenant_id | on 模式必带；off 模式恒为默认租户 |
| 登录通道 | 邮箱全局唯一登录（消解跨租户用户名歧义）；单机模式登录方式不变 |

### LLMProvider（既有，变更）

| 变更 | 说明 |
|------|------|
| + tenant_id uint | 0=平台公共池；default 0 保持离线行为 |
| 加载顺序 | ctx 租户自有 → 平台池（tenant_id=0）→ 既有 failover |

## 全量既有业务表（租户表）

所有进登记表的业务表统一变更：

```sql
ALTER TABLE {t} ADD COLUMN IF NOT EXISTS tenant_id bigint NOT NULL DEFAULT 1;
-- 唯一索引按 D8 复合化（分批清单在 tasks 阶段逐表列明）
```

- 默认值 1 = 存量数据自动归默认租户，升级幂等。
- 首批必改唯一索引（示例，全量清单 tasks 阶段产出）：`system_users(username)`、`douyin_cards/xiaohongshu_cards/…(card_key)`、`short_links(code)`、`live_codes(code)`、`chat_channels(channel_ref)`、`bridge_accounts(account_ref)`。

## 全局表清单（tenant:"global"，豁免隔离）

首期盘点（tasks 阶段逐表确认后固化）：

| 表 | 豁免理由 |
|----|---------|
| tenants | 租户实体本身 |
| migration 记录/audit 表 | 系统级 |
| config_params / config_param_groups | 实例级调优参数，首期保持实例级 |
| feature_flags | 实例级功能开关 |
| platform 心跳/商户标识元数据 | 平台协议（可选模块） |
| cron_job_leases / telegram_polling_locks | 进程协调（键内已含租户分域） |
| backups / system 相关 | 系统操作（SystemTenant） |
| dictionary 等纯只读共享数据 | 全局只读 |

## 异步"行自证"待办表（必须携带 tenant_id）

SOP timer/execution、sop outbox、delayed_outbound、approval_request、agent_checkpoint、human_task、reach 任务、导入导出任务等全部待办/队列行——写入时由 callback 自动填 tenant_id，消费端读行取租户。**禁止**任何消费路径依赖环境上下文推断租户。

## 衍生资源分域

| 资源 | 规则 |
|------|------|
| Redis 键 | `t{tenant}:{原键}`；进程协调锁额外带租户段；off 模式前缀为默认租户（行为不变） |
| uploads | `uploads/tenant_{id}/...`；静态服务校验归属 |
| 导出/报表文件 | 同 uploads 规则 |

## 迁移策略

1. 分批幂等迁移（每批 ≤30 表）：加列 → 回填校验 → 唯一索引重建。
2. 升级中断可续跑（每批独立事务 + 进度记录）。
3. off 模式部署同样执行迁移（tenant_id 恒 1，为未来切 SaaS 留路）。
4. pgvector 表随业务表同规则处理（向量列不受影响，检索 SQL 由 GORM Where 片段携带租户条件）。
