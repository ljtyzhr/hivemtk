# 全面数据库驱动配置方案（CONFIG_DB_DRIVEN_PLAN）

> 配套清单：`docs/HARDCODE_INVENTORY.md` + `hardcode-sweep/INVENTORY.csv`
> 适用工程：`hivemtk/user-server`（Go）+ `hivemtk/user-web`（Vue 3）
> 架构约束：五层 `Router → Controller → Service → Repository → Model`，本方案不破坏该约束

---

## 1. 目标与非目标

### 目标

1. **所有"运维想改"的标量有单一事实源**：库里一改就生效，不改代码、不重启。
2. **登记即接线**：进了参数中心页面就必须在生产代码里有读取点，否则门禁 rc=1。
3. **默认值只有一个出处**：`DefaultParamDefs()` 是唯一定义源，Go 常量、`defaultConfig()`、
   前端 `DEFAULT_LIMIT` 全部退化为"兜底 fallback"，不再各写一份。
4. **审计可追**：每次改动有 actor / old / new / 时间（现成 `config_param_audit_logs`）。

### 非目标（明确不做，避免范围失控）

- ❌ 不把**错误码、协议版本、端口/DSN、向量维度**做成可改配置
  （理由见 `HARDCODE_INVENTORY.md` §5；它们是契约或部署期变量）。
- ❌ 不把**中文文案**塞进参数中心。文案是 i18n 词条，生命周期与数值阈值不同
  （一个要翻译、一个要单位与范围校验）。
- ❌ 不把**业务字典/状态机**塞进参数中心。它们需要有序、标签、多语言，走字典表。
- ❌ 不追求"零硬编码常量"。Go 里应保留**编译期兜底值**：DB 不可用时服务要能起来，
  兜底值与 `DefaultValue` 一致，由测试钉住。

---

## 2. 现状诊断：4 套机制并存，能力不对等

| 机制 | 表 | 结构 | 默认值元数据 | 范围校验 | 审计 | 统一注册表 | 前端可读 | 读点数 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| **`config_params`** | `config_params` | 标量 KV | ✅ | ✅（duration 分支有漏洞） | ✅ | ✅ `DefaultParamDefs()` | ✅（`/api/manage/config-params`） | **43 / 115** |
| `system_config` | `system_config` | **固定 16 列单行** | ❌ 散在 `defaultConfig()` | ❌ 手写 clamp | ❌ | ❌ | ✅ | — |
| `system_config_kv` | `system_config_kv` | JSON blob | ❌ 散在各 service 常量 | ❌ | ❌ | ❌ | ❌ | 20+ 处各塞一个 blob |
| `feature_flags` | `feature_flags` | 结构化 + 灰度 | ✅ | — | ✅ | ✅ `RegisterCodeReference` | ❌ | — |

**四个致命问题**：

1. **登记 ≠ 接线**：115 条登记只有 43 条有人读，72 条僵尸（63%）。
   参数中心页面把 72 条渲染成可编辑控件，运维改完看到"保存成功"，但生产代码不读。
   这比对运维说"这个功能还没做"更糟。
2. **已暴露但没人读**：`system_config.MaxUploadSizeMB`（`gorm:"default:50"`，运维可改）
   与 `controller/upload.go:22 MaxUploadSize = 10MB` 并存，
   **上传链路一处都不读前者**。改配置页面显示成功、实际永远 10MB 拦截。
3. **key 全局唯一而 schema 说是复合**：gorm tag `uniqueIndex:idx_group_key`，
   实测库是 `UNIQUE(key)` 单列。同名跨 group 的第二条会被 `logger.Warnf` 静默丢弃。
4. **无默认值事实源**：同一个概念在 Go 常量、`defaultConfig()`、前端常量、
   甚至 3 个不同调用点各有一份默认值（`max_tokens` = 500/800/2048）。

---

## 3. 目标架构

```
                        ┌─────────────────────────────────────────┐
   运维改配置 ─────────► │  /api/manage/config-params （唯一写入口） │
                        └──────────────┬──────────────────────────┘
                                       │ UpdateValue（写库 + 审计 + 失效缓存）
                                       ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  ParamRegistry（唯一定义源，代码内）                              │
   │   DefaultParamDefs()  —— 现有 config_param_seeds.go 扩成全量      │
   │   + ReadPoints（编译期声明：每个 def 对应哪个读取函数）          │
   └──────────────┬────────────────────────────────────────────────────┘
                  │ SeedConfigParams（启动期 upsert，只刷定义列不碰 value）
                  ▼
   ┌────────────────────────────┐   ┌─────────────────────────────────┐
   │ config_params（标量 KV）    │   │ dictionaries（字典/状态机）     │ 新增
   │  阈值/上限/超时/TTL/开关    │   │  有序项 + 多语标签 + 状态迁移    │
   └────────────┬───────────────┘   └─────────────────────────────────┘
                │ ConfigParamService（60s TTL 缓存，按 group.key）
      ┌─────────┼──────────┬──────────────┐
      ▼         ▼          ▼              ▼
   Go Service  中间件      前端 /api/…    门禁脚本
```

### 3.1 `config_params` 只做"标量 KV"，加 3 列不加结构

新增列（**不改已有列语义，不破坏 AutoMigrate**）：

| 列 | 类型 | 用途 |
| --- | --- | --- |
| `owner` | `varchar(64)` | 归属服务/模块，如 `inbox_ingress`。用于"谁该来接线"的责任归属 |
| `source_ref` | `varchar(255)` | 硬编码原位置，如 `internal/service/inbox_ingress.go:30`。接完线后可一键自查 |
| `wired_at` | `timestamptz` | 首次接线的提交号/时间。用于区分"僵尸"和"已接线" |

> 不引入 `json` 列存结构化配置 —— `system_config_kv` 已经是这条路，
> 20+ 个 JSON blob 已经证明它必然退化成没人看得懂的泥潭。

### 3.2 `ParamDef` 扩展（在 `config_param_seeds.go` 内，不新建包）

```go
type ParamDef struct {
    Group, Key, Name, Description string
    ValueType                     string // int|float|bool|duration|string
    DefaultValue                  string
    Min, Max, Step                *string
    ReadOnly, Restart             bool
    Category                      string

    // 新增
    Owner      string // 归属模块，对应新增列 owner
    SourceRef  string // 硬编码原位置 file:line，对应新增列 source_ref
    Wiring     string // 读取点函数名，如 "inbox.Ingress.lockTTL()"；空 = 未接线
}
```

`Wiring` 非空即"声明已接线"，`check-config-param-readpoints.py` 的判据从
**「有 `Get*` 调用 或 Name 含『未接线』」** 升级为
**「`Wiring` 非空 且 该函数确实存在 且 已撤掉『未接线』标注」**——
从"扫字符串形状"升级为"声明与实现双向核对"。

### 3.3 收敛边界（哪些东西搬到 `config_params`）

| 现状 | 去向 | 判据 |
| --- | --- | --- |
| `system_config` 16 列 | **保留**（站点身份/品牌/开关，是单行实体不是 KV） | 扩展要改 model+migration 的，就不适合 KV 化 |
| `system_config_kv` 的 `password_policy` / `security.rules` / `handoff_rules` 等 **结构化规则** | **保留 KV**（它们是文档，不是数值） | 规则集合不是标量 |
| `system_config_kv` 里的**散落默认值** | 提到 `DefaultParamDefs()`，KV 只存覆盖值 | 默认值必须在定义源里有一份 |
| `feature_flags` | **保留独立表**（灰度百分比 + 分桶，语义与 bool 不同） | 不要用 bool 参数模拟灰度 |
| 硬编码阈值（346 个点位） | **进 `config_params`** | 见清单 §2 |
| 业务字典（44 项） | **新建 `dictionaries` 表** | 需要有序 + 标签 + 多语言 |
| 中文文案 | **i18n 词条**，不进配置 | 见清单 §4 |

### 3.4 新增表：`dictionaries`（业务字典，阶段四才建）

```go
type Dictionary struct {           // 表 dictionaries
    ID        uint
    Domain    string  // human_task / opportunity / payment ...
    Code      string  // 字典项代码，全局唯一
    LabelZh   string
    LabelEn   string
    SortOrder int     // 有序阶段用
    Group     string  // 次级分组（如 fix_layer）
    Meta      string  // JSON：颜色/图标/终态标记
    Enabled   bool
}
type DictionaryTransition struct { // 表 dictionary_transitions：状态机
    ID         uint
    Domain     string
    FromCode   string
    ToCode     string
    RequiredRole string
}
```

**前置要求**：字典可配 = 状态可被运维改 = 任何"当前状态是什么"的判断都要走查表。
改造前必须有一份"状态被写死在 if 里的位置"的全量清单，否则改一个状态会静默改变历史数据的语义。
**这就是为什么字典表排在阈值接线之后**——阈值错了能立刻看出来，字典错了不能。

---

## 4. 阶段计划

### 阶段 0：前置修复（阻塞项，必须先做）

> **进度（2026-10-10）：0.1 ~ 0.5 全部完成并已提交。**

| # | 动作 | 验收 | 状态 |
| --- | --- | --- | --- |
| 0.1 | 对齐 `config_param_test.go` 的 `defaultParamDefsWant = 114` 与脚本实测 115 | `go test ./internal/service/ -run TestSeedConfigParams` 绿 | ✅ |
| 0.2 | 修 `validateValue` 的 `duration` 分支：`ParseDuration` 失败不应 `return nil`，应按"秒数 float"解析并继续校验（与 `GetDuration` 的读取口径对齐） | 新增用例：非法 duration + 设了 min/max → 拒绝写入 | ✅ 另抽出 `parseDurationSeconds` 供读写两侧共用；min/max 自身解析失败不再吞错；删掉「无 min/max 就早退」导致类型校验被跳过的口子 |
| 0.3 | 修索引漂移：要么把 gorm tag 改成单列 `uniqueIndex` 与实际一致，要么写 migration 把它改成 `(param_group,key)` 复合。**倾向后者**（复合索引更符合设计意图，且 key 命名不用再带 group 前缀） | migration 幂等可重跑；`TestDefaultParamDefsKeysGloballyUnique` 按新口径改写 | ✅ 核实发现 migration v3.47.0 早已注册（`initial_schema.go:207`），漂移只在存量库；真正没闭环的是审计行只记 key（已加 `param_group` 列）、唯一性用例盯单列比库更严（已按 `(group,key)` 改写）、seed 失败只 `Warnf`（已改为逐条 Error + 汇总返回 error） |
| 0.4 | 修 `NewSystemConfigKVRepository()` nil DB panic → 改 nil-safe 或全量改用 `WithDB` 装配 | `TestFallbackNilDB` 覆盖 | ✅ 三方法统一返回哨兵错 `ErrKVStoreUnavailable`；`browser_automation_routes.go:38` 改走 `WithDB` 装配 |
| 0.5 | 修 `MaxUploadSizeMB` 无人读的实证 bug：`upload.go` 改读 `system_config.MaxUploadSizeMB`（保留 `MaxUploadSize` 常量为兜底） | 上传 11MB 文件在配置调大后成功；API 验证通过 | ✅ 优先级链 env > 库 > 10MB 兜底，逐格钉死在 `upload_maxsize_test.go` |

> 0.5 单独列，因为它是**已暴露给运维却完全不生效**的配置，比 72 条僵尸更值得优先修。
> 0.5 落地时确认的一处取舍：`ResolveUploadMaxBytes` 刻意**不**走 `GetConfig`——那条路读不到行时
> 会拿 `defaultConfig()` 的 50MB 顶上，而上传链路真正生效的兜底是 10MB；跟着走等于把存量站点
> 的上限悄悄放宽 5 倍。50MB 与 `middleware/body_limit.go:36` 注释声明的「本仓最大合法单文件 50MB」一致，
> 说明 10MB 才是那个偏离产品意图的硬编码。

### 阶段一：72 条僵尸参数接线（投入产出比最高）

> **进度（2026-10-10）：阶段一全部 8 批已入库（8/8）。**
> misc(16) / confidence(5/7) / agent_llm(7) / cache(5) / agent_tool(4/5) / telemetry(4)+workflow(4) /
> inbox_sales(4)+session(3)+pagination(1) / sales(1)+middleware(2)+knowledge(2)+channelgw(1)。
> 门禁读数从 `wired=40 / UNDECLARED=72` 变为 `wired=102 / 已声明未接线=13 / UNDECLARED=0 / 声明过期=0`。
>
> **入库位置（如实记录，勿按 commit message 找）**：
> - 第 1 批 misc(16) → `c75c78b6 feat(config-params): 阶段一第1批 —— misc 组 16 条僵尸参数接线`
> - 第 2 批 confidence(5/7) → **`86b92349`**。这批的 17 个文件当时已在暂存区等待提交，
>   被一次并行的他人提交（`chore(scripts): 删孤儿重复门 check-enum-consistency.py`）一并带走，
>   因此 commit message 与内容不符。内容已逐文件核对完整（17/17），且在干净检出上全绿，
>   故不改写已推送的历史，只在此登记归属。
> - 第 3 批 agent_llm(7) → `97ebaefb feat(config-params): 阶段一第3批 —— agent_llm 组 7 条僵尸参数接线`
> - 第 4 批 cache(5) → `8bcc0443 feat(config-params): 阶段一第4批 —— cache 组 5 条僵尸参数接线`
> - 第 5 批 agent_tool(4/5) → `5fb6f400 feat(config-params): 阶段一第5批 —— agent_tool 组 4 条僵尸参数接线`
> - 第 6 批 telemetry(4)+workflow(4) → `21dff6d0 feat(config-params): 阶段一第6批 —— telemetry(4)+workflow(4) 组 8 条僵尸参数接线`
> - 第 7 批 inbox_sales(4)+session(3)+pagination(1) → `7a0ae7b0 feat(config-params): 阶段一第7批 —— inbox_sales(4)+session(3)+pagination(1) 组接线`
> - 第 8 批 sales(1)+middleware(2)+knowledge(2)+channelgw(1) → 本提交。
> - 台账回填另计：`2f2ba9cb`（第 3 批）、以及各批随附的 docs 提交。
>
> **第 8 批查出一个门禁管不到的 bug（比 72 条僵尸更隐蔽，值得单独记）**
>
> `channelgw.ws_push_interval` 之前被种子里标着「未接线」，本批去接线时发现它其实**有读取点**
> —— `internal/channelgw/ws.go` 的 `runtimeWSPushIntervalDefault` 真真切切在调
> `GetDuration(ctx, "channelgw", <key>, …)`。但那个 `<key>` 是 `"ws_push_interval_default"`，
> 而种子里登记的是 `"ws_push_interval"`。多一个 `_default` 后缀，于是
> **每次读都 miss、每次都回落到兜底值 2s**：参数在管理台改得动、页面显示保存成功、
> 代码就是读不到。同组另外 4 条 key（`ws_register_timeout` / `ws_read_idle_timeout` /
> `ws_write_timeout` / `ws_pipeline_timeout`）都是对的，所以这是一处单点笔误。
>
> **这类故障 readpoints 门结构上抓不到**：它检查的是「种子 key 有没有读取点」，
> 而这里读取点有，只是用的不是那个 key。反向的「登记了没人读」它能抓（判 UNDECLARED），
> 正向的「有人读但读的是别的 key」它一个字都看不出来。本批已改正 key，
> 并补 `TestRuntimeWSPushIntervalReadsSeededKey`（真库改值 → 断言读取函数读到改后的值，
> key 再写错就会红）把这类回归钉住。
>
> **第 8 批：8 条目标里接了 6 条，另 2 条经核实确实没有生产消费点**
> （同第 2/5/7 批口径：如实保留「未接线」标注并把 Description 改写为写明真实原因）：
> - `wecom.error_rate_degrade`：`WeComErrorRateDegradeThreshold = 0.3` 只出现在常量声明和
>   一句**撒谎的注释**里（「运行时通过 GlobalConfigParam() 读取」）—— 根本没有对应的读取函数。
>   现存实现 `computeHealthScore` 的成功率判定是写死的三档（`<50` 扣 30 / `<80` 扣 15 /
>   `<95` 扣 5），单一「错误率阈值」表达不了这种递减分档；硬套进来只能新造第四档或改掉现有
>   扣分行为，所以没接。**顺带修掉那句撒谎的注释**（规则2：不留"已发现但不改"的东西）。
> - `sales.insight_limit`：`trace_learning.TopInsights` 的 `limit` 是**入参不是默认值**，
>   全仓唯一调用方是 `insights_test.go`（且三处都硬传 3），生产链路没有任何地方取这个数量。
>
> **第 8 批顺带确认的一处「配额降级半接线」**：`wecom.quota_degrade` 是真接线
> （`runtimeWeComQuotaDegrade` 有真实 `GetFloat`），但 `computeHealthScore` 的配额三档
> （`>0.95` 扣 25 / `>quotaDegrade` 扣 15 / `>0.7` 扣 5）里只有中间一档读了参数，
> **上下两档 0.95 / 0.7 仍是写死的**。所以把 `quota_degrade` 调到 0.95 以上时，
> 上面那档会先命中、参数等于失效。这属于「新增点位」（阶段二/三范围），本批只如实记录不扩范围。
>
> **第 7 批：16 条目标里只接了 8 条，另 8 条经核实确实没有生产消费点**
> （同第 2/5 批口径：如实保留「未接线」标注并把 Description 改写为写明真实原因，
> 不为让门变绿而凭空造读取点）：
> - `pagination.page_max_size` / `page_default_size`：对应的
>   `internal/pkg/utils/pagination.go` `ParsePagination` / `ParsePaginationOffset`
>   **全树只有 `pagination_test.go` 调用**，零生产调用方；`page_default_size` 在
>   `ParseCursorParams` 里那条兜底分支又被 operation_log / security_audit / customer
>   三个调用方传的正数 `defaultLimit` 遮蔽。顺带发现种子写 100、代码写 200 本就不一致。
> - `wechat.chat_ws_ping_period` / `chat_ws_pong_wait` / `chat_ws_write_wait`：常量在
>   `internal/controller/chat_ws.go` 有读取点，但承载它的
>   `internal/router/ws.go` `RegisterWSRoutes` **全树零调用方**（`GET /ws/chat`
>   从未注册，实测落到 SPA 的 NoRoute 兜底）。另：后端根本没有微信渠道 WS 客户端，
>   `internal/channelbot` 下只有 core/qq/telegram/whatsapp，种子「微信渠道 WS」本身名不副实。
> - `bridge.polling_max_timeout` / `polling_default_timeout` /
>   `bridge.max_reply_content_bytes`：承第 6 批已核实，长轮询未实现，
>   `maxReplyContentBytes` 自带 `//nolint:unused`。
>
> **第 7 批的又一处「种子与代码不一致」**：`session.max_delay_seconds` 的种子写
> 「触达流水线排程允许的最大延迟，超过视为放弃」，实现
> `internal/content/service/marketing_flow.go handleDelay` 其实是「营销流程 delay 节点
> 单次等待时长的上限，超过则截断」。代码里没有「超期放弃」这个概念，已连接线带改写一并对齐。
>
> **第 7 批踩到的一个坑（值得写下来）**：seam setter 的 `nil` 语义是「**不注入**」
> （`if fn != nil` 守卫，防装配顺序错时把兜底值顶掉），不是「复位」。按直觉写
> 「传 nil 后应该回到兜底」的用例会红，而且红得有理有据。本批复用了前六批的同一范式，
> 于是测试里统一改成两件事：① 把「传 nil 等于撤销注入」改写成「传 nil 是空操作」并
> 先重新注入一个已知的非兜底值再断言；② `t.Cleanup` 里同包直接写包内 `provider` 变量复位
> （生产 setter 复位不了，测试能）。**这个坑不是本批独有的，前六批的 seam 测试也写了
> 「传 nil 等于撤销注入」，只是它们的断言恰好在 provider 仍是被注入值的位置才没红。**
>
> **第 6 批的两条「种子与代码不一致」处置**（这是接线时才发现的、写死没人看得出来的漂移）：
> 1. `telemetry.trace_sink_buffer` 种子写 8192，代码兜底是 `aiagent/llm/trace_sink.go` 的 2048。
>    取**代码的 2048** 回填种子，理由：参数中心的默认值语义是「没人配置时系统现在的行为」，
>    不是「更合理的目标值」。照抄 8192 等于升级即把每个 trace sink 的缓冲内存翻 4 倍，
>    而丢弃事件只在缓冲区满时暴露 —— 越晚暴露、越难定位。
> 2. `telemetry.geo_position_window` 的 Name/Description 与实现完全不是一回事：种子写
>    「Geo 地理位置统计的滑动窗口大小」，实现 `geo/service/metrics.go dedupMatches` 里
>    比较的是**字符偏移**（`m.start - prevEnd > window`），语义是「相邻两处品牌词提及
>    间隔小于该字符数即算同一处」。已一并改写，否则配的人会按"条数"去理解一个字符参数。
>
>
> **第 4 批踩到的三个坑（都写下来）**：
> 1. `internal/controller/platform.go` 的 `platformCacheTTL` 原本是**函数内的局部 const**，
>    只改名不提到包级，新加的包级 `PlatformCacheTTL()` 读不到它 —— 只跑
>    `go build ./internal/controller/` 才暴露，只 `go vet` 挑过的子包漏掉了这个包。
>    **凡是加 seam 的包，都要单独 `go build` 一次，不能只 vet 自己"以为相关"的那几个。**
> 2. `internal/aiagent/rag/retrieval` 的**包名是 `ragretrieval`**，导入必须写别名；
>    同目录的 `NewTranslationCache(client, ttl, prefix)` 参数顺序也容易记反。
> 3. `git worktree add` 到 `/tmp/xxx` 时，若该路径被更早的会话留成过 worktree，
>    git 会报「路径已存在」而**不切换 base**，导致把文件倒进了一个旧 commit 的检出里，
>    验证结论完全无效。**每次用新目录名；add 之后必须 `git worktree list` 确认 HEAD 对得上。**
>
> **教训（写下来防止再犯）**：暂存区是**共享**的，不属于某一条泳道。跨多个工作阶段把文件
>   长期留在暂存区，等于把「我下一条 commit 会带上它们」这件事暴露给任何并行提交的人。
>   正确姿势是**验完立刻提交**，把暂存区停留时间压到最短；`git commit` 前先
>   `git diff --cached --name-only` 核一遍自己到底要带谁走。
>
> **工作区随时可能是半成品状态**：本轮多次撞见他人正在编辑、留下未使用 import 导致
>   `internal/service` 编译不过。碰到这种共享工作区的破损，正确做法是**把自己的文件倒进
>   干净 worktree 验证**（`git worktree add --detach /tmp/xxx HEAD` + `git show ":$f" >`），
>   既能证明「我的改动是绿的」，又不去动别人正在写的东西。
> confidence 组 7 条里只接了 5 条：`persona_default_threshold` 与 `persona_max_retry` 的实现
> `internal/service/persona_evaluator.go` 已随 `9a5f716c` 整体删除（749 行实现 + 958 行测试），
> 代码里不存在等价能力，**不为了凑数凭空造一个读取口**，两条如实保留「未接线」标注，
> 并在 Description 里写明「实现已随 9a5f716c 删除」以及「若要暴露重生成上限请另立
> `humanize_max_retry` 而不是改这条」——免得后来人以为只是漏接线。
> 余下 2 条 UNDECLARED 是 `cache.faq_ttl` 与 `agent_llm.default_semantic_threshold`，
> 它们的唯一读取点在 `app/faq_cache_wiring.go`——一个尚未入库的在途文件，属于别人正在做的
> FAQ 语义缓存特性（该特性还要改 `rag/cache/service.go` 的构造签名并连带两个测试文件）。
> **不为了让门变绿而把别人的半成品特性带进本提交**；等对方入库后这两条自然转绿。
>
> **这一句已经兑现（2026-10-13 现测，写在原话下面而不是改原话）**：
> `user-server/internal/app/faq_cache_wiring.go` 现在**在 HEAD 里**
> （`git ls-files` 命中、`git cat-file -e HEAD:…` 通过、`git status --porcelain` 对它空＝工作树与库内一致）；
> 两条键都已进种子（`config_param_seeds.go:192` 的 `cache.faq_ttl`、`:429` 的
> `agent_llm.default_semantic_threshold`），读取点常量在同一文件 `:20-21`；
> G1 现测 **UNDECLARED 0 / ORPHAN 0**（影子 B 面 `15ccabb7`，08:36，`fp_shadow4/g1.log`）。
> 也就是说"孤儿 key"这一类判据真的把这条链闭过：它当时红的是**事实**，不是门坏了。

