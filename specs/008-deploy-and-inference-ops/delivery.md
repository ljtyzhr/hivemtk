# Delivery Report: C1 部署向导 + C2 推理降级档

**Feature**: `008-deploy-and-inference-ops` ｜ **交付日期**: 2026-10-24 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

部署侧补齐「`scripts/deploy-wizard.py` 交互向导 + `--check` 只读自检」，推理侧补齐
「启动时显式定档（full_local / cloud_llm_local_embedding / local_down_no_cloud）+ 记录 +
`[inference-tier]` 日志」——C1 让 `.env` 不再靠手工，C2 让「本地推理挂了现在跑哪一档」
从一堆 warn 变成一条可判定状态。

## 交付物清单

| 类型 | 文件 | 说明 |
|---|---|---|
| 新增 | `scripts/deploy-wizard.py` | C1：交互向导 + `--check` 只读自检 + `--yes` 非交互；纯 Python3 stdlib，约 330 行 |
| 新增 | `user-server/internal/app/inference_tier.go` | C2：档位枚举 / 选择器 / HTTP 探测（根路径 `/health`）/ 策略组装 / 启动装配入口，约 245 行 |
| 新增 | `user-server/internal/app/inference_tier_test.go` | C2：表驱动纯单测（fake probe + httptest），6 个测试函数 |
| 修改 | `user-server/internal/config/server.go` | `InferenceConfig.Tier` + `InferenceTierConfig`；`DefaultInferenceConfig()` 显式 `FallbackToCloudLLM:false` |
| 修改 | `user-server/config.yaml` | `inference.tier.fallback_to_cloud_llm: false`（带中文注释） |
| 修改 | `user-server/cmd/api/main.go` | provider/路由装载后接入 `app.SelectAndRecordInferenceTier` |
| 修改 | `docs/DEPLOYMENT_GUIDE.md` | 目录 + §五第 1 步向导推荐注 + 新增 §十二/§十三；未改他人既有段落 |
| 新增 | `specs/008-deploy-and-inference-ops/{spec,plan,tasks,delivery}.md` | 本泳道四件套 |

## 验收读数（本轮实测，非推测）

### C2 单测（`go test` 单文件包，绕开并发泳道阻塞）

```
cd user-server
go test ./internal/app/inference_tier.go ./internal/app/inference_tier_test.go -count=1
  → ok  command-line-arguments  (全部 PASS)
```

覆盖：三档判定（本地优先 / 降级 / 无云端 / 禁用降级）、nil selector、
`BuildTierPolicyFromConfig` 云端判据四态、httptest 探测（2xx 可达、5xx 不可达、
空 base_url、非绝对地址、尾斜杠、`/v1` 前缀仍钉根路径 `/health`）、决策记录副本语义。

### 静态检查

```
gofmt -l internal/app/inference_tier.go internal/app/inference_tier_test.go \
        internal/config/server.go cmd/api/main.go
  → 空输出（全部干净）

go build ./internal/config   → OK
go vet ./internal/config     → OK
```

### C1 自检实证

```
python3 -m py_compile scripts/deploy-wizard.py   → OK
python3 scripts/deploy-wizard.py --check          → rc=0
```

本机输出（截取判定行）：

```
[PASS] Python 3.14.6 可用
[PASS] .env-example 模板存在
[PASS] .env 存在
[WARN] JWT 密钥已设置但仍是占位值（Chan…(len=55)）
[WARN] FIELD_ENCRYPTION_KEY 仍为占位值（Chan…(len=46)）
[PASS] PostgreSQL 可达（127.0.0.1:8202）
[PASS] Redis 可达（127.0.0.1:8203）
[WARN] 本地 LLM 未就绪（http://127.0.0.1:8207/v1）——启动后 user-server 会定档并打日志 [inference-tier]
[PASS] Embedding 健康（http://127.0.0.1:8208/v1 → /health 2xx）
[PASS] Rerank 健康（http://127.0.0.1:8209 → /health 2xx）
自检完成：FAIL=0，WARN=3 → rc=0
```

`.env` 读写逻辑在**临时目录**实测（不碰真实 `.env`）：

- 占位值 `ChangeMe_*` 被替换为 64 字符 hex；
- 已设置的非占位值（`JWT_SECRET`）保持不变；
- 注释与未涉及键逐字节保留（含 `JWT_SECRET='abc'` 原样）；
- 新键追加到文件尾；写入为原子（tmp + `os.replace`）。

## 决策点清单

1. **C1 用 Python3 纯 stdlib**：规避 shellcheck / bash 3.2 / CJK 展开三道 shell 门。
2. **`--check` rc 语义**：`0`=无 FAIL，`1`=有 FAIL，`2`=自检无法执行；FAIL 仅限
   `.env` 存在但 JWT/FIELD 缺失或 <32 字符；其余全 WARN → 本机 rc=0。
3. **C2 落点 `internal/app`（装配层）**：档位是部署决策不是调度决策；`aiagent/llm`
   已有运行时回落链，不叠档位概念、不改 Dispatcher 路由。
4. **云端判据**与 `registerCloudProvidersFromConfig` 一致（enabled + api_key + base_url）。
5. **健康探测钉根路径 `/health`**（origin 拼法）：与 inference-host `wait_health` 同口径，
   避免 `/v1/health` 在非 llama.cpp 网关上 404 误报。
6. **Embedding 不参与降级**：无论档位都走本地（数据不出域）。

## 未实施与原因

| 事项 | 原因 |
|---|---|
| Makefile / CI 接线（`deploy-check` / `deploy-init`） | 铁律：接线由主智能体做，本泳道只给建议（见下） |
| 档位写成 DB 可配置项 | 非目标：只读 config.yaml，避免运行时漂移 |
| 启动时自动重启本地 LLM / 自动切路由 | 非目标：运行时回落链已存在 |

## 建议接线点（由主智能体实施）

```makefile
deploy-check:
	python3 scripts/deploy-wizard.py --check
deploy-init:
	python3 scripts/deploy-wizard.py --yes
```

- `deploy-check` 挂到发布门禁（与 `make lint vet test-go` 并列）；
- `deploy-init` 作为 `make install` 的向导替代入口。

## 全仓构建阻塞（非本泳道）

`cd user-server && go build ./...` 当前失败，责任文件非本泳道：

```
# hivemtk-user/internal/repository
internal/repository/observability_dashboard.go:57:6: FunnelStageRow redeclared in this block
    internal/repository/dashboard_stats.go:30:6: other declaration of FunnelStageRow
```

- `observability_dashboard.go` 为并发泳道（007 观测性）**未跟踪新增文件**；
  `dashboard_stats.go` 为既有已提交文件；二者重名声明 `FunnelStageRow`。
- 本泳道文件未触碰 `internal/repository/**`，以「单文件包 `go test` +
  `go build ./internal/config` + `gofmt` + `go vet ./internal/config`」验证类型正确。
