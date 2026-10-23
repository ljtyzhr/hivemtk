# Implementation Plan: Wave 2 止血切片

**Feature**: `006-wave2-gates-and-selector-health` ｜ **Created**: 2026-10-24 ｜ **Spec**: `./spec.md`

## 交付物总览

| 项 | 新增 | 修改 | 牙齿 |
|---|---|---|---|
| A2-a | `scripts/check-dependency-policy.py`、`scripts/dependency-policy.baseline` | `Makefile`、`user-server-ci.yml` | `--self-check` 10 枚 |
| A2-b | `scripts/check-sbom-artifacts.py` | `Makefile`、`user-server-ci.yml`、`sbom.yml` | `--self-check` 16 枚 |
| A2-c | `internal/service/whatsapp_provider.go` | `internal/service/whatsapp.go`（重写为纯委派） | `go build` + 既有用例 |
| A4-a | `scripts/check-core-coverage.py`、`scripts/core-coverage.baseline` | `Makefile`、`user-server-ci.yml` | `--self-check` 8 枚 |
| B1-a | `web/bridge/src/core/selector-health.js`、`web/bridge/test/selector-health.test.js`、`model/bridge_selector_health.go`、`repository/bridge_selector_health.go`、`service/selector_health.go`、`service/selector_health_test.go`、`repository/bridge_selector_health_test.go`、`controller/bridge_selector_health.go` | `channel-adapter.js`、`background/index.js`、`router.go`、`migrate.go`、`config_param_seeds.go`、`hardcode-tunable.baseline` | 前端 20 例 + 后端 service/repo 用例 |

## A2-a 依赖策略门

### 判据面（只守三类，且刻意**不**重复 CVE 面）

1. **SEAM_LEAK**：弃维护库的实际引用面（去掉 `*_test.go`）必须落在基线声明的 seam 里。
   基线格式 `module | seam | review | why`，`seam` 支持空格分隔多路径。
2. **REVIEW_OVERDUE / REVIEW_BAD**：每条登记必须有到期复查日（`YYYY-MM-DD`），过期即红。
   这条治的是「登记一次就永远躺在那」——弃维护库的**处置决定**是有时效的。
3. **BAD_DECL**：`go.mod`/`package.json` 里的 `latest` / `*` / URL 版本 / 相对路径 `replace`。
   版本不可复现的依赖在第一次 `go mod tidy` 就会漂移。
4. **BANNED**：禁入名单（无 CVE 也禁）。

### 关键设计决策

- **基线是手工合约，不给 `--refresh-baseline`**：能自动刷新的基线等于「引用面变宽时
  跑一下命令就绿了」。seam 属于架构决策，必须人改。
- `*_test.go` **不计**引用面：测试文件里 import 一个弃维护库不会让它进生产二进制。
- `lib/pq` 放 `OUT_OF_SCOPE` 并**每次打印原因**：`postgres` driver 需要空白导入注册，
  它不在可替换面内。打印而不是静默跳过，是为了让「本门看不见什么」每次运行都可见。

## A2-b SBOM 产物断言

- 两段分离：`--workflow` 只需 PyYAML（静态 gates 跑），`--dir` 需要 syft（`sbom.yml` 里跑）。
- 判据 A–J（工作流段）+ K–O（产物段）见 spec。
- **只对账直接依赖**：`go.mod` 不带 `// indirect` 的行、`package.json` 的 `dependencies`。
  devDependencies 与传递依赖只打印数额、不判定——syft 各 cataloger 对这两类的收录口径
  不一致（`node_modules` 在场与否结果不同），写成硬判据会得到一道「本机能过、CI 上红」的门。
- 名称匹配走 purl 优先、`name` 兜底，两侧都归一化；**只做精确匹配**，不做模糊/后缀匹配
  （模糊匹配会把「少了 github.com/x/y」变成「因为有 example.com/y 所以放行」）。

## A2-c WhatsApp 接口化

- `whatsapp_provider.go` 是 `gowa` 的唯一 seam，含三种实现：
  `cloudWhatsAppProvider`（Cloud API）、`legacyWhatsAppProvider`（gowa）、
  `unavailableWhatsAppProvider`（未配置时的显式失败）。
- `whatsAppProviderFor` 保留**旧的判据**：`Type=="cloud" && PhoneID!="" && Token!=""`。
  为什么保留而非"更合理"的判据：历史数据里存在 `Type=="cloud"` 但凭据为空的账号，
  按类型直接走 Cloud 会让它们从「能发」变成「500」。
