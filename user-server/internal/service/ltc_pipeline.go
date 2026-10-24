// ltc_pipeline.go D1「Agentic Lead-to-Cash 全流程闭环」的编排层。
//
// 一句话职责：把 T-P6-02（AI 报价草稿）、T-P6-03（发送/审批）、T-P7-01（账单派生）、
// 旧订单域（OrderService）四段已经各自能跑的腿，串成一条**可推进、每步都过阶段门**的闭环：
//
//	quote_draft → price_confirmed → order_created → bill_derived →（回款，调用点）
//
// 为什么是编排层而不是又一条写路径：四段各自已有唯一写入口与幂等判据（报价 accepted 的
// 生产写入口在 bill.go、订单在 order.go、账单在 bill.go、回款在 payment.go），再写第五个
// 写路径等于把「唯一入口」这句承诺拆掉。所以本层**不重写任何一格的业务算法**，只做三件事：
//
//  1. 状态机：读域内几行，推出这条报价链现在走到哪一格（FlowStage）；
//  2. 门判定：每一步推进之前，用 ltc.config 的域阶段门（quote / bill）判一次；
//  3. 落库调用：确认（sent→accepted 的 CAS）、转订单、开账单，全部通过注入的窄接口。
//
// 状态机与 ltc_gate.go 的闸门是**两件事、同一份判据**：闸门是 HTTP 层「请求进 handler 之前」
// 的拦截（shadow / block 两态，四类失败码），本层是服务内「一次推进动作之前」的判定，
// 两侧都调 LTCConfig.StageActive 这一个函数，所以「运营点一下开关」在两侧同时生效，
// 不会出现「路由放行、编排拦下」或反过来。本层不碰 middleware 的 shadow 计数与四类失败码，
// 那两个文件一个字节都不改。
//
// 回款刻意不在本层实现：RecordPayment 已经实现入库（payment.go），它要的 ChannelRef /
// Platform / OrderID 是**渠道事实**，编排层拿不到也不该伪造。本层在 bill_derived 之后停住，
// 把 LTCPaymentCallout 带在结果里，由接线层（webhook / 控制器）拿渠道回调的事实调
// service.RecordPayment 完成最后一条腿（论证见 specs/011 的 delivery.md）。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// LTCFlowStage 编排状态机的阶段名。它是**派生态**：不从任何一张表里读出来，
// 由 FlowStage 根据报价链最新版的状态 + 订单/账单的存在性推出来。
//
// 与 LTCStage（ltc.config 的六阶段域门）刻意不共用一个类型：一个是「运营开关按域划」，
// 一个是「这条链走到哪一步」。共用一个类型会把「订单」这种六阶段里根本不存在的步骤
// 硬塞进 LTCStage，而那正是六阶段值域校验要拦的形状。
type LTCFlowStage string

const (
	// LTCFlowQuoteDraft 价格尚未被客户确认。覆盖 quotes.status 的 draft（AI 报价草稿 /
	// 还价草稿）与 sent（已发出、等客户确认）两格 —— 对编排而言，它们都还停在
	// 「没有一张应收依据」的那一侧。sent 那一步（含审批门）是 QuoteSendService 的职责，
	// 本层不越权发报价。
	LTCFlowQuoteDraft LTCFlowStage = "quote_draft"
	// LTCFlowPriceConfirmed 改价确认完成：链上最新版已是 accepted，但订单与账单都还没落。
	LTCFlowPriceConfirmed LTCFlowStage = "price_confirmed"
	// LTCFlowOrderCreated 已从确认版转出订单，账单还没开。
	LTCFlowOrderCreated LTCFlowStage = "order_created"
	// LTCFlowBillDerived 已开出账单（应收）。本层能推进的终点；再往后是回款腿。
	LTCFlowBillDerived LTCFlowStage = "bill_derived"
)

// ltcFlowStageOrder 按推进顺序排列；下标关系就是「合法跃迁 = 恰向前一步」。
var ltcFlowStageOrder = []LTCFlowStage{
	LTCFlowQuoteDraft, LTCFlowPriceConfirmed, LTCFlowOrderCreated, LTCFlowBillDerived,
}

// LTCPipelineStageKnown 报告 s 是否是值域内的阶段名（逐字比，不规范化）。
func LTCPipelineStageKnown(s LTCFlowStage) bool {
	for _, st := range ltcFlowStageOrder {
		if st == s {
			return true
		}
	}
	return false
}

