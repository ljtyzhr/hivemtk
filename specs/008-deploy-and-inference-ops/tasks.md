# Tasks: C1 部署向导 + C2 推理降级档

**Feature**: `008-deploy-and-inference-ops` ｜ 勾选即以「已验证」为准（无验证不算完成）

## Phase 1 · C2 推理降级档

- [x] T001 `user-server/internal/app/inference_tier.go`：`InferenceTier` 枚举三档 +
      `TierDecision` + `InferenceReachProbe` 接口 + `httpInferenceProbe`（GET `/health`）
      + `InferenceTierSelector`（nil 安全）+ `BuildTierPolicyFromConfig` +
      `SelectAndRecordInferenceTier`（记录 + 日志）
- [x] T002 云端判据与 `registerCloudProvidersFromConfig` 一致：enabled + api_key 非空 +
      base_url 非空；多 provider 有一个齐全即算已配置
- [x] T003 `user-server/internal/config/server.go`：`InferenceConfig.Tier` +
      `InferenceTierConfig{FallbackToCloudLLM bool}`；`DefaultInferenceConfig()` 显式 false
- [x] T004 `user-server/config.yaml`：`inference.tier.fallback_to_cloud_llm: false`（带注释）
- [x] T005 `user-server/cmd/api/main.go`：provider/路由装载后调用
      `app.SelectAndRecordInferenceTier`（`BuildTierPolicyFromConfig` + `NewHTTPInferenceProbe`）
- [x] T006 `user-server/internal/app/inference_tier_test.go`：表驱动纯单测（fake probe）——
      三档判定 / nil selector / 默认探测 / 云端判据四态 / httptest 探测 URL 拼接与状态码 /
      决策记录副本语义
- [x] T007 验证：`go test`（单文件包）绿、`gofmt` 干净、`go build ./internal/config` 绿、
      `go vet ./internal/config` 绿

## Phase 2 · C1 部署向导

- [x] T008 `scripts/deploy-wizard.py`：纯 stdlib；`load_dotenv` / `update_dotenv`（保留
      注释与既有键、原子写）
- [x] T009 交互模式：四必改 + 可选密钥 + 平台集成开关；占位值自动强随机替换（64 hex）
- [x] T010 `--yes` 非交互：占位密钥全部自动替换，其余保持现状
- [x] T011 `--check` 只读自检：工具链 / `.env` / JWT+FIELD 长度 / PG+Redis TCP /
      LLM+Embedding+Rerank `GET /health`；rc 语义 0/1/2；FAIL 仅限 `.env` 存在但密钥
      缺失或 <32 字符
- [x] T012 自检实证：本机 `--check` rc=0（3 WARN：JWT/FIELD 占位、本地 LLM 未就绪）；
      `.env` 读写逻辑用临时目录验证（注释保留 / 占位替换 / 已设置保持 / 新键追加）
- [x] T013 `docs/DEPLOYMENT_GUIDE.md`：目录增 §十二/§十三；§五第 1 步加向导推荐注；
      追加「部署向导」「推理降级档」两节；不修改他人既有段落

## Phase 3 · 四件套

- [x] T014 `specs/008-deploy-and-inference-ops/{spec,plan,tasks,delivery}.md`：全程中文，
      交付报告记录改动文件 / 命令结果 / 未实施原因 / 建议接线点