- **动作**：清单 §7 对照表右列的常量，逐个换成 `config.GetInt/GetDuration/GetBool/GetFloat/GetString`，
  保留原常量名做 fallback（避免大面积改调用点）。
- **接线范式**（以 `InboxLockTTL` 为例，符合五层架构：常量在 Model 层、读取在 Repository/Service 层）：

```go
// internal/service/inbox_ingress.go
func InboxLockTTL(ctx context.Context) time.Duration {
    d, _ := GlobalConfigParam().GetDuration(ctx, InboxIngressGroup, "inbox_lock_ttl", InboxLockTTL)
    return d
}
```

- **同步**：种子条目 `Wiring` 字段填上函数名，**撤掉 Name 里的「（未接线）」与
  Description 里的「【当前不生效，改了也没人读】」**——门禁第 2 条会检查标注是否过期。
- **验收**：
  - `check-config-param-readpoints.py` 输出 `未接线且已声明=0`
  - 每个域一个回归：改库 → 60s 内行为变化（`configParamTTL = 60s`）→ 改回

- **第 1 批（misc 组 16 条）实际落地形态与原计划的出入**：

| 原计划 | 实际落地 | 为什么改 |
| --- | --- | --- |
| 每条常量旁写一个 `GetXxx(ctx)` | 统一 `const xxxDefault` + `var xxxProvider` + `SetXxxProvider(fn)` + 包级 `xxx()` | 点位横跨 `model` / `repository` / `middleware` / `pkg` / `ops`，这些包**不能反向依赖 `service`**（Model 层禁止 import 上层），就地接线只能各造函数变量。装配层 `internal/app/misc_params_wiring.go` 是唯一能同时看见全部 16 条的地方 |
| 逐个替换调用点 | 只改**取值那一行**，调用点结构不动 | 16 条里有几条的常量被多处引用（`text_truncate_max_bytes` 在 `pkg/tracing` 等），逐点替换会把 diff 撑大到无法 review |
| — | provider 传 nil 视为不注入；`xxx()` 拿到非正数一律回落兜底 | 装配顺序错时不能把兜底值顶掉；`make(chan, 0)` 会死锁、上限取到 0 会把内容全截没 |
| — | `backup_page_size` 按「单页上限 clamp」接入，保留默认分页 10 | 种子默认 1000 只在「上限」语义下说得通。若当「默认分页大小」接入，升级后备份列表默认每页会从 10 行静默变成 1000 行 |
| — | `sop_scheduler_interval` 不与 `utils.LongTimeout` 共用常量 | 一个是外部调用超时、一个是调度周期，共用会让「改超时」意外改掉调度频率 |

### 阶段二：P0 新增 58 个点位入库

按 §2.1 四类分四个 PR，每 PR 一个类：

| PR | 内容 | 关键点 |
| --- | --- | --- |
| 2a | 上传/body 上限（`upload_max_size_mb`、`max_json_body_mb`） | **合并三份副本**为 1 键；`system_config.MaxUploadSizeMB` 保留为兼容列，读新键优先 |
| 2b | Webhook 验签降级开关（**不新增参数键，改为收敛护栏强度**） | 方案原文「单键覆盖 14 个硬编码点、默认改 `false`」经实测**前提全部不成立**，实际处置见下方「2b 已落地」；正确地讲这是「全面数据库驱动」的一个受控例外 |
| 2c | 锁与幂等 TTL（收件箱/消息中台/人工接管/TG 轮询锁，13 个点位） | **已落地**：13 个点位逐条核实后只接得动 6 条，新建 `lock` 组 7 条（含 1 条刻意不接）；`message_hub_idem_ttl` 单独标 `Restart=true`；`TG 轮询锁`那条早在 1.1 已接、不重复；`human_escalation` 三条是死代码不建种子。实际处置见下方「2c 已落地」 |
| 2d | 渠道与配额上限（QQ/TG 长度、SSE 每 IP 连接、配额降级阈值，15 个点位） | **已落地**：15 个候选逐条核实后接 10 条，新建 `channelbot`（5）+ `sse`（3）+ `bridge`（2）三组种子。`internal/service` 反向 import `channelbot/{qq,telegram,core}`，这三个子包走 provider seam + 装配层注入；`service/sse_hub.go` 与 `bridge/sse.go` 本就能直接调 `GlobalConfigParam()`，走直连不造 seam。实际处置见下方「2d 已落地」 |

- 每条种子的 `Min/Max` 必填（`config_params` 已支持），避免运维填出 `max_tokens=999999`。
- `ValueType` 用 `duration` 的，`Step` 填合理步长（如 `30s`）。
- **验收**：`scripts/api_verify_full.py` 跑通 + 每个 PR 附"改值→行为变化→改回"三步证据。

#### 2a 已落地（2026-10-10）

上传/body 上限两条入库，`wired 102→104`、种子条目 115→117、锚点同步到 117。

**上传链最终优先级**（`internal/controller/upload.go` 的 `resolveUploadMaxSize`）：

```
env UPLOAD_MAX_SIZE > misc.upload_max_size_mb > system_config.max_upload_size_mb（兼容列）> 10MB 代码兜底
```

- `misc.upload_max_size_mb` **默认 0**，语义是「不覆盖，沿用下游」，`Min: "0"`。这个默认值不是偷懒，是刻意取舍的两个反面各错一遍：填 `10` 会把运维已在「系统配置」页调好的 200MB 又压回 10MB（把阶段 0.5 刚修好的 bug 原样退回）；填 `50` 则让新装站点从 10MB 静默放宽 5 倍。填 0 时存量与新装站点行为与上线前**逐字节一致**。
- 兼容列保留在第二顺位：存量站点已经在那儿调过值，新键一上线就把它顶掉会造成「配置莫名失效」。
- 该键是**每请求读**，改完即时生效（上传入口不停服）。
- 配套 `mbToBytes(mb)` 辅助：非正数原样返回 0，即「0 = 未配置」是本链上下游共用的哨兵值，混进换算会把 0 变成"0 字节"从而打死所有上传。

**body 链最终优先级**（`internal/middleware/body_limit.go` 的 `BodyLimitFromEnv`）：

```
env MAX_JSON_BODY_MB > middleware.max_json_body_mb > DefaultMaxJSONBodyMB(8)
```

- 默认 8 必须保持：`TestGlobalDefaultDoesNotTightenExistingCaps` 钉死全局默认值 ≥ webhook 可调顶格 4MB，调小会把运维已放行的高段请求截断。
- `Min: "1"` **有意不给 0**：env 侧 `<=0` 的「不限制」是部署层排障应急开关，参数中心是运维日常调的面，再开一个关掉全局 body 封顶的口子风险大于收益。Description 里写明「临时放开请用 env 设 0」。
- `Restart: true` —— 这是 **`ParamDef.Restart` 字段首次被真正用上**（此前 115 条种子一条都没填）：`router.Setup` 里 `r.Use(middleware.BodyLimit(...))` 是**装配期读一次**，标成 false 会让运维改完看不到任何生效提示。
- `maxMultipartMemoryMB` 保持独立不联动：它是把 gin 默认 32MB 主动调小的内存优化，跟随 body 上限会顺带把每个并发上传的内存放大。

**接线范式**：`middleware.max_json_body_mb` 走 provider 注入（middleware 不能反向依赖 service）；`misc.upload_max_size_mb` 走读取函数直连参数中心（读取点在 controller，本来就能依赖 service），两者的正确性分别由 `internal/app/upload_body_limit_params_wiring_test.go` 与 `internal/controller/upload_param_limit_test.go` 的真库行为测试盯住。

**未纳入本批**（如实记录，避免后人误以为已覆盖）：前端第三份副本 `user-web/src/views/system/MaterialLibrary.vue:323` 的 `maxSize = 10` 属阶段 3.3；`knowledge_base_import.go:35 MaxUploadFileSize = 50<<20` 已在阶段 1.8 作为 `knowledge.max_upload_file_size` 单独入库，与本条不是同一个上限。

#### 2b 已落地（2026-10-10）——「不 DB 化」是一个受控例外

**方案原文的三处前提经实测全部不成立，如实记录以免后人按错的方向再走一遍**：

| 原文说法 | 实测 |
| --- | --- |
| 14 个硬编码点 | **只有 3 个真实读取点**（`internal/service/webhook.go:509`、`internal/channelbot/telegram/telegram.go:463`、`internal/content/service/marketing_flow_action.go:703`）；其余 11 处是错误文案/日志里提到变量名 |
| 默认 `true`（硬编码开着） | 三处**本来就 fail-closed**（只认字面量 `"true"`，且都要求「该账号压根没配密钥」时才可能豁免），不存在要改的默认值 |
| 单键可覆盖 | 三把开关护栏强度分三档（见下），语义各不相同，**一个键覆盖不了** |

三把开关的真实护栏强度（这是本批真正要解决的问题）：

| 变量 | 原护栏 | 风险 |
| --- | --- | --- |
| `ALLOW_INSECURE_WEBHOOK` | 启动护栏：`APP_ENV` 非开发值 ⇒ `log.Fatalf` 拒绝启动 | 挡得住 |
| `ALLOW_INSECURE_TELEGRAM_WEBHOOK` | **完全没有**，仅 `logger.Warnf` | **生产误设 ⇒ 验签被静默跳过，启动不报错**（本批修的真实缺陷） |
| `MARKETING_WEBHOOK_ALLOW_INSECURE` | 运行时判 `config.IsDevelopmentEnv()` | 挡得住，但只挡运行时，启动不报 |

**为什么不做成 DB 参数**（这是本节的核心结论，比"多接一条线"重要得多）：

把验签绕过做成数据库可改，等于让 admin API 的写操作能在**运行时翻转签名校验** —— 相比现在的"env + 启动 `log.Fatalf` 护栏"是**安全回退**：DB 改完即时生效、不需要重启、没有启动拦截、且改完那一刻没有任何日志。env 的代价（要重发一次部署）恰恰是它作为安全闸的屏障。

另一个更朴素的理由：**DB 镜像 env 状态会造假值**。若为了让控制台"看得见"而建一条种子行，它的 `Value` 要么是启动时写入的（只反映最后一次重启时的 env，与当前进程实际读到的 env 可能不同），要么永远静态 false（控制台显示"未开启"但 env 可能正开着）。前者需要 ReadOnly 的键在启动时被写入（`ReadOnly` 与「服务端自写」自相矛盾），后者是纯误导。因此**不给这三处加 `config_params` 种子行**，`wired` 计数不变。

**实际落地的四件事**：

1. **修真缺陷**：`telegram.go` 旁路从裸 `os.Getenv(...)=="true"` 改为 `insecureTelegramWebhookAllowed()`（开关 **且** `config.IsDevelopmentEnv()`），与 marketing 那处同口径。
2. **补第二处缺陷**：`internal/channelbot/whatsapp/whatsapp.go` 的 `VerifyWebhook` 在空 secret 时从 **fail-open 改为 fail-closed**（旧注释写"与项目既有行为一致"，实际是本仓唯一一处无开关、无护栏、无审计痕迹的验签绕过）。改后打 `logger.Warnf`。
3. **扩启动护栏**：`insecureWebhookStartupError` 加第 5 个参数 `allowInsecureTelegram`，`guardInsecureWebhookAtStartup` 同时读两个变量 —— 生产误设任一都拒绝启动。
4. **钉住语义**：`"TRUE"`/`"1"`/`"yes"`/`"true "`（带尾空格）**不算开启**，测试逐格覆盖，避免将来有人图省事改成 `strconv.ParseBool` 放宽口径。

**这是「全面数据库驱动」的一个受控例外，不是漏做**：判定标准是——**能不能让运行时的一个写操作翻掉安全闸**。阈值、TTL、上限都是"调坏了我看得见、改回来就行"；验签旁路不是，翻错的代价是外部伪造回调直接进业务。凡属此类的一律留在 env + 启动护栏，本批次起在两份文档中逐条登记。

#### 2c 已落地（2026-10-10）

锁与幂等 TTL 入库，新建 `lock` 组 7 条（117→124），`wired 104→110`、锚点同步到 124。

**13 个点位逐条核实后，只有 6 条接得动**：

| 种子 key | 默认（秒） | Restart | 代码点位 |
| --- | --- | --- | --- |
| `lock.inbox_human_ttl` | 86400 | false | `internal/service/inbox_ingress.go:28` `InboxLockTTL`；读取点 `:227`（`LockSessionForHuman` 的 `cache.Set`）、`:251`（`RenewSessionHumanLock` 的 `ttl<=0` 兜底）、**`:231` 的裸 `24*time.Hour`** |
| `lock.inbox_pending_ttl` | 300 | false | 同文件 `InboxPendingTTL`；读取点 `:290` `AppendPendingMessage` 的 `cache.LPush`（生产调用方 `internal/service/ai_debounce.go:68`） |
| `lock.inbox_content_dedup_ttl` | 300 | false | 同文件 `InboxContentDedupTTL`；经既有 `contentDedupWindow()`（`:140-145`，字段优先、零值回落）在 `inbox_ingress_ingest.go:167` 的 `SetNX` |
| `lock.ingest_lock_ttl` | 25 | false | 同文件 `IngestLockTTL`；读取点 `:635` `withIngestLock` 的 `SetNX`（生产调用方 `inbox_ingress_persist.go:137`） |
| `lock.ai_processing_ttl` | 120 | false | 同文件 `InboxAIProcessingTTL`；读取点 `:600` `markAIProcessing`（生产调用方 `:483`/`:585`，防重复回复的并发闸） |
| `lock.message_hub_idem_ttl` | 86400 | **true** | `internal/service/message_hub.go:172` `MessageHubDefaultIdemTTL`；读取点 `:255`（构造期 `idemTTL` 字段初值）与 `:373`（`cache.Set`） |
| `lock.ai_lock_ttl` | 15 | false | **刻意不接**，见下 |

`internal/service/inbox_ingress.go:231` 有一处**裸 `24*time.Hour`**（人类锁 reason 键的 TTL），与人类锁同一把但走字面量。本批一并接上与锁共用同一参数：理由先于锁过期 = 被接管会话在界面上显示不出「为什么被接管」，理由比锁活得久 = 一段无主数据，两者都不值得单开一个参数。

**`lock.message_hub_idem_ttl` 是本组唯一 `Restart: true`**：它的读取点在账号服务的构造期（`NewMessageHubServiceWithDB` 的 `idemTTL` 字段初值），而账号服务有 6 处构造点全在启动时完成（`qq_account.go:146`、`wecom_integration.go:34`、`feishu.go:115/562/932`、`email.go:55/63`），运行时改值对已启动的账号不生效，必须重启。同一条链上已有 `WithIdemTTL`（`message_hub.go:262`）可显式覆盖，但全树无生产调用。其余 5 条都是每请求/每次调用读，`Restart: false` 热生效。

**`lock.ai_lock_ttl` 刻意不接**：唯一读取点 `:265` 在 `tryAcquireAILock`，该函数自带 `//nolint:unused //// 仅被 *_test.go 引用，生产路径未用`，`ReleaseAILock`/`IsSessionAIBusy` 同样无生产调用。真正在跑的并发闸是 `ai_processing_ttl` 那条。保留「（未接线）」标注并在 Description 写明这层关系——**去掉标注测试会先红**（`TestAILockTTLStaysUnwired` 反向钉死）。

**`human_escalation.go` 三条常量不新增种子，只在文档记死代码事实**：`HumanLockDefaultTTL`/`HumanLockReasonTTL`/`LockExpiryCheckInterval`（`human_escalation.go:22/25/27`）所在的 `HumanEscalationManager` 在生产链路上死了 —— `NewHumanEscalationManager` 唯一生产引用为零（只有 `human_escalation_test.go` 19 处调用），`TriggerCensorshipEscalation`/`IsSessionLockedForHuman`/`StartLockExpiryChecker` 全树零生产调用。这是「运行时有同名锁、但实现不在这个文件」的典型：真正在跑的是 `inbox_ingress.go` 的 `InboxHumanLockKey` 系列（`hivemtk:lock:human:`）。给一条死代码建参数只会让人以为改它有用。

