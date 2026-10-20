// handoff_chain_repo_tenant_test.go 升级链仓储的租户与会话号口径。
//
// 这四组判据钉的是同一件事：cron 经 Fanout 逐租户执行时，"扫哪些会话""升级哪一行"
// 必须落在本租户内，且 on 模式下漏绑租户 ctx 要被拒而不是静默扫全表。
//
// 为什么必须显式写 WHERE：隔离 callback 只给"认得出目标表"的语句注入 tenant_id，
// 而 Table("customer_sessions") 是 schema-less 形态——它既不会被拒、也不会被过滤。
// 2026-10-20 线上日志实测过这个洞的形状：一轮 tick 打印两行完全相同的 sessions=1
// （两个租户各扫到同一批会话），修 Fanout 之前它甚至不报错。
package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// handoffOldSessionAt 是所有造行共用的创建时刻：30 小时前，跨过 escalate_24h 的门槛。
var handoffOldSessionAt = time.Now().Add(-30 * time.Hour)

func setupHandoffTenantTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	return testutil.NewTestDB(t, models...)
}

// seedHandoffSessionAs 以 tenantID 身份种一条会话。
//
// rating 显式给值：列默认 0 会让 transfer_low_csat（csat<=2）顺带触发，
// 用例想只断 escalate_24h 时就得把好评先占上。
func seedHandoffSessionAs(t *testing.T, db *gorm.DB, tenantID uint, sessionID string, status model.SessionStatus, rating int) {
	t.Helper()
	ctx := tenant.WithTenantID(context.Background(), tenantID)
	row := &model.CustomerSession{
		SessionID: sessionID,
		OneID:     "oneid_" + sessionID,
		Status:    status,
		Rating:    rating,
		CreatedAt: handoffOldSessionAt,
	}
	if err := db.WithContext(ctx).Create(row).Error; err != nil {
		t.Fatalf("以租户 %d 种会话 %s 失败: %v", tenantID, sessionID, err)
	}
}

// readHandoffStatus 按租户读回会话状态（Model 形态，走隔离 callback）。
func readHandoffStatus(t *testing.T, db *gorm.DB, tenantID uint, sessionID string) string {
	t.Helper()
	var sess model.CustomerSession
	if err := db.WithContext(tenant.WithTenantID(context.Background(), tenantID)).
		Model(&model.CustomerSession{}).
		Where("session_id = ?", sessionID).
		First(&sess).Error; err != nil {
		t.Fatalf("读回会话 %s（租户 %d）状态失败: %v", sessionID, tenantID, err)
	}
	return string(sess.Status)
}

// TestHandoffScan_ListsOnlyBoundTenantSessions 逐租户扇出的前提是扫描本身按租户过滤：
// 不过滤则每个租户都拿到同一批会话，升级动作会跨租户互踩。
func TestHandoffScan_ListsOnlyBoundTenantSessions(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := setupHandoffTenantTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{})
	seedHandoffSessionAs(t, db, 11, "sess_t11_a", model.SessionStatusPending, 5)
	seedHandoffSessionAs(t, db, 11, "sess_t11_b", model.SessionStatusResolved, 5)
	seedHandoffSessionAs(t, db, 12, "sess_t12_a", model.SessionStatusPending, 5)

	r := NewHandoffChainRepositoryWithDB(db)

	ids11, err := r.ListUnresolvedSessionIDs(tenant.WithTenantID(context.Background(), 11), 50)
	if err != nil {
		t.Fatalf("租户 11 扫描失败: %v", err)
	}
	if len(ids11) != 1 || ids11[0] != "sess_t11_a" {
		t.Errorf("租户 11 扫到 %v，期望只有一条未解决的 sess_t11_a（已解决的按状态排除、别租户的不该出现）", ids11)
	}

	ids12, err := r.ListUnresolvedSessionIDs(tenant.WithTenantID(context.Background(), 12), 50)
	if err != nil {
		t.Fatalf("租户 12 扫描失败: %v", err)
	}
	if len(ids12) != 1 || ids12[0] != "sess_t12_a" {
		t.Errorf("租户 12 扫到 %v，期望只有 sess_t12_a", ids12)
	}

	// 快照同样受限：拿别人的业务号读，只能读到零值（Scan 无行不报错，所以必须断字段）。
	sess, err := r.GetHandoffSessionSnapshot(tenant.WithTenantID(context.Background(), 11), "sess_t12_a")
	if err != nil {
		t.Fatalf("跨租户读快照失败: %v", err)
	}
	if sess.SessionID != "" || sess.Status != "" {
		t.Errorf("租户 11 读到了租户 12 的会话快照 %+v ⇒ 过滤没生效", sess)
	}
}

// TestHandoffScan_UnboundCtxIsRejectedInOnMode on 模式下无租户 ctx 一律拒绝：
// 放行等于"漏注入 ctx 的调用方静默扫全表"，那正是 Fanout 修掉的行为，不能留第二条路。
func TestHandoffScan_UnboundCtxIsRejectedInOnMode(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := setupHandoffTenantTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{})
	seedHandoffSessionAs(t, db, 11, "sess_unbound", model.SessionStatusPending, 5)

	r := NewHandoffChainRepositoryWithDB(db)
	ctx := context.Background()

	if _, err := r.ListUnresolvedSessionIDs(ctx, 50); !errors.Is(err, ErrHandoffTenantContextMissing) {
		t.Errorf("无租户 ctx 扫描返回 err=%v，期望 ErrHandoffTenantContextMissing", err)
	}
	if _, err := r.GetHandoffSessionSnapshot(ctx, "sess_unbound"); !errors.Is(err, ErrHandoffTenantContextMissing) {
		t.Errorf("无租户 ctx 读快照返回 err=%v，期望 ErrHandoffTenantContextMissing", err)
	}
	if err := r.EscalateSession(ctx, "sess_unbound"); !errors.Is(err, ErrHandoffTenantContextMissing) {
		t.Errorf("无租户 ctx 升级会话返回 err=%v，期望 ErrHandoffTenantContextMissing", err)
	}
	if got := readHandoffStatus(t, db, 11, "sess_unbound"); got != string(model.SessionStatusPending) {
		t.Errorf("被拒的升级仍然改了数据：status=%s，期望仍是 pending", got)
	}
}

