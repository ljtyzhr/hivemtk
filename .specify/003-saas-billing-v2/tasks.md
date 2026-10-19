# 003 · SaaS 计费二期 · 任务清单

> 生成：2026-10-19 · 含每任务二次论证（头脑风暴结论已并入方案）
> **v1.1 修订（2026-10-19 oracle 对抗审查并入）**：以 plan.md「审查修订」节为准，关键更正——
> 订阅锁不建路径白名单表（改 ModuleOwnerOf 复用 + 4305 新码，4302 已占用）；客户面（widget/chat/bridge/webhook）显式不锁；
> 锁判定本地化（镜像 period_end 惰性比较，fetch 时效只 Warn 不影响锁）；镜像表部署级单行无 tenant_id；
> fulfillSubscription 订单级幂等锚+原子 SQL 顺延；expired 下单按 order_type 分流（订阅放行/模块拒绝）；0 元订单免支付直通；
> Notify 挪公开组+body 解析修复+重放三态+验签公式统一+trade_no 复合唯一；EPAY_MOCK 与 release 互斥启动失败；
> T006 用 offlineGiftModuleRows 新函数+存量升级 reconcile；T005 同批改 FreeToEnable 既有断言。
> 依赖记号：`[A←B]` = A 依赖 B 完成后开工；标注 ⚠ 的任务已过 oracle 对抗审查

## Phase 0 · 规格与语义（先行，半天）

### T001 spec/plan/tasks 三件套落盘 ← 本清单
- 交付：`.specify/003-saas-billing-v2/{spec.md,tasks.md,plan.md}`
- 论证：项目惯例是 specify 目录驱动施工（001/002 先例）；不落盘直接改码会在收口对账时失去唯一施工源。plan.md 记录 ADR-M8（订阅模型）/ADR-M9（易支付接入）。
- 验收：三文件存在且本清单任务编号连续。

### T002 术语与文档口径修订
- 交付：`website/TERMINOLOGY.md`（「云端版」转正、「代管版」仍禁）；`docs/architecture/MODULAR_MODULES.md` §3 定价章节改新口径并注明覆盖声明；`LLM_BILLING_FLOW.md` §七 移除「不接支付通道」；两仓 `docs/INDEX.md` 补录 002/003。
- 论证：TERMINOLOGY 是官网文案的强制闸，不先改会卡住 T025-T027；MODULAR_MODULES 写着「定价写进 spec 不可改」，必须显式勘误而非静默改，否则未来会话会按旧口径回滚。官网 CI 有术语检查风险，先改词表最便宜。
- 验收：grep「代管版」仍为唯一禁用命中；「云端版」在允许清单。

### T003 public config 增加 edition 语义
- 交付：`/api/public/config` 增 `edition: opensource|saas`（=TENANT_MODE 映射）与 `cloud_url`（平台端可下发，离线回落常量 `https://saas.hivemtk.cn`）；`user-web` 按 edition 决定是否显示订阅/续费入口。
- 论证：Q1「开关配置控制」的前端感知点；比让前端猜 TENANT_MODE 更干净。cloud_url 做成配置而非硬编码，占位域名上线时零代码替换。最小 diff：一个 handler 函数 + 两个字段，无表结构变更。
- 验收：off/on 两模式 curl 该端点字段正确。

## Phase 1 · 定价模型与目录（1 天）

### T004 BillingCycle 枚举扩展 monthly/once（双端）
- 交付：平台端 `model/module_catalog.go` 与用户端 `model/module_bindings.go` 的 BillingCycle 增 `monthly|once`（保留 yearly|metered）；校验、文档、种子 JSON 同步。
- 论证：买断(once)与月订阅(monthly)语义不同——once 永不写 ExpiresAt，monthly 写 +1 月。放同一枚举复用现有 Usable/过期判定，避免新增第二套周期字段。两端枚举必须同批改，否则 gen-module-bindings 重生成会因未知 cycle 失败。
- 验收：两端单测覆盖 4 种 cycle 的 Usable 判定。

