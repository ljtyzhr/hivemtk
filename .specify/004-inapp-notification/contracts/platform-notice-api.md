# 契约：平台端通知接口

> 平台运营端（platform-web）→ 平台端（platform-server）。
> 响应统一信封 `{"code":0,"data":...,"message":"ok"}`。

## 1. 发布平台消息

`POST /platform/message/send`

**权限**：`JWTAuthMiddleware` + `RequirePlatformAdmin` + `PlatformAuditMiddleware`
（挂在既有 `/platform` 组下，自动获得这三道守卫）。

**请求体**

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `title` | string | ✅ | 标题，非空，≤255 字符 |
| `content` | string | ✅ | 正文，非空 |
| `level` | string | ❌ | `info`(默认) / `warning` / `error` / `success` |
| `targets` | []string | ❌ | 目标租户 `tenant_code` 列表；**空或缺省 = 全体租户** |

**成功响应**

```json
{
  "code": 0,
  "data": { "id": 12, "title": "系统维护通知", "published_at": "2026-10-22T10:00:00+08:00" },
  "message": "发送站内信成功"
}
```

**失败响应**

| code | message | 触发 |
|---|---|---|
| 400 | `参数错误: title 与 content 必填` | 标题或正文为空 |
| 400 | `参数错误: title 超长` | 标题 > 255 字符 |
| 400 | `参数错误: level 取值非法` | level 不在枚举内 |
| 500 | `发送站内信失败` | 落库失败 |

**行为变更说明**：既有 `Send` 的 `level` 恒为 `info`（硬编码），
本次改为取入参；`targets` 既有实现只落库不消费，本次让它**真正参与投递过滤**。
`is_read` 语义不变（平台运营视角）。

## 2. 平台消息列表

`GET /platform/message/list?page=1&page_size=20`

**权限**：同上。

**成功响应**

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 12,
        "title": "系统维护通知",
        "content": "……",
        "level": "warning",
        "type": "announcement",
        "targets": "tz3rmdj,tgzgc8d",
        "is_read": false,
        "published_at": "2026-10-22T10:00:00+08:00",
        "created_at": "2026-10-22T10:00:00+08:00"
      }
    ],
    "total": 1
  },
  "message": "获取站内信列表成功"
}
```

`page_size` 上限 100，越界回落 20。

## 3. 单条平台消息详情

`GET /platform/message/:id`

**权限**：同上。

**成功响应**：`data` 为该消息的完整字段（含 `content` 与 `targets`）。
列表接口已带 `content`，本端点主要给"点标题看全文"的详情场景用。

**失败响应**：404 `消息不存在`（id 非法或查不到）。

## 4. 投递台账

`GET /platform/message/:id/deliveries?page=1&page_size=20`

**权限**：同上。

**成功响应**

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 5,
        "message_id": 12,
        "merchant_key": "azbL6ptzRDm3haVkOJE9IHsDU7n0R916",
        "tenant_code": "tz3rmdj",
        "status": "success",
        "error_msg": "",
        "attempted_at": "2026-10-22T10:03:11+08:00"
      }
    ],
    "total": 1
  },
  "message": "ok"
}
```

`status` ∈ `success` / `failed` / `skipped`。
`skipped` = 该租户不在投放目标内。

## 5. 商户侧拉取端点（平台 → 用户端，用户端调用）

`GET /merchant-api/notices/list?tenant_code=<code>&since=<RFC3339>`

**权限**：`MerchantAuth`（HMAC-SHA256）。
签名头 `X-Merchant-Key` / `X-Timestamp` / `X-Signature`，
签名串 = `METHOD\nPATH(不含query)\nTIMESTAMP\nBODY`。

> **签名 PATH 不含 query string** —— 含了就 401。既有
> `module_client.go:213` 就是这么构造的，照此办理。

**Query**

| 参数 | 必填 | 说明 |
|---|---|---|
| `tenant_code` | ✅ | 拉取方租户短码 |
| `since` | ❌ | 增量游标（RFC3339）。缺省 = 全量拉取（首次同步用） |

**成功响应**

```json
{
  "code": 0,
  "data": {
    "list": [
      {
        "id": 12,
        "title": "系统维护通知",
        "content": "……",
        "level": "warning",
        "published_at": "2026-10-22T10:00:00+08:00"
      }
    ],
    "server_time": "2026-10-22T10:03:00+08:00"
  },
  "message": "ok"
}
```

**过滤规则**：`published_at IS NOT NULL`（已发布）
且（`targets` 为空 **或** `targets` 逗号分隔后含 `tenant_code`）
且 `published_at > since`（`since` 为空则不过滤）。

**响应给量**：单次最多返回 200 条，按 `published_at` 升序。
超出时 `data.has_more = true`，用户端下一轮用最新 `published_at` 继续拉。

**失败响应**

| code | message | 触发 |
|---|---|---|
| 400 | `参数错误: tenant_code 必填` | 缺 tenant_code |
| 401 | 签名/时间戳校验失败 | MerchantAuth 拦截 |
| 500 | `查询通知失败` | 落库查询失败 |

> `tenant_code` 不在平台登记表中时返回空列表（不是 404）——
> 未登记的租户就是"没有消息给它"，返回空让用户端安静收工。
