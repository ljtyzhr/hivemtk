// tuning_confidence_test.go 置信度管理面的写路径语义（实体 #63 审计补充）。
//
// 本文件锁住：
//  1. 策略 Upsert 以 policy_id 为业务唯一键：重复提交是改版（版本号服务端递增），不是新增；
//  2. 同意图只保留一条 active（旧版本自动下线），且只影响本租户；
//  3. 信号详情把「不存在」映射成 ErrConfidenceSignalNotFound，供 controller 区分 404 与真实故障。
package service

import (
	"context"
	"errors"
	"testing"

	"hivemtk-user/internal/model"
	pkgdb "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/pkg/tenant"
	"hivemtk-user/internal/pkg/testutil"
)

const (
	tuningTenantA = uint(325)
	tuningTenantB = uint(326)
)

func tuningServiceSetup(t *testing.T) TuningService {
	t.Helper()
	tenant.SetMode(tenant.ModeOn)
	t.Cleanup(func() {
		tenant.ResetRegistryForTest()
		tenant.SetMode(tenant.ModeOff)
	})
	db := testutil.NewTestDB(t, &model.ThresholdPolicy{}, &model.ConfidenceSignal{})
	if db == nil {
		t.Fatal("测试库不可达：tuning 的置信度契约判不了（不 Skip，跳过等于没锁）")
	}
	pkgdb.SetTestDB(db)
	return NewTuningService(db)
}

func TestTuningService_UpsertThresholdPolicyIsVersionedAndTenantScoped(t *testing.T) {
	svc := tuningServiceSetup(t)
	ctxA := tenant.WithTenantID(context.Background(), tuningTenantA)
	ctxB := tenant.WithTenantID(context.Background(), tuningTenantB)

	p := &model.ThresholdPolicy{
		PolicyID: "policy_purchase", IntentType: "purchase", BaseThreshold: 0.61, Version: 1,
	}
	if err := svc.UpsertThresholdPolicy(ctxA, p); err != nil {
		t.Fatalf("A 首次保存策略失败: %v", err)
	}

	// 同一 policy_id 再提交 = 改版：版本号 +1，不新增第二行。
	p2 := &model.ThresholdPolicy{
		PolicyID: "policy_purchase", IntentType: "purchase", BaseThreshold: 0.66, Version: 1,
	}
	if err := svc.UpsertThresholdPolicy(ctxA, p2); err != nil {
		t.Fatalf("A 改版失败: %v", err)
	}
	if p2.Version != 2 {
		t.Errorf("改版后版本号应为 2，实际 %d", p2.Version)
	}

	// 换一个 policy_id 发布同意图新策略：旧版本必须下线。
	if err := svc.UpsertThresholdPolicy(ctxA, &model.ThresholdPolicy{
		PolicyID: "policy_purchase_v3", IntentType: "purchase", BaseThreshold: 0.70, Version: 1,
	}); err != nil {
		t.Fatalf("A 发布新策略失败: %v", err)
	}

	listA, err := svc.ListThresholdPolicies(ctxA)
	if err != nil {
		t.Fatalf("A 列策略失败: %v", err)
	}
	if len(listA) != 1 || listA[0].PolicyID != "policy_purchase_v3" {
		t.Fatalf("A 同意图只应剩一条 active，实际 %+v", listA)
	}

	// 另一租户独立配置，看得见的仍只有自己的策略。
	if err := svc.UpsertThresholdPolicy(ctxB, &model.ThresholdPolicy{
		PolicyID: "policy_purchase", IntentType: "purchase", BaseThreshold: 0.55, Version: 1,
	}); err != nil {
		t.Fatalf("B 保存策略失败: %v", err)
	}
	listB, err := svc.ListThresholdPolicies(ctxB)
	if err != nil {
		t.Fatalf("B 列策略失败: %v", err)
	}
	if len(listB) != 1 || listB[0].PolicyID != "policy_purchase" || listB[0].BaseThreshold != 0.55 {
		t.Fatalf("B 只该看见自己的策略，实际 %+v", listB)
	}
}

func TestTuningService_GetConfidenceSignalTranslatesNotFound(t *testing.T) {
	svc := tuningServiceSetup(t)
	ctx := tenant.WithTenantID(context.Background(), tuningTenantA)

	if _, err := svc.GetConfidenceSignal(ctx, "999999"); !errors.Is(err, ErrConfidenceSignalNotFound) {
		t.Fatalf("不存在的信号应映射为 ErrConfidenceSignalNotFound，实际 %v", err)
	}
}
