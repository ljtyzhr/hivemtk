# sopAgent.js（#890–#905，16 条）+ sopTemplate.js（#906–#912，7 条）

> 实例 off `8454` / on `8455`，令牌 `hvapi/tok`。数据：sop_agents 5 条（id 32/33/34/36/37）、sop_executions 2026 行、sop_templates 366 条（4363 起）。

## sopAgent.js

| 890 | [x] | GET | `/api/sop` | 用户端 | query: params | 200 code=0 5 条（sop_graph 完整回传） | `src/api/sopAgent.js` | 5 |
| 891 | [x] | POST | `/api/sop` | 用户端 | body: data | 正确体 → 200 code=0（建 38/39）；**前端实际 body → 400 缺 Scenario**（见 F1） | `src/api/sopAgent.js` | 9 |
| 892 | [x] | GET | `/api/sop/stats` | 用户端 | - | 200 code=0 `{active:5,total:5,success:1187,failed:838,running:0}` 双腿一致 | `src/api/sopAgent.js` | 13 |
| 893 | [x] | GET | `/api/sop/match` | 用户端 | query: params | 200 code=0 `[]`（`intent=price_inquiry` 无命中资产：本机无 active SOP 资产，属环境前提） | `src/api/sopAgent.js` | 17 |
| 894 | [x] | GET | `/api/sop/executions` | 用户端 | query: params | 200 code=0 分页 2026 行；`_approval_token` 已 redacted | `src/api/sopAgent.js` | 21 |
| 895 | [x] | GET | `/api/sop/executions/{id}` | 用户端 | path: id | 200 code=0（id=2026）；999999 → 404「执行不存在」 | `src/api/sopAgent.js` | 25 |
| 896 | [x] | POST | `/api/sop/executions/{id}/pause` | 用户端 | path: id | 200 code=0；**已完成（success）的执行也照收**（见 F3） | `src/api/sopAgent.js` | 29 |
| 897 | [x] | POST | `/api/sop/executions/{id}/resume` | 用户端 | path: id | 200 code=0；999999 → 404「record not found」 | `src/api/sopAgent.js` | 33 |
| 898 | [x] | POST | `/api/sop/executions/{id}/cancel` | 用户端 | path: id | 200 code=0（2026 → canceled） | `src/api/sopAgent.js` | 37 |
| 899 | [x] | GET | `/api/sop/{id}` | 用户端 | path: id | 200 code=0（id=32）；已删/不存在 → 404「SOP 不存在」 | `src/api/sopAgent.js` | 41 |
| 900 | [x] | PUT | `/api/sop/{id}` | 用户端 | path: id / body: data | 正确体 → 200 code=0 version 递增；前端 body 缺 scenario/sop_graph → 400（同 F1） | `src/api/sopAgent.js` | 45 |
| 901 | [x] | DELETE | `/api/sop/{id}` | 用户端 | path: id | 200 code=0；再删/不存在 → 400「sop not found」（见 F4） | `src/api/sopAgent.js` | 49 |
| 902 | [x] | POST | `/api/sop/{id}/activate` | 用户端 | path: id | 200 code=0「启用成功」 | `src/api/sopAgent.js` | 53 |
| 903 | [x] | POST | `/api/sop/{id}/deactivate` | 用户端 | path: id | 200 code=0「停用成功」 | `src/api/sopAgent.js` | 57 |
| 904 | [x] | POST | `/api/sop/execute` | 用户端 | body: data | 200 code=0 建执行（id 2027/2028，dispatcher 自动推进到 success）；重复进入同座席 → 400「sop entry suppressed by entry_policy」（见 F5） | `src/api/sopAgent.js` | 61 |
| 905 | [x] | POST | `/api/sop/step` | 用户端 | body: data | 400「execution is not running」（执行被 dispatcher 秒推进到 success，取不到 running 样本；判据正确） | `src/api/sopAgent.js` | 65 |

## sopTemplate.js

