package repository

// 契约锁（实体审查 #6，TENANT_MODE=on）：LLM 成本账的读写各锁在自己租户的分片里。
//
// 为什么这一格要单独钉：这张表的写家全在 Brain 层的执行路径里（plan/judge/summary/
// jev_choice 四类记录都往这一张表落），读家只有一个（session 导出）。两条路各自都能
// 「看起来正常」而把账记到别人名下：
//   - 写侧漏绑租户 ctx ⇒ 隔离 callback fail-closed，Brain 每轮只留下一条 Warn，成本账
//     整段缺失而任务照常跑完——「LLM 这次花了多少没人说得清」正是 P1-1 立项要收的事；
//   - 读侧少一道租户过滤 ⇒ session_id 是个能猜到的递增小整数，别人的会话成本账（连
//     steps/reasoning 全文）随导出接口漏出去。SessionService 那道归属预检查是唯一的遮挡，
//     仓储这层是它身后的第二道，不能因为「上面查过归属了」就省。
//
// 种子刻意让两个租户用**同一个** session 号：那样断言的才是 tenant_id 谓词本身，
// 而不是「两个租户的会话号恰好不撞」这种随时会被 PK 复用拆穿的巧合。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	llmPlanTenantA = uint(14)
	llmPlanTenantB = uint(35)
	// 号段不与 pruneSessionA/B（retention_tenant_fanout_test.go）重叠：同包并行时
	// 两个用例若共用行号，另一方的清理会改变本方的计数。
	llmPlanSharedSession = uint(792601)
	llmPlanTaskA         = uint(792611)
	llmPlanTaskB         = uint(792612)
	llmPlanSnapshotText  = "snapshot-64kib-placeholder"
	llmPlanPruneDays     = 200
)

func llmPlanDB(t *testing.T) (*gorm.DB, context.Context, context.Context) {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserLLMPlan{})
	if db == nil {
		t.Fatal("测试库不可达：LLM 账本的租户归属无法判定（不 Skip，跳过等于没锁）")
	}
	return db,
		tenant.WithTenantID(context.Background(), llmPlanTenantA),
		tenant.WithTenantID(context.Background(), llmPlanTenantB)
}

// llmPlanSeed 走被测的 Create 落一条账（不是夹具直插 db）：写侧的归属填充本身就是被审的那件事。
func llmPlanSeed(t *testing.T, db *gorm.DB, ctx context.Context, taskID uint, kind string) *model.BrowserLLMPlan {
	t.Helper()
	p := &model.BrowserLLMPlan{
		TaskID: taskID, SessionID: llmPlanSharedSession, Kind: kind,
		Goal: "goal-" + kind, Snapshot: llmPlanSnapshotText,
		Model: "test-model", TokenIn: 10, TokenOut: 5,
	}
	if err := NewBrowserLLMPlanRepositoryWithDB(db).Create(ctx, p); err != nil {
		t.Fatalf("租户 ctx 落 LLM 记录失败: %v", err)
	}
	return p
}

// llmPlanShiftOld 把某租户那批行的 created_at 推到保留期之前（谓词带 task_id，不跨租户位移）。
func llmPlanShiftOld(t *testing.T, db *gorm.DB, ctx context.Context, taskID uint) {
	t.Helper()
	if err := db.WithContext(ctx).Model(&model.BrowserLLMPlan{}).
		Where("task_id = ?", taskID).
		Update("created_at", time.Now().AddDate(0, 0, -llmPlanPruneDays-1)).Error; err != nil {
		t.Fatalf("位移 created_at 失败: %v", err)
	}
}

// TestLLMPlanCreateFillsTenantAndRejectsBareContext 写侧两格：绑租户的 ctx 落自己的号，裸 ctx 被拒。
func TestLLMPlanCreateFillsTenantAndRejectsBareContext(t *testing.T) {
	db, ctxA, _ := llmPlanDB(t)
	repo := NewBrowserLLMPlanRepositoryWithDB(db)

	p := llmPlanSeed(t, db, ctxA, llmPlanTaskA, "plan")
	if p.TenantID != llmPlanTenantA {
		t.Errorf("Create 后 TenantID got=%d want=%d：隔离 callback 没填发起租户，"+
			"这一行会落进默认租户名下（别人读不到、自己的导出也少一条账）", p.TenantID, llmPlanTenantA)
	}

	if err := repo.Create(context.Background(), &model.BrowserLLMPlan{
		TaskID: llmPlanTaskA, SessionID: llmPlanSharedSession, Kind: "judge", Goal: "unbound",
	}); err == nil {
		t.Error("裸 ctx 的 Create 未被拒绝：on 模式下这等于允许把账记到任意租户")
	}
	var n int64
	if err := db.WithContext(ctxA).Model(&model.BrowserLLMPlan{}).Count(&n).Error; err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if n != 1 {
		t.Errorf("被拒的 Create 仍落了行：租户 %d 计数 got=%d want=1", llmPlanTenantA, n)
	}
}

