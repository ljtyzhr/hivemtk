# Tasks: D1 Agentic Lead-to-Cash 全流程闭环实挂

**Feature**: `011-lead-to-cash-closure` ｜ 完成标准：纯单测全绿 + build/vet 干净 + 四件套齐。

- [x] T1 调研取证：读四段腿的写入口/幂等判据（quote/quote_send/bill/order/payment）、
      repository 的 CAS 语义、model 三表字段、`LTCStage`/`StageActive`/`LTCReason*`、
      middleware 闸门形状、`LTCConfigReader` 定义处、计划文档 D1 段原文。
- [x] T2 确认无命名冲突：`LTCPipeline` / `LTCFlowStage` 全仓无先占；`ltc_pipeline*.go` 未被并发 lane 改过。
- [x] T3 写 `internal/service/ltc_pipeline.go`：状态机纯函数 + 八哨兵 + 四个端口接口 +
      `LTCPipelineService`（FlowStage/Advance/Run + 三段编排 + terminal/resultAfter +
      `LTCPaymentCallout.RecordPaymentInputFor`）。
- [x] T4 写 `internal/service/ltc_pipeline_test.go`：内存假仓储 + 表驱动，覆盖：
      - 跃迁表全部合法/非法边、`GateOf`/`StageKnown`/`NextStage`；
      - FlowStage 推导（draft/sent/多版本认最新/order/bill/rejected/expired/值域外/不存在）；
      - Advance 全闭环（price_confirmed → order_created → bill_derived → 终点）；
      - 三种门关（master_off/stage_off/config_degraded）零写入、bill 门关单独一例；
      - 全部失败哨兵（NotSent/Terminal/NotFound/InputInvalid/LinesMissing/Unavailable）；
      - CAS 冲突幂等愈合 + 卡死；订单/账单 race 幂等复用；存储故障上抛；
      - Run 走到底（埋点 [order, bill]）、Run 停在关着的门、Run 从 draft 失败零写入；
      - `RecordPaymentInputFor` 拼参、`pipelineCurrencyOf`/`newPipelineOrderKey` 纯函数；
      - 接口收窄编译期断言。
- [x] T5 校验：`gofmt -l`（两文件干净）、`go test ./internal/service -run '^TestLTCPipeline' -count=1`
      （18 个测试函数全过）、`go build ./...`（exit 0）、`go vet ./internal/service`（exit 0）。
- [x] T6 写 specs/011 四件套：spec（已有什么/缺哪一环取证 + US/AC）、plan（7 条决策）、
      tasks（本文件）、delivery（改动清单 + 测试读数 + 决策点 + 建议接线点）。
