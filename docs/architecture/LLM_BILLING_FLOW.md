# LLM 双轨计费：完整流程论证（任务五交付）

> **状态**：v1.0 · 2026-10-18
> **需求原文**：「平台支持用户自己配置 llm 也支持使用平台配置 llm……预付费……
> 商户使用平台端 llm 时，**实时扣减剩余 token 数，输入和输出分别计价以及扣费**，
> **完整的流程看情况论证补全**」
> **一期边界**：不接入实际支付，但契约按线上支付建模（回调复用）

---

## 一、双轨的分工与计价口径

| 轨道 | Key 谁保管 | 计量 | 扣费 | 断网可用 |
| --- | --- | --- | --- | --- |
| **自带 Key**（`source=self`） | 商户 | ✅ 记 `llm_routing_logs` | ❌ **不计费** | ✅ |
| **平台垫资**（`source=platform`） | **仅平台** | ✅ 双落库 | ✅ **实时扣减** | ❌ |

### 1.1 为什么自带 Key 不收费

收费对象是**平台垫付的算力**，不是"路由能力"。路由、降级、成本可视化本身是基础能力
（`llm-self` 模块，永久免费）。若对自带 Key 收费，用户的第一反应是"凭什么用自己的钱还要付费"。

### 1.2 为什么平台 Key 绝不下发

`plan.md ADR-M7`：下发 = 把垫资凭据交出去。商户可直连厂商**无限量使用**，
平台既无法计量也无法止损。因此调用链必须是：

```text
自带 Key：user-server ──────────────────→ 厂商        （Key 在商户侧）
平台垫资：user-server → 平台代理 ────────→ 厂商        （Key 只在平台侧）
```

代价是：这是本项目「数据零出域」铁律的**唯一显式例外**。成立条件（缺一不可）：

1. 商户在平台端开通了 `llm-cloud` 并购买套餐（**显式同意**）；
2. 代理地址由商户自己配置（`PLATFORM_API_URL`），平台**不主动外联**；
3. 只有 `source='platform'` 的 provider 会走这条路 —— 自带 Key 一次都不会调到这里
   （`TestSelfProviderDoesNotUseProxy` 钉死）。

---

## 二、完整时序（含人工确认与异步回调预留）

### 2.1 主流程：一期（线下人工确认）

```text
① 商户端                     ② 平台端                        ③ 商户端
──────────────────────────────────────────────────────────────────────
 打开 LLM 页面
   └─ GET /api/llm/plans ──────────────→ 已上架套餐列表
   └─ GET /api/llm/wallet ─────────────→ 余额快照（输入/输出分列）

 点「购买套餐」→ 填联系方式
   └─ POST /api/llm/recharge ────────→ CreateOrder(order_type=llm)
                                        状态 pending_payment
                                        返回 payment_guide
   ← 提示「联系运营付款」

                              运营收到线下汇款
                                └─ POST /platform/orders/:no/confirm-paid
                                     └─ MarkPaid(order_no, PaymentNotice{
                                          source: offline_manual })
                                          ├─ CAS pending_payment → paid
                                          ├─ 写 payment_records
                                          ├─ 扣减/入账 wallet（按套餐 token 数）
                                          └─ CAS paid → fulfilled
   ◀── 模块/套餐同步后可见余额（10 分钟内）

 调用 LLM
   └─ dispatcher 选到 source=platform 的 provider
        └─ POST {PLATFORM}/llm-api/v1/chat/completions?provider=X
             ├─ Precheck(估输入, ≥1)   余额不足 → 402（不到厂商）
             ├─ 转发厂商
             ├─ RecordUsage(request_id=trace_id)   ← 幂等
             │    ├─ 扣 wallet（SELECT FOR UPDATE 行锁）
             │    └─ 写 llm_usage_records
             └─ 返回厂商报文 + X-Llm-Usage-* 响应头
        └─ 本地：recordPlatformUsage（镜像，不二次扣费）
```