// TestLLMPlanListBySessionIDIsTenantScoped 同一 session 号在两租户各有一条账：各自只读得到自己那条。
func TestLLMPlanListBySessionIDIsTenantScoped(t *testing.T) {
	db, ctxA, ctxB := llmPlanDB(t)
	llmPlanSeed(t, db, ctxA, llmPlanTaskA, "plan")
	llmPlanSeed(t, db, ctxB, llmPlanTaskB, "summary")

	repo := NewBrowserLLMPlanRepositoryWithDB(db)
	for _, tc := range []struct {
		name     string
		ctx      context.Context
		wantTask uint
		wantKind string
	}{
		{"tenantA", ctxA, llmPlanTaskA, "plan"},
		{"tenantB", ctxB, llmPlanTaskB, "summary"},
	} {
		list, err := repo.ListBySessionID(tc.ctx, llmPlanSharedSession, 0)
		if err != nil {
			t.Fatalf("%s 读取失败: %v", tc.name, err)
		}
		if len(list) != 1 {
			t.Errorf("%s 读到 %d 条 want=1（多出来的那条是别人的会话成本账）", tc.name, len(list))
			continue
		}
		if list[0].TaskID != tc.wantTask || list[0].Kind != tc.wantKind {
			t.Errorf("%s 读到的账 got=(task=%d kind=%s) want=(task=%d kind=%s)",
				tc.name, list[0].TaskID, list[0].Kind, tc.wantTask, tc.wantKind)
		}
		if list[0].TenantID != tenantIDOf(t, tc.ctx) {
			t.Errorf("%s 读到的行 tenant_id=%d 不是发起租户", tc.name, list[0].TenantID)
		}
	}

	// 裸 ctx 同样 fail-closed：读侧一旦放行就等于把「谁都能按号导账」写回了库里。
	if _, err := repo.ListBySessionID(context.Background(), llmPlanSharedSession, 0); err == nil {
		t.Error("裸 ctx 的 ListBySessionID 未被拒绝")
	}
}

// TestLLMPlanSnapshotPruneIsTenantScoped 租户 A 的快照裁剪不许动租户 B 的大文本。
// 这条腿与 b19g 的分批腿判的是不同的事：分批管「一句吃多少行」，本条管「吃的是谁的行」。
func TestLLMPlanSnapshotPruneIsTenantScoped(t *testing.T) {
	db, ctxA, ctxB := llmPlanDB(t)
	llmPlanSeed(t, db, ctxA, llmPlanTaskA, "plan")
	llmPlanSeed(t, db, ctxB, llmPlanTaskB, "judge")
	llmPlanShiftOld(t, db, ctxA, llmPlanTaskA)
	llmPlanShiftOld(t, db, ctxB, llmPlanTaskB)

	cutoff := time.Now().AddDate(0, 0, -llmPlanPruneDays)
	repo := NewBrowserLLMPlanRepositoryWithDB(db)

	// 未绑租户：整轮裁剪必须报错且一行不清（service 侧的 Fanout 因此不是可选优化）。
	if _, err := repo.PruneSnapshotText(context.Background(), cutoff); err == nil {
		t.Error("裸 ctx 的快照裁剪未被拒绝：少一道扇出就是拿全租户的账本换一次扫描")
	}

	cleared, err := repo.PruneSnapshotText(ctxA, cutoff)
	if err != nil {
		t.Fatalf("租户 %d 的裁剪报错: %v", llmPlanTenantA, err)
	}
	if cleared != 1 {
		t.Errorf("裁剪行数 got=%d want=1（只该清发起租户那 1 行）", cleared)
	}

	var kept int64
	if err := db.WithContext(ctxB).Model(&model.BrowserLLMPlan{}).
		Where("snapshot <> ''").Count(&kept).Error; err != nil {
		t.Fatalf("读租户 %d 快照失败: %v", llmPlanTenantB, err)
	}
	if kept != 1 {
		t.Errorf("租户 %d 还剩 %d 条带快照 want=1：被别的租户裁掉了＝跨租户销毁成本账原文", llmPlanTenantB, kept)
	}
}

func tenantIDOf(t *testing.T, ctx context.Context) uint {
	t.Helper()
	tid, ok := tenant.FromContext(ctx)
	if !ok {
		t.Fatal("夹具 ctx 未绑租户")
	}
	return tid
}