### T005 目录重定价：全免费 + 浏览器自动化 ¥99 买断
- 交付：`ModuleCatalogMeta` 全部模块 `price_cents=0`（保留 category 语义：base 常开、其余 0 元=FreeToEnable）；browser-automation=9900 分/once；跑 `go run ./cmd/gen-module-bindings` 重生成平台种子与官网 content.js；平价路线旧文案（¥999/年）清除。
- 论证（已知冲突）：002 的「平价路线」决策被用户新口径覆盖——在 commit message 与 MODULAR_MODULES 勘误里显式声明，防止旧文档回滚。0 元付费模块走 FreeToEnable 而非全并入 base：保住「将来恢复收费」的开关（改价即可），且不动 BaseModuleKeys/菜单绑定/闸测试面。浏览器自动化单独留 paid 类目 = 平台端可单独控制开关。
- 验收：种子 JSON 24 模块中恰 1 个 price>0；用户端 market list 显示「免费/¥99买断」；官网 modules/pricing 页数值一致。

### T006 离线闸策略调整：本地同锁浏览器自动化
- 交付：`bootstrapOffline`/`baseTenantModuleRows` 改为「全部 0 元模块开启 + browser-automation 锁定」（原为仅 BaseModuleKeys）；`MODULAR_MODULES.md` FR-5.7 勘误同步。
- 论证：用户裁决「本地与云端一致锁定」。实现上注意：0 元模块=曾是付费类目，离线开启需写 tenant_modules 行（source=gift），否则闸空态只认 BaseModuleKeys 会仍锁——这正是 002 勘误里 allow-all 作废的坑，必须显式落行。迁移策略：存量离线部署升级时在 Sync/bootstrap 路径补行（幂等）。
- 验收：离线模式启动后 EnabledKeys=23（24-浏览器自动化）；升级路径幂等测试。

### T007 平台端价格成为运行时权威
- 交付：用户端 `MarketList/MarketDetail` 价格改从 sync 快照读（镜像表已有 PriceCents），本地常量仅兜底；平台端 Catalog.vue 改价后经 sync 生效。
- 论证：Q4 明确「定价在平台端」。镜像表已存 PriceCents，改动只是读路径切换 + 兜底顺序，低风险。不改会导致「后台改了价用户端还显示旧价」——一期就有的半通问题，本期必须闭合，否则 149/99 调价无法运营。
- 验收：平台改价→触发 sync→用户端 market 显示新价。

### T008 ⚠ 平台端订阅模型（T001-plan ADR-M8）
- 交付：新表 `tenant_subscriptions`（tenant_id/merchant_key 唯一、status: trial|active|expired、started_at、trial_expires_at、current_period_end、price_cents 快照）；注册钩子建 trial 行（15 天，天数从系统配置读）；`MarkPaid` fulfill 路径对 subscription 订单顺延 `current_period_end`（+1 月，从 max(now, current_period_end) 起算防叠送）；到期惰性判定函数 `CurrentStatus()`。
- 论证：不把订阅塞进 Merchant 表——订阅会有历史流水（续费记录）需求，独立表天然留痕；顺延基准取 max(now, period_end) 是防「提前续费被吞时长」的关键细节（头脑风暴里被点名的坑）。无 cron：Expired 判定=读时比较 now，heartbeat/sync 响应实时算。价格快照进表防改价影响存量订阅。
- 验收：单测——trial 期内续费顺延、过期后续费从 now 起、重复回调幂等（复用订单幂等键）。

### T009 平台端订阅/价格管理页
- 交付：platform-web 新增「订阅配置」页：月订阅价格（默认 14900）、体验期天数（默认 15）、当前各商户订阅状态列表；模块 Catalog.vue 表单支持 monthly/once 文案。
- 论证：Q3/Q4 要求定价全在平台端配置。与 T008 同仓可并行于用户端任务。商户订阅列表页复用 merchant/List.vue 骨架，增量小。
- 验收：改价/改天数后新注册租户按新值生效。

## Phase 2 · 初始化与赠送（1 天，可与 Phase 1 并行部分）

