# Tasks: Wave 2 止血切片

**Feature**: `006-wave2-gates-and-selector-health` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 · A2-a 依赖策略门

- [x] T001 `scripts/check-dependency-policy.py`：读 `go.mod` / 全部前端 `package.json`，
      扫弃维护库引用面（排除 `*_test.go`）
- [x] T002 基线 `scripts/dependency-policy.baseline`：`module | seam | review | why`，
      seam 支持空格分隔多路径；手工合约（**无** `--refresh-baseline`）
- [x] T003 四条判据：`SEAM_LEAK` / `REVIEW_OVERDUE`（含 `REVIEW_BAD` 日期形状）/
      `BAD_DECL`（latest / * / URL / 相对 replace）/ `BANNED`
- [x] T004 `OUT_OF_SCOPE`：`lib/pq` 等登记为门外但**每次打印原因**
- [x] T005 `--self-check` 10 枚控制（每类判据至少一正一负 + 极性对）
- [x] T006 真牙验证：把 `gomail` 的 seam 收窄成单文件 ⇒ `SEAM_LEAK` 报 3 处泄漏 rc=1，
      验证后还原
- [x] T007 真实仓库实测：37 go 直接依赖 / 63 前端依赖 / 禁入 8 条 / 观察 3 条，rc=0

## Phase 2 · A2-b SBOM 产物断言

- [x] T008 `scripts/check-sbom-artifacts.py`：工作流段判据 A–J（`EXPECTED` 列死三份产物）
- [x] T009 产物段判据 K–O（SPDX 结构 / `packages` 非空 / `SPDXID` 唯一 /
      **直接依赖全覆盖** / 体积下界）
- [x] T010 `--self-check` 16 枚控制
- [x] T011 `sbom.yml` 加 `pull_request.paths`（依赖清单 + 本门脚本）与
      `--dir .` 断言步（在 Generate 之后、upload 之前）
- [x] T012 真实 `sbom.yml` 实测：3 扫描站点 / 3 产物 / 1 upload，rc=0
- [x] T013 `--dir` 段在**真实 syft 产物**上跑过 ✅（2026-10-24 第二轮收口：
      syft v1.55.0 本地源码构建——GitHub Release / Docker Hub / 镜像下载全失败，
      改走 `go install` + goproxy.cn；干净提交树复刻 sbom.yml 三条 scan 生成产物后
      `--dir .` rc=0，直接依赖全覆盖；无 node_modules 场景（与 CI 同形）实测通过）

## Phase 3 · A2-c WhatsApp 接口化

- [x] T014 `internal/service/whatsapp_provider.go`：`WhatsAppProvider`（Kind/SendText/SendTemplate）
      + 独立 `WhatsAppLoginProvider`；三个实现（cloud / legacy / unavailable）
- [x] T015 `whatsAppProviderFor` 保留旧判据（`Type=="cloud" && PhoneID!="" && Token!=""`）——
      历史数据有 `cloud` 但凭据为空的账号，按类型直走会把「能发」变「500」
- [x] T016 `normalizeWhatsAppPhone` / `buildWhatsAppTemplateComponents` 从 `whatsapp.go` 迁入 seam
- [x] T017 `whatsapp.go` 重写为纯委派（`providerFor` / `loginProvider`），不再直接 import `gowa`
- [x] T018 个人版不支持模板 ⇒ `ErrWhatsAppProviderUnsupported`（不退化成文本）
- [x] T019 `go build ./...` 绿；`gowa` 引用面收敛到单文件（由 A2-a 门持续守）

## Phase 4 · A4-a 核心域分档覆盖率

- [x] T020 `scripts/check-core-coverage.py`：`CORE_PREFIXES` 8 条显式清单 +
      `parse_profile`（按语句块去重、取 max count，不重复计分母）
- [x] T021 判据：`BELOW_FLOOR` / `UNREGISTERED` / `NOT_IN_PROFILE` / `STALE`，
      profile 缺失 ⇒ rc=2
- [x] T022 棘轮：`--refresh` 只上调（算低时保持原值并打印拒绝）；`--allow-drop <包名>` 才下调
- [x] T023 `--self-check` 8 枚控制（含「等于底线不红」的极性对与 profile 去重口径）
- [x] T024 `scripts/core-coverage.baseline`：用**真实 profile**（核心域 8 个子树分批跑后合并，
      等价性依据：CI 不给 `-coverpkg`）生成 44 条底线
