// feature_flag_eval_log_tenant_test.go FeatureFlagEvalLog 的租户契约。
//
// 曝光日志是灰度决策的证据：写错租户等于把 A 家用户的曝光记到 B 家账上，
// 读错租户则会把别家的评估明细展示出来。写侧填充与读侧过滤各钉一组；
// 异步写路径（logEvalAsync）必须把租户带进新 ctx，on 模式漏绑要拒。
package repository

import (
	"context"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tenant"
)

func ffSeedEvalLog(t *testing.T, repo FeatureFlagRepository, ctx context.Context, flagKey, contextID string) {
	t.Helper()
	if err := repo.CreateEvalLog(ctx, &model.FeatureFlagEvalLog{
		FlagKey: flagKey, ContextID: contextID, Enabled: true, Reason: "rollout",
	}); err != nil {
		t.Fatalf("写曝光日志失败: %v", err)
	}
}

func TestFeatureFlagEvalLog_WritesAndReadsAreTenantScoped(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOn)
	repo := NewFeatureFlagRepository(db)
	ctxA := tenant.WithTenantID(context.Background(), ffTenantA)
	ctxB := tenant.WithTenantID(context.Background(), ffTenantB)

	ffSeedEvalLog(t, repo, ctxA, "ff_eval_a", "user_a")
	ffSeedEvalLog(t, repo, ctxB, "ff_eval_b", "user_b")

	var physical model.FeatureFlagEvalLog
	if err := db.WithContext(tenant.WithSystemScope(context.Background())).
		Where("flag_key = ?", "ff_eval_a").First(&physical).Error; err != nil {
		t.Fatalf("物理读取曝光日志失败: %v", err)
	}
	if physical.TenantID != ffTenantA {
		t.Fatalf("曝光日志 tenant_id=%d，期望 %d", physical.TenantID, ffTenantA)
	}

	listA, err := repo.ListEvalLogs(ctxA, "ff_eval_a", 100)
	if err != nil {
		t.Fatalf("租户 A 读曝光日志失败: %v", err)
	}
	if len(listA) != 1 || listA[0].ContextID != "user_a" {
		t.Fatalf("租户 A 读到的曝光日志 %+v，期望仅自己那条", listA)
	}

	// 同一个 flag key 在 B 租户下必须读不到 A 的记录（key 只租户内唯一）。
	listB, err := repo.ListEvalLogs(ctxB, "ff_eval_a", 100)
	if err != nil {
		t.Fatalf("租户 B 读曝光日志调用失败: %v", err)
	}
	if len(listB) != 0 {
		t.Fatalf("租户 B 读到了租户 A 的曝光日志 %+v ⇒ 过滤没生效", listB)
	}
}

// TestFeatureFlagEvalLog_UnboundCtxIsRejected on 模式下漏绑租户的曝光写入必须被拒：
// 队列/异步路径漏带租户时，行会静默落默认租户，曝光数据整体串账。
func TestFeatureFlagEvalLog_UnboundCtxIsRejected(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOn)
	repo := NewFeatureFlagRepository(db)

	err := repo.CreateEvalLog(context.Background(), &model.FeatureFlagEvalLog{
		FlagKey: "ff_eval_x", ContextID: "u", Enabled: true, Reason: "rollout",
	})
	if err == nil {
		t.Fatal("on 模式无租户 ctx 写曝光日志成功 ⇒ 行会落默认租户")
	}
}

// TestFeatureFlagEvalLog_OffModeWritesDefaultTenant 离线单机显式落默认租户，
// 语义与审计日志一致（不依赖列默认值）。
func TestFeatureFlagEvalLog_OffModeWritesDefaultTenant(t *testing.T) {
	db := ffSetupDB(t, tenant.ModeOff)
	repo := NewFeatureFlagRepository(db)

	if err := repo.CreateEvalLog(context.Background(), &model.FeatureFlagEvalLog{
		FlagKey: "ff_eval_off", ContextID: "u", Enabled: false, Reason: "disabled",
	}); err != nil {
		t.Fatalf("off 模式写曝光日志失败: %v", err)
	}
	var physical model.FeatureFlagEvalLog
	if err := db.WithContext(context.Background()).Where("flag_key = ?", "ff_eval_off").First(&physical).Error; err != nil {
		t.Fatalf("读取曝光日志失败: %v", err)
	}
	if physical.TenantID != tenant.DefaultTenantID {
		t.Fatalf("off 模式曝光日志 tenant_id=%d，期望默认租户 %d", physical.TenantID, tenant.DefaultTenantID)
	}
}
