// ltc_pipeline_test.go D1「Agentic Lead-to-Cash 全流程闭环」编排层的纯单元测试。
//
// 三条纪律（与 delivery.md 的测试口径一致）：
//   - 纯内存：所有仓储都是本文件里的假实现，不碰 testutil.NewTestDB、不连真库；
//   - 只断「编排层的判据」：推进一步 = 先过门、再落一次库，落库的算法本身
//     （quoteSumAmount / newBillKey / CAS 语义）由各自域的用例负责，这里只断
//     「编排层有没有在正确的时机、用正确的入参去调它们」；
//   - 期望值写字面量：合计 150.5 是手算的（100 + 50.5），不调用被测式子生成。
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"
)

// —— 编译期断言：假实现逐字满足编排层收窄后的端口 ——————————————————————
var (
	_ ltcPipelineQuoteStore = (*pipelineMemQuote)(nil)
	_ ltcPipelineOrderStore = (*pipelineMemOrder)(nil)
	_ ltcPipelineBillStore  = (*pipelineMemBill)(nil)
	_ LTCConfigReader       = pipelineGate{}
	_ salesTraceWriter      = (*pipelineTrace)(nil)
)

// —— 假报价仓储（只实现 Latest / ListLines / UpdateStatus 三格）————————————
//
// UpdateStatus 是 CAS：起点对得上才改，对不上回 repository.ErrQuoteStatusConflict。
// 两个测试钩子：
//   - loseUpdateOnce：模拟「并发对手已经赢了」——把行改到 to 但报 conflict；
//   - conflictNoChangeOnce：模拟「起点过期且重读也不是目标值」——不改行只报 conflict。
type pipelineMemQuote struct {
	mu    sync.Mutex
	rows  map[string]*model.Quote
	lines map[string][]*model.QuoteLineItem

	loseUpdateOnce       bool
	conflictNoChangeOnce bool
	updateErr            error
}

func newPipelineMemQuote() *pipelineMemQuote {
	return &pipelineMemQuote{
		rows:  map[string]*model.Quote{},
		lines: map[string][]*model.QuoteLineItem{},
	}
}

func (m *pipelineMemQuote) add(row *model.Quote, lines ...*model.QuoteLineItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[row.ID] = row
	if len(lines) > 0 {
		m.lines[row.ID] = lines
	}
}

func (m *pipelineMemQuote) Latest(_ context.Context, quoteID string) (*model.Quote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var best *model.Quote
	for _, row := range m.rows {
		if row.QuoteID == quoteID && (best == nil || row.Version > best.Version) {
			best = row
		}
	}
	if best == nil {
		return nil, nil
	}
	cp := *best
	return &cp, nil
}

func (m *pipelineMemQuote) ListLines(_ context.Context, quoteRowID string) ([]*model.QuoteLineItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	src := m.lines[quoteRowID]
	out := make([]*model.QuoteLineItem, len(src))
	copy(out, src)
	return out, nil
}

func (m *pipelineMemQuote) UpdateStatus(_ context.Context, id, from, to string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateErr != nil {
		return m.updateErr
	}
	if m.loseUpdateOnce {
		m.loseUpdateOnce = false
		if row, ok := m.rows[id]; ok {
			row.Status = to // 并发对手已经把它改成 accepted 了
		}
		return repository.ErrQuoteStatusConflict
	}
	if m.conflictNoChangeOnce {
		m.conflictNoChangeOnce = false
		return repository.ErrQuoteStatusConflict
	}
	row, ok := m.rows[id]
	if !ok {
		return repository.ErrQuoteNotFound
	}
	if row.Status != from {
		return repository.ErrQuoteStatusConflict
	}
	row.Status = to
	return nil
}

// —— 假订单仓储 ————————————————————————————————————————————————
//
// raceOrder 非 nil 时，CreatePipelineOrder 先把「对手写的那一笔」落进 map、
// 再回 ErrLTCPipelineOrderExists —— 用于测 createOrder 的幂等愈合分支。
type pipelineMemOrder struct {
	mu         sync.Mutex
	byQuoteRow map[string]*LTCPipelineOrder
	createErr  error
	raceOrder  *LTCPipelineOrder
}

func newPipelineMemOrder() *pipelineMemOrder {
	return &pipelineMemOrder{byQuoteRow: map[string]*LTCPipelineOrder{}}
}

