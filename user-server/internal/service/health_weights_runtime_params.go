package service

import (
	"context"
	"time"
)

// 域名健康度 与 RAG 健康度的参数（阶段 3.1 第四批，对应 config_params 的
// domain_health 与 rag_health 两组）。
//
// 这两组是"看一眼分数就做决定"的类型：域名侧决定什么时候把流量切走（score 低于阈值
// 或连续失败次数到顶），RAG 侧决定六个维度各占多少比重。改错的结果不是性能问题，
// 而是"该切的没切"或"总分被某一维带偏"。
//
// 走直连不造 provider seam：读取点全在 internal/service 包内（3.1 第一批
// geo.default_visibility_days 定的判据）。每键拆 raw / 带兜底两个函数的理由同
// webhook_runtime_params.go：读取点门禁认的是「某行同时出现 Get*( 与键名常量」。

const (
	// ParamGroupDomainHealth / ParamGroupRagHealth 是两组在参数中心里的组名。
	ParamGroupDomainHealth = "domain_health"
	ParamGroupRagHealth    = "rag_health"

	DomainHealthScoreSwitchThresholdParam    = "score_switch_threshold"
	DomainHealthScoreHealthyParam            = "score_healthy"
	DomainHealthConsecutiveFailureLimitParam = "consecutive_failure_limit"
	DomainHealthCheckTimeoutParam            = "check_timeout"

	RagHealthDefaultWindowParam     = "default_window"
	RagHealthWeightRetrievalParam   = "weight_retrieval"
	RagHealthWeightRecallParam      = "weight_recall"
	RagHealthWeightEmbeddingParam   = "weight_embedding"
	RagHealthWeightCoverageParam    = "weight_coverage"
	RagHealthWeightPerformanceParam = "weight_performance"
	RagHealthWeightAlertsParam      = "weight_alerts"
)

func positiveIntOrDH(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func positiveDurationOrDH(v, def time.Duration) time.Duration {
	if v > 0 {
		return v
	}
	return def
}

// positiveFloatOrDH 浮点参数：0 与负数都当"没配过"。权重组里 0 的语义是
// "这个维度不计分"，那不是运营想要的调参方式（想降比重应该调小，不是清零），
// 清零只会让总分少一块而看不出原因。
func positiveFloatOrDH(v, def float64) float64 {
	if v > 0 {
		return v
	}
	return def
}

// ── 域名健康度 ──

func domainHealthScoreSwitchThresholdRaw(ctx context.Context) int {
	return GlobalConfigParam().GetInt(ctx, ParamGroupDomainHealth, DomainHealthScoreSwitchThresholdParam, 0)
}

func domainHealthScoreHealthyRaw(ctx context.Context) int {
	return GlobalConfigParam().GetInt(ctx, ParamGroupDomainHealth, DomainHealthScoreHealthyParam, 0)
}

func domainHealthConsecutiveFailureLimitRaw(ctx context.Context) int {
	return GlobalConfigParam().GetInt(ctx, ParamGroupDomainHealth, DomainHealthConsecutiveFailureLimitParam, 0)
}

func domainHealthCheckTimeoutRaw(ctx context.Context) time.Duration {
	return GlobalConfigParam().GetDuration(ctx, ParamGroupDomainHealth, DomainHealthCheckTimeoutParam, 0)
}

func domainHealthScoreSwitchThreshold(ctx context.Context) int {
	return positiveIntOrDH(domainHealthScoreSwitchThresholdRaw(ctx), HealthScoreSwitchThreshold)
}

func domainHealthScoreHealthy(ctx context.Context) int {
	return positiveIntOrDH(domainHealthScoreHealthyRaw(ctx), HealthScoreHealthy)
}

func domainHealthConsecutiveFailureLimit(ctx context.Context) int {
	return positiveIntOrDH(domainHealthConsecutiveFailureLimitRaw(ctx), ConsecutiveFailureLimit)
}

// domainHealthCheckTimeout 域名探测超时。http.Client 在构造函数里定型，
// 改完只对下次新建的服务实例生效，故种子标 Restart=true。
func domainHealthCheckTimeout(ctx context.Context) time.Duration {
	return positiveDurationOrDH(domainHealthCheckTimeoutRaw(ctx), HealthCheckTimeout)
}

// ── RAG 健康度 ──

func ragHealthDefaultWindowRaw(ctx context.Context) time.Duration {
	return GlobalConfigParam().GetDuration(ctx, ParamGroupRagHealth, RagHealthDefaultWindowParam, 0)
}

func ragHealthDefaultWindow(ctx context.Context) time.Duration {
	return positiveDurationOrDH(ragHealthDefaultWindowRaw(ctx), RagHealthDefaultWindow)
}

func ragHealthWeightRetrievalRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightRetrievalParam, 0)
}

func ragHealthWeightRecallRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightRecallParam, 0)
}

func ragHealthWeightEmbeddingRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightEmbeddingParam, 0)
}

func ragHealthWeightCoverageRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightCoverageParam, 0)
}

func ragHealthWeightPerformanceRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightPerformanceParam, 0)
}

func ragHealthWeightAlertsRaw(ctx context.Context) float64 {
	return GlobalConfigParam().GetFloat(ctx, ParamGroupRagHealth, RagHealthWeightAlertsParam, 0)
}

// ragHealthWeights 六个维度的权重，**按总和归一化后**返回。
//
// 归一化是必需的：report.Score 是六个 WeightedScore 直接相加再取整
// （rag_health.go:132-136），权重若不可归一化，运营把某一项调成 3 就会让总分超出
// 0-100、等级表（scoreToGrade 的 90/75/60 分档）全部错位。归一化后每个旋钮的语义
// 就是干净的"相对比重"。
//
// 总和 ≤ 0（六项全读不到、或全被写成 0）时回落常量权重——那时 report.Weight 会是
// 0，任何维度看起来都是 0 分，比百分比失准更难排查。
func ragHealthWeights(ctx context.Context) (retrieval, recall, embedding, coverage, performance, alerts float64) {
	retrieval = positiveFloatOrDH(ragHealthWeightRetrievalRaw(ctx), RagHealthWeightRetrieval)
	recall = positiveFloatOrDH(ragHealthWeightRecallRaw(ctx), RagHealthWeightRecall)
	embedding = positiveFloatOrDH(ragHealthWeightEmbeddingRaw(ctx), RagHealthWeightEmbedding)
	coverage = positiveFloatOrDH(ragHealthWeightCoverageRaw(ctx), RagHealthWeightCoverage)
	performance = positiveFloatOrDH(ragHealthWeightPerformanceRaw(ctx), RagHealthWeightPerformance)
	alerts = positiveFloatOrDH(ragHealthWeightAlertsRaw(ctx), RagHealthWeightAlerts)

	sum := retrieval + recall + embedding + coverage + performance + alerts
	if sum <= 0 {
		return RagHealthWeightRetrieval, RagHealthWeightRecall, RagHealthWeightEmbedding,
			RagHealthWeightCoverage, RagHealthWeightPerformance, RagHealthWeightAlerts
	}
	return retrieval / sum, recall / sum, embedding / sum,
		coverage / sum, performance / sum, alerts / sum
}