**`misc.polling_lock_stale_threshold` 早在 1.1 已接**，本批不重复。

**顺手修掉一个真 bug**：`internal/cache/memory.go` 的 `PopAll` 不判过期。`Get`/`Exists`/`LRange`/`scanExpired` 全都判 `item.expiration`，唯独 `PopAll` 只判 `listMode` 和 `len(listItems)`，所以 `LPush(key, v, 60ms)` 写入的列表过了 TTL 仍被原样 Pop 出来。影响面：只在内存缓存（`cache.NewMemoryCache()`，无 Redis 时的回退路径）上显形；Redis 路径由 `EXPIRE` 保证不受影响。对应业务：AI 防抖暂存的入站消息本该随 TTL 过期，内存回退下会被迟到的 Pop 重新投给 AI。修法是补一段与 `Get`/`Exists` 逐字同构的过期判断（`IsZero() && Before(time.Now())`），顺带把过期项从 LRU 摘掉。**这个 bug 是 `TestPendingMessagesExpireByConfiguredTTL` 首跑红暴露的**——不是因为接线写错，是「写对了才看得见底下有坑」。

**`lock` 组是方案新增的一个 group**（原计划倾向塞进 `session` 组）：这些是「正确性」型参数（改错 = 死锁或重复处理，不是调优），与 `session` 组「交互窗口」型混在一起会让运维在调 TTL 时误判影响面。每条 Description 都写「改小/改大会出什么事」。6 个 seam 集中在新建的 `internal/service/lock_idempotency_params.go` 单文件里：常量原本分处 `message_hub.go`/`inbox_ingress.go`/`inbox_ingress_ingest.go` 三处，散着看不出「锁的世界观」全貌。

**验证**：gofmt 空、`go build ./...`、`go vet ./...` 全过；`go test ./internal/cache/ ./internal/app/` 绿（app 100s）、`internal/service` 全包 **ok 852s**。门禁 `wired 110 / 已声明未接线 14 / UNDECLARED 0 / STALE 0`，rc=0，种子 124 条。行为级测试覆盖 6 条（`lock_idempotency_behavior_test.go`）：锁在注入的小 TTL 后真的消失、TTL 内第二次获取真的失败（防重复回复）、结构体字段非零时仍优先于 provider。

#### 2d 已落地（2026-10-10）

渠道与配额上限入库，新建 `channelbot`（5）+ `sse`（3）+ `bridge`（2）三组种子（124→134），`wired 110→120`、锚点同步到 134。

**15 个候选逐条核实，接 10 条**：

| 种子 key | 默认 | Restart | 代码点位与消费点 |
| --- | --- | --- | --- |
| `channelbot.qq_message_max_len` | 2000 | false | `internal/channelbot/qq/qq.go:38` `QQMessageMaxLen`；`SendMessage` 分段 + `splitQQMessage` 兜底，生产调用方 `service/qq_account.go:258` |
| `channelbot.tg_message_max_length` | 4096 | false | `telegram.go:26`；`SendMessage` + `splitMessage` 兜底，生产调用方 `service/telegram_gate.go` 6 处 |
| `channelbot.tg_inline_rows_max` | 100 | false | `telegram.go:27`；`buildInlineKeyboard` 截断 |
| `channelbot.tg_inline_buttons_per_row_max` | 8 | false | `telegram.go:28`；`buildInlineKeyboard` 截断 |
| `channelbot.http_timeout` | 30s | **true** | `channelbot/core/core.go:26`；`NewBaseClient` 构造期读两处（`:108` `HTTPClient.Timeout`、`:124` 字段） |
| `sse.heartbeat_interval` | 15s | false | `service/sse_hub.go:27`；`:392` `time.NewTicker` |
| `sse.max_conn_per_ip` | 5 | false | `sse_hub.go:28`；`:152` 每 IP 连接闸 |
| `sse.client_buffer_size` | 100 | **true** | `sse_hub.go:62`；`NewSSEClient` 构造期 `make(chan,…)` |
| `bridge.sse_max_backlog_events` | 1000 | false | `bridge/sse.go:57`；`:770` 回放截断 |
| `bridge.sse_bus_buffer_size` | 100 | **true** | `bridge/sse.go:58`；`:178` `buffer:` 构造期 |

三条 `Restart: true` 全是**构造期读取**：账号 HTTP 客户端、SSE 客户端事件信道、SSE 广播总线缓冲。运行时改值只对新建对象生效，已连接的客户端拿不到，必须重启——这一点写进 Description，否则运维改完看不到效果会以为又出僵尸参数。

**不接的 5 条**：`qq.CallbackOpVerify=13`（QQ 回调 event op 码，属协议常量不可调）；`bridge.sse_heartbeat_interval` / `bridge.sse_max_stream_duration`（**早已 DB 驱动**，`runtimeSSEHeartbeatInterval`/`runtimeSSEMaxStreamDuration` 已在读，不重复接线）；`wecom.error_rate_degrade`（承 1.8：`computeHealthScore` 的成功率判定是写死三档，单一阈值表达不了，保留未接线标注）。

**架构判断：两个子域走两条路，不要强行统一**。`internal/service` 反向 import `channelbot/{qq,telegram,core}`（`webhook.go:14-15`、`qq_account.go:14-15`、`telegram_gate.go:14`、`channel_error.go:12`、`feishu.go:24-25`），这三个子包若 import service 即成环 ⇒ 必须走 **provider seam + 装配层注入**（`internal/app/channelbot_params_wiring.go`，5 条显式闭包）；而 `internal/service/sse_hub.go` 与 `internal/bridge/sse.go` 本就能直接调 `GlobalConfigParam()`（`bridge/sse.go` 已有 `runtimeSSE*` 先例）⇒ 走**直连**，不造 seam。硬给后者造 seam 会多出 5 个一辈子只被读一次的函数。

**顺带修掉 1.8 发现的「配额降级半接线」**：`service/wecom_account_health.go` 的 `computeHealthScore` 原本有两套独立阈值——`quotaRate > 0.95` 写死扣 25 分，`quotaRate > quotaDegrade`（读 `wecom.quota_degrade`，默认 0.9）扣 15 分。运维把参数调到 0.95 以上时，最高档先命中，参数根本轮不到生效——**「已登记 + 已暴露 UI + 就是不改行为」比僵尸参数更隐蔽**。修法是引入 `quotaCriticalRate := 0.95; if quotaDegrade > quotaCriticalRate { quotaCriticalRate = quotaDegrade }`，取两者较大值：默认 0.9 < 0.95 时行为与写死逐档一致（零回归），调高后两档同时抬高。测试 `wecom_quota_degrade_tiers_test.go` 用 4 组参数值 × 9 个 `quotaRate` 采样点逐档钉住。

**种子的 Min/Max 一律贴平台硬上限**：QQ 2000、TG 4096、TG inline 100 行 × 8 钮都是平台自己的限制，填超了平台直接拒收。`TestChannelBotSeedsStayWithinPlatformCaps` 反向钉死「种子默认值与 `Max` 都等于平台上限」。

**本轮新踩的坑（前几批的 seam 测试可能也埋着）**：`SetXxxProvider(nil)` 是**空操作，不是复位**（`if fn != nil` 守卫）。在 `core`/`qq`/`telegram` 三个新测试里写 `t.Cleanup(func(){ SetXxxProvider(nil) })` 导致用例间串味（`TestNewBaseClientPicksUpTimeoutSeam` 读到上一个用例注入的 `90s`）。**正确做法是 `t.Cleanup` 里直接写包内 provider 变量**（`httpTimeoutProvider = nil` 等），同包测试是唯一能真正摘掉 provider 的地方。

**验证**：gofmt 空、`go build ./...`、`go vet ./...` 全过；`go test ./internal/app/`（ok 38s）、`./internal/channelbot/...`（4 包全 ok）、`./internal/bridge/`（ok 9.5s）、service 相关用例全绿。门禁 **`wired 120 / 已声明未接线 14 / UNDECLARED 0 / STALE 0`，rc=0，种子 134 条**。行为级测试：`splitQQMessage`/`splitMessage` 真按配置值分段、`buildInlineKeyboard` 真按配置值截断、`NewBaseClient` 真拿到配置超时、`WithTimeout(120s)` 仍赢过注入值。

### 阶段三：P1 130 个点位 + 前端接线

- 后端 130 个按 group 打包 PR。

#### 3.1 第一批已落地（2026-10-10）：9 条

**本批范围**（不是把 130 条一次做完，而是先取"有真实生产消费点 + 语义清楚"的一批）：

| group.key | 默认 | 代码点位 | 接线方式 |
| --- | --- | --- | --- |
| `confidence.humanize_boundary_low` | 0.70 | `internal/service/humanize/service.go` `DefaultBoundaryLow` | provider seam |
| `confidence.humanize_boundary_high` | 0.85 | 同文件 `DefaultBoundaryHigh` | provider seam |
| `confidence.humanize_sample_rate` | 0.10 | 同文件 `DefaultSampleRate` | provider seam |
| `confidence.humanize_max_retry` | 3 | 同文件 `DefaultMaxRetry` | provider seam |
| `sales.audience_default_limit` | 200 | `internal/service/audience_selector.go` `DefaultAudienceLimit` | provider seam |
| `sales.audience_max_limit` | 500 | 同文件 `MaxAudienceLimit` | provider seam |
| `misc.edit_lock_ttl` | 300 | `internal/service/customer_service_plus.go` `EditLockTTL` | provider seam |
| `memory.l1_ttl_hours` | 86400 | `internal/service/memory_system.go` `L1TTLHours` | provider seam |
| `geo.default_visibility_days` | 30 | `internal/geo/service/visibility.go` 原写死 `q.Days = 30` | **直连**，不造 seam |

**三条判断，写在这里免得下轮重新论证一遍**：

1. **humanize 四条与 `DefaultThreshold` 是同一套判据**。阈值定"谁算不达标"、边界定
   "哪些算边缘样本值得送 LLM"、采样率定"其中送多少"、重试定"重写几轮"。阈值早在
   阶段 1.2 接上了，剩下三条散着没人读参数中心，于是"改一个旋钮只动了四分之一"。
2. **`geo` 走直连而不是 seam**。`internal/geo/service` 早已 import `internal/service`
   （`llm.go`、`keyword_pipeline.go`），没有环可成；再造一层 provider 只是多一层转发。
   判据统一为「**读取点在不在 `internal/service` 包内**」：在则 seam，不在则直连。
3. **`q.Days <= 0 || q.Days > 365` 那一个条件原本兼任两件事**——"没给"和"给超了"共用
   同一个兜底 30。拆开后：没给走参数中心，给超了仍夹 365（查询成本闸，一次要回 365 行
   按引擎的聚合，不该跟着默认值一起被放大）。

**三个 float 参数的合法区间并不相同，必须逐条声明**（本轮写错过一次，把 0 当坏值）：

| 参数 | 合法区间 | 0 的语义 |
| --- | --- | --- |
| `humanize_boundary_low` | `[0,1)` | 0 = "所有样本都算边缘"，**合法** |
| `humanize_boundary_high` | `(0,1]` | 0 会让"边缘区间"这个概念消失，**非法，回落** |
| `humanize_sample_rate` | `[0,1]` | 0 = "一个都不送 LLM"，**合法** |

**顺带修掉 master 上一个既有红灯**：`config_param_test.go` 的 `TestDefaultParamDefsCount`
只把 `DefaultValue == ""` 一律判坏，而 PR#39 引入的 `lead.industry_profile`（自由文本行业
画像）合法地没有默认值——HEAD 上这条用例本来就是红的。改成"只对 `value_type != string`
要求有可解析默认值"，并把空默认的豁免面收成一条**具名清单**（`TestDefaultParamDefsEmptyDefaultRatchet`），
而不是"凡是 string 都放行"——否则这道门对自由文本的放行会变成又一个改了没人读的入口。

（**2026-10-12 收尾**：那条具名豁免清单现在是**空的** —— `lead.industry_profile` 补了
`DefaultLeadIndustryProfile` 常量，种子里的 `DefaultValue` 与该常量逐字一致，兜底位不再是空串
（`config_param_seeds.go:463-465`，判据与来路见 `docs/HARDCODE_INVENTORY.md` §8 的 G3 行）。
`TestDefaultParamDefsEmptyDefaultRatchet` 是双向棘轮：清单里有而种子不再为空 ⇒ 也判红，
所以这条豁免不会以"永久放行"的形式留在树里。`config_param_test.go:308` 现读 `allowedEmptyDefault := map[string]bool{}`。）

**仍未修的 master 红灯（他的文件，未动）**：`TestD12_NoNewLegacyKVDirectQuery` 在纯净 HEAD
上同样红，报的是 `internal/aiagent/llm/embedding_global_config.go` 直查 `system_config_kv`。
那是他自己的 D12 在途工作（`agent_llm.provider_failover_policy` / `embedding.global_override`），
往白名单里加一条就等于替一个真实违规盖章，等他本人定夺。

**门禁**：`wired 131 → 133`、`已声明未接线 14`、`UNDECLARED 0`、`STALE 0`，rc=0。
种子条目 138 → 147。**`internal/service` 全包从"2 红"变成"0 红"**：除上面那条
`TestDefaultParamDefsCount` 外，纯净 HEAD 上的 `TestD12_NoNewLegacyKVDirectQuery` 也红了
（报他自己的 `embedding_global_config.go` 直查 `system_config_kv`），已随他的 D12 收尾自行转绿。
- **前端**：15 个真阈值走一个 `GET /api/manage/config-params/public?group=frontend_ws` 之类的
  轻量只读端点（**不要把 `/api/manage/config-params` 原样暴露给终端用户**，那是管理端全量列表），
  前端在 Pinia 里建一个 `useRuntimeConfig()` 一次性拉取并缓存。
- 顺手把前端 1752 处自造默认分页/时间窗统一到 `pagination` group（分批，先改 `src/api/*.js` 的默认值，再改 `views/**` 的 `pageSize||20` 兜底）。

#### 3.1 第二批已落地（2026-10-10）：`webhook` 组 7 条

**本批范围**：webhook 回调配发的并发、限流与重试。七条全是"改错就丢消息"的那类旋钮，
且原本本已有一层 env 覆盖，只是参数中心看不见。

**优先级链**（每条都一样）：

```
env（WEBHOOK_WORKER_COUNT / QUEUE_SIZE / REPLY_CONCURRENCY）
  > config_params(webhook 组)
  > 常量兜底（WebhookDedupTTL / WebhookRateLimit / ...）
```

env 保留为**部署层应急开关**：改 env 必须重启进程，参数中心改完即生效。日常调参走
参数中心，临时顶一把才动 env。非法 env（空串/`abc`/`0`/负数）**不短路整条链**——
一个残留的空环境变量不能把运维刚调好的库值盖掉。

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `webhook.dedup_ttl` | 300 | `webhook_dedup.go` `WebhookDedupTTL` | 每请求（`isDuplicate` 的 SetNX） |
| `webhook.rate_limit` | 30 | 同文件 `WebhookRateLimit` | 每请求（令牌桶 refillRate，桶按 key 懒构造） |
| `webhook.rate_burst` | 60 | 同文件 `WebhookRateBurst` | 每请求（令牌桶 capacity/tokens 初值） |
| `webhook.max_retries` | 3 | `webhook_ai.go` 两处重试循环 | 每请求 |
| `webhook.worker_count` | 4 | `webhook.go` `NewWebhookService` | **构造期，Restart=true** |
| `webhook.queue_size` | 512 | 同上 `make(chan, N)` | **构造期，Restart=true** |
| `webhook.reply_concurrency` | 32 | 同上信号量 `make(chan, N)` | **构造期，Restart=true** |

**三条判断**：

1. **走直连而不是 provider seam**。七个读取点全在 `internal/service` 包内，本就看得见
   `GlobalConfigParam()`，与 3.1 第一批的 `geo.default_visibility_days` 同一判据
   （"读取点在不在 `internal/service` 包内"）。
2. **每个键拆成 raw / 带兜底两个函数**，不是为了对称——`scripts/check-config-param-readpoints.py`
   认的是"某一行里同时出现 `GetInt(`/`GetDuration(` 与键名常量"这个形状。第一版把读点藏进
   通用 helper 的形参（`webhookParamInt(ctx, key, def)`），门禁当场把 6 条判成 UNDECLARED。
   这是 1.1 批"`Get*(` 与键名同行"约束的另一种形态：**helper 的通用性是门禁的盲区**。
3. **`max_retries` 与 `agent_llm.max_retries` 同名不同义**（那一条管 LLM 调用本身的重试，
   本条目管渠道出网投递），Description 里写明区分。这与 2d 的
   `max_retries`/`heartbeat_interval` 同名是同一类问题：库里 key 是全局单列，同名只有
   靠复合唯一 + Description 区分，不能靠命名。

**`Min` 全部 ≥ 1（duration 那条 ≥ 10s）**：这一组没有"配 0 就照做"的合法语义——
并发 0 = 一个 worker 都不起、队列 0 = 每条消息同步处理、速率 0 = 全拒、burst 0 = 一条
都放不进去、去重 0 = 之后所有重试都被判重复而丢掉。读取侧再套一层
`positiveIntOr`/`positiveDurationOr` 兜底，防的是"库里已有坏值"（历史数据、直接改库、
绕过 service 写）——service 的 min/max 守卫管不到这三条来路。

**顺带修掉门禁的一个假阳性盲区**：`TestWebhookParamKeysAreAllRead` 逐条钉死"登记了必须
有人读"。读取点门禁只看 `Get*(` 的形状，看不出某个键是不是压根没被任何读口接过
（`max_retries` 就因 `agent_llm.max_retries` 已有 `CONST_ASSIGN` 而被门禁误判为已接线，
实际是另一条参数）。

**验证**：干净 worktree（`git worktree add --detach /tmp/hivemtk_v24 HEAD`，倒本批 7 个
文件）上 gofmt 空、`go build ./...`、`go vet ./...` 全过；`internal/service` 全包
**ok 434.777s**（0 失败）。门禁 **`wired 133 → 140`、已声明未接线 14、UNDECLARED 0、
STALE 0，rc=0**，种子条目 147 → 154、锚点同步到 154。
行为级测试：`TestWebhookTokenBucketPicksUpConfiguredRate`（burst=2 时前 5 次只放行 2 次、
换成 burst=4 后新 key 的桶放行 4 次）、`TestWebhookEnvWinsOverParamCenter`（env 压过库值，
四种非法 env 全部让位给库值）。

### 阶段四：字典表 + i18n（单独立项，不与阈值混做）

- 44 个 Go 字典 + `user-web/src/constants/*.js` 24 个字典**合并为一套 DB 字典**。
- 文案缺口（Go 9013 + web 7205 条 `API_MESSAGE`）按域批量补 i18n 词条，
  后端 `internal/pkg/i18n` 从 265 条扩到覆盖高频业务消息。

### 阶段五：把字典/文案从"清点"变成"进不来"

见 §6 门禁。

---

## 5. 前端读取配置的最小契约

```
GET  /api/manage/config-params              # 管理端全量（现有，admin）
PUT  /api/manage/config-params/:group/:key  # 改值（现有，admin）
POST /api/manage/config-params/:group/:key/reset
GET  /api/manage/config-params/audit-logs
```

新增（阶段三）：

```
GET  /api/config/runtime?groups=frontend_ws,frontend_ui,pagination
→ {"code":0,"data":{"frontend_ws":{"ping_interval_ms":"25000", ...}}, "message":"ok"}
```

要点：
- 只暴露 `ValueType in (int,float,bool,duration)` 且 `ReadOnly=false` 的项，**只出 value，不出
  description/source_ref**（避免把内部路径泄露给终端用户）。
- 前端拿到的值一律是**字符串**，与 `param_value` 列类型一致；类型转换在前端做，
  避免后端为两种客户端出两种 schema。
- 前端对未知 key 有 fallback 常量（现状），所以**新增参数不需要发版前端即可生效**。

---

## 6. 门禁清单（防回退，这是方案能不能活下来的关键）

