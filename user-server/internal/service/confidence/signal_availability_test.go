package confidence

import (
	"testing"

	"hivemtk-user/internal/dto"
)

// 回归：官网右下角网页客服问「介绍下项目」被判「AI 置信度不足 (0.62 < 0.70)」并转人工。
//
// 这一条钉的是**缺失信号的处理**：首轮对话里 ctx_relev（没有历史对话）与
// llm_entropy（没有 logprobs）都拿不到证据，采集器给 0.5 中性占位值。
// 0.5 不是"中等证据"，只是"这里没数据"，但等权聚合把它当实打实的 0.5 算进
// 加权和 —— 两项合计 35% 权重，把总置信度按在 0.7 阈值之下。
//
// 线上真实构成（confidence_signals 表）：
//
//	intent_conf 0.90  entity_comp 1.00  ctx_relev 0.50  rag_qual 0.1344  llm_entropy 0.50
//	= 0.30*0.90 + 0.15*1.00 + 0.15*0.50 + 0.20*0.1344 + 0.20*0.50 = 0.6219
//
// 修完之后：不可用信号不参与加权，其余信号按比例放大。

// TestWeightedAggregator_UnavailableSignalsAreExcluded 不可用信号不参与加权。
func TestWeightedAggregator_UnavailableSignalsAreExcluded(t *testing.T) {
	agg := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 0.90,
		EntityComp: 1.00,
		CtxRelev:   0.50, // 占位：没历史对话
		RAGQual:    0.80,
		LLMEntropy: 0.50, // 占位：没 logprobs
		Available: dto.SignalAvailability{
			Set:        true,
			IntentConf: true,
			EntityComp: true,
			RAGQual:    true,
			// CtxRelev / LLMEntropy 不可用
		},
	}
	// 可用权重 = 0.30 + 0.15 + 0.20 = 0.65
	// 期望 = (0.30*0.90 + 0.15*1.00 + 0.20*0.80) / 0.65 = 0.58/0.65 = 0.892308
	got := agg.Aggregate(signals)
	if !approxEqual(got, 0.58/0.65) {
		t.Errorf("缺失信号不应参与加权，聚合分应 ≈ 0.892308, got %v", got)
	}
}

// TestWeightedAggregator_AllAvailableKeepsLegacyFormula 全部可用时公式与旧版逐字一致。
func TestWeightedAggregator_AllAvailableKeepsLegacyFormula(t *testing.T) {
	agg := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 0.90,
		EntityComp: 1.00,
		CtxRelev:   0.50,
		RAGQual:    0.1344,
		LLMEntropy: 0.50,
		Available:  dto.AllAvailable(),
	}
	want := 0.30*0.90 + 0.15*1.00 + 0.15*0.50 + 0.20*0.1344 + 0.20*0.50
	got := agg.Aggregate(signals)
	if !approxEqual(got, want) {
		t.Errorf("全部可用时应与旧公式一致：want %v, got %v", want, got)
	}
}

// TestWeightedAggregator_ZeroAvailabilityMeansAllAvailable 零值（未设）= 全部可用，
// 保证既有构造代码（不填 Available）行为不变。
func TestWeightedAggregator_ZeroAvailabilityMeansAllAvailable(t *testing.T) {
	agg := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 0.90,
		EntityComp: 1.00,
		CtxRelev:   0.50,
		RAGQual:    0.1344,
		LLMEntropy: 0.50,
	}
	want := 0.30*0.90 + 0.15*1.00 + 0.15*0.50 + 0.20*0.1344 + 0.20*0.50
	got := agg.Aggregate(signals)
	if !approxEqual(got, want) {
		t.Errorf("未设 Available 时应等同全部可用：want %v, got %v", want, got)
	}
}

// TestWeightedAggregator_AllUnavailableFallsBackToEqualWeight 全部不可用时退回等权平均，
// 不返回 0（0 会被判成"完全不可信"而直接转人工）。
func TestWeightedAggregator_AllUnavailableFallsBackToEqualWeight(t *testing.T) {
	agg := NewDefaultWeightedAggregator()
	signals := &dto.FiveSignals{
		IntentConf: 0.90,
		EntityComp: 1.00,
		CtxRelev:   0.50,
		RAGQual:    0.80,
		LLMEntropy: 0.50,
		Available:  dto.NoneAvailable(),
	}
	got := agg.Aggregate(signals)
	if got <= 0 {
		t.Errorf("全部不可用时不应返回 0（会被判成完全不可信而直接转人工），got %v", got)
	}
	want := (0.90 + 1.00 + 0.50 + 0.80 + 0.50) / 5
	if !approxEqual(got, want) {
		t.Errorf("全部不可用时应退回等权平均：want %v, got %v", want, got)
	}
}
