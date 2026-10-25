# RELEASE_PREPARATION §5.4 / §5.5 实跑结果（本机只读预检）

- 执行时间：2026-10-24 23:00 — 2026-10-25 00:05（CST）
- 执行对象：[docs/RELEASE_PREPARATION.md](../RELEASE_PREPARATION.md) §5.4「启动命令」与 §5.5「验收」（原文未改一字）
- 执行方式：只读 / 幂等。会写库、改配置、需重启的步骤一律未执行，逐条登记在 §4
- 产物目录：`/tmp/w8-release-preflight/`（24 份，索引见该目录 `00-index.txt`）
- 边界：未改产品代码、未改被核查文档、未 commit / push；未 kill / 重启任何在跑服务；未写共享库；
  全篇不落凭证明文（口令 / 密钥只报"是否设置 + 长度档位"）

---

## 0. 执行环境与"跑的是哪一版"

| 项 | 实测值 | 证据 |
|---|---|---|
| 用户端进程 | PID 51764，PPID 1，启动于 2026-10-22 22:55:29 | `01-process-identity.txt` |
| 用户端二进制 | `/tmp/hvtask3-api`（96,639,090 B，mtime 2026-10-22 22:55） | `01-process-identity.txt` |
| 进程 CWD | `…/hivemtk/hivemtk/user-server`（= 文档要求） | `lsof -a -p 51764 -d cwd` |
| 进程内嵌 VCS 身份 | `vcs.revision=ebca55b9289aea4fc60694d5af9f891427109855`、`vcs.time=2026-10-22T13:10:20Z`、`vcs.modified=true` | `go version -m /tmp/hvtask3-api` |
| 仓库 HEAD（开跑时） | `28b7bdee` 2026-10-24 22:39:59 `fix(api-smoke): …` | 会话首条 `git log -1` |
| 仓库 HEAD（收尾时） | `1ab95e00` 2026-10-24 23:23:58 `fix(ui-smoke): …` | `02-binary-vs-head.txt` |
| 二进制 ↔ HEAD 距离 | `ebca55b9..HEAD` = **118 笔**；开跑时的 HEAD 为 **117 笔** | `git rev-list --count` |
| 平台端进程 | PID 92045，`/tmp/hv-platform-api`，`vcs.revision=379d99a5`，启动 2026-10-22 16:29，CWD=`hivemtk-platform/platform-server` | `02-binary-vs-head.txt`、`07-platform-startup-not-executed.txt` |
| DB | `127.0.0.1:8202`（容器 `mtk-postgres`，pgvector/pg15，healthy，`user_db` = 796 MB，public 表 395 张） | `13-db-schema.txt` |
| Redis / 平台 PG | `127.0.0.1:8203`（healthy，但本实例 `REDIS_HOST` 未设 ⇒ `not_configured`）/ `127.0.0.1:8201`（healthy） | `05-listen-and-log-redirect.txt`、`10-health-endpoints.txt` |

### 版本判定结论

1. **在跑的是 2026-10-22 21:10 那版（`ebca55b9`）加未提交改动的构建**，不是 HEAD，也不是 `user-server/bin/`
   下任何一份二进制（`bin/user-server` mtime 09-28，`bin/user-server-linux-amd64` 10-23）。
   按 mtime 与 `vcs.revision` 双证据，二进制**陈旧约 2 天、落后 HEAD 117–118 笔提交**。
2. **DB 架构版本反而领先在跑二进制**：`migration_records` 里 `v3.85.0`（`kb_item_bindings 成员表`）
   的 `executed_at` 为 **2026-10-24 18:57:49**，晚于二进制启动时间；且该迁移文件在工作树里是**未跟踪文件**
   （`?? user-server/internal/migration/migrations/v3_85_0_kb_item_binding_migration.go`），
   HEAD 跟踪的迁移最高只到 `v3.84.0`。⇒ 库是被 10-24 的另一份构建迁移过的，与在跑进程不同代。
3. HEAD 在本次执行期间前进了一笔（`28b7bdee` → `1ab95e00`，共享工作树并行泳道提交）；上表两个读数都保留，
   **不影响任何判定**（两版都远新于二进制）。

---

## 1. §5.4 启动 —— 逐项结果

原文只有两条启动命令 + 一条"别照抄 DEPLOYMENT_GUIDE"的告诫。拆成可执行核验项如下
（"真跑启动"两格因端口占用 / 需停机登记为 SKIP）。

