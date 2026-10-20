# 配置全量清单（2026-10-20 双栈 + 部署层调研）

> 配套：`CONFIG_DB_DRIVEN_PLAN.md`（既有方案与落地记录）· `HARDCODE_INVENTORY.md`（点位清册）
> 范围：`user-server` + `user-web` · `hivemtk-platform/platform-server` + `platform-web` · `deploy/` 部署层
> 性质：调研清单（现状快照），不重复方案正文；批次与取舍见 `CONFIG_DB_DRIVEN_PLAN.md` §10 追加记录。

## 0. 判定框架（双轴，本清单每条按此归类）

**轴一 · 来源层**（值归谁管）：

| 层 | 定义 | 例 |
|---|---|---|
| **P1 引导/密钥** | 必须在 DB 可用之前存在，或属密码学材料 | `POSTGRES_PASSWORD`、`MASTER_KEY`、三个 JWT secret、`FIELD_ENCRYPTION_KEY`、`PAYMENT_ENC_KEY`、`PORT`、`GIN_MODE`、`TENANT_MODE`、`SERVER_CONFIG` |
| **P1.5 安全姿态开关** | 放宽防线的开关，DB 化=「库里被写即防线关闭」，读失败必须 fail-closed | `EPAY_MOCK`、`BRUTE_FORCE_DISABLED`、`ALLOW_INSECURE_*_WEBHOOK`、`REQUIRE_PRIVATE_NETWORK` |
| **P2 业务可运营值** | 运营会改、改了要生效：价格/天数/限额/阈值/开关/URL/档期 | 订阅价、试用天数、限流、免打扰窗口、上传限额 |
| **P3 协议/契约常量** | 改了要同步外部系统或属类型契约 | 枚举、官方渠道 API 域名、epay 回调路径、向量维度、错误码 |
| **D 死配置** | 无读取点或无消费方 | `JWT_EXPIRE`、platform `smtp.*` 段、`viper WatchConfig`（存在无人调用） |

**轴二 · 生效时机**（改了多久生效）：`每请求`（TTL 缓存内最终一致）/ `构造期 Restart=true`（下次重启）/ `装配期读一次`（包级 var、wiring）。
两轴必须同时标注——只标来源不标时机，运营会把「改了没生效」当 bug 上报。

**域（scope）**：`config_params` 是**全局**参数（无 tenant 维）。凡本意为租户偏好的值，接线前先定域；现清单默认全局运营参数，租户级需求不在本轮。

---

## 1. 用户端（user-server / user-web）

### 1.1 环境变量（约 171 个字面量读取点；部署侧仅出现 17 个，其余靠代码默认兜底）

**机制**：`LoadDotEnv`（只填空缺，真实 env 优先）→ `config.yaml` `${VAR:default}` → `os.Getenv` 直读；另有 `/run/secrets` 文件预载（`main.go:282`）。

