package service

// 契约：触发器的键位（(tenant_id, task_id) 上的唯一约束）必须随删除一起释放。
//
// 「一条 cron 任务只有一个触发器」这条前置是库里的唯一约束在守（uk_browser_cron_task /
// 模型 tag 的 uniqueIndex），而删除走的是软删——行还在，键位就还占着。于是用户最常做的那步
// 「删掉重建一个」（改时间走这条路比 PUT 改表达式省事）在服务端必然撞唯一约束：
// service 的重名预检查 GetByTaskID 自带 deleted_at IS NULL，读不到那行 ⇒ 放行；
// 紧接着 INSERT 撞约束 ⇒ 裸 error ⇒ 控制器回 500 INTERNAL_ERROR_6002。
// 任务的 cron→非 cron→cron 是同一条路（RemoveByTask 也是软删）。
//
// 判据打在库里，不打错误文案：删完的物化行数必须归零（键位真空出来），
// 重建成功且重新排程（条目在、next_run_at 有值）——只验「不报错」会放过
// 「行删了但条目还挂着」这种半吊子收口。

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/browser_automation/model"
	barepo "hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	keySlotOwner = uint(782401)
	// 每年 1 月 1 日 / 2 月 2 日：测试期间不会被真唤起
	keySlotExprA = "0 0 1 1 *"
	keySlotExprB = "0 0 2 2 *"
)

type keySlotEnv struct {
	taskSvc *TaskService
	cronSvc *CronService
	cron    barepo.BrowserCronTriggerRepository
	db      *gorm.DB
	ctx     context.Context
}

func newKeySlotEnv(t *testing.T) *keySlotEnv {
	t.Helper()
	db := testutil.NewTestDB(t, &model.BrowserTask{}, &model.BrowserSession{}, &model.BrowserCronTrigger{})
	if db == nil {
		t.Fatal("测试库不可达：唯一键位是否释放无法判定（不 Skip，跳过等于没锁）")
	}
	cronRepo := barepo.NewBrowserCronTriggerRepositoryWithDB(db)
	taskRepo := barepo.NewBrowserTaskRepositoryWithDB(db)
	cronSvc := NewCronService(cronRepo, taskRepo, nil)
	taskSvc := NewTaskService(taskRepo, barepo.NewBrowserSessionRepositoryWithDB(db), nil)
	taskSvc.SetTriggerRemover(cronSvc)
	return &keySlotEnv{taskSvc: taskSvc, cronSvc: cronSvc, cron: cronRepo, db: db, ctx: context.Background()}
}

func (e *keySlotEnv) seedTask(t *testing.T, name string) *model.BrowserTask {
	t.Helper()
	task := &model.BrowserTask{Name: name, TaskType: "cron", Status: "ready",
		Url: "https://example.com", UserID: keySlotOwner, Platform: "xiaohongshu",
		TimeoutSec: 120, Steps: []byte(`[{"action":"open_tab"}]`)}
	if err := barepo.NewBrowserTaskRepositoryWithDB(e.db).Create(e.ctx, task); err != nil {
		t.Fatalf("种子任务落库失败：%v", err)
	}
	t.Cleanup(func() {
		cleanCtx := context.Background()
		_ = e.db.WithContext(cleanCtx).Unscoped().Delete(&model.BrowserCronTrigger{}, "task_id = ?", task.ID)
		_ = e.db.WithContext(cleanCtx).Unscoped().Delete(&model.BrowserTask{}, task.ID)
	})
	return task
}

// physicalRows 物理行数（含软删）——键位占没占就看这个。
func (e *keySlotEnv) physicalRows(t *testing.T, taskID uint) int64 {
	t.Helper()
	var n int64
	if err := e.db.WithContext(e.ctx).Unscoped().Model(&model.BrowserCronTrigger{}).
		Where("task_id = ?", taskID).Count(&n).Error; err != nil {
		t.Fatalf("数触发器行失败：%v", err)
	}
	return n
}

