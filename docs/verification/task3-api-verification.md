# 任务三 · SaaS 多租户改造后 API 逐路由 curl 验证记录

> 仓库：`hivemtk`（gitee-upstream=xhpmayun / upstream=xiaofang142）
> 起止：2026-10-22
> 清单：`api-list.md`（`scripts/gen_api_list.py` 生成，1035 条）

## 验证环境

| 档 | TENANT_MODE | 监听 | 租户 | 登录账号 |
|---|---|---|---|---|
| 离线单机 | `off` | `http://127.0.0.1:8211` | tenant_id=1（默认租户） | `apitest_off_admin` / `ApiTest@123456` |
| SaaS 多租户 | `on` | `http://127.0.0.1:8204` | tenant_id=17（`t6da6jw`，API测试租户B） | `apitest_admin_b` / `ApiTest@123456` |

- 两档同库（PG 127.0.0.1:8202 / Redis 8203），故数据层隔离差异可被直接对照。
- SaaS 档的开户走公开端点 `POST /api/tenant/register`（token 随注册响应下发）；
  离线档该端点按设计回 **4104**「当前为离线部署模式，注册未开放」——双形态行为分叉已实测确认。
- 租户隔离实测：SaaS 档用 tenant 17 的令牌读 tenant 1 的行（`/api/ai-agents/73`、
  `/api/ab-experiments/22`）均回 **404**，列表接口回空集 ⇒ 行级隔离生效。

## 本轮发现并修复的缺陷

### BUG-01 · 批量操作页「预览 / 执行」两个按钮永久 400（前后端契约错位）

**路由**：`POST /api/batch/preview`（前端 `previewBatch`）、`POST /api/batch/import`（前端 `runBatch`）、
`POST /api/batch/export`（前端 `batchExport`）、`POST /api/batch/delete` / `/api/batch/update`（`ids` 类型）

**现象**：后端四个端点的入参形状各不相同，且**都不接受**前端页面
（`user-web/src/views/batchOperation/List.vue`）原本发的那套字段：

| 后端端点 | 实际入参 | 页面发的 |
|---|---|---|
| `POST /api/batch/preview` | `{operation_type, data_type, ids: string[]}` | `{tool, source, filter, params, dryRun}` |
| `POST /api/batch/import` | multipart：`file`(必填) + `type`(表单字段) | JSON |
| `POST /api/batch/export` | 表单字段：`type` / `format` / `ids`(逗号分隔) | JSON |
| `POST /api/batch/delete` | `{ids: string[]}` | `data` 透传（`ids` 可能是 number[]） |
| `POST /api/batch/update` | `{ids: string[], fields: {k:string}}` | `data` 透传 |

⇒ 「预览」恒 400「Key: 'PreviewRequest.OperationType' failed on the 'required' tag」；
「执行」恒 400「上传文件失败：request Content-Type isn't multipart/form-data」。

**判定**：调用错误（契约错位），不是业务错误。两个按钮 100% 不可用。

**处置**：后端契约是事实源，翻译放在 `user-web/src/api/batchOperation.js` 这一层做：
- `operation_type` ← `tool`（后端 `GET /api/batch/tools` 返回的 name 恰是
  import/export/delete/update，与页面卡片同名）
- `data_type` ← `'clue'`（后端 `PreviewRequest` 只认 clue/user/account，而这四个工具
  在服务端作用的都是线索：`ImportTypeClue` / `ExportTypeClue` / `BatchDeleteClues` / `BatchUpdateClues`）
- `ids` ← 由 `source`（all/filter/import）+ `filter` 解析：走 `GET /api/clues/list`
  取 id，**有界**（上限 500 条；超过就报错要求收窄筛选，不静默截断——
  截断等于删掉一半数据，比报错更坏）
- `ids` 一律归一成**字符串数组**（后端是 `[]string`，前端传 number 会被反序列化拒掉）
- `export` 改发 `application/x-www-form-urlencoded`（后端读 `ctx.PostForm`）
- `import` 改发 multipart

页面侧（`views/batchOperation/List.vue`）：
- 预览读后端真实响应字段 `selected_count`（原先读的是不存在的 `total`/`totalCount`）
- 「执行」失败时透出后端原话（原代码换成了「执行失败」四个字，把唯一有用的信息吃掉）

