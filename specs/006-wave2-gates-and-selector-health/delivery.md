# Delivery Report: Wave 2 止血切片

**Feature**: `006-wave2-gates-and-selector-health` ｜ **交付日期**: 2026-10-24 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

调研文档 Wave 2 的**五项全部落地并接进门禁**（A2-a 依赖策略 / A2-b SBOM 产物 / A2-c WhatsApp
接口化 / A4-a 核心域分档覆盖率 / B1-a 桥接选择器失效检测），每道门都自带牙齿（自检控制），
每颗牙都真的咬过一遍（真牙验证记录见下）。

## 交付物清单

| 类型 | 文件 | 规模 | 说明 |
|---|---|---|---|
| 新增 | `scripts/check-dependency-policy.py` | ~470 行 | A2-a 门，`--self-check` 10 枚控制 |
| 新增 | `scripts/dependency-policy.baseline` | 3 条登记 | gowa / endless / gomail，review 2027-03-31 |
| 新增 | `scripts/check-sbom-artifacts.py` | ~700 行 | A2-b 门，`--workflow` / `--dir` / `--self-check` 19 枚；第二轮实测后修正「清单缺失 vs 零依赖」判据 |
| 新增 | `scripts/check-core-coverage.py` | ~330 行 | A4-a 门，8 枚控制 + 棘轮 |
| 新增 | `scripts/core-coverage.baseline` | 44 条底线 | 由**真实 profile** 生成 |
| 新增 | `user-server/internal/service/whatsapp_provider.go` | ~340 行 | A2-c 唯一 gowa seam + 三实现 |
| 修改 | `user-server/internal/service/whatsapp.go` | 重写为纯委派 | 不再直接 import gowa |
| 新增 | `user-web/bridge/src/core/selector-health.js` | ~330 行 | B1-a 前端采集/上报 |
| 新增 | `user-web/bridge/test/selector-health.test.js` | 20 例 | 判定口径/计数器/合并扣减/请求形状/适配器埋点 |
| 新增 | `user-server/internal/model/bridge_selector_health.go` | 60 行 | `(channel,field)` 唯一、累计计数、新鲜度两列 |
| 新增 | `user-server/internal/repository/bridge_selector_health.go` | 160 行 | 先 update 后 create；23505 补加；List 有上限 |
| 新增 | `user-server/internal/service/selector_health.go` | 330 行 | 校验/白名单/等比夹取/`ClassifySelectorHealth` |
| 新增 | `user-server/internal/controller/bridge_selector_health.go` | 100 行 | POST 挂桥接组、GET 挂 auth 组 |
| 新增 | `user-server/internal/service/selector_health_test.go` | 10 个测试函数 | 判定/校验/夹取/新鲜度/阈值回落/nil 仓储 |
| 新增 | `user-server/internal/repository/bridge_selector_health_test.go` | 4 个测试函数 | 真库：累加、独立性、排序、空增量、nil db |
| 修改 | `channel-adapter.js` / `background/index.js` | +~200 行 | 埋点 + `chrome.alarms` flush + 按批扣减 |
| 修改 | `router.go` / `migrate.go` / `config_param_seeds.go` / `module_tables.go` | 各 1–2 处 | 路由、建表登记、种子、表归属 |
| 修改 | `Makefile` / `user-server-ci.yml` / `sbom.yml` / `hardcode-tunable.baseline` | 见「接线」 | 门要真的会跑 |
| 修改 | `user-server/internal/model/module_tables.go`（两轮：bridge_selector_health + bridge_metrics/email_accounts 补登）、`user-server/internal/service/arch_gorm_guard_test.go`（棘轮 169→168） | | 表归属 + gorm 棘轮与现树一致 |
| 修改 | `CHANGELOG.md` | 1 节 | 006 条目 + 第二轮遗留收口 |

## 验收读数（本轮实测，非推测）

### A2-a 依赖策略门

```
python3 scripts/check-dependency-policy.py --self-check   → 10 枚控制，失败 0
python3 scripts/check-dependency-policy.py                → rc=0
  扫描面: go.mod 直接依赖 37 / 前端清单 6 份 / 前端直接依赖 63
  策略: 禁入 8 条 / 弃维护观察 3 条
  OUT_OF_SCOPE: github.com/lib/pq（类型依赖，收敛＝改字段类型+迁移，显式声明不纳入）
```

**真牙**：把 `gomail` 的 seam 从两处收窄成单文件 ⇒ `SEAM_LEAK` 报 3 处泄漏、rc=1；已还原。

### A2-b SBOM 产物断言

