# Implementation Plan: D1 Agentic Lead-to-Cash 全流程闭环实挂

**Feature**: `011-lead-to-cash-closure` ｜ 对应 spec/tasks/delivery 同目录

## 设计决策（拍板记录）

### D1 · 新状态机 `LTCFlowStage`，不并入 `LTCStage`

`LTCStage` 是运营按域划的六阶段门（opportunity/outreach/quote/bill/payment/collection），
`LTCFlowStage` 是「这条链走到哪一步」的派生态。两者**刻意不共用一个类型**：
把 `order_created`（六阶段里不存在的步骤）硬塞进 `LTCStage`，正是六阶段值域校验要拦的形状。

状态机只有一条向前边：`quote_draft → price_confirmed → order_created → bill_derived`，
恰向前一步、回退无合法边（订单/账单都是凭证表，回退 = 改史）。

### D2 · 域阶段门映射

| 推进目标 | 过的门 | 理由 |
|---|---|---|
| price_confirmed | quote | 确认改价是报价域的动作 |
| order_created | quote | 订单不在 LTC 六阶段，转订单是报价域收口；挂 bill 门会「没开账单」被误读成「没开订单」 |
| bill_derived | bill | 开应收是账单域的动作 |

回款不在状态机内，只留 callout（payment 阶段的闸门与 webhook 自己管）。

### D3 · 状态是派生态，不建新表

`FlowStage` 读 `quotes.latest.status + 订单存在性 + 账单存在性` 推导，纯读零写入。
先看账单后看订单：账单带 `quote_row_id` 唯一索引（幂等键真实存在于库），
订单表今天没有 quote 引用列——账单是更硬的事实源。

### D4 · 编排层不钉 model.Order，用 DTO `LTCPipelineOrder`

`model.Order` 三处与报价域对不齐：无 quote 引用列、`Price` 是 string、`AccountID` 是
varchar(36)（商机 `CustomerID` 是 varchar(64)）。钉进签名等于把「订单表怎么补列」这个
接线决策藏进 service 层。生产接线由适配器把 `LTCPipelineOrder` 映射到 `model.Order`
（或给订单表补 quote 引用列）。

### D5 · 端口收窄到「这一步真的用得到」

- `ltcPipelineQuoteStore`：只有 Latest / ListLines / UpdateStatus（无 Create/Append/AddLines——编排放行不了造报价）；
- `ltcPipelineOrderStore`：GetByQuoteRowID / CreatePipelineOrder；
- `ltcPipelineBillStore`：GetByQuoteRowID / Create（`repository.BillRepository` 结构上满足其真子集）；
- `ltcPaymentRecorder`：只有 `RecordPayment`，编译期断言 `(*PaymentService)` 满足——这是「留出调用点」的编译期证据，不是替代。

### D6 · 错误哨兵：一种失败一个哨兵

`ErrLTCPipelineUnavailable / InputInvalid / QuoteNotFound / QuoteTerminal / QuoteNotSent /
LinesMissing / StatusStuck / OrderExists` 八种，各自对应一种修法不同的失败。
门关着**不是** error（运营的选择），终态推进到顶**不是** error（`Advanced=false` + 空 GateReason）。

### D7 · API 形状

- `FlowStage(quoteID)`：纯读推导；
- `Advance(quoteID)`：恰一步，门关着 = `Advanced=false` + `GateReason`，不报错；
- `Run(quoteID)`：走到终点或第一道关着的门；真故障上抛，已推进部分不回滚（凭证表）；
- 纯函数 `LTCPipelineNextStage` / `LTCPipelineTransitionLegal` / `LTCPipelineGateOf` /
  `LTCPipelineStageKnown`：状态机本身可独立断言。

## 实施步骤

1. 写 `internal/service/ltc_pipeline.go`：状态机纯函数 + 八哨兵 + 端口接口 + 服务
   （FlowStage/Advance/Run + confirmPrice/createOrder/deriveBill 三段编排 + terminal/resultAfter）。
2. 写 `internal/service/ltc_pipeline_test.go`：内存假仓储（quote CAS 含两个并发钩子、
   order/bill 含 race 钩子），覆盖跃迁表全量、FlowStage 推导、Advance 全闭环、三种门关零写入、
   全部失败哨兵、CAS/幂等愈合与卡死、Run 走到底/停在关着的门、callout 拼参、接口收窄断言。
3. 校验：`gofmt -l`、`go test ./internal/service -run '^TestLTCPipeline'`、`go build ./...`、
   `go vet ./internal/service`（只测自己改的包）。
4. 写 specs/011 四件套（本目录），delivery.md 记测试读数与决策点、建议接线点。

## 明确不碰

`CHANGELOG.md`、docs 计划文档、scripts/*、payment*/opportunity 相关、module_tables.go、
specs/006-010、.github/workflows、`ltc_gate*.go`（一个字节不改）。新表归属只写 delivery.md；
router/controller/Makefile/CI 接线只建议不实施。
