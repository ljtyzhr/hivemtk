# API 全量测试进度（任务三）

## 环境

| 项 | 值 |
|---|---|
| 仓库根 | `/Users/xiaofang/Documents/www/go/hivemtk/hivemtk` |
| 后端 | `user-server/`（Go + Gin，五层架构） |
| 前端 API 源 | `user-web/src/api/*.js`（110 个文件） |
| 路由清单 | `docs/api-list.md`（1028 处调用点，993 条唯一 method+url） |
| SaaS 实例 | `http://127.0.0.1:8204`，`TENANT_MODE=on` |
| 独立部署实例 | `http://127.0.0.1:8304`，`TENANT_MODE=off` |
| 数据库 | `127.0.0.1:8202` / `user_db` / 用户 `admin`，口令存 `/tmp/hv_dbpass` |
| 登录账号 | `uit_admin` / `Hive@Test123`（id=26，tenant_id=1；口令已在 DB 中重置为 bcrypt 哈希） |
| Token 缓存 | `/tmp/hv_token_8204`、`/tmp/hv_token_8304` |
| 启动环境变量 | `APP_ENV=development`（因 `.env` 的 `MASTER_KEY` 为空，非 development 会 `os.Exit(1)`） |

启动命令（两个实例）：

```bash
cd user-server
set +H; CPW=$(cat /tmp/hv_dbpass)
nohup env PORT=8204 TENANT_MODE=on  GIN_MODE=release APP_ENV=development \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" go run ./cmd/api > /tmp/hv-on.log 2>&1 &
nohup env PORT=8304 TENANT_MODE=off GIN_MODE=release APP_ENV=development \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" go run ./cmd/api > /tmp/hv-off.log 2>&1 &
```

重新取 token：

```bash
for p in 8204 8304; do
  curl -s -X POST http://127.0.0.1:$p/api/auth/login -H 'Content-Type: application/json' \
    -d '{"username":"uit_admin","password":"Hive@Test123"}' \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])' > /tmp/hv_token_$p
done
```

## 测试规则

- 每个路由手工构造参数、手工 curl（禁止脚本批量循环）。
- 两个实例都过：SaaS（8204）与独立部署（8304）。
- 响应分三类判定：①调用错误（4xx/5xx、路由未注册、参数名不符）②业务逻辑错误（200 但数据错/租户串号/空实现）③通过。
- 发现问题立即修复，修完回归同一路由。
- 路由清单按 `docs/api-list.md` 文件顺序推进，一个文件一个文件过。

## 已完成

| 文件 | 路由数 | 状态 | 备注 |
|---|---|---|---|
| （待填） | | | |

## 已发现问题

| # | 路由 | 现象 | 根因 | 状态 |
|---|---|---|---|---|
| 1 | 后台任务（非路由） | SaaS 模式下 `sop_outbox` / `reach_dispatcher` 等 worker 日志报 tenant ctx 未绑定 | 后台 goroutine 未注入 system scope 或按租户遍历 | 待确认 |
| 2 | `GET /api/ab-experiments` | SaaS(8204) ⇒ 500 `[tenant] 表 ab_experiments 需要租户上下文但 ctx 未绑定（query）`；独立(8304) ⇒ 200 | on 模式请求链已过 `TenantMiddleware`（否则会是 401），但 handler 侧把 ctx 换掉了（repo 用 `context.Background()` 或没 `db.WithContext(c.Request.Context())`） | 待定位 |

## 上下文管理纪律（避免反复压缩停顿）

1. 不用 `read` 工具读 `user-web/` 与仓库根下的文件——目录上下文会自动注入 4 个 README（≈15K token/次）。读源码用 `grep` 工具或 shell + `head -c` 限长。
2. 所有 shell 输出必须限长（`| head -c 2000`）；禁止 `ls` 大目录全量输出。
3. 已消费完的工具输出一律 `ctx_reduce` 打标；已结束的区段用 ACP `compress` 折叠。
4. 状态只认本文件（`docs/api-test-progress.md`），压缩后不依赖聊天记录。
5. 一轮多测几条路由，减少回合数 = 减少检查点重注入。