func (m *pipelineMemOrder) GetByQuoteRowID(_ context.Context, quoteRowID string) (*LTCPipelineOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o := m.byQuoteRow[quoteRowID]
	if o == nil {
		return nil, nil
	}
	cp := *o
	return &cp, nil
}

func (m *pipelineMemOrder) CreatePipelineOrder(_ context.Context, in LTCPipelineOrder) (*LTCPipelineOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	if m.raceOrder != nil {
		if _, exists := m.byQuoteRow[m.raceOrder.QuoteRowID]; !exists {
			cp := *m.raceOrder
			m.byQuoteRow[m.raceOrder.QuoteRowID] = &cp
		}
		return nil, ErrLTCPipelineOrderExists
	}
	if _, exists := m.byQuoteRow[in.QuoteRowID]; exists {
		return nil, ErrLTCPipelineOrderExists
	}
	cp := in
	m.byQuoteRow[in.QuoteRowID] = &cp
	return &cp, nil
}

func (m *pipelineMemOrder) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byQuoteRow)
}

// —— 假账单仓储 ————————————————————————————————————————————————
type pipelineMemBill struct {
	mu         sync.Mutex
	byQuoteRow map[string]*model.Bill
	createErr  error
	raceBill   *model.Bill
}

func newPipelineMemBill() *pipelineMemBill {
	return &pipelineMemBill{byQuoteRow: map[string]*model.Bill{}}
}

func (m *pipelineMemBill) GetByQuoteRowID(_ context.Context, quoteRowID string) (*model.Bill, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.byQuoteRow[quoteRowID]
	if b == nil {
		return nil, nil
	}
	cp := *b
	return &cp, nil
}

func (m *pipelineMemBill) Create(_ context.Context, b *model.Bill) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return m.createErr
	}
	if m.raceBill != nil {
		if _, exists := m.byQuoteRow[m.raceBill.QuoteRowID]; !exists {
			cp := *m.raceBill
			m.byQuoteRow[m.raceBill.QuoteRowID] = &cp
		}
		return repository.ErrBillAlreadyDerived
	}
	if _, exists := m.byQuoteRow[b.QuoteRowID]; exists {
		return repository.ErrBillAlreadyDerived
	}
	cp := *b
	m.byQuoteRow[b.QuoteRowID] = &cp
	return nil
}

func (m *pipelineMemBill) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.byQuoteRow)
}

// —— 闸门假实现 + 埋点假实现 ———————————————————————————————————
type pipelineGate struct {
	cfg *LTCConfig
}

func (g pipelineGate) Config(_ context.Context) *LTCConfig { return g.cfg }

type pipelineTrace struct {
	mu     sync.Mutex
	events []*model.SalesEvent
}

func (t *pipelineTrace) Create(_ context.Context, ev *model.SalesEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, ev)
	return nil
}

func (t *pipelineTrace) types() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.events))
	for i, ev := range t.events {
		out[i] = ev.EventType
	}
	return out
}

// —— 构造与造数助手 ——————————————————————————————————————————

var pipelineTestClock = func() time.Time {
	return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
}

func newPipelineForTest(quotes *pipelineMemQuote, orders *pipelineMemOrder, bills *pipelineMemBill, gate LTCConfigReader) *LTCPipelineService {
	svc := NewLTCPipelineService(quotes, orders, bills, gate)
	svc.SetClock(pipelineTestClock)
	return svc
}

// openGate 返回只打开指定阶段的配置读口（总开关开）。
func openGate(stages ...LTCStage) LTCConfigReader {
	cfg := DefaultLTCConfig()
	cfg.Enabled = true
	for _, st := range stages {
		cfg.StagesEnabled = cfg.StagesEnabled.With(st)
	}
	return pipelineGate{cfg: cfg}
}

func memQuoteRow(quoteID string, version int64, status string) *model.Quote {
	return &model.Quote{
		ID:            fmt.Sprintf("%s-v%d", quoteID, version),
		QuoteID:       quoteID,
		OpportunityID: "opp:" + quoteID,
		Version:       version,
		Status:        status,
		Currency:      "CNY",
	}
}

func memLines(rowID string, amounts ...float64) []*model.QuoteLineItem {
	lines := make([]*model.QuoteLineItem, len(amounts))
	for i, a := range amounts {
		lines[i] = &model.QuoteLineItem{QuoteRowID: rowID, LineNo: int64(i + 1), Amount: a}
	}
	return lines
}

