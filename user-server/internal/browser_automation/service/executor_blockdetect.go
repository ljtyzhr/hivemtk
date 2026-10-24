package service

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/platform"
	"hivemtk-user/internal/pkg/utils/logger"
)

// executor_blockdetect.go —— 铁律 4 拦截页检测（步后 snapshot → 平台判据 → 风控归因）。
// detectBlockedIfFatal 拦截页检测（铁律 4 全自动闭环）：步后 snapshot 命中平台拦截判据
// → 按 ClassifyError 归因，disconnect 类风控直接终止（重试无意义），其余继续（交给重试/自愈）。
// 检测失败（snapshot 拿不到）不阻断主流程——检测是增强不是闸门。
// seq：session 局部命令日志计数（F6：探测帧必须进审计包——本轮真机取证只能靠服务端日志
// 时间线反推探测实耗，审计面上「检测到底跑没跑、跑了多久、看了什么」完全不可见）。
func (e *Executor) detectBlockedIfFatal(ctx context.Context, task *model.BrowserTask, session *model.BrowserSession, seq *int) (blocked bool, reason string) {
	p, err := platform.Get(taskPlatformID(task))
	if err != nil {
		return false, ""
	}
	if session.ChromeTabID <= 0 {
		// 本轮没开过 tab，或 close_tab 步已把 tab 回收（F10 后内存 id 归零）——没有页面可检。
		// 必须在写审计帧之前返回：拿 tab 0 去 snapshot 只会留一条注定失败的探测行污染审计包。
		return false, ""
	}
	var (
		start       = time.Now()
		snap        string
		pageURL     string
		snapErr     string
		probed      int
		blockedNow  bool
		selectorHit bool
	)
	defer func() {
		if seq == nil {
			return
		}
		*seq++
		// 审计用外层 ctx：探测预算可能已耗尽，落库不能被探测自身的超时带走。
		// ok=「探测这条命令本身成没成」，不是「有没有拦到」——若传 blocked，于是每一页
		// 正常页面的探测都记成 ok=false（验证：审计包里 block_detect 全红，
		// 读包的人会以为检测链路坏了；拦截结论另有 blocked/reason 字段承载）。
		e.appendCommandLog(ctx, session.ID, task.ID, 0, *seq, "event", "block_detect", map[string]any{
			"url": pageURL, "snapshot_chars": len(snap), "snapshot_error": snapErr,
			"selectors_probed": probed, "detect_block_hit": blockedNow, "selector_hit": selectorHit,
			"blocked": blocked, "reason": reason,
		}, time.Since(start).Milliseconds(), verdict(snapErr == ""))
	}()
	// 整段探测共用一个短预算：snapshot（实测 24 次 max 876ms）+ 弹层选择器逐个 query，
	// 若各自吃满 defaultCmdTimeout，失败步（本身已耗 30s）会被拖成 60s+，而且探测命令自身的
	// 超时还会计入 A5 僵尸判定，把「步失败」误升级成「Host 假死」。预算内拿不到证据即放行。
	pctx, cancel := context.WithTimeout(ctx, blockDetectBudget)
	defer cancel()
	snap, pageURL, err = e.hand.snapshot(pctx, task.UserID, session.ChromeTabID)
	if err != nil {
		snapErr = err.Error()
		return false, ""
	}
	// 风控提示页只有标题+两个按钮，a11y 采集出的是空快照。
	// 旧闸门 `snap == ""` 于是把「URL 已经明写 website-login/error?error_code=300012」的
	// 拦截页判成「无数据」直接放行——URL 层判定不该依赖快照有没有内容。
	// 只有「快照和 URL 都拿不到」（旧扩展 / tab 已死）才算真无证据。
	if snap == "" && pageURL == "" {
		return false, ""
	}
	blockedNow = p.DetectBlock(pageURL, snap)
	// A2 容器层：URL 不变的弹层式拦截（如闲鱼 baxia 滑块）——风控专用选择器经 query
	// exists 下探。fail-soft：查询出错不作拦截证据（检测是增强不是闸门）。
	if !blockedNow {
		if bs, ok := p.(platform.BlockSelectorDeclarer); ok {
			for _, sel := range bs.BlockSelectors() {
				probed++
				res, qerr := e.hand.query(pctx, task.UserID, session.ChromeTabID, "exists", sel, "")
				if qerr == nil && res["exists"] == true {
					blockedNow = true
					selectorHit = true
					logger.Infof("[BrowserExec] 拦截容器命中 platform=%s selector=%s session=%d", p.Identifier(), sel, session.ID)
					break
				}
			}
		}
	}
	if !blockedNow {
		return false, ""
	}
	et := p.ClassifyError(pageURL + "\n" + snap)
	if selectorHit && et != platform.ErrDisconnect {
		// 风控专用容器命中即定性拦截（弹层文案可能不进快照结构行，分类不得漏回 Unknown）
		et = platform.ErrDisconnect
	}
	logger.Warnf("[BrowserExec] 平台拦截页命中 platform=%s errType=%s session=%d", p.Identifier(), et, session.ID)
	switch et {
	case platform.ErrDisconnect:
		return true, fmt.Sprintf("平台风控拦截（%s）：检测到验证码/风控页，账号需人工介入，会话终止", p.Identifier())
	default:
		// refresh_token/bad_body/retry 类：记录但不终止（bad_body 语义是内容被拒，页面上不会常驻）
		return false, ""
	}
}
