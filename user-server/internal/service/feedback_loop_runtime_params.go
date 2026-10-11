package service

import (
	"context"
	"math"
	"time"

	feedbackloop "hivemtk-user/internal/service/feedback_loop"
)

// 反馈学习闭环的参数（阶段 3.1 第五批，对应 config_params 的 feedback 组）。
//
// 22 条全部是**构造期读一次**：五个组件的 DefaultXxxConfig() 在
// InitFeedbackLoopComponents / InitFeedbackCollector（cmd/api/main.go:408/418）里
// 被调用，读完成果就固化在各组件的 config 字段里。所以全部标 Restart=true。
//
// 走直连而不是 provider seam：配置在 internal/service/feedback_loop_init.go 里组装，
// 那个包「不能」反向依赖 feedback_loop 的读取点以外的任何东西——真正的常量在
// feedback_loop/types.go 里，而 feedback_loop 不能 import internal/service（会把
// 组件包和装配包缠成环）。所以读取点放在 internal/service 侧，正是 3.1a 定的判据。
//
// 每条的 Get*( 与键名常量写在同一行：读取点门禁（scripts/
// check-config-param-readpoints.py）认的就是这个形状，读点藏进 helper 形参里它判不出来
// （3.1b 在 webhook 批踩过一次，6 条一起被判 UNDECLARED）。

// ParamGroupFeedback 是这一组在参数中心里的组名。
const ParamGroupFeedback = "feedback"

// 22 个键。抽成常量而不是在调用点写字面量：1.8 栽过一次——channelgw/ws.go 把种子
// 登记的 "ws_push_interval" 写成 "ws_push_interval_default"，每次读都 miss、每次回落
// 兜底，而读取点门禁结构上看不出这类漂移（读取点存在，只是用的别的 key）。
const (
	FBBanditMinSamplesForExploit = "bandit_min_samples_for_exploit"
	FBBanditExplorationFloor     = "bandit_exploration_floor"
	FBBanditTrafficCeiling       = "bandit_traffic_ceiling"
	FBBanditConvergenceThreshold = "bandit_convergence_threshold"
	FBBanditMinSamplesForPromote = "bandit_min_samples_for_promote"
	FBBanditPosteriorSamples     = "bandit_posterior_samples"

	FBCollectorQueueSize     = "collector_queue_size"
	FBCollectorFlushInterval = "collector_flush_interval"
	FBCollectorBatchSize     = "collector_batch_size"

	FBAnalyzerMinReward           = "analyzer_min_reward"
	FBAnalyzerClusterSimThreshold = "analyzer_cluster_sim_threshold"
	FBAnalyzerMinClusterSize      = "analyzer_min_cluster_size"
	FBAnalyzerTopKPerCluster      = "analyzer_top_k_per_cluster"
	FBAnalyzerMaxDialoguesPerRun  = "analyzer_max_dialogues_per_run"

	FBIteratorMinSamplesForIteration  = "iterator_min_samples_for_iteration"
	FBIteratorNegativeRewardThreshold = "iterator_negative_reward_threshold"
	FBIteratorCandidatesPerRun        = "iterator_candidates_per_run"
	FBIteratorAutoApprove             = "iterator_auto_approve"

	FBOptimizerAutoApplyPriority      = "optimizer_auto_apply_priority"
	FBOptimizerRollbackDropThreshold  = "optimizer_rollback_drop_threshold"
	FBOptimizerRollbackComplaintRatio = "optimizer_rollback_complaint_ratio"
	FBOptimizerABTestDuration         = "optimizer_ab_test_duration"
)

// fbMissingFloat 是「这条参数没配过」的哨兵值。GetFloat 的 fallback 语义是
// 「读不到就返回 fallback」，所以对一个合法取值可以是 0 的参数（如
// iterator_negative_reward_threshold 填 0 = 关掉门限），用 0 当 fallback 就没法
// 区分「没配过」和「配了 0」——两者都会落到兜底值上，配了 0 的人会困惑
// 「我明明填了 0 为什么不生效」。改用 NaN：库里不可能存下 NaN
// （validateValue 走 strconv.ParseFloat，NaN 字面量解析失败），所以它只在
// fallback 这一条路上出现。
var fbMissingFloat = math.NaN()

// fbOrInt / fbOrPosFloat / fbOrAnyFloat / fbOrDur：读到的值缺省或越界时回落默认值。
//
// 一律回落而不是「配 0 就照做」：这一组里 0 在每种语义下都是写坏了——
// bandit 的样本量 0 = 永远判定样本不足、探索下限 0 = 完全不做探索、
// 采集批量 0 = 每来一条事件刷一次盘、AB 时长 0 = 实验立即结束。
// 唯一的例外是 iterator_negative_reward_threshold（默认 -0.5，负数），
// 用 fbOrAnyFloat：0 的语义是「关掉负向门限」，不是「没配过」。
func fbOrInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func fbOrPosFloat(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
}

// fbOrAnyFloat 只把「没配过」当缺省：v 是 NaN（哨兵）时回落 def，其余一律透传。
// 用于合法取值可以是 0 或负数的参数。
func fbOrAnyFloat(v, def float64) float64 {
	if math.IsNaN(v) {
		return def
	}
	return v
}

