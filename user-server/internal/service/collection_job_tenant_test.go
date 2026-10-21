// collection_job_tenant_test.go 催收任务的租户扇出契约（T-P7-03 / 实体审查 #44）。
//
// 改前 loop 拿装配点的 context.Background() 直接跑 RunOnce：bills 是租户表，
// on 模式下 ScanOverdue 每轮都撞「需要租户上下文」——单机（off）全绿，SaaS 里
// 催收整条腿静默空转。这条用例钉住「一轮必须逐租户执行、每个租户拿到自己的 ctx」。
package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/repository"
)

type tenantRecordingScanner struct {
	mu   sync.Mutex
	seen []uint
}

func (s *tenantRecordingScanner) ScanOverdue(ctx context.Context, _ time.Time, _ int) (*repository.BillOverdueScan, error) {
	id, _ := tenant.FromContext(ctx)
	s.mu.Lock()
	s.seen = append(s.seen, id)
	s.mu.Unlock()
	return &repository.BillOverdueScan{}, nil
}

func TestCollectionJobRunRoundFansOutPerTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	tenant.SetActiveTenantLoader(func(context.Context) ([]uint, error) { return []uint{301, 302}, nil })
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	scanner := &tenantRecordingScanner{}
	job := NewCollectionJob(
		scanner,
		&fakeCollectionOpps{rows: map[string]*model.Opportunity{}},
		&fakeCollectionReach{},
		&fakeCollectionTasks{created: true},
		&fakeCollectionStage{on: true},
	)
	job.mode = RecoveryWorkerModeShadow
	job.batch = collectionJobDefaultBatch
	job.interval = 6 * time.Hour
	job.remindWindow = CollectionReminderWindow
	job.escalateWindow = CollectionEscalateWindow
	job.nowFunc = collFixtureNow

	if err := job.runRound(context.Background()); err != nil {
		t.Fatalf("逐租户扇出失败: %v", err)
	}
	if len(scanner.seen) != 2 {
		t.Fatalf("扫描次数=%d, want 2（每个活跃租户各一轮）；seen=%v", len(scanner.seen), scanner.seen)
	}
	if scanner.seen[0] != 301 || scanner.seen[1] != 302 {
		t.Errorf("扫描到的租户=%v, want [301 302]（不得退化成裸 ctx 或单租户）", scanner.seen)
	}
}
