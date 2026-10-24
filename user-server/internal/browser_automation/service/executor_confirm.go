package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"hivemtk-user/internal/browser_automation/model"
)

// executor_confirm.go —— D7 写操作人工确认闸门：挂起、放行、超时归因与审计帧。
// ---- D7 写操作人工确认闸门（与 stop 同构，语义相反：stop 取消、confirm 放行）----

// PendingConfirmGate 闸门的对外可见身份：一次挂起「在等谁批、批的是哪份载荷、
// 什么时候到期」必须可查证。Preview 只在读侧（归属校验之后）出现，绝不进审计帧。
type PendingConfirmGate struct {
	SessionID   uint      `json:"session_id"`
	StepIndex   int       `json:"step_index"`
	PayloadHash string    `json:"payload_hash"`
	Preview     string    `json:"preview"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// confirmGate 一次挂起的全部事实。载荷哈希在写下时定死、闸门存续期间不许改——
// 「批的是哪份载荷」有唯一答案，比对才有意义。
type confirmGate struct {
	ch          chan struct{}
	sessionID   uint
	stepIndex   int
	payloadHash string
	preview     string
	expiresAt   time.Time
	// budget 挂起时给定的名义预算：超时归因要说「等了多久」，用 time.Until(expiresAt) 在
	// 到期那一刻恒等于 0，等于把归因文案写成谎。
	budget time.Duration

	// mismatchAttempts 拿错载荷来要的放行次数。放行不撤闸门，所以这个数只能由闸门自己记，
	// 事后无从重建；judge 帧收口时把它一起落下去（拒过谁来，和批没批同样值得审计）。
	mu               sync.Mutex
	mismatchAttempts int
}

func (g *confirmGate) attempts() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.mismatchAttempts
}

func (g *confirmGate) info() PendingConfirmGate {
	return PendingConfirmGate{
		SessionID: g.sessionID, StepIndex: g.stepIndex, PayloadHash: g.payloadHash,
		Preview: g.preview, ExpiresAt: g.expiresAt,
	}
}

// ConfirmVerdict 放行请求的结论。三态而不是 bool：
// 「没有闸门」「批错了东西」「批对了」是三件不同的事，压成一句文案就等于让运维去猜。
// 命名前缀 Verdict 与闸门自己的出路 confirmOutcome 分开——两者都叫「granted」时，
// 「放行命中了」和「等待方被放行」会在读码时互相顶替。
type ConfirmVerdict uint8

const (
	VerdictNoGate   ConfirmVerdict = iota // 该 session 当前没有挂起闸门
	VerdictGranted                        // 载荷相符，已放行（一次性消费）
	VerdictMismatch                       // 有闸门，但批的不是这份载荷——已拒且闸门仍在
)

// SignalConfirm POST /sessions/:id/confirm 放行。payloadHash 是**被批准的那份载荷**的指纹，
// 由读侧（GET /sessions/:id/confirm-gate）取得后原样带回；不符即 VerdictMismatch——
// 不撤闸门、不下发、只计数（fail-closed，且「再问一次」拿不到放行）。
// 先摘后关：同一通道只可能被 close 一次，第二次请求落到 VerdictNoGate。
// 空请求不需要特判：闸门的哈希在开闸时已保证非空（见 awaitConfirmGate 的前置），
// 所以「没带载荷」天然落进不等式这一侧。
func (e *Executor) SignalConfirm(sessionID uint, payloadHash string) ConfirmVerdict {
	e.confirmMu.Lock()
	g, ok := e.confirmRegistry[sessionID]
	if !ok {
		e.confirmMu.Unlock()
		return VerdictNoGate
	}
	if payloadHash != g.payloadHash {
		g.mu.Lock()
		g.mismatchAttempts++
		g.mu.Unlock()
		e.confirmMu.Unlock()
		return VerdictMismatch
	}
	delete(e.confirmRegistry, sessionID)
	e.confirmMu.Unlock()
	close(g.ch)
	return VerdictGranted
}

// ConfirmPending 该 session 是否正停在确认闸门（读侧暴露给前端按钮可见性）
func (e *Executor) ConfirmPending(sessionID uint) bool {
	e.confirmMu.Lock()
	defer e.confirmMu.Unlock()
	_, ok := e.confirmRegistry[sessionID]
	return ok
}

// PendingGate 读取挂起闸门的详情（第二返回值 false=没有在等人工放行）。
func (e *Executor) PendingGate(sessionID uint) (PendingConfirmGate, bool) {
	e.confirmMu.Lock()
	defer e.confirmMu.Unlock()
	g, ok := e.confirmRegistry[sessionID]
	if !ok {
		return PendingConfirmGate{}, false
	}
	return g.info(), true
}

// registerConfirmGate 登记一次挂起并返回它——与 unregister 配对，且「同一 session 只可能有一个
// 在等的闸门」由这里唯一裁决（后到者覆盖：闸门是每用户 1 个 running session 的串行面）。
func (e *Executor) registerConfirmGate(g *confirmGate) *confirmGate {
	e.confirmMu.Lock()
	e.confirmRegistry[g.sessionID] = g
	e.confirmMu.Unlock()
	return g
}

// confirmOutcome 闸门三种出路（终态语义必须可区分——「操作者主动不提交」和
// 「确认超时」在审计面同形记 failed，等于把一次正常的人工否决统计成系统故障）。
type confirmOutcome uint

const (
	confirmGranted confirmOutcome = iota
	confirmStoppedByUser
	confirmWaitTimedOut
)

// errConfirmAbortedByStop 挂起期间被 stop 中止的步错误原文。不复用「用户手动中断」那句
// 原文：步与审计面要写清「停在确认闸门、评论从未提交」，终态收口再按此常量精确认出
// 「这次失败的起因就是用户中断」→ 记 stopped。
// 起 post_comment 与派生写步（type+回车 / click 发送按钮）共用此原文——精确匹配
// 只认这一条，措辞按步分叉就会把「闸门处中止」重新掉回 failed。
const errConfirmAbortedByStop = "写步等待人工确认期间被用户中止，评论未提交"

// waitForConfirm 挂起等人工放行，自带独立计时器（不再让 execCtx.Done 兼职确认超时）。
// 前置：闸门已由 registerConfirmGate 挂上注册表——**登记早于落帧、落帧早于等待**，
// 反过来排会在「帧已写、注册表还空着」的窗口里把一次真放行答成「没有待确认的提交点」。
// 返回值第二项是**哪条预算到头**的原文（「确认等待 600s」还是「任务执行预算掐断」）——
// 两者在审计面上必须可区分，否则人就会去调 timeout_sec 而真正该调的是 confirm_wait_sec。
// 调用点在不可逆提交点之前，非 confirmGranted 分支从未点击过任何按钮——失败可安全重下发。
// stopChFor 未注册时返回 nil，select 对 nil 通道分支永不就绪，与 ctx.Done 并存安全。
func (e *Executor) waitForConfirm(ctx context.Context, gate *confirmGate) (confirmOutcome, string) {
	defer func() {
		e.confirmMu.Lock()
		if cur, ok := e.confirmRegistry[gate.sessionID]; ok && cur == gate {
			delete(e.confirmRegistry, gate.sessionID)
		}
		e.confirmMu.Unlock()
	}()

	wait := time.Until(gate.expiresAt)
	if wait < 0 {
		wait = 0
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	stopCh := e.stopChFor(gate.sessionID)
	select {
	case <-gate.ch:
		return confirmGranted, ""
	case <-stopCh:
		return confirmStoppedByUser, "用户在确认闸门处中止"
	case <-timer.C:
		return confirmWaitTimedOut, fmt.Sprintf("人工确认等待 %ds", int(gate.budget.Seconds()))
	case <-ctx.Done():
		// 执行预算（TimeoutSec）到头。它与确认预算是两条独立计时器，谁先到谁说话。
		return confirmWaitTimedOut, "任务执行预算用尽"
	}
}

// gateDecision 把闸门出路翻成审计面上的词（终态能分辨「谁否决了这次提交」，
// 帧也必须分辨——granted/stopped/timeout 三词在 command_log 里各占一类，不许合并）。
func gateDecision(out confirmOutcome) string {
	switch out {
	case confirmGranted:
		return "granted"
	case confirmStoppedByUser:
		return "stopped"
	default:
		return "timeout"
	}
}

// d7PreviewRunes 待批内容预览的截断长度：闸门读侧要让人「看见自己批的是什么」，
// 但一个 8000 字的长评论不该整段塞进弹窗——预览是给人核对用的，不是正文的第二份副本。
const d7PreviewRunes = 200

// 闸门的两类审计帧动作名。写侧在这里、读侧在 session.go 的 gateElsewhere——
// 两处各写一遍字面量，就会有一个拼错的常量悄悄把跨进程查证变成死代码（帧一直在落，没人查得到）。
const (
	gateWaitFrame    = "d7_wait"    // event：闸门开过、批的是这份载荷、到这个时候
	gateConfirmFrame = "d7_confirm" // judge：闸门收口，decision 分辨 granted/stopped/timeout
)

// 写链路上「试过了 / 接了 / 平台确认了」三件事的另两个名字。
// 三态不是给一列改名，而是每一态各有一帧可指：
//   - command 帧（ok=nil）= attempted：这一条命令被写给了 Host，此刻没有任何结论；
//     comment_send 单独成帧，是因为不可逆点埋在 post_comment 那一步内部——只有步级帧的审计流
//     分不清「填好文本没点出去」与「点出去了不知道成没成」，而这两者的处置完全相反。
//   - event 帧 = accepted：Host 回执到手且无错。
//   - judge 帧 write_confirm = confirmed：服务端收紧回查裁出的结论（state 带 verified /
//     unattributed）。它是审计流里唯一有资格回答「评论发出去了吗」的一帧，
//     且只带载荷哈希——正文证据在 extracted_data 与步结果里，不进离线导出件。
const (
	commentSendAction = "comment_send"
	writeConfirmFrame = "write_confirm"
)

// awaitConfirmGate 闸门的唯一入口（绑载荷 + 留痕 + 可跨进程查证）。
// 三件事必须同生同死，所以不做成调用方各自记得加的三步——漏一步的形状与漏一个闸门同构：
//   - 等待之前落 d7_wait 帧（登记之后）：这是「闸门确实开过、批的是这份载荷、到什么时候」的唯一落盘凭据。
//     进程内 map 在重启/多副本下什么都不是，而帧在库里。
//   - 载荷只带哈希不带正文：审计导出会把 payload 原样带进离线件，评论正文不该住在那儿。
//   - 出路收口时落 d7_confirm 帧（judge 类）：连「没人批、超时了」都要留一行，
//     否则审计面上「用户拒了」与「用户没来」又折成一句。
func (e *Executor) awaitConfirmGate(ctx context.Context, task *model.BrowserTask,
	session *model.BrowserSession, stepID uint, stepIndex int, preview, payloadHash string,
	seq *int, wait time.Duration) (confirmOutcome, string) {
	// 绑一份「空载荷」的闸门等于没绑：任何人都能以「我也批了这份」放行。调用方的键来自
	// writeStepKey（永不为空），这里只把那条前提变成硬前置——
	// 前提哪天被绕过，是编排 bug，该炸在原地而不是悄悄开一扇匿名放行口。
	if payloadHash == "" {
		panic("awaitConfirmGate: 空载荷哈希不得开闸门")
	}
	gate := &confirmGate{
		ch: make(chan struct{}), sessionID: session.ID, stepIndex: stepIndex,
		payloadHash: payloadHash, preview: truncateRunes(preview, d7PreviewRunes, "…"),
		expiresAt: time.Now().Add(wait), budget: wait,
	}
	// 登记必须先于落帧：帧一到，读侧（GET /sessions/:id/confirm-gate 与跨进程查证）就认为
	// 有人能放行，而此刻注册表还空着的话，一次真放行会被答成「没有待确认的提交点」。
	e.registerConfirmGate(gate)
	*seq++
	e.appendCommandLog(ctx, session.ID, task.ID, stepID, *seq, "event", gateWaitFrame, map[string]any{
		"payload_hash": payloadHash, "step_index": stepIndex,
		"expires_at": gate.expiresAt.Format(time.RFC3339Nano),
		"wait_sec":   int(wait.Seconds()),
	}, 0, verdict(true))

	start := time.Now()
	out, why := e.waitForConfirm(ctx, gate)
	waited := time.Since(start).Milliseconds()

	*seq++
	e.appendCommandLog(ctx, session.ID, task.ID, stepID, *seq, "judge", gateConfirmFrame, map[string]any{
		"payload_hash":      payloadHash,
		"step_index":        stepIndex,
		"decision":          gateDecision(out),
		"mismatch_attempts": gate.attempts(),
		"waited_ms":         waited,
	}, waited, verdict(out == confirmGranted))
	return out, why
}