沿用 `check-config-param-readpoints.py` 的「登记 ≠ 接线，靠机器守住」思路，扩成 6 道：

| # | 脚本（已有/新增） | 判据 | rc |
| --- | --- | --- | --- |
| G1 | `check-config-param-readpoints.py`（**改造**） | 每条 def 要么 `Wiring` 非空且函数存在，要么 Name 带「未接线」 | 1 |
| G2 | 同上 | 带「未接线」标注但已有 `Wiring` ⇒ 声明烂掉 | 1 |
| G3 | `check-config-param-defaults.py`（**新增**） | 每个 `DefaultValue` 必须在代码里至少有一处 fallback 常量且**值相等**（防"库里改了默认、代码兜底还是老值"） | 1 |
| G4 | `check-hardcode-tunable.py`（**新增**） | 跑 `hardcode_classify.py`，TUNABLE 且 `Owner` 已有归属的点位不得出现在新增代码里（**只卡新增，不卡存量**，存量靠阶段一二三四消化） | 1 |
| G5 | `check-config-param-keystyle.py`（**新增**） | key 命名规范 + 全局唯一 + group 必须在种子 group 白名单内 | 1 |
| G6 | `check-i18n-coverage.py`（**新增**，阶段四） | 域内中文文案必须走词条；按域设覆盖率基线，只许涨不许跌 | 1 |

全部接进 `make audit`。CI 触发 paths 必须包含各脚本 + `config_param_seeds.go`
（否则改判据不触这道门——这是现有 `check-config-param-readpoints.py` 注释里明确记过的教训）。

**G4 是整套方案的关键**：只卡新增不卡存量，才能让"新增硬编码"在 review 前就被挡住，
而存量靠阶段一二三四慢慢消化。一上来就卡全量会让所有 PR 全红，实际结果一定是大家学会绕过门禁。

### 6.1 七道门已落地（G1–G6 于 2026-10-11，G7 于 2026-10-12）

七道门全部接进 `make audit`（紧邻 `check-config-param-readpoints.py`，读数连着看），
并接进 `.github/workflows/user-server-ci.yml` 的 `static-gates` 作业。
CI 触发面按 `check-ci-gate-paths.py` 的纪律登记：**门脚本本体 + 它读的判据基线**
都要进 `on.push.paths` 与 `on.pull_request.paths` 双份，缺一份就是
"只改判据/只动基线的提交不重跑这道门"。

| # | 脚本 | 判据（落地形态） | 当前读数（2026-10-13 08:35–08:38 复跑，测于 **tip `15ccabb7` 影子树 ＋ 本泳道 45 个改动文件**那一面，即"提交后别人克隆到就应当复现"的那棵树；同一批门在工作树上跑（同一 tip，多并行泳道的在途件全在）**判据行逐字节相同**，只有 G7 的扫描面变：影子 `7245 路径／7226 文本` vs 工作树 09:01:28 现测 `7248／7229`。**差在哪是现取的**：G1／G3／盲区的枚举源是 `Path.rglob`（`check-config-param-readpoints.py:123,192`、`check-config-param-defaults.py:310`、`measure-tunable-blindspot.py:103`）——只认磁盘目录树，两棵树里本泳道的文件都实实在在在盘上，所以读数逐字相同（G1 `Go 1867／Get* 232`、G3 `Go 1768／常量池 2822`）；只有 G7 用 `git ls-files --cached --others --exclude-standard`（`check-text-encoding.py:34,103`）把**未跟踪且没被忽略**的文件并进来，09:01 那趟差集逐条列出来是 3 个、都不在本泳道名单里（`user-server/docs/capabilities.md`、`user-server/internal/bridge/testdb_helper_test.go`、`user-server/internal/service/menu_ceiling_test.go`），08:58 那趟是前两个。这一格几分钟就在动（工作树 08:21＝7237／7218 → 08:57＝7247／7228 → 09:01＝7248／7229），**引它只能连时间戳一起引**。另有两处**上一版记成"树间差"、其实是"tip 间差"**的数，一并订正：`Get* 行 230 → 232` 与 G7 的 `7234／7215 → 7245／7226` 都发生在 `f7381630 → 15ccabb7` 那三笔之间（他们新增了 `data_scope` 面的读取点、又把 `MenuRouteAudit` 的 9 个证据文件入库），对账口径与本轮两次自我订正见 `docs/HARDCODE_INVENTORY.md` §7.1 注。**这整格读数的 base 已在 09:19 又被推前**：GitHub master 现为 `dd7ee2e6`，其中含 `user-server/go.mod`／`go.sum` 的 11 项 go-minor 抬级（这两枚连同 `user-web/bridge/package.json`／`package-lock.json` 本泳道均未触碰，`git status --porcelain -z` 现测四条＝非脏），所以新依赖集下这批 Go 侧门**没有复跑过**，分叉与取证见 `未完成任务.md` §7） |
| --- | --- | --- | --- |
| G1+G2 | `check-config-param-readpoints.py`（改造） | **新增两类**：代码里有 `GetX(ctx, g, k, …)` 而种子里没有 `(g,k)` ⇒ `ORPHAN_READ`（每次读都 miss、每次回落兜底，而页面上看不到旋钮）；出现种子里没有的组名 ⇒ `ORPHAN_GROUP` | 217 条种子 / **wired 204** / 未接线且已声明 **0** / 已下架 13 / **UNDECLARED 0 / STALE 0 / ORPHAN 0**（前端真实读取点 17 个 group.key；这 17 是按 `rt*` 调用点数的，不是按 `runtimePublicGroups` 白名单点数——白名单当过读取点证据的那段原委见 `docs/HARDCODE_INVENTORY.md` §7.1 的「wired 这一格为什么从 206 掉到 204」） |
| G3 | `check-config-param-defaults.py` | 认 `Get*(ctx,g,k,<兜底>)` 的**实参位置**（五种兜底形态），不比名字；再加种子自己过 Min/Max | 种子 217 / 常量池 2822（同名歧义剔除 209）/ 定位到兜底点的键 187；四向对账 **可比对 184 + 无法比对 3 + 前端下发 17 + 已下架 13 = 217**；**不一致 0 / 越界 0**；覆盖率棘轮 `scripts/config-param-defaults.baseline` = 184 |
| G4 | `check-hardcode-tunable.py` | 命名带 `Threshold/Limit/TTL/…` 的常量，既不在 `scripts/hardcode-tunable.baseline` 里、又**不是**任何 `Get*()` 的兜底实参 ⇒ 红 | Go 常量声明 **294** / 基线 TUNABLE **326** / **新增未入库 0** / 已作兜底 3 / 豁免 52 |
| G5 | `check-config-param-keystyle.py` | key 匹配 `^[a-z][a-z0-9_]*$`、长度 ≤ 64、非 SQL 关键字；`(group,key)` 不重复；key 无连续重复词（`dedup_dedup_ttl`）；runtime 白名单组必须在种子里真实存在 | 217 条 / 35 组 / **违规 0 / 重复条目 0 / 重复词 0 / 幽灵组 0** |
| G6 | `check-i18n-coverage.py` | 三条棘轮 + 一条硬门，见下 | t() 去重键 923 / 真 i18n 键 183 / 覆盖率 19.83% / 中文直传 740 / 孤儿词条 1455（zh 1846）/ 9 语言齐整 |
| G7 | `check-text-encoding.py`（**新增**，2026-10-12） | 文本文件的三条编码判据：严格 UTF-8 解码（`NOT_UTF8`，无豁免）、U+FFFD 替身字符按**次数**放行 `scripts/text-encoding.baseline` 登记的刻意用法（新增红 / 变小 STALE 红）、双重编码签名（`MOJIBAKE`，建门时全仓 0 处故无豁免）；缺基线 / `git ls-files` 失败 / 扫描面为 0 / 基线登记的文件不在面上或读不到 ⇒ **rc=2** | 扫描面 `git` 列出 7245 路径 / 文本 **7226** 个（二进制按前 8 KiB 无 NUL 跳过 19 个）/ **NOT_UTF8 0 / FFFD 超基线 0 / MOJIBAKE 0 / STALE 0** / 基线 1 处（`message_hub_inbox_conversation.go:318` 的 `strings.ToValidUTF8`）；牙齿常驻 `scripts/check-text-encoding.test.sh`（21 枚控制，假 git 树夹具）。7245 这一格 = 已跟踪文件 ＋ 未跟踪未忽略件，本泳道那 5 个新脚本／基线件在 `git add` 之前正是加数；工作树同 tip 现测 7248／7229（09:01:28），多出的 3 个是并行泳道的在途未跟踪件（见本表表头那份逐条名单，08:58 那趟只有 2 个）——这一格随他们的落盘在动，只能连时间戳引。耗时 2026-10-13 现测：user CPU 1.62–1.69s（两棵树各测两次），wall 1.95–6.17s——wall 受同机负载摆动，不是可比口径 |

**G7 是这一批里唯一由"发现即建门"生出来的门**：它抓的不是配置改造的漏项，而是改造过程中
被静默损坏的两处中文注释——`user-web/src/layout/Layout.vue:143` 的注释里是 **2 个连续 U+FFFD**、
`user-server/internal/service/config_param_test.go:335` 是 **3 个**（两处都在 HEAD 的 blob 里
按 `git show HEAD:<路径>` 现数，不在工作树里——工作树已经是修好的那份）。
建门过程本身有两条值得留下的教训：

1. **门自己先红了一次**：docstring 里举的签名实例被自己的正则命中。这不是门的缺陷，
   是它的第一个有效发现——"举实例"这种写法在编码门里必须换成转义描述。
2. **不带 `-z` 的 `git ls-files` 会静默漏扫中文名文件**：默认 `core.quotePath` 把非 ASCII
   路径八进制转义并加引号，门于是"打不开"这 5 个文件（含 `改进清单.md`）。第一版的自证只印
   `读不到跳过 5 个`——计数完全看不出"没扫"。现在列面用 `-z`，且读不到的路径**按名字打印**，
   并新增一格判据：基线登记的豁免对象若不在面上或读不到 ⇒ rc=2。

G7 建门当趟在真树上做过 14 格反向对账（三种内容红各一格、撤探针复绿、基线抬高与撤豁免两向、
缺基线 / 幽灵登记 / 空扫描面各退 2、中文名文件真在面里、未跟踪件真在面里）+ CI 侧两格
（摘 `push.paths` 两行 ⇒ `check-ci-gate-paths.py` 点名这两行；摘 `pull_request.paths` 同）。
**这套对账已经常驻成电池**：`scripts/check-text-encoding.test.sh`（21 枚控制，全部跑在
`mktemp -d` 出来的假 git 树里，不碰工作树），每枚控制另跑过 10 枚专杀变异确认它不是橡皮章，
并已挂进 CI `static-gates`（紧跟门那一步）。**上一段那 14 格是来路，不是当前防线**——
当前防线是这 21 枚，怀疑门坏了直接复跑 `bash scripts/check-text-encoding.test.sh`，
现测读数见 §7.1 那张表。

**G6 的三条棘轮**（存量太高，卡全量会让 CI 永远红，等于没有门）：

- `resolved_ratio` = 能解析到词条库的点分键 / 全部 `t()` 键，**不许跌**（19.83%）
- `raw_t_calls` = 把中文字面量直接当 `t()` 键传入的去重条数，**不许增**（740）
- `locale_orphans` = locales 里无人引用的词条数，**不许增**（1455 / 1846）
  - 口径：`menu.*` 里被后端菜单注册表（`user-server/internal/model/menu.go` 的
    `var SystemMenuTree`，208 个 key）点名的词条**不计入**——它们的消费方是拼接键
    `t('menu.' + key)`，静态 `t()` 扫描抓不到，但页面确实在用。注册表读不到或解析出
    的 key 少于 20 个时本门直接 rc=2，不给"豁免集为空 ⇒ 全量放行"的假绿留位置。
    注册表之外残留的 `menu.xxx` 死词条照旧计入孤儿（2026-10-12 反向实测：注入一条
    `menu.menuNotInRegistry` 后 1455→1456，门红）。
- 硬门：非基准语言词条数不得少于基准语言（9 种语言目前齐整）

**四条与方案原文不符、已按实测改写的判据**：

1. **G4 的"已接线"判据不是名字匹配，是实参位置**。接线时刻意保留原常量名
   （避免把参数化放大成一片 diff），所以常量名与种子 key 往往毫无关系：
   `HTTPIngestMaxMessages` 的 key 是 `bridge.ingest_max_messages`、
   `L1WindowSize` 的是 `memory.l1_window_size`。按名字配对只能靠一张人工映射表，
   而映射表必然和代码漂移。改判据为：**该常量是否出现在 `Get*(ctx, g, k, <常量>)`
   的兜底实参位**（含直连范式 `GetInt(..., 0)` 后 `return <常量>`）。
2. **G6 必须认 `$t()` 形态**。实测 918 个去重键里 **716 个走 `$t()`、只有 203 个走裸
   `t()`**（vue-i18n 组合式 API 的全局属性形态）。只认裸 `t()` 会漏掉 78% 的 i18n 使用，
   而且漏得悄无声息——数字看着仍然是个正数，只是全错。本门第一次写完就踩了这个。
   （**这两个分子是发现当时的读数**，2026-10-12 复跑分母已是 923、真 i18n 键 183；
   比例口径没变，别拿 716/203 去和今天的 183 对账。）
3. **G6 的"覆盖率"不是"中文有没有进 i18n"**，是"写了 `t()` 的里有多少真解析得到"。
   实测的真 i18n 键只有 178 个（2026-10-11 收官快照；2026-10-12 现测 **183 / 923 = 19.83%**），
   另有 **740 处把中文当 `t()` 的键直接传入**——
   这种调用查找必然 miss，vue-i18n 回落成 key 本身，于是"中文看起来正常、8 种语言全是中文"。
   它比不写 i18n 更坏：不写至少能被扫描器数出来，写成 `t()` 之后人 review 时会以为这页已国际化。
4. **G1 的孤儿判定必须支持组名常量**。`GetInt(ctx, ParamGroupMemory, Key, fallback)`
   这种写法里第一个实参是标识符而非字符串字面量，正则只认字面量就会漏掉整条读取点。

**G1/G4 各抓出一条真问题（都已修）**：

| 门 | 抓出 | 真因 | 修法 |
| --- | --- | --- | --- |
| G1 | `agent_llm.geo_pipeline_batch_per_run`、`bridge.sse_max_stream_duration` 各一条孤儿 | 代码注释里写着"可经该 key 调整"，但种子里根本没登记 ⇒ 每次读都 miss、每次回落兜底，运维在页面上也看不到旋钮 | 补种子两条（当批 212→214；2026-10-12 现测 217） |
| G4 | `memory.l1_window_size` 真漏接 | `L1WindowSize = 10` 没有任何读取点走参数中心 | `runtimeL1WindowSize(ctx)` 直连读取 + **补上 `BuildFullContext` 里漏掉的一处**（LLM 提示词的生产构造入口，原先直接传常量，运维调了不生效——同文件 `L1List`/`l1Trim` 都读参数、唯独提示词不读，这种半接线最难发现） |

**顺带修掉的两个既有缺陷（规则 2）**：

- `audit_api_contract.py` 的按名推断表只有 `auth`/`public`/`router`，不含 3.2/4.1 引入的
  `plainAuth`/`admin` 形参名 ⇒ `GET /api/config/runtime` 与
  `GET /api/config/dictionaries/:parent` 被判成"前端调了后端没有"，`--strict` 直接红。
  按该脚本**已有**的按名推断规则补齐（两者装配处分别绑到 `auth := r.Group("/api")`
  与 `systemAdmin := auth.Group("")`，前缀都是 `/api`），UNMATCHED 2→0。
- `check-hardcode-tunable.py` 的跳过目录判定原写 `seg in p.parts`，而 `p` 是绝对路径
  ⇒ 仓库只要检出在含 `tmp`/`test`/`migrations` 段的位置（本仓的干净 worktree 验证就在
  `/tmp` 下），**每个文件都被跳过**。改为只看相对扫描根的路径段。
  这个 bug 是干净 worktree 验证抓出来的——主仓路径不含这些段，门一直是绿的。

---

## 7. 风险与回滚

| 风险 | 概率 | 影响 | 缓解 |
| --- | --- | --- | --- |
| 参数被改坏导致线上事故（尤其锁 TTL、熔断阈值） | 中 | 高 | ① 所有"正确性类"参数（TTL/锁/熔断）配 `Min/Max`；② P0/P1 逐条 PR 不打包；③ 每 PR 附"改值→观察→改回"证据；④ `config_param_audit_logs` 可回滚到 old |
| 60s 缓存导致"改了不生效"的二次误导 | **高** | 中 | 接线 PR 的验收必须显式验证"改库 → ≤60s 生效"；必要时对 P0 参数单独缩短 TTL |
| 参数中心变成"什么都往里塞"的垃圾桶 | **高** | 中 | G5 强制 key 规范与 group 白名单；§1 非目标写死；DATA 类强制走字典表 |
| 启动期 seed 撞唯一索引被静默丢弃 | 中 | 高 | 阶段 0.3 修索引；G5 校验唯一性；`SeedConfigParams` 的 `logger.Warnf` 升级为返回 error 让启动失败 |
| 前端无 `t()` 兜底导致白屏 | 低 | 中 | 前端所有读配置处保留原常量 fallback；新增 `useRuntimeConfig()` 失败即回落常量 |
| 72 条僵尸接线改动面过大，一次 PR 难 review | **高** | 中 | 按 group 拆 8 个 PR（见 §7 阶段一分组），每个 PR ≤ 15 个点位 |

---

## 8. 验收标准（方案生效的判据）

以下是方案初稿写的口径，逐条跑完发现**第 1、2、5 条的口径本身有错**，已按实测改写并注明原口径为什么不可达。改写不是放宽标准——错的口径无论接线做多少都达不成，留着只会让台账失真。

1. `python3 scripts/check-config-param-readpoints.py` → `未接线但未声明=0` 且 `声明过期=0`。
   - 原口径写的是「`未接线且已声明=0`」，**方向反了**：「已声明未接线」恰恰是本方案要保留的那一类——如实标注「为什么没接线」比偷偷留一个永远不生效的旋钮要好。
   - 真正必须为 0 的是另外两项：**未声明未接线**（有旋钮没人读、也没人承认）与**声明过期**（已经接线了却还标着未接线）。
   - 实测（2026-10-11 阶段六收官）：`wired 206 / 已声明未接线 11 / UNDECLARED 0 / STALE 0`，rc=0。
     **2026-10-12 复跑**：`wired 204 / 未接线且已声明 0 / 已下架 13 / UNDECLARED 0 / STALE 0 / ORPHAN 0`，
     rc=0。`wired` 掉 2 不是有人拆了接线，是 G1 的前端判据本身被修严（白名单不再当读取点证据），
     那两条改挂 `deprecated=true`——机制见 `docs/HARDCODE_INVENTORY.md` §7.1。

2. `python3 scripts/hardcode_inventory.py --summary` → Go 侧 TUNABLE 从 **338 → 278**（当批读数；2026-10-12 现测 258 未接 ＋ 已接 120 ＝ 可迁 378，见下一条），且 `WIRED + PARAM_FALLBACK` 两栏非零。
   - 原口径写的是「TUNABLE 346 → 0」，**口径本身自相矛盾**：接线时按约定**保留原常量名**（避免把一次参数化放大成一片 diff），所以接完之后常量名字里依然带 `Limit/TTL/Threshold`，在按名字形状分类的清单里和没接的一模一样。原实现把这两类混在一起，接线做完后「可行动点位」不降反升（338 → 370），验收标准变成一个永远达不成的数。
   - 修法：清单**复用 G4 的实参位置判据**（常量出现在 `Get*(...)` 的兜底实参位，或直连范式里 `Get*` 兜底传 0 而随后回落到该常量），把 `WIRED`（确实已接线）与 `PARAM_FALLBACK`（`Default*` 前缀＝参数化标记，G4 `DEFAULT_PREFIX` 豁免的同一口径）单独摘出。
   - **两个脚本口径必须一致**——否则台账说「已参数化」而门说「这是硬编码」，两边数字无法互相校验。
   - 实测（2026-10-11 阶段六收官）：`WIRED 14 / PARAM_FALLBACK 78 / TUNABLE 278`。
     **2026-10-12 复跑**（`--summary` 的 `user-server CONFIG` 三行）：`TUNABLE 258`（未接）
     `+ WIRED 89 + PARAM_FALLBACK 31` = **Go 侧可迁总量 378**；前端另 `TUNABLE 50 / WIRED 17`。
     TUNABLE 不为 0 是正常的：3.1 只做到 P1/P2 优先级，剩下的是 P2/P3 档。
     逐档分布与"已接 120"的对账见 `docs/HARDCODE_INVENTORY.md` §7.1（那张表按同一次复扫写）。