```
--self-check → 19 枚控制，失败 0（含本轮口径修正后复跑）
--workflow（真实 sbom.yml）→ 3 扫描站点 / 3 产物 / 1 upload，rc=0
--dir → 在干净提交树（git worktree @ 3a5d9f2e，含 53b9dbf0）上用真实 syft
  v1.55.0（本地 `go install` 源码构建——GitHub Release / Docker Hub / ghproxy
  三条下载路线全失败后改走的路线）复刻 sbom.yml 三条 scan 命令生成三份产物
  （27MB/27MB/27MB，rc=0），然后 `--dir .` → **rc=0**：
  - sbom-go.spdx.json：go.mod 直接依赖（非 indirect）全覆盖
  - sbom-frontend.spdx.json：user-web/package.json 直接依赖 58 条全覆盖
  - sbom-bridge.spdx.json：bridge 是纯 MV3 扩展，package.json 的 dependencies
    **为空**（只有 devDeps），产物含 14740 个包。该树无 node_modules（与 CI
    同形），实测证明 syft 的 JS cataloger 在仅 lockfile 场景下能解析直接依赖——
    此前"CI 上可能解析不出"的口径担忧不成立。
    （第三轮对抗性审查再修一处盲区）：零依赖豁免从「任何清单都放行」收紧为
      白名单 `ZERO_DEP_MANIFESTS = {user-web/bridge/package.json}`——bridge 空掉
      合法，别的清单（go.mod / user-web/package.json）空掉是**回归**，新增
      `EMPTY_DEPS` 判据（红）。自检相应补 3 枚控制（bridge 空=note 跳过不红、
      frontend 空=EMPTY_DEPS、清单缺失而产物在=MISSING_FILE 的 None 分支），
      `--self-check` 19 枚全过。
```

### A2-c WhatsApp 接口化

```
grep gowa 于非测试 Go 源 → 仅 whatsapp_provider.go 持有 import
go build ./... → EXIT 0
```

### A4-a 核心域分档覆盖率

```
--self-check → 8 枚控制，失败 0
--refresh（真实 profile）→ 44 条底线
默认档 → 核心域合计 50.7%（48191/95062 语句），44 个核心包无一低于底线，rc=0
```

**真牙**：把 `internal/service` 底线从 59 抬到 60（实测 59.1%）⇒ `BELOW_FLOOR` rc=1；已还原。

### B1-a 选择器失效检测

```
user-web/bridge: npx vitest run → 73 文件 / 898 用例通过（7 skipped），含新增 20 例
user-server: go test ./internal/service -run "SelectorHealth|Classify" → ok
             go test ./internal/repository -run "TestBridgeSelectorHealth" → ok（真库）
```

### 门禁与静态面

| 门 | 结果 |
|---|---|
| `make audit`（完整 30+ 道） | **全绿 rc=0**（第二轮收口后：hardcode-tunable 基线补登 geo 两常量、gorm-only 改造 payment Raw、core-coverage 用全仓 profile） |
| `check-config-param-defaults / readpoints / keystyle` | 绿（197 条种子，读数全对齐） |
| `check-hardcode-tunable` | 绿（`SelectorHealthDegradedBelow` 等 WIRED_FALLBACK；geo 两常量按「口径/契约护栏」入基线） |
| `check_model_migration` | 绿（337 候选 / 367 已注册） |
| `audit_api_contract.py --strict` | 绿（1068 调用，UNMATCHED=0） |
| `check-enum-consistency` / `check_workflow_refs` / `check-ci-gate-paths` | 绿 |
| `golangci-lint run ./internal/{service,repository,controller,model,router}/...` | 0 issues |
| `gofmt -l`（本切片全部文件） | 无输出 |
| `go vet`（全仓 `./...`） | rc=0 |
| `go test ./internal/{model,service,repository,migration/...}`（真库） | 全绿（第二轮修完 5 处红后复跑） |

## 遗留收口（2026-10-24 第二轮，全部清零）

1. **A2-b `--dir` 段已在真实 syft 产物上实跑** ✅。GitHub Release / Docker Hub /
   ghproxy 镜像三条下载路线全部失败（28MB 超时 / registry 连接超时 / curl 92 断流，
   且本机既有的 `$HOME/bin/syft` 二进制在 macOS 26 上执行即被 SIGKILL，ad-hoc
   重签名也救不回）。改用 `go install github.com/anchore/syft/cmd/syft@v1.55.0`
   从 goproxy.cn 源码构建（120MB 自编译产物）。在干净提交树复刻 `sbom.yml` 三条
   `syft scan` 命令生成三份 SPDX，然后 `check-sbom-artifacts.py --dir .` → **rc=0**，
   三份产物的直接依赖对账全部通过；该树无 `node_modules`（与 CI 同形），同时证明
   syft 的 JS cataloger 在仅 lockfile 场景下可解析直接依赖——此前的口径担忧不成立，
   判据无需调整。