| # | 步骤（命令 / 请求） | rc | 关键输出 | 判定 |
|---|---|---|---|---|
| 4-1 | 根 `.env` 可 source 且四个关键键非空：`bash -c 'set -a; source ./.env; set +a; …'` | 0 | `MASTER_KEY: set len>=32`、`JWT_SECRET: set len>=32`、`FIELD_ENCRYPTION_KEY: set len>=32`、`POSTGRES_PASSWORD: set len>=32` | PASS |
| 4-2 | `user-server/.env` 可 source：同法检查 | 0 | `USER_JWT_SECRET / FIELD_ENCRYPTION_KEY / POSTGRES_PASSWORD / VISITOR_TOKEN_SECRET` 均 `len>=32` | PASS |
| 4-3 | 进程 CWD 必须是 `user-server`：`lsof -a -p 51764 -d cwd -Fn` | 0 | `n…/hivemtk/hivemtk/user-server` | PASS |
| 4-4 | 可执行文件应为 `../user-server/bin/user-server`：`lsof -a -p 51764 -d txt -Fn` | 0 | `n/private/tmp/hvtask3-api` | **FAIL**（二进制路径与文档不符） |
| 4-5 | stdout/stderr 应落 `logs/user-server.log`：`lsof -a -p 51764 -d 1,2 -Fn` | 0 | `f1 → /private/tmp/hv-on.log`、`f2 → 同`（文件已 1.67 GB，无轮转） | **FAIL**（日志落点与文档不符） |
| 4-6 | 端口 8204 监听：`lsof -nP -iTCP -sTCP:LISTEN` | 0 | `hvtask3-a 51764 … TCP *:8204 (LISTEN)` | PASS |
| 4-7 | 根 `.env` 是否真注入进程（`ps eww` 初始 env）：`MASTER_KEY/JWT_SECRET/FIELD_ENCRYPTION_KEY` | 0 | 三者 `=0`（未出现）；`POSTGRES_PASSWORD=1` | **FAIL**（未按 §5.4 注入；JWT/字段加密键疑经 `LoadDotEnv(user-server/.env)` 运行期补齐） |
| 4-8 | 旗标与 `.env` 一致性：`PLATFORM_ENABLED` / `TENANT_MODE` | 0 | 进程 `true` / `on`；根 `.env` 为 `false` / `off` | **FAIL**（启动时被覆盖，非文档命令） |
| 4-9 | `MASTER_KEY` 缺失时非 dev 姿态强制退出（致命分支应被触发/验证） | — | 进程 `APP_ENV=development`、`GIN_MODE=release` | SKIP（dev 姿态下致命校验不会触发；混合姿态本身需拍板） |
| 4-10 | 按 §5.4 真跑用户端启动命令 | — | 8204 被 PID 51764 占用 | SKIP（需停机窗口，见 `06-…txt`） |
| 4-11 | 按 §5.4 真跑平台端启动命令 | — | 8205 被 PID 92045 占用 | SKIP（同上，见 `07-…txt`） |
| 4-12 | 二进制与 HEAD 同代 | 0 | `vcs.revision=ebca55b9`、`vcs.modified=true`；落后 117 笔 | **FAIL**（陈旧） |
| 4-13 | 静态资源随进程可用：`GET /`、`GET /assets/AISuggestion-CraL3M3m.js` | 0 | `/` → 200/3175 B；asset → 200/3181 B（与磁盘 3175/3181 逐字节一致） | PASS |

**§5.4 计数：PASS 5 / FAIL 5 / SKIP 3（共 13）**

---

## 2. §5.5 验收 —— 逐项结果