| 906 | [x] | GET | `/api/sop-templates` | 用户端 | query: params | 200 code=0（`keyword`/`enabled` 过滤生效） | `src/api/sopTemplate.js` | 5 |
| 907 | [x] | GET | `/api/sop-templates/{id}` | 用户端 | path: id | 200 code=0（4363）；不存在 → 404「SOP 模板不存在」 | `src/api/sopTemplate.js` | 9 |
| 908 | [x] | POST | `/api/sop-templates` | 用户端 | body: data | 带 agent_id → 200 code=0（4364/4365）；**前端两个入口（Editor 保存 / Market 导入）不带 agent_id → 400 required**（见 F2） | `src/api/sopTemplate.js` | 13 |
| 909 | [x] | PUT | `/api/sop-templates/{id}` | 用户端 | path: id / body: data | off 200 code=0；**SaaS 500「更新不可改写 tenant_id（上下文 1，SET 值 0）」**（见 F6，已修） | `src/api/sopTemplate.js` | 17 |
| 910 | [x] | PATCH | `/api/sop-templates/{id}/toggle` | 用户端 | path: id / body: { enabled } | 200 code=0（4364 → false）；`{}` → 400 required；不存在 → 404 | `src/api/sopTemplate.js` | 21 |
| 911 | [x] | DELETE | `/api/sop-templates/{id}` | 用户端 | path: id | 200 code=0；再删 → 404「record not found」 | `src/api/sopTemplate.js` | 25 |
| 912 | [x] | POST | `/api/sop-templates/match` | 用户端 | body: data | 400 required AgentID（契约如此，前端无调用点）；按 agent 维度匹配，未造绑定样本 | `src/api/sopTemplate.js` | 29 |

## Findings

### F1 `POST/PUT /api/sop` 前端提交必 400（契约错位，未改）
- 后端 `dto.CreateRequest` 必填 `name`+`scenario`+`sop_graph`（`internal/dto/sop.go:84`）。
- 前端 `sopAgent/List.vue:1176` 提交的是 `{name, description, trigger_intent, status, nodes}`：
  无 `scenario`、无 `sop_graph`（节点放在顶层 `nodes`，后端忽略）。实测两形态均
  `400 请求参数错误: Key: 'CreateRequest.Scenario' ... required`。
- 判定：前端契约错位（与 A3 同类）。修法需产品定：表单要加「场景」，并把 `nodes` 包进
  `sop_graph:{nodes,edges,entry}`、`trigger_intent` 落 `trigger_type/trigger_config`。
  本轮只记录，未擅自改页面语义。

### F2 `POST/PUT /api/sop-templates` 前端必 400（契约与设计冲突，未改）
- 后端 `sopTemplateCreateReq.AgentID` 是 `binding:"required"`，service 还有 Task 16
  强 1对1 校验（`service/sop_template.go:212`）；前端 `sopTemplate/Editor.vue:345`
  与 `Market.vue:544` 都没有 agent 概念，从不发 `agent_id`。
- 实测：前端同形 body → 400 `AgentID ... required`；补 `agent_id=32` → 200。
- 判定：产品口径冲突（同 A9）。要么前端加「归属座席」选择器，要么后端放行全局模板。
  本轮未改（改后端等于推翻 Task 16 语义）。

### F3 `pause/resume/cancel` 不校验执行状态
- `success` 的 2026 被依次 pause→resume→cancel 全部 200，最终态 `canceled`
  （`service/sop.go` 的 Pause/Resume/Cancel 只按 id 改状态，无前置状态判断）。
- 判定：业务逻辑偏松，观察项（未改）：终态执行被"暂停/恢复"会让统计口径漂移。

### F4 `DELETE /api/sop/{id}` 不存在时回 400 + 英文「sop not found」
- 同形态下应 404（`GetExecution` 已经分了 404/500）；这里把 service 的英文
  `ErrSOPSNotFound` 直接 `response.Error(400)` 抛给前端。观察项。

### F5 `POST /api/sop/execute` 拒绝文案是内部枚举
- 重复进入同 (sop, customer) 时返回 `400 sop entry suppressed by entry_policy`
  —— 英文 + 内部策略名，用户读不懂。观察项（文案）。

### F6 `PUT /api/sop-templates/{id}` 在 SaaS 形态 500（已修）
- 现象（仅 on 腿）：`500 [tenant] 更新不可改写 tenant_id（上下文租户 1，SET 值 0）`。
- 根因：`repository/sop_template.go Update` 用 `Select("*").Updates(struct)`，
  body 里没有 TenantID ⇒ 零值 1→0 被租户插件 fail-closed 拒绝；同一写法还会把
  `hit_count` 清零。与 A13（faq_entry）完全同根。
- 处置：改为只写表单拥有的列（name/intent/stage/template/vars/priority/confidence/
  updated_at + 非空时的 enabled/agent_id），tenant_id 与 hit_count 不再被覆盖。
- 回归：待本泳道重建二进制后复核（on 腿 PUT → 200 且 hit_count 不变）。
