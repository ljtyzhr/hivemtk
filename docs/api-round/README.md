# API 逐路由实测 · 子任务协议（并行泳道）

主任务：`docs/api-verification-round-2026-10-26.md`。清单：`api-list.md`。
本目录是**并行泳道的交接区**：每条泳道只写自己的 `*.rows.md` / `*.findings.md`，
**不要直接改 `api-list.md`**（避免多泳道同时写同一文件互相覆盖），由主线合并。

## 环境

| 形态 | Base | 说明 |
|---|---|---|
| 私域独立部署 | `http://127.0.0.1:8324` | `TENANT_MODE=off` |
| SaaS 多租户 | `http://127.0.0.1:8325` | `TENANT_MODE=on`，同一隔离库 `user_db_api` |

- 令牌：`/private/var/folders/xw/pd_rr_z54w38gq41yfm8h7sm0000gn/T/opencode/hvapi/tok`（`uit_admin`，tenant 1，admin）。
- DB 只读口令查询：`PGPASSWORD=<见本机 .env 的 POSTGRES_PASSWORD>! psql -h 127.0.0.1 -p 8202 -U admin -d user_db_api`
- 源码：`user-server/`（Go）、`user-web/src/api/*.js`。
- 本机 LLM 未起：真要调 LLM 的端点会 500，属环境前提，记 `[x]` 并注明。

## 每条路由的动作（**禁止写脚本批量发请求**）

1. 读前端 api 文件，按调用点实参形状构造 curl（path 取真实 id；body 用前端真实字段）。
2. 两条腿各发一次（8324、8325），看 HTTP 状态 + `code` + `message` + 关键 `data`。
3. 判读：`code=0` 通过；非 0 先判「我构造错了」还是「代码错」。业务错误要指出代码位置。
4. 确认是缺陷：马上改源码（前端契约错位改前端；后端逻辑错改后端），改完回归同一路由。
5. 写行交接文件（见下）。**不要**跑 `scripts/curl_sweep.py` / `mark_api_list.py` 之类批量脚本。

## 交接文件格式

`docs/api-round/<源文件名>.rows.md`：逐行贴**完整的 api-list 表格行**，只把状态/响应两格填好，例如

```
| 42 | [x] | POST | `/api/asset-bundle` | 用户端 | body: data | 200 code=0 id=9 | `src/api/assetBundle.js` | 4 |
```

（`#`、方法、路径、档位、参数、源文件、行号必须与 `api-list.md` 现有行**逐字一致**，只改第 2 格与第 7 格。）

`docs/api-round/<源文件名>.findings.md`：发现的缺陷/待判项，含路由、形态、现象、判定、处置。

## 纪律

- 不碰他人泳道的表/路由；测试数据用 `api26_` 前缀，用完删；不得改写既有业务行（确实需要时先备份原值并恢复）。
- 破坏性端点（删库/清空/恢复/重启/群发消息/真实外呼）先读 handler；只验证入参校验与权限，不做真实破坏/外发。
- 每条路由两条腿都跑过才能写 `[x]`。
