# Implementation Plan: C1 部署向导 + C2 推理降级档

**Feature**: `008-deploy-and-inference-ops` ｜ **Created**: 2026-10-24 ｜ **Spec**: `./spec.md`

## 交付物总览

| 项 | 新增 | 修改 | 牙齿 |
|---|---|---|---|
| C1 | `scripts/deploy-wizard.py` | `docs/DEPLOYMENT_GUIDE.md`（合并现有内容） | `--check` rc 语义 0/1/2 + 交互/读写逻辑自检 |
| C2 | `user-server/internal/app/inference_tier.go`、`user-server/internal/app/inference_tier_test.go` | `user-server/internal/config/server.go`、`user-server/config.yaml`、`user-server/cmd/api/main.go` | 表驱动单测覆盖三档判定 + 探测 URL 拼接 + nil 保护 |

## C1 部署向导

### 结构

- 纯 Python3 标准库（`argparse/os/re/secrets/shutil/socket/sys/urllib`）；
- 模块级常量钉死单一源：项目根目录、`.env-example` / `.env` 路径、端口默认值
  （8202/8203/8207/8208/8209，与 `user-server/internal/config/ports.go` 一致）；
- `load_dotenv` / `update_dotenv`：读 `.env` 风格键值；写回时按行替换目标键、保留
  注释与其它键、新键追加尾部；临时文件 + `os.replace` 原子写；
- `strong_secret()` = `secrets.token_hex(32)`（64 hex）；
- `is_placeholder()`：匹配 `ChangeMe_*` / `your_*`（与 `.env-example` 占位命名一致）。

### 交互模式（默认）

1. 若 `.env` 不存在，先从 `.env-example` 复制（保留全部注释）；
2. 四个必改密钥逐个询问：占位/缺失 ⇒ 回车自动生成；已设置 ⇒ 回车保持；可输入新值；
3. 可选密钥：`MASTER_KEY`（缺省时默认生成）、`SEED_PASSWORD`（缺省时默认跳过）；
4. `PLATFORM_ENABLED` 询问；开启时再补 `MERCHANT_API_SECRET` / `PLATFORM_ADMIN_PASSWORD`；
5. 打印掩码后的写入结果与后续步骤提示。

`--yes` 非交互：占位密钥全部自动强随机替换，其余保持现状（CI / 一键脚本用）。

### `--check` 只读自检

| 检查项 | 判定 |
|---|---|
| Python ≥3.7 / 项目根目录 | 不可用 ⇒ rc=2 |
| `.env-example` 存在 | 缺失 ⇒ WARN |
| `.env` 存在 | 缺失 ⇒ WARN（按纯 env 注入模式继续） |
| JWT（`USER_JWT_SECRET` > `JWT_SECRET`） | `.env` 存在且缺失/短于 32 ⇒ **FAIL** |
| `FIELD_ENCRYPTION_KEY` | `.env` 存在且缺失/短于 32 ⇒ **FAIL** |
| 密钥仍是占位值 | WARN |
| PG / Redis TCP | 不可达 ⇒ WARN |
| LLM / Embedding / Rerank `GET /health` 2xx | 不可达 ⇒ WARN |

健康探测：`_http_health` 用 `urllib.parse.urlparse` 取 scheme+netloc 后探 `{origin}/health`
（base_url 带 `/v1` 前缀也钉根路径）。

### 关键设计决策

- **FAIL 只留给硬配置错误**：`.env` 存在但密钥缺失/过短。其余全部 WARN。
- **写 `.env` 前先复制模板**：`.env` 不存在时交互模式自动从 `.env-example` 复制。
- **不写 PyYAML**：不解析 config.yaml，避免引入依赖；档位预测留给 user-server 启动日志。

## C2 推理降级档

### 结构（`internal/app/inference_tier.go`）

| 符号 | 职责 |
|---|---|
| `InferenceTier`（`full_local` / `cloud_llm_local_embedding` / `local_down_no_cloud`） | 档位枚举 |
| `TierDecision` | 一次决策：档位 + 事实面（LLM/Embedding 可达、云端已配、降级允许）+ 原因 |
| `InferenceReachProbe` 接口 | `ProbeLLM` / `ProbeEmbedding`（测试注入 fake，不连真服务） |
| `httpInferenceProbe` | 默认实现：`GET {origin}/health`，2xx 视为可达 |
| `TierPolicy` | 决策输入（全来自配置，本层不做网络/DB 读取） |
| `InferenceTierSelector.Select` | 纯决策，判定顺序固定，nil 安全 |
| `BuildTierPolicyFromConfig` | 从 `config.AppConfig` 组装策略；云端判据=enabled+api_key+base_url |
| `SelectAndRecordInferenceTier` | 启动装配入口：选择 → 记录 → 打 `[inference-tier]` 日志 |

### 装配接线（`cmd/api/main.go`）

在 `LoadProvidersFromDB` / `LoadRoutesFromDB` 之后、`service.InitIntentRecognizer` 之前：

```go
app.SelectAndRecordInferenceTier(
    context.Background(),
    app.BuildTierPolicyFromConfig(appCfg),
    app.NewHTTPInferenceProbe(3*time.Second),
)
```

### 配置接线（`internal/config/server.go` + `config.yaml`）

- `InferenceConfig` 增 `Tier InferenceTierConfig`（`yaml:"tier"`）；
- `InferenceTierConfig{FallbackToCloudLLM bool}`（`yaml:"fallback_to_cloud_llm"`）；
- `DefaultInferenceConfig()` 显式 `Tier{FallbackToCloudLLM: false}`（合规基线，便于 grep 审计）；
- `config.yaml` 增 `inference.tier.fallback_to_cloud_llm: false`（带中文注释）。

### 测试策略（`inference_tier_test.go`）

- `fakeReachProbe` 可编程探测；
- 表驱动覆盖三档判定、nil selector、默认探测构造；
- `BuildTierPolicyFromConfig` 云端判据四态（缺 api_key / enabled=false / 三样齐 / 多 provider 有一个齐）；
- `httptest` 覆盖健康 2xx / 5xx / 空 base_url / 非绝对地址 / 尾斜杠 / `/v1` 前缀钉根路径；
- `recordTierDecision` / `GetLastTierDecision` 副本语义。

## 风险与约束

- **全仓 `go build ./...` 当前被并发泳道阻塞**：`internal/repository/observability_dashboard.go`
  与既有 `dashboard_stats.go` 重名声明 `FunnelStageRow`（007 泳道在途文件）。本切片文件
  以「`go test` 单文件包 + `go build ./internal/config`」验证类型正确，并在 delivery.md 记录
  阻塞证据与责任方；
- **check-hardcode-tunable 约束**：探测超时由 `NewHTTPInferenceProbe(3*time.Second)` 装配层
  传入，不新增包级数字常量；
- **不动他人文件**：`docs/DEPLOYMENT_GUIDE.md` 只追加新节、不改既有段落；
  Makefile / CI 接线只建议不实施。