func fbOrDur(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

// BanditConfigFromParams 按参数中心组装 bandit 配置。采样量、探索下限、流量上限、
// 收敛阈值、晋升样本量、后验采样数六项；读不到或写坏时逐项回落 DefaultBanditConfig()。
func BanditConfigFromParams(ctx context.Context) feedbackloop.BanditConfig {
	d := feedbackloop.DefaultBanditConfig()
	d.MinSamplesForExploit = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBBanditMinSamplesForExploit, 0), d.MinSamplesForExploit)
	d.ExplorationFloor = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBBanditExplorationFloor, 0), d.ExplorationFloor)
	d.TrafficCeiling = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBBanditTrafficCeiling, 0), d.TrafficCeiling)
	d.ConvergenceThreshold = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBBanditConvergenceThreshold, 0), d.ConvergenceThreshold)
	d.MinSamplesForPromote = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBBanditMinSamplesForPromote, 0), d.MinSamplesForPromote)
	d.PosteriorSamples = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBBanditPosteriorSamples, 0), d.PosteriorSamples)
	return d
}

// FeedbackCollectorConfigFromParams 采集器配置：队列容量、刷盘间隔、批量大小三项。
// Weights（信号权重表）**刻意不入库**：它是一张 12 键的双射表（dto.FeedbackSignalKey →
// float64），改成 12 条参数后每一行仍是"配了这一格才覆盖一格的权重"，运维要看全貌反而
// 更难。要动它应走字典表（阶段四），不是参数中心。
func FeedbackCollectorConfigFromParams(ctx context.Context) feedbackloop.FeedbackCollectorConfig {
	d := feedbackloop.DefaultFeedbackCollectorConfig()
	d.QueueSize = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBCollectorQueueSize, 0), d.QueueSize)
	d.FlushInterval = fbOrDur(GlobalConfigParam().GetDuration(ctx, ParamGroupFeedback, FBCollectorFlushInterval, 0), d.FlushInterval)
	d.BatchSize = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBCollectorBatchSize, 0), d.BatchSize)
	return d
}

// ChampionAnalyzerConfigFromParams 销冠对话分析器配置五项。
func ChampionAnalyzerConfigFromParams(ctx context.Context) feedbackloop.ChampionAnalyzerConfig {
	d := feedbackloop.DefaultChampionAnalyzerConfig()
	d.MinReward = fbOrAnyFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBAnalyzerMinReward, fbMissingFloat), d.MinReward)
	d.ClusterSimThreshold = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBAnalyzerClusterSimThreshold, 0), d.ClusterSimThreshold)
	d.MinClusterSize = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBAnalyzerMinClusterSize, 0), d.MinClusterSize)
	d.TopKPerCluster = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBAnalyzerTopKPerCluster, 0), d.TopKPerCluster)
	d.MaxDialoguesPerRun = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBAnalyzerMaxDialoguesPerRun, 0), d.MaxDialoguesPerRun)
	return d
}

// PromptIteratorConfigFromParams Prompt 迭代器配置四项。
// iterator_auto_approve 是布尔：GetBool 的 fallback 本身就是 DefaultPromptIteratorConfig()
// 的值（false），库里有值就返回库里的，没有歧义——布尔参数的歧义只在「默认 true」时出现。
func PromptIteratorConfigFromParams(ctx context.Context) feedbackloop.PromptIteratorConfig {
	d := feedbackloop.DefaultPromptIteratorConfig()
	d.MinSamplesForIteration = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBIteratorMinSamplesForIteration, 0), d.MinSamplesForIteration)
	d.NegativeRewardThreshold = fbOrAnyFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBIteratorNegativeRewardThreshold, fbMissingFloat), d.NegativeRewardThreshold)
	d.CandidatesPerRun = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBIteratorCandidatesPerRun, 0), d.CandidatesPerRun)
	d.AutoApprove = GlobalConfigParam().GetBool(ctx, ParamGroupFeedback, FBIteratorAutoApprove, d.AutoApprove)
	return d
}

// SOPAutoOptimizerConfigFromParams 自动优化器配置四项。
// ab_test_duration 的默认是 7*24h，duration 参数只存秒数，装配层换算后传进来。
func SOPAutoOptimizerConfigFromParams(ctx context.Context) feedbackloop.SOPAutoOptimizerConfig {
	d := feedbackloop.DefaultSOPAutoOptimizerConfig()
	d.AutoApplyPriority = fbOrInt(GlobalConfigParam().GetInt(ctx, ParamGroupFeedback, FBOptimizerAutoApplyPriority, 0), d.AutoApplyPriority)
	d.RollbackDropThreshold = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBOptimizerRollbackDropThreshold, 0), d.RollbackDropThreshold)
	d.RollbackComplaintRatio = fbOrPosFloat(GlobalConfigParam().GetFloat(ctx, ParamGroupFeedback, FBOptimizerRollbackComplaintRatio, 0), d.RollbackComplaintRatio)
	d.ABTestDuration = fbOrDur(GlobalConfigParam().GetDuration(ctx, ParamGroupFeedback, FBOptimizerABTestDuration, 0), d.ABTestDuration)
	return d
}