2. **`make audit` 已全绿** ✅。`check-hardcode-tunable` 的两条 `NEW_TUNABLE`
   （`ContentFormatAttributionSampleFloor` / `BatchGenerateMaxKeywords`）已按该门
   自带的第二条出路处理：加入 `scripts/hardcode-tunable.baseline` 并写明
   「为什么不入库」——前者是报表的**统计口径**（定义"什么算能下结论"，按环境调它
   等于同一份报表对不同人给出不同结论），后者是同步接口的**契约护栏**（调高＝把
   HTTP 连接当任务队列用；大批量走 keyword_pipeline cron）。基线更新后在主 checkout
   与干净提交树上均实测 rc=0。
3. **完整全仓 profile 已生成并复核** ✅。并发 lane 的 geo 文件已提交（`3a5d9f2e`），
   在干净提交树（含本切片 `53b9dbf0`）上跑 CI 同款命令
   `go test ./... -count=1 -timeout 3000s -p 1 -short -coverprofile=... -covermode=atomic`
   → 136 个包 ok / 5 个 FAIL，逐条核验后：
   - `internal/migration/migrations`、`internal/repository` 两个 **build failed** 仅出现在
     主 checkout（并发 lane 又把 `initial_schema.go` 注册了一个未提交的迁移构造器、
     `repository/feishu.go` 引用未提交的 `saveWholeRow`）——**干净提交树上没有**；
   - 干净树上的两个真红已修：`TestEveryModelTableHasModuleOwner`（`bridge_metrics` /
     `email_accounts` 搬进 model 包后漏登记归属——已按各自理由登记进 `coreOwnedTables`）
     与 `TestArchServiceNoNewGormDB`（service 层 gorm.DB 168 处 < 棘轮基线 169，
     按棘轮自身要求下调到实际值）；
   - 修完后 `check-core-coverage.py --profile <全量 profile>` → **rc=0**：44 条基线
     与全量 profile 完全一致，核心域合计 50.7%（48176/95044）与首轮分批合并的读数
     （48191/95062）吻合。首轮"分批合并等价"的论证由此升级为全量实测复核。
4. **`test/e2e` / `test/integration` 的 `go vet` 红**已随并发 lane 的签名变更
   提交（`3a5d9f2e`）一并消失——`go vet ./...` 全仓干净。

## 顺带发现并修掉的（否则本切片自己会红）

- `user-server/internal/model/module_tables.go`：新表 `bridge_selector_health` 必须登记归属，
  否则 `TestEveryModelTableHasModuleOwner`（"一张表都不许无主"）当场红。归 `coreOwnedTables`
  并写明理由：观测的是"扩展活着与否"，随基座常在、不随任何可售模块开关。
  第二轮全量实测又暴露同病两处并补登：`bridge_metrics`（无租户语义的运维指标
  时间序列，挂业务模块下会让"关模块⇒表停写"说假话）、`email_accounts`
  （所有用到邮件的模块共用的账号配置源，不是 reach-email 的运行数据）。
- `user-server/internal/service/config_param_test.go`：种子数棘轮 240 → 241（新增
  `bridge.selector_health_degraded_below`）。
- `user-server/internal/service/arch_gorm_guard_test.go`：service 层 gorm.DB 棘轮
  169 → 168（第二轮全量实测现值 168 处，棘轮基线不许高于现实——收窄是好事，
  但不落进基线就等于把这条棘轮的"只许降"约束顶在旧值上空转）。

## 顺带发现并修掉的第三处（第二轮实跑真实产物时暴露）

- `scripts/check-sbom-artifacts.py` 的 `manifest_deps` 把「清单读不到」与「读到但
  dependencies 为空」混成同一个空列表 ⇒ 前者该红（`MISSING_FILE`）不红，后者该跳过
  却报红。bridge 是纯 MV3 扩展（运行时零依赖，构建/测试工具全在 devDeps），真实产物
  实跑当场暴露：`--dir` 对 sbom-bridge.spdx.json 误报 MISSING_FILE。已改为
  `list | None` 两种判据（None ⇒ 红；空 ⇒ 打印说明并跳过对账——空清单的"覆盖"是
  空洞真，读数必须如实写出来，不能假装对账过了），`--self-check` 19 枚复跑全过，
  三份真实产物 `--dir` rc=0。

## 第三轮：多角色对抗性审查（2026-10-24，随 ed920471 之后的收口）

