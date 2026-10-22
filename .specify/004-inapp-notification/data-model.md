# 数据模型：站内消息通知模块

> 两侧各一张新增表 + 若干扩列。实体名沿用仓内既有命名习惯。

## 1. 平台消息（PlatformMessage）— 平台库 `platform_db`

**归属**：平台运营方。**表名**：`platform_messages`（**已存在**，本次扩列 + 启用死列）

| 字段 | 类型 | 本次变更 | 说明 |
|---|---|---|---|
| `id` | bigint PK | 既有 | 平台消息唯一标识 |
| `title` | varchar(255) | 既有 | 标题，非空 |
| `content` | text | 既有 | 正文，非空 |
| `level` | varchar(16) | **启用** | 级别：`info`/`warning`/`error`/`success`。**既有列但恒为 info**（service 硬编码），本次让发布方真正选 |
| `type` | varchar(32) | 既有 | 保留原语义（`announcement` 等），不作为投递判据 |
| `targets` | text | **启用** | 目标租户 code 逗号分隔；**空 = 全体租户**。既有列但从不被读取 |
| `is_read` | bool | 既有 | **平台侧已读位，本次不改语义也不使用**（平台消息的已读由用户端回执承担） |
| `published_at` | timestamptz | **新增** | 发布时间，投递游标用。发布时写入 |
| `created_at` / `updated_at` | timestamptz | 既有 | |

**投递判据**（平台侧）：`published_at IS NOT NULL`（= 已发布）且
（`targets` 为空 OR `targets` 含本租户的 `tenant_code`）。

> `is_read` 是平台运营视角的"我已看过这条"，与用户端"用户已读"无关。
> 本次不改它、不在用户端读它 —— 两套语义不同域，避免混淆。

## 2. 通知投递台账（NoticeDeliveryLog）— 平台库 `platform_db`（新增表）

**归属**：平台运营方。**表名**：`notice_delivery_logs`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigint PK | |
| `message_id` | bigint | 平台消息 ID，索引 |
| `merchant_key` | varchar(32) | 哪个部署实例 |
| `tenant_code` | varchar(32) | 哪个租户，索引 |
| `status` | varchar(16) | `success` / `failed` / `skipped` |
| `error_msg` | text | 失败原因 |
| `attempted_at` | timestamptz | 投递尝试时间 |

**唯一约束**：`(message_id, tenant_code)` —— 同一消息对同一租户只记一条终态。

**为什么需要**：FR-019 要求"记录每条消息的投递结果，供运营方追溯"。
用户端投递失败只落在用户端自己的日志里，运营方在平台后台看不到。
这张表让平台侧能回答"这条公告到底送达了没有、哪些租户没收到"。

## 3. 通知（Notification）— 用户库 `user_db`

**归属**：用户端。**表名**：`notifications`（**已存在**，本次扩列）

| 字段 | 类型 | 本次变更 | 说明 |
|---|---|---|---|
| `id` | bigint PK | 既有 | |
| `tenant_id` | bigint | 既有 | **租户隔离列** ⇒ 触发 GORM 隔离 callback |
| `user_id` | bigint | 既有 | `0` = 全体可见（广播）。平台消息一律落 `user_id=0` |
| `type` | varchar(32) | 既有 | 通知类型。平台消息用新类型 `platform_notice` |
| `title` | varchar(255) | 既有 | |
| `content` | text | 既有 | |
| `link` | varchar(512) | 既有 | 跳转链接，可空 |
| `is_read` | bool | 既有 | **本地业务通知的已读位，语义不变**。平台消息此列恒 false，不参与平台消息的已读判定 |
| `read_at` | timestamptz | 既有 | 同上 |
| `metadata` | text | 既有 | 平台消息在此存 JSON：`{"source":"platform","level":"warning"}` |
| `created_at` | timestamptz | 既有 | 平台消息落库时取平台的 `published_at`（保持时间线一致） |
| `updated_at` | timestamptz | 既有 | |
| `level` | varchar(16) | **新增** | 平台消息级别；本地业务通知用 `info` |
| `source_msg_id` | bigint | **新增** | 来源平台消息 ID。`0` = 本地产生 |
| `source` | varchar(16) | **新增** | `platform` / `local`，用于前端按来源区分 |

**新增索引**：

| 索引 | 类型 | 判据 |
|---|---|---|
| `idx_notifications_tenant_source_msg` | **部分唯一** `(tenant_id, source_msg_id) WHERE source_msg_id > 0` | 幂等去重（FR-007）。部分唯一的原因见下 |
| `idx_notifications_tenant_level` | 普通 `(tenant_id, level)` | 按级别筛选（FR-010） |

> **为什么用部分唯一索引**：本地业务通知的 `source_msg_id` 恒为 `0`，
> 若用普通复合唯一索引 `(tenant_id, source_msg_id)`，**第二条本地业务通知
> 就会撞唯一键** ⇒ 存量生产者全部写失败。部分唯一（`WHERE source_msg_id > 0`）
> 只约束平台消息，不影响本地通知。

## 4. 通知回执（NotificationRead）— 用户库 `user_db`（新增表）

**归属**：用户端。**表名**：`notification_reads`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | bigint PK | |
| `tenant_id` | bigint | **租户隔离列**（回执同样按租户隔离） |
| `notification_id` | bigint | 指向 `notifications.id`，索引 |
| `user_id` | bigint | 谁读的 |
| `read_at` | timestamptz | 读取时间 |

**唯一约束**：`(tenant_id, notification_id, user_id)` —— 同一用户对同一消息
只有一条回执，标记已读是 **upsert**（幂等，重复点不报错）。

**为什么单独成表**：`notifications.is_read` 是消息行上的单一布尔位，
一条广播消息（`user_id=0`）代表 N 个人，A 点已读会让 B 的那条也变已读
⇒ 未读角标被一个人清零。回执表把"消息 × 人"的已读关系显式建模，
从根本上解决（FR-012 / FR-014 / SC-003 / SC-007）。

## 5. 关系

```text
PlatformMessage (平台库)
        │ id
        │  1:N  经 tenant_code 匹配
        ▼
Notification (用户库, 每租户一行)
        │ id
        │  1:N
        ▼
NotificationRead (用户库, 每用户一行)
```

`PlatformMessage` 与 `Notification` **跨库**，靠 `tenant_code` +
`source_msg_id` 建立逻辑关联，不做物理外键。

## 6. 状态流转

**平台消息**：`created`（落库，`published_at` 为空）→ `published`（`published_at` 写入）。
本次不实现撤回/编辑（平台端无此需求）；已发布消息若在平台端被删除，
用户端已投递的副本仍可查看（以投递时快照为准，见 spec Edge Cases）。

**通知（用户端）**：`delivered`（落库）→ 用户视角的 `read`（存在对应回执）。
`notifications.is_read` 对平台消息恒 false —— 已读只由回执表表达。

## 7. 校验规则（源自 FR）

| 规则 | 落点 |
|---|---|
| 标题非空 | 平台端发布校验（Service 层） |
| 正文非空 | 平台端发布校验 |
| 标题 ≤ 255 字符 | 平台端发布校验（与列宽对齐） |
| 级别 ∈ {info, warning, error, success} | 平台端发布校验 |
| 目标集合为空 = 全体 | 平台端投递过滤 |
| `source_msg_id > 0` 时该租户内唯一 | 部分唯一索引 |
| 回执 `(tenant_id, notification_id, user_id)` 唯一 | 复合唯一索引 |
