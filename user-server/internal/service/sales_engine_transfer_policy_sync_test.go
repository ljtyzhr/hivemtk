package service

// sales_engine_transfer_policy_sync_test.go P0-2 残点 1：双阈值源失同步传导的反向测试。
//
// 判据：改 objection 的 **DB policy 行**后，转人工前置门槛的行为必须跟着变——
// 这正是「门槛读 GetPolicy 单一来源」的可观测后果；若有人把门槛改回独立常量
// （0.7 或任何写死的数），两个方向里至少一个方向会失配而红。
//
// 构造：policy 行经 repository 落库（version=2 > 内置 version=1），引擎按 ctx 懒加载
// 覆盖内置表——与生产运营后台改阈值的路径一致（SaveActive + 失效重载）。

import (
	"context"
	"strings"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
	confidencesvc "hivemtk-user/internal/service/confidence"
)

// objectionPolicyRow 构造一行 objection 策略（默认口径字段照抄内置 policy，仅阈值/版本不同）。
// bandUpper 单独给出：用它把聚合结果钉进 handoff 带，使「进了聚合」变成可见的转人工理由。
func objectionPolicyRow(threshold, bandUpper float64) model.ThresholdPolicy {
	return model.ThresholdPolicy{
		PolicyID:                "policy_objection_custom",
		IntentType:              "objection",
		BaseThreshold:           threshold,
		CustomerLevelWeight:     0.05,
		TimeslotWeight:          0.05,
		AgentAvailabilityWeight: 0.10,
		BandHandoffUpper:        bandUpper,
		BandFallbackUpper:       0.60,
		BandReviewUpper:         0.75,
		ReviewSLASeconds:        30,
		IsActive:                true,
		Version:                 2, // > 内置 version=1 才会覆盖
	}
}

// newAggregatorWithDBPolicies 用 DB 策略仓库装配聚合器（生产路径形状）。
func newAggregatorWithDBPolicies(t *testing.T, rows ...model.ThresholdPolicy) *confidencesvc.ConfidenceAggregator {
	t.Helper()
	db := testutil.NewTestDB(t, &model.ThresholdPolicy{})
	if err := db.Exec("DELETE FROM threshold_policies").Error; err != nil {
		t.Fatalf("清理策略表失败: %v", err)
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatalf("写入策略失败: %v", err)
		}
	}
	engine := confidencesvc.NewThresholdPolicyEngine(repository.NewThresholdPolicyRepositoryWithDB(db))
	return confidencesvc.NewConfidenceAggregator(
		confidencesvc.NewSignalCollector(nil),
		nil,
		confidencesvc.NewWeightedAggregator(confidencesvc.DefaultSignalWeights()),
		confidencesvc.NewVetoChain(),
		confidencesvc.NewDynamicThresholdCalculator(engine),
		nil,
	)
}

// TestTransferGate_FollowsDBPolicy_Objection 双方向：
//
//	A. DB 把 objection 调高到 0.95：0.80 的 objection 置信 **低于** 档位 ⇒ 前置门放行，
//	   进入五信号聚合（聚合侧 nil embedder 下走降级，但进入聚合本身可观测：
//	   返回理由来自聚合分支而非空串，或聚合把低分判成 handoff）。
//	B. DB 把 objection 调低到 0.55：0.80 置信 ≥ 档位 ⇒ 前置门拦截，不聚合。
//
// 两格的差异只能由「门槛读 policy」解释——写死常量的实现无法同时满足 A、B。
func TestTransferGate_FollowsDBPolicy_Objection(t *testing.T) {
	t.Run("调高到0.95_前置门放行并进入聚合", func(t *testing.T) {
		e := &SalesEngine{}
		e.SetConfidenceAggregator(context.Background(), newAggregatorWithDBPolicies(t, objectionPolicyRow(0.95, 0.99)))

		p := e.confidenceAggregator.GetPolicy(context.Background(), "objection")
		if p == nil || p.BaseThreshold != 0.95 {
			t.Fatalf("门槛来源应为 DB 行 0.95，实际 %+v（policy 未被单一来源消费）", p)
		}

		boolRes, reason := e.shouldTransferToHuman(context.Background(), &dto.RecognizeResult{
			IntentType: "objection", Confidence: 0.80,
		}, nil, &SalesRequest{})
		// 0.80 < 0.95 ⇒ 必须进聚合；band 被夹具钉进 handoff（BandHandoffUpper=0.99），
		// 因此聚合分支一定产出转人工 + 非空理由。写死常量的门槛会在前置门空返回。
		if !boolRes || reason == "" {
			t.Errorf("0.80 < 0.95 应通过前置门进入聚合并转人工，实际 (%v,%q)（门槛未读 policy=0.95）", boolRes, reason)
		}
	})

	t.Run("调低到0.55_前置门拦截不聚合", func(t *testing.T) {
		e := &SalesEngine{}
		e.SetConfidenceAggregator(context.Background(), newAggregatorWithDBPolicies(t, objectionPolicyRow(0.55, 0.99)))

		p := e.confidenceAggregator.GetPolicy(context.Background(), "objection")
		if p == nil || p.BaseThreshold != 0.55 {
			t.Fatalf("门槛来源应为 DB 行 0.55，实际 %+v", p)
		}

		boolRes, reason := e.shouldTransferToHuman(context.Background(), &dto.RecognizeResult{
			IntentType: "objection", Confidence: 0.80,
		}, nil, &SalesRequest{})
		if boolRes || reason != "" {
			t.Errorf("0.80 ≥ 0.55 ⇒ 前置门应拦截并返回 (false,\"\")，实际 (%v,%q)（门槛未随 policy 调低）", boolRes, reason)
		}
	})
}

// TestTransferGate_DefaultPolicyBehaviorLocked 残点 2 的行为锁：内置口径下
// objection 0.80 被前置门拦（0.80 ≥ 0.75），reason 为空；且档位读数 = 内置 0.75。
// 该用例与上面两格一起把「默认口径」钉死，防改 policy 表时误伤默认行为。
func TestTransferGate_DefaultPolicyBehaviorLocked(t *testing.T) {
	e := &SalesEngine{}
	e.SetConfidenceAggregator(context.Background(), newMinimalAggregator(t))

	p := e.confidenceAggregator.GetPolicy(context.Background(), "objection")
	if p == nil || p.BaseThreshold != 0.75 {
		t.Fatalf("内置 objection 档应为 0.75，实际 %+v", p)
	}
	should, reason := e.shouldTransferToHuman(context.Background(), &dto.RecognizeResult{
		IntentType: "objection", Confidence: 0.80,
	}, nil, &SalesRequest{})
	if should {
		t.Errorf("默认口径下 0.80 objection 应被前置门拦（不转人工），实际转人工 reason=%q", reason)
	}
	if strings.TrimSpace(reason) != "" {
		t.Errorf("前置门拦截路径 reason 应为空，实际 %q", reason)
	}
}
