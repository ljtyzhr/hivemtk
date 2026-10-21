// system_config_kv_scope_test.go 钉住 backup.strategy 的键归属。
//
// v3.65.0 把 backup.strategy 从 system_config_kv 迁到 platform_config_kv，
// 但 BackupGapService 曾继续用租户级仓储读写：迁移后读不到原值（回退默认），
// 新保存又落回租户表，形成两份互不相干的策略。这里从真实构造入口出发，
// 断言值只落在平台表。
package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

func TestBackupGapService_StrategyLivesInPlatformKV(t *testing.T) {
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.PlatformConfigKV{}, &model.SystemConfigKV{})
	if db == nil {
		t.Fatal("测试库不可达：键归属判不了（不 Skip，跳过等于没锁）")
	}
	ctx := tenant.WithTenantID(context.Background(), 91)
	sysCtx := tenant.WithSystemScope(context.Background())

	svc := NewBackupGapService(db)
	if err := svc.SaveStrategy(ctx, &BackupStrategy{Enabled: true, RetentionDays: 30}); err != nil {
		t.Fatalf("保存备份策略失败: %v", err)
	}

	var platformRow model.PlatformConfigKV
	if err := db.WithContext(sysCtx).Where("key = ?", backupStrategyKey).First(&platformRow).Error; err != nil {
		t.Fatalf("策略没有落在 platform_config_kv：%v", err)
	}
	var tenantRows int64
	if err := db.WithContext(sysCtx).Model(&model.SystemConfigKV{}).
		Where("key = ?", backupStrategyKey).Count(&tenantRows).Error; err != nil {
		t.Fatalf("统计 system_config_kv 行数失败: %v", err)
	}
	if tenantRows != 0 {
		t.Fatalf("system_config_kv 里出现了 %d 行 backup.strategy ⇒ 又写回了租户表", tenantRows)
	}

	got := svc.GetStrategy(ctx)
	if !got.Enabled || got.RetentionDays != 30 {
		t.Fatalf("读回的策略=%+v，期望 enabled=true retention_days=30", got)
	}
}