### 2.2 二期：接真实支付

唯一变化是**入账来源**，其余全部复用：

```text
渠道异步回调 POST /platform/payments/notify/{channel}
  ├─ InsertCallbackIfAbsent(callback_id)   ← 唯一索引，重复回调被挡
  ├─ 验签（二期实现；一期恒 false 并返回 501）
  └─ MarkPaid(order_no, PaymentNotice{ source: channel_notify })
       ↑ 与线下确认**同一个函数**
```

**「支付回调复用」的确切含义**：线下人工确认与线上渠道回调**构造同一个
`PaymentNotice`、调用同一个 `MarkPaid`**。区别只有两个字段：`source` 与 `operator`。

> 若一期线下路径自己写一套 SQL，二期接回调时必然要重写，且极易漏幂等。
> 契约先立、通道后接，是这次设计唯一正确的顺序。

### 2.3 幂等的三道独立防线

| 层 | 机制 | 防的是 |
| --- | --- | --- |
| 订单 | `UPDATE ... WHERE status='pending_payment'` CAS | 两个管理员同时点「确认收款」 |
| 计量 | `llm_usage_records.request_id` 唯一索引 | 网络抖动导致的自动重发 |
| 回调 | `payment_callbacks.callback_id` 唯一索引 | 渠道重投 |

> 订单层的 CAS 曾是一个真实缺陷：有 `fields` 时**不写 `status`**，
> 导致订单永远停在 `pending_payment` 而 CAS 仍返回"成功"—— 已修复并由
> `TestMarkPaidConcurrentCASOnlyOneWins` 钉死。

---

## 三、分别计价（需求核心）

### 3.1 口径

```text
输入成本 = prompt_tokens     × input_price_per_1k  / 1000
输出成本 = completion_tokens × output_price_per_1k / 1000
总成本   = 输入成本 + 输出成本
```

**绝大多数厂商的输出单价是输入的数倍**（例如 0.5 / 3 元每 1k）。
把两者合成一列会系统性低估成本，并让"高输出场景"看起来比实际便宜。

### 3.2 为什么不能"总价按比例拆分"

改造前的实现是：

```text
cost = total_tokens / 1000 × 单一单价
promptCost     = cost × (prompt / total)      ← 按 token 比例劈两半
completionCost = cost - promptCost
```

这在数学上**等价于输入输出同价** —— 你把单价设成任何值，都表达不出"输出贵"这个事实。

改造后 `computeCost` 分别乘各自单价；`splitCost` 保留但**仅用于历史数据展示**
（`llm_routing_logs` 里旧记录的 `prompt_cost`/`completion_cost` 是比例拆出来的，
报表要继续能画）。两者在代码里分开并各有注释，防止有人误把 `splitCost` 当计费依据。

### 3.3 向后兼容

只配了 `cost_per_1k` 的存量 provider **行为完全不变**：

```text
EffectiveInputCostPer1k()  = cost_per_1k_input > 0 ? cost_per_1k_input : cost_per_1k
EffectiveOutputCostPer1k() = cost_per_1k_output > 0 ? cost_per_1k_output : cost_per_1k
```

因此迁移 `v3.62.0` **不触碰任何历史成本数据**（见该迁移的注释）。

---

## 四、余额的三个不变量

| 不变量 | 保障方式 | 测试 |
| --- | --- | --- |
| 余额永不为负 | `SELECT … FOR UPDATE` 行锁 + 事务内判定 | `TestWalletConcurrentConsumeNeverOverdraw` |
| 重试不重复扣费 | `request_id` 唯一索引，冲突即跳过扣减 | `TestRecordUsageIdempotentByRequestID` |
| 本地与平台不双扣 | 扣费权威在平台侧；本地只落明细 + 镜像 | 设计约束（`llm_platform_usage.go` 文件头） |

### 4.1 过期余额

