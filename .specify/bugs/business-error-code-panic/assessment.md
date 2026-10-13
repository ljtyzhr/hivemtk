# Bug Assessment: 文档规定的业务错误码 10001 触发 `panic: invalid WriteHeader code`

- **Slug**: business-error-code-panic
- **Created**: 2026-10-13
- **Source**: pasted text（本轮由代码取证 + 本地复现产生，非外部 issue）
- **Verdict**: valid（已复现）
- **Severity**: high

## Report (verbatim or summarized)

仓库最高编码准则 `CLAUDE.md` 的「API 规范 → 错误码」段落规定：

```
# 业务错误码（10000-19999）
10001=用户不存在 10002=客户不存在 10003=订单不存在
10010=余额不足 10011=库存不足 10012=权限不足
```
（`CLAUDE.md:100-102`，由 `8a6d0c03 docs(audit): hivemtk 编码规范审计与规范文档更新` 引入，2026-10-12）

按这一条写出的最自然调用是 `response.Error(c, 10001, "用户不存在")`，实测请求直接 panic。

## Symptom

开发者或编码 Agent 依 `CLAUDE.md` 规定的业务错误码返回错误时，`response.Error` 把业务码当 HTTP
状态码传给 gin，进程 panic（`invalid WriteHeader code 10001`），该请求拿不到任何响应体；
预期是按文档语义返回一条带业务码的错误响应。

## Reproduction

1. `cd user-server`，取一笔含 `CLAUDE.md:100-102` 的提交（本轮测于 `15ccabb7`）。
2. 写一枚最小用例：`gin.CreateTestContext(httptest.NewRecorder())` 上调
   `response.Error(c, 10001, "用户不存在")`。
3. `go test ./internal/pkg/utils/probe/ -run TestDocumentedBusinessCode10001 -v -count=1`
4. 实测输出（本轮 09:41 现取）：

```
=== RUN   TestDocumentedBusinessCode10001
--- FAIL: TestDocumentedBusinessCode10001 (0.00s)
panic: invalid WriteHeader code 10001 [recovered, repanicked]
...
hivemtk-user/internal/pkg/utils/response.Error(...)  response.go:130
```

`httptest.ResponseRecorder` 与真实 `net/http` 服务端都会拒绝 ≥1000 的状态码，因此这一条不只在
测试里红——线上同样把请求打成 500/断连。

[NEEDS CLARIFICATION: 是否有前端/调用方已经在按 10000-19999 这一族码值做分流？本轮全仓检索
`10001|10010|业务错误码` 在 Go 代码里的唯一命中是 `internal/service/channel_error.go` 的
`100001/100007/100016/10004`，那是 QQ 官方**渠道**错误码表，与 `CLAUDE.md` 这一族无关。]

## Suspected Code Paths

- `CLAUDE.md:95-103` — 规范本体，定义了代码里不存在的 10000-19999 业务码段（根因所在）
- `user-server/internal/pkg/utils/response/response.go:100-131` — `Error(c, code any, …)`：
  `case int:` 直接把该 int 当 `httpCode` 传给 `c.JSON`（L112-114、L130），无任何范围校验
- `user-server/internal/pkg/utils/response/response.go:191-212` — `errorCodeFromHTTPCode`：
  非 HTTP 码落到 `default` → `ErrorCodeUnknown`，即业务码进这条路径必然丢失语义
- `user-server/internal/pkg/utils/response/response.go:262-281` — `ErrorWithBusinessCode`：
  **其注释已经写明本缺陷**（「业务错误码（如 5001/6001）只能放在响应体 code 字段，不能作为 HTTP
  状态码（gin 会 panic: invalid WriteHeader code）」）——正解通道的存在反证 `CLAUDE.md` 写错了
- `user-server/internal/domain/errors/codes.go:4-18` — 真实业务码登记表：`0/4000/4001/4003/4004/
  4009/5000/5001/6001-6005`，全是 4 位，与 `CLAUDE.md` 的 5 位段完全不同
- `docs/architecture/adr/ADR-009-error-handling.md:3,25,50-58,66-70` — 同一主题的第三份互相矛盾的
  口径（见下 Root Cause）

## Root Cause Hypothesis

置信度：high。

根因不是某处调用写错，而是**规范文档与代码契约脱节，且 `response.Error` 的 `code any` 入参对
"HTTP 状态码"与"业务错误码"两种语义不设防**。项目里其实有两条正解通道：HTTP 语义走
`Error(c, utils.ErrorCode|httpStatus, msg)`，业务码走 `ErrorWithBusinessCode(c, bizerr.CodeXxx, msg)`
（响应体 `code`，HTTP 恒 200）。`CLAUDE.md` 新增的 10000-19999 段既不对应 `bizerr` 登记表，也不对应
`utils.ErrorCode` 字符串码表，却写在「最高编码准则」里，于是照文档实现的第一条分支就是 panic。