| 分类 | 代表项 | 位置 | 归层 | DB化 |
|---|---|---|---|---|
| 密钥/引导 | `POSTGRES_PASSWORD` `MASTER_KEY` `FIELD_ENCRYPTION_KEY` `USER_JWT_SECRET` `VISITOR_TOKEN_SECRET` `PORT` `GIN_MODE` `TENANT_MODE` `HIVEMTK_SECRETS_DIR` | `db.go:43` `secrets/aesgcm.go:30` `jwt.go:64` `main.go:663` `tenant.go:67` | P1 | 否（留 env fail-closed） |
| 安全姿态 | `REQUIRE_PRIVATE_NETWORK` `ALLOW_INSECURE_*_WEBHOOK` `BRUTE_FORCE_DISABLED` | `network_exposure_guard.go:26` `webhook.go:557` `brute_force.go:16` | P1.5 | 否 |
| 网络/URL | `PUBLIC_BASE_URL` `CORS_ALLOW_ORIGINS_USER` `SSE_CORS_*` `ALLOWED_WS_ORIGINS` `PLATFORM_API_HOST/URL` `PLATFORM_ENABLED` `PLATFORM_CLOUD_URL` `GEO_SITE_BASE_URL` `FRONTEND_URL` | `config/server.go:414` `router.go:30` `ws_origin.go:44` `platform.go:117` `password_reset.go:159` | P2/P2 | **易**（WS 已三级合并；CORS 为包级 var=装配期） |
| AI/推理 | `LLM_*`(已被 `llm_providers` 表覆盖) `EMBEDDING_*` `RERANK_*` `DEEPL_API_KEY` `BROWSER_JEV_*` | `llm.go:214` `embedding.go:103` `rerank.go:123` `config.yaml:155` | P2 | **LLM 已 DB 化**；rerank/deepl 易 |
| 外部服务 | `SMTP_*`(7) `EMAIL_163_*` `QQ_EMAIL_*` `QINIU_*`(4) `UPLOAD_*` `TELEGRAM_POLLING_ENABLED` `WECOM_DISABLE_OUTBOUND` `SMS_ALLOW_NIGHT_SEND` `FEISHU_*` `NOTION_API_KEY` | `email.go:260` `email_send.go:433` `config.yaml:137` `upload.go:112` `sms.go:246` | P2 | **易**（SMTP 建议 DB+FIELD_ENCRYPTION_KEY 加密，运营免重启换凭据） |
| 凭据闸门 | `MCP_TOKEN(_PREV)` `BRIDGE_INGEST_*` `INGRESS_API_KEY` `EMAIL_*_SECRET` `ORDER_WEBHOOK_*` | `mcp_guard.go:44` `bridge_ingress_guard.go:29` `webhook_signature.go:180` | 半 P1 | **bridge 已半 DB 化**（`system_config_kv` > env）；webhook 阈值易（已有 `envDurationOr` 封装） |
| 功能开关 | `FF_*` 两套（直读 env 12 项 + `pkg/featureflag` 5s 轮询） | `ltc_flag.go:1-31` 及各 `*_wiring.go` | P2 | **否——已有 `feature_flags` 独立表**（灰度+分桶），再接参数中心=第三真值源 |
| 运维/调试 | `ENABLE_SWAGGER` `ENABLE_DEBUG_ROUTES` `HEALTH_EMBEDDING_CRITICAL` `ALERT_*` `LLM_DEBUG_DUMP_DIR` `IS_TEST_MODE` | 各处 | P2/P3 | 部分易；调试类低价值 |
| 死配置 | `JWT_EXPIRE`（.env 有、全仓 0 读取点） | `.env` | D | **删，不迁** |

**部署侧覆盖缺口**：~154 个 env 在 `deploy/` 完全不出现，靠默认兜底——多数本就不必出现（默认即正确），少数（`FRONTEND_URL`、`PUBLIC_BASE_URL` 派生项）属部署首启种子职责。

### 1.2 配置文件

| 文件 | 性质 | 要点 |
|---|---|---|
| `config.yaml`（156 行） | 环境为主 + 业务档 | `platform_api/external/security/logging/database/inference/storage/i18n/proxy/sso`；`inference.llm` 真相源已是 `llm_providers` 表；`inference.embedding/rerank` 部分参数化（`${RERANK_*}`） |
| `config/platform.yaml`（27 行） | 环境 + 2 业务值 | `api_url/secret/admin_password` 必填三件（缺则 LoadPlatform 报错不装半截）；`log_report_interval=60`、`admin_username=admin` 是业务值；**文件必须 rsync+Dockerfile COPY 双同步**（线上踩坑先例） |
| `.env-example`（70 键）/`user-server/.env`(4 键) | 部署模板 | `JWT_EXPIRE` 为死键 |

### 1.3 业务硬编码（代表项）

| 项 | 位置 | 现值 | 归层/DB化 |
|---|---|---|---|
| 模块定价 | `model/module_bindings.go:119-158` | 24 模块，browser-automation 9900 | **已 DB 化**（运行时权威=平台，镜像 `tenant_modules.price_cents`） |
| 试用 15 天 / ¥149 月价文案 | `website/src/config/content.js:542` 等 4 处 | 文案硬编码 | 中（文案层，与后端无共享源） |
| GEO 模型单价 | `geo/service/llm.go:60-113` | 数十条 | **已 DB 化**（`geo_dicts/model_prices`，缺行播种 fail-open） |
| 全局限流/访客限流/路径配额 | `ratelimit.go:25` `visitor_rate_limit.go:21` `rate_quota.go:26` | 10rps/20rps/分路径 | **易**（`InitRateLimiter` 注入口现成） |
| 免打扰窗口 | `reach_send_pipeline_quiethours.go:28`(22-8) vs `webhook.go:175`(23-7) | **两处不一致** | 易，但先定「全局参数 + 单键字符串 or 跨键校验」 |
| webhook 队列常量 | `webhook_dedup.go:56-66` | TTL/Workers/Queue… | **已有三级**：env > `config_params` > 常量（`webhook_runtime_params.go`，本方案样板） |
| 报表窗口 `days=30` | 30+ 处 controller | 兜底窗口 | **过度迁移风险**：入口收口或保持常量，不逐点接 |
| URL 常量 | `platform.go:32`(`work.xapptool.cn`) `ports.go:45-75`(推理栈端口) | — | 易（前缀已有 env 覆盖） |
| 分账比例 | — | **不存在**（无分账代码） | 不适用 |

