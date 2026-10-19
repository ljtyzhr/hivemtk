# 易支付上线前真人支付验收清单（EPAY_LIVE_CHECKLIST）

> **任务来源**：003 · SaaS 计费二期 **T032**（`.specify/003-saas-billing-v2/tasks.md`）· 2026-10-19
> **性质**：**只交付清单，不在本期执行**——用户裁决「上线发布前才真人测试」。
> **依据**：`hivemtk/.specify/003-saas-billing-v2/plan.md` **ADR-M9 易支付接入**（含「审查修订」#10–#15）与 **ADR-M8 租户订阅模型**。
> **执行前提**：`scripts/epay_mock_e2e.py`（或等价 mock 全流程）已全绿——mock 是行为基线，真人验收只验「真实网关 + 真实签名源 + 公网回调」这三段 mock 覆盖不到的差异。

每一步都给出**操作**与**预期输出**；任一步预期不达成即停，不得进入下一步。

---

## 0. 前置条件（清单开跑前打勾）

| # | 项 | 判据 | 预期输出 |
|---|---|---|---|
| 0.1 | mock 全流程已通过 | 注册 → trial → 买浏览器自动化 → mock 支付 → 到期 4305 → 续费恢复 全绿 | 脚本退出码 0 |
| 0.2 | 已取得真实易支付商户号 | 网关地址 + 商户号 `pid` + 密钥 `key` + 可用支付方式 | 三项凭据到位（密钥仅走配置页/env，不进聊天记录与代码仓） |
| 0.3 | 平台端有公网可达地址 | `https://<平台对外域名>` 可从公网访问 | 浏览器打开返回平台响应（非内网 IP） |
| 0.4 | 已知晓回滚开关 | 见 §7 | 关渠道 `enabled=false` 可在 1 分钟内回到线下指引 |

---

## 1. 环境与互斥确认（`EPAY_MOCK` × `GIN_MODE`）

**操作**：

1. 平台端 `.env`（`hivemtk-platform/platform-server/`）：
   ```bash
   EPAY_MOCK=false          # 真人验收必须关 mock
   GIN_MODE=release         # 生产形态
   PAYMENT_ENC_KEY=<32 字节以上随机串>   # 不设则支付渠道 api_key 无法加密落库，Upsert 报错
   ```
2. 启动 platform-server，确认进程**正常起来**（互斥是启动期失败，不是运行期降级）。
3. 反向确认互斥规则仍生效（可回滚演练）：临时改回 `EPAY_MOCK=true` + `GIN_MODE=release` 启动一次。

**预期输出**：

| 检查 | 预期 |
|---|---|
| `EPAY_MOCK=false` + `GIN_MODE=release` 启动 | 进程正常拉起，启动日志**无** mock 相关行 |
| `EPAY_MOCK=true` + `GIN_MODE=release` 启动 | **启动失败**并报互斥错误（ADR-M9 修订 #15），绝不带 mock 上生产 |
| `EPAY_MOCK=false` 时访问 `POST /platform/payments/mock/confirm/:order_no` | 路由**不存在**（mock 路由条件注册，404），而非逻辑分支拦下 |
| 库检 | `SELECT enabled FROM payment_channels WHERE channel='epay';` → 将在 §2 置为 `true` |

---

## 2. 真实商户号配置（平台端「支付配置」页字段对照）

**入口**：平台端后台 → 计费 → **支付配置**（`platform-web/src/views/platform/billing/PaymentConfig.vue`；
接口 `PUT /platform/payments/channels/epay`）。也可用 env 兜底初始化（首启无行时灌一行 disabled 配置，后台可覆盖）。

