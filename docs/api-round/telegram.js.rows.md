# telegram.js（#932–#944，13 条）+ templateMarket.js（#945–#947，3 条）

> 数据：telegram account id=9（`lovesanrenxing_bot`）、gate id=4；测试数据 `api26_*` 已删净。

| 932 | [x] | GET | `/api/telegram/accounts` | 用户端 | query: params | 200 code=0 分页（token 已掩码） | `src/api/telegram.js` | 4 |
| 933 | [x] | GET | `/api/telegram/accounts/{id}` | 用户端 | path: id | 200 code=0（id=9）；999999 → 404「账号不存在」 | `src/api/telegram.js` | 8 |
| 934 | [x] | POST | `/api/telegram/accounts` | 用户端 | body: data | 合法格式 token → 200 code=0（建 10，后台验证异步）；`bad-token` → 400 精确文案（缺 `:` / 长度 35 位） | `src/api/telegram.js` | 12 |
| 935 | [x] | PUT | `/api/telegram/accounts/{id}` | 用户端 | path: id / body: data | 200 code=0（改名 + ai_agent_enabled 生效，token 保留） | `src/api/telegram.js` | 16 |
| 936 | [x] | DELETE | `/api/telegram/accounts/{id}` | 用户端 | path: id | 200 code=0；再删 → 404「账号不存在」 | `src/api/telegram.js` | 20 |
| 937 | [x] | POST | `/api/telegram/accounts/{id}/register-webhook` | 用户端 | path: id / body: data | 400「WebhookURL 格式不合法 / scheme 必须是 https」（环境前提：本机无 https 公网域名） | `src/api/telegram.js` | 24 |
| 938 | [x] | POST | `/api/telegram/accounts/{id}/test-send` | 用户端 | path: id / body: data | `chat_id` 传字符串 → 400 明确类型错；传 int64 → 请求 20s 无响应（真实调 api.telegram.org，本机出网受限，环境前提） | `src/api/telegram.js` | 28 |
| 939 | [x] | GET | `/api/telegram/gates` | 用户端 | query: params | 200 code=0（`account_id=9` 过滤生效） | `src/api/telegram.js` | 34 |
| 940 | [x] | POST | `/api/telegram/gates` | 用户端 | body: data | 新建 → 200 code=0（5/6）；**同 chat_id 重复配置 → 500「创建失败（群组可能已配置）」**（见 F1） | `src/api/telegram.js` | 38 |
| 941 | [x] | PUT | `/api/telegram/gates/{id}` | 用户端 | path: id / body: data | 200 code=0（按前端全量 payload；只发可编辑字段会被 account_id/chat_id 的 required 拦成 400，见 F2） | `src/api/telegram.js` | 42 |
| 942 | [x] | DELETE | `/api/telegram/gates/{id}` | 用户端 | path: id | 200 code=0；再删 → 404「删除失败」 | `src/api/telegram.js` | 46 |
| 943 | [x] | GET | `/api/telegram/gates/{id}/members` | 用户端 | path: id / query: params | 200 code=0 `{list:[],total:0}` | `src/api/telegram.js` | 50 |
| 944 | [x] | POST | `/api/telegram/gates/{id}/members/{memberId}/authorize` | 用户端 | path: id, memberId | 404「放行失败」（无该成员台账，判据正确） | `src/api/telegram.js` | 54 |
| 945 | [x] | GET | `/api/template-market/list` | 用户端 | query: params / body: config | 200 code=0 目录数据（`card-douyin` 等，公开档） | `src/api/templateMarket.js` | 15 |
| 946 | [x] | GET | `/api/template-market/categories` | 用户端 | body: undefined; config | 200 code=0 9 个分类 | `src/api/templateMarket.js` | 18 |
| 947 | [x] | GET | `/api/template-market/detail/{id}` | 用户端 | path: id | `card-douyin` → 200 code=0；**不存在 id → HTTP 200 + code=4004「模板/技能不存在」**（见 F3） | `src/api/templateMarket.js` | 22 |

## Findings

### F1 `POST /api/telegram/gates` 重复群组回 500（观察，未改）
- 同 (account_id, chat_id) 重复配置 → `500 INTERNAL_ERROR_6002 创建失败（群组可能已配置）`
  （`controller/telegram_gate.go:127` 用 `ErrorFromDB`）。文案已提示可能原因，状态码仍误导成服务故障。
  属"预期内的唯一键冲突"应回 409/400。同 A12 家族；本轮只记录。

### F2 `PUT /api/telegram/gates/{id}` 复用创建 DTO 的 required 校验（观察）
- `telegramGateReq` 的 `account_id`/`chat_id` 是 `binding:"required"`，而 Update 实际不写这两列。
  只提交可编辑字段 → 400。前端 `telegram/gate.vue:410` 全量 `{...gateForm}` 提交所以能过；
  但这是个"改一个标题要带全部必填"的隐形契约。观察项。

### F3 `GET /api/template-market/detail/{id}` 不存在时 HTTP 200 + code=4004（缺陷，未改）
- 两形态一致：`200 {"code":4004,"message":"模板/技能不存在"}`。前端按 `code` 判断即可，
  但 HTTP 层把"没这条"读成"请求成功"，监控/日志/网关缓存都会误判。
- 处置建议：`response.NotFound`；本轮未改（涉及前端 `request.js` 对 4xx 的统一 toast 行为，
  需要一起看，先记录）。

### 备注：test-send 反向依赖
- `POST /api/telegram/accounts/{id}/test-send` 会真实调用 `api.telegram.org`；本机出网受限时
  请求挂到 20s 超时（curl 000）。这是唯一一条"连 server 都没回"的路由，属环境前提，不是后端缺陷。
