package service

// 契约：显式传 enabled:false 建出来的触发器，必须**真的**是停用状态。
//
// 坑：BrowserCronTrigger.Enabled 声明了 gorm `default:true`。GORM 对带 default 标签的字段，
// 零值（false）会被当成"调用方没赋值"从 INSERT 里剔除，交给列默认值兜底 —— 而列默认值是 true。
// 于是 POST /api/browser-automation/cron 带 enabled:false：
//   - 落库 enabled=true；
//   - service.applySchedule 读到的还是内存里的 true，照样把这条触发器注册进 TaskManager。
// 用户明确说"先别跑"的定时任务，会在下一个 09:00 准时开火。
//
// 本文件把两件事钉死：
//  1. 落库值：读回来必须是 false（不能是列默认值 true）。
//  2. 调度副作用：停用的触发器不得被注册进 TaskManager（next_run_at 必须为 NULL）。
//
// 反向腿：enabled 缺省（请求里不传）仍然必须是 true —— 缺省语义归 service 层管，
// 不能因为修了显式 false 就把"不传 = 默认开"也一起改掉。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	barepo "hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	cronEnabledOwner = uint(781431)
	// 每年 2 月 2 日：本次测试内不会触发的挂钟排程。
	// 真触发就是 nil taskSvc 解引用当场 panic，比任何断言都响。
	cronEnabledExpr = "0 0 2 2 *"
	cronEnabledTZ   = "Asia/Shanghai"
)

func newCronEnabledSvc(t *testing.T) (*CronService, *gorm.DB, context.Context) {
	t.Helper()
	db := testutil.NewTestDB(t, &model.BrowserTask{}, &model.BrowserSession{}, &model.BrowserCronTrigger{})
	if db == nil {
		t.Fatal("测试库不可达：本文件的落库结论无法判定（不 Skip，跳过等于没锁）")
	}
	svc := NewCronService(
		barepo.NewBrowserCronTriggerRepositoryWithDB(db),
		barepo.NewBrowserTaskRepositoryWithDB(db), nil,
	)
	return svc, db, context.Background()
}

// seedCronEnabledTask 建一条 cron 类型任务（Create 要先过 task_type 闸门）。
func seedCronEnabledTask(t *testing.T, db *gorm.DB, ctx context.Context, name string) *model.BrowserTask {
	t.Helper()
	task := &model.BrowserTask{Name: name, TaskType: "cron", Status: "ready",
		Url: "https://example.com", UserID: cronEnabledOwner, Platform: "xiaohongshu",
		TimeoutSec: 120, Steps: []byte(`[{"action":"open_tab"}]`)}
	if err := barepo.NewBrowserTaskRepositoryWithDB(db).Create(ctx, task); err != nil {
		t.Fatalf("种子任务落库失败：%v", err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(context.Background()).Delete(&model.BrowserCronTrigger{}, "task_id = ?", task.ID)
		_ = db.WithContext(context.Background()).Delete(&model.BrowserTask{}, task.ID)
	})
	return task
}

// readCronEnabledTrigger 用独立零值 struct 读回（复用已填充 struct 会把旧字段并进 WHERE）。
func readCronEnabledTrigger(t *testing.T, db *gorm.DB, ctx context.Context, id uint) *model.BrowserCronTrigger {
	t.Helper()
	var got model.BrowserCronTrigger
	if err := db.WithContext(ctx).Where("id = ?", id).First(&got).Error; err != nil {
		t.Fatalf("读回触发器 %d 失败：%v", id, err)
	}
	return &got
}

// TestCronCreateWithEnabledFalseStaysDisabled 显式 enabled:false ⇒ 落库 false、且不排程。
func TestCronCreateWithEnabledFalseStaysDisabled(t *testing.T) {
	svc, db, ctx := newCronEnabledSvc(t)
	task := seedCronEnabledTask(t, db, ctx, "cron-enabled-false")

	tr, err := svc.Create(ctx, cronEnabledOwner, task.ID, cronEnabledExpr, cronEnabledTZ, false)
	if err != nil {
		t.Fatalf("Create(enabled=false) 失败：%v", err)
	}
	t.Cleanup(func() {
		_ = svc.SetEnabled(context.Background(), tr.ID, cronEnabledOwner, false)
		_ = db.WithContext(context.Background()).Delete(&model.BrowserCronTrigger{}, tr.ID)
	})

	if tr.Enabled {
		t.Errorf("返回体里 enabled 应为 false，实际 true（内存态就被默认值污染了）")
	}

	got := readCronEnabledTrigger(t, db, ctx, tr.ID)
	if got.Enabled {
		t.Errorf("落库 enabled 应为 false，实际 true —— 显式停用被 GORM 的 default:true 标签吞掉了，" +
			"这条触发器会在用户明确要求停用的情况下照常开火")
	}
	if got.NextRunAt != nil {
		t.Errorf("停用状态不得排程，next_run_at 应为 NULL，实际 %v", got.NextRunAt)
	}
}

// TestCronCreateWithoutEnabledDefaultsToOn enabled 缺省仍必须是 true。
//
// 这是修「显式 false 被吞」时最容易顺手改坏的一条：缺省语义归 service 层
// （CreateCronReq.Enabled 为 nil ⇒ 默认开），模型层不得越权改这个默认值。
func TestCronCreateWithoutEnabledDefaultsToOn(t *testing.T) {
	svc, db, ctx := newCronEnabledSvc(t)
	task := seedCronEnabledTask(t, db, ctx, "cron-enabled-default")

	tr, err := svc.Create(ctx, cronEnabledOwner, task.ID, cronEnabledExpr, cronEnabledTZ, true)
	if err != nil {
		t.Fatalf("Create(enabled=true) 失败：%v", err)
	}
	t.Cleanup(func() {
		_ = svc.SetEnabled(context.Background(), tr.ID, cronEnabledOwner, false)
		_ = db.WithContext(context.Background()).Delete(&model.BrowserCronTrigger{}, tr.ID)
	})

	got := readCronEnabledTrigger(t, db, ctx, tr.ID)
	if !got.Enabled {
		t.Errorf("落库 enabled 应为 true，实际 false")
	}
	if got.NextRunAt == nil {
		t.Errorf("启用状态必须排程，next_run_at 不应为 NULL")
	} else if !got.NextRunAt.After(time.Now()) {
		t.Errorf("next_run_at 必须是将来的时刻，实际 %v", got.NextRunAt)
	}
}