3. `make audit` 七道门全绿（G1–G7；G7 是 2026-10-12 补的文本编码门），且 G4 只卡新增（已实现：基线独立成 `scripts/hardcode-tunable.baseline`，刷新用 `--refresh-baseline`）。

4. **人工验收清单（每次发版前抽 3 条）**：
   - 改 `lock.inbox_human_ttl` → 收件箱人工接管锁的过期时间变化（2c，已补行为级测试）
   - 改 `wecom.success_rate_degrade` → 企微账号健康分档位变化（本批，递减关系由代码 clamp 强制）
   - 改 `pagination.cursor_page_size` → 前端列表页实际每页条数变化（1.7）
   - ~~改 `allow_insecure_webhook` → 新 webhook 开始/停止验签~~ **已作废**：2b 判定「安全绕过开关刻意不 DB 化」，它只在 env 里，不在参数中心，改了不会有任何变化。

5. 参数中心页面上标着「（未接线）」的条目**逐条都能说出为什么没接线**（不必为 0）。
   - 原口径写的是「不存在任何标着（未接线）的条目」。这要求把 14 条全部接线或全部下架，但其中 9 条**接不了**：`/ws/chat` 路由从未注册、`ParsePagination` 只有测试调用、`NewResultCache` 生产链路从未构造——这些是**死代码事实**，不是没做的工作。强行接线只会造出一批「读取点存在但整条链路是死的」假接线（阶段 1.8 在 `channelgw.ws_push_interval` 上就栽过这种：key 写错导致每次读都 miss，门禁还判它已接线）。
   - 已当轮处理能处理的：下架 3 条（`wecom.error_rate_degrade` 拆成三条真实阈值、`confidence.persona_*` 两条因实现随 `9a5f716c` 删除），`未接线 14 → 11`。
   - 余下 11 条的分类处置见 `docs/HARDCODE_INVENTORY.md` §7.2。

---

## 9. 与已有门禁/规范的关系

- 复用 `scripts/check-config-param-readpoints.py` 的纪律与注释风格（它已把"读点口径看不见什么"
  写进文件头——本方案新增的 G3/G4 同样必须写清自己的盲区）。
- 五层架构不受影响：`ParamRegistry` 是 Service 层的东西，
  `dictionaries` 走标准 `Model → Repository → Service → Controller → Router`。
- 不引入新框架：不加 ORM 之外的依赖，不改 `config_params` 已有列语义，
  新增列全部可空并给默认值，**旧数据零迁移风险**。

---

## 10. 落地记录（按批次追加）

每批参数/端点落地后在本节末尾追加一段记录，**小节编号沿用其所属阶段**（所以本节里会同时出现
`3.1 第三批`、`4.1~4.3`、`阶段六` 这样的编号），不改动上方 §3/§4 的方案正文。
每条记录给出「`group.key` → 代码点位 → 生效时机」，并写明装配路径已核实的那一段——
这是与"僵尸参数"的区别所在。

### 10.1 记录清单

#### 3.1 第三批已落地（2026-10-10）：`sop` 组 6 条

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `sop.dispatcher_worker_count` | 16 | `sop_dispatcher.go` `DefaultSOPDispatcherConfig` | 构造期 · Restart=true |
| `sop.dispatcher_queue_capacity` | 1000 | 同上 | 构造期 · Restart=true |
| `sop.dispatcher_llm_concurrency` | 4 | 同上 | 构造期 · Restart=true |
| `sop.compensation_max_attempts` | 3 | `sop_compensation.go` `DefaultCompensationConfig` | 构造期 · Restart=true |
| `sop.compensation_ttl` | 30 | 同上 | 构造期 · Restart=true |
| `sop.compensation_max_plans_kept` | 512 | 同上 | 构造期 · Restart=true |

**六条全部是构造期读一次**：调度器的 worker 数、队列 channel 容量、LLM 信号量在
`NewSOPExecutionDispatcher` 里 `make(chan, N)` / 起 goroutine 时定型；补偿管理器的
`MaxPlansKept` 决定的是内存里保留的计划表大小。改完只对下次启动的进程生效，
故六条全标 `Restart=true`。

**装配路径已核实**（这是本批与"僵尸参数"的区别所在）：
- `cmd/api/main.go:380` `InitSOPExecutionDispatcher(db, sopSvc, nil)` —— cfg 传 **nil**，
  旧代码在这里回落写死的 `16/1000/4`。改成 `cfg == nil` 时读
  `SOPDispatcherConfigFromParams(ctx)`。
- `cmd/api/main.go:384` `InitSOPCompensation(execDispatcher)` —— 受
  `FF_LTC_SAGA_COMPENSATION` 开关控制（**默认关闭**）。开关开启时才构造管理器，
  所以这三个补偿参数在现网默认配置下不被读取——但这不是"僵尸"：开关一开就生效，
  Description 里写明了这层关系。

**两条判断**：
1. **走直连不造 seam**：读取点全在 `internal/service` 包内。
2. **`DefaultSOPDispatcherConfig()` 里没纳入参数中心的四项**（`MaxAttempts` /
   `InitialBackoff` / `MaxBackoff` / `BackoffMultiplier`）只从字面量提成导出常量
   `DefaultSOPDispatcher*`，**语义不变**。理由：它们是单条 SOP 节点的重试与退避，
   已经有一套独立的按 dispatcher 注入的配置通道（`SOPDispatcherConfig` 结构体本身），
   再造参数中心入口只会让同一个旋钮有两个 UI，改哪个生效说不清。
   `CompensationConfig.TotalTimeout` 同理（仍取 `DefaultCompensationConfig()` 的 5m），
   避免两处各写一份 5 分钟。

**一个需要说明的口径**：`sop.compensation_max_attempts` 的 `Min` 是 `0`（"显式关掉
补偿重试"是合法取值），但读取侧 `sopCompensationMaxAttempts` 仍把 0 当"没配过"
回落 3。二者只在"重启前已构造的管理器"这一层有区别，Description 里写明了。

**验证**：干净 worktree `/tmp/hivemtk_v25`（@`4ca99e15`）gofmt 空、
`go build ./...` 全过；`internal/service` 全包 **ok 476.324s**（0 失败）。
门禁 **`wired 140 → 146`、已声明未接线 14、UNDECLARED 0、STALE 0，rc=0**，
种子条目 154 → 160、锚点同步到 160。
行为级测试：`TestNewSOPExecutionDispatcherNilConfigPicksUpParams`（走生产的 nil cfg
路径，断言 workerCount / 队列容量 / LLM 信号量真的是参数值）、
`TestInitSOPCompensationPicksUpParams`（开关开启后装配出的管理器真的用参数值，
且 `TotalTimeout` 未被参数化）。

#### 3.1 第四批已落地（2026-10-10）：`domain_health`(4) + `rag_health`(7) 共 11 条

**域名健康度 4 条**——"看一眼分数就做决定"的类型，改错的结果不是性能问题而是
"该切的没切"：

| group.key | 默认 | 生效时机 |
| --- | --- | --- |
| `domain_health.score_healthy` | 80 | 每请求（参与选优 / 状态提升 / 无可用域名报错） |
| `domain_health.score_switch_threshold` | 30 | 每请求（自动切换触发线，须低于 score_healthy） |
| `domain_health.consecutive_failure_limit` | 3 | 每请求（"分数还没掉下来就先切走"的旁路） |
| `domain_health.check_timeout` | 5s | **构造期，Restart=true**（`http.Client.Timeout`） |

**RAG 健康度 7 条**：`default_window`（1h）+ 六维权重（0.10 / 0.30 / 0.15 ×4，
总和 1.0）。

**本批最关键的一个设计决定：六个权重按总和归一化后使用。**

`report.Score` 是六个 `WeightedScore` 直接相加再取整（`rag_health.go:132-136`），
等级表 `scoreToGrade` 按 90/75/60 分档。如果权重不归一化，运营把某一项调成 3 就会
让**总分超出 0-100、等级分档全部错位**——而且症状是"分数突然变 300 分会"，不是
"比重变了"，极难定位。归一化后每个旋钮的语义变成干净的"相对比重"：
`召回质量` 调到 3 就是"召回占总分 3/(3+其余)"这么多。

六项全被写成 0（或非正）时**回落常量权重**：那时 `report.Weight` 会是 0、
任何维度看起来都是 0 分，比百分比失准更难排查。读取侧 `positiveFloatOrDH` 把
单一项的 0 也当"没配过"回落常量——想降比重应该调小，清零只会让总分少一块而看不出原因。

**两条判断**：
1. **走直连不造 provider seam**：两组读取点全在 `internal/service` 包内。
2. **`domain_health.check_timeout` 是四条的 Restart 异类**：它在 `http.Client`
   构造时定型，改完只对下次新建的服务实例生效，故单独标 `Restart=true`；
   其余三条每请求读，标 false。

**一个测试上的坑（已写进测试注释）**：`setDHRaw` 直接写库绕过 service，
而 `ConfigParamService.invalidate` 只在 `UpdateValue`/`Reset` 路径上被调——
所以"先 `UpdateValue` 再直接写库"的混合用例，60s 内读到的还是旧值。
修法是 `refreshParamCache(db)` 换一个冷缓存的服务实例。这不是新问题，
是 3.1b 起就存在的 `configParamTTL` 语义，只是本批第一次把它撞出来。

**验证**：干净 worktree `/tmp/hivemtk_v26`（@`d9f8fa3e`）gofmt 空、
`go build ./...` 全过；`internal/service` 全包 **ok 452.764s**（0 失败）。
门禁 **`wired 146 → 157`、已声明未接线 14、UNDECLARED 0、STALE 0，rc=0**，
种子条目 160 → 171、锚点同步到 171。
行为级测试：`TestDomainHealthNewServicePicksUpTimeout`（新服务实例的
`http.Client.Timeout` 真的是参数值）、`TestRagHealthWeightsNormalizedToSumOne`
（召回调成 3.0 后占主导且六项和仍为 1.0）、
`TestRagHealthWeightsFallBackWhenAllZero`（全 0 回落常量）、
`TestRagHealthWeightIndividualFallback`（只回落那一项）。

#### 3.1 第五批已落地（2026-10-10）：`feedback` 组 22 条（阶段三收尾）

反馈学习闭环全部旋钮。**22 条全部构造期读一次，故全标 `Restart=true`**：
五个组件的配置在 `InitFeedbackLoopComponents` / `InitFeedbackCollector`
（`cmd/api/main.go:408/418`）里组装完就固化在各组件的 `config` 字段里。

| 子域 | 条数 | 参数 |
| --- | --- | --- |
| `bandit` | 6 | `min_samples_for_exploit` 30 / `exploration_floor` 0.10 / `traffic_ceiling` 0.60 / `convergence_threshold` 0.95 / `min_samples_for_promote` 100 / `posterior_samples` 1000 |
| `collector` | 3 | `queue_size` 1000 / `flush_interval` 2s / `batch_size` 50 |
| `analyzer` | 5 | `min_reward` 1.0 / `cluster_sim_threshold` 0.85 / `min_cluster_size` 3 / `top_k_per_cluster` 3 / `max_dialogues_per_run` 500 |
| `iterator` | 4 | `min_samples_for_iteration` 50 / `negative_reward_threshold` -0.5 / `candidates_per_run` 3 / `auto_approve` false |
| `optimizer` | 4 | `auto_apply_priority` 2 / `rollback_drop_threshold` 0.20 / `rollback_complaint_ratio` 0.50 / `ab_test_duration` 7d |

**两条克制（不做什么）**：
1. **`DefaultSignalWeights`（12 键信号权重表）刻意不入库**。它是一张
   `dto.FeedbackSignalKey → float64` 的双射表，拆成 12 条参数后每一行仍是
   "配了这一格才覆盖一格的权重"，运维要看全貌反而更难，而且 key 是 `dto` 常量
   （与 `model/feedback_loop.go:39` 的注释绑定，改了要同步两处）。要动它应走
   **字典表（阶段四）**，不是参数中心。
2. **`iterator_auto_approve` 默认 false，Description 写明了"改成 true 等于把
   提示词改写这条产线全自动"**。它是本组唯一一个"改了影响每一通对话"的开关，
   与 2b 的验签旁路同类——但区别在于它**不翻安全闸**，只翻产线行为，
   所以入库 + 默认关闭是正确的处置。

**本批一个真实的设计坑：`fbMissingFloat` 哨兵。**
`iterator_negative_reward_threshold` 的默认是 **-0.5**，而 `0` 是它的合法语义
（"关掉这道负向门限"）。第一版照抄正值参数的写法用 0 当 `GetFloat` 的 fallback，
结果「没配过」和「配了 0」都落到兜底 -0.5 上——运维填 0 后看不到任何变化，
且没有报错。改用 `math.NaN()` 当哨兵：库里存不下 NaN（`validateValue` 走
`strconv.ParseFloat`），所以它**只可能来自 fallback 这一条路**。
同类参数（合法取值可以是 0 或负数的）今后一律走 `fbOrAnyFloat`。

**验证**：干净 worktree `/tmp/hivemtk_v27`（@`e86ab783`）gofmt 空、
`go build ./...` 全过；`internal/service/...` **10 个包全绿**
（service 453.953s、feedback_loop 14.407s）。门禁 **`wired 157 → 179`、
已声明未接线 14、UNDECLARED 0、STALE 0，rc=0**，种子条目 171 → 193、
锚点同步到 193。
行为级测试：`TestFeedbackConfigsFromParamsMatchDefaults`（无库时五个组件逐字段
等于各自的 `DefaultXxxConfig()`——这是"参数缺省时行为不变"的总闸）、
`TestFeedbackFloatFallbackSemantics`（`fbOrPosFloat` 与 `fbOrAnyFloat` 的分界）、
`TestFeedbackParamKeysAreAllRead`（22 条逐一改库后断言对应字段真的变了）。

**阶段三到此收尾**：3.1 五批共接 **54 条**（9 + 7 + 6 + 11 + 22），门禁从
`wired 131` 到 `wired 179`，种子从 138 到 193。

#### 3.2 已落地（2026-10-11）：`GET /api/config/runtime` 只读端点

前端拿运行时配置的唯一出口。**三条判据写死在代码里，不给「可配置」的口子**：

1. **只返回白名单内的组**（`runtimePublicGroups`：`frontend_ws` / `frontend_ui` /
   `pagination`）。传入的 groups 里不在白名单的**静默忽略**——不报错也不回显，
   免得调用方拿它当「探测有哪些组」的手段。
2. **只返回 value**（`group.key -> string` 扁平映射）。`Name`/`Description`/`Min`/
   `Max`/`Restart`/`ReadOnly` 一个都不给。`/api/manage/config-params` 是**管理端全量
   列表**，把它暴露给终端用户等于把整套运维决策面摊开——里面既有
   `misc.upload_max_size_mb` 这种可直接放开的口子，也有 `read_only` 标记为系统推导的键。
3. **不读数据库时返回空 map + code 0**，前端据此用代码兜底值。不能因为运维配置面挂了
   就让整个 SPA 白屏。

**顺带修掉一个后端口径缺口（规则2，当轮修）**：`setupConfigParamRoutes` 此前把那五个
全量/写端点挂在 `auth` 上，而 `auth` **只要求登录、不要求管理员**——任何普通登录用户
都能改上传上限、改验签旁路、改锁 TTL。前端侧 `router/modules/system.js` 的
ConfigParams 路由一直标了 `requiresAdmin: true`，两端口径不一致，靠前端藏菜单挡着。
本批把五个端点改挂 `systemAdmin`（已过 `AdminAuthMiddleware` 的 auth 子组），
`plainAuth` 只留只读的 `Runtime` 一条。

**顺带补掉两处 nil DB panic（同为规则2）**：
- `ConfigParamRepository` 七个方法（`List`/`ListByGroup`/`GetByGroupKey`/`UpdateValue`/
  `ResetToDefault`/`BulkResetGroup`/`AuditLogs`）在 `r.db == nil` 时 `WithContext` 直接
  panic。统一返回哨兵错 `ErrConfigParamStoreUnavailable`（与 0.4 的
  `ErrKVStoreUnavailable`、0.5 的 `ErrSystemConfigStoreUnavailable` 同一口径）。
- `ConfigParamService` 的 `List`/`ListByGroup`/`ResetToDefault`/`BulkResetGroup`/
  `AuditLogs` 补 `s.repo == nil` 守卫；`ConfigParamController.Runtime` 补 `c.svc == nil`
  守卫——**Runtime 是前端启动链上的一环，它 panic 会让整个 SPA 打不开**，这一条比
  别的端点更要 nil-safe。

**验证**：干净 worktree `/tmp/hivemtk_v28`（@`400a79fc`）gofmt 空、`go build ./...`、
`go vet ./internal/...` 全过；`controller`(95.859s) / `repository`(76.867s) /
`router`(11.095s) / `service`(474.315s) **全绿**。
门禁读数不变（`wired 179 / 已声明未接线 14 / UNDECLARED 0 / STALE 0`，rc=0）——
本批不新增参数键。
行为级测试：`TestRuntimeReadsRealValues`（真库 seed 后把 `pagination` 组一条改成 77，
断言端点读到 77）。

#### 3.3 已落地（2026-10-11）：`useRuntimeConfig` + 19 条前端阈值入库

**前半：参数入库（`wired 179 → 198`，种子 193 → 212）**

新增 `frontend_ws`(12) + `frontend_ui`(7) 两组共 19 条。这些常量的读取点在 `user-web`，
后端 Go 代码里一个都没有——所以它们不是" wired "，而是换了一种 wired。

| 组 | 条数 | 覆盖的常量 |
| --- | --- | --- |
| `frontend_ws` | 12 | agentSocket / chatSocket 的重连次数、首连延迟、退避上限、心跳、ACK 合批 ×2，journeyTracker 的 history/queue/flush ×3 |
| `frontend_ui` | 7 | 审批未读轮询、坏案例统计轮询、浏览器监控失败上限、深链扫描页宽、素材库上限、WS 重连最大等待、toast 时长 |

**后半：门禁的判定粒度从组细化到键（这是本批最关键的一处）**

`scripts/check-config-param-readpoints.py` 原本只认 `user-server` 下的 `Get*(` 调用。
`frontend_ws`/`frontend_ui` 的读取点在前端，门禁看不见 ⇒ 两个选择都错：
全判 UNDECLARED（前端明明在读，门在撒谎），或整组挂「未接线」（文案在撒谎）。

解法：**让前端自己的注册表成为事实源**。`controller/config_param_runtime.go` 的
`runtimePublicGroups` 白名单就是那份注册表，门禁直接解析它。

粒度必须到 **`group.key`** 而不是 `group`：`pagination` 组整体给前端，但组里
`page_max_size` / `page_default_size` 两条的后端消费点是 0（`ParsePagination` 全树只有
测试调用）。第一版按 group 判定，门禁当场抓出 2 条 STALE——那两条的 Description 里
明写着「当前不生效」，判成已接线就是让门禁自己替谎话盖章。于是补一层
`runtimePublicKeys` 键级排除清单，两处由同一个脚本核对。

**前半里两个自查出来的真 bug（都是规则2，当轮修）**

1. `num()` 对 `'0.4'` 不回落：`Number('0.4')=0.4 > 0` 通过检查，之后 `Math.trunc`
   截成 0。**判下限必须在 `Math.trunc` 之前**——否则 0.4 秒的心跳间隔截成 0 就是
   "每条消息都发一次 ping"。改用 `n < 1` 判定。