// seedAccepted 落一条已确认的报价链 + 两条行项目（合计 150.5），返回行。
func seedAccepted(m *pipelineMemQuote, quoteID string) *model.Quote {
	row := memQuoteRow(quoteID, model.QuoteVersionFirst, model.QuoteStatusAccepted)
	m.add(row, memLines(row.ID, 100, 50.5)...)
	return row
}

func errorsIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("期望 errors.Is(err, %v)，实际 err=%v", target, err)
	}
}

// —— 状态机纯函数 ——————————————————————————————————————————————

func TestLTCPipelineStageMachine(t *testing.T) {
	next := []struct {
		from LTCFlowStage
		to   LTCFlowStage
		ok   bool
	}{
		{LTCFlowQuoteDraft, LTCFlowPriceConfirmed, true},
		{LTCFlowPriceConfirmed, LTCFlowOrderCreated, true},
		{LTCFlowOrderCreated, LTCFlowBillDerived, true},
		{LTCFlowBillDerived, "", false},
		{"", "", false},
		{"bogus", "", false},
	}
	for _, tc := range next {
		got, ok := LTCPipelineNextStage(tc.from)
		if ok != tc.ok || got != tc.to {
			t.Errorf("LTCPipelineNextStage(%q) = (%q, %t)，期望 (%q, %t)", tc.from, got, ok, tc.to, tc.ok)
		}
	}

	legal := []struct {
		from, to LTCFlowStage
		want     bool
	}{
		{LTCFlowQuoteDraft, LTCFlowPriceConfirmed, true},
		{LTCFlowPriceConfirmed, LTCFlowOrderCreated, true},
		{LTCFlowOrderCreated, LTCFlowBillDerived, true},
		{LTCFlowQuoteDraft, LTCFlowOrderCreated, false},       // 跳格
		{LTCFlowBillDerived, LTCFlowOrderCreated, false},      // 回退
		{LTCFlowPriceConfirmed, LTCFlowPriceConfirmed, false}, // 自环
		{LTCFlowQuoteDraft, "", false},
	}
	for _, tc := range legal {
		if got := LTCPipelineTransitionLegal(tc.from, tc.to); got != tc.want {
			t.Errorf("LTCPipelineTransitionLegal(%q, %q) = %t，期望 %t", tc.from, tc.to, got, tc.want)
		}
	}

	for _, st := range ltcFlowStageOrder {
		if !LTCPipelineStageKnown(st) {
			t.Errorf("LTCPipelineStageKnown(%q) 应返回 true", st)
		}
	}
	for _, st := range []LTCFlowStage{"", "bogus", "price-confirmed", "ORDER_CREATED"} {
		if LTCPipelineStageKnown(st) {
			t.Errorf("LTCPipelineStageKnown(%q) 应返回 false（逐字比，不规范化）", st)
		}
	}

	gate := []struct {
		target LTCFlowStage
		stage  LTCStage
		ok     bool
	}{
		{LTCFlowPriceConfirmed, LTCStageQuote, true},
		{LTCFlowOrderCreated, LTCStageQuote, true},
		{LTCFlowBillDerived, LTCStageBill, true},
		{LTCFlowQuoteDraft, "", false},
		{"bogus", "", false},
	}
	for _, tc := range gate {
		got, ok := LTCPipelineGateOf(tc.target)
		if ok != tc.ok || got != tc.stage {
			t.Errorf("LTCPipelineGateOf(%q) = (%q, %t)，期望 (%q, %t)", tc.target, got, ok, tc.stage, tc.ok)
		}
	}
}

// —— FlowStage 推导：只认链上最新版 ——————————————————————————————

