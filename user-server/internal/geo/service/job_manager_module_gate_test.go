package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"hivemtk-user/internal/geo/model"
	"hivemtk-user/internal/geo/repository"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// FR-5.6 接线冒烟：SetupGeoJobs 注册的**每个** cron cmd 在派发时都要先问
// 模块闸（jobModuleGate），闸说 false 就直接返回 —— 不落运行记录、不
// StartJob。这里用桩闸 + 桩调度器把这条链路钉住。

// cmdCapturingScheduler 按注册顺序捕获 cmd（AddTask 收不到任务名，
// 只能按 SetupGeoJobs 的注册顺序取）。
type cmdCapturingScheduler struct {
	mu   sync.Mutex
	cmds []func()
}

func (s *cmdCapturingScheduler) AddTask(spec string, cmd func()) (cron.EntryID, error) {
	if _, err := specParser.Parse(spec); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cmds = append(s.cmds, cmd)
	return cron.EntryID(len(s.cmds)), nil
}

func (s *cmdCapturingScheduler) RemoveTask(id cron.EntryID) {}

// countingRunRepo 统计 Create 调用：闸拦住时 StartJob 不会被调到，
// Create 自然为 0 —— 这是"闸真的挡住了派发"的可观测判据。
type countingRunRepo struct {
	repository.GeoJobRunRepository
	creates int
}

func (c *countingRunRepo) Create(ctx context.Context, run *model.GeoJobRun) error {
	c.creates++
	return c.GeoJobRunRepository.Create(ctx, run)
}

// TestSetupGeoJobsCmdConsultsModuleGate SetupGeoJobs 注册的任务 cmd 必须
// 每次派发都过闸：闸返回 false 时该任务不启动、不写运行记录。
func TestSetupGeoJobsCmdConsultsModuleGate(t *testing.T) {
	database := testutil.NewTestDB(t, &model.GeoJobRun{}, &model.GeoConfig{})
	db.SetTestDB(database)

	m := GetGeoJobManager()
	m.mu.Lock()
	oldRun, oldCfg := m.runRepo, m.cfgRepo
	oldSched, oldSetup, oldEntry := m.sched, m.setup, m.entryID
	counter := &countingRunRepo{GeoJobRunRepository: repository.NewGeoJobRunRepositoryWithDB(database)}
	m.runRepo = counter
	m.cfgRepo = repository.NewGeoConfigRepositoryWithDB(database)
	m.entryID = map[jobKey]cron.EntryID{}
	m.mu.Unlock()

	oldGate := jobModuleGate
	t.Cleanup(func() {
		jobModuleGate = oldGate
		m.mu.Lock()
		m.runRepo, m.cfgRepo = oldRun, oldCfg
		m.sched, m.setup, m.entryID = oldSched, oldSetup, oldEntry
		m.mu.Unlock()
	})

	// 桩闸：记录收到的 worker 名，恒判关
	var checked []string
	SetGeoJobModuleGate(func(_ context.Context, workerName string) bool {
		checked = append(checked, workerName)
		return false
	})

	sched := &cmdCapturingScheduler{}
	SetupGeoJobs(sched)

	sched.mu.Lock()
	n := len(sched.cmds)
	sched.mu.Unlock()
	if n != len(geoJobDefs) {
		t.Fatalf("SetupGeoJobs 应注册 %d 个任务 cmd，实际 %d", len(geoJobDefs), n)
	}

	// 按注册顺序取第一个（geoJobDefs[0] = sov_refresh）并派发一次
	sched.mu.Lock()
	cmd := sched.cmds[0]
	sched.mu.Unlock()
	cmd()

	if len(checked) != 1 || checked[0] != JobSOVRefresh {
		t.Fatalf("任务 cmd 应以 worker 名 %q 询问模块闸一次，实际 %v", JobSOVRefresh, checked)
	}
	if counter.creates != 0 {
		t.Errorf("闸判关时不得启动任务（不应写运行记录），实际 Create 调用 %d 次", counter.creates)
	}
	if m.runningState(jobKey{name: JobSOVRefresh, tenant: tenant.DefaultTenantID}) {
		t.Error("闸判关时任务不得被标记为 running")
	}
	// 闸判关的整段停顿时长（一轮误跑的代价）：断言 cmd 是同步快速返回
	done := make(chan struct{})
	go func() { cmd(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("闸判关时 cmd 应立即返回（不得挂起）")
	}
}