2. `bool()` 把「配了 false」也当没配：第一版"非 true 一律回落 fallback"，结果库里明写
   `false` 的开关在 fallback=true 的调用点显示成开——**运维在控制台关掉的开关，
   前端照样开着，且没有任何报错**。改成三分：没配过才回落，`false`/`0` 就是 false，
   认不出的脏值回落 fallback。

**前半里最重的一处：`useRuntimeConfigStore()` 在无 Pinia 环境会抛，把调用方整条路径干掉**

仓内既有的 `customerSession_deeplink` / `browser_monitor_resilience` 等测试文件 mount
组件时没 `app.use(pinia)`。`useRuntimeConfigStore()` 在这种环境直接抛
`getActivePinia() was called but there was no active Pinia`，于是深链扫描不再发那一次
宽列表请求（实测 26 例前端渲染测试同时红）。

修法是加一层 **nil-safe 读取层** `utils/runtimeConfig.js`：`rtNum/rtDur/rtStr/rtBool`
内部 try `useRuntimeConfigStore()`，catch 后回落 fallback，并 only-once 打一行 warn。
所有调用点（2 个 util + 5 个 .vue）都改走这一层。兜底常量全部保留，取值就是改造前的
硬编码值 ⇒ 这两种环境下行为与改造前**逐字节一致**。

代价是静默回落。这是故意的取舍：这些阈值全是前端自己的行为旋钮，拉不到配置时站点必须
还能跑；store 内部已把失败记进 `failed` 标志，需要时可查。

**验证**：
- 后端：`internal/service` 全包 ok（锚点同步到 212）；门禁 **`wired 179 → 198`、
  已声明未接线 14、UNDECLARED 0、STALE 0，rc=0**，种子 193 → 212。
- 前端：**`vitest run` 53 个文件 582 例全绿**（新增 `tests/unit/runtimeConfigStore.test.js`
  10 例，覆盖三种坏输入 × 四个读取口 + 截断顺序 + 秒→毫秒 + 非对象响应）；
  `eslint` **0 errors**（18687 warnings 是仓内既有基线）；`vite build` 通过。

#### 4.1~4.3 已落地（2026-10-11）：业务字典两表 + 种子 + 前后端读取

**与 `config_params` 的分界（这是两套机制并行以来第一次把边界写下来）**：

| | 管什么 | 一个 key 对应 |
| --- | --- | --- |
| `config_params` | 「这一个旋钮拧到几」 | 一个标量值（int/float/bool/duration/string） |
| `dictionaries` | 「有哪几种状态」「A 能不能到 B」 | 一组枚举项 + 一组跃迁 |

不让字典寄生在参数中心上的理由：一个 5 状态的审批流要 5 条 string 参数 + 一张 10 行的
有序二元组，而参数中心既没有顺序也没有「成组」的概念，翻出来是 15 行平铺记录，
运维看不出它们是一套。字典表用 `parent_code` 分层表达「选项」与「跃迁」两种行，
一次查询就能拿到整张状态机。

**五层全套**（Model/Repository/Service/Controller/Router）：
- `internal/model/dictionary.go`：`Dictionary` + `DictionaryTransition`。复合唯一
  `(parent_code, code)` 而不是单列——不同域完全可能各有一个 `pending`
  （`approval_status.pending` 与 `bad_case_status.pending` 是两条合法行），
  单列唯一会逼着人给键名加域前缀，那正是 config_params 在 v3.47.0 之前踩过的坑。
- `internal/service/dictionary_seeds.go`：5 个状态域种子（approval_status /
  bad_case_status / human_task_status / opportunity_stage / payment_status），
  共 21 项 + 22 条跃迁，与 `model/*.go` 的常量逐字对应。
- 建表走 `RegisterExtraModels`（AutoMigrate），**没有写 migration**。

**读取口径：DB 优先，DB 没有这一批就回落 Go 种子。** 不反过来（常量优先、DB 只做覆盖），
因为字典描述的是**存量行正在用的值域**——历史数据里躺着的状态码必须一直在册，
否则前端翻译存量值时会翻车。回落常量还保证「没配过字典的站点行为与今天逐字节一致」。

**与 config_params 种子不同的一点**：那边的 DefaultValue 是「没人配置时的行为」，
可随代码演进调整；这边的种子是**存量数据的值域**，删一项等于让历史行变成非法值。
因此字典种子只增不改，要停用一项走 `Enabled=false`，不删行。`SystemLocked` 标记的项
管理端不能改名或停用（代码是权威的那一侧）。

**4.3 的受控边界（没有把前端 24 个字典全搬）**：
`src/constants/*.js` 那 24 个是含 `label/tagType/sort/group` 的**富展示对象**，
与后端字典表的 `code/name/description/sort_order` 不是同一种东西。全量搬要么给后端
加一堆展示专用列，要么前端丢掉 tagType（筛选色就没了）。所以只做三件事：
1. 建后端只读端点 `GET /api/config/dictionaries/:parent`（挂 plainAuth）+ 管理端 CRUD
   （挂 admin），两条返回体一致、只差鉴权。
2. 建前端 `useDictionaryStore`，口径与后端一致（DB 优先 → 回落本地），失败静默回落，
   且**不写进 loaded**（否则后端恢复后前端会永久用本地常量）。
3. 剩下 21 个纯展示字典保持本地，在文档里记明边界。

**顺带修掉一个 3.2 引入的测试样本错误**：`TestRuntimeReadsRealValues` 原本取
`pagination` 组第一条，而该组前两条恰好落在 `runtimePublicKeys` 排除清单里
（后端消费点为 0，故意不下发）。那是排除逻辑按设计工作，不是 bug——测试改取
`pagination.cursor_page_size`（明确断言它不在排除清单里）。

**验证**：干净 worktree `/tmp/hivemtk_v29`（@`5de6b1d4`）gofmt 空、`go build ./...`、
`go vet` 全过；`internal/service`(427.641s) / `controller`(108.689s) /
`router`(15.952s) / `repository`(90.585s) / `migration/...` **全绿**。
前端 `vitest run` **54 文件 590 例全绿**（新增 `tests/unit/dictionaryStore.test.js` 8 例）。
门禁读数不变（`wired 198`，rc=0）——本批不新增 config_params 键。

---

#### 阶段六已落地（2026-10-11）——清零最后一类债务：11 条僵尸旋钮**下架** + 配置页重做 + 字典管理页

上一阶段收尾时，`HARDCODE_INVENTORY` §7.2 记着 11 条「已声明未接线」，当时的结论是
「8 条要动死代码与路由、3 条要动产品决策，没有一条是再花半小时接线能解决的」。
这一阶段把那个结论兑现了——**不是把 11 条接上，而是把 11 条下架**。

##### 6.1 为什么是「下架」而不是继续挂着标注

前 11 个阶段里，「未接线」这个状态一直是个**过渡态**：条目知道自己没接线，
`Name` 上挂「（未接线）」、`Description` 上挂「【当前不生效，改了也没人读】」，
门禁认它为「已声明未接线」并放过。但过渡态放着不收，最终就是永久态——
页面上永远挂着 11 个改了不生效的旋钮，用户改完排查半天，日志里连一条
「这个旋钮根本没接线」的记录都不会有（因为请求确实成功返回了 code=0）。

下架（`deprecated=true`）是**终态**：它承认「这个旋钮不对外存在了」，
并且必须写清为什么下线、将来要接回来得先做什么。于是：

| 处置 | 从管理台 | 从读路径 | 门禁 |
|---|---|---|---|
| 未接线（过渡） | 可见可编辑，改了不生效 | 读不到 | 第五类之一 |
| 已下架（终态） | **默认不可见**，带开关才显示且禁用编辑 | 仍可读 | 新增 `已下架` 计数 + `接上了却没摘下架标记` 判红 |

读路径**刻意保留**读取能力这一点值得单说：`ConfigParamService` 内部缓存加载
（`getString` 里的 `ListByGroup(ctx, group, true)`）传的是 `true`，
管理端 `List` 传的是查询参数默认 `false`。两者口径故意相反——
管理端不该给用户看改了不生效的旋钮，但代码侧必须能读到它，
否则「有人重新接线却忘了摘下架标记」会退化成最隐蔽的那种故障：
每次读都 miss、每次静默回落兜底值，而页面上那个旋钮写着「可配置」。

写入侧同步收口：`UpdateValue` / `ResetToDefault` 遇已下架直接返回
`ErrParamDeprecated`，**不做「静默成功」**。理由：API 收下改动 + 审计表留下一条
`update` 记录 + 用户在页面上看到「保存成功」，然后发现行为没变——这三件事叠加起来
比直接报错糟糕得多，它把一个五秒钟能发现的错误变成一个查不出根因的错误。
`BulkResetGroup` 则是**跳过**而非整组失败，因为整组重置的语义是「这组回到出厂」，
一条下架项让全组失败对用户毫无意义。

##### 6.2 顺手删掉的死代码（不是顺手清理）

下架 11 条的同时删掉了三段**只被测试引用**的生产常量：
`internal/bridge/handler_http.go` 的 `HTTPPollingMaxTimeout` / `HTTPPollingDefaultTimeout`
（服务端没有长轮询实现）、`internal/bridge/bridge_helpers.go` 的 `maxReplyContentBytes`
（自带 `//nolint:unused`）。删它们不是洁癖：这两个常量的存在本身，
就是在证明「有个旋钮改了不生效」这件事还有代码在维护它。
连带重写了 `defaults_test.go`、删了 `handler_http_test.go` 里两个只测它们的用例。

##### 6.3 三组新字段：`Unit` / `Risk` / 组元数据

217 条参数此前只有 12 个定义列，UI 能显示的只有 key/name/description/类型/值/范围。
「用户能不能配得友好」卡在三处，全部需要后端先有数据：

- **`Unit`（95 条）**：只给「填这个数字时用户怎么理解」用。`duration` 和 `bool`
  一律不填——前者的单位恒为秒、后者的单位根本不存在，UI 自己知道，
  填了是噪声。像 `top_k`、`max_concurrent` 这类「数出来的东西」也不填。
- **`Risk`（52 条，闭集 `""` / `warning` / `danger`）**：`danger` 34 条全是
  「填错会直接挂」的（锁 TTL、上传上限、body 上限、队列容量、并发闸）；
  `warning` 18 条是「不会挂但会静默改业务语义」的（自动回复开关、评分阈值）。
  用字符串而不是 bool 存，是因为空串本身就有意义（「普通」），
  用 bool 表达三态要么多一列要么让 false 同时表示「普通」和「未标注」。
- **`config_param_groups` 新表**：35 组的**中文名、说明、排序**。
  此前 29 组中文名硬编码在前端 `GROUP_LABELS` 里——那正是「新增一个组没人知道叫什么」
  的根因。组元数据独立成表而不是加列在参数上，有三个理由：
  ① 35 组 × 几十条参数会让同一个中文标签重复几百遍；
  ② 排序是**组级属性**，参数表里没有位置；
  ③ 组说明挂在单条参数上错位（「本组控制所有锁的超时时长」不是任何一条参数的说明）。
  建表走 `RegisterExtraModels`（AutoMigrate）而**不写自建表 migration**——
  阶段四已经在字典表上踩过这个坑：`TestFullMigrationChainUpThenRollback`
  的「Down 不删不属于它的表/索引/列」规则会把自建表迁移判红。

**组元数据的 seed 与参数 seed 解耦成两个入口**这件事也是被测试逼出来的：
`seedParamGroups` 原本挂在 `SeedConfigParams` 尾部，导致 **27 个只声明
`&model.ConfigParam{}` 的测试 fixture 全部撞 `relation "config_param_groups" does not exist`**。
拆成独立的 `service.SeedParamGroups(ctx, db)` 之后，两个入口天然表达了
「哪张表和哪张表绑着」这条知识。拆分的判据是**失败后果不同级**：
参数值没进库 ⇒ 管理台看不到、代码读兜底值，行为与声明不一致，必须冒泡；
组元数据没进库 ⇒ 侧栏那列回落英文 group key，纯外观降级，
所以 `main.go` 里它只 `logger.Errorf` 不阻断启动。

`ListGroups` 的排序有个坑：原来的 `Order("sort_order")` 会把
`sort_order = 0`（哨兵值，意为「没排过序」）顶到**最前**，
于是每加一个新组，侧栏顺序就在两次发布之间悄悄变。
改成 `Order("(sort_order = 0)").Order("sort_order").Order("param_group")`——
Postgres 布尔序 false<true，天然把排过序的排在没排的最后。

##### 6.4 门禁加第五类「已下架」

`check-config-param-readpoints.py` 的 `parse_entries` 返回值从 4 元组扩到 6 元组，
主分类从四类扩成**五类互斥**：`deprecated` 先于其他判据分诊——
「不再对外承诺」不属于「接线做得好不好」。两条新判红：

- **RETIRED_BUT_WIRED**：已下架却接上了。后果是管理端 `List` 默认过滤掉它，
  运维看不见也改不了，但参数被改过、行为跟着变，却找不到在哪改的。
- **DEPRECATED_NO_REASON**：只有「已下架」标签，没有理由。
  用户既不知道为什么，也没有办法知道将来怎么接回来——这跟没标注是一个问题。

**下架与未接线的分工**：未接线是过渡态（承认「现在改了不生效」），
下架是终态（承认「这个旋钮不对外存在了」）。两者都留着就是债。
本阶段把 11 条从过渡态推到终态，`已声明未接线` 归零。

##### 6.5 配置页重做：`ConfigParams.vue`

| 原缺口 | 现在 |
|---|---|
| 11 条僵尸旋钮看不出来 | 「显示已下架」开关（默认关）+ 行底色变红 + 徽标 + tooltip 给出**下架原因与时间** + 编辑/重置按钮禁用 |
| 29 组中文名硬编码在前端 | 删掉 `GROUP_LABELS`，改读 `GET /manage/config-params/groups`，按 `sort_order` 排（`0` 排末尾）；后端没给元数据时回落英文 key 而不崩 |
| duration 一律显示「N 秒」 | `formatDuration()`：60~3600 整除才升「分钟」，<86400 整除才升「小时」，≥86400 显示「N 天 M 小时」。**只在整除时升单位**——90 秒显示「1.5 分钟」不如「90 秒」 |
| 无 `Unit` / `Risk` / `Restart` 提示 | 单位后缀（duration/bool 不挂）、名称列挂「高危」「敏感」「需重启」徽标、编辑框内按四档给不同的 alert |
| 高危参数无警示 | `risk === 'danger'` 保存走 `ElMessageBox.confirm`（`type:'error'`，文案列出当前值与将改为的值），保存按钮改 danger 色 |
| 编辑框内无法恢复默认 | footer 加「恢复默认值」，调 reset 接口并把表单填成默认值，同时提示「记得点保存提交」 |
| 搜索仅组内 | 加 `searchScope`（本组 / 全部组）。**无搜索词时永远只看当前组**（左侧分组栏是主导航），输入搜索词后才按 scope 搜；跨组时表格自动多一列「分组」 |
| 状态标记漏进界面 | `stripSuffix()` 剥掉 Name 里的「（未接线）/（已下架）/（只读）」与 Description 里的「【…】」前缀——那是给扫描器和审计看的 |

还有三处**判断**值得记下来：

1. **JSON 识别不按 `value_type` 白名单**。4 条 `string` 参数里有 3 条实际装的是
   JSON blob 但类型写的是 `string`，只认 `json` 类型会让它们退化成单行输入框。
   改成：`type == json` **或** key 以 `_json` 结尾 **或** default/value 以 `{`/`[` 开头
   ⇒ 走 textarea + `JSON.parse` 语法校验 + 「格式化 JSON」按钮。
2. **零值拦截按 key 语义分档**。int 一律拦 0；duration 只拦 key 匹配
   `/ttl|buffer|size|cap|limit|timeout/i` 的——因为 duration 的 0 到底是「零秒」
   还是「永不过期」取决于读取侧有没有回落兜底，前端不可能知道，
   但对明显是上限/超时的旋钮拦下来，比事后排查便宜。
3. **前端兜底过滤是有意的冗余**。后端 `List` 默认过滤 `deprecated=true`，
   前端又加了一层 `visibleParams` computed。看起来重复，但这一页**唯一不能出的错**
   就是「页面挂着改了不生效的旋钮」——而那种错会同时留下一条 `update` 审计记录，
   是所有配置故障里最难排查的一种。前端过滤是最后一道防线，
   哪怕后端哪天回归了这道防线还在。

统计卡第 4 张从「只读参数」改成「高危参数」（danger 计数）；
侧栏每组的重置徽标改为排除已下架项（后端 `BulkResetGroup` 会跳过它们，
报一个含它们的条数是骗人）。

##### 6.6 字典管理页：`DictionaryManager.vue`

阶段四建好了字典的完整五层与 CRUD 端点，但**前端一个视图都没有**——
只有 `src/api/runtimeConfig.js` 里的 `dictionary(parent)` 与 `dictionariesApi`。
这一阶段补上管理页 `/system/dictionaries`：

- 左侧字典域列表：中文 label + 英文 `code` + 「代码种子」标签 + 项数徽标。
  为此给 `DictionaryService.ListParents` 的返回类型从 `[]string` 改成
  `[]DictionaryParent`（含 `Label` / `Description` / `ItemCount` / `Seeded`）。
  **为什么不让它继续返回 `[]string`**：管理端只能显示英文 key，
  前端就会再硬编码一份中文映射——`config_param_groups` 正是这么出问题的，
  不想同一个坑踩第二次。`Seeded` 标出来是为了让运维知道哪些域还有代码种子兜底。
- 右侧上块「字典项」：Code / 名称 / 说明 / 排序 / 状态 / 操作。
  `system_locked` 的项带「锁定」标签与 tooltip：
  「代码常量是权威的一侧，改名会让两侧分叉」——这正是字典表存在的理由，
  所以锁的语义必须在界面上说清楚。
- 右侧下块「状态跃迁」：`from_code`/`to_code` 用 items 建 code→中文名映射补上，
  否则运营盯着两个英文 code 看不出这是「待审批 → 已批准」。这一格是整个字典管理页
  最有价值的部分——状态机才是业务方真正想维护的东西，字典项本身反倒是配角。
- 「新增字典项」对话框：code 必须匹配 `/^[a-z][a-z0-9_]*$/`（与后端 G5 键规范门同源），
  `system_locked` 项禁改 code。说明文字讲清「停用而不是删除，因为历史行还躺着这个值」。
- 「替换状态跃迁」对话框：`ReplaceTransitions` 是**整体替换**语义
  （先清空再写入），所以 el-alert 里明写这一点，且提交前必须 `ElMessageBox` 确认；
  空行与「起点=终点」都在前端拦下并给出行内错误——因为整体替换漏掉一条
  会在下次提交时被静默抹掉，用户根本不会发现。

##### 6.7 顺带修掉的既有红灯

`TestConfigParamUpdateResetAudit` 拿 `bridge.polling_default_timeout` 做样本，
而那条已下架 → 被新的 `ErrParamDeprecated` 护栏拒了。护栏在正确工作，
但基线用例不该因此失去样本，换成 `bridge.online_grace_window`
（默认仍是 30s，两条断言的期望值一字未改）。

##### 6.8 验证

前端：`vitest run` **56 文件 605 例全绿**
（新增 `configParams_render.test.js` 8 例 + `dictionaryManager_render.test.js` 7 例），
`vite build` 通过（PWA precache 572 entries），`eslint` 对两个新/改 `.vue` 文件 0 errors。

门禁：`已声明未接线` 从 11 归零、`UNDECLARED` 0、`STALE` 0、`RETIRED_BUT_WIRED` 0；
keystyle 门 217 条 / 35 组 / 违规 0 / runtime 幽灵组 0；契约门 `--strict` UNMATCHED=0
（1032 个前端调用全部可解析并与后端匹配，含本批新增的 `/manage/config-params/groups`
与字典四端点）。

后端：**这里原本写的是「干净 worktree 上八包全绿」，是错的**——首轮干净检出验证里
`internal/app` 就是 FAIL（build failed，`migrate.go` 混了他人 WIP 的
`&model.RoleMenuPermission{}`），`internal/service` 的结论行又被管道截断没看到。
按「不记假绿」改写如下，逐条都是实测：

