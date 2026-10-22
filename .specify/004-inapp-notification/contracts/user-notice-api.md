# 契约：用户端通知接口

> 用户端（user-web）→ 用户端后端（user-server）。
> 响应统一信封 `{"code":0,"data":...,"message":"ok"}`。
> 错误码：0=成功 400=参数 401=认证 403=权限 404=不存在 500=服务端。

## 0. 与既有端点的关系

本模块**复用既有通知中心端点**（`user-server/internal/router/auth_routes.go:41-50`），
只扩充语义，不新增平行端点。既有别名路径（`/api/notifications*`）同时保留。

| 既有路径 | 动作 | 本次变更 |
|---|---|---|
| `GET /api/auth/notifications` | 列表 | **返回的 `is_read` 改为按当前用户的回执计算**（平台消息）；本地业务通知仍按行上 `is_read` |
| `POST /api/auth/notifications/:id/read` | 标记已读 | **平台消息改写回执表**（upsert）；本地业务通知保持原逻辑 |
| `POST /api/auth/notifications/read-all` | 全部已读 | **平台消息批量写回执**；本地业务通知保持原逻辑 |
| `GET /api/auth/notifications/unread-count` | 未读数 | **平台消息未读 = 该用户无回执的条数**；本地业务通知沿用原口径 |

## 1. 通知列表

`GET /api/auth/notifications?page=1&page_size=20&type=&is_read=&keyword=&level=`

**权限**：登录态（`auth` 组，已挂 JWT 中间件）。

**Query**

| 参数 | 说明 |
|---|---|
| `page` / `page_size` | 分页，`page_size` 上限 100，越界回落 20 |
| `type` | 通知类型过滤，如 `platform_notice` |
| `level` | **新增**：级别过滤 `info`/`warning`/`error`/`success` |
| `is_read` | 已读状态过滤（布尔） |
| `keyword` | 标题或正文模糊匹配 |

**成功响应**

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 21,
        "user_id": 0,
        "type": "platform_notice",
        "source": "platform",
        "level": "warning",
        "title": "系统维护通知",
        "content": "……",
        "link": "",
        "is_read": false,
        "read_at": null,
        "metadata": "{\"source\":\"platform\",\"level\":\"warning\"}",
        "created_at": "2026-10-22T10:00:00+08:00",
        "updated_at": "2026-10-22T10:00:00+08:00"
      }
    ],
    "total": 1,
    "page": 1,
    "size": 20
  },
  "message": "ok"
}
```

**字段语义**：
- `source`：`platform`（平台下发）/ `local`（本地业务通知）——前端按此区分。
- `is_read`：**当前用户**视角。平台消息 = 该用户是否已回执；本地业务通知 = 行上原值。
- `content` 为纯文本，前端按纯文本渲染（FR-020）。

**失败响应**：500 `查询失败`（DB 错误）。

## 2. 标记单条已读

`POST /api/auth/notifications/:id/read`

**权限**：登录态。

**成功响应**

```json
{ "code": 0, "data": null, "message": "已标记为已读" }
```

**失败响应**

| code | message | 触发 |
|---|---|---|
| 400 | `无效的 id` | id 非数字 |
| 400 | `通知不存在或无权限` | 该通知不属于当前租户/用户 |
| 500 | `操作失败` | DB 错误 |

**幂等**：重复标记同一消息返回成功（回执 upsert）。

**跨租户保护**：通知不属于当前租户 ⇒ 400，**不泄露该消息存在**。

## 3. 全部标记已读

`POST /api/auth/notifications/read-all`

**权限**：登录态。

**成功响应**

```json
{ "code": 0, "data": { "marked": 7 }, "message": "已全部标记为已读" }
```

`marked` = 本次新写入回执的条数 + 本地业务通知置已读的条数。

## 4. 未读数

`GET /api/auth/notifications/unread-count`

**权限**：登录态。

**成功响应**

```json
{ "code": 0, "data": { "count": 3 }, "message": "ok" }
```

**口径**：平台消息未读数 = 当前租户内、当前用户**无回执**的平台消息条数；
本地业务通知未读数沿用既有口径。两者相加。

> **重要**：此口径与既有实现不同 —— 既有 `CountUnread` 是
> `WHERE is_read = false AND (user_id = 0 OR user_id = ?)`，
> 即"消息行上的全局未读位"。平台消息必须改成回执口径，
> 否则一人点已读、全场清零（SC-003 违约）。

## 5. 平台消息同步（内部，用户端 → 平台）

`GET /merchant-api/notices/list?tenant_code=<code>&since=<RFC3339>`
（由用户端发起，调用的是平台端）

**用户端内部行为**（非对外 API，但需可观测）：

1. 每 3 分钟一轮，首轮在启动时**同步执行**。
2. 按本地租户 `code` 逐个拉取（多租户部署时每个租户各拉一份）。
3. 落库到 `notifications`：`source_msg_id` = 平台消息 ID，`tenant_id` = 本地租户号，
   `user_id` = 0（广播），`type` = `platform_notice`，`level` 取平台值。
4. 幂等：`(tenant_id, source_msg_id)` 部分唯一索引 + upsert。
5. 平台不可达 ⇒ 保留上次已投递，**不清空、不报错中断**。
6. `PLATFORM_ENABLED=false` ⇒ **完全跳过**，不发起任何平台请求。

**日志关键字**（便于运维排查）：
`[notice-sync] 首轮同步完成`、`[notice-sync] 周期同步失败`、`[notice-sync] 已跳过（平台联动未启用）`。

## 6. 前端交互契约

### 6.1 平台端发布页

- 路由：`/platform/notice`，菜单分组"系统"。
- 表单字段：标题（必填）、正文（必填）、级别（单选，默认"普通"）、
  投放对象（多选，含"全体租户"选项）。
- 租户选项来自既有 `GET /platform/tenants`。
- 提交成功后刷新列表。

### 6.2 用户端通知中心

- 既有页面 `user-web/src/views/Notifications.vue` **保留**，
  去除"用户端 API 失败 ⇒ 退化成只拉平台最新一条"的 fallback
  （该 fallback 读的是平台 latest，与本模块语义重复且已无必要）。
- 详情弹窗**既有**（`viewVisible` / `current`），补充展示 `level` 标签与 `source`。
- 列表区分平台消息与本地业务通知（`source` 字段）。

### 6.3 用户端铃铛角标

- `user-web/src/layout/Layout.vue` 的 `unreadCount` **接线**：
  轮询 `GET /api/auth/notifications/unread-count`，形状照抄同文件
  `geoAlertCount` 的既有实现。
- 无未读时角标隐藏（`:hidden="unreadCount === 0"` 已就位）。

### 6.4 用户端飘条

- `MessageNotification.vue` 改为读**本地通知中心**最新一条（不再打平台 latest）。
  理由：平台 latest 是全局一条、且已读状态记在平台侧；
  改读本地后与通知中心同源同口径。