### T010 初始化静默合规采集（双入口）
- 交付：`InitAdminWithRegistration` 与 `TenantService.RegisterTenant` 采集 ClientIP/UA/设备指纹（fingerprint 包），落 tenants 扩展列（init_ip/init_ua/init_device_fingerprint，migration 加列）+ 随 `TenantRegistrationPayload` 上报平台（平台端 tenant_registration 落库字段同步扩展）；平台端心跳 IP 已有，不重复。
- 论证：Q2 核心诉求。采集必须服务端做（前端可伪造且用户不可见才叫静默）；IP 用 gin ClientIP 但要注意 trusted-proxy 配置现状（离线单机场景风险低，记录到 plan 已知限制）。privacy：只存 IP/UA/指纹哈希，不存原始 UA 明文可接受（短字段）。迁移只加列不改老列，零回滚风险。
- 验收：两入口注册后 tenants 行三字段有值；平台端 payload 含三字段；DB 测试断言。

### T011 /setup 前端字段对齐
- 交付：`InitSetup.vue` 补 address（必填）、company/legal_person/phone 保持必填、email/license_no 降选填；提交 payload 对齐后端新校验；监管告知勾选保持。
- 论证：侦察确认前端缺 address 且后端 ValidateOfflineRegistration 要求 email 必填——两端必须同批改（先发后端放宽会出现前端仍强制 email 的不一致）。用 TDD：先改校验测试再改代码。
- 验收：api_verify 场景「最小字段注册成功/缺地址 400/缺邮箱成功」。

### T012 后端初始化校验收敛
- 交付：`ValidateOfflineRegistration`/`validateRegulatoryFields` 改为 名称/负责人/电话/地址 必填，邮箱执照选填；`reg_mode` 按 TENANT_MODE 实际返回（修 auth.go 硬编码 offline）；SaaS 注册通道同步。
- 论证：与 T011 是同一契约的两端，编号拆开仅为分工清晰，**必须同批提交**。email 降选填的副作用检查：唯一索引/登录标识若依赖 email 需改为「email 可空但非空时唯一」——查 model 确认后在实现时处理。
- 验收：单测覆盖必填矩阵。

### T013 开户赠送 provisioning（1 智能体 + 1 资产包）
- 交付：`provisionTenantStarterKit(tenantID)`：克隆 default_sales 智能体为租户级副本（tenant_id 归属）+ 创建 1 个起步资产包（内置模板 starter-bundle：欢迎语/常见问答/话术各若干）；`RegisterTenant` 与 `InitAdminWithRegistration` 成功路径调用（幂等：以 tenant_id 查重）；失败仅 Warn 不阻断注册（赠送可后补）。
- 论证：default_sales 是全局行不能直接改归属（会破坏存量）——克隆。资产包选「内置模板」而非克隆 seed 的 8 个 official 包：官方包属平台资产域，租户级克隆语义混乱；自建小而全的 starter 模板最干净。幂等键=tenant_id+template_key。不阻断注册：赠送失败不该让商户开不了户（可用性优先，可观测 Warn+可重跑脚本）。
- 验收：注册后 ai_agents/asset_bundles 各有租户行；重复调用不重复创建；赠送失败注册仍成功。

### T014 智能体/资产包列表租户过滤复核
- 交付：核对 ai_agents、asset_bundles 两域的 GORM tenancy 注入与市场/列表查询，确保租户副本只在本租户可见；补漏网测试。
- 论证：T013 引入「同名 agent 多租户副本」后，任何未走 callback 的裸查询都会串数据——001 的教训（fail-closed）。侦察到 seed/迁移存在系统域写入，需确认 system scope 豁免通道覆盖 provisioning（用 WithSystemScope 写，读走租户上下文）。
- 验收：跨租户查询不互见的单测。

## Phase 3 · 易支付（2 天，核心风险区）⚠

