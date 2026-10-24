# Feature Specification: Wave 2 止血切片 —— 五道判据门 + 桥接选择器失效检测

**Feature Branch**: `006-wave2-gates-and-selector-health`

**Created**: 2026-10-24

**Status**: Approved

**Input**: 用户指令「继续执行，不留任何遗留」——把《HiveMtk 深度调研论证与改进方向》里
Wave 2 的五项（A2-a / A2-b / A2-c / A4-a / B1-a）**一次做完并全部接进门禁**，
不留给下一轮。

## 本切片的共同主题：把「看起来做完了」变成「坏了就会红」

Wave 1（E1 合规策略）解决的是**能力缺失**。Wave 2 的五项解决的完全是另一类问题，
它们都有同一个形状：**功能在跑、CI 在绿，而它其实什么都没守住**。

| 项 | 现状的假绿形态 |
|---|---|
| A2-a 依赖策略 | `go.mod` 里有弃维护库在到处被 import，`govulncheck`/`npm audit` 只说「没有已知 CVE」⇒ 静默腐烂 |
| A2-b SBOM | 工作流退出码 0，产物可能是 `table` 格式、三份内容雷同、或零 package |
| A2-c WhatsApp | 旧库（`gowa`）被 `whatsapp.go` 直接深度调用，任何替换动作都是全仓改 |
| A4-a 覆盖率 | 全仓一个 22% 的数：核心包掉 20 点、边缘包涨回来 ⇒ 绿灯 |
| B1-a 选择器 | 平台改版后扩展仍在跑，只是再也读不到消息；后端看到「这个账号今天没客户」 |

## User Scenarios & Testing

### US1 · A2-a 依赖策略门（Priority: P0）

作为维护者，我要在依赖被引入的**当天**就知道「这个库已弃维护、且引用面散在 7 处」，
而不是在换库那天才发现要改 7 个文件。

**关键判据（本门刻意不做什么也一样重要）**：不重复 `govulncheck` 与 `npm audit` 的 CVE 面——
那两者已经在 CI 里跑，重复只会让本门在「都绿」时提供零信息。本门只守它们结构上抓不到的：

1. 弃维护库的**引用面**必须收敛到声明的 seam（`module | seam | review | why`），
   引用面变宽 ⇒ `SEAM_LEAK`；
2. 每条弃维护登记必须有**到期复查日**，过期 ⇒ `REVIEW_OVERDUE`；
3. 依赖声明**形状**：`latest` / `*` / URL / 相对路径 `replace` ⇒ `BAD_DECL`；
4. **禁入名单**：即便没有 CVE 也禁（例如已确认的供应链风险库）⇒ `BANNED`。

**Acceptance Scenarios**:

1. Given 基线声明 `gowa` 只许出现在 `service/whatsapp_provider.go`，
   When 有人在 `service/whatsapp.go` 里 `import gowa`，Then 门红 `SEAM_LEAK` rc=1。
2. Given `lib/pq` 刻意留在门外（driver 注册需要空白导入），Then 每次运行都打印
   `OUT_OF_SCOPE` 与原因，而不是静默跳过。

### US2 · A2-b SBOM 产物断言（Priority: P0）

作为拿到 SBOM 的人（出事了才去拿），我要保证拿到的那份**真的覆盖本仓的直接依赖**。

判据分两段，缺一不可：
- `--workflow`（纯静态，不需要 syft）：三份期望产物都被某步生成、每步声明了
  `working-directory`、输出是 `spdx-json`、产物不落在被扫描目录内、每个产物都被某个
  upload 步骤收录、每个上传路径都有生成步骤、upload 设了 `retention-days`、
  挂 Release 的工作流必须有 `contents: write`；
- `--dir`（产物级，需要 syft 已跑过）：文件存在、可解析 SPDX、`packages` 非空、
  `SPDXID` 不重复、**每份清单的直接依赖全部出现在产物里**。

### US3 · A2-c WhatsApp 接口化（Priority: P0）

作为要换库的人，我希望旧库（`gowa`）只出现在**一个文件**里。

判据：`whatsapp_provider.go` 是唯一 seam；`whatsapp.go` 变成纯委派；
Cloud API 与旧库两个实现都满足 `WhatsAppProvider`（Kind / SendText / SendTemplate）；
登录面单独一个 `WhatsAppLoginProvider`（不是所有渠道都需要登录）；
个人版不支持模板消息时返回 `ErrWhatsAppProviderUnsupported` 而**不退化成文本**
（退化会让「本应发模板的消息」在某天静默变成纯文本，且没人会知道）。

### US4 · A4-a 核心域分档覆盖率（Priority: P0）

作为 tech lead，我要看到「核心包各自的底线」，而不是一个能互相抵消的平均数。

判据：`CORE_PREFIXES` 命中的每个包在 `scripts/core-coverage.baseline` 里各有一条底线；
低于底线 ⇒ `BELOW_FLOOR`；新核心包没登记 ⇒ `UNREGISTERED`；基线登记了但 profile 里
没有 ⇒ `NOT_IN_PROFILE`（**没测到 ≠ 0%**）；基线里的包已删除 ⇒ `STALE`。
棘轮只涨；下调要 `--allow-drop <包名>` 显式签字。

### US5 · B1-a 桥接选择器失效检测（Priority: P0）

作为运营，平台改版后我要**当天**看到「抖音的 messageItem 命中率从 97% 掉到 0%」，
而不是一周后发现这个账号"没客户"。

分工是本功能最重要的一条设计：
- **扩展侧只上报事实**（attempts / hits），不做判定；
- **服务端做判定**，阈值走参数中心 `bridge.selector_health_degraded_below`（兜底 60），
  改阈值**不需要发扩展版本**。

判定顺序固定：样本不足 ⇒ `warming`（不是 broken）；样本够了且零命中 ⇒ `broken`；
命中率低于阈值 ⇒ `degraded`；否则 `ok`。

## 明确不在本切片内（边界，写清而不是假装没有）

- A2-b 的 `--dir` 段**本地未跑过真实 syft**（本机 syft 下载在该网络下超时 90s+）。
  自检夹具（19 枚控制）已覆盖它的全部判据，首次真实运行发生在 CI 的 `sbom.yml`。
- A2-c 只做接口化与 seam 收敛，**不删旧库**：删 `gowa` 是独立的一次决策
  （还有存量个人版账号在用）。
- B1-a 只做「事实采集 + 判定 + 运营读数」，**不做告警推送**（阈值告警属 Wave 3 的 D2 观测主线）。
- A4-a 只给核心包定底线，不改 CI 现有的全仓 FLOOR/TARGET 与 codecov 配置。