### BUG-03 · 批量导入的文件上传不带 Authorization ⇒ 401

**路由**：`POST /api/batch/import`（页面 `el-upload action="/api/batch/import"`）

**现象**：`el-upload` 用 `action=` 直连时发的是**裸 XHR**，不经过 axios 拦截器 ⇒
**不带 `Authorization` 头** ⇒ 后端回 401「未提供认证令牌」；且这个 401 不进全局
错误处理，页面上看不出原因。

实测（不带令牌直发该端点）：`{"code":"UNAUTHORIZED_2001","message":"未提供认证令牌"}`。

**判定**：调用错误（认证头缺失），不是业务错误。

**处置**：改用 `el-upload` 的 `:http-request` 自定义上传，走 api 层
（`batchImportFile`）⇒ 由 axios 拦截器统一带令牌、统一处理业务码与错误提示。

---

### BUG-02 · 渠道 `allowed_origins` 写接口不认字符串形态（读接口吐字符串，写接口只收数组）

**路由**：`POST /api/chat-channels`（#127）、`PUT /api/chat-channels/{id}`（#128）

**现象**：入参 DTO `service.CreateChannelRequest.AllowedOrigins` / `UpdateChannelRequest.AllowedOrigins`
声明为 `[]string`，而
- 存储层 `model.ChatChannel.AllowedOrigins` 是 **text**（逗号分隔串）；
- 读接口 `GET /api/chat-channels/:id` 返回的 `allowed_origins` 是**字符串**（`"*"`）；
- 前端 `Create.vue` / `Edit.vue` 写侧发**数组**（`originsText.split('\n').map(...).filter(Boolean)`），
  读侧 `Edit.vue:225` 却按**字符串**处理（`res.allowed_origins.split(',')`），
  `InstallGuide.vue:289` 的 `originsToText` 更是同时兼容两种形状（说明调用方对形状并不确定）。

⇒ 「照着读到的原样回写」这条极常见的客户端动作被反序列化直接拒掉：
`json: cannot unmarshal string into Go struct field CreateChannelRequest.allowed_origins of type []string`。

**判定**：调用错误（写接口与读接口/存储层口径不一致），不是业务错误。

**处置**：新增 `service.AllowedOriginsInput` 自定义类型，`UnmarshalJSON` 同时吃下
**字符串**与**数组**两种形状（数字/对象等非法形状显式拒绝），归一化后一律以逗号分隔串落库
（与列类型一致）；`MarshalJSON` 统一吐字符串（与读接口口径一致）。
配套回归用例 `user-server/internal/service/chat_channel_allowed_origins_test.go`（8 个子用例）。

**验证**：
- 字符串形态 `{"allowed_origins":"https://x.com,https://y.com"}` ⇒ 200，落库 `https://x.com,https://y.com`
- 数组形态 `{"allowed_origins":["https://a.com"]}` ⇒ 200（回归，未破坏既有调用方）
- 空值 `{"allowed_origins":""}` ⇒ 400「至少配置一个允许的 origin」（原有闸门未被削弱）
- 非法形态（数字/对象/布尔）⇒ 反序列化拒绝

---

### BUG-04 · 流失预测写入用 `Save(整结构体)` ⇒ 行落 tenant_id=0，任何租户都读不到

**路由**：`GET /api/churn-prediction`（读侧）、`GET /api/churn/risk-distribution`、
`POST /api/user-segment/rfm/calculate`（间接触发）

**现象**：`ChurnPredictionRepository.Upsert` 用 `Save(prediction)` —— Save 把**整个结构体**
写回，包括调用方从未赋值的 `tenant_id`（零值 0）。实测数据：
`SELECT tenant_id, count(*) FROM churn_predictions GROUP BY tenant_id` ⇒
**`0 | 121`**（全部 121 行都是孤儿行，tenant_id=1 的行一条都没有）。