func TestLTCPipelineFlowStageDerivation(t *testing.T) {
	cases := []struct {
		name    string
		seed    func(m *pipelineMemQuote)
		want    LTCFlowStage
		wantErr error
	}{
		{
			name: "draft 最新版 ⇒ quote_draft",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusDraft))
			},
			want: LTCFlowQuoteDraft,
		},
		{
			name: "sent 最新版 ⇒ quote_draft（发送是发送腿的职责）",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusSent))
			},
			want: LTCFlowQuoteDraft,
		},
		{
			name: "链上 v1 accepted 但 v2 是 draft ⇒ 认最新版 draft",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusAccepted))
				m.add(memQuoteRow("q1", 2, model.QuoteStatusDraft))
			},
			want: LTCFlowQuoteDraft,
		},
		{
			name: "链上 v1 rejected 但 v2 accepted ⇒ 认最新版 accepted",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusRejected))
				m.add(memQuoteRow("q1", 2, model.QuoteStatusAccepted))
			},
			want: LTCFlowPriceConfirmed,
		},
		{
			name: "accepted 且无订单无账单 ⇒ price_confirmed",
			seed: func(m *pipelineMemQuote) { seedAccepted(m, "q1") },
			want: LTCFlowPriceConfirmed,
		},
		{
			name: "rejected 终态 ⇒ 报错",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusRejected))
			},
			wantErr: ErrLTCPipelineQuoteTerminal,
		},
		{
			name: "expired 终态 ⇒ 报错",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, model.QuoteStatusExpired))
			},
			wantErr: ErrLTCPipelineQuoteTerminal,
		},
		{
			name: "值域外状态 ⇒ 数据事故，报 StatusStuck",
			seed: func(m *pipelineMemQuote) {
				m.add(memQuoteRow("q1", 1, "weird"))
			},
			wantErr: ErrLTCPipelineStatusStuck,
		},
		{
			name:    "链不存在 ⇒ QuoteNotFound",
			wantErr: ErrLTCPipelineQuoteNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quotes := newPipelineMemQuote()
			if tc.seed != nil {
				tc.seed(quotes)
			}
			orders := newPipelineMemOrder()
			bills := newPipelineMemBill()
			svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
			got, err := svc.FlowStage(context.Background(), "q1")
			if tc.wantErr != nil {
				errorsIs(t, err, tc.wantErr)
				return
			}
			if err != nil {
				t.Fatalf("期望无错误，实际 %v", err)
			}
			if got != tc.want {
				t.Fatalf("FlowStage = %q，期望 %q", got, tc.want)
			}
		})
	}
}

func TestLTCPipelineFlowStageOrderAndBillPresence(t *testing.T) {
	t.Run("有订单无账单 ⇒ order_created", func(t *testing.T) {
		quotes := newPipelineMemQuote()
		row := seedAccepted(quotes, "q1")
		orders := newPipelineMemOrder()
		orders.byQuoteRow[row.ID] = &LTCPipelineOrder{ID: "o_1", QuoteRowID: row.ID}
		bills := newPipelineMemBill()
		svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
		got, err := svc.FlowStage(context.Background(), "q1")
		if err != nil {
			t.Fatal(err)
		}
		if got != LTCFlowOrderCreated {
			t.Fatalf("FlowStage = %q，期望 order_created", got)
		}
	})

	t.Run("有账单 ⇒ bill_derived（账单是更硬的事实源，订单即使也在也认账单）", func(t *testing.T) {
		quotes := newPipelineMemQuote()
		row := seedAccepted(quotes, "q1")
		orders := newPipelineMemOrder()
		orders.byQuoteRow[row.ID] = &LTCPipelineOrder{ID: "o_1", QuoteRowID: row.ID}
		bills := newPipelineMemBill()
		bills.byQuoteRow[row.ID] = &model.Bill{ID: "b_1", QuoteRowID: row.ID, Status: model.BillStatusOpen}
		svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
		got, err := svc.FlowStage(context.Background(), "q1")
		if err != nil {
			t.Fatal(err)
		}
		if got != LTCFlowBillDerived {
			t.Fatalf("FlowStage = %q，期望 bill_derived", got)
		}
	})
}

// —— Advance：全闭环一步一步走 ————————————————————————————————

