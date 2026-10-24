package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/browser_automation/dto"
	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// executor_session.go —— Session 生命周期：显式编排 ExecuteSession 与 Brain 模式 executeBrain。
// cleanupSessionTab 会话收口时关闭自身 tab。脱离执行 ctx 的取消状态（超时/中止腿的
// ctx 此刻必然已 Done，用它发命令等于注定失败），幂等——tab 已被 close_tab 步或用户关掉时
// 扩展侧吞掉异常返回 ok。回收失败只降级为日志与审计行，绝不改判终态。
func (e *Executor) cleanupSessionTab(baseCtx context.Context, task *model.BrowserTask,
	session *model.BrowserSession, seq *int) {
	if session.ChromeTabID <= 0 {
		return // 没开过 tab（如 Host 离线即失败）——无从回收，也不该发命令
	}
	cctx, cancel := context.WithTimeout(context.WithoutCancel(baseCtx), sessionTabCleanupBudget)
	defer cancel()
	start := time.Now()
	err := e.hand.closeTab(cctx, task.UserID, session.ChromeTabID)
	*seq++
	e.appendCommandLog(cctx, session.ID, task.ID, 0, *seq, "event", "session_tab_cleanup",
		map[string]any{"tab_id": session.ChromeTabID, "error": sendErrText(err)},
		time.Since(start).Milliseconds(), verdict(err == nil))
	if err != nil {
		logger.Warnf("[BrowserExec] session=%d tab=%d 收口回收失败（不影响终态）: %v",
			session.ID, session.ChromeTabID, err)
	}
}