### T015 ADR-M9 易支付接入决策记录（随 T001 plan.md）
- 内容：验签算法（md5(pid+trade_no+type+status+key) 排序拼接）、金额以分比对订单 amount_cents、回调必须落 payment_callbacks 后校验、同一 trade_no 不可跨单复用、回调处理全程幂等、Notify 永不返回非 200 成功除非验签+入账完成。
- 论证：支付是资金安全区，先把不变量写死再写码。易支付是个人商户网关生态，实现差异大——MOCK 模式做行为基线，真实网关上线前用同样测试向量验证。

### T016 平台端支付渠道配置
- 交付：`PaymentChannel` 扩表（gateway_url/pid/api_key(加密落库,AES-GCM,key 从 env PAYMENT_ENC_KEY)/trade_type 列表/enabled）；后台「支付配置」页（平台端）；env 兜底初始化行。
- 论证：侦察确认现表只有 channel/name/enabled/notify_url 放不下三元组。key 加密是平台端持钥场景的底线（DATABASE 泄露不泄支付密钥）。渠道模型保留 wechat/alipay 枚举位，本期只实现 epay 通道名 `epay`。
- 验收：配置页 CRUD；库中 key 非明文；无配置时下单返回「支付通道未配置」引导线下。

### T017 平台端易支付下单与回调整体
- 交付：`payment_service`：CreateEpayPayment（生成 epay 跳转表单参数+签名校验出参）、Notify 处理（验签→金额比对→trade_no 绑单→`MarkPaidForChannel`→fulfill）；`EPAY_MOCK=true` 时：CreateEpayPayment 返回 mock 收银台地址，提供 `POST /platform/payments/mock/confirm/:order_no` 模拟用户支付成功→走完整回调内部路径（含验签模拟）；paymentMethodOfChannel 增 epay 映射。
- 论证：mock 必须走**与真实回调相同的内部路径**（只跳过外部 HTTP+真实验签源），否则 mock 全绿真实翻车。金额比对用分。trade_no 唯一索引防重放。回调失败策略：验签失败 401 落 verified=false；订单不存在 404；入账异常 500（易支付会重试，幂等兜住）。「绝不假装成功」原则延续。
- 验收：单测——正常入账/验签失败/金额不符/重放同 trade_no/未知订单五向量。

### T018 用户端支付发起与状态拉取
- 交付：用户端 `CreateOrder` 增返回 `payment` 段（pay_url/pay_type/qr 由平台 merchant-api 下发）；订单详情轮询接口存在性复核（sync 已有）；`ErrNoContactForPayment` 约束改为「线下支付必留联系人，在线支付不强制」；新增 `GET /api/module-market/orders/:no/pay-status` 直查平台（比 10 分钟 sync 快）。
- 论证：购买链路用户端只做「拿支付参数+查状态」，钱的事全在平台端——保持「平台不碰业务数据、用户端不碰钱」的既有边界。pay-status 直查走既有 merchant-api HMAC 通道，不加新鉴权面。联系人约束放宽是 Q4 隐含要求（在线支付不需要先加好友）。
- 验收：mock 模式下单返回 pay_url；支付确认后 30s 内订单态变 paid/fulfilled。

### T019 用户端收银 UX（moduleMarket）
- 交付：Market/Detail 下单成功弹「收银台」（跳转 pay_url 或展示二维码/轮询条）；Orders 页状态自动刷新（支付完成 toast + 模块激活提示）；订阅续费入口（见 T021）共用收银组件。
- 论证：复用现有 Dialog 风格，不新造页面。轮询间隔 3s×40 次封顶后转手动刷新（防无限轮询）。element-plus 已在栈内。
- 验收：手工冒烟 + 组件单测（轮询停止条件）。

### T020 订单类型扩展 subscription + fulfill 履约
- 交付：平台端 Order.type 增 `subscription`（迁移 + 校验）；fulfillModules 之外新增 `fulfillSubscription`（→T008 顺延）；用户端订阅下单 API（先平台 merchant-api 后用户端代理或直连——按现 market 同构：用户端代理）。
- 论证：复用 Order 状态机/幂等/日志全套，只是加 type+fulfill 分支（ADR-M3「统一三类订单」的自然延伸为四类）。金额=订阅快照价，下单时锁价。
- 验收：订阅订单全状态机测试。

