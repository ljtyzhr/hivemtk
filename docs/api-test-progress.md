# API 全量测试进度（任务三）

## 环境

| 项 | 值 |
|---|---|
| 仓库根 | `/Users/xiaofang/Documents/www/go/hivemtk/hivemtk` |
| 后端 | `user-server/`（Go + Gin，五层架构） |
| 前端 API 源 | `user-web/src/api/*.js`（113 个文件） |
| 路由清单 | `api-list.md`（1028 条调用点，含状态列） |
| SaaS 实例 | `http://127.0.0.1:8204`，`TENANT_MODE=on`，`PLATFORM_ENABLED=true` |
| 独立部署实例 | `http://127.0.0.1:8305`，`TENANT_MODE=off`，`PLATFORM_ENABLED` 未开 |
| 平台服务 | `http://127.0.0.1:8205`（`hivemtk-platform/platform-server`，独立仓库） |
| 数据库 | `127.0.0.1:8202` / `user_db` / 用户 `admin`，口令存 `/tmp/hv_dbpass` |
| 平台库 | `127.0.0.1:8201` / `platform_db`，口令取 `hivemtk-platform/.env` |
| 登录账号 | `uit_admin` / `Hive@Test123`（id=26，tenant_id=1） |
| Token 缓存 | `/tmp/hv_token_8204`、`/tmp/hv_token_8304` |
| 启动环境变量 | `APP_ENV=development`（因 `.env` 的 `MASTER_KEY` 为空，非 development 会 `os.Exit(1)`） |

### 启动命令（两个实例 + 平台）

```bash
cd user-server
set +H; CPW=$(cat /tmp/hv_dbpass)
MS=$(grep '^MERCHANT_API_SECRET=' ../.env | cut -d= -f2-)
PAP=$(grep '^PLATFORM_ADMIN_PASSWORD=' ../.env | cut -d= -f2-)

# SaaS 多租户实例
nohup env PORT=8204 TENANT_MODE=on GIN_MODE=release APP_ENV=development \
  PLATFORM_ENABLED=true PLATFORM_API_HOST=http://127.0.0.1:8205 \
  MERCHANT_API_SECRET="$MS" PLATFORM_ADMIN_PASSWORD="$PAP" \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" /tmp/hv-api-new > /tmp/hv-on.log 2>&1 &

# 私域独立部署实例（端口 8305：8304 已被另一条并行 lane 的 api-standalone 占用）
nohup env PORT=8305 TENANT_MODE=off GIN_MODE=release APP_ENV=development \
  DB_HOST=127.0.0.1 DB_PORT=8202 POSTGRES_PASSWORD="$CPW" /tmp/hv-api-new > /tmp/hv-off.log 2>&1 &

# 平台服务（另仓 hivemtk-platform/platform-server）
cd ../../hivemtk-platform/platform-server
PP=$(grep '^POSTGRES_PASSWORD=' ../.env | cut -d= -f2-)
nohup env PORT=8205 GIN_MODE=release POSTGRES_PASSWORD="$PP" \
  PLATFORM_ADMIN_PASSWORD="$PAP" MERCHANT_API_SECRET="$MS" /tmp/hv-platform-api > /tmp/hv-platform.log 2>&1 &
```

**坑位备忘**：
- `PLATFORM_ENABLED` 只认**进程环境变量**，不读 `.env`（`.env` 里是 `false`）。
  SaaS 实例要联平台必须显式 `export PLATFORM_ENABLED=true`，
  且 `MERCHANT_API_SECRET` / `PLATFORM_ADMIN_PASSWORD` 必须一起注入，
  否则 `LoadPlatform` 报「缺少必填字段 secret」，`PlatformCfg` 为 nil。
- **不要用 `go run ./cmd/api` 起常驻实例**：`go run` 是父进程，杀掉编译出来的子进程后
  它会自己重新编译拉起，端口被抢、`ps` 里看到一堆同名孤儿进程。
  用 `go build -o /tmp/hv-api-new ./cmd/api` 再直接跑二进制。
