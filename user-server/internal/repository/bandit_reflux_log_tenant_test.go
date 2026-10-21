// bandit_reflux_log_tenant_test.go 回流台账的租户契约。
//
// 台账靠两个唯一键防重复入账；隔离后唯一键是 (tenant_id, event_id) 与
// (tenant_id, session_id, sop_id, signal_key)——A 租户入账过的事件/会话转化
// 不得让 B 租户的同键事件被判重，否则 B 的奖励被静默吞掉。
package repository

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

const (
	brlTenantA = uint(251)
	brlTenantB = uint(252)
)

func brlSetupDB(t *testing.T) *gorm.DB {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.BanditRefluxLog{})
	if db == nil {
		t.Fatal("测试库不可达：回流台账的租户去重判不了（不 Skip，跳过等于没锁）")
	}
	return db
}

func brlLog(eventID, sessionID, signalKey string, sopID uint) *model.BanditRefluxLog {
	return &model.BanditRefluxLog{
		EventID:   eventID,
		SessionID: sessionID,
		SOPID:     sopID,
		SignalKey: signalKey,
		Reward:    2,
		Success:   true,
	}
}

func TestBanditRefluxLogRepository_DedupIsTenantScoped(t *testing.T) {
	db := brlSetupDB(t)
	repo := NewFeedbackLoopRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), brlTenantA)
	ctxB := tenant.WithTenantID(context.Background(), brlTenantB)
	sys := tenant.WithSystemScope(context.Background())

	inserted, err := repo.CreateRefluxLog(ctxA, brlLog("evt-shared", "sess-1", "conversion", 7))
	if err != nil || !inserted {
		t.Fatalf("租户 A 首次入账应成功, inserted=%v err=%v", inserted, err)
	}
	var row model.BanditRefluxLog
	if err := db.WithContext(sys).Where("event_id = ?", "evt-shared").First(&row).Error; err != nil {
		t.Fatalf("读取租户 A 台账失败: %v", err)
	}
	if row.TenantID != brlTenantA {
		t.Errorf("台账 tenant_id=%d, want %d（应由 ctx 盖章）", row.TenantID, brlTenantA)
	}

	// 同 event_id + 同 (session,sop,signal) 在 B 租户必须能独立入账
	inserted, err = repo.CreateRefluxLog(ctxB, brlLog("evt-shared", "sess-1", "conversion", 7))
	if err != nil || !inserted {
		t.Fatalf("租户 B 同键事件应能入账, inserted=%v err=%v", inserted, err)
	}

	// 同租户内换 event_id 但转化键相同 → 仍须去重
	inserted, err = repo.CreateRefluxLog(ctxA, brlLog("evt-dup", "sess-1", "conversion", 7))
	if err != nil {
		t.Fatalf("同租户重复入账不应报错: %v", err)
	}
	if inserted {
		t.Error("同租户同会话同 SOP 同类信号应被唯一键去重")
	}

	var total int64
	if err := db.WithContext(sys).Model(&model.BanditRefluxLog{}).Count(&total).Error; err != nil {
		t.Fatalf("统计台账失败: %v", err)
	}
	if total != 2 {
		t.Errorf("台账行数=%d, want 2（每租户一条）", total)
	}
}

func TestBanditRefluxLogRepository_OnModeUnboundContextFailsClosed(t *testing.T) {
	db := brlSetupDB(t)
	repo := NewFeedbackLoopRepositoryWithDB(db)

	if _, err := repo.CreateRefluxLog(context.Background(), brlLog("evt-unbound", "sess-u", "conversion", 1)); err == nil {
		t.Fatal("on 模式无租户绑定 ctx 时应 fail-closed")
	}
	var total int64
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Model(&model.BanditRefluxLog{}).Count(&total).Error; err != nil {
		t.Fatalf("统计台账失败: %v", err)
	}
	if total != 0 {
		t.Errorf("fail-closed 后不应落行, got %d", total)
	}
}

func TestBanditRefluxLogRepository_DeleteCompensationIsTenantScoped(t *testing.T) {
	db := brlSetupDB(t)
	repo := NewFeedbackLoopRepositoryWithDB(db)
	ctxA := tenant.WithTenantID(context.Background(), brlTenantA)
	sys := tenant.WithSystemScope(context.Background())

	if _, err := repo.CreateRefluxLog(ctxA, brlLog("evt-del", "sess-del", "conversion", 3)); err != nil {
		t.Fatalf("预置台账失败: %v", err)
	}
	var row model.BanditRefluxLog
	if err := db.WithContext(sys).Where("event_id = ?", "evt-del").First(&row).Error; err != nil {
		t.Fatalf("读取台账失败: %v", err)
	}

	// 跨租户补偿删除必须报「不存在」而不是静默成功
	if err := repo.DeleteRefluxLogByID(tenant.WithTenantID(context.Background(), brlTenantB), row.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("跨租户删除应返回 ErrRecordNotFound, got %v", err)
	}
	var count int64
	db.WithContext(sys).Model(&model.BanditRefluxLog{}).Where("id = ?", row.ID).Count(&count)
	if count != 1 {
		t.Fatalf("跨租户删除不应动他人行, got %d", count)
	}

	if err := repo.DeleteRefluxLogByID(ctxA, row.ID); err != nil {
		t.Fatalf("本租户补偿删除失败: %v", err)
	}
	db.WithContext(sys).Model(&model.BanditRefluxLog{}).Where("id = ?", row.ID).Count(&count)
	if count != 0 {
		t.Errorf("本租户删除后应无行, got %d", count)
	}
}