## Phase 4 · 订阅态下发与续费墙（1 天）

### T021 ⚠ 平台端订阅状态下发（merchant-api）
- 交付：`/merchant-api/modules/sync` 响应扩展 `subscription` 段（status/current_period_end/trial_expires_at/price 快照）；heartbeat 响应保持不变（控制走 sync 不走心跳，维持既有边界）；expired 商户在 merchant_auth 层**不整体拒绝**（否则续费墙信息都拉不到），仅 LLM 代理 Precheck 与受业务面拒绝。
- 论证（oracle 级风险点）：心跳被拒会断「证明在线」统计且无法续费——所以拒绝面必须精确：sync 照常给（带 expired 态），LLM 代理拒绝（钱相关），业务数据面在**用户端**锁（平台端拒绝会让用户端 500 体验差）。用户端拿到 expired → 本地闸锁。纵深：即使用户端被破解绕过，平台端 LLM 与模块 sync 也已断供。
- 验收：expired 商户 sync 返回 expired；LLM 调用被 402/403。

### T022 用户端订阅镜像与到期闸
- 交付：tenants 扩列（subscription_status/current_period_end/trial_expires_at）或独立 `tenant_subscription_mirror` 表（倾向后者：语义清晰）；sync 写镜像；`GuardModulePath` 前加 `SubscriptionGuard` 中间件：expired → 除白名单外 402「订阅已到期」业务码；白名单=登录/public config/模块市场只读+下单+订阅状态/pay-status/logout；fail-open：平台不可达时用最后已知镜像态（TTL 内不升级为锁）。
- 论证：锁在中间件层与模块闸同构（复用 pattern 表机制另建一张订阅白名单 pattern 表）。402 业务码而非 HTTP 402 混用——按项目 {code,message} 惯例给专用码（沿用 4301 模块闸先例定 4302）。fail-open 是「网络抖动不砖头」的关键：镜像态>1h 未更新时仍不锁（可用性>收入保护，运营靠平台端统计催收）。
- 验收：单测——expired 锁业务面、白名单放行、镜像过期不锁。

### T023 用户端续费墙 UX
- 交付：4302 拦截 → 续费墙页（展示到期时间/订阅价格/去支付按钮=订阅收银台）；工作台顶栏订阅横幅（trial 剩 N 天提醒/到期红条）；Login 后若 expired 直达续费墙。
- 论证：复用 request.js 拦截器分流（4301 先例复制 4302）。横幅仅 on 模式渲染（edition gate）。文案走 i18n。
- 验收：mock 全流程可演示：到期→锁→支付→恢复。

## Phase 5 · 官网（1 天）

### T024 官网云端版入口
- 交付：`content.js` 常量 `CLOUD_URL='https://saas.hivemtk.cn'`（单点）；hero CTA 增「云端版在线使用」按钮（复用现有 CTA 样式类）；navbar「云端版」链接；pricing 页顶部云端引导条；footer 增云端链接。TERMINOLOGY 通过检查。
- 论证：四处入口但一个常量——上线换域名改一行。不新造 UI 组件，全部复用现有 button/link 类，满足「UI 风格相同」硬约束。
- 验收：本地 `npm run build` + dev 预览四入口可见；术语 lint 过。

### T025 官网内容审计增删
- 交付：features/modules/pricing 内容按新定价刷新（gen 重生成已覆盖数值，本任务补叙述文案）；开源信息区强化（AGPL、双仓库、5 分钟部署）；删除过时「平价路线 ¥999」叙述；i18n contentExtra 同步。
- 论证：T005 重生成只管结构化数据，散文文案需人工审。范围控制：只动 content.js 与 i18n 文件，组件零改动（守住 UI 不变）。
- 验收：全站无 ¥999/年 残留；中英一致。

### T026 官网构建与视觉校验
- 交付：`npm run build` 通过；Playwright 截图对比（首页/pricing/modules 三页改前后各一张）确认布局无位移；404/postbuild 正常。
- 论证：website 有 postbuild 壳生成，只验 vite build 不够。截图对比是「UI 不变」的硬证据。
- 验收：截图归档 docs/assets 或 PR 描述。

