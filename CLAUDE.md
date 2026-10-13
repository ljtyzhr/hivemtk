# CLAUDE.md - 最高规则

## 🏆 规则0：Git 仓库与协作模式(最高指示,优先级最高)

- **只允许使用这两个仓库**,其他仓库一律不用(jungle-hero/hivemtkbot 等 forks 已弃用,勿推送):
  - `gitee.com/xhpmayun/hivemtk`(本地 remote 名 `gitee-upstream`)
  - `github.com/xiaofang142/hivemtk`(本地 remote 名 `upstream`)
- **禁止使用 PR 流程**(不发 PR、不提 PR、不走评审合并)。
- 协作模式为**成员开发直推**:每次开发完成后,将 `master` 直推到上述两个 remote。
- 推送前必须 `git fetch` 确认是 fast-forward;若分叉,先 merge 再推。
- 本规则与规则1冲突时以本规则为准。

## 🏆 规则0b：禁止使用子 Agent 并行（RPM 限制，最高优先级）
- 因 RPM（请求速率）限制，**禁止调用子 Agent（Task/搜索子代理等）并行执行任务**
- 所有任务由主 Agent 直接执行，串行推进，不依赖子 Agent 分担
- 需要探索/搜索/实现时，直接使用主 Agent 的工具（Read/Grep/Glob/LS/WebSearch 等）
- 本规则优先于其他一切规则，与其他规则冲突时以本规则为准

## 🏆 规则1：每次任务完成自动提交并推送
- 任务/轮次完成后，必须自动 `git add` 变更文件
- 自动 `git commit` 含清晰 message（轮次/功能/测试状态）
- 自动 `git push` 到 **gitee-upstream(xhpmayun) 和 upstream(xiaofang142)** 双远端
- 不等待确认，不询问，作为工作流固化步骤执行

---

## 🏆 规则2：发现问题必须立即修复（禁止记录待办）
- 任何检查/测试/审查/日志中发现的问题，**发现当轮立即修复**，闭环验证
- **禁止**"记录待办、下轮处理、专项重构轮"等任何形式的延迟——延迟=逃避
- 修复后必须回归验证（构建+测试+lint），确认归零后才允许提交推送
- 问题只允许两种状态：已修复 / 修复中。不存在"已记录"状态

---

## 🏆 Go 后端五层架构（本项目最高编码准则）

所有 Go 代码必须严格遵循五层架构：

```
Router → Controller → Service → Repository → Model
```

> 目录即层名：`internal/{router,controller,service,repository,model}`。**本仓库无 `handler` 包**：
> `internal/controller/` 下实测 157 个非测试文件、165 个控制器类型全为 `XxxController`（如 `CustomerController`），`XxxHandler` 零命中。
> 新增业务域一律按 `controller` 落地，不引入 `handler` 同义词（避免双命名）。

### 各层职责
| 层 | 职责 | 禁止 |
|----|------|------|
| Router | URL→Controller映射 | 写业务逻辑、内联闭包 |
| Controller | 参数绑定+调Service+返回响应 | 写SQL、业务判断 |
| Service | 业务逻辑 | 直接操作DB、直接持有 `*gorm.DB` |
| Repository | GORM数据访问 | 包含业务判断 |
| Model | 纯数据结构 | 外部依赖 |

### 辅助层职责
| 层 | 职责 | 禁止 |
|----|------|------|
| DTO | 数据传输对象 | 包含业务逻辑 |
| Middleware | 请求中间件 | 直接操作DB |
| Pkg | 公共工具包 | 依赖业务层 |

### 铁律
1. **禁止跨层调用** — Controller → Repository = 违规
2. **禁止内联闭包**（旧文档亦称"内联 handler"，指同一件事）— `func(c *gin.Context) { c.JSON(...) }` 不允许在 router.go 中出现
3. **每域四层齐全** — 一个业务域 Controller/Service/Repository/Model 四文件必须完整
4. **Router 只做映射** — DI 装配在 main.go 完成

