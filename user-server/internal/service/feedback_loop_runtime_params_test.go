package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	feedbackloop "hivemtk-user/internal/service/feedback_loop"
)

func newFeedbackParamFixture(t *testing.T) (*ConfigParamService, *gorm.DB) {
	t.Helper()
	database := testutil.NewTestDBOrSkip(t, &model.ConfigParam{}, &model.ConfigParamAuditLog{})
	if database == nil {
		t.Skip("no test DB")
	}
	if err := SeedConfigParams(context.Background(), database); err != nil {
		t.Fatalf("SeedConfigParams 失败：%v", err)
	}
	svc := NewConfigParamService(database)
	SetGlobalForTest(svc)
	t.Cleanup(func() { SetGlobalForTest(nil) })
	return svc, database
}

// TestFeedbackConfigsFromParamsMatchDefaults 无库时五个组件的配置应与各自的
// DefaultXxxConfig() 逐字段一致——这是"参数缺省时行为不变"的总闸。
func TestFeedbackConfigsFromParamsMatchDefaults(t *testing.T) {
	SetGlobalForTest(nil)
	ctx := context.Background()

	if got, want := BanditConfigFromParams(ctx), feedbackloop.DefaultBanditConfig(); got != want {
		t.Errorf("Bandit 配置应逐字段等于默认值:\n got %+v\nwant %+v", got, want)
	}
	gotC := FeedbackCollectorConfigFromParams(ctx)
	wantC := feedbackloop.DefaultFeedbackCollectorConfig()
	// FeedbackCollectorConfig 含 SignalWeightMap（map 不可比较），逐字段比。
	if gotC.QueueSize != wantC.QueueSize || gotC.FlushInterval != wantC.FlushInterval ||
		gotC.BatchSize != wantC.BatchSize || len(gotC.Weights) != len(wantC.Weights) {
		t.Errorf("Collector 配置应逐字段等于默认值:\n got %+v\nwant %+v", gotC, wantC)
	}
	if got, want := ChampionAnalyzerConfigFromParams(ctx), feedbackloop.DefaultChampionAnalyzerConfig(); got != want {
		t.Errorf("Analyzer 配置应逐字段等于默认值:\n got %+v\nwant %+v", got, want)
	}
	if got, want := PromptIteratorConfigFromParams(ctx), feedbackloop.DefaultPromptIteratorConfig(); got != want {
		t.Errorf("Iterator 配置应逐字段等于默认值:\n got %+v\nwant %+v", got, want)
	}
	if got, want := SOPAutoOptimizerConfigFromParams(ctx), feedbackloop.DefaultSOPAutoOptimizerConfig(); got != want {
		t.Errorf("Optimizer 配置应逐字段等于默认值:\n got %+v\nwant %+v", got, want)
	}
}