### 1.4 已 DB 驱动（现状基线，新增键前必查这十处）

| 机制 | 表 | 时机 | 覆盖 |
|---|---|---|---|
| **参数中心·平台定义与默认值** | `config_params` + `config_param_groups`（**38 组 240 条种子 / 227 个读取点**，2026-10-20 现测；60s TTL+负缓存） | 启动 seed + 请求期 `Get*` | 阈值/上限/超时/TTL/开关；`Restart=true` 标记装配期项。此表只存**定义与平台默认值**（实例级目录，逐租户复制定义反而会造出「漏补行的租户读到代码兜底值」） |
| **参数中心·租户覆盖**（R5 新增） | `tenant_config_params` + `tenant_config_param_audit_logs` | 请求期，缓存键 `t{tenantID}\|group.key`（无租户时才是 `group.key`） | 只存「本租户改过的键」；未覆盖的键回落平台默认；写与「回默认」都只动覆盖行，租户管理员改不动平台默认值。审计新旧值 dbencrypt 加密 |
| LLM 路由 | `llm_providers` `llm_routing_rules` | 启动 + 平台下发重载 | 覆盖 `inference.llm` |
| 业务字典 | `dictionaries`/`dictionary_transitions` | 请求期 | 状态机、GEO 定价（标量 vs 枚举分工） |
| **平台接入 KV**（R5 拆出） | `platform_config_kv`（`key` 单列主键，实例一份） | 请求期 | 对端只认一个接入点的凭证：桥采集合令牌（含 prev）、`mcp_token`、订单回调验签密钥、浏览器宿主令牌、embedding 全局项、`security.rules`、`backup.strategy` |
| 通用 KV（R5 租户化） | `system_config_kv`，主键由 `(key)` 复合化为 `(tenant_id, key)`（迁移 v3.65.0，存量归默认租户） | 请求期 | password_policy、工具集成 JSON、**`sso.providers`（OIDC 身份源，`client_secret` 字段级 `enc:v1:` AES-GCM）** |
| 站点配置（R5 租户化） | `system_config`，一租户一行（`uni_system_config_tenant`，迁移 v3.66.0） | 请求期 | 站点名、注册开关、上传上限等商户自己的运营口径 |
| 功能开关（R5 租户化） | `feature_flags` + 两张开关日志表，唯一键 `key` → `(tenant_id, key)`（v3.66.0，旧的单列唯一索引必须由迁移删除） | 请求期 | 灰度与 kill switch 每租户独立决策 |
| 模块/订阅镜像 | `tenant_modules`、`tenant_subscription_mirror` | 平台周期同步 | 定价、开通、订阅锁（**每租户一行，行锚 tenant_id**） |
| 前端下发 | `config_params(frontend_ws/ui/pagination)` → `GET /api/config/runtime` | 请求期 | 前端轮询/心跳/上传上限（常量兜底） |

**归属判据只有一条**：表上有没有 `tenant_id` 列（`internal/pkg/tenancyidx` 数据库内省，没有第二份豁免清单）。所以「这张表算不算租户表」要在模型 Tag 与迁移**两处**对账——漏一处就是全量放行。凭证类不落参数中心（参数中心的 value 列是明文，只有审计行加密），走 `system_config_kv` + `internal/secrets`。落地明细见 `CONFIG_DB_DRIVEN_PLAN.md` §10 的 R5 段。

**读取链路**：env → `.env`(不覆盖已有) → `config.yaml` 展开 → DB 就绪 → `SeedConfigParams`(只刷定义列不碰 value) → 12 批 wiring 注入闭包 → secrets → LLM 装载 → platform.yaml → router.Setup（Restart 项在此读一次）。运行期四类并存：启动冻住的包级 var / 请求期直读 env / 参数中心(60s TTL) / FF_* 两套。

---

## 2. 平台端（platform-server / platform-web）

### 2.1 环境变量