`expires_at` 早于当前时间时，`Precheck` 与 `Consume` 都按 0 处理（返回 `ErrInsufficientToken`），
**不允许扣成负数**。钱包视图把 `expired=true` 透给前端，由前端提示续费。

### 4.2 预检阈值（本轮修掉的真实缺陷）

预检最初写死「输出至少 1024 token」，后果是 **`output_tokens < 1024` 的小规格套餐
一次都调不通** —— 商户付了钱、后台余额为正、调用却报"余额不足"，而日志里只有这一行。

输出量在调用前**不可知**，因此预检只判「输入够不够」+「输出余额 > 0」：
宁可放过一点、事后按实际扣，也不要用臆想的阈值挡住合法用户。

---

## 五、错误语义（处置动作必须可区分）

| 情况 | HTTP | 业务码 | 前端动作 |
| --- | --- | --- | --- |
| 余额不足 | 402 | `4401 CodeLLMBalanceInsufficient` | **去充值** |
| 平台未配置 / provider 未启用 | 503 | — | 去配置平台地址 |
| 商户密钥无效 | 401 | — | 检查平台配置 |
| 上游 5xx | 502 | — | 稍后重试或换模型 |

> 余额不足**必须**独立成类（`ErrInsufficientLLMBalance`）。混成"调用失败"会让用户在
> 余额已为负时反复重试，把失败日志刷满，同时拿不到正确引导。

---

## 六、provider 生命周期

| 动作 | 平台端 | 用户端 |
| --- | --- | --- |
| 创建 | `POST /platform/llm/providers`（含真实 Key） | — |
| 下发 | `deliver=true` | `GET /merchant-api/llm/providers` → 只取元信息，**无 Key** |
| 落库 | — | `llm_providers(source='platform', plan_code, …)`（按 name upsert，**不动 api_key**） |
| 启用判定 | `enabled` | 平台未连接时该 provider **不进候选**，调度自动走 fallback |

### 6.1 一条被修掉的破坏性缺陷

`UpdateModel` 原先不处理 `source`，而前端「启用/停用」开关只发 `{enabled}`：

```
用户点一下"停用" → Source 落库为空串 → ProviderSource() 判成 self
                 → 平台 provider 的 Key 本就不在商户侧 → 此后每次调用必 401
```

**用户做的事只是点了个开关，界面上没有任何异常提示。**
现在 `Update` 一律沿用旧 `source` / `plan_code` / 分列单价，`Create` 强制 `self`
（拒绝商户伪造 `source=platform` —— 平台上并没有它，每次调用都会 404）。
由 `TestUpdateModelPreservesPlatformSource` / `TestCreateModelAlwaysSelf` 钉死。

---

## 七、一期不做与预留

| 项 | 一期 | 预留 |
| --- | --- | --- |
| 支付通道 | ❌ 人工确认 | 回调端点 501 占位 + 幂等表 + `MarkPaid` 复用 |
| 退款 | ❌ | `WalletTxnRefund` + `order.cancel/reject` 已具备 |
| 多次充值叠加 | ✅ 天然支持 | — |
| 优惠券 / 试用 | ❌ | — |
| 按月订阅 | ❌（只有年费/计量） | `billing_cycle` 枚举已含 `metered` |

---

## 八、可观测性

| 数据 | 位置 | 用途 |
| --- | --- | --- |
| 平台侧权威明细 | `llm_usage_records` | 对账（与厂商账单核对） |
| 商户侧明细 | `platform_llm_usage_records` | 商户自助查询、按人/场景归因 |
| 全量路由日志 | `llm_routing_logs` | 路由决策、缓存命中、降级 |
| 钱包流水 | `llm_wallet_txns` | 每次余额变动带变动后快照 |

**两份 usage 表不是重复**：`llm_usage_records`（用户端既有）记聚合成本，
`platform_llm_usage_records` 记平台垫资的**分列**明细并带幂等键与归因字段。
合并会迫使两边语义混合。