| # | 步骤（命令 / 请求） | rc | 关键输出 | 判定 |
|---|---|---|---|---|
| 5-1 | `curl 127.0.0.1:8204/api/health` | 0 | `http=200` `{"code":0,"message":"ok","data":{"status":"ok"}}` | PASS |
| 5-2 | `curl 127.0.0.1:8204/healthz` | 0 | `http=200` `{"status":"alive"}` | PASS |
| 5-3 | `curl 127.0.0.1:8204/readyz` | 0 | `http=503` `{"code":50305,"reason":"inference: no healthy LLM provider","status":"not_ready"}` | **FAIL**（未就绪） |
| 5-4 | `curl 127.0.0.1:8205/health`（平台端） | 0 | `http=200` `{"status":"alive"}` | PASS |
| 5-5 | `curl 127.0.0.1:8204/api/system/init-status` | 0 | `http=200` `{"has_admin":true,"initialized":true,"state":"INITIALIZED","version":"unknown"}` | PASS（`version:unknown` 与 `install.lock` 的 `"version":""` 一致，属 §5.6 已知形态） |
| 5-6 | G0 步 1：`GET ${BASE}/health` 必须 200 | 0 | `http=200`，但体为 `code=50301 "service degraded"`（`inference: down`、`redis: not_configured`、`database: ok`、`embedding: up`） | PASS（判据只看 200；降级原因见 §4） |
| 5-7 | G0 步 2：`knowledge_bases` + `agent_kb_bindings` 都在 | 0 | `kb_table=1`、`binding_table=1` | PASS |
| 5-8 | G0 步 3：两表索引数 ≥10 | 0 | `kb_indexes=23`（文档 2026-09-22 读数为 12，下界 10） | PASS |
| 5-9 | G0 步 4：登录 → 建 KB → 删 | — | 会写共享库（`kb_code` 非 partial 唯一索引，软删占位） | SKIP（见 `22-…txt`） |
| 5-10 | G0 步 5：`go test ./internal/repository ./internal/service ./test/... -timeout 2500s` | — | 文档自述 ≥45 min；需独占测试库 | SKIP（见 `22-…txt`） |
| 5-11 | 容器：`docker ps --filter name=mtk-` | 0 | `mtk-postgres` / `mtk-redis` / `mtk-platform-postgres` 全部 `Up … (healthy)` | PASS |
| 5-12 | 日志：近 3 MB 有无 panic / FATAL / error | 0 | `panic=0`、`FATAL=0`、`"level":"error"` = 2（两条均为本次口令探针触发的 `login_events` 租户上下文错误，见 §4） | PASS（无崩溃级日志） |
| 5-13 | `bash scripts/check-admin-default-credential.sh` | 0 | `probe-1: 口令长度=11 http=401`；`RESULT: GREEN —— 公开口令不认` | PASS |
| 5-14 | `make db-backup` 出基线备份 | 0 | 等价命令 `pg_dump -U admin -p 8202 user_db` → `/tmp/…/backup_20261024_234742.sql`，480,901,538 B，395 条 `CREATE TABLE`，`-- PostgreSQL database dump complete` 在册；8.09 s | PASS（**注**：原目标会把 480 MB 写到仓库根，本次改落 `/tmp`，命令与参数一致） |
| 5-15 | 静态资源托管 | 0 | `/` 200/3175；`/assets/<asset>.js` 200/3181 与磁盘一致 | PASS |
| 5-16 | i18n 资源存在性 | 0 | `user-web/src/i18n/locales/` 9 个语言 JSON（ar/de/en/es/fr/ja/pt/ru/zh），逐个 `json.load` 通过，均 1215 键 | PASS |
| 5-17 | 迁移状态查询（`migration_records` vs 代码） | 0 | 117 行全 `completed`；最新 `v3.85.0` 执行于 **2026-10-24 18:57:49**；HEAD 跟踪的迁移最高 `v3.84.0`；`v3_85_0*.go` 为未跟踪文件 | **FAIL**（DB schema 领先在跑二进制 / 领先 HEAD） |

**§5.5 计数：PASS 13 / FAIL 2 / SKIP 2（共 17）**

---

## 3. 关键 FAIL 明细

| 编号 | FAIL | 证据 | 性质 |
|---|---|---|---|
| F1 | 在跑二进制陈旧：`vcs.revision=ebca55b9`、`modified=true`、mtime 10-22 22:55；HEAD 10-24 23:23；落后 117–118 笔 | `02-binary-…` | 发布验收对象不对（不是待发布的那版） |
| F2 | `/readyz` 503：`inference: no healthy LLM provider`；`/health` 同时报 `50301 degraded`（`redis: not_configured`） | `10-health-endpoints.txt` | 环境缺失（推理栈未就绪）；若照此上 LB 会被判不健康 |
| F3 | 启动形态不符 §5.4：exe=`/tmp/hvtask3-api`（非 `bin/user-server`）、日志=`/tmp/hv-on.log`（非 `logs/user-server.log`）、进程初始 env 缺 `MASTER_KEY`/`JWT_SECRET`/`FIELD_ENCRYPTION_KEY`、`PLATFORM_ENABLED`/`TENANT_MODE` 与 `.env` 相反 | `01/04/05` | 验收得到的"绿"不代表文档命令能起得来 |
| F4 | 迁移版本偏斜：DB 已执行 `v3.85.0`（10-24 18:57）且该迁移文件不在库里；HEAD 跟踪最高 `v3.84.0` | `14-migration-state.txt` | 发布带与库状态不一致，回滚/重放会踩 |

## 4. 未执行项清单与阻塞原因

