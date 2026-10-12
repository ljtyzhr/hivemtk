# API 违规清单

**扫描日期：** 2026-10-12
**扫描范围：** `hivemtk/user-server/internal/router/`

---

## 1. 路由路径格式违规

### 规范要求
```
/api/{domain}/{resource}           # 用户端
/api/manage/{resource}             # 管理端
```

### 扫描结果

| 路由路径 | 文件 | 是否违规 | 说明 |
|----------|------|----------|------|
| `/health` | router.go:200 | 是 | 健康检查路由，不符合 API 路径规范 |
| `/healthz` | router.go:201 | 是 | 健康检查路由，不符合 API 路径规范 |
| `/readyz` | router.go:202 | 是 | 健康检查路由，不符合 API 路径规范 |
| `/api/ws/visitor` | chat_routes.go:54 | 否 | WebSocket 路由，符合规范 |
| `/share/douyin/:id` | chat_routes.go:97 | 是 | 分享路由，不符合 API 路径规范 |
| `/share/kuaishou/:id` | chat_routes.go:100 | 是 | 分享路由，不符合 API 路径规范 |
| `/share/xiaohongshu/:id` | chat_routes.go:103 | 是 | 分享路由，不符合 API 路径规范 |
| `/share/xianyu/:id` | chat_routes.go:106 | 是 | 分享路由，不符合 API 路径规范 |
| `/files/*filepath` | files_guard.go:31 | 是 | 文件服务路由，不符合 API 路径规范 |
| `/swagger/*any` | swagger.go:41 | 是 | Swagger 路由，不符合 API 路径规范 |
| `/api/swagger.json` | swagger.go:49 | 否 | Swagger JSON，符合规范 |
| `/chat/embed/*path` | embed_static_routes.go:48 | 是 | 嵌入路由，不符合 API 路径规范 |
| `/chat/embed` | embed_static_routes.go:56 | 是 | 嵌入路由，不符合 API 路径规范 |
| `/` | embed_static_routes.go:60 | 是 | 根路由，不符合 API 路径规范 |
| `/index.html` | embed_static_routes.go:84 | 是 | 静态文件路由，不符合 API 路径规范 |
| `/__debug__/routes` | router.go:211 | 是 | 调试路由，不符合 API 路径规范 |

### 结论
发现 14 条路由路径不符合规范。主要分为以下几类：
1. **健康检查路由**（3 条）— `/health`、`/healthz`、`/readyz`
2. **分享路由**（4 条）— `/share/douyin/:id` 等
3. **文件/静态路由**（5 条）— `/files/*filepath`、`/chat/embed/*path` 等
4. **调试路由**（1 条）— `/__debug__/routes`
5. **Swagger 路由**（1 条）— `/swagger/*any`

**建议：** 这些路由属于特殊用途（健康检查、文件服务、调试等），建议在规范中明确特殊路由的路径规范，而不是强制要求所有路由都符合 `/api/{domain}/{resource}` 格式。

---

## 2. 响应格式违规

### 规范要求
```json
{"code": 0, "data": {...}, "message": "ok"}           // 成功
{"code": 400, "message": "参数错误"}                   // 失败
```

### 扫描结果

| 文件 | 行号 | 内容 | 是否违规 |
|------|------|------|----------|
| `controller/bridge_capabilities.go` | 40 | 注释说明直接 `c.JSON` 而非 `response.Success` | 需确认 |

### 结论
需要进一步扫描确认是否有直接 `c.JSON` 调用。从注释来看，存在直接 `c.JSON` 的情况，但需要确认具体代码。

---

## 3. 错误码使用

### 规范要求
```
0=成功 400=参数 401=认证 403=权限 404=不存在 409=冲突 500=服务端
```

### 扫描结果
需要进一步扫描确认错误码使用是否正确。

---

## 总结

| 违规类型 | 数量 | 严重程度 |
|----------|------|----------|
| 路由路径格式 | 14 | 中（特殊路由需规范补充） |
| 响应格式 | 待确认 | 待确认 |
| 错误码使用 | 待确认 | 待确认 |

**主要问题：** 路由路径格式存在违规，但大部分属于特殊用途路由。建议在规范中补充特殊路由的路径规范。