// ltcFlowIndexOf 阶段在推进顺序上的位置；未知返回 -1。
func ltcFlowIndexOf(s LTCFlowStage) int {
	for i, st := range ltcFlowStageOrder {
		if st == s {
			return i
		}
	}
	return -1
}

// LTCPipelineNextStage 返回 from 之后唯一合法的下一格；from 是终点（或未知）时 ok=false。
//
// 「恰向前一步」是刻意的：每一格都是一道门，跳格 = 某一格的证据从未产生过
// （与商机阶段同一条判据）。回退没有合法边 —— 订单/账单都是凭证表，回退等于改史。
func LTCPipelineNextStage(from LTCFlowStage) (LTCFlowStage, bool) {
	i := ltcFlowIndexOf(from)
	if i < 0 || i+1 >= len(ltcFlowStageOrder) {
		return "", false
	}
	return ltcFlowStageOrder[i+1], true
}

// LTCPipelineTransitionLegal 报告 from→to 是否是状态机里的一条边。
func LTCPipelineTransitionLegal(from, to LTCFlowStage) bool {
	next, ok := LTCPipelineNextStage(from)
	return ok && next == to
}

// LTCPipelineGateOf 返回「推进到 target 这一步」要过的域阶段门。
//
// 映射是 D1 的灰度口径（plan 文档「按 opportunity→quote→bill 分阶段门」）：
//   - price_confirmed 挂 quote 门：确认价格是报价域的动作；
//   - order_created   挂 quote 门：订单不在 LTC 六阶段里，转订单是报价域的收口，
//     挂 quote 门而不是 bill 门 —— 否则「没开账单」会被误读成「没开订单」；
//   - bill_derived    挂 bill 门：开应收是账单域的动作。
//
// 回款那一格不在本状态机里，它由 payment 阶段的闸门与 webhook 自己管（见 delivery.md）。
func LTCPipelineGateOf(target LTCFlowStage) (LTCStage, bool) {
	switch target {
	case LTCFlowPriceConfirmed, LTCFlowOrderCreated:
		return LTCStageQuote, true
	case LTCFlowBillDerived:
		return LTCStageBill, true
	}
	return "", false
}

// —— 错误哨兵：每种对应一种修法不同的失败 ——————————————————————————————

var (
	// ErrLTCPipelineUnavailable 依赖缺件（部署的事）。
	ErrLTCPipelineUnavailable = errors.New("ltc pipeline: 编排服务未装配（报价读口 / 订单写口 / 账单写口 / 闸门任一缺失）")
	// ErrLTCPipelineInputInvalid 入参不合法（调用方的事：改载荷）。
	ErrLTCPipelineInputInvalid = errors.New("ltc pipeline: 入参不合法")
	// ErrLTCPipelineQuoteNotFound 报价链不存在（拿错了逻辑号）。
	ErrLTCPipelineQuoteNotFound = errors.New("ltc pipeline: 报价链不存在")
	// ErrLTCPipelineQuoteTerminal 链上最新版已 rejected / expired，编排放行不了它。
	ErrLTCPipelineQuoteTerminal = errors.New("ltc pipeline: 报价链已到终态，无法推进")
	// ErrLTCPipelineQuoteNotSent 要确认的最新版还是 draft：没发出去的报价谈不上客户确认。
	ErrLTCPipelineQuoteNotSent = errors.New("ltc pipeline: 链上最新版尚未发出，不能确认改价")
	// ErrLTCPipelineLinesMissing 版本行在库里而行项目一条都没有（数据事故，非入参问题）。
	ErrLTCPipelineLinesMissing = errors.New("ltc pipeline: 该版报价没有行项目，金额无从算出")
	// ErrLTCPipelineStatusStuck 状态写不动（CAS 落败且重读也补不回来）。
	ErrLTCPipelineStatusStuck = errors.New("ltc pipeline: 状态没能落到该落的位置，本次未推进")
	// ErrLTCPipelineOrderExists 同一条链已经转出过订单。编排侧幂等，由调用方复用那一笔。
	ErrLTCPipelineOrderExists = errors.New("ltc pipeline: 该报价链已转出订单")
)

// —— 端口接口：本层对存储的全部依赖，方法集收窄到「这一步真的用得到」 ————————————