## Phase 6 · 测试与收口（1 天）

### T027 双端测试全绿
- 交付：用户端/平台端 `go build ./...` + 相关包测试；DB 测试带 `export POSTGRES_TEST_PORT=8202 POSTGRES_TEST_PASSWORD=...`；service 大包 `-timeout 15m`；新增测试见各任务验收。
- 论证：CLAUDE.md 红线；历史假红（28P01/超时）已知，按记忆中的环境变量处理。
- 验收：全绿记录。

### T028 mock 端到端流程脚本
- 交付：`scripts/epay_mock_e2e.py`（或 shell）：注册 SaaS 租户→查 trial→下单浏览器自动化→mock 支付→验证模块激活→把订阅改 expired（SQL）→验证 4302 锁→订阅续费 mock 支付→验证解锁。可对本地双栈重复执行。
- 论证：Q4 明确「先模拟测试保证流程通畅」。脚本化=上线前回归资产。SQL 直改 expired 是测试后门（生产无此路径），脚本注释标注。
- 验收：脚本一次跑通。

### T029 api_verify 三端验证
- 交付：按 `scripts/api_verify_full.py` 惯例补新端点（订阅状态/pay-status/config edition 等）验证段。
- 论证：CLAUDE.md 验收标准三端口径。
- 验收：脚本输出全 code=0。

### T030 文档收口
- 交付：两仓 CHANGELOG；`PLATFORM_BUSINESS_CHAINS.md` 增计费链一节；`MERCHANT_INITIALIZATION_FLOW.md` 更新字段与赠送；INDEX 补录；`.env-example` 增 PAYMENT_* / EPAY_MOCK / SUBSCRIPTION_TRIAL_DAYS。
- 论证：002 收口时发现 INDEX/CHANGELOG 漏账，本期预防性收口。
- 验收：文档 diff 审过。

### T031 整体提交推送（按 CLAUDE.md 规则1）
- 交付：两仓各自一次性 commit+push（内层双远端 gitee-upstream+upstream；平台端 origin）；commit message 含任务号与验证状态。
- 论证：规则1 禁小步提交；30 任务全程改动留在工作区到此收口。风险：大 commit 难 review——以 message 分节缓解。
- 验收：git log 两仓各 1 个新 commit，工作区干净。

### T032 上线前真人支付 checklist（文档，不执行）
- 交付：`docs/operations/EPAY_LIVE_CHECKLIST.md`：真实商户号配置步骤/小额真实支付验证/回调可达性/对账核对/回滚开关（关 EPAY→回线下流程）。
- 论证：用户明确「上线发布前才真人测试」——本期只交付 checklist 不执行。
- 验收：文档存在。

---

## 依赖与并行图

```
T001→T002,T015
T003 ─┐
T004→T005→T006,T007 ─┤（Phase1）
T008→T009,T020      ─┤
T010,T011↔T012,T013→T014（Phase2，独立可并行）
T016→T017→T018→T019（Phase3 平台→用户端）
T020→T021→T022→T023（Phase4）
T024→T025→T026（Phase5，依赖T005重生成）
T027←全部；T028←T023;T029,T030,T031,T032 收口
```

## 风险登记簿

| 风险 | 等级 | 缓解 |
|---|---|---|
| 易支付网关实现差异（签名/参数各家不同） | 高 | MOCK 基线+上线前 checklist 真人验证；真实网关适配层独立文件 |
| 支付回调安全（伪造/重放/金额篡改） | 高 | T015 不变量先行；verified 落库；trade_no 唯一；金额分比对 |
| 0 元模块批量开启破坏既有闸测试 | 中 | T006 显式落行+幂等升级测试 |
| email 降选填的唯一性/登录影响 | 中 | T12 实现时查索引；必要时部分唯一索引 |
| 大 commit 回滚粒度 | 低 | message 分节+分支保护不在本期 |
| 官网术语 lint 卡 CI | 低 | T002 先行改词表 |
