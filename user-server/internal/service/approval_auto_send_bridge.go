// approval_auto_send_bridge.go E-10（2026-10-24，产品可复议）：审批通过后自动外发报价。
//
// 规划 §4.10 的原判据：
//   - 审批通过后只发一次（Send 自带幂等：approval_id 带结论去派发，重复调用
//     会撞 awaiting/approved 已跃迁的状态而不再外发）；
//   - 发送失败可重试（失败只出声，运营可重发——发送腿的状态机不变）；
//   - 关闭开关时行为与现在一致（nil 桥 = 无订阅者 = 纯手动档）。
//
// 为什么做成 ApprovalWaitNotifier 的追加订阅者而不是在 Send 里再校验一遍：
// 裁决端点（decide）是唯一知道"批了"的时刻，从这里驱动外发是事件驱动单入口；
// 在发送腿里再加一份"批了就发"的判定则是两处各写一遍同一结论（规划原话：
// 不要在两处重复校验）。
//
// 口径：只认 subject_type=quote、结论=approved 的裁决。rejected/expired/
// reach_plan/order_command 等一律不动——它们要么没有"发"这个动作，要么
// 外发口径未定义（等产品复议后扩展）。
package service

import (
	"context"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// approvalAutoSendOperator 自动外发的 operator 名（落 sales_events.owner_id）。
// 不装作真人：审计读这一列时必须能分辨"机器替人发的"与"人自己点的"。
const approvalAutoSendOperator = "approval-auto-send"

// ApprovalAutoSendBridge 审批通过 → 报价自动外发（E-10，产品可复议）。
//
// quoteSend 用全局实例（装配在 InitQuoteRuntime）：审批运行时与报价运行时的
// 装配顺序不固定，持全局句柄可以让"谁先装配都能接上"（nil 时本桥静默 = 手动档）。
type ApprovalAutoSendBridge struct {
	quoteSend func() *QuoteSendService
}

// NewApprovalAutoSendBridge 构造。quoteSend 允许回 nil（运行时未装配）。
func NewApprovalAutoSendBridge(quoteSend func() *QuoteSendService) *ApprovalAutoSendBridge {
	return &ApprovalAutoSendBridge{quoteSend: quoteSend}
}

// NotifyDecided 实现 ApprovalWaitNotifier（E-10）。
//
// 无返回值是接口定的：人的裁决已经落库，外发失败不能回滚它——失败只出声，
// 运营可按原有手动路径重发（QuoteSendResult 的 awaiting/sent 语义不变）。
func (b *ApprovalAutoSendBridge) NotifyDecided(ctx context.Context, req *model.ApprovalRequest) {
	if b == nil || req == nil {
		return
	}
	if req.SubjectType != QuoteApprovalSubjectType {
		return
	}
	if req.Status != model.ApprovalStatusApproved {
		return
	}
	svc := b.quoteSend()
	if svc == nil || !svc.Available() {
		logger.Ctx(ctx).Warn().
			Str("approval_id", req.ID).
			Str("quote_row_id", req.SubjectID).
			Msg("[approval-auto-send] 报价发送腿未装配 ⇒ 审批已通过但未自动外发，需人工发送（手动档行为）")
		return
	}
	res, err := svc.Send(ctx, QuoteSendInput{
		QuoteRowID: req.SubjectID,
		ApprovalID: req.ID,
		Operator:   approvalAutoSendOperator,
	})
	if err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str("approval_id", req.ID).
			Str("quote_row_id", req.SubjectID).
			Msg("[approval-auto-send] 审批通过后自动外发失败 ⇒ 需人工按原路径重发（裁决已生效，不受影响）")
		return
	}
	logger.Ctx(ctx).Info().
		Str("approval_id", req.ID).
		Str("quote_row_id", req.SubjectID).
		Str("disposition", res.Disposition).
		Str("status", res.Status).
		Msg("[approval-auto-send] 审批通过后自动外发完成")
}