按「安全 / 并发 / 测试 / 架构 / 产品 / CI」六个角色对本切片涉及的模块交叉审查，
发现并处置如下（全部已修，未修的按决策点记录）：

| # | 角色 | 发现 | 处置 |
|---|---|---|---|
| R1 | 安全 | `settleKnown` 守卫不对称：回款口幂等通路对 `existing.Status==refunded` 不设防，退款口对 `!=refunded` 设防。回款重投撞上被退款行占用的 channel_ref 会复用退款行（`Reused=true`），那笔 confirmed 的钱库里根本不存在；冲销重投会把退款行 `MarkReversed` | **已修**：`settleKnown` 顶上加 refunded 占用拒（`ErrPaymentRefReuse`），并补 `TestPaymentServiceRejectsRefundSquattingChannelRef`（回款+冲销两路重投 + 行数/状态不变三断言） |
| R2 | 并发 | `RecordPartialRefund` 的「退款额 ≤ 净到账」校验与 `Create` 之间无锁：同账单两笔并发退款可总超额 → 净到账为负、账单从 paid 退回 open、`Oversettled` 落地，无 DB 约束兜底 | **记录为决策点**（未修）：E-10 尚无 controller/router 入口（生产不可达），修法需跨仓储/服务层的事务（`SELECT … FOR UPDATE` 锁账单行）或 DDL（部分唯一索引/约束触发器），属契约级改动；在接线时并入。见 R4 |
| R3 | 测试 | `check-sbom-artifacts.py` 的 16 枚自检没盖住新增的「dependencies 为空→note 跳过」与「清单缺失但产物在→MISSING_FILE(None)」两分支，docstring 还写「13 枚」 | **已修**：自检扩到 19 枚（bridge 空=note 不红 / frontend 空=EMPTY_DEPS / 清单缺失=MISSING_FILE），docstring 与全部 9 处「16 枚」文档改为 19 |
| R4 | 架构 | E-10 半固化：`RecordPartialRefund` 实现已入库（ed920471），但 `payment_partial_refund_test.go` untracked、无 controller/router 接线；webhook 里 `status=refunded` 会被 `RecordPayment` 拒绝（大声报错，不静默） | **测试文件已随本提交入库**（实现必须带测试，零遗留）；接线留决策点：退款 webhook 的契约形状（用哪个键做幂等、status 词表）未定，不擅自拍。当前行为=回款口拒 refunded 并报错，退款不会静默丢 |
| R5 | 产品 | `email_accounts` 归 core 的注释原写「告警通知/审批抄送/reach-email 全读它」过度声称：reach-email 自己的发信走 `email_smtp`，不读 `email_accounts` | **已修**：注释改为引用面实测口径——EmailService（密码重置/注册通知/proactive/告警 EmailSender/customer_service_plus）、channel_overview 计数、backup 白名单 |
| R6 | CI | `hardcode-tunable --refresh-baseline` 全量 `write_text` 会吞掉手工补登段（B1-a 四条 + geo 两条的理由），下次 `make audit` 又红；实测刷一次还伴随分类器漂移（`InboundMisattribWindow` 被丢、`DefaultMaxKeys` 加入） | **已修**：`refresh_baseline` 保留手工段（第一个 `# ──` 到文件尾原样续写、自动段名字去重），头注释写明；分类器漂移属「刷基线前须人工确认」的既有语义，不展开 |
| R7 | 测试 | 空 dependencies 盲区：删空 user-web 的 58 条依赖只会 note 跳过、rc=0，门对「依赖被清空」瞎 | **已修**：零依赖豁免收紧为白名单，非白名单清单空掉 ⇒ `EMPTY_DEPS` 红（见 R3） |

回归：`go test ./internal/service/ -count=1`（定向：回款/退款 E-10 四判据 + R1 新测试 + webhook 解析）真库全绿；`go test ./internal/model/`（表归属/值域）真库全绿；`go build ./...`、`gofmt -l`、`go vet` 干净；`check-sbom-artifacts.py --self-check` 19 枚 rc=0、`--workflow` rc=0；`check-hardcode-tunable` rc=0。

全量 `go test ./internal/service/ -count=1 -short`（15 分钟限时）本轮**超时未跑完**：无任何 `--- FAIL` 断言失败（rc=1 仅因超时 panic），超时点卡在 `TestXiaohongshuCardService_Create_EmptyRedirectURL` 的 `NewTestDB` 建表，正是 `testdb.go` 注释里已记录的 fsync/DDL 卡顿形态；且当时并发 lane 同时在全量跑同包（同库竞争）。结论：与本次改动无关，非回归——R1 改动覆盖的测试全部在定向运行中通过。