func TestLTCPipelineAdvanceWalksTheChain(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	r1, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("第一步（转订单）失败：%v", err)
	}
	if !r1.Advanced || r1.Stage != LTCFlowOrderCreated {
		t.Fatalf("第一步 = (%t, %q)，期望 (true, order_created)", r1.Advanced, r1.Stage)
	}
	if r1.Order == nil || r1.Order.QuoteRowID != row.ID {
		t.Fatalf("第一步结果缺订单或订单引错版本行：%+v", r1.Order)
	}
	if r1.Order.Amount != 150.5 || r1.Order.LineCount != 2 || r1.Order.Currency != "CNY" {
		t.Fatalf("订单形状不对：amount=%v line_count=%d currency=%q", r1.Order.Amount, r1.Order.LineCount, r1.Order.Currency)
	}
	if r1.Bill != nil {
		t.Fatalf("第一步不该有账单：%+v", r1.Bill)
	}

	r2, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("第二步（开账单）失败：%v", err)
	}
	if !r2.Advanced || r2.Stage != LTCFlowBillDerived {
		t.Fatalf("第二步 = (%t, %q)，期望 (true, bill_derived)", r2.Advanced, r2.Stage)
	}
	if r2.Bill == nil || r2.Bill.Amount != 150.5 || r2.Bill.Status != model.BillStatusOpen || r2.Bill.DueAt != nil {
		t.Fatalf("账单形状不对：%+v", r2.Bill)
	}
	if r2.Payment == nil || r2.Payment.BillID != r2.Bill.ID || r2.Payment.Amount != 150.5 {
		t.Fatalf("回款调用点形状不对：%+v", r2.Payment)
	}

	r3, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("第三步（终点）失败：%v", err)
	}
	if r3.Advanced || r3.GateReason != "" || r3.Stage != LTCFlowBillDerived {
		t.Fatalf("终点结果 = (%t, %q, %q)，期望 (false, bill_derived, 空 GateReason)", r3.Advanced, r3.Stage, r3.GateReason)
	}
	if r3.Order == nil || r3.Bill == nil || r3.Payment == nil {
		t.Fatalf("终点结果应带齐订单/账单/回款调用点：%+v", r3)
	}
	if orders.count() != 1 || bills.count() != 1 {
		t.Fatalf("落库数量不对：订单 %d、账单 %d，期望各 1", orders.count(), bills.count())
	}
}

func TestLTCPipelineAdvanceConfirmPrice(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := memQuoteRow("q1", model.QuoteVersionFirst, model.QuoteStatusSent)
	quotes.add(row, memLines(row.ID, 100, 50.5)...)
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	r, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("确认改价失败：%v", err)
	}
	if !r.Advanced || r.Stage != LTCFlowPriceConfirmed {
		t.Fatalf("确认改价 = (%t, %q)，期望 (true, price_confirmed)", r.Advanced, r.Stage)
	}
	if quotes.rows[row.ID].Status != model.QuoteStatusAccepted {
		t.Fatalf("报价状态 = %q，期望 accepted", quotes.rows[row.ID].Status)
	}
	if orders.count() != 0 || bills.count() != 0 {
		t.Fatalf("确认改价不该动订单/账单（订单 %d、账单 %d）", orders.count(), bills.count())
	}
}

// —— 门关着 = 一行都不写、不是错误 ————————————————————————————

func TestLTCPipelineAdvanceGateClosedWritesNothing(t *testing.T) {
	cases := []struct {
		name       string
		gate       LTCConfigReader
		wantReason string
	}{
		{
			name:       "总开关关 ⇒ master_off",
			gate:       pipelineGate{cfg: DefaultLTCConfig()},
			wantReason: LTCReasonMasterOff,
		},
		{
			name:       "quote 阶段没开 ⇒ stage_off",
			gate:       openGate(LTCStageBill), // 只开了 bill
			wantReason: LTCReasonStageOff,
		},
		{
			name: "配置降级 ⇒ config_degraded 且带原因",
			gate: func() LTCConfigReader {
				cfg := DefaultLTCConfig()
				cfg.Degraded = true
				cfg.DegradeReason = "存储不可用"
				return pipelineGate{cfg: cfg}
			}(),
			wantReason: LTCReasonDegraded,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			quotes := newPipelineMemQuote()
			row := seedAccepted(quotes, "q1")
			orders := newPipelineMemOrder()
			bills := newPipelineMemBill()
			svc := newPipelineForTest(quotes, orders, bills, tc.gate)

			r, err := svc.Advance(context.Background(), "q1")
			if err != nil {
				t.Fatalf("门关着不该是错误：%v", err)
			}
			if r.Advanced || r.GateReason != tc.wantReason {
				t.Fatalf("结果 = (%t, %q)，期望 (false, %q)", r.Advanced, r.GateReason, tc.wantReason)
			}
			if orders.count() != 0 || bills.count() != 0 {
				t.Fatalf("门关着必须零写入：订单 %d、账单 %d", orders.count(), bills.count())
			}
			if quotes.rows[row.ID].Status != model.QuoteStatusAccepted {
				t.Fatalf("门关着不该动报价状态：%q", quotes.rows[row.ID].Status)
			}
			if tc.wantReason == LTCReasonDegraded && !strings.Contains(r.GateDetail, "存储不可用") {
				t.Fatalf("degraded 的 detail 应带底层原因：%q", r.GateDetail)
			}
		})
	}
}

