# Feature Specification: C1 部署向导 + C2 推理降级档

**Feature Branch**: `008-deploy-and-inference-ops`

**Created**: 2026-10-24

**Status**: Approved

**Input**: 交付泳道「部署与推理运维」两个缺口——部署阶段还靠手工编辑 `.env`、
本地推理不可达时没有一条「当前跑在哪一档」的可判定状态。

## 背景与问题

### C1：部署配置靠手工编辑 `.env`

现状 `make install` 只做「复制 `.env-example` 为 `.env` 并提示你修改敏感字段」，
密钥长度、占位值是否替换、组件是否就绪全靠人眼。部署方需要：

1. 交互收集关键项的向导，且对占位密钥自动强随机替换；
2. 一个 CI / 巡检可用的**非交互只读自检**，退出码语义明确。

### C2：本地推理不可达时，系统没有「档位」概念

本地 LLM（llama.cpp / 宿主机推理栈）挂掉时，运行时回落链（`pickEnabledFallback` +
`ProviderFailover` 熔断 + 模板兜底）一直存在，但它是**运行时**动作——日志里只有一串
warn，没有一条「现在跑在 full_local 还是 cloud_llm_local_embedding 还是无降级可用」
的可判定状态。需要补上**启动时显式定档 + 记录 + 打日志**这个缺口。

## 需求

### R1（C1）部署向导 `scripts/deploy-wizard.py`

- 交互收集关键项：`POSTGRES_PASSWORD` / `REDIS_PASSWORD` / `JWT_SECRET` /
  `FIELD_ENCRYPTION_KEY`，可选 `MASTER_KEY` / `SEED_PASSWORD`，平台集成开启时补
  `MERCHANT_API_SECRET` / `PLATFORM_ADMIN_PASSWORD`；
- 占位密钥（`ChangeMe_*` / `your_*`）自动替换为强随机值；
- 写回 `.env` 时保留注释与既有键，原子写入；
- `--check` 非交互只读自检：工具链、`.env`、JWT/FIELD 长度、DB/Redis TCP、
  LLM/Embedding/Rerank `GET /health`；
- **退出码语义**：`0` = 无 FAIL；`1` = 有 FAIL；`2` = 自检自身无法执行；
- **FAIL 仅限硬配置错误**（`.env` 存在但 JWT/FIELD 缺失或 <32 字符）；
  组件不可达 / 占位密钥 / 缺 `.env` / 缺工具链为 WARN（不挡 rc）；
- 用法写入 `docs/DEPLOYMENT_GUIDE.md`（合并现有内容，不动他人段落）。

### R2（C2）推理降级档配置项

- 配置项 `inference.tier.fallback_to_cloud_llm`（默认 `false`），落到
  `user-server/internal/config/server.go`（`InferenceTierConfig`）与 `config.yaml`；
- 默认 `false` 的合规基线：云端 LLM 出域必须显式开启。

### R3（C2）档位选择逻辑（service/app 层）

- 档位枚举：`full_local` / `cloud_llm_local_embedding` / `local_down_no_cloud`；
- 判定顺序固定：① 本地 LLM 可达 ⇒ `full_local`；② 否则允许降级且已配云端 ⇒
  `cloud_llm_local_embedding`；③ 其余 ⇒ `local_down_no_cloud`；
- 「云端已配置」判据与 `registerCloudProvidersFromConfig` 一致：
  `enabled=true` 且 `api_key` 非空且 `base_url` 非空；
- Embedding 无论档位都保持本地（数据不出域）；
- 可用性探测必须可注入接口（单测不连真服务）；
- 启动时记录决策 + 打 `[inference-tier]` 日志，不修改 Dispatcher 运行时路由。

## 关键设计决策

1. **C1 用 Python3 纯 stdlib 而非 .sh**：规避 shellcheck / macOS bash 3.2 / CJK 展开
   三道 shell 门。
2. **`--check` 的 rc 语义朝「本机可 rc=0」**：组件不可达、占位密钥、缺 `.env` 都是
   WARN 不挡 rc；FAIL 只留给「配了但配错了」的硬错误。这样一台「推理栈还没拉起」的
   机器自检不会误报为配置失败。
3. **C2 落点在 `internal/app`（装配层）而非 `aiagent/llm`**：档位是部署决策不是调度
   决策；`aiagent/llm` 已有运行时回落链，不再叠一层档位概念；装配层读 config、
   组装组件、记录决策，不引入新的依赖方向。
4. **C2 不修改 Dispatcher 路由**：运行时回落链已存在，本档位只补「启动时显式定档
   并说出来」。
5. **健康探测钉根路径 `/health`**：与 `scripts/inference-host/_common.sh` 的
   `wait_health` 同口径；base_url 带 `/v1` 前缀时也探 `origin/health`，避免在非
   llama.cpp 网关（如 8787 freeapi）上把 `/v1/health` 误判成 404。

## User Scenarios & Testing

### US1 · C1 部署向导（Priority: P1）

作为部署方，我跑一条命令就能把 `.env` 的必改密钥换成强随机值，再用同一条命令的
`--check` 模式拿到一份「哪些是硬错误、哪些只是没启动」的体检单。

**Acceptance Scenarios**:

1. Given `.env` 里 `POSTGRES_PASSWORD=ChangeMe_...` 是占位值，When 交互模式对这四个
   必改密钥回车，Then 四个密钥被替换为 64 字符 hex，`JWT_SECRET` 等已设置的非占位值
   保持不变，注释与其它键逐字节保留；
2. Given 一台只拉了代码、没起任何组件的机器，When 跑 `--check`，Then rc=0（只有 WARN）；
3. Given `.env` 存在但 `FIELD_ENCRYPTION_KEY` 缺失，When 跑 `--check`，Then rc=1；
4. Given `.env` 的 JWT 密钥仍是 `ChangeMe_` 占位但长度 ≥32，When 跑 `--check`，
   Then rc=0 且打印 WARN。

### US2 · C2 推理降级档（Priority: P1）

作为运维，我要在启动日志里看到一条明确的档位声明，而不是从一串熔断 warn 里猜。

**Acceptance Scenarios**:

1. Given 本地 LLM `/health` 2xx，When 选择器执行，Then 档位 = `full_local`（即使
   允许降级且已配云端，本地仍优先）；
2. Given 本地 LLM 不可达、`fallback_to_cloud_llm=true` 且云端 provider 三样齐，
   Then 档位 = `cloud_llm_local_embedding`；
3. Given 本地 LLM 不可达、云端未配置，Then 档位 = `local_down_no_cloud`；
4. Given 本地 LLM 不可达、云端已配置但 `fallback_to_cloud_llm=false`，
   Then 档位 = `local_down_no_cloud`；
5. Given 云端 provider 只有 `api_key` 而无 `base_url`，Then 不算「已配置云端」；
6. Given nil selector，Then 安全返回 `local_down_no_cloud`（不 panic）。

## 非目标（本切片刻意不做）

- 不改 Dispatcher / ProviderFailover 运行时回落链；
- 不做启动时自动重启本地 LLM 或自动切换路由；
- 不把档位选择写成 DB 可配置项（只读 config.yaml，避免运行时漂移）；
- C1 不接 Makefile / CI（接线由主智能体实施，本切片只给建议）。
