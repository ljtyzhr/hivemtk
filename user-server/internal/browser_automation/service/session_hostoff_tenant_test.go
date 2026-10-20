package service

// 契约锁（实体审查 #10，TENANT_MODE=on）：Host 掉线收口必须真的收得掉。
//
// 立项依据（读装配现场，不是推断）：`HostRegistry.onDisconnect` 递给钩子的 ctx 是
// `context.WithTimeout(context.Background(), …)`（service/host_registry.go:231）——
// WS 读协程里没有请求，也就没有租户。而钩子在 routes 里直接调
// `sessionRepo.FailRunningByUser(ctx, …)`（router/browser_automation_routes.go:85），
// browser_sessions 是租户表，于是 saas 形态下这一刀被隔离 callback fail-closed：
// 日志里只有一行「断连清理失败」，库里那些 active 会话永远躺着。
// 后果不止是脏数据：在途会话占着并发闸（CountRunningByUser），
// 用户关掉 Chrome 之后**再也起不了新任务**，而界面上看不出为什么。
//
// 收口判据是 user_id，而 core_users 的主键在全库唯一（一个用户只属于一个租户），
// 所以 system scope 下的这条 UPDATE 触达的行集与「已知其租户时租户内执行」完全相同；
// 反过来，把 ctx 硬绑成某个租户反倒要在此处先去查用户归属（跨域依赖 + 多一次读）。
// 口径与本域对账器（stale_reconcile 整体跑在 WithSystemScope 下）一致。

import (
	"context"
	"testing"

	"gorm.io/gorm"

	"hivemtk-user/internal/browser_automation/model"
	"hivemtk-user/internal/browser_automation/repository"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	hostOffTenantA = uint(22)
	hostOffTenantB = uint(44)
	// 号段避开 session_duration(791xxx)/receipt(792xxx)/session 租户锁(793xxx)。
	hostOffTaskA = uint(794001)
	hostOffTaskB = uint(794002)
	hostOffUserA = uint(794101)
	hostOffUserB = uint(794102)
)

func hostOffSeed(t *testing.T, db *gorm.DB, ctx context.Context, id, taskID, userID uint, status string) {
	t.Helper()
	row := &model.BrowserSession{ID: id, TaskID: taskID, UserID: userID,
		Url: "https://www.xiaohongshu.com/explore", Status: status}
	if err := db.WithContext(ctx).Create(row).Error; err != nil {
		t.Fatalf("会话 %d 落库失败: %v", id, err)
	}
}

func hostOffStatus(t *testing.T, db *gorm.DB, id uint) string {
	t.Helper()
	var s model.BrowserSession
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		First(&s, id).Error; err != nil {
		t.Fatalf("回读会话 %d 失败: %v", id, err)
	}
	return s.Status
}

// TestHostDisconnectHookConvergesWithUnboundContext 钩子拿到的就是裸 ctx：
// 收口必须生效（修复前 fail-closed，两行 active 原样躺着）。
func TestHostDisconnectHookConvergesWithUnboundContext(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserSession{})
	if db == nil {
		t.Fatal("测试库不可达：掉线收口的有效性无法判定（不 Skip，跳过等于没锁）")
	}
	ctxA := tenant.WithTenantID(context.Background(), hostOffTenantA)
	ctxB := tenant.WithTenantID(context.Background(), hostOffTenantB)
	svc := NewSessionService(repository.NewBrowserSessionRepositoryWithDB(db), nil, nil)

	hostOffSeed(t, db, ctxA, 794201, hostOffTaskA, hostOffUserA, "active")
	hostOffSeed(t, db, ctxA, 794202, hostOffTaskA, hostOffUserA, "created") // 排队中也算在途
	hostOffSeed(t, db, ctxA, 794203, hostOffTaskA, hostOffUserA, "completed")
	hostOffSeed(t, db, ctxB, 794204, hostOffTaskB, hostOffUserB, "active")

	// 与生产同形：context.Background()，无租户。
	n, err := svc.FailRunningForUser(context.Background(), hostOffUserA, "Host 掉线（Chrome 断开或扩展重载）")
	if err != nil {
		t.Fatalf("裸 ctx 的掉线收口失败: %v", err)
	}
	if n != 2 {
		t.Errorf("收口影响 %d 行，期望 2（active + created，已终态的 794203 不算）", n)
	}
	for id, want := range map[uint]string{794201: "failed", 794202: "failed", 794203: "completed"} {
		if got := hostOffStatus(t, db, id); got != want {
			t.Errorf("会话 %d 状态 = %q，期望 %q", id, got, want)
		}
	}
	// 别的用户不受牵连：钩子按 user_id 收口，system scope 不等于全场清理。
	if got := hostOffStatus(t, db, 794204); got != "active" {
		t.Errorf("B 用户会话状态被改成 %q，期望仍是 active", got)
	}

	// 并发闸随之释放：A 的在途数必须归零，B 仍占 1。
	repo := repository.NewBrowserSessionRepositoryWithDB(db)
	if c, err := repo.CountRunningByUser(ctxA, hostOffUserA); err != nil || c != 0 {
		t.Errorf("收口后 A 的在途数 = %d（err=%v），期望 0（闸不放=用户再也起不了任务）", c, err)
	}
	if c, err := repo.CountRunningByUser(ctxB, hostOffUserB); err != nil || c != 1 {
		t.Errorf("B 的在途数 = %d（err=%v），期望仍是 1", c, err)
	}
}

// TestHostDisconnectHookConvergesWithBoundContext 同一条路在已绑租户的 ctx 下也必须生效
// （将来钩子若改由带请求上下文的路径触发，不能因为多包了一层 system scope 就失配）。
func TestHostDisconnectHookConvergesWithBoundContext(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })
	db := testutil.NewTestDB(t, &model.BrowserSession{})
	if db == nil {
		t.Fatal("测试库不可达：掉线收口的有效性无法判定（不 Skip，跳过等于没锁）")
	}
	ctxA := tenant.WithTenantID(context.Background(), hostOffTenantA)
	svc := NewSessionService(repository.NewBrowserSessionRepositoryWithDB(db), nil, nil)
	hostOffSeed(t, db, ctxA, 794211, hostOffTaskA, hostOffUserA, "active")

	n, err := svc.FailRunningForUser(ctxA, hostOffUserA, "Host 掉线")
	if err != nil || n != 1 {
		t.Fatalf("已绑租户 ctx 的掉线收口 = %d 行（err=%v），期望 1", n, err)
	}
	if got := hostOffStatus(t, db, 794211); got != "failed" {
		t.Errorf("会话状态 = %q，期望 failed", got)
	}
}