func TestLTCPipelineAdvanceBillGateClosed(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	orders.byQuoteRow[row.ID] = &LTCPipelineOrder{ID: "o_1", QuoteRowID: row.ID}
	bills := newPipelineMemBill()
	// quote 开着（转订单能过），bill 没开（开账单被拦）。
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote))

	r, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("门关着不该是错误：%v", err)
	}
	if r.Advanced || r.GateReason != LTCReasonStageOff || r.Stage != LTCFlowOrderCreated {
		t.Fatalf("结果 = (%t, %q, %q)，期望 (false, stage_off, order_created)", r.Advanced, r.GateReason, r.Stage)
	}
	if bills.count() != 0 {
		t.Fatalf("bill 门关着不该有账单写入：%d", bills.count())
	}
}

// —— 失败形状：每种一个哨兵，零写入 ————————————————————————————

func TestLTCPipelineAdvanceFailureShapes(t *testing.T) {
	t.Run("draft 未发出 ⇒ QuoteNotSent", func(t *testing.T) {
		quotes := newPipelineMemQuote()
		row := memQuoteRow("q1", 1, model.QuoteStatusDraft)
		quotes.add(row)
		orders := newPipelineMemOrder()
		bills := newPipelineMemBill()
		svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
		_, err := svc.Advance(context.Background(), "q1")
		errorsIs(t, err, ErrLTCPipelineQuoteNotSent)
		if quotes.rows[row.ID].Status != model.QuoteStatusDraft {
			t.Fatalf("未发出就不该改状态：%q", quotes.rows[row.ID].Status)
		}
	})

	t.Run("终态报价 ⇒ QuoteTerminal", func(t *testing.T) {
		quotes := newPipelineMemQuote()
		quotes.add(memQuoteRow("q1", 1, model.QuoteStatusExpired))
		svc := newPipelineForTest(quotes, newPipelineMemOrder(), newPipelineMemBill(), openGate(LTCStageQuote, LTCStageBill))
		_, err := svc.Advance(context.Background(), "q1")
		errorsIs(t, err, ErrLTCPipelineQuoteTerminal)
	})

	t.Run("链不存在 ⇒ QuoteNotFound", func(t *testing.T) {
		svc := newPipelineForTest(newPipelineMemQuote(), newPipelineMemOrder(), newPipelineMemBill(), openGate(LTCStageQuote, LTCStageBill))
		_, err := svc.Advance(context.Background(), "q-missing")
		errorsIs(t, err, ErrLTCPipelineQuoteNotFound)
	})

	t.Run("入参为空 ⇒ InputInvalid", func(t *testing.T) {
		svc := newPipelineForTest(newPipelineMemQuote(), newPipelineMemOrder(), newPipelineMemBill(), openGate(LTCStageQuote, LTCStageBill))
		_, err := svc.Advance(context.Background(), "   ")
		errorsIs(t, err, ErrLTCPipelineInputInvalid)
	})

	t.Run("行项目缺失 ⇒ LinesMissing，零写入", func(t *testing.T) {
		quotes := newPipelineMemQuote()
		quotes.add(memQuoteRow("q1", 1, model.QuoteStatusAccepted)) // 没有行项目
		orders := newPipelineMemOrder()
		bills := newPipelineMemBill()
		svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
		_, err := svc.Advance(context.Background(), "q1")
		errorsIs(t, err, ErrLTCPipelineLinesMissing)
		if orders.count() != 0 {
			t.Fatalf("明细缺就不该转订单：%d", orders.count())
		}
	})

	t.Run("未装配 ⇒ Unavailable", func(t *testing.T) {
		svc := NewLTCPipelineService(nil, nil, nil, nil)
		if svc.Available() {
			t.Fatal("空装配应报不可用")
		}
		_, err := svc.Advance(context.Background(), "q1")
		errorsIs(t, err, ErrLTCPipelineUnavailable)
		if _, err := svc.Run(context.Background(), "q1"); !errors.Is(err, ErrLTCPipelineUnavailable) {
			t.Fatalf("Run 也应报不可用，实际 %v", err)
		}
		if _, err := svc.FlowStage(context.Background(), "q1"); !errors.Is(err, ErrLTCPipelineUnavailable) {
			t.Fatalf("FlowStage 也应报不可用（不 panic），实际 %v", err)
		}
	})
}

// —— 并发落败的愈合与卡死 ———————————————————————————————————

