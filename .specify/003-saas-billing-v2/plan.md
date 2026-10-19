# 003 · plan.md — 技术决策（ADR-M8 / ADR-M9）

> 沿用 002 plan.md 的 ADR-M1…M7 编号序列。

## ADR-M8 · 租户订阅模型

**状态**：Accepted（2026-10-19）

**背景**：云端 SaaS 版需要「15 天体验 + ¥149/月仅按月订阅 + 到期整站锁定」。现有模型无任何订阅/体验期概念；Merchant 表无有效期字段（license 流程已下线）。

**决策**：
1. 平台端新表 `tenant_subscriptions`（merchant_key 唯一），字段：status(trial|active|expired)、started_at、trial_expires_at、current_period_end、price_cents（下单快照）、updated_at。
2. 商户注册（tenant_registration 落库）即创建 trial 行，`trial_expires_at = now + SUBSCRIPTION_TRIAL_DAYS`（默认 15，平台端配置）。
3. 订阅订单经 `MarkPaid → fulfillSubscription` 顺延：`current_period_end = max(now, current_period_end) + 30d`（防提前续费吞时长；按自然月简化为 30 天，运营口径「按月」）。
4. `CurrentStatus()` 惰性判定：now < trial_expires_at → trial；now < current_period_end → active；否则 expired。无 cron。
5. 控制下发走 `/merchant-api/modules/sync` 响应的 `subscription` 段；心跳响应不变。
6. 用户端独立镜像表 `tenant_subscription_mirror`（sync 写入），`SubscriptionGuard` 中间件按镜像态锁业务面（白名单放行登录/市场/订阅/支付查询）；平台不可达时 fail-open 用最后已知态（镜像行带 fetched_at，>1h 仍不升级为锁，仅 Warn）。
7. 纵深防御：平台端对 expired 商户在 LLM 代理 Precheck 拒绝；用户端 402 业务码（4302）触发续费墙 UX。

**否决的替代**：
- 订阅字段并入 Merchant 表——无历史留痕，续费流水无处安放。
- cron 到期扫描——多一个常驻任务与分布式锁复杂度，惰性判定足够。
- 心跳携带控制——心跳是「证明在线」统计面，混入控制语义会把续费问题变成断连问题（oracle 审查意见：拒绝面精确化，sync 通道保留）。

## ADR-M9 · 易支付接入

**状态**：Accepted（2026-10-19）

**背景**：002 遗留支付回调 501 占位（payment_callback_controller），N1 声明本期不接真实支付；003 用户裁决接入易支付，先 mock 后真人。

**决策**：
1. **资金边界**：钱的事全在平台端。用户端只做「下单→拿支付参数→查状态→sync 拿权益」，不接触支付密钥、不验签。
2. **唯一入账口不变**（ADR-M3 延续）：真实回调与 mock 确认都收敛到 `MarkPaidForChannel → MarkPaid → fulfill`。`Notify` 端点替换 501：验签→金额比对→trade_no 绑单→入账。
3. **验签不变量**（先于代码写入测试向量）：
   - 签名算法：易支付标准 `md5(sort(params)+key)`，排除 sign/empty 值；
   - 金额以「分」比对 `order.amount_cents`，不符拒绝并落 verified=false；
   - `trade_no` 唯一索引：同一 trade_no 不可绑定第二张订单（防重放/跨单）；
   - 回调处理幂等：重复通知返回成功但不重复入账；
   - 验签失败 401、未知订单 404、入账异常 500（让网关重试，幂等兜住）。
4. **MOCK 模式**（`EPAY_MOCK=true`）：CreateEpayPayment 返回 mock 收银台地址；`POST /platform/payments/mock/confirm/:order_no` 触发后走**与真实回调完全相同的内部路径**（仅跳过外部 HTTP 与真实签名源）。mock 是行为基线，不是旁路。
5. **渠道配置**：PaymentChannel 扩表 gateway_url/pid/api_key；api_key AES-GCM 加密落库，密钥从 env `PAYMENT_ENC_KEY`；无配置时下单返回「支付通道未配置」引导线下（延续一期人工确认路径不删除）。
6. **联系人约束放宽**：在线支付路径不强制 `ErrNoContactForPayment`；线下路径保持。
7. **回滚开关**：平台端关渠道 enabled → 用户端自动回落线下指引，零代码回滚。

**风险与缓解**：见 tasks.md 风险登记簿；上线前执行 EPAY_LIVE_CHECKLIST（T032）真人小额验证。

## 审查修订（2026-10-19 · oracle 对抗审查并入，覆盖上文冲突条款）

### ADR-M8 修订
1. **锁的技术载体（替代原 #6 的路径白名单表方案）**：不建第二张路径表。`SubscriptionGuard` 实现=三层判定：
   - 显式小**白名单**（续费必需面，逐路径列全+对账测试）：`/api/auth/*`、`/api/public/*`、`/api/module-market/*`（浏览/下单/pay-status）、订阅状态与续费、`/api/modules/sync`、`/api/modules/my`、logout；
   - 其余路径走 `ModuleOwnerOf(path)`（复用 4300+ 路由归属表）：有归属模块 → 4305 拦；无归属 → 放行+WARN（与 4301 同风险等级取舍）；
   - **客户面显式不锁**（产品裁决，写入 spec）：widget/chat public、chat WebSocket、bridge ingress/outbox、wechat webhook 在 expired 时保持可用——商户存量客户的 AI 应答不断线；锁的是员工工作台业务面。