// ExecuteSession 执行一个 session（在独立 goroutine 中运行）。
// ctx 由调用方包上 task.TimeoutSec 超时。
func (e *Executor) ExecuteSession(ctx context.Context, task *model.BrowserTask, session *model.BrowserSession, steps []parsedStep) {
	stopCh := e.registerStop(session.ID)
	defer e.unregisterStop(session.ID)
	defer e.clearLedgerBroken(session.ID)
	// 回执兜底清理：正常路径上 captureSessionReceipts 自己摘掉这条 entry，
	// 这条 defer 只在「会话中途 panic / 提前退出、收口没跑到」时兜底。
	// 不加它，Executor 是进程级单例，泄漏的 pendingReceipts 会随异常会话单调增长且永不回收——
	// 而这类泄漏的特征是「进程跑得越久越慢」，事后从日志里根本看不出来是哪次会话留下的。
	defer e.discardPendingReceipts(session.ID)

	_ = e.sessionRepo.UpdateStatus(ctx, session.ID, "active", "")
	_ = steps // Brain 模式忽略显式编排，由 LLM 生成

	var success, failed int
	sessionFailed := ""
	cmdSeq := 0 // session 局部命令日志计数（跨 session 隔离，避免共享计数器 data race）

	if task.BrainMode {
		// Brain 模式：LLM 每轮看快照出 plan → 执行 → done 判定（loop_count 不适用）
		//
		// cmdSeq 必须与本函数共用同一个计数器（**不**在 executeBrain 内部另开一个）：
		// Brain 段落的帧与本函数收口段落的帧（captureSessionReceipts / cleanupSessionTab）
		// 落在同一个 session 的同一条命令流里，seq 是那条流里的唯一序号。两个计数器
		// 都从 0 起 ⇒ 收口帧与 Brain 帧撞号，而撞号之后按 seq 排序无法还原先后，
		// 裁剪摘要的 [FirstSeq,LastSeq] 区间也会与行数对不上（断号信号误报成真缺行）。
		success, failed, sessionFailed = e.executeBrain(ctx, task, session, stopCh, &cmdSeq)
	} else {
		loopCount := task.LoopCount
		if loopCount < 1 {
			loopCount = 1
		}

	loop:
		for round := 0; round < loopCount; round++ {
			for i, step := range steps {
				// 手动中断
				if e.stopFired(stopCh) {
					sessionFailed = "用户手动中断"
					break loop
				}
				// ctx 超时（task.TimeoutSec）
				if ctx.Err() != nil {
					sessionFailed = fmt.Sprintf("执行超时（%ds）", task.TimeoutSec)
					break loop
				}
				// 步间间隔（可被 stop 打断）
				if i > 0 || round > 0 {
					if !sleepInterruptible(ctx, stopCh, humanizedDelay(task.DelayMs)) {
						sessionFailed = "用户手动中断"
						break loop
					}
				}
				// 轮次 >0 时重新 open_tab 的场景由显式 steps 表达；此处不隐式开 tab
				status, errMsg, _ := e.executeStepWithRetry(ctx, task, session, i, step, stopCh, &cmdSeq, "")
				switch status {
				case "success":
					success++
				case "skipped":
					// 写步在自动重试轮撞见历史提交尝试 → 整步不下发。两种事实分开判：
					//   前一轮 verified → 目标已达成，本轮只是补完剩余步，不计成败也不中断；
					//   前一轮 sent/unattributed → 提交从未被证明，本轮防双发也无从证明，
					//     必须让本轮判红（否则「重试轮全绿」就是双发闸要消灭的那类假绿），
					//     但不 break——后面的只读步照常跑完，现场证据越全越好判。
					if errMsg != "" {
						failed++
						if sessionFailed == "" {
							sessionFailed = errMsg
						}
					}
				case "failed":
					failed++
					if !step.ContinueOnError {
						sessionFailed = errMsg
						// 风控页最常见的表现恰恰是「某步定位失败」（小红书跳
						// website-login/error?error_code=300012「IP存在风险」即此形）：
						// 若在此直接 break 跳过步后拦截检测，风控页会被误报成
						// selector_timeout——归因错方向，人工就会去改选择器而不是换网络。
						// 失败收口前先过一次拦截判定，命中即用语义更准的风控归因替换原始错误。
						if blocked, reason := e.detectBlockedIfFatal(ctx, task, session, &cmdSeq); blocked {
							sessionFailed = reason
							// 熔断写入：同平台新任务启动门 + 人工恢复（best-effort，不改终止语义）
							_ = e.recordProfileBlocked(ctx, task, session, reason)
						}
						break loop
					}
				}
				// 拦截页检测（铁律 4）：成功步后页面可能已跳风控页——disconnect 类终止，全自动闭环
				if blocked, reason := e.detectBlockedIfFatal(ctx, task, session, &cmdSeq); blocked {
					sessionFailed = reason
					// 熔断写入：同平台新任务启动门 + 人工恢复（best-effort，不改终止语义）
					_ = e.recordProfileBlocked(ctx, task, session, reason)
					break loop
				}
			}
		}
	}

	finalStatus := "completed"
	if sessionFailed != "" {
		finalStatus = "failed"
		// F4：stopped=「用户主动叫停」，与「系统跑失败」是两类事实。D7 挂起期间被 stop
		// 中止同样归 stopped（原文常量精确匹配，不靠字符串模糊匹配）。
		if e.stopFired(stopCh) && (sessionFailed == "用户手动中断" || sessionFailed == errConfirmAbortedByStop) {
			finalStatus = "stopped"
		}
	}
	// executeBrain 因 ctx 超时收敛返回后，ctx 已 Done——用原 ctx 写终态会被 DB
	// 驱动取消，session 永久停留 active（兜底收敛白跑）。
	// 终态收口必须用脱离取消的 context（独立时限见 timeouts.go，只保写库完成，不继承执行期取消）。
	writeCtx, cancelWrite := context.WithTimeout(context.WithoutCancel(ctx), sessionFinalWriteBudget)
	defer cancelWrite()
	_ = e.sessionRepo.UpdateStatus(writeCtx, session.ID, finalStatus, sessionFailed)
	_ = e.sessionRepo.UpdateMetrics(writeCtx, session.ID, success+failed, success, failed)

	// F10 会话收口必须回收自己打开的 tab。openTab 从不复用（tab-manager.js:6 每次 tabs.create），
	// 而 failed/stopped/超时 三条路径根本走不到编排里末尾的 close_tab 步——同一
	// Profile 会持续累积泄漏 tab（cron 任务按周期无限增长，后台 tab 常驻还拖慢 SW）。
	// 「留着给用户看结果」不成立：证据已在审计包（快照/截图/command_log），且崩后
	// chrome_tab_id 本就失效（续跑唯一合法入口是重新 open_tab）。
	// 触达回执：拍验收截图 + 落本会话的回执行。
	// **必须在 cleanupSessionTab 之前**：tab 一回收，captureVisibleTab 就只能截到用户
	// 自己的页面（静默假内容，比没有更坏），顺序颠倒等于回执永远拍不到真页面。
	e.captureSessionReceipts(writeCtx, task, session, &cmdSeq)
	e.cleanupSessionTab(ctx, task, session, &cmdSeq)

	// Brain 模式：LLM 总结执行结果落 llm_summary（P2-2：completed 与 failed 都总结——失败归因同样是交付物）
	if e.brain != nil && task.BrainMode && (finalStatus == "completed" || finalStatus == "failed") {
		if summ := e.brain.SummarizeSession(writeCtx, task.ID, session.ID, task.BrainGoal, success, success+failed, string(session.ExtractedData)); summ != "" {
			_ = e.sessionRepo.UpdateArtifacts(writeCtx, session.ID, nil, "", summ)
		}
	}

	// Brain 总结 / 失败重试调度 / 通知 —— FeedbackService 内部异步
	if e.feedback != nil {
		e.feedback.OnSessionFinished(ctx, task, session, finalStatus, success, success+failed)
	}
}