// TestEscalateSession_CannotEscalateOtherTenantSession 升级这一写动作跨租户即数据损坏：
// 把别人家的会话标成"已升级"，对方的人工队列里凭空多出一条没人认领的单子。
func TestEscalateSession_CannotEscalateOtherTenantSession(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := setupHandoffTenantTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{})
	seedHandoffSessionAs(t, db, 11, "sess_mine", model.SessionStatusPending, 5)
	seedHandoffSessionAs(t, db, 12, "sess_theirs", model.SessionStatusPending, 5)

	r := NewHandoffChainRepositoryWithDB(db)
	if err := r.EscalateSession(tenant.WithTenantID(context.Background(), 11), "sess_theirs"); err != nil {
		t.Fatalf("跨租户升级调用失败: %v", err)
	}
	if got := readHandoffStatus(t, db, 12, "sess_theirs"); got != string(model.SessionStatusPending) {
		t.Errorf("租户 11 把租户 12 的会话升级成了 %s，期望仍是 pending", got)
	}

	if err := r.EscalateSession(tenant.WithTenantID(context.Background(), 12), "sess_theirs"); err != nil {
		t.Fatalf("本租户升级调用失败: %v", err)
	}
	if got := readHandoffStatus(t, db, 12, "sess_theirs"); got != "escalated" {
		t.Errorf("本租户升级后 status=%s，期望 escalated", got)
	}
}

// TestHandoffScan_OffModeKeepsLegacyPassThrough 离线单机退化：不过滤、不拒绝。
//
// 这条不是兼容包袱——off 模式下隔离 callback 本身就直通，仓储若在这里加默认租户条件，
// 等于让"离线库里的历史数据"（tenant_id 是任意值）读不出来；口径要和 callback 一致。
func TestHandoffScan_OffModeKeepsLegacyPassThrough(t *testing.T) {
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(tenant.ModeOff) })

	db := setupHandoffTenantTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{})
	for _, row := range []struct {
		tenantID  uint
		sessionID string
	}{
		{11, "sess_off_11"},
		{12, "sess_off_12"},
	} {
		if err := db.Create(&model.CustomerSession{
			TenantID:  row.tenantID,
			SessionID: row.sessionID,
			OneID:     "oneid_" + row.sessionID,
			Status:    model.SessionStatusPending,
			Rating:    5,
			CreatedAt: handoffOldSessionAt,
		}).Error; err != nil {
			t.Fatalf("种离线会话 %s 失败: %v", row.sessionID, err)
		}
	}

	r := NewHandoffChainRepositoryWithDB(db)
	ids, err := r.ListUnresolvedSessionIDs(context.Background(), 50)
	if err != nil {
		t.Fatalf("离线模式扫描失败: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("离线模式无租户 ctx 扫到 %v，期望两条都在（直通语义）", ids)
	}
}

// TestGetHandoffSessionSnapshot_ResolvedAtIsNotUpdatedAt 快照的"已解决时刻"必须读
// resolved_at：updated_at 由 autoUpdateTime 维护、每行都非空，而两条默认规则都要求
// "解决时刻为空"才触发 ⇒ 映射到 updated_at 会让整条升级链永不触发。
func TestGetHandoffSessionSnapshot_ResolvedAtIsNotUpdatedAt(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := setupHandoffTenantTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{})
	seedHandoffSessionAs(t, db, 11, "sess_open", model.SessionStatusPending, 5)
	seedHandoffSessionAs(t, db, 11, "sess_done", model.SessionStatusResolved, 5)

	r := NewHandoffChainRepositoryWithDB(db)
	open, err := r.GetHandoffSessionSnapshot(tenant.WithTenantID(context.Background(), 11), "sess_open")
	if err != nil {
		t.Fatalf("读未解决会话快照失败: %v", err)
	}
	if open.ResolvedAt != nil {
		t.Errorf("未解决会话（resolved_at 为 NULL）读出的 ResolvedAt=%v ⇒ 这一列取的不是 resolved_at", open.ResolvedAt)
	}
	if open.CreatedAt.After(handoffOldSessionAt.Add(time.Minute)) {
		t.Errorf("created_at 读出 %v，期望约 %v（规则按它判 24 小时）", open.CreatedAt, handoffOldSessionAt)
	}

	// 已解决的那行补上 resolved_at，读回必须非空——否则"NULL 才触发"这条判据没牙：
	// 一个恒为 NULL 的映射同样能让上一段断言通过。
	now := time.Now()
	if err := db.WithContext(tenant.WithTenantID(context.Background(), 11)).
		Model(&model.CustomerSession{}).
		Where("session_id = ?", "sess_done").
		UpdateColumn("resolved_at", &now).Error; err != nil {
		t.Fatalf("写入 resolved_at 失败: %v", err)
	}
	done, err := r.GetHandoffSessionSnapshot(tenant.WithTenantID(context.Background(), 11), "sess_done")
	if err != nil {
		t.Fatalf("读已解决会话快照失败: %v", err)
	}
	if done.ResolvedAt == nil {
		t.Error("写进 resolved_at 的时刻读不出来 ⇒ 这一列根本没映射对")
	}
}