2. **锁判定本地化（替代原 #6 fail-open 条款）**：用户端用镜像里的 `current_period_end/trial_expires_at` 跑与平台 `CurrentStatus()` 同款惰性判定；`now > period_end → 锁`，**与平台可达性无关**；`fetched_at` 仅用于 Warn/可观测，不参与锁决策。fail-open 仅对「镜像无 period_end 数据」一种情形生效。堵死「断网 1h 白嫖」。
3. **错误码 4305**（`CodeSubscriptionExpired`，4302 已被 `CodeModuleNotFound` 占用）；前端 `INIT_REDIRECT_MAP` 注册跳转，但 4302/4305 跳转互斥节流（sessionStorage 10s），墙页与市场页依赖请求集不得互相触发对方业务码。
4. **镜像表结构硬约束**：`tenant_subscription_mirror` **部署级单行、不带 tenant_id**（sync 跑在 SystemScope，带 tenant_id 必踩默认租户串写坑——002 模块镜像 purchaseTenants 同款问题）；`status` 列仅为判定缓存，权威=`current_period_end`/`trial_expires_at` 惰性比较。
5. **顺延幂等+原子（替代原 #3 写法）**：新表 `subscription_grants(order_no UNIQUE)` 做订单级幂等锚（MarkPaid 会重复调 fulfill，靠它挡重复送时长）；顺延用**单条原子 SQL**：`UPDATE tenant_subscriptions SET current_period_end = GREATEST(current_period_end, now()) + interval '30 days' WHERE merchant_key=$1`，列 `timestamptz`，比较统一 UTC。禁止 Go 读-改-写。
6. **trial 横幅逻辑**：`current_period_end > trial_expires_at` 时显示「有效期至 X」而非「体验期剩 N 天」（防对已付费用户显示体验期）。
7. **「按月」=30 天口径**：UI 只显示绝对到期日，运营对账按订单，spec 明写「按 30 天计」。
8. **expired 商户下单分流**：平台端 MerchantCreate 按 order_type 分流——`subscription` 放行（续费是恢复通道），`module/llm` 对 expired 拒绝（防「付了钱模块激活但订阅锁着」的履约死锁）。merchant_auth 不整体拒绝的豁免仅覆盖 sync/查询。
9. **0 元订单直通**：`total==0` 不走 epay（最小 0.01），走免支付直通但**仍收敛进** `MarkPaidForChannel → MarkPaid`，状态直接 fulfilled。

### ADR-M9 修订
10. **Notify 路由挪公开组**：从管理员 JWT 组挪到无鉴权公开组（md5 验签是唯一防线，其权重相应提升）；`payments.GET("/callbacks")` 运营看板留 admin 组。
11. **回调参数解析修复（真人验收翻车点）**：现 `io.ReadAll` 后再 `PostForm` 读到的全是空（body 已消费，gin 实测验证）——改为读原始 body 后 `url.ParseQuery` 直接解析（顺带覆盖 GET-notify/query 变体）；**mock 必须从同一 HTTP handler 起跑**（测试用 httptest 打真路由），禁止 Go 内手工拼 notice。
12. **回调重放三态**：`processed_at != nil` → 200 真重复；`processed_at IS NULL` → 重放验签+入账（订单级幂等由 MarkPaid CAS 兜）；修「首次入账 500 后重投被 200 吞掉=钱不入账」缺陷。
13. **验签公式统一**：排除 sign 与空值后的全部请求参数按 key 升序拼接+key 的 md5；签名必须覆盖 `money/total_fee`、`out_trade_no`、`trade_status` 三组；金额解析只用于比对、签名用原始字符串；`parseAmountCents` 遇非数字字符即拒绝（防静默截断）；验签失败一律 401 不降级。
14. **trade_no 防重放域**：`(channel, pid, trade_no)` 复合唯一索引（现无索引；防换 pid 后误判）；入账前校验 trade_no 未绑定其它订单。
15. **EPAY_MOCK 防泄漏**：mock 路由条件注册（false 时路由不存在）；`EPAY_MOCK=true` 且 `GIN_MODE=release` → **启动失败**；mock confirm 保持 admin+审计组。
16. **配套**：`paymentMethodOfChannel` 增 epay 分支；死字段 `IdempotencyKey` 实现或删除注释掉；T005 与 `module_gate_test.go` 的 `FreeToEnable` 既有断言同批改。

### T006 实现修订（P1 并入）
17. **不改共享函数 `baseTenantModuleRows`**（会被 on 模式补齐分支误用导致开通/关闭震荡）——新增 `offlineGiftModuleRows()` 仅 `bootstrapOffline` 使用；`bootstrapOffline` 的 early-return（存量部署）分支补 reconcile + `refreshProcessGate`（否则升级后菜单 16 组不出现）；平台端 0 元模块 owned 下发口径两端同批定死；验收加「升级路径+菜单节点数对账」。

## 执行顺序（建议施工波次）

- 波次 A（规格）：T001→T002→T003, T015
- 波次 B（并行双仓）：用户端 T004-T007, T010-T014 ‖ 平台端 T008-T009, T016-T017
- 波次 C（集成）：T018-T023
- 波次 D（官网+收口）：T024-T032