| 未执行项 | 类别 | 阻塞原因（具体到"需要什么"） |
|---|---|---|
| §5.4 用户端启动命令 | 需重启 | 8204 被 PID 51764 占用；需**停机窗口**停旧进程 + 一份与 HEAD 同代的二进制（现有 `bin/` 内无此件） |
| §5.4 平台端启动命令 | 需重启 | 8205 被 PID 92045 占用；需停机窗口 + `platform-server/bin/platform-server` 现构建 |
| §5.5 G0 步 4（登录→建 KB→删） | 写库 | 需**专用验收库或可回滚快照** + 一次可用登录凭据；共享库写入会留下 `kb_code` 永久占位 |
| §5.5 G0 步 5（go test 全量） | 时长/资源 | 需 ≥45 min 独占窗口 + 独占 `POSTGRES_TEST_PORT`（同机并行跑会互抢测试库） |
| §5.5 `make db-backup`（原命令） | 产物范围 | 原目标把 480 MB 落到仓库根，超出本次"只落 `/tmp`"的边界 ⇒ 以等价 `pg_dump` 执行并核完成标记 |
| MASTER_KEY 非 dev 强校验 | 需重启 | 现进程 `APP_ENV=development`，致命分支不会开火；需以非 dev 姿态重启一次才能验证 |

## 5. 非门禁观察（不影响上表判定，但应进发布记录）

1. **`login_events` 租户上下文错误**：本次 1 次口令探针（23:46:03）在日志里留下两条 error ——
   「查询最近失败次数失败 / 写入 login_events 失败：表 login_events 需要租户上下文但 ctx 未绑定」。
   ⇒ `TENANT_MODE=on` 下登录事件的读写都被租户门拒绝，**防爆破计数与登录审计实际不落库**。
   ~~建议单独开条确认~~ **〔2026-10-25 已修复〕**：成功路径此前已修（resp.TenantID 回填）；本日补齐**失败登录腿**——`recordLoginRiskAsync` 无租户可绑时落 `tenant.WithSystemScope`（登录遥测=部署级安全数据，与 C-5 的 401 遥测同裁决），回归用例 `TestAuthController_Login_FailedAttempt_RecordsViaSystemScope` 在案。注意复跑验收时读数口径要先切回 off 模式（on 模式下验收方自己的 Count 也会被闸拒，曾把"行已落库"误读成"零行"）。
2. **PWA 入口回落 SPA**：`/manifest.webmanifest`、`/favicon.svg` 返回 200 但内容是 `index.html`（3175 B）。
   `embed_static_routes.go` 只挂了 `/assets`、`/favicon.ico`、`/logo.png`，其余走 `NoRoute` → SPA。
3. **前端与后端不同代**：`user-web/dist` 的 `buildId=28b7bdee`（=开跑时 HEAD），`buildTime 2026-10-24T15:21:19Z`；
   后端是 10-22 的 `ebca55b9`。
4. **日志无轮转**：`/tmp/hv-on.log` 已 1.67 GB 单文件。
5. **端口面**：8204 监听在 `*:8204`（全接口），未按 §5.3 收到 `127.0.0.1`。
6. **Redis 未接入**：`REDIS_HOST` 未设 ⇒ `health.redis=not_configured`，缓存/限流走降级路径；
   容器 `mtk-redis` 在跑但本实例没用上。

---

## 6. 结论

**不可放行。**

理由：以本机实例做"发布前验收"，四项关键 FAIL 都落在"验收对象 / 环境"上而不是文案上 ——
跑的不是待发布那版二进制（F1、F4），就绪探针红（F2），而 §5.4 的启动命令在这台机上**从未按原样跑过**（F3），
所以上表里的 PASS 只能证明"10-22 那版二进制在 dev 姿态下能应答"，不能证明"按 §5.4 起得来、起来是 ready"。

放行前最小动作（按序）：

1. 停机窗口内按 §5.4 原命令（`set -a && source .env` + `nohup ../user-server/bin/user-server > logs/user-server.log`）
   用**与 HEAD 同代**的二进制重启，`APP_ENV` 置非 dev 以让 `MASTER_KEY` 强校验真正开火；
2. 先把 `v3.85.0` 迁移文件入库并确认发布带包含它（否则库与代码继续错位）；
3. 起推理栈（`inference.llm.base_url` 本机回环 8787 或网关 8207）→ `/readyz` 必须 200；决定 `REDIS_HOST` 是否接入；
4. 以上就绪后，把本表 §5.4、§5.5 整表复跑一遍（含 G0 步 4/5，在专用验收库与独占窗口），产物另存一版；
5. `login_events` 租户上下文错误单独立项，别让它在发布窗口里被"复跑绿了"掩盖。

---

## 7. 产物索引

全部在 `/tmp/w8-release-preflight/`：`00-index.txt` 为总索引；
`01`–`07` 对应 §5.4，`10`–`22` 对应 §5.5（其中 `backup_20261024_234742.sql` 为 480 MB 基线备份）。