// TestFeedbackConfigsReadParamCenter 22 条参数真的从库里读，逐个字段验证。
func TestFeedbackConfigsReadParamCenter(t *testing.T) {
	svc, _ := newFeedbackParamFixture(t)
	ctx := context.Background()

	set := func(key, val string) {
		t.Helper()
		if err := svc.UpdateValue(ctx, ParamGroupFeedback, key, val, 1); err != nil {
			t.Fatalf("UpdateValue(feedback.%s=%s) 失败：%v", key, val, err)
		}
	}

	// ── bandit 6 条 ──
	set(FBBanditMinSamplesForExploit, "7")
	set(FBBanditExplorationFloor, "0.25")
	set(FBBanditTrafficCeiling, "0.4")
	set(FBBanditConvergenceThreshold, "0.9")
	set(FBBanditMinSamplesForPromote, "200")
	set(FBBanditPosteriorSamples, "500")
	b := BanditConfigFromParams(ctx)
	if b.MinSamplesForExploit != 7 || b.ExplorationFloor != 0.25 || b.TrafficCeiling != 0.4 ||
		b.ConvergenceThreshold != 0.9 || b.MinSamplesForPromote != 200 || b.PosteriorSamples != 500 {
		t.Errorf("bandit 配置未按参数生效: %+v", b)
	}

	// ── collector 3 条 ──
	set(FBCollectorQueueSize, "333")
	set(FBCollectorFlushInterval, "9")
	set(FBCollectorBatchSize, "11")
	c := FeedbackCollectorConfigFromParams(ctx)
	if c.QueueSize != 333 || c.FlushInterval != 9*time.Second || c.BatchSize != 11 {
		t.Errorf("collector 配置未按参数生效: %+v", c)
	}
	// Weights 刻意不入库：应与默认权重表逐键相等。
	if len(c.Weights) != len(feedbackloop.DefaultSignalWeights) {
		t.Errorf("Weights 应仍为默认权重表（刻意不入库），got %d 键", len(c.Weights))
	}

	// ── analyzer 5 条 ──
	set(FBAnalyzerMinReward, "2.5")
	set(FBAnalyzerClusterSimThreshold, "0.9")
	set(FBAnalyzerMinClusterSize, "5")
	set(FBAnalyzerTopKPerCluster, "8")
	set(FBAnalyzerMaxDialoguesPerRun, "900")
	a := ChampionAnalyzerConfigFromParams(ctx)
	if a.MinReward != 2.5 || a.ClusterSimThreshold != 0.9 || a.MinClusterSize != 5 ||
		a.TopKPerCluster != 8 || a.MaxDialoguesPerRun != 900 {
		t.Errorf("analyzer 配置未按参数生效: %+v", a)
	}

	// ── iterator 4 条 ──
	set(FBIteratorMinSamplesForIteration, "80")
	set(FBIteratorNegativeRewardThreshold, "-1.5")
	set(FBIteratorCandidatesPerRun, "6")
	set(FBIteratorAutoApprove, "true")
	i := PromptIteratorConfigFromParams(ctx)
	if i.MinSamplesForIteration != 80 || i.NegativeRewardThreshold != -1.5 ||
		i.CandidatesPerRun != 6 || !i.AutoApprove {
		t.Errorf("iterator 配置未按参数生效: %+v", i)
	}

	// ── optimizer 4 条 ──
	set(FBOptimizerAutoApplyPriority, "1")
	set(FBOptimizerRollbackDropThreshold, "0.1")
	set(FBOptimizerRollbackComplaintRatio, "0.3")
	set(FBOptimizerABTestDuration, "86400")
	o := SOPAutoOptimizerConfigFromParams(ctx)
	if o.AutoApplyPriority != 1 || o.RollbackDropThreshold != 0.1 ||
		o.RollbackComplaintRatio != 0.3 || o.ABTestDuration != 24*time.Hour {
		t.Errorf("optimizer 配置未按参数生效: %+v", o)
	}
}

// TestFeedbackFloatFallbackSemantics 两类 float 兜底语义不同，必须分开钉：
//   - fbOrPosFloat（exploration_floor 等正值参数）：0 回落默认值。
//   - fbOrAnyFloat（negative_reward_threshold，默认负数）：0 是合法语义
//     「关掉这道门限」，不是「没配过」，不该回落。
func TestFeedbackFloatFallbackSemantics(t *testing.T) {
	if got := fbOrPosFloat(0, 0.10); got != 0.10 {
		t.Errorf("fbOrPosFloat(0) 应回落, got %v", got)
	}
	if got := fbOrPosFloat(-1, 0.10); got != 0.10 {
		t.Errorf("fbOrPosFloat(-1) 应回落, got %v", got)
	}
	if got := fbOrPosFloat(0.25, 0.10); got != 0.25 {
		t.Errorf("fbOrPosFloat(0.25) 应透传, got %v", got)
	}
	if got := fbOrAnyFloat(0, -0.5); got != 0 {
		t.Errorf("fbOrAnyFloat(0) 应透传 0（= 关掉门限）, got %v", got)
	}
	// NaN 哨兵只在 fallback 路上出现：库里存不下 NaN（validateValue 走
	// strconv.ParseFloat），所以它唯一来源就是「这条参数没配过」。
	if got := fbOrAnyFloat(fbMissingFloat, -0.5); got != -0.5 {
		t.Errorf("fbOrAnyFloat(NaN) 应回落默认 -0.5, got %v", got)
	}
	if got := fbOrAnyFloat(-1.5, -0.5); got != -1.5 {
		t.Errorf("fbOrAnyFloat(-1.5) 应透传, got %v", got)
	}
	if got := fbOrInt(0, 30); got != 30 {
		t.Errorf("fbOrInt(0) 应回落, got %v", got)
	}
	if got := fbOrDur(0, 2*time.Second); got != 2*time.Second {
		t.Errorf("fbOrDur(0) 应回落, got %v", got)
	}
}