func TestLTCPipelineConfirmPriceCASConflictHeals(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := memQuoteRow("q1", 1, model.QuoteStatusSent)
	quotes.add(row, memLines(row.ID, 100)...)
	quotes.loseUpdateOnce = true // 并发对手已把它改成 accepted
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	r, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("CAS 落败但对手已赢，应幂等愈合：%v", err)
	}
	if !r.Advanced || r.Stage != LTCFlowPriceConfirmed {
		t.Fatalf("愈合后应停在 price_confirmed：(%t, %q)", r.Advanced, r.Stage)
	}
	if quotes.rows[row.ID].Status != model.QuoteStatusAccepted {
		t.Fatalf("状态 = %q，期望 accepted", quotes.rows[row.ID].Status)
	}
}

func TestLTCPipelineConfirmPriceCASConflictStuck(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := memQuoteRow("q1", 1, model.QuoteStatusSent)
	quotes.add(row, memLines(row.ID, 100)...)
	quotes.conflictNoChangeOnce = true // 落败且重读仍不是 accepted
	svc := newPipelineForTest(quotes, newPipelineMemOrder(), newPipelineMemBill(), openGate(LTCStageQuote, LTCStageBill))

	_, err := svc.Advance(context.Background(), "q1")
	errorsIs(t, err, ErrLTCPipelineStatusStuck)
	if quotes.rows[row.ID].Status != model.QuoteStatusSent {
		t.Fatalf("卡死时状态不该被改：%q", quotes.rows[row.ID].Status)
	}
}

func TestLTCPipelineOrderExistsHeals(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	orders.raceOrder = &LTCPipelineOrder{ID: "o_race", QuoteRowID: row.ID, QuoteID: "q1"}
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	r, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("订单已存在应幂等复用：%v", err)
	}
	if !r.Advanced || r.Stage != LTCFlowOrderCreated || r.Order == nil || r.Order.ID != "o_race" {
		t.Fatalf("应复用对手写的那一笔订单：(%t, %q, %+v)", r.Advanced, r.Stage, r.Order)
	}
	if orders.count() != 1 {
		t.Fatalf("订单应只有一笔：%d", orders.count())
	}
}

func TestLTCPipelineBillAlreadyDerivedHeals(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	orders.byQuoteRow[row.ID] = &LTCPipelineOrder{ID: "o_1", QuoteRowID: row.ID}
	bills := newPipelineMemBill()
	bills.raceBill = &model.Bill{ID: "b_race", QuoteRowID: row.ID, QuoteID: "q1", Status: model.BillStatusOpen}
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	r, err := svc.Advance(context.Background(), "q1")
	if err != nil {
		t.Fatalf("账单已派生应幂等复用：%v", err)
	}
	if !r.Advanced || r.Stage != LTCFlowBillDerived || r.Bill == nil || r.Bill.ID != "b_race" {
		t.Fatalf("应复用对手写的那张账单：(%t, %q, %+v)", r.Advanced, r.Stage, r.Bill)
	}
	if bills.count() != 1 {
		t.Fatalf("账单应只有一张：%d", bills.count())
	}
}

func TestLTCPipelineOrderStoreFailure(t *testing.T) {
	quotes := newPipelineMemQuote()
	seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	orders.createErr = errors.New("orders 表炸了")
	svc := newPipelineForTest(quotes, orders, newPipelineMemBill(), openGate(LTCStageQuote, LTCStageBill))
	_, err := svc.Advance(context.Background(), "q1")
	if err == nil || errors.Is(err, ErrLTCPipelineStatusStuck) || errors.Is(err, ErrLTCPipelineOrderExists) {
		t.Fatalf("存储故障应原样上抛（包装成转订单失败），实际 %v", err)
	}
}

// —— Run：一路推到底 / 停在第一道关着的门 ——————————————————————

func TestLTCPipelineRunToCompletion(t *testing.T) {
	quotes := newPipelineMemQuote()
	seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	trace := &pipelineTrace{}
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))
	svc.SetSalesTrace(trace)

	r, err := svc.Run(context.Background(), "q1")
	if err != nil {
		t.Fatalf("Run 失败：%v", err)
	}
	if r.Stage != LTCFlowBillDerived || r.Advanced {
		t.Fatalf("Run 终点 = (%q, %t)，期望 (bill_derived, false)", r.Stage, r.Advanced)
	}
	if r.Order == nil || r.Bill == nil || r.Payment == nil {
		t.Fatalf("Run 终点应带齐订单/账单/回款调用点：%+v", r)
	}
	if r.Payment.BillID != r.Bill.ID {
		t.Fatalf("回款调用点账单号 %q ≠ 账单 %q", r.Payment.BillID, r.Bill.ID)
	}
	if orders.count() != 1 || bills.count() != 1 {
		t.Fatalf("落库数量：订单 %d、账单 %d，期望各 1", orders.count(), bills.count())
	}
	if types := trace.types(); len(types) != 2 || types[0] != model.SalesEventTypeOrder || types[1] != model.SalesEventTypeBill {
		t.Fatalf("埋点应为 [order, bill]，实际 %v", types)
	}
}