| 包 | 结果 |
|---|---|
| `internal/router` | **ok 18.578s**（`TestFrontendAliases_NoFailedRegistration` PASS 4.83s，修掉 `/api/system/menus` 重复注册之后复跑） |
| `internal/app` | 定向 ok 0.803s（`TestAILockTTLStaysRetired` / `TestMessageHubIdemTTLIsMarkedRestart`） |
| `internal/service` | **整包 ok 1350.258s**（连跑两遍，两遍都无 FAIL；`TestLockGroupSeedDefaultsPassValidation` 修完后复跑）；定向 ok 8.330s |
| `internal/{controller,repository,model,bridge,pkg/db}` | 全 ok（首轮 HEAD 全量验证即通过） |
| gofmt / `go build ./...` / `go vet ./...` | 全过（`GOFMT_RC=0 / BUILD_RC=0 / VET_RC=0`，且未接管道，退出码可信） |

`internal/service` 整包在本轮 HEAD 全量验证里失败过一次，原因分两块：
`TestLockGroupSeedDefaultsPassValidation` 是**测试没跟上本批下架改动**（已修，见 6.9 第 3 条）；
`TestRecognize_MultiSession` **单跑稳定通过（1.02s）、整包高负载下偶发失败**，
代码核实已排除本批改动（`GetRecentIntents` 是纯 DB 查询无 TTL；`embRetryCooldown()`
未注入时逐字节等于原常量；`recognizeByEmbedding` 在 `embedSvc == nil` 时直接返 nil，
根本不碰 cooldown 逻辑）。

**结论：判为既有用例的 flaky，本轮不修。** 专门重跑整包两遍取证——两遍都绿
（`ok 1350.258s`，耗时是平时的三倍多，因为两遍连着跑且同时有 `go build` 在抢 CPU），
**这一条记下来不是为了说明它没问题，而是为了说明它确实失败过一次**：
它在 HEAD 全量验证那一轮真的红了，只是重跑不复现。判 flaky 的依据是「同一棵树、
同一份代码，单跑稳定通过、整包偶发失败」，而不是「这次没复现就算过」。
按「不记假绿」，本条留在台账里；真要根治得改那条用例对异步落库的等待方式
（`time.Sleep(200ms)` 换成轮询），属于意图识别的测试质量，不在配置治理范围内。

##### 6.9 归属登记：本阶段随他人提交入库 + 接手他人提交留下的三条红灯

**归属**（第三次被夹带，前两次是 `86b92349` 的 confidence 批次、`40a19abb` 的 2d 生产代码）：

阶段六的 28 个代码文件 + 2 份文档，在我做完干净检出验证、正准备提交时，
被一次并行的 `feat(rbac): user-web 菜单权限 + 角色菜单分配 + 收件箱数据权限`
（`64b46dd1`）一并带走，该提交的 commit message 与这些文件的内容毫无关系。
逐文件核对（`git diff --quiet HEAD` 对 28 个文件逐个检查为空 + `git grep`
抽查 `config_param_groups` / `已下架` / `DeprecatedReason` / `deprecated_reason` / `risk`）
确认内容完整无缺，双远端也已含，故不改写已推送历史，只在此登记归属。

**代价**：暂存区是共享的，「验证通过后立刻提交」这条纪律连续第三次失效。
未造成内容丢失，但每次都要在最后回头核对一遍归属，是纯粹的返工。

**接手的三条红灯**（他人提交把两处冲突一起带进 master）：

1. `/api/system/menus` 重复注册 —— `menu_routes.go:25` 的
   `adminSystem.GET("/menus")` 与既有 `frontend_aliases.go:442` 的
   `doReg("GET","/system/menus", …)` 撞同一路径，`doReg` 对已注册路径 panic，
   整个 `internal/router` 包起不来。
   删掉旧别名不是绕过测试，是修真 bug：旧 `SystemMenus` 自陈「当前开源版无动态菜单配置，
   返回空数组供前端做初始渲染占位」，该前提已被 RBAC 推翻；前端 `RoleList.vue:285`
   读 `treeRes?.tree || treeRes?.data?.tree`，新 handler 返回 `{"tree":…}` 命中，
   而旧桩返回 `[]any{}` 落到 `undefined` —— **菜单分配对话框一个节点都渲染不出来，
   RBAC 特性看起来接上了，管理员却无法给任何角色分配菜单**。
   处置：删 `frontend_aliases.go:441-442` 两行（连带 `systemInfoCtrl` 变量）与
   `controller/system_info.go` 的 `SystemMenus` 方法，两处原位各留注释写清理由。
2. G6 判 4 个 `t()` 键解析不到词条 —— 同一提交里 `RoleList.vue` 用了
   `t('role.assignMenus', '分配菜单权限')` 这种形式：中文放在第二参数 fallback 位，
   **界面看着正常，但其余 8 种语言无词条、全回落中文**（"看起来做了 i18n，
   实际八种语言全是中文"，比不写 i18n 更难发现）。给 9 种语言各补 4 个键后门绿。
   不动 `t()` 的 fallback 参数——它是词条缺失时的兜底，留着无害且是防御。
3. 两处我自己写的反向钉死测试未跟上本批下架改动：`lock.ai_lock_ttl` 从「未接线」
   改判「已下架」后，`TestLockGroupSeedDefaultsPassValidation` 与
   `TestAILockTTLStaysUnwired` 还在断言「未接线」。跟着改成断言「已下架」+
   `d.Deprecated == true`，并**反向断言 Name 不含「未接线」**
   （已是终态就不能同时挂过渡态标注，否则 readpoints 门会判声明过期）。
   后者重命名为 `TestAILockTTLStaysRetired`，让名字与它断言的状态一致。

##### 6.10 接手他人提交暴露的前端测试基础设施缺陷（不是配置治理，但会阻塞 CI）

上一条红灯修完之后，前端全量测试冒出**一批与本任务毫无关系的新失败**。它值得单独
一节，因为它比任何一条配置缺陷都更容易让人做出错误判断。

**取证（同一棵树、同一份代码，前端全量 605 例连跑四次）**：

| 场景 | 失败数 | 涉及文件 |
|---|---|---|
| 空闲 | **0** | — |
| 中等负载（与 go test 并行） | **7** / 4 个 | browser_detail_run_error、browser_duration_text、browser_monitor_resilience、clue_list_filter_push |
| 重负载 RUN 1 | **30** / 15 个 | 横跨 approvalTask / badCase / browser_\* / clue_list / cohort / followups / messageHub / router 等互不相干的页面 |
| 重负载 RUN 2 | **11** / 4 个 | approvalTask_render、badCase_render、clue_list_filter_push、error_reporting_console_scope |

失败集合每次都不同、且这 4 个（乃至 15 个）文件**单跑全绿** ⇒ flaky 坐实，
且与本批改动零交集（本批只动了 9 个 i18n json、3 个 router 用例、两页新配置界面）。

**根因（定位到机制，不是猜）**：`@vue/test-utils@2.4.11` 的 `flushPromises()`
实现是 `new Promise(resolve => scheduler(resolve, 0))`，也就是**一次宏任务**，
只排空**微任务**队列。而本仓的组件用例普遍用「刷新 N 次」等渲染稳定
（`tests/unit/` 里裸 `await flushPromises()` 98 处 / 21 个文件，
`for (let i=0;i<N;i++) await flushPromises()` 15 处 / 8 个文件），
**对组件里 `setTimeout` 驱动的防抖搜索、轮询、重试退避一个都不等**。
空闲时 N 次碰巧够用；worker 一抢 CPU，定时器回调就排在 N 次刷新之外，
断言在渲染完成前就去读 DOM。

**为什么必须当成真问题处理**：这类失败比真 bug 更坏——它每次落在不同的文件上，
让人怀疑刚改的代码，久了整套测试就没人信了。而它同时会**阻塞 CI**：
本仓 CI 里前端是并行跑的，红灯是随机的、无法复现、无法定位。

**处置（两处）**：

1. 新建 `user-web/tests/unit/settle-helper.js`，导出 `settle(w)`：每轮
   「`await flushPromises()` + 让出一个宏任务」之后比对一次 `w.html()`，
   **连续两轮渲染结果相同即认为稳定并早停**，`MAX_ROUNDS = 40` 只作硬上限。
   不用「多刷几次」那种概率式写法——那只是把同一个问题往后推。
   自己新写的两个文件（`configParams_render.test.js` 14 处、
   `dictionaryManager_render.test.js` 17 处）已全部改用它，残留裸调用为 0。
2. `vitest.config.js` 加 `fileParallelism: false`。这一步一次性消灭整类假失败。
   代价是 56 个文件串行约一分多钟，注释里写明了将来要恢复并行，
   必须先把 `settle()` 铺进所有用例，**而不是把这个开关调回 true**。

**没有一并做的事（如实记录，不假装清零）**：另外 23 个含 `flushPromises`
的文件（67 处裸调用）仍是固定次数写法，只是靠串行不再触发。
一次性重写 15 个跨域文件与本任务无关，强行塞进本轮会让已完成并验证的一批改动
失去稳定性。串行开关 + 助手已经把它们从「随机红」变成「确定绿」，
**剩下的路已写在配置注释里，谁想并行谁照着走**。

##### 6.11 复查「所有数据项都能在 UI 配置」时挖出的缺口（2026-10-12）

阶段六做完了参数页与字典页。任务书里那句「**所有数据项在 UI 页面允许用户配置**」这句话，到这一步才算有资格被认真读一遍——于是又挖出四件事，前两件是产品缺口，后两件是既有缺陷。

**（1）`feature_flags` 的五个管理动作有实现、有签名，但没有任何路由能到。**

`internal/controller/feature_flag.go` 实现了 Update(`:68`) / Delete(`:86`) / Enable(`:98`) / Disable(`:111`) / Rollout(`:124`) 五个动作，而 `internal/router/business_routes.go` 的 `flagAdmin` 组**只挂了 `POST ""`（Create）**。后果不是"少几个端点"，而是**功能整体不可用**：能建出开关，却没有一条路能把它启用——`Evaluate` 于是永远走 `not_found` 分支返回 false，一个用不起来的开关比没有开关更坑（运维看到它躺在列表里，以为灰度没生效是策略选错了，实际上压根没启用过）。

已补挂 5 条：`PUT /:id`、`DELETE /:id`、`POST /:id/enable`、`POST /:id/disable`、`POST /:id/rollout`。

**没有**顺手把 `RegisterCodeReference` 也挂上：它全仓零调用方（只有 Controller 与 Service 两处定义），自陈是给工具链调的，挂上去等于开一个没有消费者的写端点。`GET` 侧本来就存在，KV 里目前恒为空，UI 也不展示这个标签。

**（2）唯一的 `ReadOnly` 参数没有写为什么不可改。**

217 条种子里 `ReadOnly: true` 只有一条：`knowledge.embedding_dimension`。UI 侧有独立的「只读」徽标与禁用按钮（不是没有提示），但 Description 只写了「向量嵌入维度（bge-m3 默认 1024）」——用户看到禁用按钮，依然不知道**为什么**。已改写为写清后果：改它必须把所有已入库的向量按新维度重新生成并重建索引；而维度与存量向量不一致时，检索结果全是噪声，**而且错得很安静**（不报错，只是答非所问），所以只能走「换嵌入模型 + 全量重建索引」的运维流程。

Name 没有加「（只读）」后缀：UI 已有独立徽标，`stripSuffix()` 也会把它剥掉，加了是重复。

**（3）只加前端路由不会出现在菜单里——菜单已经改成后端驱动。**

这条值得单拎出来，因为它会**静默失败**：路由注册成功、`/system/feature-flags` 能直接输地址打开，但左侧菜单里没有入口。菜单树在他人 RBAC 提交后搬到了 `user-server/internal/model/menu.go` 的 `var SystemMenuTree`，由 `GET /api/menus` 下发。我自己的 `/system/dictionaries` 其实也中招了（上一轮只加了前端路由）。

已在 `SystemMenuTree` 的 `dynamicThreshold`（动态阈值参数）之后并列加两条。**放在它后面而不是塞进别的分组**，是因为运维调这三页时的心理模型是同一个：「这个系统有哪些可配置的东西」。

**（4）15 个菜单项一直显示着错的图标，而且是静默的。**

两条原因叠在一起：

- **解析路径不一致**：`Layout.vue:25` 写的是 `<component :is="iconComponents[menu.icon]" />`，直接下标、**没有兜底**；而 `SubMenuItem.vue:9/21` 走的是 `resolveRouteIcon(menu.icon)`，兜底 Document。一级菜单没兜底 ⇒ 图标名不认识时渲染出空的组件。
- **`routeIconMap` 登记不全**：未登记的 13 个名字里，`Cloud` / `QrCode` / `Shield` 在 `@element-plus/icons-vue` 里**根本不存在**（只有 `Cloudy` / `Postcard` / `Lock` 之类），另外 10 个有效但没登记。合计 15 个菜单项一直静默显示成 Document。

处置三件：`Layout.vue` 改走 `resolveRouteIcon`（统一两条路径）；`iconMap.js` 补登记 12 个（实测新增的全部是有效图标）；`menu.go` 里三个无效名换成有效等价图标——`livecode: QrCode→Postcard`、`systemObsConfig: Cloud→Box`、`securityAudit: Shield→Lock`。

**（5）顺带修掉我自己刚写的一个"看起来在守、其实在漏"的 helper。**

`menuRegistry-helper.js` 原本用 `NODE_RE = /\{([^{}]*)\}/g` 解析 `menu.go`，**跨不过嵌套花括号**。凡是写成一行且带 `Roles: []string{"admin"}` 的菜单项（含 `dynamicThreshold`、`systemDictionaries`、`systemFeatureFlags` 这三条）全部被静默丢弃：170 个 `Path:` 字面量只解析出 139 个。后果特别恶劣——「菜单 path 都能在前端路由里找到」那条断言，因为**列表本身少了几条**而显示通过。

改成逐行取 `Key/Title/Icon/Path`（都是 `字段: "字面量"` 形态，不受嵌套花括号影响），并加一道自检：**解析出的 path 条数必须等于 `Path:` 字面量个数**，不等直接抛错。这类失败是静默的，只能显式拦一道。

解析器修好后立刻又暴露 2 处「父子同 path」（`/browser-automation/tasks`、`/geo-tools/keyword-mining`）。核实后确认**不是真重复**：分组节点自己也带 `Path` 指向默认子页，点分组名进默认页、点子项进具体页，是有意设计。是我最初的断言没区分分组与叶子。helper 加 `isGroup` 字段并导出 `backendLeafMenuItems()`，判据是「抹掉 `Roles: []string{...}` 后该行是否自带闭合 `}`」——分组节点的 `Children:` 落在下一行、自己这行不闭合。

**新增的 `menuRegistry_routes.test.js`（6 例）**

这道门是加完上面 (3)(4)(5) 之后才写的，而它一上来就把这三件事全抓了出来——包括 (5) 那个我自己刚写出来的漏。六个判据里有两个值得单独说：

- 「菜单里每一条 path 都能在前端路由里找到」这条，**直接 import 各路由模块并递归展开 children**。不用 `router.getRoutes()`：路由是懒加载的，`router.beforeEach` 里 `ensureRouteLoaded → addRoute`，刚 import 时只有 13 条骨架，会把未加载的路由全误判成缺失。也不用 `router.push`：beforeEach 带登录守卫，未登录弹到 `/login`，每个 path 的 matched 都变成登录页。也不用正则扫 path 字面量：`browserAutomation` 的 `path: 'tasks'` 是嵌套相对路由，正则看不到 `/browser-automation/tasks`。
- 「控制台里的配置页都登记进了菜单」只钉 `['/system/config-params','/system/dictionaries','/system/feature-flags']` 这组入口。**不能**写成「所有前端路由都必须在菜单里」——详情页、编辑页本来是从列表点进去的，不该出现在菜单。

**一个必须记下来的教训**：这道门第一次跑是绿的，我顺手把 `systemFeatureFlags` 菜单项拆掉做破坏验证，它**正确报红**了；可把菜单项还原之后，它又红了——暴露的是 (5) 那个 helper bug。**门能通过不代表门在守，必须做一次破坏验证。** 这个习惯此前只在 `check-config-param-readpoints` 上用过，这次证明它对测试同样成立。

**新建 `FeatureFlags.vue` + `api/featureFlags.js`**

页面结构与字典页一致（顶部 el-alert 讲清与阈值参数的分工、工具栏、表格、对话框、日志抽屉）。三个交互决定：

- 开关的停用/启用走 `ElMessageBox.confirm` 二次确认。开关是**运行时立刻生效**的，停用会让用户立刻看不到某个功能，不是那种"改了明天生效"的配置。
- 失败后**回读服务端真实状态**，不让开关停在一个"本地以为改了、实际没改成"的态——那个态比报错更难发现。
- 删除的确认文案区分「删除不可恢复」与「只是想让它不生效请用停用」。这两件事经常被混为一谈。

编辑保存时若 `enabled` 与列表现状不一致，额外调一次 enable/disable：`FlagUpdateRequest` 结构体里**没有** `Enabled` 字段（开关状态只走那两条端点，以便单独留审计记录）。

**本轮验证（2026-10-12）**

| 项 | 结果 |
|---|---|
| `menuRegistry_routes.test.js` + `router_approvalTask` + `router_badCase` | **16 passed (16)**（3 文件全绿；helper 解析器改动影响到的另两个文件一并复跑） |
| 前端全量 `vitest run` | **57 文件 611 例全绿**（较 §6.8 的 56/605 多出 `menuRegistry_routes.test.js` 的 6 例），`fileParallelism: false` 串行 |
| `vite build` | 通过（PWA precache 574 entries） |
| `eslint` 12 个新增/改动文件 | **0 errors**（559 warnings 全是 `vue/max-attributes-per-line` 一类格式告警，既有的 `LTCConfig.vue`/`ObsConfig.vue` 同样 100+，warning 不是本仓门槛） |
| `go build ./...` / `go vet ./...` / `gofmt -l internal router` | 全过（`BUILD_RC=0` 未接管道可信；`gofmt -l` 无论有无输出都返回 0，只能看输出文字） |
| `go test ./internal/router/ -count=1` | **ok 30.848s**（`flagAdmin` 补挂 5 条之后；gin 的 `:id` 与静态段 `stale` 同级不冲突） |
| `go test ./internal/service/ -run 'TestSeedDefaultsPassTheirOwnValidation\|TestDefaultParamDefs'` | **ok 1.124s**（改 `embedding_dimension` 的 Description 之后） |
| 五道门 + 契约门 + CI paths 门 | 全 rc=0；readpoints 读数 `wired 206 / 已声明未接线 0 / UNDECLARED 0 / STALE 0 / 已下架 11（接上了却没摘下架标记 0）` |
| 文档门 | `check-md-links-offline.py` 断链 0（扫描 201 个 md）；`check-doc-consistency.sh` 27 个关键文件全通过 |

前端全量这次是**与另一个全量 vitest 会话并发**跑的（即机器有明显 CPU 争抢），仍然 611/611 全绿——这是 §6.10 那个串行开关的实证，不是空闲下的侥幸绿。

#### R0 已落地（2026-10-20）：纯减配零行为变更——死配置删除 + 部署种子行 + 文档对齐

调研与批次设计见 `docs/CONFIG_INVENTORY_20261020.md`（2026-10-20 双栈+部署层全量清单，判定框架=来源层 × 生效时机双轴）。

1. **user 仓删死键 `JWT_EXPIRE`**：删除前取证——Go 读取点 0；`website/src` 命中 2 处均为 DocsPage.vue 静态示例文案；env-coverage 基线 0 命中（⇒ 不触发 STALE）。删 `.env-example`、根 `.env`、`RELEASE_PREPARATION.md` 2 处提及、DocsPage.vue 2 处文案。**一行 Go 未动**：有效期仍硬编码 24h。
2. **platform 仓删两处死配置**（`config_viper.go` 净 −38 行）：a) `smtp.*` 段（结构体/字段/6 条 SetDefault，消费方 0，yaml 本就无此段）；b) `WatchConfig()`（调用方 0，连带 fsnotify/logger import，`go mod tidy` 后 fsnotify 转 indirect）。热重载从未接线，配置变更仍需重启，与删除前行为一致。
3. **deploy**：`env_kv_block` platform 分支新增 `PLATFORM_PUBLIC_URL=$PLATFORM_API_URL`（deploy.conf:28 派生；服务器 `.env:28` 已手工存在同值，本地幂等模拟 diff 为空）；README 密钥清单补 `MASTER_KEY`（5→6）并同步 §4 幂等 sed 字段清单。
4. **顺带修复预存在门禁红**：`check-env-coverage.py` 的 `UNDOCUMENTED RERANK_PROTOCOL`（读取点由 `67c2f18f` 引入但未文档化）——`.env-example` 补 `RERANK_PROTOCOL=` 与协议分支注释。