- 8204/8214/8304 三个实例曾同时在跑（8214 是任务四的页面巡检实例），
  拉起前先 `lsof -nP -iTCP:<port> -sTCP:LISTEN` 确认端口归属。

重新取 token：

```bash
for p in 8204 8305; do
  curl -s -X POST http://127.0.0.1:$p/api/auth/login -H 'Content-Type: application/json' \
    -d '{"username":"uit_admin","password":"Hive@Test123"}' \
    | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])' > /tmp/hv_token_$p
done
```

## 测试规则

- 每个路由手工构造参数、手工 curl（禁止脚本批量循环）。
- 两个实例都过：SaaS（8204）与独立部署（8305）。
- 响应分三类判定：①调用错误（4xx/5xx、路由未注册、参数名不符）②业务逻辑错误（200 但数据错/租户串号/空实现）③通过。
- 发现问题立即修复，修完回归同一路由。
- 路由清单按 `api-list.md` 文件顺序推进，一个文件一个文件过。

### 共用同一测试库带来的判定干扰（重要）

两个实例连的是**同一个 user_db**，且 `unique` 约束跨实例生效。因此：

- **写操作**（create/bind/update）必须用**带实例标识的唯一值**（如 `apitest_agent_8204`），
  否则第二个实例会撞唯一键报 400，看起来像 bug，实为测试数据互相踩。
- **删操作**（delete/unbind）必须**每个实例各自新建一份夹具**再删：
  若两个实例删同一条记录，先删的那个成功、后删的必然 404（record not found），
  这不是缺陷。
- 断言「查不到」时先确认这条记录不是被另一个实例删掉的。

## 已完成

| 文件 | 路由数 | 状态 | 备注 |
|---|---|---|---|
| abExperiment.js | 11 | ✅ 全通过 | 含 start/pause/stop 状态机 draft→running→paused→completed |
| abExperimentPlus.js | 5 | ✅ 全通过 | 上一轮已完成 |
| agentKBBinding.js | 5 | ✅ 全通过 | bind/unbind 双实例各自建夹具 |
| aiAgent.js | 9 | ✅ 全通过 | test 端点 8204 走「AI 服务不可用」兜底、8304 走话术匹配，均 code=0 |
| aiTool.js | 7 | ✅ 全通过 | 绑账号时「同工具单 primary」规则生效，非缺陷 |
| assetBundle.js | 14 | ✅ 全通过 | **修复了 submit-platform 的 :id 语义 bug**（见下） |
| browserAutomation.js | 8 | ✅ 全通过 | 修了 cron 的 `enabled:false` 被吞（见问题 #6） |
| tiktokCard.js / douyinCard.js / xiaohongshuCard.js / xianyuCard.js / kuaishouCard.js | 31 | ✅ 全通过 | 修了 `is_active:false` 被吞 + tiktok 短链入参名不符（见问题 #7/#8） |
| assetMarket.js | 11 | ✅ 全通过 | 平台侧数据污染（见问题 #5），user-server 侧行为正确 |
| backup.js | 7 | ✅ 全通过 | 备份任务真跑完并产出 zip；restore 任务真跑完 |
| sms.js | 20 | ✅ 全通过 | 修了租户改写 + 渠道错误被误判成「记录不存在」（见问题 #5/#9/#10） |
| knowledge.js | 27 | ✅ 全通过 | 修了 OpenAPI 源更新改写租户（见问题 #13）；文档 CRUD/进度/分块/统计全通 |
| reachPipeline.js | 16 | ✅ 全通过 | 管道与任务的 CRUD + 状态机（pause/resume/archive/cancel/retry/execute）+ 限流重置全通 |
| featureFlags.js | 11 | ✅ 全通过 | 增删改查 + enable/disable/rollout/audit/eval-log 全通（tenant_id 保持 1） |
| sessionTags / quickReplies / dashboards / domainPool / whatsapp 模板 / community 群组 | 18 | ✅ 全通过 | 更新类路由逐条验过，tenant_id 均保持 1，未出现同族改写 |
| telegram.js | 13 | ✅ 全通过 | 账号/闸门全通；**该模块本就把 bot_token 掩码返回**（`8602****TJ_k`），是同族做法的正面样板 |
| users.js（部分） | — | ✅ | 含 `POST /api/auth/mfa/setup`：**第二次**调用曾回 500（问题 #14），已修 |

