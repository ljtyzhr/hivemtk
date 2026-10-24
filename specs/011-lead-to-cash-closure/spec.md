# Feature Specification: D1 Agentic Lead-to-Cash 全流程闭环实挂

**Feature Branch**: `011-lead-to-cash-closure`

**Created**: 2026-10-24

**Status**: Approved

**Input**: 计划文档《开发计划-改进清单-2026-10-23》D1 行——「AI 报价草稿 → 改价确认 →
自动转订单/回款」链路未闭环，重定级 P1，本轮实挂，灰度口径为「按 opportunity→quote→bill
分阶段门」。

## 一句话

四段已经各自能跑的腿（报价生成/发送、改价确认、账单派生、订单/回款），缺一个
**串起它们、每步都过阶段门、且不另写一条写路径**的编排层。本切片交付这个编排层：
`quote_draft → price_confirmed → order_created → bill_derived →（回款，只留调用点）`。

## 背景：已有什么 / 缺哪一环（取证）

### 已有的四段腿（各段都有唯一写入口与幂等判据）

| 段 | 写入口 / 幂等判据 | 证据 |
|---|---|---|
| 报价生成 | `QuoteService` 追加版本行 + 行项目 | `user-server/internal/service/quote.go` |
| 报价发送/审批 | `QuoteSendService`，发 `SalesEventTypeQuote` | `user-server/internal/service/quote_send.go`、`model/sales_event.go` |
| 改价确认 | sent→accepted 的 CAS（`repository.QuoteRepository.UpdateStatus`，`WHERE id=? AND status=?`） | `user-server/internal/repository/quote.go` |
| 账单派生 | `BillService.DeriveFromQuote`，幂等键 `bills.quote_row_id` 唯一索引（`ErrBillAlreadyDerived`） | `user-server/internal/service/bill.go`、`internal/repository/bill.go` |
| 订单 | 旧订单域 `OrderService`（`model.Order`，无报价引用列） | `user-server/internal/service/order.go`、`internal/model/order.go` |
| 回款 | `PaymentService.RecordPayment`（幂等键 channel_ref），输入 `RecordPaymentInput` | `user-server/internal/service/payment.go` |

### 已有的阶段门（本层必须复用、一个字节不改）

- `internal/middleware/ltc_gate.go`：`LTCStageGate` shadow/block 两态，四类失败码用例；
- `internal/service/ltc_config.go`：`LTCStage`（opportunity/outreach/quote/bill/payment/collection）、
  `LTCConfig.StageActive`、`LTCReason*`（master_off/stage_off/config_degraded/unknown_stage）；
- `LTCConfigReader` 接口（`opportunity_convert.go` 定义，只读 `Config(ctx)`）。

### 缺的那一环

四段腿之间**没有一条把它们串起来的路**：一条报价链从 draft 走到「开出应收」需要
人工在各服务之间搬运上下文，而且每步推进都**不经过阶段门**——运营关了 quote 阶段的开关，
照样有人能确认改价、转订单、开账单。这正是 D1 判 35% 的原因。

## User Scenarios & Testing

### US1 · 一条链被推进到底（Priority: P0）

作为接线层（webhook / 定时任务），我调一次 `Run(quoteID)`，这条链就从当前格一路走到
`bill_derived`，并拿到「现在就能调 `RecordPayment`」的回款调用点。

**Acceptance Scenarios**:

1. Given 链上最新版是 accepted、有行项目、quote/bill 两道门都开，
   When 调 `Run`，Then 订单落一笔、账单落一张、结果为 `bill_derived`，
   且结果带齐 Order/Bill/Payment 调用点。
2. Given 链上最新版是 sent，When 调 `Advance`，Then 报价状态 CAS 成 accepted、
   结果停在 `price_confirmed`，订单/账单零写入。
3. Given 链上最新版是 draft，When 调 `Advance`，Then 报 `ErrLTCPipelineQuoteNotSent`
   （发送是发送腿的职责，编排放行不了「造一版报价」），零写入。

### US2 · 每一步都过阶段门，门关着 = 一行都不写（Priority: P0）

作为运营，我关掉 quote 阶段的开关后，任何人都不能再确认改价或转订单；
关掉 bill 阶段后，不能再开账单。门关着**不是错误**：结果里 `Advanced=false` +
`GateReason` 说明是哪一道门拦的。

**Acceptance Scenarios**:

1. Given quote 门关（总开关关 / quote 未开 / 配置降级三种之一），
   When 调 `Advance`，Then 返回 `Advanced=false` + 对应 `LTCReason*`，且订单/账单/报价状态零写入。
2. Given quote 门开但 bill 门关、链停在 `order_created`，When 调 `Advance`，
   Then 返回 `stage_off`，账单零写入。
3. Given 配置降级，When 调 `Advance`，Then `GateReason=config_degraded` 且 `GateDetail` 带底层原因。

### US3 · 状态是派生的，不建第五张表（Priority: P0）

作为 DBA，我不要为「这条链走到哪了」新建一张表——它应该是
`quotes.latest.status + 订单存在性 + 账单存在性` 的推导结果，避免第二个事实源。

**Acceptance Scenarios**:

1. Given 最新版 draft/sent，Then `FlowStage = quote_draft`。
2. Given 最新版 accepted，Then 按「先账单后订单」判：有账单=`bill_derived`、
   有订单=`order_created`、都没有=`price_confirmed`。
3. Given 链上 v1 accepted、v2 draft，Then 认最新版 v2 = `quote_draft`（旧版被拒/被接受都不作数）。
4. Given 最新版 rejected/expired，Then `FlowStage` 报 `ErrLTCPipelineQuoteTerminal`。

### US4 · 并发落败能愈合，卡死能报出来（Priority: P0）

作为接线层，我在多副本下重放一次推进时，不会因为「对手已经赢了」而制造重复写入或假失败。

**Acceptance Scenarios**:

1. Given 确认改价 CAS 落败但重读已是 accepted，Then 本次算成功（幂等）。
2. Given CAS 落败且重读仍不是 accepted，Then 报 `ErrLTCPipelineStatusStuck`（人工核对）。
3. Given 转订单撞「已转出」，Then 回读复用那一笔，不写第二笔。
4. Given 开账单撞「已派生」，Then 回读复用那一张，不写第二张。

## 非目标（本切片刻意不做）

- **回款腿不在本层实现**：`RecordPayment` 已存在，它要的 ChannelRef/Platform/OrderID
  是渠道事实，编排层拿不到也不该伪造。本层在 `bill_derived` 停住，把 `LTCPaymentCallout`
  带在结果里，由接线层调 `service.RecordPayment` 完成最后一条腿。
- **不建新表、不改 middleware**：`ltc_gate*.go` 一个字节不改，shadow/block 与四类失败码用例保持绿。
- **不重写任何一格的业务算法**：金额复用 `quoteSumAmount`，账单号复用 `newBillKey`/`nextBillSeq`，
  CAS 语义复用 `repository.ErrQuoteStatusConflict` / `ErrBillAlreadyDerived`。
