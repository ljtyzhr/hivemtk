# sms.js（#870–#889，20 条）

> 本泳道实例：off `8454` / on `8455`（`hvapi8454/`，二进制 11:36 构建）。令牌 `hvapi/tok`（uit_admin, tenant 1）。
> 数据：草稿 2、任务 1（`apitest 短信任务`）、记录 41；渠道凭据为占位 `AK_TEST/SECRET`（真实外发不可达）。

| 870 | [x] | GET | `/api/sms/config` | 用户端 | - | 200 code=0 双腿一致（secret 掩码 `****TEST`） | `src/api/sms.js` | 5 |
| 871 | [x] | POST | `/api/sms/config` | 用户端 | body: data | 200 code=0 双腿；回写后 GET 值不变（用 DB 原值回写，未改配置） | `src/api/sms.js` | 8 |
| 872 | [x] | GET | `/api/sms/list` | 用户端 | query: params | 200 code=0 total=41；`status=failed` 命中、`status=success` 空集、`phone=13800` 空集（过滤生效） | `src/api/sms.js` | 12 |
| 873 | [x] | GET | `/api/sms/detail/{id}` | 用户端 | path: id | 200 code=0（id=41 全字段）；id=999999 → 404 `NOT_FOUND_1002 记录不存在` | `src/api/sms.js` | 15 |
| 874 | [x] | POST | `/api/sms/resend/{id}` | 用户端 | path: id | 400 `重发短信失败（渠道不可达或凭证无效）: ... Specified access key is not found.`（环境前提：占位凭据）；id=999999 → 404 | `src/api/sms.js` | 18 |
| 875 | [x] | POST | `/api/sms/send` | 用户端 | body: data | 合法体 → 502 渠道错误（环境前提）；`phone=138`/空 content → 400 逐字段校验文案 | `src/api/sms.js` | 21 |
| 876 | [x] | GET | `/api/sms/draft/list` | 用户端 | query: params | 200 code=0 total=19（分页/标题过滤均可） | `src/api/sms.js` | 25 |
| 877 | [x] | GET | `/api/sms/draft/{id}` | 用户端 | path: id | 200 code=0（id=2）；id=1 已软删 → 404 | `src/api/sms.js` | 28 |
| 878 | [x] | POST | `/api/sms/draft` | 用户端 | body: data | 200 code=0 双腿各建一行（22/23）；缺字段 → 400 | `src/api/sms.js` | 31 |
| 879 | [x] | PUT | `/api/sms/draft/{id}` | 用户端 | path: id / body: data | 200 code=0（id=22 标题/内容均更新） | `src/api/sms.js` | 34 |
| 880 | [x] | DELETE | `/api/sms/draft/{id}` | 用户端 | path: id | 200 code=0（22 删两次均 200、999999 也 200，见 F1） | `src/api/sms.js` | 37 |
| 881 | [x] | POST | `/api/sms/draft/{id}/send` | 用户端 | path: id / body: data | `{phone}` → 502 渠道错误（环境前提）；`{}` → 400「手机号不能为空」 | `src/api/sms.js` | 40 |
| 882 | [x] | GET | `/api/sms/job/list` | 用户端 | query: params | 200 code=0 total=1（软删任务不计入） | `src/api/sms.js` | 44 |
| 883 | [x] | GET | `/api/sms/job/{id}` | 用户端 | path: id | 200 code=0（id=1）；999999 → 404 | `src/api/sms.js` | 47 |
| 884 | [x] | POST | `/api/sms/job/{id}/pause` | 用户端 | path: id | 200 code=0（running→paused）；重复暂停 → 400「只能暂停运行中的任务」 | `src/api/sms.js` | 50 |
| 885 | [x] | POST | `/api/sms/job/{id}/resume` | 用户端 | path: id | 200 code=0（paused→running）；非 paused → 400「只能继续已暂停的任务」 | `src/api/sms.js` | 53 |
| 886 | [x] | POST | `/api/sms/job/{id}/stop` | 用户端 | path: id | 200 code=0（running→failed）；已终态 → 400「只能停止运行中、暂停或待执行的任务」 | `src/api/sms.js` | 56 |
| 887 | [x] | POST | `/api/sms/job` | 用户端 | body: data | 200 code=0（建 3/4，落 sms_job_details 1 行）；空字段 → 400 三条校验 | `src/api/sms.js` | 59 |
| 888 | [x] | DELETE | `/api/sms/job/{id}` | 用户端 | path: id | 删 failed 任务 200；删 running 任务 → **500「删除任务失败」**（见 F2）；999999 → 404 | `src/api/sms.js` | 62 |
| 889 | [x] | GET | `/api/sms/job/{id}/records` | 用户端 | path: id / query: params | 200 code=0 total=1（jobId 对齐）；job 已删 → 404 | `src/api/sms.js` | 65 |

## Findings

### F1 `DELETE /api/sms/draft/{id}` 对不存在/已删 id 回 `code=0`
- 两形态一致：删 22 两次都 200，删 999999 也 200。软删 0 行仍算成功（与 A6/A5 同类）。
- 判定：观察项（删除幂等语义可接受），未改。若要与 A2 口径统一，可在 service 加 `FindByID` → 404。

### F2 `DELETE /api/sms/job/{id}` 删运行中任务吞掉业务原因（500）
- 两形态：`running` 任务 → `500 INTERNAL_ERROR_6002 删除任务失败`。
- 根因：`controller/sms.go DeleteJob` 用 `HandleDBError`，而 `service.DeleteJob` 的校验错误是
  `errors.New("只能删除已完成或失败的任务")`（`internal/service/sms.go:828`），非 DB 错误被压成 500。
  同文件 `PauseJob/ResumeJob/StopJob` 已改 `HandleServiceError`（上一轮），删除这条漏了。
- 判定：缺陷（与 A11/A15 同根）。处置：改 `HandleServiceError`，回归期望 400 + 原文案。