---

## 🏆 API 规范

### 路径
```
/api/{domain}/{resource}           # 用户端
/api/manage/{resource}             # 管理端
```

### 响应格式
```json
{"code": 0, "data": {...}, "message": "ok"}           // 成功
{"code": 400, "message": "参数错误"}                   // 失败
{"code": 0, "data": {"list":[], "total":N}}           // 列表
```

### 特殊路由（不受 `/api/{domain}/{resource}` 约束）
```
/health,/healthz,/readyz    # 健康检查
/ws/*                        # WebSocket
/files/*                     # 文件服务
/__debug__/*                 # 调试路由（仅开发环境）
/swagger/*                  # API 文档
```

### 错误码（两条通道，不得混用）

**业务码不是 HTTP 状态码。** 把 `10001` 这类业务码当状态码传给 `response.Error`，gin 会
`panic: invalid WriteHeader code 10001`，该请求拿不到响应体。

| 通道 | 用法 | 码值事实源 |
|------|------|-----------|
| 传输语义（HTTP 状态码） | `response.Error(c, utils.ErrorCodeXxx, msg)`；或 `response.Error(c, http.StatusBadRequest, msg)`——int 形参只允许 100–999，越界一律降级为 500 + `UNKNOWN_1000` | `user-server/internal/pkg/utils/error_code.go`（字符串码，形如 `INVALID_PARAM_1001`，登记表内含 HTTP 映射与多语言文案） |
| 业务语义（码进响应体） | `response.ErrorWithBusinessCode(c, bizerr.CodeXxx, msg)`——HTTP 恒 200，真话在响应体 `code` | `user-server/internal/domain/errors/codes.go`（4 位数字段：4xxx 参数/鉴权、5xxx 内部与平台、6xxx 资产域） |

成功响应固定 `response.Success(c, data, msg)` ⇒ 响应体 `code: 0`（int 零，不是字符串 `"SUCCESS"`）。
`bizerr.CodeSuccess` 同为 `0`；`utils.ErrorCodeSuccess` 是字符串 `"SUCCESS"`，只出现在 ErrorCode 通道，不用于成功响应体。

- 新增业务码**只在** `internal/domain/errors/codes.go` 登记；本文件不重抄码值清单（此前这里抄过一份
  代码里并不存在的 `10000-19999` 段，照它写的第一条分支就 panic —— 见 `.specify/bugs/business-error-code-panic/`）。
- 未在 `errorCodeRegistry` 登记的 `utils.ErrorCode`：状态码兜底成 500、空文案兜底成「未知错误」，
  但响应体 `code` 仍回显那个没登记的字符串 ⇒ 加码必须同步登记，否则线上只是 500、不报错。
- 渠道官方错误码（如 `internal/service/channel_error.go` 里的 `100001/10004`）属**上游平台码表**，
  与上面两条通道无关，不得当作本服务的业务码复用。

---

## 🏆 前端规范

| 项目 | 框架 | API出口 | 状态管理 |
|------|------|---------|---------|
| `user-web/`（唯一业务 Web 工程） | Vue 3.5 + Element Plus 2.9 | `src/api/{module}.js`（实测 98 个模块文件，统一 `import { http } from '@/utils/request'`） | Pinia 3 |
| `embed-sdk/`（客服 Widget 嵌入 SDK，ADR-011） | 原生 JS + Vite，运行时零依赖 | 不直连业务 API，iframe 嵌 `user-web` 的 `/chat/embed/:channel_ref` | 无 |

---

## 🏆 API 测试验收标准（三端验证）

每次修改后必须运行 `scripts/api_verify_full.py`，每个端点验证：
```
【入参】前端发送字段
【返回】API实际响应
【预期】code=0 + 字段完整
【DB】  写操作后SQL确认
```

---

## 🏆 工程文档索引

- 工程开发文档总索引见 `docs/DEV_DOCS_INDEX.md`;架构决策记录见 `docs/architecture/adr/`