// executeBrain Brain 模式执行循环：兜底开 tab → snapshot → LLM plan → 执行 → done 判定。
// 返回 (success, failed, sessionFailed)；sessionFailed 为空表示目标达成（done）。
// 容错：LLM 编排失败/snapshot 失败不直接终止——先尝试恢复 tab 再重试本轮，
// 连续 brainMaxFailures 次失败才放弃（LLM JSON 抖动与 Host 空闲断连都是瞬态）。
//
// seq 是**调用方那个** session 局部计数器（`cmdSeq *int`），不是本函数另开的一个：
// Brain 段落的帧与 ExecuteSession 收口段落的帧落在同一个 session 的同一条命令流里，
// seq 是那条流里的唯一序号。另开一个计数器 ⇒ 收口帧与 Brain 帧撞号 ⇒ 按 seq 排序
// 无法还原先后，裁剪摘要的区间也会与行数对不上。
func (e *Executor) executeBrain(ctx context.Context, task *model.BrowserTask, session *model.BrowserSession, stopCh chan struct{}, cmdSeq *int) (int, int, string) {
	if e.brain == nil {
		return 0, 0, "Brain 服务未装配"
	}
	success, failed := 0, 0
	stepIdx := 0
	exceeded := true
	consecutiveFails := 0
	// 执行历史摘要（给 LLM 的跨轮记忆：做过什么、当前进度），防止原地打转
	var history []string
	// reflect 跨轮状态（对标 browser-use evaluation/memory 回喂）
	var prevEvaluation, memory string
	if cmdSeq == nil {
		cmdSeq = new(int) // 兜底：直接调本函数的测试不该因此 panic
	}
	// P0-4：LLM 下发参数钳位状态
	consecutiveActionFails := 0
	judgeFailOpen := 0 // P1-2 连续 fail-open 计数
	// 压缩台账：滑窗溢出条目按动作折叠计数，台账首行回喂 LLM（零 LLM 成本的 browser-use 压缩等价）
	foldedHistory := map[string]int{}
	// P1-1：session 级 token 预算熔断（plan+judge 全计入）
	// Chunk4：预算可由任务字段 TokenBudget 覆盖（0=用默认常量），取数唯一入口 taskTokenBudget。
	tokenUsed := 0
	tokenBudget := taskTokenBudget(task)
	// JEV session 级熔断/独立预算（Executor 进程级单例不放计数，随 task 结束丢弃）
	jevSess := &jevSessionState{}
	// wall-clock 兜底收敛：ctx 取消链在某些 LLM/DB 调用栈不生效（实测出现过 11min+ active），
	// 以真实时钟兜底——超执行预算+30s 强制收敛，会话必有终态。
	// 预算走 taskExecBudget（Brain 模式下 LLM 也能编排出写步，确认挂起同样要留出时长）
	deadline := time.Now().Add(taskExecBudget(task) + taskWatchdogGrace)

	for iter := 0; iter < maxBrainIterations; iter++ {
		if e.stopFired(stopCh) {
			return success, failed, "用户手动中断"
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return success, failed, fmt.Sprintf("执行超时（%ds）", int(taskExecBudget(task).Seconds()))
		}
		if iter > 0 && !sleepInterruptible(ctx, stopCh, humanizedDelay(task.DelayMs)) {
			return success, failed, "用户手动中断"
		}
		// 兜底开 tab：LLM 首轮 plan 若未显式 open_tab，保证有页面可操作
		if session.ChromeTabID == 0 {
			tabID, _, err := e.hand.openTab(ctx, task.UserID, task.Url, false, 0)
			if err != nil {
				return success, failed, "open_tab 失败: " + err.Error()
			}
			session.ChromeTabID = tabID
			_ = e.sessionRepo.UpdateChromeTabID(ctx, session.ID, tabID)
		}
		snap, pageURL, err := e.hand.snapshot(ctx, task.UserID, session.ChromeTabID)
		if err != nil {
			// tab 可能在 LLM 思考间隙被 SW 空闲回收/用户关闭：重开一次再 snapshot
			logger.Warnf("[BrowserExec] brain snapshot 失败 session=%d tab=%d: %v，尝试重开 tab", session.ID, session.ChromeTabID, err)
			tabID, _, openErr := e.hand.openTab(ctx, task.UserID, task.Url, false, 0)
			if openErr != nil {
				return success, failed, "snapshot 失败且重开 tab 失败: " + openErr.Error()
			}
			session.ChromeTabID = tabID
			_ = e.sessionRepo.UpdateChromeTabID(ctx, session.ID, tabID)
			*cmdSeq++ // P2-1 自愈动作落审计
			e.appendCommandLog(ctx, session.ID, task.ID, 0, *cmdSeq, "event", "open_tab_recovery", map[string]any{"reason": "snapshot_failed", "new_tab": tabID}, 0, verdict(true))
			if snap, pageURL, err = e.hand.snapshot(ctx, task.UserID, session.ChromeTabID); err != nil {
				return success, failed, "snapshot 失败: " + err.Error()
			}
		}
		// reflect 状态（对标 browser-use MessageManager）：跨轮评估/记忆/历史。
		// 压缩视图：折叠台账非空时作为历史首行喂 LLM（滑窗只保近史，构成信息不丢）。
		histView := history
		if hdr := buildFoldHeader(foldedHistory); hdr != "" {
			histView = append([]string{hdr}, history...)
		}
		st := &reflectState{History: histView, PrevEvaluation: prevEvaluation, Memory: memory}
		// JEV 优先、Brain 回退（设计步骤四 §2：planRound 内决策；JEV 未启用/失败即原路径）。
		planOut, err := e.planRound(ctx, task, session, snap, pageURL, st, history, cmdSeq, jevSess)
		stepsJSON, done, terminal := planOut.stepsJSON, planOut.done, planOut.terminal
		if terminal != "" {
			return success, failed, terminal
		}
		prevEvaluation, memory = st.PrevEvaluation, st.Memory
		tokenUsed += planOut.brainTokens // P1-1 session 级 token 计量（Brain 口径；JEV 走独立预算）
		jevSess.tokensUsed += planOut.jevTokens
		if jevSess.tokensUsed > jevSessionTokenBudget {
			jevSess.off = true // JEV 独立预算超限：本 session 剩余轮次直走 Brain
		}
		if planOut.jevAttempted && !planOut.jevOK {
			jevSess.fails++
			if jevSess.fails >= jevMaxSessionFails {
				jevSess.off = true // JEV 连续失败熔断：剩余轮次直走 Brain
			}
		}
		if tokenUsed > tokenBudget {
			return success, failed, "Token 预算耗尽（" + itoa(tokenUsed) + " > " + itoa(tokenBudget) + "）"
		}
		if err != nil {
			consecutiveFails++
			logger.Warnf("[BrowserExec] brain plan 连续失败 %d/%d session=%d: %v", consecutiveFails, brainMaxPlanFailures, session.ID, err)
			if consecutiveFails >= brainMaxPlanFailures {
				return success, failed, "LLM 编排失败: " + err.Error()
			}
			// 瞬态失败：回到循环顶部重新 snapshot（跳过本轮已耗时）
			continue
		}
		consecutiveFails = 0
		if done {
			// 独立 judge 验收（对标 browser-use judge）：agent 自称完成 ≠ 真完成。
			// F5：证据改为**重拍的页面真实快照**（不再是自报摘要）——验收员只信页面；
			// 快照重拍失败才降级回自报摘要（fail-soft：judge 增强不阻断主流程）。
			judgeCtx := ctx
			evidenceSnap, _, snapErr := e.hand.snapshot(ctx, task.UserID, session.ChromeTabID)
			var evidence string
			if snapErr == nil && evidenceSnap != "" {
				evidence = fmt.Sprintf("最新页面快照（独立复核证据，agent 无法伪造）：\n%s", truncateRunes(evidenceSnap, 16000, "\n…[证据快照已截断]"))
			} else {
				evidence = fmt.Sprintf("提取数据摘要:%s\n最近动作:%s", truncate(string(session.ExtractedData), 2048), strings.Join(lastN(history, 6), "; "))
			}
			approve, reason := e.brain.JudgeDone(judgeCtx, task.ID, session.ID, task.BrainGoal, evidence)
			tokenUsed += e.brain.LastAuxTokens() // D3/G3：judge 消耗计入 session 预算
			if tokenUsed > tokenBudget {
				return success, failed, "Token 预算耗尽（" + itoa(tokenUsed) + " > " + itoa(tokenBudget) + "）"
			}
			// P1-2：连续 fail-open 视为未通过（验收增强挂掉≠放行伪装成功）
			if reason == "judge_unavailable" {
				judgeFailOpen++
				if judgeFailOpen >= 2 {
					approve = false
					reason = "连续两次验收服务不可用，不放行"
				}
			} else {
				judgeFailOpen = 0
			}
			// P1-2：judge 结果落 command_log（direction=judge，审计可查）
			*cmdSeq++
			e.appendCommandLog(ctx, session.ID, task.ID, 0, *cmdSeq, "judge", "judge_done",
				map[string]any{"approve": approve, "reason": reason}, 0, verdict(approve))
			if approve {
				exceeded = false
				break
			}
			logger.Warnf("[BrowserExec] judge 拒绝 done（%s），继续循环 session=%d", reason, session.ID)
			prevEvaluation = "验收员拒绝了 done 判定（" + reason + "），目标未真正达成，继续执行"
			continue
		}
		var items []dto.StepItem
		if err := json.Unmarshal(stepsJSON, &items); err != nil {
			consecutiveFails++
			if consecutiveFails >= brainMaxPlanFailures {
				return success, failed, "LLM plan steps 解析失败: " + err.Error()
			}
			continue
		}
		// done=false 且 steps 空 = 无进展轮（模型只输出了思维链/空计划）：
		// 计入连续失败，避免 0 步死循环烧完 maxBrainIterations
		if len(items) == 0 {
			consecutiveFails++
			logger.Warnf("[BrowserExec] brain 空 plan（done=false steps=[]）连续 %d/%d session=%d", consecutiveFails, brainMaxPlanFailures, session.ID)
			if consecutiveFails >= brainMaxPlanFailures {
				return success, failed, "Brain 计划连续为空（模型未产出可执行步骤），目标未达成"
			}
			continue
		}

		abort := ""
		truncated := ""
		for si, it := range items {
			if abort != "" {
				break
			}
			if e.stopFired(stopCh) {
				abort = "用户手动中断"
				continue
			}
			if ctx.Err() != nil {
				abort = fmt.Sprintf("执行超时（%ds）", task.TimeoutSec)
				continue
			}
			// Brain 的 steps 是 LLM 原文 Unmarshal 出来的，REST 那条 oneof 校验在这条路上
			// 一行都不跑——落库前先过服务端闸门（G17 的 screenshot 闸是它的一条已存在的腿）。
			if rej := brainPlanStepRejection(it.Action); rej != "" {
				logger.Warnf("[BrowserExec] brain 轮内步骤被服务端拒绝 session=%d: %s", session.ID, rej)
				history = appendHistoryBounded(history, rej, foldedHistory)
				continue
			}
			step := parsedStep{
				StepItem:        it,
				ContinueOnError: it.ContinueOnError,
				RetryCount:      it.RetryCount,
				RetryBackoffMs:  it.RetryBackoffMs,
			}
			// P0-4：LLM 幻觉参数服务端钳位
			step.RetryCount, step.RetryBackoffMs = clampBrainStepParams(step.RetryCount, step.RetryBackoffMs)
			status, errMsg, stepResult := e.executeStepWithRetry(ctx, task, session, stepIdx, step, stopCh, cmdSeq, pageURL)
			stepIdx++
			// 历史记录（滑窗压缩：溢出最旧条折叠入账）：LLM 下轮能看到已做过的关键动作
			hist := step.Action + " " + step.Target
			if step.Action == "click" {
				hist += " → " + status
			}
			history = appendHistoryBounded(history, hist, foldedHistory)
			switch status {
			case "success":
				success++
				consecutiveActionFails = 0
			case "skipped":
				// Brain 重试轮里的写步跳过，按前一轮台账态分判（见 ExecuteSession 同分支说明）。
				// verified → 目标已达成，计成功让 LLM 继续收尾；未验证 → 计败并终止本轮，
				// 因为「发这条内容」这个目标在本轮已不可能再达成（重发即双发），继续烧 LLM 调用没有出路。
				if errMsg == "" {
					success++
					consecutiveActionFails = 0
				} else {
					failed++
					abort = errMsg
				}
			case "failed":
				failed++
				consecutiveActionFails++
				if consecutiveActionFails >= brainMaxActionFails {
					abort = "连续动作失败达上限（" + errMsg + "）"
				} else if !step.ContinueOnError {
					abort = errMsg
				}
			}
			// F6a（G15 先行项）：页面改变型动作成功且本轮仍有后续步 → 立即截断本轮。
			// 后续步引用的 @eN/CSS 属于旧页面快照，继续执行=错位（browser-use「页面变即截断」语义；
			// 下一迭代自然重拍快照，规划不丢上下文）。证据=open_tab 必然换页 / click 回包 navigated。
			if status == "success" && si < len(items)-1 && stepChangedPage(it.Action, stepResult) {
				truncated = it.Action
				break
			}
		}
		if truncated != "" {
			hist := truncated + " → 页面已跳转，本轮计划剩余步骤已跳过（下轮将基于最新快照重规划）"
			history = appendHistoryBounded(history, hist, foldedHistory)
			logger.Infof("[BrowserExec] brain 轮内 %s 引发页面变化，截断剩余步骤 session=%d", truncated, session.ID)
		}
		if abort != "" {
			// 步骤失败多为 tab 丢失（SW 空闲回收）：不清场，交给下一轮 snapshot 恢复逻辑；
			// 只有 open_tab 本身失败（Host 不可用）才终止
			if strings.HasPrefix(abort, "open_tab 失败") || strings.Contains(abort, "browser host 未连接") {
				return success, failed, abort
			}
			logger.Warnf("[BrowserExec] brain 轮内步骤失败 session=%d: %s（下轮 snapshot 自恢复）", session.ID, abort)
		}
		// 拦截页检测（铁律 4）：Brain 轮后同样检测——风控页命中即终止，全自动闭环
		if blocked, reason := e.detectBlockedIfFatal(ctx, task, session, cmdSeq); blocked {
			// 熔断写入：同平台新任务启动门 + 人工恢复（best-effort，不改终止语义）
			_ = e.recordProfileBlocked(ctx, task, session, reason)
			return success, failed, reason
		}
		// 循环检测 nudge（对标 browser-use 循环指纹）：连续 3 轮同序列 → 注入换路径提示
		// 文案走 brain_prompts 工厂（prompt 资产化铁律，禁止散落拼接）
		if stuck, seq := loopFingerprint(history); stuck {
			logger.Warnf("[BrowserExec] brain 循环检测命中 session=%d seq=%s，注入 nudge", session.ID, seq)
			prevEvaluation = strings.TrimSpace(BuildLoopNudge(strings.Split(seq, "|")))
		}
	}
	if exceeded {
		return success, failed, fmt.Sprintf("Brain 模式超过最大迭代次数（%d），目标未达成", maxBrainIterations)
	}
	return success, failed, ""
}
