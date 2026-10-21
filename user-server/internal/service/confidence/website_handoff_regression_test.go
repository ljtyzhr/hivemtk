package confidence

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
)

// 回归：官网右下角网页客服问「介绍下项目」被判「AI 置信度不足 (0.62 < 0.70)」并转人工。
//
// 这条用例锁的是**整条判据链**在"召回质量正常"时的取值：
//   - 意图识别给 0.90（ask_product，识别完全正确）
//   - 实体完整性 1.0（没有可抽实体）
//   - 上下文相关性 0.5（首轮对话，无历史）
//   - RAG 质量：线上实测 0.1344 —— 因为混合检索没跑 rerank，分数停在 RRF 名次分上
//
// 修完之后 RAG 质量必须回到"绝对相关度"量纲，聚合分必须越过转人工阈值。
// 判据取自线上真实数据：意图 0.9 + 实体 1.0 + 上下文 0.5 + RAG 0.5（中性）
// 加权后为 0.3*0.9 + 0.15*1.0 + 0.15*0.5 + 0.2*0.5 + 0.2*0.5 = 0.62。
// 这正是线上那个 0.6219 的构成 —— 唯一的异常项就是 RAG 质量被压到 0.1344。
func TestConfidence_WebsiteProductQuestion_NotPushedBelowHandoffThreshold(t *testing.T) {
	c := NewSignalCollector(nil)
	policyEngine := NewThresholdPolicyEngine(nil)
	_ = policyEngine.LoadPolicies(context.Background())
	agg := NewConfidenceAggregator(
		c,
		nil,
		NewDefaultWeightedAggregator(),
		NewVetoChain(),
		NewDynamicThresholdCalculator(policyEngine),
		nil,
	)

	// 线上「介绍下项目」的召回：3 条强相关（rerank 概率域 / 向量余弦量纲）。
	chunks := []dto.RAGChunk{
		{Score: 0.85},
		{Score: 0.72},
		{Score: 0.63},
	}
	in := &dto.SignalCollectionInput{
		SessionID:     "sess_website_probe",
		Text:          "介绍下项目",
		IntentType:    "ask_product",
		RawIntentConf: 0.90,
		RAGExecuted:   true,
		RAGChunks:     chunks,
		RAGTopK:       3,
	}
	dec, err := agg.Aggregate(context.Background(), in)
	if err != nil {
		t.Fatalf("Aggregate 失败: %v", err)
	}

	// 判据一：RAG 质量必须落在绝对相关度量纲（>= 0.5），不能是名次分摊薄出来的 0.13。
	if dec.Signals.RAGQual < 0.5 {
		t.Errorf("RAG 质量应 >= 0.5（绝对相关度量纲），got %.4f —— 线上该值是 0.1344，"+
			"把总置信度压到 0.62 < 0.70 而转人工", dec.Signals.RAGQual)
	}

	// 判据二：总置信度必须越过转人工阈值 0.70。
	if dec.AggregatedConf < 0.70 {
		t.Errorf("聚合置信度应 >= 0.70（不触发转人工），got %.4f", dec.AggregatedConf)
	}
}