// ltcPipelineQuoteStore 报价链读口 + 确认那一步的 CAS 写口。
// 没有 Create / Append / AddLines：编排放行不了「造一版报价」这件事（那是 QuoteService）。
type ltcPipelineQuoteStore interface {
	Latest(ctx context.Context, quoteID string) (*model.Quote, error)
	ListLines(ctx context.Context, quoteRowID string) ([]*model.QuoteLineItem, error)
	UpdateStatus(ctx context.Context, id, from, to string) error
}

// ltcPipelineOrderStore 转订单这一步的写口与幂等读口。
//
// 刻意不直接钉 model.Order：那张表没有 quote 引用列、价格列是字符串、account_id 是
// varchar(36) —— 三处都与报价域对不齐。把 model.Order 钉进编排层签名，等于把
// 「订单表怎么补列」这个接线决策藏进 service 层。生产接线由适配器把 LTCPipelineOrder
// 映射到 model.Order（或补 quote 引用列），见 delivery.md 的决策点。
type ltcPipelineOrderStore interface {
	GetByQuoteRowID(ctx context.Context, quoteRowID string) (*LTCPipelineOrder, error)
	CreatePipelineOrder(ctx context.Context, in LTCPipelineOrder) (*LTCPipelineOrder, error)
}

// ltcPipelineBillStore 账单写口与幂等读口（repository.BillRepository 结构上满足其真子集）。
type ltcPipelineBillStore interface {
	GetByQuoteRowID(ctx context.Context, quoteRowID string) (*model.Bill, error)
	Create(ctx context.Context, b *model.Bill) error
}

// ltcPaymentRecorder 是回款腿在编排层看到的唯一形状（*PaymentService 结构上满足）。
// 本编排不实现回款，只把调用点收窄成这一个方法；接线的 webhook 拿着 LTCPaymentCallout
// 与渠道事实调它即可。这是「留出调用点」的编译期证据，不是本层对回款腿的替代。
type ltcPaymentRecorder interface {
	RecordPayment(ctx context.Context, in RecordPaymentInput) (*PaymentReceipt, error)
}

var _ ltcPaymentRecorder = (*PaymentService)(nil)

// —— 结果形状 ————————————————————————————————————————————————

// LTCPipelineOrder 转订单这一步产出的订单（编排层视图，不是 model.Order 本身）。
type LTCPipelineOrder struct {
	ID            string  `json:"id"`
	QuoteID       string  `json:"quote_id"`
	QuoteRowID    string  `json:"quote_row_id"`
	OpportunityID string  `json:"opportunity_id"`
	Amount        float64 `json:"amount"`
	Currency      string  `json:"currency"`
	LineCount     int     `json:"line_count"`
}

