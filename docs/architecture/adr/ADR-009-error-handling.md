# ADR-009: 错误处理规范

- **状态**：本 ADR **独立生效**（原文正文不改写，与现行代码的差异见文末「与现行代码的对账」）
- **范围**：所有 Go 后端服务
- **原始编号**：DOC-ERR-001

## 背景

项目早期错误处理五花八门：

- 业务错误用 `errors.New("xxx")`，调用方无法判断错误类型
- HTTP 状态码与错误码混用（400 / 401 / 403 / 500）
- 错误信息泄露内部细节（SQL、堆栈）
- 错误日志缺少 trace_id，无法串联链路

## 决策

**已合并到 `MASTER_RULES.md` §4.3**，核心规范：

### 1. 错误分类

```go
// 业务错误（可恢复）
type BizError struct {
    Code    int    // 业务错误码：5 位数字
    Message string // 用户可读
    Cause   error  // 原始错误（不暴露给前端）
}

// 系统错误（不可恢复）
type SysError struct {
    Code    int    // 系统错误码：6xxx
    Message string
    Cause   error
}
```

### 2. 错误码分段

| 段位 | 含义 | HTTP |
|------|------|------|
| 0 | 成功 | 200 |
| 1xxx | 通用客户端错误 | 400 |
| 2xxx | 鉴权错误 | 401 / 403 |
| 3xxx | 资源错误 | 404 / 409 |
| 4xxx | 第三方错误 | 502 / 503 |
| 5xxx | 业务规则错误 | 422 |
| 6xxx | 系统错误 | 500 |

### 3. Controller 层处理

```go
if err != nil {
    if bizErr, ok := err.(*BizError); ok {
        response.Error(ctx, httpStatus, bizErr.Code, bizErr.Message)
        return
    }
    // 未知错误
    response.Error(ctx, 500, 6000, "系统繁忙")
    logger.Error(ctx, "unhandled", "err", err)
    return
}
```

### 4. 错误响应格式

```json
{
  "code": 1001,
  "message": "参数错误：name 不能为空",
  "request_id": "abc-123-def"
}
```

### 5. 日志规范

- 所有错误必须记录到结构化日志（含 trace_id）
- DB 错误必须用 `response.ErrorFromDB(ctx, err, msg)`
- 不在响应中暴露内部错误对象

## 后果

### 正面

- 前端可通过 `code` 字段精确判断错误类型
- 错误码文档化（`docs/api/error-codes.md`）
- 全链路 trace_id 串联

### 负面

- 老代码重构工作量大
- 需要维护错误码字典

## 落地

- `internal/pkg/utils/response/error.go`
- `internal/middleware/error_handler.go`
- `controller/error_helper.go`

## 关联

- ADR-001：五层架构（错误处理分层）
- ADR-005：数据库设计（DB 错误码映射）
- ADR-008：触达限流（限流错误的特殊处理）

## 与现行代码的对账（2026-10-13）

上文「决策」正文按 ADR 惯例不改写。本节记录它与现行代码的差异，**冲突处以本节为准**。
证据均为本次实测（`user-server/` 下的路径省略该前缀）。