// liveRow 走被测读侧（带 deleted_at IS NULL），读不到就是没有活行。
func (e *keySlotEnv) liveRow(t *testing.T, taskID uint) *model.BrowserCronTrigger {
	t.Helper()
	tr, err := e.cron.GetByTaskID(e.ctx, taskID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		t.Fatalf("读触发器失败：%v", err)
	}
	return tr
}

func TestCronDeleteReleasesTaskKeyForRecreate(t *testing.T) {
	e := newKeySlotEnv(t)
	task := e.seedTask(t, "keyslot-recreate")

	if _, err := e.cronSvc.Create(e.ctx, keySlotOwner, task.ID, keySlotExprA, "Asia/Shanghai", true); err != nil {
		t.Fatalf("首次创建触发器失败：%v", err)
	}
	tr := e.liveRow(t, task.ID)
	if tr == nil {
		t.Fatal("先决条件坏了：读不到刚建的触发器")
	}
	if err := e.cronSvc.Delete(e.ctx, tr.ID, keySlotOwner); err != nil {
		t.Fatalf("删除触发器失败：%v", err)
	}

	// 重建：删掉再挂上，是用户改时间的常规走法，必须成功
	if _, err := e.cronSvc.Create(e.ctx, keySlotOwner, task.ID, keySlotExprB, "Asia/Shanghai", true); err != nil {
		t.Errorf("删掉触发器后重建失败（err=%v）：软删的行仍占着 (tenant_id, task_id) 唯一键位，"+
			"重名预检查读不到它于是放行，INSERT 撞约束——用户只会看到 500", err)
	}
	if got := e.physicalRows(t, task.ID); got != 1 {
		t.Errorf("重建后该任务名下物理触发器行数 = %d，want 1：删除必须把键位腾出来，不能留软删行占位", got)
	}
	if !entryRegistered(e.cronSvc, task.ID) {
		t.Errorf("重建成功却没在 TaskManager 上留排程条目：这条 cron 再也不会响")
	}
	after := e.liveRow(t, task.ID)
	if after == nil {
		t.Fatal("重建后读不到触发器")
	}
	if after.CronExpr != keySlotExprB {
		t.Errorf("重建后表达式 = %q，want %q", after.CronExpr, keySlotExprB)
	}
	if after.NextRunAt == nil {
		t.Errorf("重建后 next_run_at = NULL：启用中的触发器必须带着下一次触发时刻")
	}
}

// 任务侧回收（cron→非 cron→cron）走的是 RemoveByTask，同一把唯一键位，同样必须腾出来。
func TestTaskTypeAwayAndBackRecreatesTrigger(t *testing.T) {
	e := newKeySlotEnv(t)
	task := e.seedTask(t, "keyslot-type-flip")

	if _, err := e.cronSvc.Create(e.ctx, keySlotOwner, task.ID, keySlotExprA, "Asia/Shanghai", true); err != nil {
		t.Fatalf("首次创建触发器失败：%v", err)
	}
	if _, err := e.taskSvc.Update(e.ctx, task.ID, keySlotOwner, func(t *model.BrowserTask) error {
		t.TaskType = "one_shot"
		return nil
	}); err != nil {
		t.Fatalf("改成 one_shot 失败：%v", err)
	}
	if e.liveRow(t, task.ID) != nil {
		t.Fatal("先决条件坏了：改走非 cron 后仍有活触发器")
	}
	if _, err := e.taskSvc.Update(e.ctx, task.ID, keySlotOwner, func(t *model.BrowserTask) error {
		t.TaskType = "cron"
		return nil
	}); err != nil {
		t.Fatalf("改回 cron 失败：%v", err)
	}
	if _, err := e.cronSvc.Create(e.ctx, keySlotOwner, task.ID, keySlotExprB, "Asia/Shanghai", true); err != nil {
		t.Errorf("任务改走再改回后重建触发器失败（err=%v）：回收留下的软删行占着唯一键位", err)
	}
	if got := e.physicalRows(t, task.ID); got != 1 {
		t.Errorf("重建后物理行数 = %d，want 1", got)
	}
	if row := e.liveRow(t, task.ID); row == nil || row.DeletedAt.Valid {
		t.Errorf("库里留下的不是那条新建的活行（row=%+v）", row)
	}
}
