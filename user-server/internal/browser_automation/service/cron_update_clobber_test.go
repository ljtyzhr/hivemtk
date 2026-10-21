package service

// 契约锁（实体审查 #49，BrowserCronTrigger）：改表达式这条入口不许把调度记账列覆回快照值。
//
// 缺陷本体：`CronService.Update` 的形状是「GetByID 读快照 → mutator 改 cron_expr/time_zone
// → repository.Update 写回」，而 repository.Update 用的是 `Save(t)` —— GORM 的 Save 写**全部**
// 列。于是 next_run_at / last_run_at 这两列也被快照里的旧值覆写。
//
// 这两列各有另一个写主：cron 触发回调（registerWithUser）每次醒来都经 UpdateTimes
// 写这一对时刻。于是「用户在改表达式的同一秒，调度器刚好触发了」这一窗口里，
// 改表达式会把刚推进的 last_run_at（本次触发时刻）覆回改之前那次触发的旧值——
// 列表页那一列于是显示「上一次触发」比实际早一次，而它恰恰是运维判断
// 「这个定时器到底还活着吗」的那一列。
//
// 与 browser_tasks 的同款修法同源（taskOwnedByExecutorCols）：写面收口到
// 「用户可编辑的那几列」，调度记账列由 UpdateTimes 独占。
//
// 本腿走真实仓储（真 GORM + 真表），断的是「改表达式后 last_run_at 仍是最近那次触发的值」。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	barepo "hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/testutil"
)

// TestUpdateExprDoesNotClobberLastRunAt 改表达式不许把 last_run_at 覆回快照值。
//
// 形状：先让调度器写一次 last_run_at（模拟一次触发），再走改表达式这条入口，
// 然后断言 last_run_at 仍是那次触发的值。
//
// 修之前：repository.Update 的 Save 把快照里的旧 last_run_at（nil 或更早的值）写回去。
// 修之后：写面收口到 cron_expr/time_zone，last_run_at 原样保留。
func TestUpdateExprDoesNotClobberLastRunAt(t *testing.T) {
	db := testutil.NewTestDB(t, &model.BrowserTask{}, &model.BrowserSession{}, &model.BrowserCronTrigger{})
	if db == nil {
		t.Fatal("测试库不可达：覆写结论无法判定（不 Skip，跳过等于没锁）")
	}
	ctx := context.Background()
	repo := barepo.NewBrowserCronTriggerRepositoryWithDB(db)

	// 种子：一条任务 + 一条触发器（enabled=false，避免注册真实排程条目）。
	task := &model.BrowserTask{Name: "cron-覆写", TaskType: "cron", Status: "ready",
		Url: "https://example.com", UserID: cronLifeOwner, Platform: "xiaohongshu",
		TimeoutSec: 120, Steps: []byte(`[{"action":"open_tab"}]`)}
	if err := barepo.NewBrowserTaskRepositoryWithDB(db).Create(ctx, task); err != nil {
		t.Fatalf("种子任务落库失败：%v", err)
	}
	tr := &model.BrowserCronTrigger{TaskID: task.ID, CronExpr: yearlyJanExpr, TimeZone: cronLifeTZ,
		Enabled: false}
	if err := repo.Create(ctx, tr); err != nil {
		t.Fatalf("种子触发器落库失败：%v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Delete(&model.BrowserCronTrigger{}, tr.ID)
		_ = db.WithContext(context.Background()).Delete(&model.BrowserTask{}, task.ID)
	})

	// 调度器的一次触发：UpdateTimes 写这一对时刻。
	oldFire := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	next := oldFire.Add(time.Hour)
	if err := repo.UpdateTimes(ctx, tr.ID, &next, &oldFire); err != nil {
		t.Fatalf("模拟一次触发（UpdateTimes）失败：%v", err)
	}

	// 改表达式这条入口：service.Update 内部是「读快照 → 改 → Save 写回」。
	// 竞态窗口在读快照与写回之间——调度器恰好在这中间醒来一次。
	// 本腿用**仓储层**复现这个窗口：先读出快照（模拟 service 的读），
	// 让调度器在这之后写一次新的 last_run_at（模拟触发），
	// 再用同一个快照调写回（模拟 service 的 Save）。
	//
	// 直接走 svc.Update 复现不了这个窗口：它把读与写封在一个函数里，
	// 测试没法在中间插一次触发。
	snapshot := readCronLifeTrigger(t, db, ctx, tr.ID)
	if snapshot.LastRunAt == nil || !snapshot.LastRunAt.UTC().Equal(oldFire) {
		t.Fatalf("先决条件坏了：快照里的 last_run_at = %v，want %v", snapshot.LastRunAt, oldFire)
	}

	// 调度器在窗口里醒来：推进 last_run_at 到「本次触发时刻」。
	fired := time.Now().UTC().Truncate(time.Second)
	next2 := fired.Add(time.Hour)
	if err := repo.UpdateTimes(ctx, tr.ID, &next2, &fired); err != nil {
		t.Fatalf("窗口内的触发（UpdateTimes）失败：%v", err)
	}

	// service 的写回：拿旧快照改掉表达式后写回。
	snapshot.CronExpr = yearlyFebExpr
	if err := repo.Update(ctx, snapshot); err != nil {
		t.Fatalf("写回失败：%v", err)
	}

	after := readCronLifeTrigger(t, db, ctx, tr.ID)
	if after.LastRunAt == nil {
		t.Fatal("改表达式后 last_run_at 成了 nil：上一次触发的时刻被抹掉了，" +
			"列表页那一列于是读成「从没触发过」")
	}
	if !after.LastRunAt.UTC().Equal(fired) {
		t.Errorf("改表达式把 last_run_at 覆成了 %v，want %v（最近那次触发的时刻）："+
			"写面没收口到用户可编辑列 ⇒ 快照里的旧值盖掉了调度器刚写的值",
			after.LastRunAt.UTC(), fired)
	}
}
