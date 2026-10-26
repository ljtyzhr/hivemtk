# stats.js（#913）+ system.js（#914–#916）+ systemUser.js（#917–#921）+ tenant.js（#948）

| 913 | [x] | GET | `/api/system/info` | 用户端 | - | 200 code=0 含 uptime/server_time/hostname/go_version（带 token；`platform_enabled=false`） | `src/api/stats.js` | 7 |
| 914 | [x] | GET | `/api/system/info` | 用户端 | body: config | 200 code=0（同 913；axios config 不是 body，公开端点匿名也 200） | `src/api/system.js` | 6 |
| 915 | [x] | GET | `/api/system/config` | 用户端 | - | 200 code=0 全字段（tenant 1 一行） | `src/api/system.js` | 9 |
| 916 | [x] | POST | `/api/system/config` | 用户端 | body: data | 整份回写 → 200 code=0；**只传部分字段会把未传布尔写成 false**（见 F1，已按原值复原） | `src/api/system.js` | 12 |
| 917 | [x] | GET | `/api/system/users` | 用户端 | query: params | 200 code=0 分页（page/size 生效，实测 id 62 等） | `src/api/systemUser.js` | 4 |
| 918 | [x] | GET | `/api/system/users/{id}` | 用户端 | path: id | 200 code=0（id=62）；999999 → **400「用户不存在: service: invalid input」**（见 F2） | `src/api/systemUser.js` | 8 |
| 919 | [x] | POST | `/api/system/users` | 用户端 | body: data | 200 code=0（建 90 staff）；同用户名再建 → 400「用户名已存在」 | `src/api/systemUser.js` | 12 |
| 920 | [x] | PUT | `/api/system/users/{id}` | 用户端 | path: id / body: data | 200 code=0（改名/改邮箱回显新值） | `src/api/systemUser.js` | 16 |
| 921 | [x] | DELETE | `/api/system/users/{id}` | 用户端 | path: id | 200 code=0（删 90）；再删/999999 → 400「用户不存在」 | `src/api/systemUser.js` | 20 |
| 948 | [x] | GET | `/api/tenant/info` | 用户端 | - | 200 code=0 `{tenant_id:1,tenant_code:"tei6385",name,status}` 双腿一致 | `src/api/tenant.js` | 8 |

## Findings

### F1 `POST /api/system/config` 是整份覆盖且无入参校验（观察，未改）
- 实测：只发 `{"theme_color":"not-a-color"}` → 200，`enable_register` 由 true 变 false
  （未传的指针/布尔取零值整份写回），`theme_color` 也接受任意字符串。
- 判定：与 A13 同根的"全量写回"语义；因为该接口明确定位为"整份保存配置"，
  且前端总是回传完整对象（`views/system/Config.vue`），本轮只记录不改。
- 复原：已用原值回写，DB `system_config(id=1,tenant_id=1)` 恢复
  `theme_color=#2f5985, enable_register=t, enable_rag=f`。

### F2 系统用户"不存在"回 400 且文案泄漏内部前缀（观察，未改）
- `GET/PUT/DELETE /api/system/users/{不存在id}` → `400 INVALID_PARAM_1001 用户不存在: service: invalid input`。
- 判定：应 404；文案把 `service: invalid input` 抖给前端。与 A2 类口径不一致，观察项。