- [x] T025 默认档实测绿：44 个核心包无一低于底线，核心域合计 50.7%（48191/95062）
- [x] T025b 真牙：`internal/service` 底线 59→60 ⇒ `BELOW_FLOOR` rc=1（已还原）

## Phase 5 · B1-a 桥接选择器失效检测

- [x] T026 前端 `src/core/selector-health.js`：classify / tracker / merge / subtract /
      buildRequest / post
- [x] T027 `channel-adapter.js` 在 `getMessageItems` / `getConversationList` 埋点 +
      `_recordSelectorHealth`（够量经 `chrome.runtime.sendMessage` 推 background）
- [x] T028 `background/index.js`：`reportSelectorHealth` / `flushSelectorHealth` 消息、
      `KEY_SELECTOR_HEALTH` 持久化、`chrome.alarms` 1 分钟 flush、成功后按已发批次扣减
- [x] T029 前端用例 `test/selector-health.test.js` 20 例；bridge 全量 72 文件 / 878 测试全过
- [x] T030 后端 `model/bridge_selector_health.go`（`(channel,field)` 唯一、累计计数、
      新鲜度两列）
- [x] T031 后端 `repository/bridge_selector_health.go`（先 update 后 create，
      23505 并发首建补加；`List` 有上限）
- [x] T032 后端 `service/selector_health.go`（通道形状 / 字段白名单 / `hits>attempts` 拒收 /
      等比夹取 / `ClassifySelectorHealth` 顺序）
- [x] T033 后端 `controller/bridge_selector_health.go`：POST 挂桥接组、GET 挂 auth 组
- [x] T034 路由 `bridgeWS.POST("/bridge/selector-health")` +
      `auth.GET("/bridge/selector-health")`
- [x] T035 `migrate.go` 注册 `&model.BridgeSelectorHealth{}`
- [x] T036 种子 `bridge/selector_health_degraded_below`（DefaultValue "60"、Min 1 / Max 100、
      `Restart:false` 热读）
- [x] T037 四个 Go 常量写进 `hardcode-tunable.baseline` 并附「为什么不入库」的理由
- [x] T044 遗留收口（2026-10-24 第二轮）：A2-b `--dir` 真实产物实跑 rc=0
      （syft v1.55.0 本地源码构建，干净提交树 + 无 node_modules 的 CI 同形场景；
      实跑暴露并修正 `manifest_deps` 判据混淆：清单缺失=红、零依赖=说明后跳过，
      `--self-check` 16 枚复跑全过）；
      `hardcode-tunable.baseline` 补登 geo 两常量（口径/契约护栏，附理由）⇒
      `check-hardcode-tunable` rc=0；完整全仓 profile（CI 同款命令、单条跑）
      44 条基线复核 rc=0、核心域 50.7% 无回落；`go vet ./...` 全仓干净；
      干净树全量测试暴露的两处真红已修：`module_tables.go` 补登
      `bridge_metrics` / `email_accounts`（附归属理由）、`arch_gorm_guard_test.go`
      棘轮 169→168（棘轮不许高于现实）
- [x] T038 后端用例：`service/selector_health_test.go`（判定/校验/夹取/新鲜度/阈值回落）、
      `repository/bridge_selector_health_test.go`（真库，累加与排序）
- [x] T039 全部门禁复跑：config-param 三道 / model_migration / api_contract --strict /
      enum / workflow_refs / ci-gate-paths / hardcode-tunable / go vet / gofmt

## Phase 6 · 接线与交付

- [x] T040 `make audit` 末尾接三步（A2-a、A2-b `--workflow`、A4-a 带 SKIP 语义）
- [x] T041 `user-server-ci.yml` static gates 两步 + coverage 作业 core-coverage 两步 +
      push/PR paths 各 7 条
- [x] T042 本 spec 目录（spec / plan / tasks / delivery）+ `CHANGELOG.md` 条目
- [x] T043 回写 `docs/开发计划-改进清单-2026-10-23.md` 的 Wave 2 状态与验收读数