// TestFeedbackSeedsMatchConfigDefaults 22 条种子的默认值必须等于 DefaultXxxConfig()。
func TestFeedbackSeedsMatchConfigDefaults(t *testing.T) {
	d := map[string]string{
		FBBanditMinSamplesForExploit:      "30",
		FBBanditExplorationFloor:          "0.10",
		FBBanditTrafficCeiling:            "0.60",
		FBBanditConvergenceThreshold:      "0.95",
		FBBanditMinSamplesForPromote:      "100",
		FBBanditPosteriorSamples:          "1000",
		FBCollectorQueueSize:              "1000",
		FBCollectorFlushInterval:          "2",
		FBCollectorBatchSize:              "50",
		FBAnalyzerMinReward:               "1",
		FBAnalyzerClusterSimThreshold:     "0.85",
		FBAnalyzerMinClusterSize:          "3",
		FBAnalyzerTopKPerCluster:          "3",
		FBAnalyzerMaxDialoguesPerRun:      "500",
		FBIteratorMinSamplesForIteration:  "50",
		FBIteratorNegativeRewardThreshold: "-0.5",
		FBIteratorCandidatesPerRun:        "3",
		FBIteratorAutoApprove:             "false",
		FBOptimizerAutoApplyPriority:      "2",
		FBOptimizerRollbackDropThreshold:  "0.20",
		FBOptimizerRollbackComplaintRatio: "0.50",
		FBOptimizerABTestDuration:         "604800",
	}
	index := make(map[string]ParamDef, len(d))
	for _, def := range DefaultParamDefs() {
		if def.Group == ParamGroupFeedback {
			index[def.Key] = def
		}
	}
	if len(index) != len(d) {
		t.Fatalf("feedback 组应有 %d 条种子, 实际 %d 条", len(d), len(index))
	}
	for key, def := range index {
		want, ok := d[key]
		if !ok {
			t.Errorf("feedback 组多出一条未登记的种子: %s", key)
			continue
		}
		if def.DefaultValue != want {
			t.Errorf("feedback.%s 种子默认值 %q != 代码兜底 %q", key, def.DefaultValue, want)
		}
		if !def.Restart {
			t.Errorf("feedback.%s 是构造期读，必须标 Restart=true", key)
		}
	}
}