## 已发现问题

| # | 路由 | 现象 | 根因 | 状态 |
|---|---|---|---|---|
| 1 | 后台任务（非路由） | SaaS 模式下 `sop_outbox` / `reach_dispatcher` 等 worker 日志报 tenant ctx 未绑定 | 后台 goroutine 未注入 system scope 或按租户遍历 | ✅ 已修复（上一轮）：日志显示 worker 已按 tenant_id 逐个遍历（11→17） |
| 2 | `GET /api/ab-experiments` | SaaS(8204) ⇒ 500 租户上下文未绑定；独立(8304) ⇒ 200 | handler 侧 ctx 被换掉 | ✅ 已修复（上一轮）：两实例现均 200 |
| 3 | `POST /api/asset-bundle/{id}/submit-platform` | 前端传数字主键 `bundle.id`，后端把它当 `asset_id` 查 → 100% 报 `record not found`，上架链路完全不可用 | handler 直接把 `:id` 原样当 asset_id 传给 `SubmitToPlatform`；兄弟路由（publish/archive/enable/disable）都把 `:id` 解析成数字主键，只有这一条不解析 | ✅ **本轮已修复**：新增 `resolveAssetID` 做「数字主键 → asset_id」归一化，非数字入参按 slug 兼容；补 3 条回归测试（`user-server/internal/controller/asset_bundle_submit_platform_test.go`），已验证「回退修复即红」 |
| 4 | `PUT /api/asset-bundle/{id}` | 响应体 `tenant_id` 回显 0（create 时是 1） | handler 用新建 model 回显，未带 TenantID | ⚠️ 轻微（仅响应回显，DB 落库 tenant_id=1 正确）；待第五步统一评估是否修 |
| 5 | `POST /api/v1/asset-market/purchase`（跨仓，平台侧） | 8204 ⇒ code 6004「拉取平台数据失败」；平台日志：`资产数据完整性校验失败: SHA256 mismatch (expected 6b114b06…, got 1eb0bfb9…)` | **平台库 `asset_data.data` 被改写过而 `asset_versions.file_sha256` 没同步重算**（数据里混进了「桥接测试900739N」等桥接 e2e 残留），平台 `SyncPull` 的完整性校验据此拒绝放行。平台仓库 `hivemtk-platform` 全部代码路径**没有**「创建后改写 asset_data」的写法 ⇒ 属共享测试库的**测试数据污染**，不是任一侧的代码缺陷 | ✅ user-server 侧行为正确（购买已在平台落单 id=35，再如实报出平台的完整性失败，没有伪成功）；8304 正确 403。**待第五步处理**：需修平台测试数据（重算 sha 或重置种子），平台仓不在本任务仓库范围 |
| 6 | `POST /api/browser-automation/cron` | 传 `enabled:false` 建出来的触发器**落库成 enabled=true**，而且被注册进调度器 ⇒ 用户明确说"先别跑"的定时任务照样会开火 | 模型 `BrowserCronTrigger.Enabled` 是**非指针 bool** 且带 gorm `default:true`：GORM 认定「零值 = 没赋值」，把 false 从 INSERT 里剔除交给列默认值（true）兜底。service.applySchedule 读到的还是内存里的 true | ✅ **本轮已修复**：模型层去掉 `default:true`（缺省语义留在 service 层：`CreateCronReq.Enabled` 是 `*bool`，nil ⇒ 默认开）；补 2 条回归测试（`user-server/internal/browser_automation/service/cron_enabled_default_test.go`），已验证「回退修复即红」；API 复测 `enabled:false` ⇒ 落库 `f`、next_run_at 为 NULL |
| 7 | `POST /api/{douyin,xiaohongshu,xianyu,kuaishou,tiktok}-card` | 传 `is_active:false` 建出来的卡片**落库成 is_active=true**，响应体也回 true | 与 #6 同源：5 张卡表的 `IsActive` 都是**非指针 bool** + gorm `default:true`，仓储走结构体形态 Create ⇒ 显式 false 被列默认值 true 吞掉。前端弹窗里有「激活」开关，关掉再保存会静默变回激活 | ✅ **本轮已修复**：5 个模型去 `default:true`；5 个 DTO 的 Create/Update `IsActive` 改 `*bool`；service 用 `dto.BoolPtrOrDefaultTrue`（缺省即开）/ 更新时 nil 保留原值（PATCH 语义）。补 3 条回归测试（`user-server/internal/service/card_is_active_default_test.go`），已验证「回退修复即红」 |
| 8 | `POST /api/tiktok-card/generate-short-link` | 前端发 `{ card_id: 123 }`（snake_case），后端绑的是 `json:"cardId"`（camelCase）⇒ **前端这一条永远 400**（binding:"required" 找不到 card_id，请求在绑定阶段就被毙掉） | handler 用了内联匿名 struct，键名与前端 API 文件（`user-web/src/api/tiktokCard.js`）不一致。兄弟路由（douyin/xiaohongshu/xianyu/kuaishou）把 id 放**路径**上、不吃 body，所以只有 tiktok 这一条踩到 | ✅ **本轮已修复**：改绑 `card_id`，`cardId` 保留为兼容别名（注意 `required` 标签不能挂在 `card_id` 上，否则只传 cardId 的请求会被绑定阶段毙掉、兼容分支走不到；必填校验挪到绑定之后手工做）。补 2 条回归测试（`user-server/internal/controller/tiktok_card_shortlink_binding_test.go`） |
| 9 | `POST /api/sms/draft/{id}/send`、`POST /api/sms/send`、`POST /api/sms/resend/{id}` | 短信渠道凭证无效时，接口回 **404「记录不存在」**（实际是阿里云回 `Specified access key is not found.`） | 判定"记录不存在"是**按错误文本**做的（`errhttp.isNotFoundError` / `response.ErrorFromDB`：文本含"不存在"或"not found" ⇒ 404），而渠道原文常自带这些词 ⇒ "密钥填错了"被翻译成"记录没了"，把操作者引去查记录而不是改密钥。**三个入口都漏**（send / resend / draft send）——第一次只修了 send，resend 仍在漏 | ✅ **本轮已修复**：(a) service 层把渠道失败包成 `bizerr.Wrap(bizerr.CodePlatformUnavail, ...)`（5xxx 平台类业务码 = 显式声明"这是上游失败"），send 与 resend **两个入口都改**；(b) `bizerr.IsUpstreamFailure` 判据；(c) `errhttp.isNotFoundError` 与 `response.ErrorFromDB` 见到上游失败就不判 not-found，HTTP 走 **502** 且消息带渠道真因。补 6 条回归测试（`internal/pkg/errhttp/upstream_failure_test.go`、`internal/pkg/utils/response/upstream_failure_test.go`、`internal/service/sms_upstream_failure_test.go`） |
| 10 | `POST /api/sms/job/{id}/{pause,resume,stop}` | 状态不对（如"只能暂停运行中的任务"）时回 **500** + 一句"暂停任务失败"，真正的原因被吞掉；客户端会把 500 当服务端故障去重试 | controller 用 `HandleDBError`（DB 错误映射器）处理**业务状态校验**失败。`errhttp.HandleDBError` 对非 not-found 错误一律 500 | ✅ **本轮已修复**：三个状态迁移改用 `HandleServiceError`（同文件 `ResendSms` 早有此先例）⇒ 400 + 真因。同步更新 3 条既有测试的期望（500→400）并注明原因 |
| 11 | `POST /api/sms/config`（以及任何"读旧行→盖回去"的写路径） | SaaS(8204) ⇒ 500 `[tenant] 更新不可改写 tenant_id（上下文租户 1，SET 值 0）`；独立(8304) ⇒ 200 | 仓储写法是"先 First 读出 existing，再把新 model 的 ID 认成 existing.ID，然后 Save"。GORM 的 `Save`（Selects=*）写**全部字段含零值**，而调用方传进来的是**新建的零值 model**（tenant_id=0）⇒ 这一行的租户被改写成 0。多租户模式下 tenant 回调 fail-closed 当场拒掉；独立部署没有这层校验 ⇒ **同一个接口 SaaS 下 500、私域下 200**（能用的那边掩盖了坏的那边） | ✅ **本轮已修复**：Save 之前把库里的 `tenant_id` 带回来。共 6 处同型：4 张短信配置表（sms_configs / sms_aliyun_configs / sms_tencent_configs / sms_huawei_configs）+ `UserRFM`（BatchUpsert）+ `EmailJobMetric`（UpsertJobMetric）。补 2 条回归测试（`internal/repository/sms_tenant_scope_test.go`），已验证「回退修复即红」；API 复测 8204/8305 均 200，落库 tenant_id=1 |
| 13 | `PUT /api/knowledge/openapi/sources/{id}` | SaaS(8204) ⇒ 500 `[tenant] 更新不可改写 tenant_id（上下文租户 1，SET 值 0）`；独立 ⇒ 200 | 与 #11 **同族**：controller 传进来的是**新建的零值 model**（tenant_id=0），`OpenAPIService.UpdateSource` 已经先 `GetByID` 查过存在性却**丢掉了那一行**，repo 走 `Save` ⇒ 零值 tenant_id 一起写进去。另一处同型：`RagConfigRepository.UpsertAccountConfig`（同样 `config.ID = existingConfig.ID` 后 `Save`，同样丢掉 existing 的 tenant_id） | ✅ **本轮已修复**：两处都在 Save 前把库里的 `tenant_id` 带回来。补 2 条回归测试（`internal/service/openapi_tenant_test.go`），已验证「回退修复即红」（回退后 tenant_id 落成 0）；API 复测 8204/8306 均 200、tenant_id=1 |
| 14 | `POST /api/auth/mfa/setup` | **第二次**调用回 500「MFA 设置失败」；私域形态同一接口 200 | 与 #11/#13 **同族**：SetupMFA 传进 repo.Save 的是**新建的零值 model**（tenant_id=0），`Save` 写**全部字段含零值** ⇒ 该行租户被改写成 0。**第一次不报**（库里没这行 ⇒ 走 Create，由隔离 callback 填 tenant_id），所以这条缺陷**只在第二次设置 MFA 时才现形** —— 手工测一次测不出来 | ✅ **本轮已修复**：更新前把库里的 `tenant_id` 带回来。补 1 条回归测试（`internal/service/mfa_tenant_test.go`，连着调两次 SetupMFA），已验证「回退修复即红」（回退后 tenant_id 落成 0）；API 复测 8204 连续两次均 200、库中 tenant_id=1 |
| 12 | `GET /api/wecom/accounts`、`GET /api/wecom/health/accounts` 等读接口 | 响应体里带**明文** `corp_secret` / `agent_secret` / `access_token` / `callback_token` / `encoding_aes_key` ⇒ 任何登录用户都能把商户的企业微信应用密钥、通讯录回调密钥、AES 密钥整个读走 | `WeComAccount` 这个 model 直接序列化给前端。同仓的短信配置那条路是脱敏的（`maskSMSConfig`），这里漏了。**不能只脱敏**：前端编辑弹窗把列表回填的 `corp_secret` 原样提交回来（`user-web/src/views/wecomAccount/List.vue` 的 `openEditDialog` + 提交 payload），只脱敏不认掩码 ⇒ 用户保存一次，真密钥就被覆盖成 `****`，账号直接废掉 | ✅ **本轮已修复**（读写成对）：读侧 `controller/wecom_secret_mask.go` 脱敏（5 个密钥字段，空串保持空、脱敏幂等）；写侧 `service/wecom_secret_mask.go` 认掩码串 ⇒ 保留原值（沿用该 service 已有的"密钥空串=保留原值"口径）。补 8 条回归测试（`internal/controller/wecom_secret_mask_test.go` 6 条 + `internal/service/wecom_secret_mask_test.go` 2 条）。**自查抓到过一次自己写错**：掩码的真实形态是"前缀+末4位"（`****cret`），写侧最初按**全等**比对 `****` ⇒ 认不出来，掩码照样覆盖了真密钥（API 实测复现：DB 变成 `****cret`）。改成**前缀**判定后才真正拦住（`strings.HasPrefix`），并把测试夹具改成真实掩码形态（`WeComSecretMask+"1234"`） |