| 页面字段 | 库列（`payment_channels`） | 填什么 | 校验点 |
|---|---|---|---|
| 通道 | `channel`（主键） | `epay` | 本期只实现 epay；wechat/alipay 枚举位保留未实现 |
| 网关地址 | `gateway_url` | 易支付网关根地址，如 `https://pay.example.com` | 末尾不带 `/`，能从公网打开 |
| 商户号 pid | `pid` | 网关分配的商户号 | 与网关后台完全一致（回调验签含 `pid`） |
| 密钥 api_key | `api_key`（**密文**，`enc:v1:` AES-GCM 前缀） | 网关密钥 key | **库里非明文**；页面回显脱敏、明文永不回传 |
| 支付方式 | `trade_types` | 逗号分隔，如 `alipay,wechat`；空 = 网关默认 | 至少一种可用 |
| 回调地址 | `notify_url` | 留空则自动 `https://<平台对外地址>/public/payments/notify/epay` | 必须是**公网 https**（见 §3） |
| 启用 | `enabled` | 真人验收时 `true`；回滚时置 `false`（§7） | 见 §7 |
| 备注 | `remark` | 例如「2026-10 上线真人验收」 | — |

订阅口径同批核对：**订阅配置**页（`billing/Subscription.vue`，`PUT /platform/subscription/config`）——
月订阅价 `monthly_price_cents=14900`、体验天数 `trial_days=15`。

**预期输出**：

- 保存成功；`SELECT channel, enabled, gateway_url, pid, left(api_key,8) FROM payment_channels WHERE channel='epay';`
  → `api_key` 以 `enc:v1:` 开头（**不可见明文**）。
- 未配置/停用时下单：用户端拿到「支付通道未配置」并**引导线下**（延续一期人工确认路径，不删除）。

---

## 3. 公网回调可达性验证（不花钱）

**操作**：

1. 计算预期回调地址：`https://<平台对外域名>/public/payments/notify/epay`（POST 与 GET 变体都要通）。
2. 从**公网侧**（不是平台本机）发一条假报文：
   ```bash
   curl -i -X POST 'https://<平台域名>/public/payments/notify/epay' \
     -d 'pid=0&out_trade_no=PROBE&trade_no=PROBE&trade_status=TRADE_SUCCESS&money=0.01&sign=deadbeef'
   ```
3. 平台端查回调留痕看板：`GET /platform/payments/callbacks`（admin）。

**预期输出**：

| 检查 | 预期 |
|---|---|
| 网络层 | **HTTP 401**（验签失败一律 401，绝不降级；若得到 404 说明公网没打到该服务或反代路由丢路径，**先修网络**） |
| 留痕 | `payment_callbacks` 新增一行：`channel='epay'`、`verified=false`、`verify_error` 含验签失败原因、`processed_at IS NULL`、`payload` 存原始报文 |
| 反代/防火墙 | 平台侧访问日志能看到该请求的**真实来源公网 IP**（证明不是本机回环） |
| GET 变体 | `curl 'https://<平台域名>/public/payments/notify/epay?pid=…&sign=…'` 同样进入处理（修订 #11 覆盖 query-notify 变体），预期同为 401 + 留痕 |

> 网关侧同步核对：易支付商户后台的「回调/通知地址」配置与 §2 的 `notify_url` 一致（有的网关以后台配置为准，会覆盖下单参数）。

---

## 4. 小额真实支付（0.01 / 网关最小额）：验签 + 入账

**操作**（金额做小，验证链路而非收入）：

1. 平台端 **Catalog** 或 **订阅配置** 把被测项价格临时改为最小额（0.01 元 = 1 分；若网关不收 0.01 则用其允许的最小金额），经 sync 下发后用户端模块市场显示新价（平台端是运行时权威）。
2. 用户端登录 → 模块市场 → 下单：`POST /api/module-market/order` → 响应 `payment.pay_url` → 收银台跳真实网关。
3. **用真实资金扫码/支付最小额**。
4. 观察平台端回调 → 用户端 `GET /api/module-market/orders/:order_no/pay-status`（比 10 分钟 sync 快）与 `GET /api/modules/sync`。
5. 订阅单再跑一笔（下单 `order_type=subscription`），验证顺延。
6. 验收后把价格改回（14900 / 9900）。

**预期输出**：

