// handoff_chain_tenant_test.go 升级链 cron 的端到端租户口径与触发判据。
//
// 这组用例钉两件事，都是 2026-10-20 线上那轮 fanoutTask 改动之后才暴露出来的：
//  1. 逐租户扇出之后，"扫本租户、升级本租户、决策登记本租户"必须成立——
//     否则每个租户各扫同一批会话、互相把对方的会话标成已升级，比不扇出更糟。
//  2. escalate_24h 这条默认规则得真的能触发。快照原先把"是否已解决"读成 updated_at，
//     而 updated_at 每行都非空 ⇒ 规则永不触发，线上表现为 cron 一直在跑但 triggered 恒为 0。
package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

// handoffCronOldAt 造行共用的创建时刻：30 小时前，跨过 escalate_24h 的 24 小时门槛。
var handoffCronOldAt = time.Now().Add(-30 * time.Hour)

// seedOldHandoffSession 以 tenantID 身份种一条"超期未解决"会话。
//
// rating 给 5：列默认 0 会顺带触发 transfer_low_csat（csat<=2 且未解决），
// 那样 triggered 的期望值就不是 1 了，用例也就说不清自己在验哪条规则。
func seedOldHandoffSession(t *testing.T, db *gorm.DB, tenantID uint, sessionID string) {
	t.Helper()
	err := db.WithContext(tenant.WithTenantID(context.Background(), tenantID)).
		Create(&model.CustomerSession{
			SessionID: sessionID,
			OneID:     "oneid_" + sessionID,
			Status:    model.SessionStatusPending,
			Rating:    5,
			CreatedAt: handoffCronOldAt,
		}).Error
	if err != nil {
		t.Fatalf("以租户 %d 种超期会话 %s 失败: %v", tenantID, sessionID, err)
	}
}

func newHandoffTenantCtx(tenantID uint) context.Context {
	return tenant.WithTenantID(context.Background(), tenantID)
}

func TestHandoffChainRunCron_FiresAndStaysInsideTenant(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{}, &model.HandoffDecisionRecord{})
	seedOldHandoffSession(t, db, 11, "sess_old_11")
	seedOldHandoffSession(t, db, 12, "sess_old_12")

	svc := NewHandoffChainService().WithDB(db)

	triggered, err := svc.RunCron(newHandoffTenantCtx(11), 200)
	if err != nil {
		t.Fatalf("租户 11 的 cron 扫描失败: %v", err)
	}
	if triggered != 1 {
		t.Errorf("租户 11 触发规则数 = %d，期望 1（escalate_24h）；0 意味着规则判据又读回了 updated_at 那类恒非空列", triggered)
	}

	// 本租户升级、别租户不动。
	var mine, theirs model.CustomerSession
	if err := db.WithContext(newHandoffTenantCtx(11)).First(&mine, "session_id = ?", "sess_old_11").Error; err != nil {
		t.Fatalf("读回租户 11 的会话失败: %v", err)
	}
	if mine.Status != "escalated" {
		t.Errorf("租户 11 的会话 status = %s，期望 escalated（触发了规则却没落库）", mine.Status)
	}
	if err := db.WithContext(newHandoffTenantCtx(12)).First(&theirs, "session_id = ?", "sess_old_12").Error; err != nil {
		t.Fatalf("读回租户 12 的会话失败: %v", err)
	}
	if theirs.Status != model.SessionStatusPending {
		t.Errorf("租户 12 的会话被租户 11 的扫描改成了 %s ⇒ 扫描没有按租户收窄", theirs.Status)
	}

	// 决策登记：一条、归本租户、按业务会话号（与置信度那路写同一口径）。
	var records []model.HandoffDecisionRecord
	if err := db.WithContext(newHandoffTenantCtx(11)).Find(&records).Error; err != nil {
		t.Fatalf("读决策记录失败: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("租户 11 名下决策记录数 = %d，期望 1（%+v）", len(records), records)
	}
	if records[0].SessionID != "sess_old_11" {
		t.Errorf("决策记录 session_id = %q，期望业务会话号 sess_old_11；登记成自增主键会让按会话查决策那条路读不到这批行", records[0].SessionID)
	}
	if records[0].TenantID != 11 {
		t.Errorf("决策记录 tenant_id = %d，期望 11", records[0].TenantID)
	}

	// 再跑租户 12 那一轮：只该轮到自己那条。
	triggered12, err := svc.RunCron(newHandoffTenantCtx(12), 200)
	if err != nil {
		t.Fatalf("租户 12 的 cron 扫描失败: %v", err)
	}
	if triggered12 != 1 {
		t.Errorf("租户 12 触发规则数 = %d，期望 1", triggered12)
	}
	var escalated11 int64
	if err := db.WithContext(newHandoffTenantCtx(11)).
		Model(&model.CustomerSession{}).
		Where("status = ?", "escalated").
		Count(&escalated11).Error; err != nil {
		t.Fatalf("统计租户 11 升级数失败: %v", err)
	}
	if escalated11 != 1 {
		t.Errorf("租户 11 升级会话数 = %d，期望仍是 1（租户 12 那一轮不该回头动它）", escalated11)
	}
}

// TestHandoffChainRunCron_UnboundCtxIsRefused 漏注入租户 ctx 的调用必须报错而不是扫全表。
func TestHandoffChainRunCron_UnboundCtxIsRefused(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})

	db := testutil.NewTestDB(t, &model.CustomerSession{}, &model.SystemConfigKV{}, &model.HandoffDecisionRecord{})
	seedOldHandoffSession(t, db, 11, "sess_unbound_11")

	svc := NewHandoffChainService().WithDB(db)
	if _, err := svc.RunCron(context.Background(), 200); err == nil {
		t.Fatal("无租户 ctx 的 RunCron 返回 nil，期望报错（放行等于静默扫全表）")
	}
	var sess model.CustomerSession
	if err := db.WithContext(newHandoffTenantCtx(11)).First(&sess, "session_id = ?", "sess_unbound_11").Error; err != nil {
		t.Fatalf("读回会话失败: %v", err)
	}
	if sess.Status == "escalated" {
		t.Error("被拒的扫描仍然升级了会话")
	}
}
