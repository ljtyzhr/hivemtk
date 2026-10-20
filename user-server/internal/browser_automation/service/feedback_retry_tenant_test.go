package service

// 契约锁（实体审查 #4 · 4.3）：到期重试扫描逐活跃租户执行，不用裸 ctx 扫租户表。
//
// 生产入口是 router/browser_automation_routes.go 的 bootstrap goroutine
// （`context.Background()` → `StartRetryScanner`）。`browser_tasks` 是租户表，
// on 模式下裸 ctx 被隔离 callback fail-closed ⇒ `scanDueRetries` 每分钟只留下一行
// 「重试扫描失败」，自动重试整条功能在 saas 形态静默停摆。
//
// 为什么这条腿值得单独钉：RunTask 下游要写 browser_sessions / browser_command_log，
// 那些行的 tenant_id 由隔离 callback 从 ctx 取 —— ctx 没租户时不是「写到默认租户」
// 而是整条链路当场失败；而 Fanout 少一层的话，重试事实会挂错租户。两处都得断言。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

type retryCall struct {
	taskID uint
	tenant uint
}

func seedDueRetryTask(t *testing.T, db *gorm.DB, tid uint, name string, userID uint) uint {
	t.Helper()
	ctx := tenant.WithTenantID(context.Background(), tid)
	at := time.Now().Add(-time.Minute)
	task := &model.BrowserTask{
		Name: name, Url: "https://example.com", UserID: userID,
		Status: "failed", RetryOnFail: true, MaxRetryTimes: 3, RetryCount: 1, NextRetryAt: &at,
	}
	if err := db.WithContext(ctx).Create(task).Error; err != nil {
		t.Fatalf("租户 %d 种到期重试任务失败: %v", tid, err)
	}
	return task.ID
}

func TestRetryScannerFansOutPerTenant(t *testing.T) {
	seedOnModeTenants(t, []uint{11, 32})
	db := testutil.NewTestDB(t, &model.BrowserTask{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	taskA := seedDueRetryTask(t, db, 11, "retry-tenant-a", 101)
	taskB := seedDueRetryTask(t, db, 32, "retry-tenant-b", 202)

	calls := make(chan retryCall, 8)
	SetRetryRunner(func(ctx context.Context, taskID, _ uint, _ int) error {
		tid, ok := tenant.FromContext(ctx)
		if !ok {
			tid = 0
		}
		calls <- retryCall{taskID: taskID, tenant: tid}
		return nil
	})
	defer SetRetryRunner(nil)

	f := NewFeedbackService(nil, repository.NewBrowserTaskRepositoryWithDB(db))
	f.scanDueRetries(context.Background())

	seen := map[uint]uint{}
	close(calls)
	for c := range calls {
		if c.tenant == 0 {
			t.Errorf("runner 收到裸 ctx（task=%d）：重试写下的 session/命令日志将无处取租户", c.taskID)
		}
		seen[c.taskID] = c.tenant
	}
	if len(seen) != 2 {
		t.Fatalf("认领到的到期重试 got=%v want 两个租户各一条", seen)
	}
	if seen[taskA] != 11 {
		t.Errorf("租户 11 的任务 %d 的执行 ctx 租户 got=%d want=11（挂错租户＝把 A 的重试记到 B 账上）", taskA, seen[taskA])
	}
	if seen[taskB] != 32 {
		t.Errorf("租户 32 的任务 %d 的执行 ctx 租户 got=%d want=32", taskB, seen[taskB])
	}

	// 认领必须真的把 next_retry_at 清空，否则下一分钟会再触发一次（双发）。
	for _, tid := range []uint{11, 32} {
		var n int64
		ctx := tenant.WithTenantID(context.Background(), tid)
		if err := db.WithContext(ctx).Model(&model.BrowserTask{}).
			Where("next_retry_at IS NOT NULL").Count(&n).Error; err != nil {
			t.Fatalf("回读 next_retry_at 失败: %v", err)
		}
		if n != 0 {
			t.Errorf("租户 %d 仍挂着到期重试 got=%d want=0", tid, n)
		}
	}
}

func TestRetryScannerOffModeRunsOnce(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	db := testutil.NewTestDB(t, &model.BrowserTask{})
	if db == nil {
		t.Fatal("测试库不可达")
	}
	taskID := seedDueRetryTask(t, db, tenant.DefaultTenantID, "retry-off-mode", 101)

	calls := make(chan retryCall, 4)
	SetRetryRunner(func(ctx context.Context, id, _ uint, _ int) error {
		tid, _ := tenant.FromContext(ctx)
		calls <- retryCall{taskID: id, tenant: tid}
		return nil
	})
	defer SetRetryRunner(nil)

	f := NewFeedbackService(nil, repository.NewBrowserTaskRepositoryWithDB(db))
	f.scanDueRetries(context.Background())
	close(calls)

	var got []retryCall
	for c := range calls {
		got = append(got, c)
	}
	if len(got) != 1 || got[0].taskID != taskID || got[0].tenant != tenant.DefaultTenantID {
		t.Errorf("off 模式重试认领 got=%+v want 一条 task=%d tenant=%d", got, taskID, tenant.DefaultTenantID)
	}
}
