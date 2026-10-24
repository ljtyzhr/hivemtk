# Delivery Report: D1 Agentic Lead-to-Cash 全流程闭环实挂

**Feature**: `011-lead-to-cash-closure` ｜ **交付日期**: 2026-10-24 ｜ **Spec/Plan/Tasks**: 同目录

## 一句话

把「报价草稿 → 改价确认 → 自动转订单/开账单 → 回款调用点」串成一条**每步都过阶段门**的
编排闭环，不建新表、不改 middleware、不重写任何一格的业务算法，回款腿只留调用点。

## 改动清单

| 类型 | 文件 | 说明 |
|---|---|---|
| 新增 | `user-server/internal/service/ltc_pipeline.go` | ~660 行。状态机 `LTCFlowStage`（4 格 + 3 条向前边）、域门映射 `LTCPipelineGateOf`、8 个错误哨兵、4 个端口接口、`LTCPipelineService`（FlowStage/Advance/Run）、`LTCPaymentCallout` 回款调用点。中文注释说明每个「为什么」 |
| 新增 | `user-server/internal/service/ltc_pipeline_test.go` | ~760 行。纯内存假仓储（quote CAS 带并发钩子、order/bill 带 race 钩子）+ 18 个测试函数，覆盖状态机/推导/闭环/门关零写入/失败哨兵/并发愈合/Run/纯函数/接口收窄 |
| 新增 | `specs/011-lead-to-cash-closure/{spec,plan,tasks,delivery}.md` | 四件套 |

未动任何他人文件；`ltc_gate*.go` 一个字节未改（shadow/block 与四类失败码用例保持原样）。

## 测试命令与结果（本轮实测）

```bash
cd user-server
gofmt -l internal/service/ltc_pipeline.go internal/service/ltc_pipeline_test.go
# → 无输出（干净）

go test ./internal/service -run '^TestLTCPipeline' -count=1
# → ok  hivemtk-user/internal/service  1.512s
#   18 个测试函数全部 PASS（含多个表驱动子用例组）

go build ./...
# → exit 0

go vet ./internal/service
# → exit 0
```

注：`go test ./internal/service -run LTC` 会顺带命中既有的 `TestLTCConfig_*` 真库用例并因
本机 PG 密码不同而失败，故用 `-run '^TestLTCPipeline'` 只跑本切片用例；`go build`/`go vet`
全量干净。

## 决策点（接线层必须知道）

1. **订单表无 quote 引用列**：编排用 DTO `LTCPipelineOrder` 表达订单，不直钉 `model.Order`。
   生产接线需在适配器里做 `LTCPipelineOrder → model.Order` 映射，或给订单表补 quote 引用列
   （补列归建表登记 owner，本层不越权）。
2. **AccountID varchar(36) 与商机 CustomerID varchar(64) 对不齐**：转订单的归属键两端宽度不一，
   接线时需定归属语义（按报价的 `OpportunityID` 走，还是按商机的 `CustomerID` 走）。
3. **accepted 双写入口张力**：编排的 `confirmPrice`（sent→accepted CAS）与 `BillService.DeriveFromQuote`
   的确认腿存在重叠。本层没有另开「accepted 的生产写入口」（只做 CAS 确认），但接线层要拍板：
   「确认改价」是否统一收敛到编排层，还是两处并存（幂等键 `bills.quote_row_id` 保证并存也不多开账单）。
4. **账单 DueAt 在编排中恒 nil**：报价域没有「付款条件」字段（`valid_days` 是报价有效期不是账期），
   要定账期的是在场的人。T-P7-03 的逾期扫描判据已把「账期未定」单独数出来，不参与扫描。
5. **订单 ID 生成仅编排层可读**：`newPipelineOrderKey` 产 `o_<unixnano>_<seq>`，但若生产适配器
   走 `model.Order` 的 `BeforeCreate` 钩子，ID 会被 uuid 覆盖。本层生成 ID 只为结果可读与用例可断言，
   不承诺它是 orders 表最终主键。
6. **回款腿只留调用点**：`LTCPaymentCallout.RecordPaymentInputFor(channelRef, platform, orderID, paidAt)`
   把链上事实（账单号/金额/币种）与渠道事实（回调路径上的 platform、渠道流水号、外部订单号）
   拼成 `RecordPaymentInput`，由接线层调 `service.RecordPayment`。`ltcPaymentRecorder` 接口的
   编译期断言 `(*PaymentService)` 满足，是这条调用点真的存在、而非纸面承诺的证据。

## 建议接线点（只建议，不实施）

- **装配**：`NewLTCPipelineService(quotes, orders, bills, gate)` 四个实参分别接
  `repository.QuoteRepository`（结构上满足 quote 端口真子集）、订单适配器、
  `repository.BillRepository`、`LTCConfigService`（满足 `LTCConfigReader`）。
- **入口**：webhook 或定时任务调一次 `Run(quoteID)`；运营关门的语义由 `GateReason` 带出，
  不需要接线层写重试。
- **回款**：支付回调 handler 拿 `Advance/Run` 结果里的 `Payment` 调用点，填渠道事实后调
  `service.RecordPayment`。
- **路由/控制器/Makefile/CI**：本轮按约定不动；如要暴露 HTTP 入口，建议在 quote 域路由组下
  挂 `LTCStageGate`（与既有 shadow 装配同形）。
- **新表归属**：本切片无新表（状态是派生态），无需 `module_tables.go` 登记。