验证：user 三道参数门 rc=0（211 wired / UNDECLARED 0 / STALE 0）+ doc-consistency / md-links rc=0；platform md-links / no-xapptool / build / vet / gofmt 全 rc=0；deploy `bash -n` ×2；`PLATFORM_PUBLIC_URL` 幂等模拟无差异。明确没做：platform 仓自带的另一份死 `JWT_EXPIRE`（`.env:9`/`.env.example:36`/`发布流程.md:75`，消费方 0）留待后续；未提交未部署由管理者统一执行。

#### 平台端 R1 已落地（2026-10-20）：platform-server 参数中心 + 7 条种子 + 管理页

移植自 user-server 生产实现（seed 语义只补缺/只刷定义列/永不碰 value、60s TTL 缓存+负缓存+写失效、审计 AES 加密、三道门禁），适配平台端五层结构与既有 admin JWT 组。

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `heartbeat.offline_ttl_minutes` | 5 | `platform-server/internal/service/async_process.go:70`（读点挪进每分钟扫描闭包） | 热 · ≤1min + 缓存 60s |
| `ratelimit.login_rps` / `login_burst` | 5 / 10 | `internal/middleware/ratelimit.go:180-181`（`NewLoginRateLimiter`） | 构造期 · Restart=true |
| `ratelimit.global_rps` / `global_burst` | 20 / 200 | `internal/middleware/ratelimit.go:198-199`（`NewGlobalRateLimiter`，装配点 `router.go:36`） | 构造期 · Restart=true |
| `upload.max_size_mb` | 10 | `internal/controller/upload_controller.go:57`（env `UPLOAD_MAX_SIZE` 保留最高优先级应急） | 热 · 每次上传请求 |
| `middleware.max_json_body_mb` | 8 | `internal/middleware/body_limit.go:48`（env 含 0=关闭的应急语义） | 装配期 · Restart=true |

**两条判断**：① 建表走 AutoMigrate（`db.go` `migrateModels`，`config_params`+`config_param_audit_logs`，唯一键 `(param_group,key)` 由 uniqueIndex 生成）⇒ 不新增 SQL/配置文件，不触发 deploy.sh rsync 与 Dockerfile COPY 两处同步；`config_params` 是逐行独立旋钮的 KV 参数中心，非「部署级单行表」反模式。② `config.HeartbeatOfflineTTLMinutes=5` 提成常量，viper 缺省/`<=0` 保护/种子 DefaultValue 三处同源，defaults 门 7/7 可比对。

**契约**：`GET /platform/config-params(?group=)` + `PUT /platform/config-params/:group/:key`（400+可读 message；`data.items`/`data` 返回形）。**前端**（platform-web，静态路由菜单「系统 › 系统参数」`/platform/config-params`）：按组 Tabs+计数角标、生效时机徽标（立即/需重启）、按 valueType 渲染编辑控件+min/max/JSON 校验、保存后回读服务端真实状态、readOnly 禁用；dev 默认契约 Mock（`VITE_CONFIG_PARAMS_MOCK=false` 切真实），生产构建恒走真实接口。

验证：三道移植门 rc=0（读取点 7 wired、默认值 7 可比对 0 mismatch），**破坏验证**（临时移除 `upload.max_size_mb` 读点 → readpoints rc=1 UNDECLARED + defaults rc=2 棘轮失守 → 还原双绿）；`go test ./... -p 1` 除既有 `TestMonitoringService_GetBusinessMetrics`（seed 未来铺 vs 期望过去铺，只在每日 00:00–00:59 窗口红，与本批无关）外全绿；新增 11 个用例（seed 幂等/缓存 TTL/GET·PUT 契约/审计加密/路由挂载+无令牌 401）；platform-web `vite build` 通过（ConfigParams chunk 15.38 kB）。未做：Swagger 再生成、门禁挂 CI、AuditLogs/ResetToDefault 端点（仓储层已具备，契约未含）。

#### R2 用户端第一批已落地（2026-10-20）：ratelimit(6) + 免打扰(4) + urls(3) 共 13 条 + SMTP 路径 A

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `ratelimit.global_rps` / `global_bucket` | **1000 / 20000** | `router/router.go:215` | 装配期 · Restart=true |
| `ratelimit.visitor_per_ip_rps` / `_bucket` | 20 / 40 | `router/chat_routes.go:26` | 装配期 · Restart=true |
| `ratelimit.visitor_per_channel_rps` / `_bucket` | 100 / 200 | 同上 | 装配期 · Restart=true |
| `reach.quiet_hours_start` / `_end` | 22 / 8 | `service/reach_send_pipeline_quiethours.go` 读口 | 每请求（60s TTL） |
| `webhook.ai_quiet_hours_start` / `_end` | 23 / 7 | `service/webhook_outbound.go` 读口 | 每请求 |
| `urls.frontend_url` | `http://localhost:3000` | `service/password_reset.go` `getFrontendBaseURL` | 每请求 |
| `urls.cloud_url` | `https://work.xapptool.cn` | `config/platform.go` provider seam（`app/url_params_wiring.go` 注入） | 每请求 |
| `urls.geo_site_base_url` | `https://xiaofang142.github.io/hivemtk` | `config/website.go` provider seam | 每请求 |

**三条判断**：① 全局限流默认取**生产值 1000/20000**（`DefaultRateLimitConfig` 的 10/100 仅测试兜底，生产被 `router.go:215` 显式覆盖）——守「默认值=当前生产行为」，避免参数化把线上限流收紧 100 倍；② 免打扰两管道（触达 22-8 / AI 回复 23-7）**分键不合并**，0-23 越界回落代码默认，`nextDayFirstSend` 恒等于 end；③ config 不能反向 import service，两条 config 侧 URL 走 provider seam。

**SMTP 路径 A（DB 化零新增表）**：勘察确认 `/email/smtp` 页面 → `email_smtp` 专表已持久化（密码 AES-GCM 落库、DTO 脱敏）；发信侧 `email/service/email_send.go` 默认通道补 DB 臂：**显式 SmtpID > env > `resolveSmtpFromDB`（首条完整行，crypto.Decrypt）> 报错**——纯增量，只填原先「无 SmtpID 且无 env 即报错」的空档，env 存量部署行为不变，运营页面配好即驱动发送、免重启。

验证：readpoints wired 211→224（UNDECLARED/STALE 0）、defaults comparable 186→197、keystyle 37 组 0 违规，三门 rc=0；gofmt/vet/build clean；`go test -p 1` 五包全 ok（service 668s，含免打扰解析/URL 三级优先/限流默认=生产值/SMTP 优先级新用例）。env 一个未删（`DISABLE_AI_QUIET_HOURS`/`FRONTEND_URL`/`PLATFORM_CLOUD_URL`/`GEO_SITE_BASE_URL` 全保留最高优先级应急）。明确未做：路径级配额 map、`FF_*`、`days=30` 报表窗口、备用邮箱通道。

#### 平台端 R3 已落地（2026-10-20）：资金类两条——分账比例写时戳 + 续费顺延天数快照优先

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `asset_market.platform_share_percent` | 30（int 0-100） | `platform-server/internal/service/asset_market_service.go` `resolveShares`/`CreateAsset`（调用点 :393） | 热 · 每次创建资产（60s 缓存） |
| `subscription.period_days` | 30（int 1-365） | 下单快照 `order_service.go:321` `buildItems`；履约兜底 `subscription_service.go:173` `ExtendByOrder` | 热 · 每次下单/履约兜底 |

**语义（种子 Description 与实现双向钉死）**：
1. **分账「写时戳」**：参数只作为**新建资产**未显式指定时的默认值——整数百分比÷100 落 `platform_share`（decimal(4,2)），`contributor_share` 同步写 `100%−本参数`（两列是同一笔分账的两半，只戳一半会造出 40%+70% 的坏行）；显式指定（service 层入口 `CreateAssetInput.PlatformShare`，HTTP 面 `json:"-"` 防贡献者自定比例）优先；**存量资产行永不被改写、无回写 job**；购买分账仍按资产行快照（`platformShareAmount = a.Price × a.PlatformShare`）。既有行默认 0.30 不受新默认影响。
2. **续费天数快照优先**：下单时 `period_days` 读参数并**快照进 `order_items.snapshot`**（锁值，改参数不追溯已下单）；履约 `fulfillSubscription` 优先快照；仅快照缺失/≤0（历史脏数据）才回落「参数 > 常量 30」。**修掉的既有缺陷**：`ExtendByOrder` 原 `days<=0 → 常量30` 的实时回退（改常量会按新值延旧单）。
3. **幂等双口径无冲突**：平台 grant 幂等锚天数=`coalesce(快照, 30)`，用户端镜像锚=下单时参数值——两者在「参数=默认 30」时相等，参数改后各按自己的时点快照，互不破坏。

**对账证据（发布后跑）**：`assets` 中比例≠0.30/0.70 的行数=0（写时戳不回写存量）；`revenue_records` 近 7 日 `platform_amount/amount` 比例逐行=资产行比例（±0.01）；`subscription_grants.days` = 订单快照 `period_days`。

验证：三道门禁 rc=0（seed 7→9、wired 9、可比对 9、命名违规 0）；`money_params_test.go` 3 例（写时戳+双列补数+显式优先+越界钳位+整数换算、**改参数后购买分账仍按行快照**、**快照单不受参数影响+无快照回落参数+默认路径延 30 天**）；主仓环境 `go test ./internal/{service,shared/service,controller}/ -p 1` 全 ok（含 money 与既有 DB 依赖用例，`POSTGRES_TEST_PORT=8201` + `.env` 密钥注入）；platform-web 构建通过（ConfigParams chunk 更新）。前端 `GROUP_LABELS` 增 `subscription: 订阅`（`asset_market` 原本已有）。实现会话自述的「8 个预存在红」经核实为该会话自身 PG 凭据环境问题，非代码回归——主仓同用例全绿。

#### 清欠批次 F1–F5（2026-10-20）：backlog 一次性清零

**F1（平台端四件套）**：① 删死键 JWT_EXPIRE（platform `.env.example`+`发布流程.md`，取证 0 消费方）；② 修 `TestMonitoringService_GetBusinessMetrics` 时间窗 flaky——根因 seed 未来铺 vs 期望过去铺公式相反（每日 00:00–00:59 必红），修法=seed 统一过去铺 + 期望函数接播种基准 logBase + 固定时刻纯函数用例覆盖跨午夜窗口（00:30→268、00:00→9）；③ 审计/回默认闭环：`GET /platform/config-params/:group/:key/audit?limit=`（解密明文 `{items:[{time,actor,old,new}]}`）+ `POST .../reset`（action="reset" 审计、只读拦、写后缓存失效），管理页行操作加「回默认」确认框与「变更历史」抽屉，新增单测 5 个（路由 401×4、解密明文、reset 落审计+双层只读拒）；④ 三道门挂 `platform-ci.yml` + 仓根/`platform-server` 双 Makefile（`config-param-gates` target）；⑤ `swag init -g cmd/api/main.go -o docs/swagger` 重生成（swag v1.16.4，头未变，含 audit/reset 与 49761d7/794cd76 租户注解欠账）。验证：三门 rc=0（9 wired/9 可比对/0 违规）、gofmt/vet/build 绿、`go test ./... -p 1` 全绿无时间窗红、npm build 绿、CI YAML valid。

**F2（user-web 测试基建收尾）**：27 文件——23 个组件用例文件 **82 处裸 flushPromises 全改 settle(wrapper)/settleBody()**（含 15 处 for 连刷、2 处自定义 flush+setTimeout 循环；断言零改动只改「怎么等」；teleport 弹层以 document.body 为判据）；`settle-helper.js` 捕获真实 setTimeout（假时钟不挂死）+ 新增 `settleBody()`；`router_query_preserve` 补 bootstrapState 桩切断「本机后端可达性」隐性依赖（该红与并行无关，单跑即红）；**`vitest.config.js` fileParallelism 恢复 true**。验证：grep 裸调用 82→0；vitest 三轮全量 **58 文件/632 例全绿**（默认并行 ×2 + threads/50% ×1，27.6–29.8s）；build 绿；改动文件 eslint 0 error。**串行豁免到期，测试基建收口完成。**

**F3（结构化配置三件 + 基线）**：① `ratelimit.path_quotas`（json，每请求热读）——勘察确认配额 map 是请求期查表非构造期；解析失败回落默认表、单条非法丢弃、空表回落默认；种子默认与 `DefaultPathQuota` 同源对账；② `reporting.default_window_days`（int，默认 30，热读）——经 `internal/reporting` 叶子包 + 装配层 provider 注入收口，接入全仓 ~24 处报表 days=30 兜底（`DailyTrend`/`sinceDays` 两签名补 ctx；geo 可见性复用已有 `geo.default_visibility_days` 读口避免双键打架）；③ **备用邮箱通道 DB 化走专表而非参数**：`email_smtp` 表行 `Name='env:163'/'env:qq'` 按 env 链同序顶替对应 env 通道——依据=专表已有 UI+AES-GCM+`json:"-"` 天然不回传，参数中心通用 GET 会明文回给管理端（密码类不宜），env 优先级不变、无此名行时选径逐字节不变；④ defaults 基线 186→**198**（棘轮验证仍挡下降）。门禁：readpoints wired **226**、defaults/keystyle/env-coverage rc=0。测试：middleware/app/reporting/controller/geo/ops 等包全 ok（knowledge 2 例需本机 8208 embedding 服务，环境性非本批）。

**F4（跨仓 drift 门转绿）**：`MERCHANT_API_SECRET` 两端不一致根因=R1 `hivemtk/.env` 是模板占位符 `ChangeMe_To_…`（48 位）、R2 平台 `.env` 是 `openssl rand -hex 32` 生产同构 64 位真值；对齐 R1 至 R2（仅本地 gitignored .env）。复跑：drift 门 **rc=0（PASS=3 FAIL=0）**、check-secrets rc=0。附带裁决：门内记载的 `azbL6ptz…` 是 merchant_key（部署级身份文件）而非 HMAC secret，不参与本门。

**F5（路径配额真功能补全并挂载）**：修复两个真缺陷——(a) 配额表 RPS/BucketSize 从未生效（原实现走 `globalRateLimiter.Allow` 全局桶，参数只回显在 429 JSON）；(b) 桶键只有 path 没有 IP（`/api/reach` 50rps 全站共享，一租户可饿死所有人）。修法=自建 `pathQuotaLimiterManager`：桶键 `ip+"\x00"+path` 每客户端每路径一桶、5min tick 清 30min 闲置、参数变更重建桶（60s 缓存 TTL 内生效）；挂载 `router.go` 全局限流后/JWT 前（未登录也计数，保护昂贵端点的本意）；`GetQuotaSnapshots` Remaining/Used 改从新管理器（Redis 分支保留）；种子 Description 同步「每客户端×路径独立计数」。新 6 用例（429 body 逐字段、跨 IP 隔离、热更新重建、快照自洽、nil 限流器）+ 既有种子/解析用例全 PASS；三门 rc=0（226 wired）。**已知边界**：多实例部署为各实例本地计数（代码注释已标），跨实例一致配额需 Redis 全局桶——未做，属部署拓扑决策。

**F-顺手修（Wall.vue）**：`user-web/src/views/subscription/Wall.vue` `recover()` 被调用但从未定义——0 元续费直通 fulfilled 成功路径会抛 ReferenceError（提示弹出但状态卡旧值）。补 `recover()`：立即一次「sync+状态回读」，未转 active 则落回既有轮询。eslint 0 error。

**F 批次验证汇总**：平台三门 rc=0 + 全量测试绿 + swag/CI/YAML 就绪；用户端四门 rc=0（wired 226、可比对 198/基线 198）+ middleware/app/reporting/email/config 等包 `-p 1` 全绿；user-web vitest 三轮 632 例全绿（并行）+ build 绿；drift/secrets 门 rc=0。

#### 平台端 R4 已落地（2026-10-20）：可运维上限与超时三条

| group.key | 默认 | 代码点位 | 生效时机 |
| --- | --- | --- | --- |
| `asset_market.max_withdrawal_amount` | 1000000（int 1-100000000，元） | `platform-server/internal/controller/asset_market_controller.go` `ContributorWithdrawalCreate` | 热 · 每次提现请求（60s 缓存） |
| `asset_market.batch_operation_limit` | 100（int 1-10000，条） | 同上 `AdminBatchPublish` / `AdminBatchApprove`（共用同一上限） | 热 · 每次批量请求 |
| `llm_proxy.timeout_seconds` | 120（int 1-1800，秒） | `platform-server/internal/service/llm_proxy_service.go` `NewLLMProxyService` | 构造期 · Restart=true |

**口径**：三条默认值都等于改动前的代码常量（1_000_000 / 100 / 120s），行为零变更；`GetInt` 的兜底实参分别是 `defaultMaxWithdrawalAmount` / `defaultBatchOperationLimit` / `DefaultProxyTimeoutSeconds`，与种子 DefaultValue 同源，defaults 门按实参位置比对。批量错误文案改为回显参数值（`上限 %d 个`），Swagger 注解同步标注「默认值，参数中心可调」；前端 `GROUP_LABELS` 增 `llm_proxy: LLM 代理`。

**明确不 DB 化（同批讨论结论）**：
- `middleware.maxUploadBodyMB`（64MB 内存调优）、LLM 代理 `maxBodyBytes`（8MB 安全上限）、审计队列 `batchSize/flushInterval`：资源/安全/内部队列参数，运行期改动风险大于运维收益；
- `log.*`（level/format/filename/rotate）：启动引导配置，必须在 DB 可用前生效。本轮只修「`main.go` 硬编码导致 `config.Cfg.Log.*` 成为死配置」——`logger.Init` 改读 `config.Cfg.Log`（viper 默认值与原硬编码逐项相同，行为零变更），`retention_days` 仍由 config.yaml 唯一驱动；
- `jwt.expire`：安全姿态；`PLAYGROUND_LLM_BASE_URL`：部署端点（env）；`FF_*` 已在 `feature_flags`。

**配置类缺陷同批修复**：
1. 用户端 `config.GetServerBaseURL` 直接 `yaml.Unmarshal`、未展开 `${VAR:default}`，跟踪/退订链接会把字面 `${PLATFORM_API_URL:...}` 拼出去；改为先 `expandEnvWithDefault`，`TestGetServerBaseURLExpandsEnvPlaceholder` 钉住（反向还原缺陷时该用例红在字面占位符）。
2. 平台端 `cmd/api/main.go` 的 `logger.Init` 由硬编码改为读 `config.Cfg.Log`。
3. 删除 `internal/config/config_viper.go` 零调用死代码 `GetViper` / `WriteConfig` / `Init`（viper 热重载从未接线）。

验证：三道门禁 rc=0（seed 9→12、wired 12、可比对 12、命名违规 0，棘轮基线 9→12）；反向摘掉提现读点 → readpoints rc=1 `UNDECLARED asset_market.max_withdrawal_amount`、defaults rc=2 棘轮 11<12，复原后双绿（md5 回读一致）；`go build ./...` / `go vet ./...` clean；`go test ./internal/controller/ ./internal/config/ ./internal/service/ -p 1` 全 ok（8.2s / 0.3s / 33.5s），新增 `TestAssetMarketCapsFollowConfigParam`（DB 上限调到 100/1 后，150 元提现与 2 条批量均被拒，证明不是只改静态形状）与 `TestLLMProxyTimeoutFollowsConfigParam`（DB 30s 同时进入 `service.timeout` 与 `http.Client.Timeout`，无 DB 回落 120s）；platform-web `vite build` 通过（ConfigParams chunk 19.02 kB）。Swagger `docs.go` 未再生成（该文件正被并行泳道修改，注解已更新，下次 `swag init` 即同步）。
