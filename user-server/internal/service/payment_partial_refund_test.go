// payment_partial_refund_test.go E-10（2026-10-24，产品可复议）：部分退款的四件判据。
//
//	① 部分退款后净到账 = Σconfirmed − Σrefunded：账单 100，收 100（paid），
//	   退 30 ⇒ 净 70，账单退回 partial，且订单/回款/冲销三处金额自洽；
//	② 幂等：同一退款流水号重投两次 ⇒ payments 只多一行、第二次 Reused=true；
//	③ 全额退款仍走整笔冲销路径（reversed），退款行不越权替代它；
//	④ 退款额超过净到账 ⇒ 拒，且库里没有任何新行（拒写判据全在第一次写之前）；
//	⑤ 退款原因（reason 格，规划 §4.10 四格之一）：落库、随视图回显、超长拒。
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func TestPartialRefundSettlesBackToPartial(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_1", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)

	// 收 100 ⇒ paid
	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-1", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	if got := payBillRow(t, db, bill.ID).Status; got != model.BillStatusPaid {
		t.Fatalf("入账后账单应 paid，得到 %s", got)
	}

	// 退 30 ⇒ 净 70 ⇒ partial（判据①）
	receipt, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-1", Amount: 30,
		Platform: "taobao", OrderID: "ord-refund-1",
	})
	if err != nil {
		t.Fatalf("部分退款失败: %v", err)
	}
	if receipt.Settlement.Settled != 70 {
		t.Fatalf("净到账应 70，得到 %v", receipt.Settlement.Settled)
	}
	if got := payBillRow(t, db, bill.ID).Status; got != model.BillStatusPartial {
		t.Fatalf("退款后账单应退回 partial，得到 %s", got)
	}
	// 回款行原样（追加不抹改）：pay-1 仍是 confirmed 100
	row := payPaymentRow(t, db, "pay-1")
	if row.Status != model.PaymentStatusConfirmed || row.Amount != 100 {
		t.Fatalf("原回款行被改动: status=%s amount=%v", row.Status, row.Amount)
	}
	// 退款行：refunded 30（判据①的三处金额自洽）
	refund := payPaymentRow(t, db, "refund-1")
	if refund.Status != model.PaymentStatusRefunded || refund.Amount != 30 {
		t.Fatalf("退款行不符: status=%s amount=%v", refund.Status, refund.Amount)
	}
}

func TestPartialRefundIdempotentReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_2", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)

	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-2", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	before := payRows(t, db)
	in := RecordPartialRefundInput{BillID: bill.ID, RefundRef: "refund-2", Amount: 40,
		Platform: "taobao", OrderID: "ord-refund-2"}
	if _, err := svc.RecordPartialRefund(ctx, in); err != nil {
		t.Fatalf("首次退款失败: %v", err)
	}
	second, err := svc.RecordPartialRefund(ctx, in) // 判据②：重投
	if err != nil {
		t.Fatalf("重投退款失败: %v", err)
	}
	if !second.Reused {
		t.Fatal("重投应标记 Reused=true")
	}
	if n := payRows(t, db); n != before+1 {
		t.Fatalf("重投后应只多一行（before=%d, now=%d）", before, n)
	}
	if second.Settlement.Settled != 60 {
		t.Fatalf("净到账应 60，得到 %v", second.Settlement.Settled)
	}
}

func TestFullRefundStillUsesReversalPath(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_3", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)

	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-3", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	// 全额退款 = 渠道对那一整笔的冲销通知 ⇒ RecordPayment status=reversed（判据③）
	if _, err := svc.RecordPayment(ctx, RecordPaymentInput{
		BillID: bill.ID, ChannelRef: "pay-3", Amount: 100, Status: model.PaymentStatusReversed,
		Platform: "taobao", OrderID: "ord-pay-3",
	}); err != nil {
		t.Fatalf("整笔冲销失败: %v", err)
	}
	row := payPaymentRow(t, db, "pay-3")
	if row.Status != model.PaymentStatusReversed {
		t.Fatalf("全额退款走 reversed 路径，得到 %s", row.Status)
	}
	if got := payBillRow(t, db, bill.ID).Status; got != model.BillStatusOpen {
		t.Fatalf("全额退后账单应回 open，得到 %s", got)
	}
	// reversed 行之后不允许再用部分退款"复活"成 refunded（复活=同一笔钱的两个说法）
	_, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "pay-3", Amount: 100,
		Platform: "taobao", OrderID: "ord-pay-3",
	})
	if err == nil {
		t.Fatal("reversed 行的流水号被退款复用时应拒绝")
	}
}

func TestRefundOverSettledRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_4", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)

	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-4", 60)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	before := payRows(t, db)
	// 净到账 60，退 61 ⇒ 拒（判据④）
	_, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-4", Amount: 61,
		Platform: "taobao", OrderID: "ord-refund-4",
	})
	if err == nil {
		t.Fatal("退款超过净到账应拒绝")
	}
	if n := payRows(t, db); n != before {
		t.Fatalf("拒后库里不应有新行（before=%d, now=%d）", before, n)
	}
	if got := payBillRow(t, db, bill.ID).Status; got != model.BillStatusPartial {
		t.Fatalf("拒后账单应保持 partial，得到 %s", got)
	}
}

func TestPartialRefundCarriesReason(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_5", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)

	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-5", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	// 判据⑤：reason 落库 + 视图回显（空渠道原因的对照格也在同一用例里）
	receipt, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-5a", Amount: 25,
		Platform: "taobao", OrderID: "ord-refund-5", Reason: "商品退货",
	})
	if err != nil {
		t.Fatalf("带原因退款失败: %v", err)
	}
	if receipt.Payment.Reason != "商品退货" {
		t.Fatalf("视图应回显退款原因，得到 %q", receipt.Payment.Reason)
	}
	if row := payPaymentRow(t, db, "refund-5a"); row.Reason != "商品退货" {
		t.Fatalf("退款行应落 reason 列，得到 %q", row.Reason)
	}
	// 渠道没给原因 ⇒ 空串（空=渠道没说，不是没有这一格）
	receipt2, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-5b", Amount: 10,
		Platform: "taobao", OrderID: "ord-refund-5",
	})
	if err != nil {
		t.Fatalf("无原因退款失败: %v", err)
	}
	if receipt2.Payment.Reason != "" {
		t.Fatalf("未给原因时应为空串，得到 %q", receipt2.Payment.Reason)
	}
	// 回款行（confirmed）的 reason 恒空——原因格只属于退款语义
	if row := payPaymentRow(t, db, "pay-5"); row.Reason != "" {
		t.Fatalf("回款行 reason 应恒空，得到 %q", row.Reason)
	}
	// 超长拒（paymentReasonMaxRunes=85，超界=渠道按另一份契约推送）
	long := strings.Repeat("退", 86)
	if _, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-5c", Amount: 5,
		Platform: "taobao", OrderID: "ord-refund-5", Reason: long,
	}); err == nil {
		t.Fatal("超长退款原因应拒绝（截断会伪造渠道陈述）")
	}
	if _, err := svc.payments.GetByChannelRef(ctx, "refund-5c"); err != nil {
		t.Fatalf("回读失败: %v", err)
	}
}