| 阶段 | 预期 |
|---|---|
| 下单 | 真实网关收银台（跳转表单或二维码），金额 = 1 分（或最小额），**不是** mock 收银台地址 |
| 回调 HTTP | 网关侧记录 **HTTP 200**、body `success`（验签+入账完成后才 200；绝不先 200 再入账） |
| `payment_callbacks` | 对应行 `verified=true`、`processed_at` 非空、`trade_no`/`pid`/`amount_cents` 有值 |
| 订单状态机 | `orders.status`：`pending_payment → paid → fulfilled`（`order_status_logs` 三条流转，`source` 记 `channel_notify`） |
| 唯一入账 | `payment_records` **恰一行**：`source`/`payment_method` 标记 epay、`amount_cents` = 订单 `total_amount_cents`；`operator` 非人工 |
| 金额篡改防护（反向用例） | 手工伪造一条同单不同金额的回调（改 `money` 不改签）→ **401** 且 `verified=false`；改签但金额不符 → 拒绝并落 `verified=false` |
| 重放 | 同一 `trade_no` 二次投递 → **200 `success`** 但**不重复入账**（`payment_records` 仍一行、金额不翻倍）；换一张订单复用同一 `trade_no` → 拒绝「trade_no 已绑定订单」 |
| 未知订单 | `out_trade_no` 不存在 → **404** |
| 用户端 | `pay-status` 30s 内返回 `paid/fulfilled`；模块单 → 模块市场「我的」出现已开通模块（`tenant_modules` 激活行，只激活下单租户）；订阅单 → `tenant_subscriptions.current_period_end` **顺延 30 天**，从 `GREATEST(now, period_end)` 起算（提前续费不吞时长） |
| 幂等锚 | `subscription_grants` 该 `order_no` **恰一行**（`order_no` UNIQUE）；重复回调不产生第二行 |
| 0 元直通 | 造一笔 `total_amount_cents=0` 订单 → **不走网关**（最小 0.01），状态直接 `fulfilled`，仍写入 `MarkPaid` 同一入账口 |

---

## 5. 对账核对：订单 ↔ 流水 ↔ subscription_grants

**操作**：在平台端 PostgreSQL 执行（列名以 `platform-server/internal/model` 为准，已按其核对）：

```sql
-- 5.1 三向对账：订单 ↔ 支付流水 ↔ 回调留痕
SELECT o.order_no, o.order_type, o.status, o.total_amount_cents,
       pr.amount_cents   AS record_amount,
       pr.source, pr.payment_method,
       cb.trade_no, cb.pid, cb.verified, cb.processed_at
FROM orders o
LEFT JOIN payment_records   pr ON pr.order_no = o.order_no
LEFT JOIN payment_callbacks cb ON cb.order_no = o.order_no
WHERE o.created_at >= now() - interval '7 days'
ORDER BY o.created_at DESC;

-- 5.2 订阅履约：订单 ↔ 幂等锚 ↔ 订阅当前到期日
SELECT g.order_no, g.merchant_key, g.days, g.amount_cents,
       s.status, s.trial_expires_at, s.current_period_end
FROM subscription_grants g
JOIN tenant_subscriptions s ON s.merchant_key = g.merchant_key
ORDER BY g.created_at DESC;

-- 5.3 空转/孤儿检测（必须 0 行）
SELECT * FROM payment_records   WHERE amount_cents <> (SELECT total_amount_cents FROM orders WHERE orders.order_no = payment_records.order_no);
SELECT * FROM subscription_grants g WHERE NOT EXISTS (SELECT 1 FROM orders o WHERE o.order_no = g.order_no);
-- 同一订单多条流水（除明确退款外必须 0 行）
SELECT order_no, count(*) FROM payment_records GROUP BY order_no HAVING count(*) > 1;
```

**预期输出**：

| 核对 | 预期 |
|---|---|
| 5.1 | 每张 `paid/fulfilled` 订单：`payment_records` 恰一行、金额与 `total_amount_cents` 相等；对应 `payment_callbacks` 有 `verified=true` 且 `processed_at` 非空的行 |
| 5.2 | 每张 `subscription` 订单在 `subscription_grants` 恰一行；`current_period_end = 起算基准 + days(30)`，与 `grants.days` 一致 |
| 5.3 | 三段查询均 **0 行** |
| 反向 | 网关后台的交易流水（含手续费）金额与平台 `payment_records` 逐笔可对应；差异只允许来自网关手续费口径，不允许订单金额错位 |
| 回调看板 | `GET /platform/payments/callbacks` 本轮真人回调全部 `verified=true`；历史探针行（§3）保持 `verified=false` 属正常 |

