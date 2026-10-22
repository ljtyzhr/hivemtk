# Test Report: LLM 路由与 provider 的后台落库被多租户隔离 fail-closed 拦截

- **Slug**: llm-route-provider-upsert-tenant-isolation
- **Tested**: 2026-10-22
- **Fix**: [fix.md](fix.md)
- **Assessment**: [assessment.md](assessment.md)
- **Result**: passed

## Environment

| 项 | 值 |
|---|---|
| 测试库 | `127.0.0.1:8202`（本地 `mtk-postgres` 容器）/ `user_db_test` |
| 口令 | `POSTGRES_TEST_PASSWORD` 取自 `docker exec mtk-postgres printenv POSTGRES_PASSWORD` |
| 串行 | `-p 1`（各测试包共享同一个测试库，必须串行） |

## 复现（第 2 步：红必须先在场）

把守卫全部摘除（= 完整还原生产现场：无 ctx 裸句柄 + 无显式 tenant_id），
`TestProviderAndRouteWrites_OnModeWithoutTenantCtx` 红，读数原文：

```
dispatcher_persistence_write_system_scope_test.go:59: 装配层写 provider 必须在 system scope 下成功: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
--- FAIL: TestProviderAndRouteWrites_OnModeWithoutTenantCtx (7.48s)
    dispatcher_persistence_write_system_scope_test.go:137: seed provider 失败: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；多租户模式下拒绝执行，请经中间件/TenantRunner/行自证提供租户
--- FAIL: TestProviderAndRouteDeletes_OnModeWithoutTenantCtx (0.13s)
```

红因文案与生产日志**逐字一致**。

## 修复后读数（第 5 步：只读不改源码）

```bash
POSTGRES_TEST_PORT=8202 POSTGRES_TEST_USER=admin POSTGRES_TEST_PASSWORD=<...> \
  go test -p 1 ./internal/aiagent/llm/ -run "TestProviderAndRoute" -v
```

```
--- PASS: TestProviderAndRouteWrites_OnModeWithoutTenantCtx (7.34s)
--- PASS: TestProviderAndRouteDeletes_OnModeWithoutTenantCtx (0.19s)
PASS
ok  	hivemtk-user/internal/aiagent/llm	8.378s
```

两个用例各自钉住的不变量：

| 用例 | 钉住什么 |
|---|---|
| `Writes_OnModeWithoutTenantCtx` | provider 与路由都能落库；行归属默认租户；provider 的 `quality_score` / `source` 保真；重复 upsert 只有 1 行 |
| `Deletes_OnModeWithoutTenantCtx` | provider 与路由都能删掉（后台「删除」不留残影、不复活） |

两个用例的首条断言都是「on 模式无租户 ctx 的裸写必须 fail-closed」
—— 证明**隔离闸本身没被放宽**。

## 守卫反向验证（第 4 步）

见 fix.md「Guard Reverse Verification」。md5 双向对账：

| 对象 | md5 |
|---|---|
| 修复后工作区（最终态） | `193d6bc4c0e7f4425f4f9b7d635e07f2` |
| 注码前基线（注码时记录的修复态） | `193d6bc4c0e7f4425f4f9b7d635e07f2` |
| HEAD blob（未改前） | `3697073cdc5897e1cfee115572ff1f35` |

## 受影响包回归

```bash
go build ./...                    # BUILD_OK
go test -p 1 ./internal/aiagent/llm/   # ok  22.904s
```

## 生产验证

**修复前**（2026-10-22 实测，线上）：后台改 8 个场景路由 → 接口 200，但

```
llm_routing_rules: 0 行
启动日志: [LLM] UpsertRouteToDB 查询 scenario=... 失败: [tenant] 表 llm_routing_rules 需要租户上下文但 ctx 未绑定（query）；...
          [LLM] SetRouteWithAudit 路由落库失败 scenario=...: 同上
```

后台改 provider 质量分 → `{"code":400,"message":"provider 落库失败: [tenant] 表 llm_providers 需要租户上下文但 ctx 未绑定（query）；..."}`

**修复后**（2026-10-22 已发版并复核）：

发版：交叉编译 linux/amd64 → 换二进制 → `docker compose build user-server` → 重启。
容器内二进制 md5 `c22cef845367488e19912b8432af5d6a` 与上传件一致。

后台重配后读数：

```
llm_routing_rules:
 tenant_id |     scenario     | version
-----------+------------------+---------
         1 | friendly_chat    |       1
         1 | high_quality     |       1
         1 | intent_recognize |       1
         1 | jev_choice       |       1
         1 | long_summary     |       1
         1 | low_cost         |       1
         1 | objection        |       1
         1 | sop_reply        |       1
(8 rows)

llm_providers: platform-sensenova | quality_score=0.96 | source=platform
落库失败日志条数: 0
```

**重启后复核**（`docker restart mtk-user-server` → 容器内 md5 仍 `c22cef84`）：

```
GET /api/llm/scene-routing → {"scenario":"high_quality","provider":"default",
  "fallbacks":["platform-sensenova"],...,"version":1}
```

⇒ 路由**重启后仍在**。这正是原缺陷的核心（改完只进内存、重启全丢），
现在彻底闭环。

## Residual Risks

- 发版后需在后台**重配一次**场景路由 —— 修复前的配置只进了内存、
  `llm_routing_rules` 0 行，重启即丢，不会因为代码上线而自动恢复。
  （本次已在发版后重配并复核落库。）
- 发版须交叉编译 + 重建镜像（本地 darwin/arm64 二进制在容器里会 exec format error；
  只 `docker compose up -d` 不改镜像时会跑旧二进制）。
- 线上二进制会被并行泳道覆盖（2026-10-22 实测：发版后 2 分钟被另一条泳道
  从更旧 base 编译的版本覆盖）。发版后必须回读容器内 md5 + 自己那条修复的
  特征日志，不能只看自己那次的输出。
- 平台端 provider 的 `api_key` 在平台库**明文存储**（平台库无加密层，
  与商户端 `enc:v1:` 密文不同）。本次沿用现状未改加密策略；
  若后续要加密平台侧密钥，需另评估 `llm_provider_bindings.api_key` 的迁移。