func TestLTCPipelineRunStopsAtClosedGate(t *testing.T) {
	quotes := newPipelineMemQuote()
	seedAccepted(quotes, "q1")
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote)) // bill 门关着

	r, err := svc.Run(context.Background(), "q1")
	if err != nil {
		t.Fatalf("门关着 Run 不该报错：%v", err)
	}
	if r.Stage != LTCFlowOrderCreated || r.Advanced || r.GateReason != LTCReasonStageOff {
		t.Fatalf("Run 应停在 order_created：(%q, %t, %q)", r.Stage, r.Advanced, r.GateReason)
	}
	if orders.count() != 1 || bills.count() != 0 {
		t.Fatalf("转完订单就该停：订单 %d、账单 %d", orders.count(), bills.count())
	}
}

func TestLTCPipelineRunFromDraftFailsBeforeAnyWrite(t *testing.T) {
	quotes := newPipelineMemQuote()
	row := memQuoteRow("q1", 1, model.QuoteStatusDraft)
	quotes.add(row, memLines(row.ID, 100)...)
	orders := newPipelineMemOrder()
	bills := newPipelineMemBill()
	svc := newPipelineForTest(quotes, orders, bills, openGate(LTCStageQuote, LTCStageBill))

	_, err := svc.Run(context.Background(), "q1")
	errorsIs(t, err, ErrLTCPipelineQuoteNotSent)
	if orders.count() != 0 || bills.count() != 0 {
		t.Fatalf("draft 推进失败不该有任何写入：订单 %d、账单 %d", orders.count(), bills.count())
	}
}

// —— 回款调用点拼参 ————————————————————————————————————————————

func TestLTCPipelinePaymentCalloutInputFor(t *testing.T) {
	paidAt := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	c := LTCPaymentCallout{BillID: "b_1", QuoteID: "q1", QuoteRowID: "r1", Amount: 150.5, Currency: "CNY"}
	in := c.RecordPaymentInputFor("ch_ref", "wxpay", "o_1", &paidAt)
	if in.BillID != "b_1" || in.ChannelRef != "ch_ref" || in.Platform != "wxpay" || in.OrderID != "o_1" {
		t.Fatalf("回款入参拼错：%+v", in)
	}
	if in.Amount != 150.5 || in.Currency != "CNY" || in.PaidAt == nil || !in.PaidAt.Equal(paidAt) {
		t.Fatalf("回款入参金额/币种/到账时刻拼错：%+v", in)
	}
	if in.Status != "" {
		t.Fatalf("Status 应由接线层/RecordPayment 默认，不该由调用点拼：%q", in.Status)
	}

	noPaidAt := c.RecordPaymentInputFor("ch_ref", "wxpay", "o_1", nil)
	if noPaidAt.PaidAt != nil {
		t.Fatalf("nil paidAt 应保持 nil（由 RecordPayment 填接收时刻）：%+v", noPaidAt.PaidAt)
	}
}

// —— 纯函数 ————————————————————————————————————————————————

func TestLTCPipelinePureHelpers(t *testing.T) {
	if got := pipelineCurrencyOf(""); got != model.BillCurrencyDefault {
		t.Fatalf("pipelineCurrencyOf(\"\") = %q，期望 %q", got, model.BillCurrencyDefault)
	}
	if got := pipelineCurrencyOf(" usd "); got != "usd" {
		t.Fatalf("pipelineCurrencyOf(\" usd \") = %q，期望 usd", got)
	}

	k1 := newPipelineOrderKey(pipelineTestClock(), 7)
	k2 := newPipelineOrderKey(pipelineTestClock(), 7)
	if k1 != k2 || !strings.HasPrefix(k1, "o_") {
		t.Fatalf("订单号应可复算且带 o_ 前缀：%q vs %q", k1, k2)
	}
	if nextPipelineOrderSeq() == 0 {
		t.Fatal("订单序号应从 1 起单调递增")
	}
}