| 分类 | 项 | 位置 | 归层 |
|---|---|---|---|
| 强校验必填（缺则 log.Fatal） | `POSTGRES_PASSWORD` `PLATFORM_ADMIN_PASSWORD` `MERCHANT_API_SECRET` `JWT_SECRET` | `config_viper.go:228-239` | P1（`PLATFORM_ADMIN_PASSWORD` 已半 DB：seed 一次入库后走 ChangePassword） |
| 独立密钥 | `CONTRIBUTOR_JWT_SECRET` `PAYMENT_ENC_KEY` `PLATFORM_JWT_SECRET`(兜底) `HIVEMTK_FIELD_KEY` | `contributor_auth.go:35` `payment_crypto.go:32` `crypto.go:29` | P1（⚠️ `HIVEMTK_FIELD_KEY` 缺失时 **dev fallback 派生密钥，生产危险**） |
| 安全姿态 | `EPAY_MOCK`（true+release→fatal 护栏） | `router.go:20` | P1.5 |
| 业务/运维 env | `PLATFORM_PUBLIC_URL` `CORS_ALLOW_ORIGINS_PLATFORM` `STORAGE_LOCAL_*` `UPLOAD_MAX_SIZE` `MAX_JSON_BODY_MB` `PLAYGROUND_LLM_BASE_URL` `SUBSCRIPTION_MONTHLY_PRICE_CENTS` `SUBSCRIPTION_TRIAL_DAYS` `EPAY_GATEWAY_URL/PID/API_KEY` | `order_service.go:799` `cors.go:14` `factory.go:9` `upload_controller.go:41` `body_limit.go:36` `playground_service.go:157` `subscription_service.go:57,60` `payment_channel_service.go:230` | P2（多为首启兜底，DB 有行即失效） |
| 引导/环境 | `SERVER_CONFIG` `GIN_MODE` `APP_ENV` `MODULE_SEED_FILE` `PLATFORM_WEB_DIST` | 各处 | P1/P3 |

### 2.2 配置文件

`config.yaml`(38 行) 与 `config.docker.yaml` 同构：端口/DSN/admin seed/jwt/`log.retention_days=45`。**结构体有而 yaml 未写的死段：`smtp.*`（全仓无消费方）**；`log.level/format` 实际被硬编码 info/json 覆盖（只有 retention_days 生效）。`heartbeat_offline_ttl_minutes=5` 可配但未入任何管理面。

### 2.3 业务硬编码

| 项 | 位置 | 现值 | 归层/DB化 |
|---|---|---|---|
| **分账比例** | `model/asset_market.go:62-63` | `PlatformShare decimal(4,2) default:0.30`，读取在 `asset_market_service.go:199`（购买时读**资产行**） | P2，但三种语义必须先选定：(a)只影响新建资产（写时戳）(b)读时现算（存量分成突变）(c)回写存量行。**选 (a)**；且 `decimal(4,2)` 天然限 2 位小数，KV 化必须 min/max 钳位 |
| 订阅月价/体验天数 | `subscription.go:129,130` | 14900 / 15 | **已 DB 化**（`subscription_config` 单行 + `PUT /platform/subscription/config`，请求期无缓存=热更新；env 仅兜首启） |
| **续费顺延 30 天** | `subscription.go:33` | `SubscriptionPeriodDays=30` | 散落：`order_service.go:317` 已把 period_days **快照进订单**、`:684` 读快照；但 `subscription_service.go:166` 有 `days<=0 → 常量` 的**实时回退**——动常量前必须先修此回退，否则旧单按新天数延 |
| 模块定价 | `module_catalog` 表（种子三级回退：`MODULE_SEED_FILE`→磁盘→内嵌） | browser-automation 9900，余 0 | **已 DB 化**（admin CRUD，base 模块不可变） |
| LLM 套餐 | `llm_plans` | **无代码种子，首启空档**（生产已手工建 4 档） | 已 DB 化；缺省档可补种子（可选） |
| 易支付 | `payment_channels`（`EnsureFromEnv` 只兜首启灌 disabled 行）；回调路径常量 `payment_service.go:170-176` | 网关 `pay.gggua.com` 只在 .env/docs | 网关**已 DB 化**；路径常量 P3（改需同步网关侧） |
| 限流 | `ratelimit.go:170`(登录 5rps) `router.go:34`(全局 20rps) | 包级 var，改需重启 | 中（需热更新支持） |
| body/上传上限 | `body_limit.go:31,36` `upload_controller.go:38` | 64MB multipart / 10MB | 易 |
| 心跳离线 TTL | `config_viper.go:208` | 5 分钟（yaml 可配，无管理面） | 易 |