同一主题另两份文档也互相冲突，属同族缺陷，需一并定性：

1. `ADR-009` 状态行称「已合并到 `docs/standards/MASTER_RULES.md` 第 4.3 节」，实测 `MASTER_RULES.md`
   只有 6 个一级节、§4 是「提交规范」、全文零命中「错误码/BizError」⇒ 悬空引用。
2. `ADR-009:25` 称 `BizError.Code` 是「5 位数字」，而 `codes.go` 实际全是 4 位；`ADR-009:66-70`
   自己的分段表又写成 1xxx-6xxx（4 位），同文档内自相矛盾。
3. `ADR-009:50-58` 示例调用 `response.Error(ctx, httpStatus, bizErr.Code, bizErr.Message)`（4 个实参），
   而真实签名是 `Error(c *gin.Context, code any, message string, data ...any)` ⇒ 照抄无法编译。

## Proposed Remediation

**Preferred**（三层各归其位，文档以代码为单一事实源）：

1. `CLAUDE.md` §错误码：删除 10000-19999 这一族虚构码值，改为写明两条通道的分工，并把码值事实源
   指向 `user-server/internal/domain/errors/codes.go`（**不在文档里重抄清单**，避免再次漂移——
   本轮缺陷正是"文档抄了一份不存在的清单"造成的）。
2. `response.Error`：在 `case int:` 上加 HTTP 状态码合法域校验（`100 <= code <= 999`），越界时
   **不进入 gin 的 panic 路径**，改为按 `utils.ErrorCodeUnknown` 回 500，并把越界事实写进日志。
   这把这一族缺陷从"照文档就崩"降级为"照文档会得到一条明确的 500"，且对既有正确调用零影响
   （合法 HTTP 码路径行为不变）。
3. `ADR-009`：改掉悬空的 §4.3 状态行、「5 位数字」表述，以及那处无法编译的 4 参示例，使其与
   `codes.go` + `ErrorWithBusinessCode` 一致。

**Alternatives**:
- 只改文档（1+3），不动 `response.Error`：改动面最小，但 `code any` 的语义陷阱仍在，下一个照
  文档写的人依旧会 panic；且 CLAUDE.md 规则2 要求「发现即修」，不留待办。
- 把 10001 等真实加进 `bizerr` 登记表：会新增一套与 `4000-6005` 并存的码域，前端分流要同时认两套，
  属扩大冲突面而非收敛，不推荐。

**Files likely to change**:
- `CLAUDE.md`（§API 规范 → 错误码）
- `user-server/internal/pkg/utils/response/response.go`（`Error` 的 `case int` 分支）
- `docs/architecture/adr/ADR-009-error-handling.md`（状态行、§1 注释、§3 示例）

**Tests to add or update**:
- 反向复现用例：`response.Error(c, 10001, "…")` 不 panic，得到 HTTP 500 + `code` 为
  `UNKNOWN_1000` 的响应体（修前该用例必红，即本轮已复现的 panic）。
- 极性对：`response.Error(c, 400/404/409, …)` 行为逐字不变（防止加校验时把正常通道一起改掉）。
- 文档一致性：`CLAUDE.md`/`ADR-009` 里出现的每个业务码字面量都能在 `internal/domain/errors` 找到定义
  （若实现成本高于收益，则退化为"文档只引用文件路径、不列码值"，从结构上消除这一类漂移）。

## Risks & Considerations

- **行为变更面**：`response.Error` 非测试调用点现数 **2053 处**（`internal/controller` 非测试文件 179 个）；
  越界 int 从 panic 变 500 是**放宽**，
  不会把现有正常响应改掉，但会让原本"崩在日志里"的请求变成"静默 500"——需同时出声（日志）以免更难查。
- **前端兼容**：`ErrorWithBusinessCode` 通道 HTTP 恒 200、语义在响应体 `code`，本轮不动它的行为。
- **规范文件带权威语义**：`CLAUDE.md` 是编码 Agent 的输入，改它等于改所有后续 Agent 的行为；
  必须保证改后与 `codes.go` 逐字同源，否则又是一次漂移。
- **并行泳道冲突面**：本轮改动三个文件在 `git status` 里均为 clean（不在并行 lane 的 161 个在途
  Go 文件名单内），落地前需再核一次。
- **不在本轮范围**：Service 层 151 处 `*gorm.DB` 的 repository 注入重构正由另一 lane 进行中
  （`internal/service/` 下 80+ 文件 M），本评估不触碰、不重复。

## Open Questions

- [NEEDS CLARIFICATION: `docs/standards/MASTER_RULES.md` 里 ADR-009 声称的 §4.3 应补进 MASTER_RULES，
  还是把 ADR-009 状态改成"独立生效、未合并"？本轮按后者处理（ADR 保留内容，只订正指向）。]
