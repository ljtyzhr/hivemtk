package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"github.com/robfig/cron/v3"
)

type fakeScheduler struct {
	specs  map[string]string
	nextID cron.EntryID
}

func newFakeScheduler() *fakeScheduler {
	return &fakeScheduler{specs: map[string]string{}}
}

func (f *fakeScheduler) AddTask(spec string, cmd func()) (cron.EntryID, error) {
	if _, err := specParser.Parse(spec); err != nil {
		return 0, err
	}
	f.nextID++
	f.specs[spec] = "ok"
	return f.nextID, nil
}

func (f *fakeScheduler) RemoveTask(id cron.EntryID) {}

func newTestJobManager(t *testing.T) (*JobManager, *fakeScheduler) {
	t.Helper()
	// ⚠️ 必须同时迁移 geo_keywords 并注入全局 DB：
	// geoJobDefs 里的任务函数（如 sovRefreshJob）**不是**从 JobManager 取仓库，
	// 而是自己调 repository.NewGeoKeywordRepository()，该构造函数读全局 _db.GetDB()。
	// 只注入 runRepo/cfgRepo 而不设置全局 DB 时，任务函数拿到 nil *gorm.DB，
	// 触发空指针 panic，被 recover 吞掉后记成 status=failed（测试期望 success）。
	// 详见 TASKS_AUDIT_2026-09-16.md · GEO-01。
	database := testutil.NewTestDB(t, &model.GeoJobRun{}, &model.GeoConfig{}, &model.GeoKeyword{})
	db.SetTestDB(database)
	m := &JobManager{
		runRepo: repository.NewGeoJobRunRepositoryWithDB(database),
		cfgRepo: repository.NewGeoConfigRepositoryWithDB(database),
		running: map[jobKey]*atomic.Bool{},
		specs:   map[jobKey]string{},
		entryID: map[jobKey]cron.EntryID{},
	}
	for _, d := range geoJobDefs {
		m.specs[jobKey{name: d.Name, tenant: tenant.DefaultTenantID}] = d.DefaultSpec
	}
	sched := newFakeScheduler()
	return m, sched
}

// TestJobManager_RunRecordsHistory 成功执行的任务应留下 success 运行记录
func TestJobManager_RunRecordsHistory(t *testing.T) {
	m, _ := newTestJobManager(t)

	// off 模式：StartJob 经 Fanout 退化为单默认租户，互斥位键为 (任务, 默认租户)
	defaultKey := jobKey{name: JobSOVRefresh, tenant: tenant.DefaultTenantID}
	m.runningFor(defaultKey).Store(true)
	started, err := m.StartJob(JobSOVRefresh, "manual")
	if err != nil {
		t.Fatalf("StartJob failed: %v", err)
	}
	if started {
		t.Fatal("任务运行中应跳过新触发")
	}
	m.runningFor(defaultKey).Store(false)

	started, err = m.StartJob(JobSOVRefresh, "manual")
	if err != nil || !started {
		t.Fatalf("StartJob started=%v err=%v", started, err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for m.runningFor(defaultKey).Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.runningFor(defaultKey).Load() {
		t.Fatal("任务应已执行完成")
	}

	runs, total, err := m.runRepo.List(context.Background(), JobSOVRefresh, 1, 10)
	if err != nil {
		t.Fatalf("List runs failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("期望 1 条运行记录, got %d", total)
	}
	if runs[0].Status != "success" {
		t.Fatalf("期望 status=success, got %s (err=%s)", runs[0].Status, runs[0].Error)
	}
}

// TestJobManager_UnknownJob 未知任务应报错
func TestJobManager_UnknownJob(t *testing.T) {
	m, _ := newTestJobManager(t)
	if _, err := m.StartJob("not_exist", "manual"); err == nil {
		t.Fatal("未知任务应返回错误")
	}
}

// TestJobManager_UpdateSchedule 合法表达式更新成功并持久化，非法表达式被拒绝
func TestJobManager_UpdateSchedule(t *testing.T) {
	m, sched := newTestJobManager(t)

	if err := m.UpdateSchedule(context.Background(), JobSOVRefresh, "0 30 2 * * *"); err == nil {
		t.Fatal("调度器未初始化时应报错")
	}

	m.setup = true
	m.sched = sched
	if err := m.UpdateSchedule(context.Background(), JobSOVRefresh, "0 30 2 * * *"); err != nil {
		t.Fatalf("UpdateSchedule failed: %v", err)
	}
	// off 模式（Background 无租户绑定）：调度落在默认租户的 (任务, 租户) 键上
	if got := m.specs[jobKey{name: JobSOVRefresh, tenant: tenant.DefaultTenantID}]; got != "0 30 2 * * *" {
		t.Fatalf("spec 未生效: %s", got)
	}

	cfg, err := m.cfgRepo.Get(context.Background())
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	if cfg.CronSpecs == "" || !containsStr(cfg.CronSpecs, "0 30 2 * * *") {
		t.Fatalf("CronSpecs 未持久化: %q", cfg.CronSpecs)
	}

	if err := m.UpdateSchedule(context.Background(), JobSOVRefresh, "invalid spec"); err == nil {
		t.Fatal("非法 cron 表达式应报错")
	}

	if err := m.UpdateSchedule(context.Background(), "not_exist", "0 30 2 * * *"); err == nil {
		t.Fatal("未知任务应报错")
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