// LTCPaymentCallout 回款调用点：bill_derived 之后，编排层能给出的、调 RecordPayment 所需的
// 全部「链上事实」。ChannelRef / Platform / OrderID 是渠道事实，不在这里 —— 由接线层
// 从支付回调里取，与本结构拼成一条完整的 RecordPaymentInput（见 RecordPaymentInputFor）。
type LTCPaymentCallout struct {
	BillID     string  `json:"bill_id"`
	QuoteID    string  `json:"quote_id"`
	QuoteRowID string  `json:"quote_row_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
}

// RecordPaymentInputFor 把回款调用点 + 渠道事实拼成 service.RecordPayment 的入参。
//
// 这一步只拼参数、不调 RecordPayment：回款腿的调用权在接线层。放在这里而不是让调用方
// 手拼，是为了让「账单号、金额、币种从调用点来」这件事有一个类型化的出口 ——
// 手拼的话，下一次有人会拿错账单号而两边的编译期都看不见。
func (c LTCPaymentCallout) RecordPaymentInputFor(channelRef, platform, orderID string, paidAt *time.Time) RecordPaymentInput {
	return RecordPaymentInput{
		BillID: c.BillID, ChannelRef: channelRef, Amount: c.Amount, Currency: c.Currency,
		PaidAt: paidAt, Platform: platform, OrderID: orderID,
	}
}

// LTCAdvanceResult 一次推进的完整结论。
//
// Advanced 与 GateReason 是两个维度：Advanced=false 可能是「已经到终点了」（GateReason 空），
// 也可能是「门关着，本次没动」（GateReason 是 LTCReason* 之一）。合成一个字段的话，
// 调用方会为「等运营开开关」写 retry，把一次配置问题变成三次轮询。
type LTCAdvanceResult struct {
	Stage      LTCFlowStage       `json:"stage"`
	Advanced   bool               `json:"advanced"`
	GateReason string             `json:"gate_reason,omitempty"`
	GateDetail string             `json:"gate_detail,omitempty"`
	Order      *LTCPipelineOrder  `json:"order,omitempty"`
	Bill       *model.Bill        `json:"bill,omitempty"`
	Payment    *LTCPaymentCallout `json:"payment,omitempty"`
}

// —— 服务 ————————————————————————————————————————————————

// LTCPipelineService 编排服务。依赖全部注入，本层不自己去拿全局 DB。
type LTCPipelineService struct {
	quotes ltcPipelineQuoteStore
	orders ltcPipelineOrderStore
	bills  ltcPipelineBillStore
	gate   LTCConfigReader

	now func() time.Time
	// trace 全链路埋点写口：nil = 没装，推进照常，不阻塞。
	trace salesTraceWriter
}

// NewLTCPipelineService 构造。缺件时构造照旧成功，由 Available / Advance 报出来 ——
// 装配点要能在没有订单/账单存储的环境里起来，但绝不能因此「静默不推进」。
func NewLTCPipelineService(quotes ltcPipelineQuoteStore, orders ltcPipelineOrderStore,
	bills ltcPipelineBillStore, gate LTCConfigReader) *LTCPipelineService {
	return &LTCPipelineService{quotes: quotes, orders: orders, bills: bills, gate: gate, now: time.Now}
}

// SetClock 注入时钟（订单/账单号里的纳秒与 created_at 共用它）。
func (s *LTCPipelineService) SetClock(now func() time.Time) {
	if s == nil {
		return
	}
	if now == nil {
		now = time.Now
	}
	s.now = now
}

// SetSalesTrace 注入链路事件写口。传 nil 表示不埋点（测试/轻装配可用）。
func (s *LTCPipelineService) SetSalesTrace(w salesTraceWriter) {
	if s == nil {
		return
	}
	s.trace = w
}

// Available 报告能不能推进。闸门配置**不在**这一条里（nil 那份按降级=关处理，
// 那是运营的选择不是故障），但 gate 这个端口缺件是装配事故，必须报得出来。
func (s *LTCPipelineService) Available() bool {
	return s != nil && s.quotes != nil && s.orders != nil && s.bills != nil && s.gate != nil
}

// FlowStage 读域内几行，推出这条报价链当前停在哪一格（纯读，零写入）。
//
// 推导规则（只认「链上最新版」这一个事实源）：
//   - latest.status ∈ {draft, sent} ⇒ quote_draft；
//   - latest.status == accepted ⇒ 看账单、再看订单，有账单 = bill_derived、
//     有订单 = order_created、都没有 = price_confirmed；
//   - latest.status ∈ {rejected, expired} ⇒ 报 ErrLTCPipelineQuoteTerminal；
//   - 其余（值域外）⇒ 报 ErrLTCPipelineStatusStuck（与「终态」分开：那是数据事故）。
func (s *LTCPipelineService) FlowStage(ctx context.Context, quoteID string) (LTCFlowStage, error) {
	if !s.Available() {
		return "", ErrLTCPipelineUnavailable
	}
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return "", err
	}
	switch latest.Status {
	case model.QuoteStatusDraft, model.QuoteStatusSent:
		return LTCFlowQuoteDraft, nil
	case model.QuoteStatusAccepted:
		return s.acceptedFlowStage(ctx, latest)
	case model.QuoteStatusRejected, model.QuoteStatusExpired:
		return "", fmt.Errorf("%w: 链 %s 最新版 v%d 是 %s（报价已被拒或过期，闭环到此为止，另起一条链）",
			ErrLTCPipelineQuoteTerminal, quoteID, latest.Version, latest.Status)
	default:
		return "", fmt.Errorf("%w: 链 %s 最新版 v%d 的状态 %q 不在值域内", ErrLTCPipelineStatusStuck, quoteID, latest.Version, latest.Status)
	}
}

// acceptedFlowStage accepted 之后的三格，按「先账单后订单」的顺序判。
//
// 先看账单是因为账单带 quote_row_id 唯一索引（幂等键真实存在于库），订单那张表今天
// 没有 quote 引用列 —— 账单是更硬的事实源。
func (s *LTCPipelineService) acceptedFlowStage(ctx context.Context, latest *model.Quote) (LTCFlowStage, error) {
	bill, err := s.bills.GetByQuoteRowID(ctx, latest.ID)
	if err != nil {
		return "", fmt.Errorf("ltc pipeline: 读报价行 %s 对应的账单失败：%w", latest.ID, err)
	}
	if bill != nil {
		return LTCFlowBillDerived, nil
	}
	order, err := s.orders.GetByQuoteRowID(ctx, latest.ID)
	if err != nil {
		return "", fmt.Errorf("ltc pipeline: 读报价行 %s 对应的订单失败：%w", latest.ID, err)
	}
	if order != nil {
		return LTCFlowOrderCreated, nil
	}
	return LTCFlowPriceConfirmed, nil
}

// Advance 把这条链向前推进**恰好一步**（阶段推进 + 门判定 + 落库调用）。
//
// 返回而不报错的三种形状必须分得开（都回 error 会让调用方写 retry）：
//   - Advanced=true：推进了一步，Stage 是新的那一格；
//   - Advanced=false 且 GateReason 空：已经到了本层能推进的终点（bill_derived）；
//   - Advanced=false 且 GateReason 非空：域阶段门关着，本次一行都没写。
//
// error 只属于真故障：未装配、入参坏、链不存在、链终态、未发出、明细缺、CAS 写不动。
func (s *LTCPipelineService) Advance(ctx context.Context, quoteID string) (*LTCAdvanceResult, error) {
	if !s.Available() {
		return nil, ErrLTCPipelineUnavailable
	}
	quoteID = strings.TrimSpace(quoteID)
	if quoteID == "" {
		return nil, fmt.Errorf("%w: 报价逻辑号为空（编排推进的是「一条链」，必须指到 quote_id）", ErrLTCPipelineInputInvalid)
	}

	stage, err := s.FlowStage(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	next, ok := LTCPipelineNextStage(stage)
	if !ok {
		// 终点：把「现在就能回款」的调用点补进结果，零写入。
		return s.terminalResult(ctx, stage, quoteID)
	}

	gateStage, known := LTCPipelineGateOf(next)
	if !known {
		return nil, fmt.Errorf("%w: 阶段 %q 没有配域阶段门（装配错误）", ErrLTCPipelineStatusStuck, next)
	}
	if reason, detail := ltcPipelineGateDetail(ctx, s.gate, gateStage); reason != "" {
		return &LTCAdvanceResult{Stage: stage, Advanced: false, GateReason: reason, GateDetail: detail}, nil
	}

	switch next {
	case LTCFlowPriceConfirmed:
		if err := s.confirmPrice(ctx, quoteID); err != nil {
			return nil, err
		}
	case LTCFlowOrderCreated:
		if err := s.createOrder(ctx, quoteID); err != nil {
			return nil, err
		}
	case LTCFlowBillDerived:
		if err := s.deriveBill(ctx, quoteID); err != nil {
			return nil, err
		}
	}

	return s.resultAfter(ctx, quoteID, true)
}

// Run 从当前格一路推到底，或停在第一道关着的门之前。
//
// 它是 Advance 的「闭环」封装：调用方（接线层 / 定时任务）只需点一次 Run。
// 门关着时**不回 error**（那是运营的选择，不是故障），结果里 Stage 停在没过去的那一格、
// GateReason 说明哪道门。真故障（链没了 / 明细缺 / CAS 写不动）原样上抛，
// 已推进的部分通过返回的 result 带出，不回滚（订单/账单都是凭证表，回滚等于改史）。
func (s *LTCPipelineService) Run(ctx context.Context, quoteID string) (*LTCAdvanceResult, error) {
	if !s.Available() {
		return nil, ErrLTCPipelineUnavailable
	}
	var last *LTCAdvanceResult
	for range ltcFlowStageOrder {
		r, err := s.Advance(ctx, quoteID)
		if err != nil {
			return last, err
		}
		last = r
		if !r.Advanced {
			return r, nil
		}
	}
	return last, nil
}

// requireLatest 读链上最新版；链不存在回 ErrLTCPipelineQuoteNotFound。
func (s *LTCPipelineService) requireLatest(ctx context.Context, quoteID string) (*model.Quote, error) {
	latest, err := s.quotes.Latest(ctx, quoteID)
	if err != nil {
		return nil, fmt.Errorf("ltc pipeline: 读报价链 %s 的最新版失败：%w", quoteID, err)
	}
	if latest == nil {
		return nil, fmt.Errorf("%w: %s", ErrLTCPipelineQuoteNotFound, quoteID)
	}
	return latest, nil
}

// confirmPrice 改价确认：latest sent → accepted 的 CAS。
//
// 只读判据在前、CAS 在后，与 bill.go 派生腿同一取向：判据没过 = 一行不写。
// 已经 accepted 的直接返回（幂等 / 并发对手已赢，零写入）。
func (s *LTCPipelineService) confirmPrice(ctx context.Context, quoteID string) error {
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return err
	}
	if latest.Status == model.QuoteStatusAccepted {
		return nil // 重入 / 并发对手已确认
	}
	if latest.Status != model.QuoteStatusSent {
		return fmt.Errorf("%w: 链 %s 最新版 v%d 是 %s（确认 = sent→accepted，draft 必须先经发送腿）",
			ErrLTCPipelineQuoteNotSent, quoteID, latest.Version, latest.Status)
	}
	if err := s.quotes.UpdateStatus(ctx, latest.ID, model.QuoteStatusSent, model.QuoteStatusAccepted); err != nil {
		if !errors.Is(err, repository.ErrQuoteStatusConflict) {
			return fmt.Errorf("%w（链 %s v%d sent→accepted）：%w", ErrLTCPipelineStatusStuck, quoteID, latest.Version, err)
		}
		// CAS 落败 = 有人抢先确认。重读一次：已经是 accepted 就当成这次也完成了（幂等），
		// 否则说明行被改成别的形状，报出来让人看。
		again, gerr := s.quotes.Latest(ctx, quoteID)
		if gerr != nil || again == nil || again.Status != model.QuoteStatusAccepted {
			return fmt.Errorf("%w（链 %s 的确认落败且重读不到 accepted，须人工核对）", ErrLTCPipelineStatusStuck, quoteID)
		}
	}
	emitSalesTrace(ctx, s.trace, &model.SalesEvent{
		EventType:     model.SalesEventTypeQuote,
		Action:        "accepted",
		OpportunityID: latest.OpportunityID,
		QuoteID:       latest.QuoteID,
		Amount:        quoteSumAmount(mustLines(ctx, s, latest)),
		OccurredAt:    s.now(),
	}, "ltc_pipeline_confirm_price")
	return nil
}

// createOrder 自动转订单：从确认版的那些行拼出订单，交给注入的写口。
func (s *LTCPipelineService) createOrder(ctx context.Context, quoteID string) error {
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return err
	}
	if latest.Status != model.QuoteStatusAccepted {
		return fmt.Errorf("%w: 链 %s 最新版 v%d 是 %s，不是 accepted，不能转订单",
			ErrLTCPipelineStatusStuck, quoteID, latest.Version, latest.Status)
	}
	lines, err := s.quotes.ListLines(ctx, latest.ID)
	if err != nil {
		return fmt.Errorf("ltc pipeline: 读报价行 %s 的行项目失败：%w", latest.ID, err)
	}
	if len(lines) == 0 {
		return fmt.Errorf("%w: 报价行 %s 在库里而明细一条都没有（合计会算成 0.00 转进订单）",
			ErrLTCPipelineLinesMissing, latest.ID)
	}
	order := LTCPipelineOrder{
		ID:            newPipelineOrderKey(s.now(), nextPipelineOrderSeq()),
		QuoteID:       latest.QuoteID,
		QuoteRowID:    latest.ID,
		OpportunityID: latest.OpportunityID,
		Amount:        quoteSumAmount(lines),
		Currency:      pipelineCurrencyOf(latest.Currency),
		LineCount:     len(lines),
	}
	stored, err := s.orders.CreatePipelineOrder(ctx, order)
	if err != nil {
		if !errors.Is(err, ErrLTCPipelineOrderExists) {
			return fmt.Errorf("ltc pipeline: 转订单失败（链 %s 仍停在 price_confirmed，重入这一动作可修）：%w", quoteID, err)
		}
		stored, err = s.orders.GetByQuoteRowID(ctx, latest.ID)
		if err != nil || stored == nil {
			return fmt.Errorf("%w: 链 %s 撞到「已转出订单」后回读不到那一笔，须人工核对", ErrLTCPipelineStatusStuck, quoteID)
		}
	}
	emitSalesTrace(ctx, s.trace, &model.SalesEvent{
		EventType:     model.SalesEventTypeOrder,
		Action:        "created",
		OpportunityID: stored.OpportunityID,
		QuoteID:       stored.QuoteID,
		OrderID:       stored.ID,
		Amount:        stored.Amount,
		OccurredAt:    s.now(),
	}, "ltc_pipeline_create_order")
	return nil
}

// deriveBill 生成账单：从确认版开一张应收。金额只有行项目相加这一个来源（quoteSumAmount，
// 与报价发送腿、bill.go 同一个函数），所以「账单金额 = 报价合计」不是两条数据比对，
// 而是同一个算法的同一个结果。
func (s *LTCPipelineService) deriveBill(ctx context.Context, quoteID string) error {
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return err
	}
	if latest.Status != model.QuoteStatusAccepted {
		return fmt.Errorf("%w: 链 %s 最新版 v%d 是 %s，不是 accepted，不能开账单",
			ErrLTCPipelineStatusStuck, quoteID, latest.Version, latest.Status)
	}
	if existing, err := s.bills.GetByQuoteRowID(ctx, latest.ID); err != nil {
		return fmt.Errorf("ltc pipeline: 读报价行 %s 对应的账单失败：%w", latest.ID, err)
	} else if existing != nil {
		return nil // 幂等重入：账单已在，零写入
	}
	lines, err := s.quotes.ListLines(ctx, latest.ID)
	if err != nil {
		return fmt.Errorf("ltc pipeline: 读报价行 %s 的行项目失败：%w", latest.ID, err)
	}
	if len(lines) == 0 {
		return fmt.Errorf("%w: 报价行 %s 在库里而明细一条都没有（应收合计无从算出）",
			ErrLTCPipelineLinesMissing, latest.ID)
	}
	now := s.now()
	bill := &model.Bill{
		ID: newBillKey(now, nextBillSeq()), QuoteID: latest.QuoteID, QuoteRowID: latest.ID,
		OpportunityID: latest.OpportunityID, Amount: quoteSumAmount(lines),
		Currency: pipelineCurrencyOf(latest.Currency),
		// 账期不进编排：报价域没有「付款条件」这一格，与 bill.go 的 DueAt 判据同源 ——
		// 要定账期的是在场的人（合同条款），不是这一条自动腿。
		DueAt:     nil,
		Status:    model.BillStatusOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.bills.Create(ctx, bill); err != nil {
		if !errors.Is(err, repository.ErrBillAlreadyDerived) {
			return fmt.Errorf("ltc pipeline: 派生账单失败（链 %s 的报价行 %s 已记为 accepted，重入这一动作可修）：%w",
				quoteID, latest.ID, err)
		}
		again, gerr := s.bills.GetByQuoteRowID(ctx, latest.ID)
		if gerr != nil || again == nil {
			return fmt.Errorf("%w: 链 %s 撞到「已派生」后回读不到账单，须人工核对", ErrLTCPipelineStatusStuck, quoteID)
		}
		return nil
	}
	emitSalesTrace(ctx, s.trace, &model.SalesEvent{
		EventType:     model.SalesEventTypeBill,
		Action:        "created",
		OpportunityID: bill.OpportunityID,
		QuoteID:       bill.QuoteID,
		Amount:        bill.Amount,
		OccurredAt:    now,
	}, "ltc_pipeline_derive_bill")
	return nil
}

// terminalResult 终点（bill_derived）的结果：补出账单与回款调用点，零写入。
func (s *LTCPipelineService) terminalResult(ctx context.Context, stage LTCFlowStage, quoteID string) (*LTCAdvanceResult, error) {
	res := &LTCAdvanceResult{Stage: stage, Advanced: false}
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	bill, err := s.bills.GetByQuoteRowID(ctx, latest.ID)
	if err != nil {
		return nil, fmt.Errorf("ltc pipeline: 读报价行 %s 对应的账单失败：%w", latest.ID, err)
	}
	if bill != nil {
		res.Bill = bill
		res.Payment = &LTCPaymentCallout{
			BillID: bill.ID, QuoteID: bill.QuoteID, QuoteRowID: bill.QuoteRowID,
			Amount: bill.Amount, Currency: bill.Currency,
		}
	}
	if order, err := s.orders.GetByQuoteRowID(ctx, latest.ID); err != nil {
		return nil, fmt.Errorf("ltc pipeline: 读报价行 %s 对应的订单失败：%w", latest.ID, err)
	} else if order != nil {
		res.Order = order
	}
	return res, nil
}

// resultAfter 推进之后重读一遍状态与产物，返回「库里说的」而不是「内存里以为的」。
func (s *LTCPipelineService) resultAfter(ctx context.Context, quoteID string, advanced bool) (*LTCAdvanceResult, error) {
	stage, err := s.FlowStage(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	res := &LTCAdvanceResult{Stage: stage, Advanced: advanced}
	latest, err := s.requireLatest(ctx, quoteID)
	if err != nil {
		return nil, err
	}
	bill, err := s.bills.GetByQuoteRowID(ctx, latest.ID)
	if err != nil {
		return nil, fmt.Errorf("ltc pipeline: 读报价行 %s 对应的账单失败：%w", latest.ID, err)
	}
	if bill != nil {
		res.Bill = bill
		res.Payment = &LTCPaymentCallout{
			BillID: bill.ID, QuoteID: bill.QuoteID, QuoteRowID: bill.QuoteRowID,
			Amount: bill.Amount, Currency: bill.Currency,
		}
	}
	if order, err := s.orders.GetByQuoteRowID(ctx, latest.ID); err != nil {
		return nil, fmt.Errorf("ltc pipeline: 读报价行 %s 对应的订单失败：%w", latest.ID, err)
	} else if order != nil {
		res.Order = order
	}
	return res, nil
}

// ltcPipelineGateDetail 一步的门判据（与 quoteStageGateDetail 同源：空串 = 放行）。
// 返回 (reason, detail)：reason 是 LTCReason* 之一，detail 在 degraded 时多带底层原因。
func ltcPipelineGateDetail(ctx context.Context, reader LTCConfigReader, stage LTCStage) (string, string) {
	cfg := reader.Config(ctx)
	active, reason := cfg.StageActive(stage)
	if active {
		return "", ""
	}
	detail := reason
	if cfg != nil && cfg.DegradeReason != "" {
		detail += "：" + cfg.DegradeReason
	}
	return reason, detail
}

// pipelineCurrencyOf 币种兜底（与 bill.go 的同一格判据：金额不带币种不可算）。
func pipelineCurrencyOf(currency string) string {
	currency = strings.TrimSpace(currency)
	if currency == "" {
		return model.BillCurrencyDefault
	}
	return currency
}

// mustLines 读并保证行项目非空（只在 confirmPrice 的埋点里用：埋点缺行项目不该吞掉
// 主线错误，所以最坏也只是金额写 0，主流程不受影响 —— 但为保持单源，仍走同一读口）。
func mustLines(ctx context.Context, s *LTCPipelineService, row *model.Quote) []*model.QuoteLineItem {
	lines, err := s.quotes.ListLines(ctx, row.ID)
	if err != nil || len(lines) == 0 {
		return nil
	}
	return lines
}

// —— 主键生成（订单侧）————————————————————————————————————————

// pipelineOrderSeq 进程内单调计数器，口径与 billSeq / quoteSeq 完全一致。
var pipelineOrderSeq int64

func nextPipelineOrderSeq() int64 { return atomic.AddInt64(&pipelineOrderSeq, 1) }

// newPipelineOrderKey 纯函数版生成器（同一时刻 + 同一 seq ⇒ 同一个号），用例可直接复算。
//
// 前缀 o_ 与 q_ / b_ / p_ 同族。注意：生产接线的订单适配器若走 model.Order 的
// BeforeCreate 钩子，ID 会被 uuid 覆盖 —— 本层生成 ID 只为编排结果可读与用例可断言，
// 不承诺它是 orders 表的最终主键（决策点见 delivery.md）。
func newPipelineOrderKey(now time.Time, seq int64) string {
	return fmt.Sprintf("o_%d_%d", now.UnixNano(), seq)
}