| # | ADR 原文说法 | 现行事实 | 证据 |
|---|--------------|----------|------|
| 1 | 「已合并到 `MASTER_RULES.md` §4.3」 | 合并未发生。该文件在 `docs/standards/MASTER_RULES.md`，共 6 节，§4 是「提交规范」；全文对「错误码」「BizError」「4.3」命中 0 处。本 ADR 即错误处理的治理入口。 | `grep -n` on that file → rc=1 |
| 2 | §1 业务错误码是「5 位数字」 | 现行两张码表都不是 5 位：数值表用 4 位（`CodeParamInvalid = 4000` … `CodeLoaderFallback = 6005`）加 `CodeSuccess = 0`；字符串表用 `INVALID_PARAM_1001` 这类「名称_数字」形态。 | `internal/domain/errors/codes.go`、`internal/pkg/utils/error_code.go:6` |
| 3 | §2 分段表：1xxx 客户端 / 4xxx 第三方 / 5xxx 业务规则 / 6xxx 系统 | 段位含义与代码不一致。数值表里 4xxx 是客户端参数错误、5xxx 是系统错误、6xxx 是资产与同步域；字符串表里 4xxx 是校验、5xxx 才是业务规则、6xxx 系统、7xxx 文件、8xxx 浏览器自动化与 LTC 运营开关。照表新增码会把码放错域。 | `codes.go`、`error_code.go:11-67` |
| 4 | §3 示例 `response.Error(ctx, httpStatus, bizErr.Code, bizErr.Message)` | 该 4 参调用无法编译：`func Error(c *gin.Context, code any, message string, data ...any)`。业务码不得作为第二个参数（见下方「本轮补上的约束」），应走 `ErrorWithBusinessCode`。 | `response.go:100`、`response.go:281` |
| 5 | §4 响应体 `{"code": 1001}` | `Response.Code` 是 `any`，三条出口类型不同且都是现行约定：`Success` 回 int `0`；`Error` 回字符串码（如 `INVALID_PARAM_1001`）；`ErrorWithBusinessCode` 回 int 业务码。前端判成功只能用 `code == 0`。 | `response.go:40`、`48`、`100`、`281` |
| 6 | §5 `logger.Error(ctx, "unhandled", "err", err)` | 该 kv 形式不存在：`func Error(err error, msg string)`。带 ctx 的写法是 `logger.Ctx(ctx).Error()`。`response.ErrorFromDB` 确实存在，签名为 `(c, err, fallbackMsg string, data ...any)`。 | `logger.go:263`、`223`；`response.go:150` |
| 7 | 落地：`response/error.go`、`internal/middleware/error_handler.go`、`controller/error_helper.go`、`docs/api/error-codes.md` | 前三处名称或位置有偏差、第四处不存在：`response/` 下是 `errors.go` + `response.go`；`internal/middleware/` 没有 error 中间件；helper 在 `internal/controller/error_helper.go`（`HandleDBError`/`HandleServiceError`），另有 `internal/aiagent/knowledge/controller/error_helper.go`；错误码文档就是本文件与 `CLAUDE.md` §错误码，没有独立的 `docs/api/error-codes.md`。 | `ls`/`find` 实测 |

### 本轮补上的约束

§2、§3 的段位混淆曾以另一种形式落地：`CLAUDE.md` §错误码抄过一份并不存在于任何码表的
`10000-19999` 段，按它写 `response.Error(c, 10001, msg)` 会打到 gin 的
`panic: invalid WriteHeader code 10001`（`httptest.ResponseRecorder` 同样 panic），
调用方拿到的是断连而不是响应体。

现行防线（三层，均已生效）：

1. `response.Error` 的 `case int:` 增加合法域校验，`100 <= v <= 999` 之外不进入 `c.JSON`，
   降级为 `http.StatusInternalServerError` + `utils.ErrorCodeUnknown`，并把越界事实写进日志
   （`response.go:112`）。合法 HTTP 码路径行为不变。
2. `CLAUDE.md` §错误码 改为「两张表 + 两条通道」的分工说明，不再重抄码值清单，码值事实源
   指向上述两个文件——缺陷正是「文档抄了一份不存在的清单」造成的。
3. 判据常驻在 `internal/pkg/utils/response/response_test.go`：越界码集合
   `{0,1,99,1000,5001,10001,19999,50301,-1}` 断言不 panic 且落 500，配对的正向用例断言
   `400/401/403/404/409/422/429/500/503/504/999` 原样透传。

新增业务码只在 `internal/domain/errors/codes.go` 登记；新增对外错误码只在
`internal/pkg/utils/error_code.go` 登记。文档不重抄清单。