// TestConcurrentPartialRefundsCannotOverdraw A11：并发退款不许把净到账退成负数。
//
// service 侧这一格守的是**接线**：仓储的 CreateRefundGuarded 已经有并发用例，
// 但若有人把 RecordPartialRefund 改回"先 SumSettledByBill 再 Create"的两步，
// 仓储那边照样全绿，只有这条会红。判据打在落库行数与复算净到账上：
// 净到账 100、16 路并发各退 80 ⇒ 至多一笔成功，退完净到账 ≥ 0。
func TestConcurrentPartialRefundsCannotOverdraw(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_race", "q_refund", "qr_refund", 100)
	svc := paySvc(t, db)
	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-race", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}

	const lanes = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, lanes)
	for i := 0; i < lanes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
				BillID: bill.ID, RefundRef: fmt.Sprintf("refund-race-%d", i), Amount: 80,
				Platform: "taobao", OrderID: fmt.Sprintf("ord-refund-race-%d", i),
			})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	succeeded := 0
	for err := range errs {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Errorf("并发 %d 路各退 80（净到账 100）：成功 %d 笔 ≠ 1（≥2 = TOCTOU 在场；0 = 守卫连该放的那笔也拒了）", lanes, succeeded)
	}

	payRepo := repository.NewPaymentRepositoryWithDB(db)
	got, err := payRepo.SumSettledByBill(ctx, bill.ID)
	if err != nil {
		t.Fatalf("复算净到账失败: %v", err)
	}
	if got != 20 {
		t.Errorf("净到账 %v ≠ 20（100 − 一笔 80）：负结清是假事实或退款被漏记", got)
	}
}

// countingRefundStore 包一层真仓储，只数"走的是哪条写路径"。
//
// 为什么要这层：并发用例只能证明"结果没坏"，证明不了"接线没退回两步"——
// 本地快库上 16 路自动提交语句会被单条池连接吸收，把 service 改回
// "先 SumSettledByBill 再 Create" 时那条并发用例仍然绿（实测：succeeded=1）。
// 这条断言把判据钉在调用面上：退款**只能**走 CreateRefundGuarded 这条原子通路。
type countingRefundStore struct {
	paymentStore
	guardedCalls int
	directCalls  int
}

func (c *countingRefundStore) Create(ctx context.Context, p *model.Payment) error {
	c.directCalls++
	return c.paymentStore.Create(ctx, p)
}

func (c *countingRefundStore) CreateRefundGuarded(ctx context.Context, p *model.Payment, guard func(float64) error) error {
	c.guardedCalls++
	return c.paymentStore.CreateRefundGuarded(ctx, p, guard)
}

// TestRefundLegOnlyUsesTheAtomicPath A11 接线判据：退款腿只走原子通路。
//
// 正控制：回款腿（RecordPayment）仍然走 Create——若两条腿都走守卫，本条也会红，
// 说明计数器真的在数（不是恒 0 的橡皮章）。
func TestRefundLegOnlyUsesTheAtomicPath(t *testing.T) {
	if testing.Short() {
		t.Skip("需要测试库")
	}
	db := testutil.NewTestDB(t, &model.Bill{}, &model.Payment{})
	ctx := context.Background()
	bill := payBill(t, db, "b_refund_wire", "q_refund", "qr_refund", 100)
	spy := &countingRefundStore{paymentStore: repository.NewPaymentRepositoryWithDB(db)}
	svc := NewPaymentService(spy, repository.NewBillRepositoryWithDB(db))
	svc.SetClock(func() time.Time { return qsClockBase })

	// 正控制：回款腿走 Create。
	if _, err := svc.RecordPayment(ctx, payIn(bill.ID, "pay-wire", 100)); err != nil {
		t.Fatalf("入账失败: %v", err)
	}
	if spy.directCalls != 1 || spy.guardedCalls != 0 {
		t.Fatalf("回款腿路径不对：direct=%d guarded=%d（期望 1/0）", spy.directCalls, spy.guardedCalls)
	}

	// 判据：退款腿走 CreateRefundGuarded，且不再直插。
	spy.directCalls, spy.guardedCalls = 0, 0
	if _, err := svc.RecordPartialRefund(ctx, RecordPartialRefundInput{
		BillID: bill.ID, RefundRef: "refund-wire", Amount: 30,
		Platform: "taobao", OrderID: "ord-refund-wire",
	}); err != nil {
		t.Fatalf("退款失败: %v", err)
	}
	if spy.guardedCalls != 1 || spy.directCalls != 0 {
		t.Fatalf("退款腿没走原子通路：guarded=%d direct=%d（期望 1/0——退回两步就是 TOCTOU 复发）",
			spy.guardedCalls, spy.directCalls)
	}
}
