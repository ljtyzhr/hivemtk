package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// executor_step.go —— 单步执行（重试/退避）与写步双发闸、预算、确认闸门的编排。
// executeStepWithRetry 单步执行（含 retry/backoff），返回 (finalStatus, errMsg, resultJSON)。
// resultJSON：成功时的原语回包（F6a 页面变化证据用），失败为 nil。
// seq：session 局部命令日志计数器（P0-2，调用方持有保证 session 内单调、跨 session 隔离）。
// pageURL：调用方手上的当前页 URL 现货（brain 模式传本轮 snapshot 的 URL；显式编排无现货传 ""，
// 由 resolveOutreachDedupe 按需快照补齐）。只用于触达去重键的归一化，查不到即 fail-open 放行。
func (e *Executor) executeStepWithRetry(ctx context.Context, task *model.BrowserTask, session *model.BrowserSession, index int, step parsedStep, stopCh chan struct{}, seq *int, pageURL string) (string, string, json.RawMessage) {
	// 写步判定先于落库——is_write 要作为事实随步行一起存，
	// 事后从 action 名字反推会把「type+回车提交」这类隐形写漏掉。
	// 三态分类。effectUnknown（平台定位表取不到）与 effectWrite 同样进闸门，
	// 落库 is_write=true——判不出副作用时宣称「没有副作用」就是三道闸门一起消失。
	effect, writeWhy := classifyStepEffect(task, step)
	writeStep := effect.needsWriteGate()
	stepRow := &model.BrowserStep{
		SessionID: session.ID,
		TaskID:    task.ID,
		StepIndex: index,
		Action:    step.Action,
		Target:    step.Target,
		Value:     step.Value,
		Status:    "running",
		IsWrite:   writeStep,
	}
	if params, err := json.Marshal(buildStepParams(step)); err == nil && string(params) != "{}" {
		stepRow.Params = params
	}
	if err := e.stepRepo.BatchCreate(ctx, []*model.BrowserStep{stepRow}); err != nil {
		return "failed", "step 落库失败: " + err.Error(), nil
	}

	retries := step.RetryCount
	backoff := step.RetryBackoffMs
	if backoff <= 0 {
		backoff = 1000
	}
	// F2①：不可逆写原语服务端强制 retries=0——「提交成功但 verify 超时」是结果未知态，
	// 重试=可能双发（Postiz 接口级契约：不可逆变更 maximumAttempts:1）。不信任编排/LLM 传入的重试参数。
	if writeStep {
		retries = 0
	}
	// 双发闸 + 重试豁免：闸门必须在任何帧下发之前——prep/type 本身会改页面状态
	// （把草稿塞进输入框），不是「零副作用探测」。
	writeKey := ""
	// outreachCtx：写步去重键（命中跳过时为 nil，未命中但已备好键时非 nil）。
	// 下传给 dispatchStep，供 verified 后落去重行——检查与插入之间页面可能跳转，
	// 键必须在检查点冻结，不能等到提交后重新取 URL/重算 hash。
	var outreachCtx *outreachDedupeCtx
	if writeStep {
		// 本会话台账已经写失败过一次 ⇒ 之后的写步一帧都不下发。
		// 闸门依据的是库里的台账，台账自己写不进去时「过闸」只是走过场——
		// 与其赌一次双发，不如把这一步变成可见、可人工重跑的红步。
		if why, broken := e.ledgerBrokenReason(session.ID); broken {
			msg := fmt.Sprintf("写步拒绝下发（%s）：%s——台账未落，本会话写能力已降级，请人工核对已下发的步后重跑", writeWhy, why)
			e.finishStep(ctx, stepRow.ID, "failed", nil, 0, msg)
			logger.Warnf("[BrowserExec] %s session=%d idx=%d", msg, session.ID, index)
			return "failed", msg, nil
		}
		writeKey = writeStepKey(step)
		// 跨任务触达去重（Chunk3）：同平台 + 同归一化目标页 + 同文案已触达过 ⇒ 本步不下发。
		// 位置排在 writeKey 备齐之后、双发闸之前：去重是「更外层」的扰民拦截，先于会话内
		// 台账判据生效；命中即整步跳过并绿返（目标本就已达成，重发只会二次扰民）。
		// 去重库未接线 / 快照失败 / 查库报错一律 fail-open 放行，绝不因去重拦停发送。
		oc, dupHit, _ := resolveOutreachDedupe(ctx, e, task, step, writeKey, pageURL, session.ChromeTabID)
		if dupHit {
			msg := fmt.Sprintf("触达去重跳过（%s）：%s 已有同文案触达记录，跨任务防重复打扰", writeWhy, oc.targetURL)
			e.finishStep(ctx, stepRow.ID, "skipped", nil, 0, msg)
			*seq++
			e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "event", step.Action,
				mergeAuditMeta(map[string]any{
					"dedupe_hit": true, "target_url": oc.targetURL, "reason": "outreach_already_sent",
				}, e.stepAuditMeta(latencyNotMeasured)), 0, verdict(true))
			logger.Infof("[BrowserExec] %s session=%d idx=%d", msg, session.ID, index)
			// 绿返：去重命中意味着内容早已触达，本轮无需再证明什么，不计成败、不中断。
			return "skipped", "", nil
		}
		outreachCtx = oc
		switch err := e.guardResubmit(ctx, task, writeKey, stepRow.ID); {
		case errors.Is(err, errRetrySkipped):
			prior := priorOfSkippedWrite(err)
			msg := fmt.Sprintf("写步跳过（%s）：%v——同文本已有提交尝试，重发即双发", writeWhy, prior)
			e.finishStep(ctx, stepRow.ID, "skipped", nil, 0, msg)
			logger.Infof("[BrowserExec] %s session=%d idx=%d", msg, session.ID, index)
			if prior.verified() {
				return "skipped", "", nil
			}
			// 前一轮只到 sent/unattributed：提交从未被证明，而本轮既不重发（双发）也就无从证明。
			// 此时让整轮判绿就是双发闸要消灭的那类假绿，所以把事实上抛给调用方计败。
			return "skipped", fmt.Sprintf(
				"重试轮防双发未重发，前一轮提交未验证（%v）——本轮无法证明内容已发布，请人工核对", prior), nil
		case err != nil:
			e.finishStep(ctx, stepRow.ID, "failed", nil, 0, err.Error())
			return "failed", err.Error(), nil
		}
		// 读闸放行 ≠ 独占。上面的查询依据的是台账，「两条腿同时过闸、彼此都查空」
		// 是并发的常态形状而非意外，所以裁决权必须下推到库里唯一约束的那一次 INSERT。
		// 位置刻意排在 D7 挂起之前：挂起一分钟，另一条腿在这一分钟里过读闸、下发、提交，
		// 正是本处要消灭的那次双发——人工确认的等待期是并发窗口最宽的一刻，不是最窄的。
		if err := e.claimWriteSlot(ctx, task.ID, session.ID, stepRow.ID, writeKey); err != nil {
			msg := fmt.Sprintf("写步拒绝下发（%s）：%v", writeWhy, err)
			e.finishStep(ctx, stepRow.ID, "failed", nil, 0, msg)
			logger.Warnf("[BrowserExec] %s session=%d idx=%d", msg, session.ID, index)
			return "failed", msg, nil
		}
		// 腾坑挂在本步收尾（executeStepWithRetry 是步级作用域），不是会话级：
		// 坑留到会话结束会把「这一步从未跨提交点、下一会话应当能重跑」也一并留死。
		defer func() { e.releaseWriteSlot(ctx, task.ID, session.ID, stepRow.ID, writeKey) }()
		// Chunk4 活动触达条数预算：这一条是「本步真的会发出去」的最后一刻，故扣减排在此处——
		// 排在占坑之后，被双发闸/D7 确认挡掉的步不消费额度（它们最终没发出去）。
		// 预算耗尽 = 配置事实，不是异常：判红 + 拒绝下发 + 说明缺什么，让用户去调预算而不是重跑。
		if !e.consumeCampaignActBudget(ctx, task) {
			msg := fmt.Sprintf("写步未下发（%s）：%s", writeWhy, campaignBudgetExhaustedMsg(task))
			e.finishStep(ctx, stepRow.ID, "failed", nil, 0, msg)
			// seq 必须自增：与本函数其余所有 appendCommandLog 调用点同纪律。
			// 漏自增会让这一帧与上一帧共用同一个 seq，审计包按 seq 排序时两帧并列，
			// 「预算耗尽发生在哪一步之前/之后」这个问题就没有答案了。
			*seq++
			e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "event", step.Action,
				mergeAuditMeta(map[string]any{
					"reason": "campaign_budget_exhausted", "campaign_key": task.CampaignKey,
					"campaign_act_budget": task.CampaignActBudget, "campaign_act_used": task.CampaignActUsed,
				}, e.stepAuditMeta(latencyNotMeasured)), 0, verdict(false))
			return "failed", msg, nil
		}
	}
	// D7 覆盖派生写。把「type+回车 / click 发送按钮 / click_near 发送」认成写步之后，
	// 闸门却仍只长在 post_comment 原语内部——开了 require_confirm 的用户，这类隐形写照样被
	// 无条件发出去。此处按步挂起（post_comment 保留原语内的闸门：先 prep 填好正文再确认，
	// 让人看见将要发什么）。位置排在双发闸之后、任何命令帧之前：
	// 该跳过的步不该先问一遍再跳过，未放行则该步一帧都没下发。
	if writeStep && step.Action != "post_comment" && task.RequireConfirm {
		preview := step.Value
		if preview == "" {
			preview = strings.TrimSpace(step.Action + " " + step.Target)
		}
		out, why := e.awaitConfirmGate(ctx, task, session, stepRow.ID, index, preview, writeKey, seq, confirmWaitBudget(task))
		if out != confirmGranted {
			msg := fmt.Sprintf("写步等待人工确认超时（%s），评论未提交", why)
			if out == confirmStoppedByUser {
				msg = errConfirmAbortedByStop
			}
			e.finishStep(ctx, stepRow.ID, "failed", nil, 0, msg)
			return "failed", msg, nil
		}
	}
	var lastErr string
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if !sleepInterruptible(ctx, stopCh, retryBackoffDelay(backoff, attempt)) {
				e.finishStep(ctx, stepRow.ID, "failed", nil, 0, "用户手动中断")
				return "failed", "用户手动中断", nil
			}
		}
		start := time.Now()
		// P8 append-only 命令日志：命令帧先落（含步骤参数全文），回包/错误随 result 事件再落。
		// 命令帧 ok=nil（/ A2）：写下这一行时 Host 连回执都还没有，甚至可能
		// 根本没进 socket——若传字面量 true，审计包里每条「下发」都自带一个 ✓，
		// 而那个 ✓ 在这一行上什么也不指。它 attempted，不 succeeded。
		*seq++
		cmdPayload := map[string]any{"action": step.Action, "target": step.Target, "params": buildStepParams(step)}
		// Chunk4：写步的下发帧带上活动预算水位（配置了活动预算时）。读审计包的人据此能回答
		// 「这次触达是第几次、还剩几条」，而不必去猜时间戳落在哪次运行里。
		// 未配置预算则整段不出现（不写 0/0 的假水位）。
		if writeStep {
			if snap := campaignBudgetSnapshot(task); snap != nil {
				for k, v := range snap {
					cmdPayload[k] = v
				}
			}
		}
		e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "command", step.Action, cmdPayload, 0, nil)
		result, err := e.dispatchStep(ctx, task, session, step, stepRow, seq, outreachCtx)
		dur := time.Since(start).Milliseconds()
		// 回包帧自己占一个号：dispatchStep 内部还会落 d7_*/comment_send/write_confirm 帧，
		// 沿用「一问一答同序号」会让步级 event 与它们同号（审计面上两条同号帧，
		// 按 seq 还原执行序的读者会看到一次动作凭空分成两叉）。
		*seq++
		if err == nil {
			// 写步的「成功」必须以台账落成前提。命令确实下发了，但库里没有这次
			// 提交的凭据 ⇒ 下一轮无从得知它发生过 ⇒ 让整轮绿就是拿不可逆动作换一次好看的状态。
			var ledgerErr error
			if writeStep {
				ledgerErr = e.recordGenericWriteLedger(ctx, task.ID, session.ID, step, stepRow.ID, writeKey, nil)
			}
			if ledgerErr != nil {
				msg := fmt.Sprintf("写步已下发但台账未落（%v）——本轮判红：命令可能已生效，重发即双发，请人工核对该步结果", ledgerErr)
				logger.Warnf("[BrowserExec] %s session=%d idx=%d action=%s", msg, session.ID, index, step.Action)
				e.finishStep(ctx, stepRow.ID, "failed", result, dur, msg)
				e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "event", step.Action,
					mergeAuditMeta(map[string]any{"error": msg}, e.stepAuditMeta(dur)), dur, verdict(false))
				return "failed", msg, json.RawMessage(result)
			}
			e.finishStep(ctx, stepRow.ID, "success", result, dur, "")
			e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "event", step.Action,
				mergeAuditMeta(map[string]any{"result": json.RawMessage(auditEventPayload(result))}, e.stepAuditMeta(dur)),
				dur, verdict(true))
			return "success", "", json.RawMessage(result)
		}
		lastErr = err.Error()
		if writeStep {
			// 步本身已判败，台账再写不进去只加一句因由：缺口由进程内兜底与降级表接管（A7）。
			if ledgerErr := e.recordGenericWriteLedger(ctx, task.ID, session.ID, step, stepRow.ID, writeKey, err); ledgerErr != nil {
				lastErr = fmt.Sprintf("%s；且写台账未落（%v）——重跑本任务前请人工核对该步是否已生效", lastErr, ledgerErr)
			}
		}
		e.appendCommandLog(ctx, session.ID, task.ID, stepRow.ID, *seq, "event", step.Action,
			mergeAuditMeta(map[string]any{"error": lastErr}, e.stepAuditMeta(dur)), dur, verdict(false))
		logger.Warnf("[BrowserExec] step 失败 session=%d idx=%d action=%s attempt=%d: %s", session.ID, index, step.Action, attempt, lastErr)
		// F7：重试按平台错误归因分线（MediaCrawler 处置矩阵语义）——
		// bad_body（内容被拒）重试无意义直接终止；refresh_token/disconnect 同样终止（交上层 session 级处置）；
		// 仅 retry（瞬态）继续退避重试。
		if attempt < retries && !stepErrRetryable(taskPlatformID(task), lastErr) {
			logger.Warnf("[BrowserExec] 错误分类为不可重试，终止步重试 session=%d action=%s", session.ID, step.Action)
			break
		}
	}
	e.finishStep(ctx, stepRow.ID, "failed", nil, 0, lastErr)
	return "failed", lastErr, nil
}