---

## 6. 订阅真人链路与续费墙（4305）

**操作**：

1. 新注册一个真实租户 → 平台 `tenant_subscriptions` 应为 `trial`、`trial_expires_at = now + 15 天`（`subscription_config.trial_days`）。
2. 用小额真实支付买一笔订阅 → 观察用户端。
3. 模拟到期（**仅测试环境** SQL 后门：把 `current_period_end`/`trial_expires_at` 改到过去）→ 用户端刷新。
4. 再跑一笔真实小额续费 → 观察解锁。

**预期输出**：

| 阶段 | 预期 |
|---|---|
| 注册 | `status='trial'`、15 天体验期；sync 响应 `subscription` 段有值 |
| 续费支付成功 | `status='active'`，`current_period_end` 顺延；用户端顶栏显示「有效期至 X」 |
| 到期 | 用户端业务面被拦，业务码 **4305**（`CodeSubscriptionExpired`）→ 续费墙页；白名单放行：登录 / public config / 模块市场（浏览·下单·我的）/ 订阅状态与续费 / `pay-status` / logout |
| 客户面不受锁 | widget/chat public、chat WebSocket、bridge ingress/outbox、wechat webhook **仍可用**（产品裁决，存量客户 AI 应答不断线） |
| 平台端纵深 | expired 商户的 LLM 代理 Precheck 拒绝（402/403）；`sync`/查询类豁免不整体拒绝 |
| 续费恢复 | 4305 消失，业务面恢复；`subscription_grants` 新增一行 |

---

## 7. 回滚开关（零代码回滚）

**操作**：平台端 → 支付配置 → 渠道 `epay` → **启用 = false** 保存（或停用该行）。

**预期输出**：

| 检查 | 预期 |
|---|---|
| 用户端下单 | 立即**回落线下指引**（联系运营 + 线下确认路径），不再返回 `pay_url` |
| 指引来源 | 一期人工确认路径未删除，`MarkPaid` 仍是唯一入账口（ADR-M3）；线下确认后台照常 `confirm-paid` 入账 |
| 已支付订单 | 不受影响，回调若仍到达按状态机幂等处理（未启用通道不应再发起新支付） |
| 恢复 | 重新置 `enabled=true` 即回到线上支付，无需发版 |

**降级顺序（建议演练一次）**：关渠道 `enabled=false` → 确认新单走线下指引 → 需要时再补 `EPAY_MOCK` 仅限**非 release** 的预发环境。

---

## 8. 验收签署

| # | 步骤 | 结论（✅/❌） | 执行人 | 时间 |
|---|---|---|---|---|
| 1 | §1 环境互斥（mock × release 启动失败） | | | |
| 2 | §2 真实商户号配置 + key 密文落库 | | | |
| 3 | §3 公网回调可达（POST/GET → 401 + 留痕） | | | |
| 4 | §4 小额真实支付：验签 / 入账 / 重放 / 金额篡改 | | | |
| 5 | §5 三向对账全绿（0 孤儿 0 多流水） | | | |
| 6 | §6 订阅真人链路与 4305 续费墙 | | | |
| 7 | §7 回滚开关演练（关渠道 → 线下指引 → 恢复） | | | |

**放行判据**：七项全 ✅；任一 ❌ 需回 ADR-M9 对应不变量定位（验签公式 #13 / 金额分比对 #13 / `trade_no` 复合唯一 #14 / 回调重放三态 #12），修复后**从 §3 起重跑**。

---

## 附录 · 相关文档

- 订阅与支付链全景：`hivemtk-platform/docs/architecture/PLATFORM_BUSINESS_CHAINS.md` §2「链 E — 订阅与支付链」
- 决策与不变量：`hivemtk/.specify/003-saas-billing-v2/plan.md`（ADR-M8 / ADR-M9 + 审查修订 #1–#17）
- 支付契约（订单/回调签名）：`docs/architecture/ORDER_WEBHOOK_SIGNATURE.md`
- 计费流程（双轨 LLM + 支付时序）：`docs/architecture/LLM_BILLING_FLOW.md` §二 / §七
- 部署与初始化：`docs/operations/MERCHANT_INITIALIZATION_FLOW.md`
