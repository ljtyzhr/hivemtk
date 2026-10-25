// cron_bridge_replay_tenant_test.go 钉住「离线消息回扫」这条 cron 的租户注入通道。
//
// 回归背景（2026-10-22 线上）：SaaS 部署（TENANT_MODE=on）下这条任务每 5 分钟打出一行
//
//	[BridgeReplay] 渠道快照读取失败: [tenant] 表 bridge_accounts 需要租户上下文但 ctx 未绑定（query）
//
// 业务后果是延后出站永远不补投、孤儿出站永远不结算（任务只报数，接口层完全无感）。
// 根因：cron 交来的 ctx 不含租户，而 bridge_accounts / message_hub / reach_delayed_outbound
// 都带 tenant_id 列，on 模式下未绑定租户即被隔离守卫 fail-closed 拒绝。
package cron

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/service"
)

// TestBridgeOfflineReplayCron_ScansEachActiveTenant 回扫必须逐活跃租户各跑一遍，
// 且每次跑的时候 ctx 已绑定该租户 —— 否则渠道快照查询被隔离守卫拒绝，整轮空转。
func TestBridgeOfflineReplayCron_ScansEachActiveTenant(t *testing.T) {
	withActiveTenants(t, 7, 8)

	gdb := testutil.NewTestDB(t, &model.BridgeAccount{}, &model.MessageHub{})
	prevDB := pkgdb.GetDB()
	pkgdb.SetTestDB(gdb)
	t.Cleanup(func() { pkgdb.SetTestDB(prevDB) })

	// 造两行账号行：分属两个租户。回扫要能在各自租户的 ctx 下读到它们。
	for _, tc := range []struct {
		tenantID uint
		account  string
	}{
		{tenantID: 7, account: "replay-acct-7"},
		{tenantID: 8, account: "replay-acct-8"},
	} {
		ctxT := tenant.WithTenantID(context.Background(), tc.tenantID)
		acc := &model.BridgeAccount{
			Channel:    "douyin",
			AccountID:  tc.account,
			Status:     "online",
			LastSyncAt: ptrTime(time.Now()),
		}
		if err := gdb.WithContext(ctxT).Create(acc).Error; err != nil {
			t.Fatalf("seed 账号行失败(tenant=%d): %v", tc.tenantID, err)
		}
	}

	stats := service.NewBridgeOfflineReplayService().RunOnceAllTenants(context.Background())

	// 判据是"扫到了渠道"：为 0 说明快照查询被守卫拒绝（这正是回归前的线上形态）。
	if stats.ScannedChannels != 2 {
		t.Fatalf("回扫扫到的渠道数=%d want 2（0/1 说明隔离守卫拒绝了快照查询，扇出未生效）", stats.ScannedChannels)
	}
	if stats.OnlineChannels != 2 {
		t.Errorf("在线渠道数=%d want 2", stats.OnlineChannels)
	}
}

// TestBridgeOfflineReplayCron_OffModeRunsOnce 离线单机模式退化口径：单默认租户一次，
// 行为与历史版本逐字节等价。
func TestBridgeOfflineReplayCron_OffModeRunsOnce(t *testing.T) {
	prev := tenant.CurrentMode()
	tenant.SetMode(tenant.ModeOff)
	t.Cleanup(func() { tenant.SetMode(prev) })

	gdb := testutil.NewTestDB(t, &model.BridgeAccount{}, &model.MessageHub{})
	prevDB := pkgdb.GetDB()
	pkgdb.SetTestDB(gdb)
	t.Cleanup(func() { pkgdb.SetTestDB(prevDB) })

	ctxT := tenant.WithTenantID(context.Background(), tenant.DefaultTenantID)
	acc := &model.BridgeAccount{
		Channel:    "douyin",
		AccountID:  "replay-acct-default",
		Status:     "online",
		LastSyncAt: ptrTime(time.Now()),
	}
	if err := gdb.WithContext(ctxT).Create(acc).Error; err != nil {
		t.Fatalf("seed 账号行失败: %v", err)
	}

	stats := service.NewBridgeOfflineReplayService().RunOnceAllTenants(context.Background())
	if stats.ScannedChannels != 1 {
		t.Fatalf("off 模式应扫到 1 个渠道，实际 %d", stats.ScannedChannels)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