- on 模式：UPDATE 带 tenant_id=0 ⇒ 撞「更新不可改写 tenant_id」闸门 ⇒ 写入链路 500；
- 落库形态：行带 tenant_id=0 ⇒ **不属于任何租户**，任何租户都读不到
  （连离线单机档的默认租户 1 也读不到）⇒ 这批流失预测对所有人隐身。

**判定**：调用错误（写入层用错了 GORM API），不是业务错误。

**注意**：同文件的 `ChurnModelConfigRepository.Upsert` 上一轮已按同样口径修过
（`Select(...)` 定向更新），**churn_predictions 被漏掉了** —— 同一文件里两个同形缺陷。

**处置**：`Upsert` 改为「按主键定向更新 + 显式列出业务列」，`tenant_id` 交给隔离
callback（INSERT 填、UPDATE 不许改），`updated_at` 交给 GORM 的 autoUpdateTime；
并把行真实的 `tenant_id` 回填进内存结构体（调用方没义务知道行归属，但下游要拿
它去写别的表时，零值会被「写入 tenant_id 与上下文租户不一致」那道闸拒掉）。

**回归**：`user-server/internal/ops/repository/churn_prediction_repo_tenant_test.go`
（同租户重复 Upsert 成功且归属不变；另一租户读不到）。

### BUG-05 · 流失预测定时任务没走租户扇出 ⇒ on 模式整轮被守卫拒绝

**路由**：`POST /api/user-segment/rfm/calculate` 的上游数据来自该 cron

**现象**：`ChurnCalculationCron` 直接用 `context.Background()` 调
`RunChurnCalculationForAllCustomers`，而同文件的活码轮询 / 过期令牌清理都走
`fanoutTask`（按活跃租户逐个绑定 ctx）。而 `customer_events` / `churn_predictions`
都带 `tenant_id` 列：on 模式下 ctx 未绑定租户即被隔离守卫 fail-closed 拒绝。

**实测（把修复回退后跑回归用例）**：
```
[tenant] 表 customer_events 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行
```
⇒ 整轮流失计算读不到任何行为数据、算不出任何预测；生产表现是「流失预测页面
永远是空的」，而接口层完全无感（失败只落在 cron 的一行日志里）。

**判定**：调用错误（后台任务缺租户注入通道），不是业务错误。

**处置**：改走 `tenant.Fanout`（与同文件其它 cron 同一通道）；计数用 Fanout 自己
跨租户求和的返回值，不在回调里自己累加（那是实现细节，不是契约）。

**回归**：`user-server/internal/pkg/cron/cron_churn_tenant_test.go`
（注入租户 12 + 种一条行为事件 ⇒ cron 跑完该租户名下必须落一条预测；
读数 0 ⇒ 扇出没生效）。**已验证该用例在修复前失败、修复后通过。**

### BUG-06 · 存量孤儿行（tenant_id=0）没有任何人收拾 ⇒ 永久隐身

**现象**：BUG-04 修复前写进去的 121 行 churn_predictions 孤儿行 + 107 行 customer_rfm
孤儿行（后者是 tenant-1 同业务键行的重复副本）留在库里，谁都读不到。

**处置**：新增迁移 `v3.76.0`（`tenant_orphan_row_repair_migration.go`）收拾存量：
1. 先**删**「目标租户已有同一业务键」的孤儿行（重复副本；先归位会撞唯一键 23505，
   整轮迁移失败 —— 这是实测踩到的：首版迁移就是这么失败的）；
2. 再把**没有对应行**的孤儿行归位到默认租户 1。

`Down` 不回滚（把行改回 tenant_id=0 等于把它们重新变成谁也读不到的孤儿行）。
**回归**：`tenant_orphan_row_repair_migration_test.go`（重复副本被删且目标行业务列
没被动过；真孤儿被归位；第二遍跑幂等不报错）。

**实测（迁移跑完）**：`churn_predictions` 121 行 → tenant_id=1；`customer_rfm`
孤儿 107 行清零，剩 121 行在 tenant 1。

---

### BUG-07 · 抖音卡片「编辑」永远存不进去（路由无 :id，控制器却读路径参数）

**路由**：`PUT /api/douyin/update`（前端 `updateDouyinCard`）