### 2.4 已 DB 驱动

`subscription_config` / `module_catalog`(+deps/menu/options) / `payment_channels` / `llm_plans` / `llm_provider_bindings` / `tenant_modules` / `site_contact_config` —— 全部**请求期查库、无缓存、天然热更新**。
**关键缺口：平台端没有任何通用参数表，也没有一道参数门禁**（`scripts/` 只有两个 md 检查脚本）——每新增一项专用表 = model+repo+service+DTO+controller+前端页的成本，且无法用脚本守「登记即接线」。

### 2.5 前端 platform-web

`.env.production` `VITE_API_BASE_URL=/`（同源相对路径，消除 CORS 依赖）；源码**无硬编码 API 域名**；`Subscription.vue:90,99` 的 `149/15` 仅为表单占位（加载后被 API 覆盖）；dev 代理 8212→8205。

---

## 3. 部署层（deploy/）

| 项 | 现状 | 归属 |
|---|---|---|
| `deploy.conf` | 唯一无密钥配置源，`DOMAIN` 一行派生六站 URL，幂等 sed | **部署者减配的既有机制**——非密钥字段已自动刷新，缺的只是个别「首启自动写入」动作 |
| `.env` 生成 | `ensure_env`：首启生成 → `chmod 600` → **永不覆盖**；已存在只对非密钥字段幂等 `set_kv` | user 侧密钥 6 项 + platform 侧 6 项（README 漏列 `MASTER_KEY`，需修文档） |
| 配置文件进镜像 | **全部 COPY（无 bind-mount）**，`deploy.sh` rsync 与 `Dockerfile COPY` 必须两处同步（`platform.yaml` 线上踩坑先例：缺文件→PlatformCfg=nil→商户上报/续费/模块同步全挂） | 新增任何配置文件的硬规则 |
| compose `environment:` 硬写 | `DB_HOST/REDIS_HOST/PLATFORM_API_URL/SERVER_CONFIG/GIN_MODE/EPAY_MOCK/TZ` | P1（容器拓扑） |
| nginx（宝塔，仓外） | `workapi→8204`、`manageapi→8205`、SPA 回退、SSE/WS/http2 off 需手工按模板落 vhost | 部署手工项（脚本不碰） |
| 人工步骤 | ssh-copy-id、架构确认、docker 确认、宝塔建站反代、**抄录 `PLATFORM_ADMIN_PASSWORD`（只打印一次）** | 文档化 |

---

## 4. 汇总：DB 化优先级（本轮候选）

| 批 | 内容 | 层 | 风险 |
|---|---|---|---|
| **R0 纯减配** | 死配置删除（`JWT_EXPIRE`、platform `smtp.*` 死段）、README 密钥清单补 `MASTER_KEY`、`PLATFORM_PUBLIC_URL` 首启种子自动写入、本清单归档 | D/文档 | 零（无 DB、无读取口变更） |
| **R1 平台端参数中心** | 移植三道门（readpoints/defaults/keystyle）→ 建同构 `config_params` 表+seed+admin API → 先接**每请求低风险项**：心跳 TTL、限流、上传/body 上限（不含钱、不含 URL） | P2 | 建表可留、读取点可回退 |
| **R2 用户端第一批** | 限流组、免打扰窗口（先定全局域与单键形态、统一 22-8/23-7 不一致）、SMTP 通道（DB+加密）、`FRONTEND_URL`/`PLATFORM_CLOUD_URL`/`GEO_SITE_BASE_URL`；**env 全保留为应急开关** | P2 | env 仍在=直接回退 |
| **R3 钱与拓扑** | 分账比例全局 override（**写时戳语义 (a)**，min/max 钳 2 位小数，上线前对账 `revenue_records`）、续费 30 天（**先修 `subscription_service.go:166` 实时回退**） | P2(钱) | 单独一发，出事只影响本批 |
| **明确不做** | FF_*（已有 `feature_flags` 表）、`days=30` 逐点接、CORS/`max_json_body_mb`（重启可接受）、`PLATFORM_PUBLIC_URL` 入业务库（部署拓扑非业务值）、密钥/引导项 | — | — |

**生效时机标注要求**：每落地一个键必须记录「`group.key` → 代码点位 → 生效时机（每请求/构造期 Restart/装配期）」，追加到 `CONFIG_DB_DRIVEN_PLAN.md` §10，沿用既有批次记录格式。
