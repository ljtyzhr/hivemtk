package confidence

import (
	"hivemtk-user/internal/dto"
)

// SignalWeights 5 维信号权重
type SignalWeights struct {
	IntentConf float64
	EntityComp float64
	CtxRelev   float64
	RAGQual    float64
	LLMEntropy float64
}

// DefaultSignalWeights 默认权重（来自设计文档 §15.2.2）
func DefaultSignalWeights() SignalWeights {
	return SignalWeights{
		IntentConf: 0.30,
		EntityComp: 0.15,
		CtxRelev:   0.15,
		RAGQual:    0.20,
		LLMEntropy: 0.20,
	}
}

// WeightedAggregator 5 维信号加权聚合器
type WeightedAggregator struct {
	weights SignalWeights
}

// NewWeightedAggregator 创建聚合器
//
// 自动归一化权重，使 Σw_i = 1
func NewWeightedAggregator(w SignalWeights) *WeightedAggregator {
	normalizeWeights(&w)
	return &WeightedAggregator{weights: w}
}

// NewDefaultWeightedAggregator 使用默认权重创建聚合器
func NewDefaultWeightedAggregator() *WeightedAggregator {
	return NewWeightedAggregator(DefaultSignalWeights())
}

// Aggregate 加权聚合
//
// signals 为 nil 时返回 0
// 公式：conf = Σ w_i * signal_i
//
// **缺失的信号不参与加权**（signals.Available 标出哪些真取到了证据）：
// 采集器在拿不到证据时给的是 0.5 中性占位值 —— 那是"这里没数据"，不是"中等证据"。
// 把占位值当实打实的分数算，等于凭空给这一维 0.5 的信用/扣分：首轮对话里
// ctx_relev（没历史）与 llm_entropy（没 logprobs）两项占 35% 权重，
// 把总置信度按在阈值之下，官网客服对正常问题一律转人工。
//
// 口径：不可用信号的权重从分母里去掉，其余信号按比例放大；
// 可用信号一个都没有时退回等权平均（不返回 0 —— 0 会被判成"完全不可信"而直接转人工）。
func (a *WeightedAggregator) Aggregate(signals *dto.FiveSignals) float64 {
	if signals == nil {
		return 0
	}
	avail := signals.Available
	if avail.IsZero() {
		// 未标可用性（零值）= 全部可用，与本字段引入前的行为逐字一致。
		avail = dto.AllAvailable()
	}

	weighted := 0.0
	weightSum := 0.0
	plainSum := 0.0
	plainCount := 0

	add := func(weight, value float64, usable bool) {
		plainSum += value
		plainCount++
		if !usable {
			return
		}
		weighted += weight * value
		weightSum += weight
	}

	add(a.weights.IntentConf, signals.IntentConf, avail.IntentConf)
	add(a.weights.EntityComp, signals.EntityComp, avail.EntityComp)
	add(a.weights.CtxRelev, signals.CtxRelev, avail.CtxRelev)
	add(a.weights.RAGQual, signals.RAGQual, avail.RAGQual)
	add(a.weights.LLMEntropy, signals.LLMEntropy, avail.LLMEntropy)

	if weightSum <= 0 {
		// 一个可用信号都没有：退回等权平均。
		if plainCount == 0 {
			return 0
		}
		return clamp01(plainSum / float64(plainCount))
	}
	return clamp01(weighted / weightSum)
}

// UpdateWeights 热更新权重（运营后台调参）
//
// 自动归一化
func (a *WeightedAggregator) UpdateWeights(w SignalWeights) {
	normalizeWeights(&w)
	a.weights = w
}

// Weights 返回当前权重（只读）
func (a *WeightedAggregator) Weights() SignalWeights {
	return a.weights
}

func normalizeWeights(w *SignalWeights) {
	sum := w.IntentConf + w.EntityComp + w.CtxRelev + w.RAGQual + w.LLMEntropy
	if sum > 0 {
		w.IntentConf /= sum
		w.EntityComp /= sum
		w.CtxRelev /= sum
		w.RAGQual /= sum
		w.LLMEntropy /= sum
	}
}