**现象**：真实路由是 `PUT /api/douyin/update`（**没有** `:id` 路径参数，ID 在 body 里），
而控制器无条件读 `ctx.Param("id")` ⇒ 空串 ⇒ `ParseUint("")` 报
`strconv.ParseUint: parsing "": invalid syntax` ⇒ 恒 400「无效的 ID 格式」
⇒ **抖音卡片的编辑页 100% 存不进去**。

**判定**：调用错误（控制器与本仓自己的路由形状对不上），不是业务错误。

**对照**：`/api/kuaishou/update` 的控制器**早就**处理了这条（`if idStr := ctx.Param("id"); idStr != ""`），
`/api/xianyu/update` 与 `/api/xiaohongshu/update` 的控制器压根不读路径参数（用 body 的 ID）——
**只有抖音这一处漏了**。三处形态各不同，所以这次统一成快手那份口径：
有 `:id` 才做一致性校验（顺带保留「JSON 与 URI 的 ID 不匹配」那道闸），
无 `:id` 时用 body 的 ID，两者都没有 ⇒ 400 且文案说清「id 必填」。

**验证**：
- `PUT /api/douyin/update {"id":61,"title":"..."}` ⇒ 200「更新成功」（修复前恒 400）
- `PUT /api/douyin/update {"title":"x"}` ⇒ 400「无效的 ID 格式 / id 必填」
- `PUT /api/douyin-card/61 {...}`（带 :id 的那条路由）⇒ 仍 200，未被改坏

**回归**：`user-server/internal/controller/douyin_card_test.go`
`TestDouyinCardController_Update_WithoutIDInPath`（路由按真实形状注册，不带 :id）。

---

### BUG-08 · 知识库连接器「资源不存在」被报成 500 / 静默成功

**路由**：`PUT /api/kb-connectors/:id`、`DELETE /api/kb-connectors/:id`、
`POST /api/kb-connectors/:id/sync`

**现象**（实测）：

| 端点 | 修复前 | 修复后 |
|---|---|---|
| `PUT /api/kb-connectors/999` | **200「已更新」**（什么都没改） | 404「连接器不存在」 |
| `DELETE /api/kb-connectors/999` | **200「已删除」**（什么都没删） | 404「连接器不存在」 |
| `POST /api/kb-connectors/999/sync` | **500「连接器不存在」** | 404「连接器不存在」 |

**判定**：调用错误（资源不存在被当成服务端故障 / 静默成功），不是业务错误。

**根因**：
- 仓储层 `Update`/`Delete` 用 GORM 的 `Updates`/`Delete`，**0 行受影响时不返回错误**
  ⇒ 改/删一个不存在的连接器回 200，页面提示「已更新」，实际什么都没发生；
- 控制器把服务层的错误一律落 `response.ErrorFromDB` ⇒ 500。

**处置**：
- 仓储层：`RowsAffected == 0` ⇒ 返回 `gorm.ErrRecordNotFound`（「不存在」是事实，
  不该被静默吞掉）；
- 控制器：`errors.Is(err, gorm.ErrRecordNotFound)` ⇒ 404（`NotFoundError`）；
- `kbconnector.Sync` 原本已经 `%w` 包了底层 err（是 `gorm.ErrRecordNotFound`），
  但控制器没按「不存在」判 ⇒ 补上后即可 404。

**回归**：`user-server/internal/controller/kb_connector_notfound_test.go`
（三个端点 × 不存在的 id ⇒ 404）。

### BUG-09 · 初始超管受保护这条业务规则被报成 500

**路由**：`PUT /api/system/permissions/:id/password`

**现象**：打 id=1（初始超管）时回
**500 `INTERNAL_ERROR_6002`「初始超管账号的密码不允许通过此入口修改（系统级保护）」**。
这条 500 与数据库毫无关系 —— 它是一条**明确的业务规则**（服务层已有哨兵
`service.ErrInitialAdminProtected`），而且**其它三个改密入口都回 400**
（自助改密 `auth.go`、管理员重置 `auth.go`、邮件重置 `password_reset.go`），
只有权限管理这一个入口回 500。

**判定**：调用错误（错误码映射漏了一个分支），不是业务错误。