// TestFeedbackParamKeysAreAllRead 反向钉死「登记了必须有人读」：22 条逐一改库后
// 断言对应字段真的变了。
func TestFeedbackParamKeysAreAllRead(t *testing.T) {
	svc, _ := newFeedbackParamFixture(t)
	ctx := context.Background()
	// 注意逐条用各自合法区间内的值：bandit_posterior_samples 的 Min 是 10，
	// 写 1 会被 service 的 min/max 守卫拒掉（那是 0.2 修的那个口子在生效）。
	// bandit_posterior_samples 的 Min 是 10，写 1 会被 service 的 min/max 守卫拒掉
	// （那是 0.2 修的那个口子在生效）；其余 21 条的合法区间都含 1。
	// optimizer_ab_test_duration 的 Min 是 3600s，同样写 1 会被拒。
	perKey := map[string]string{
		FBBanditPosteriorSamples:  "10",
		FBOptimizerABTestDuration: "3600",
		// 负向门限的合法区间是 [-100, 0]，0 = 关掉门限是其合法语义
		FBIteratorNegativeRewardThreshold: "-1",
	}
	for _, k := range []string{
		FBBanditMinSamplesForExploit, FBBanditExplorationFloor, FBBanditTrafficCeiling,
		FBBanditConvergenceThreshold, FBBanditMinSamplesForPromote,
		FBCollectorQueueSize, FBCollectorFlushInterval, FBCollectorBatchSize,
		FBAnalyzerMinReward, FBAnalyzerClusterSimThreshold, FBAnalyzerMinClusterSize,
		FBAnalyzerTopKPerCluster, FBAnalyzerMaxDialoguesPerRun,
		FBIteratorMinSamplesForIteration,
		FBIteratorCandidatesPerRun,
		FBOptimizerAutoApplyPriority, FBOptimizerRollbackDropThreshold,
		FBOptimizerRollbackComplaintRatio,
	} {
		perKey[k] = "1"
	}
	for k, def := range perKey {
		if def == "" {
			def = "1"
		}
		if err := svc.UpdateValue(ctx, ParamGroupFeedback, k, def, 1); err != nil {
			t.Fatalf("UpdateValue(feedback.%s=%s) 失败：%v", k, def, err)
		}
	}
	if err := svc.UpdateValue(ctx, ParamGroupFeedback, FBIteratorAutoApprove, "true", 1); err != nil {
		t.Fatalf("UpdateValue(feedback.%s=true) 失败：%v", FBIteratorAutoApprove, err)
	}

	b := BanditConfigFromParams(ctx)
	c := FeedbackCollectorConfigFromParams(ctx)
	a := ChampionAnalyzerConfigFromParams(ctx)
	i := PromptIteratorConfigFromParams(ctx)
	o := SOPAutoOptimizerConfigFromParams(ctx)

	for name, cse := range map[string]struct{ got, want int }{
		"bandit_min_samples_for_exploit":     {b.MinSamplesForExploit, 1},
		"bandit_min_samples_for_promote":     {b.MinSamplesForPromote, 1},
		"bandit_posterior_samples":           {b.PosteriorSamples, 10}, // Min 10，见 perKey
		"collector_queue_size":               {c.QueueSize, 1},
		"collector_batch_size":               {c.BatchSize, 1},
		"analyzer_min_cluster_size":          {a.MinClusterSize, 1},
		"analyzer_top_k_per_cluster":         {a.TopKPerCluster, 1},
		"analyzer_max_dialogues_per_run":     {a.MaxDialoguesPerRun, 1},
		"iterator_min_samples_for_iteration": {i.MinSamplesForIteration, 1},
		"iterator_candidates_per_run":        {i.CandidatesPerRun, 1},
		"optimizer_auto_apply_priority":      {o.AutoApplyPriority, 1},
	} {
		if cse.got != cse.want {
			t.Errorf("feedback.%s 应读到 %d, got %d", name, cse.want, cse.got)
		}
	}
	for name, got := range map[string]float64{
		"bandit_exploration_floor":           b.ExplorationFloor,
		"bandit_traffic_ceiling":             b.TrafficCeiling,
		"bandit_convergence_threshold":       b.ConvergenceThreshold,
		"analyzer_min_reward":                a.MinReward,
		"analyzer_cluster_sim_threshold":     a.ClusterSimThreshold,
		"optimizer_rollback_drop_threshold":  o.RollbackDropThreshold,
		"optimizer_rollback_complaint_ratio": o.RollbackComplaintRatio,
	} {
		if got != 1 {
			t.Errorf("feedback.%s 应读到 1, got %v", name, got)
		}
	}
	if c.FlushInterval != time.Second {
		t.Errorf("feedback.collector_flush_interval 应读到 1s, got %v", c.FlushInterval)
	}
	if o.ABTestDuration != time.Hour {
		t.Errorf("feedback.optimizer_ab_test_duration 应读到 3600s, got %v", o.ABTestDuration)
	}
	if i.NegativeRewardThreshold != -1 {
		t.Errorf("feedback.iterator_negative_reward_threshold 应读到 -1, got %v", i.NegativeRewardThreshold)
	}
	if !i.AutoApprove {
		t.Errorf("feedback.iterator_auto_approve 应读到 true")
	}
}
