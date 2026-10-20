package service

// 契约锁（实体审查 #3）：治理裁剪逐活跃租户执行，不用裸 ctx 直连租户表。
//
// 调用链的入口是 router/browser_automation_routes.go 的 bootstrap goroutine，
// ctx = context.Background()。saas 形态下把这样的 ctx 直接交给 PruneBefore，
// 隔离 callback fail-closed ⇒ 每小时一条 Warn、一行都不裁 —— 表现不是报错刷屏，
// 而是「90 天保留期在云上根本没生效」，command_log 无界增长（G19 立项要防的那件事）。
//
// 与 r23 那两条腿分开：r23 钉「界与来源的接线」，本文件钉「租户的接线」。
// 两者在库里的失效形状不同，混在一条断言里会互相掩盖。

import (
	"context"
	"sort"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/tenant"
)

type fanoutCmdRepo struct {
	repository.BrowserCommandLogRepository // 只用得到 PruneBefore，误用其余方法当场 panic 可见
	tenants                                []uint
	bareCalls                              int
}

func (r *fanoutCmdRepo) PruneBefore(ctx context.Context, _ time.Time, _ string) (int64, error) {
	tid, ok := tenant.FromContext(ctx)
	if !ok {
		r.bareCalls++
		return 0, nil
	}
	r.tenants = append(r.tenants, tid)
	return 1, nil
}

type fanoutPlanRepo struct {
	repository.BrowserLLMPlanRepository
	tenants   []uint
	bareCalls int
}

func (r *fanoutPlanRepo) PruneSnapshotText(ctx context.Context, _ time.Time) (int64, error) {
	tid, ok := tenant.FromContext(ctx)
	if !ok {
		r.bareCalls++
		return 0, nil
	}
	r.tenants = append(r.tenants, tid)
	return 1, nil
}

func seedOnModeTenants(t *testing.T, ids []uint) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	tenant.SetActiveTenantLoader(func(context.Context) ([]uint, error) { return ids, nil })
	t.Cleanup(func() {
		// activeTenantLoader 是进程级 atomic.Value，用例间必须复位（见 pkg/tenant/fanout.go）。
		tenant.SetActiveTenantLoader(nil)
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
}

func tenantsOf(ids []uint) []uint {
	out := append([]uint(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TestRetentionPruneFansOutPerTenant on 模式一轮扫描 = 每个活跃租户各裁一次，
// 且每次拿到的 ctx 都绑着那个租户（裸 ctx 到仓储就是停摆或跨租户，两个方向都不许出现）。
func TestRetentionPruneFansOutPerTenant(t *testing.T) {
	t.Setenv(auditRetentionEnv, "30")
	seedOnModeTenants(t, []uint{11, 32})

	cmd, plan := &fanoutCmdRepo{}, &fanoutPlanRepo{}
	pruneAuditOnce(context.Background(), cmd, plan)

	if cmd.bareCalls != 0 || plan.bareCalls != 0 {
		t.Errorf("裸 ctx 到达仓储 got=(cmd=%d,plan=%d) want=(0,0)（少一层扇出＝裁剪在 saas 下永不生效）",
			cmd.bareCalls, plan.bareCalls)
	}
	if got := tenantsOf(cmd.tenants); len(got) != 2 || got[0] != 11 || got[1] != 32 {
		t.Errorf("command_log 被裁的租户 got=%v want=[11 32]", got)
	}
	if got := tenantsOf(plan.tenants); len(got) != 2 || got[0] != 11 || got[1] != 32 {
		t.Errorf("llm_plans 被裁的租户 got=%v want=[11 32]", got)
	}
}

// TestRetentionPruneOffModeRunsOnce 独立部署形态退化为单默认租户：一轮扫描各表一次，
// 且 ctx 已绑默认租户（与修复前的裸 ctx 行为等价，但不靠「不隔离」兜底）。
func TestRetentionPruneOffModeRunsOnce(t *testing.T) {
	t.Setenv(auditRetentionEnv, "30")
	tenant.SetMode(tenant.ModeOff)

	cmd, plan := &fanoutCmdRepo{}, &fanoutPlanRepo{}
	pruneAuditOnce(context.Background(), cmd, plan)

	if len(cmd.tenants) != 1 || cmd.tenants[0] != tenant.DefaultTenantID {
		t.Errorf("off 模式 command_log 的租户 got=%v want=[%d]", cmd.tenants, tenant.DefaultTenantID)
	}
	if len(plan.tenants) != 1 || plan.tenants[0] != tenant.DefaultTenantID {
		t.Errorf("off 模式 llm_plans 的租户 got=%v want=[%d]", plan.tenants, tenant.DefaultTenantID)
	}
	if cmd.bareCalls != 0 || plan.bareCalls != 0 {
		t.Errorf("off 模式仍有裸 ctx 到达仓储 got=(cmd=%d,plan=%d) want=(0,0)", cmd.bareCalls, plan.bareCalls)
	}
}

// TestRetentionPruneOneFailureDoesNotSkipOther 两次扇出各自独立：command_log 报错时
// llm_plans 那半边照样跑完（并成一个回调会让一处故障带走整个治理任务）。
func TestRetentionPruneOneFailureDoesNotSkipOther(t *testing.T) {
	t.Setenv(auditRetentionEnv, "30")
	seedOnModeTenants(t, []uint{11})

	cmd := &failingCmdRepo{}
	plan := &fanoutPlanRepo{}
	pruneAuditOnce(context.Background(), cmd, plan)

	if cmd.calls != 1 {
		t.Fatalf("command_log 调用次数 got=%d want=1", cmd.calls)
	}
	if len(plan.tenants) != 1 || plan.tenants[0] != 11 {
		t.Errorf("llm_plans 未执行 got=%v want=[11]（一处失败不该带走另一处）", plan.tenants)
	}
}

type failingCmdRepo struct {
	repository.BrowserCommandLogRepository
	calls int
}

func (r *failingCmdRepo) PruneBefore(context.Context, time.Time, string) (int64, error) {
	r.calls++
	return 0, context.DeadlineExceeded
}