**根因**：`writePermissionServiceError` 只把 `ErrInvalidInput` 判成 400，
其余一律 `response.ErrorFromDB` ⇒ 500。

**处置**：把 `ErrInitialAdminProtected` 一并判成 400。

**回归**：`user-server/internal/controller/permission_error_mapping_test.go`。

---

### BUG-10 · 邮件草稿「编辑」永远存不进去（更新接口只认 body 里的 id）

**路由**：`PUT /api/email/drafts/{id}`（前端 `updateDraft(id, data)`）

**现象**：前端把 id 放在**路径**上（`PUT /api/email/drafts/${id}`）、body 里没有，
而控制器只读 **body** 里的 `dto.UpdateEmailDraftRequest.ID` ⇒ 拿到空串 ⇒
恒 400「无效的 ID 格式」⇒ **邮件草稿的编辑页 100% 存不进去**。

**判定**：调用错误（同一个资源的三个出口有两套 id 口径），不是业务错误。

**对照**：同一个资源的 `GET /api/email/drafts/:id` 与 `DELETE /api/email/drafts/:id`
读的都是**路径参数**（`uri:"id"`），只有 PUT 读 body —— 漏网的正是这一个。

**处置**：PUT 改为以**路径参数**为准（与 GET/DELETE 一致），body 里的 id 留作兜底；
两者都带且不一致 ⇒ 400「路径参数与请求体中的草稿 ID 不一致」（不许悄悄改到别的草稿上）。

**回归**：`user-server/internal/controller/email_draft_id_source_test.go`
（真实控制器 + 真实库：id 只在路径上 ⇒ 200 且草稿真被改到；两处一致 ⇒ 200；
两处不一致 ⇒ 400）。

---

## 环境前提（影响判定口径，务必先读）

本机 **LLM 推理栈没有起**：
- `models/llm/` 目录是空的（只有 `models/embedding/bge-m3-Q4_K_M.gguf` 与
  `models/rerank/bge-reranker-v2-m3-q4_k_m.gguf`），8208（embedding）与 8209（rerank）
  的 llama-server 在跑，**8207（LLM）没有进程**；
- `GET /health` 因此报 `inference: down / no healthy LLM provider`；
- `llm_providers` 里启用的是 `local-mlx → http://127.0.0.1:8207/v1`。

⇒ **凡是真正要调 LLM 的端点，本轮一律回 500 `INTERNAL_ERROR_6002`
（如 `POST /api/geo/config/optimize`）**。这是**环境前提缺失**，不是代码缺陷，
不计入本轮的缺陷清单；判据是「同一个端点在有 LLM 时能不能通」，本轮不成立。
需要 LLM 的结论要等推理栈起来后复测。

## 已确认「不是缺陷」的项（记录口径，避免下轮重复判）

| 路由 | 现象 | 判定 |
|---|---|---|
| `GET /api/approvals` | 404 `{"error":"not found"}` | **无此路由**：controller 刻意不提供列表口（待办列表的唯一事实源是 `/api/human-tasks`）。前端 `approval.js` 也没有 `list()`。 |
| `GET|POST /api/approvals/{id}`(/decide) | 503 `SERVICE_UNAVAILABLE_6003` | **设计如此**：`FF_LTC_APPROVAL_RESUME=off` ⇒ 审批运行时不装配。刻意回 503 而非 404（404 会被读成「路由没挂」）。 |
| `POST /api/asset-bundle/{id}/submit-platform` | 403 `FORBIDDEN_2002` | 平台集成未启用（`PLATFORM_ENABLED=false`），属预期。 |
| `POST /api/v1/asset-market/purchase|sync|report-usage` | 403 `FORBIDDEN_2002` | 同上，平台集成未启用。 |
| `GET /api/ai-tools/{name}` 等 | 404 `NOT_FOUND_1002` | 工具名不存在（该端点按 tool_name 查）。 |
| `POST /api/batch/export` | 400「请至少指定一个要导出的记录 ID（ids）」 | 后端有**防空全量导出**的硬闸（数据暴露防护），400 是正确行为。 |
| `GET /api/ai-productivity/*`、`/api/ai/sales-cockpit` | 200 | 通过。 |