## 第五步 · 对抗性复核（自审本批改动）

对本批 8 条修复逐条反向攻击，抓到 **2 处自己引入的缺陷**，都已修掉并回归：

| # | 自查发现 | 现象 | 修法 |
|---|---|---|---|
| R1 | `bizerr.IsUpstreamFailure` 按**码段**（5xxx 整段）判上游失败 | 5xxx 段里还有 `CodeInternal(5000)`＝**内部**故障（写库失败之类）。把它算成"上游失败" ⇒ 内部错误被报成 **502 Bad Gateway**，运维会照着去查渠道，而真因在自己的库里 | 改成**显式枚举** `CodePlatformUnavail` 才算上游失败；`CodeInternal` 保持内部语义（500）。补 2 条回归测试（`internal/domain/errors/upstream_failure_test.go`） |
| R2 | 502 分支只回 `BizError.Message`，**丢了 cause** | 渠道真因（"Specified access key is not found."）不在消息里，操作者还得翻日志 | 改回 `BizError.Error()`（Message + cause）。为避免"原因抄两遍"，同时把 service 层消息里冗余的 `+errMsg` 去掉（cause 里已经有了） |

**8 条修复的 API 端到端回归**（每条一次 curl，8204 SaaS + 8306 私域各一遍）：

| 修复 | 回归口径 | 结果 |
|---|---|---|
| #1 submit-platform 数字主键 | `POST /api/asset-bundle/160/submit-platform` | 200，`platform_asset_id=145`（原先 404 record not found） |
| #2 cron `enabled:false` | `POST /api/browser-automation/cron` | 落库 `enabled=false`、`next_run_at=NULL`（原先 true 且会被排程） |
| #3 卡片 `is_active:false` | `POST /api/tiktok-card` | 落库 `f`、响应 `is_active=false`（原先 true） |
| #4 tiktok 短链入参 | `POST /api/tiktok-card/generate-short-link` 传 `{card_id}` | 200（原先必 400） |
| #5 sms/config 租户 | `POST /api/sms/config`（8204 + 8306） | 均 200，落库 `tenant_id=1`（原先 8204 500） |
| #6 渠道失败不再误判 404 | `POST /api/sms/send` | **502** + 真因（原先 404「记录不存在」） |
| #7 任务状态校验 | `POST /api/sms/job/1/pause`（已完成任务） | **400**「只能暂停运行中的任务」（原先 500） |
| #8 企微密钥读写成对 | GET 拿掩码 → PUT 回填掩码 → 查库 | GET 返 `****cret`；库中仍是 `hack_secret`（真密钥未被覆盖） |

## 上下文管理纪律（避免反复压缩停顿）

1. 不用 `read` 工具读 `user-web/` 与仓库根下的文件——目录上下文会自动注入 4 个 README（≈15K token/次）。读源码用 `grep` 工具或 shell + `head -c` 限长。
2. 所有 shell 输出必须限长（`| head -c 2000`）；禁止 `ls` 大目录全量输出。
3. 已消费完的工具输出一律 `ctx_reduce` 打标；已结束的区段用 ACP `compress` 折叠。
4. 状态只认本文件（`docs/api-test-progress.md`），压缩后不依赖聊天记录。
5. 一轮多测几条路由，减少回合数 = 减少检查点重注入。