- 登录面单独拆 `WhatsAppLoginProvider`：只有个人版扫码登录需要，Cloud API 没有这个概念。
- 个人版不支持模板消息 ⇒ `ErrWhatsAppProviderUnsupported`，**不退化成文本**。

## A4-a 核心域分档覆盖率

- `CORE_PREFIXES` 是显式清单（8 条完整包路径前缀），不是关键词猜。
- 基线每条 `包路径 | 最低覆盖率`；`--refresh` 只上调（算出更低时保持原值并打印拒绝），
  真下调要 `--allow-drop <包名>`。
- `NOT_IN_PROFILE` 与 `0%` 严格区分：前者是「没测到」，后者是「测了但没覆盖」。
- **基线必须用真实 profile 生成**：用不完整 profile 刷出来的底线会把「缺包」
  永久写进豁免。

## B1-a 桥接选择器失效检测

### 分工（最重要的一条）

扩展侧只上报 `attempts` / `hits`；判定与阈值全在服务端。理由：
扩展不知道「平台改版」与「今天没客户」的区别；让它判阈值会在各版本客户端出现
N 套互相矛盾的规则，且改一次阈值要发一次扩展版本。

### 前端

- `src/core/selector-health.js`：`createSelectorHealthTracker`（无 IO 的纯计数器）、
  `mergeSelectorHealthItems`（多标签页合并）、`subtractSelectorHealthItems`
  （**减**而不是清空已发批次）、`buildSelectorHealthRequest`（纯函数）、
  `postSelectorHealth`（不抛，观测面失败不许带崩业务）。
- `channel-adapter.js`：把探针放在 `getMessageItems` / `getConversationList` 两个
  **胶水方法**上。为什么不是逐渠道 hooks 埋点：5 个渠道实现各不相同（虚拟列表/iframe/
  React fiber），逐渠道 = 5 份口径且新增渠道必然漏；而所有渠道都经过这两个方法。
- `background/index.js`：`reportSelectorHealth` 消息只落 storage，`chrome.alarms`
  1 分钟 flush 一次；成功后按已发批次扣减。
  **为什么用 alarms 而不是 setInterval**：MV3 service worker 会被浏览器挂起，
  setInterval 不可靠。

### 后端

- `model.BridgeSelectorHealth`：`(channel, field)` 唯一，`attempts`/`hits` 单调累计，
  `first_seen_at` / `last_report_at`。累计而非快照：扩展会离线/被休眠，快照会漏掉
  整段离线时段；需要「近期」口径时按 `last_report_at` 判新鲜度。
- `repository`：先 update 后 create（并发首建撞 23505 时补加），不写
  `ON CONFLICT (tenant_id, channel, field)`——那条唯一索引由 `tenancyidx.Normalize`
  在迁移期重建，列序与名字是运维事实，不能假设。
- `service`：通道形状校验、字段白名单、`hits > attempts` 拒收（不静默夹平）、
  增量**等比夹取**（夹不是丢：丢掉分母会让命中率偏高，正好掩盖问题）。
- `controller`：`POST /api/bridge/selector-health` 挂桥接组（实例级凭证），
  `GET` 挂 auth 组（登录态）。分开不是笔误：扩展凭证不该读得到全渠道运营读数。
- 四个 Go 常量（`SelectorHealthMinAttempts` / `MaxItemsPerReport` / `MaxDeltaPerReport` /
  `BridgeSelectorHealthListLimit`）**刻意不入参数中心**，写进
  `hardcode-tunable.baseline` 并附理由：两端协议形状与请求边界，单边可调只会让
  扩展与服务端互相打架。

## 接线（门要真的会跑）

| 位置 | 内容 |
|---|---|
| `Makefile`（`audit:` 末尾） | A2-a、A2-b `--workflow`、A4-a（无 profile 时**大声 SKIP**、不阻断） |
| `user-server-ci.yml` static gates | dependency-policy（含 `--self-check`）、sbom-artifacts（`--self-check` + `--workflow`） |
| `user-server-ci.yml` coverage 作业 | core-coverage `--self-check` + 实际判定 |
| `user-server-ci.yml` push/PR paths | 5 个脚本 + 2 个基线共 7 条（判据文件必须进触发面） |
| `sbom.yml` | 新增 `pull_request.paths`（依赖清单与本门脚本）+ `--dir .` 断言步 |
