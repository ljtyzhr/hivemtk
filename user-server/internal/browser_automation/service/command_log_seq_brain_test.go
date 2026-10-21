package service

// 契约锁（实体审查 #48，BrowserCommandLog）：seq 在一个 session 内必须单调递增且不重复。
//
// 为什么这一格承重：`browser_command_log.seq` 是「session 内单调递增」的序号，审计包
// 按它排序、裁剪摘要用它算 [FirstSeq, LastSeq] 区间与「有没有断号」。一旦同一个 session
// 里出现两个相同的 seq：
//   - 按 seq 排序时两帧并列 → 「这一步发生在那一步之前还是之后」没有答案；
//   - 摘要的区间长度与行数不再对得上（FirstSeq..LastSeq 的跨度 ≠ 行数），
//     而那正是「裁剪之前就少了行」的判据 —— 断号信号会误报成真缺行；
//   - 「seq 连续 ⇒ 没缺帧」这条推理整个塌掉。
//
// 缺陷本体：Brain 模式有**两个**命令日志计数器。
// `executeBrain` 自己开一个 `cmdSeq := 0`（executor.go 内 Brain 段），而 `ExecuteSession`
// 另有自己的 `cmdSeq := 0`；Brain 模式跑完后 `ExecuteSession` 还要用**自己那个**计数器写
// 收口帧（captureSessionReceipts / cleanupSessionTab）。两个计数器都从 0 起 ⇒ 收口帧
// 的 seq 与 Brain 帧的 seq 撞号。
//
// 修法：Brain 段接调用方那个计数器（`cmdSeq *int`），一个 session 只有一个计数器；
// 而不是让收口帧改用别的编号域（那会让审计包里出现两个互不相干的序号，
// 「按 seq 排序还原每一步」就废了）。
//
// 本腿走真实链路（WS e2e 夹具 + 真 Executor + 真 command_log 仓储），
// 断的是**整段 session 的 seq 唯一且连续**这一件事。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
)

// seqBrainPlanner 假 Brain：首轮就给一个空计划并宣告 done，JudgeDone 放行。
// 目标是让 executeBrain 至少落一帧（judge_done）再返回。
type seqBrainPlanner struct{}

func (p *seqBrainPlanner) GeneratePlanReflect(context.Context, uint, uint, string, string, string, *reflectState) ([]byte, bool, error) {
	return []byte(`[]`), true, nil
}
func (p *seqBrainPlanner) LastPlanTokens() int   { return 0 }
func (p *seqBrainPlanner) LastPlanModel() string { return "fake" }
func (p *seqBrainPlanner) LastAuxTokens() int    { return 0 }
func (p *seqBrainPlanner) JudgeDone(context.Context, uint, uint, string, string) (bool, string) {
	return true, ""
}
func (p *seqBrainPlanner) SummarizeSession(context.Context, uint, uint, string, int, int, string) string {
	return ""
}

// seqRows 按 seq 升序回读一个 session 的全部命令日志帧。
func seqRows(t *testing.T, bundle *wsE2EDeps, sessionID uint) []model.BrowserCommandLog {
	t.Helper()
	var rows []model.BrowserCommandLog
	if err := bundle.db.WithContext(context.Background()).Model(&model.BrowserCommandLog{}).
		Where("session_id = ?", sessionID).Order("seq asc").Find(&rows).Error; err != nil {
		t.Fatalf("command_log 回读失败: %v", err)
	}
	return rows
}

// TestBrainModeCommandLogSeqIsUniqueAndContinuous Brain 模式下整个 session 的 seq
// 不得重复、且必须是 1..N 的连续序列。
//
// 修之前的形状：Brain 段落帧用自己那个计数器（从 0 起），收口段落帧用 ExecuteSession
// 那个计数器（也从 0 起）⇒ 两段各有一套编号，撞号。判据「seq 唯一」当场红。
//
// 修之后：Brain 段接调用方的计数器 ⇒ 整段 session 只有一个连续序号。
//
// 为什么必须走真实链路：撞号发生在 executeBrain 与 ExecuteSession 收口段**之间**，
// 只测 executeBrain 单个函数看不见对手方。WS e2e 夹具给的是真 Hand（能开 tab、
// 能收口），真 Executor，真 command_log 仓储。
func TestBrainModeCommandLogSeqIsUniqueAndContinuous(t *testing.T) {
	exec, _, bundle := newWSE2E(t, happyReply)
	// Brain 段落的 plan 来自假 planner（真 Brain 要调 LLM），其余（开 tab / snapshot /
	// 收口）走真链路。
	exec.brain = &seqBrainPlanner{}

	task, session := bundle.seedTask(t, `[]`, false)
	task.BrainMode = true
	task.BrainGoal = "g"

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	exec.ExecuteSession(ctx, task, session, nil)

	rows := seqRows(t, bundle, session.ID)
	if len(rows) == 0 {
		t.Fatal("Brain 模式没落下任何命令日志帧：前提不成立，本腿无判据")
	}
	seen := map[int]bool{}
	for _, r := range rows {
		if seen[r.Seq] {
			t.Errorf("同一 session 内 seq=%d 出现两次（全部 seq：%v）："+
				"两个计数器各从 0 起 ⇒ 撞号后按 seq 排序无法还原先后，"+
				"裁剪摘要的 [FirstSeq,LastSeq] 区间也会与行数对不上（断号信号误报成真缺行）",
				r.Seq, seqList(rows))
		}
		seen[r.Seq] = true
	}
	// 连续性：seq 必须是 1..N，无洞。
	for i, r := range rows {
		if r.Seq != i+1 {
			t.Errorf("第 %d 帧 seq=%d（应=%d，全部 seq：%v）："+
				"一个 session 的命令流必须是一条连续序号，中间有洞就说明有帧没落库或落重了",
				i+1, r.Seq, i+1, seqList(rows))
		}
	}
}

// seqList 把帧列表压成 seq 序列，便于失败信息里直接读。
func seqList(rows []model.BrowserCommandLog) []int {
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Seq)
	}
	return out
}
