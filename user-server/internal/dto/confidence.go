package dto

import (
	"time"
)

// FiveSignals 5 维置信度信号值
//
// 信号源：
//   - IntentConf:  意图分类器置信度（经温度缩放校准）
//   - EntityComp:  实体完整性 = |extracted ∩ expected| / |expected|
//   - CtxRelev:    上下文相关性 = cosine(query, last_3_turns_mean)
//   - RAGQual:     RAG 检索质量 = mean(top-k score) * coverage_ratio
//   - LLMEntropy:  LLM 生成熵 = 1 - normalize(ShannonEntropy(top-20 logprobs))
type FiveSignals struct {
	IntentConf float64 `json:"intent_conf"`
	EntityComp float64 `json:"entity_comp"`
	CtxRelev   float64 `json:"ctx_relev"`
	RAGQual    float64 `json:"rag_qual"`
	LLMEntropy float64 `json:"llm_entropy"`

	// Available 标出哪些信号**真的取到了证据**。
	//
	// 为什么需要它：采集器在拿不到证据时会给 0.5 这个中性占位值（没有历史对话、
	// 没有 logprobs、没有 embedder）。占位值不是"中等证据"，它只是"这里没数据"，
	// 可加权聚合把它当成实打实的 0.5 参与计算 —— 首轮对话里 ctx_relev 与
	// llm_entropy 恒为 0.5，两项合计占 35% 权重，把总置信度死死按在 0.7 阈值之下：
	// 线上「介绍下项目」意图识别 0.9、召回也命中，仍被算成 0.62 ⇒ 转人工。
	//
	// 口径：某信号不可用 ⇒ 它的权重从加权分母里去掉、其余信号按比例放大。
	// 可用信号一个都没有时，退回原来的等权平均（不改变"全不可用"的历史行为）。
	//
	// **必须用 Set 字段表达可用性**：结构体零值表示"未设置 = 全部可用"，
	// 而"全部不可用"要靠 Set=true 显式表达（见 AllAvailable / NoneAvailable）。
	Available SignalAvailability `json:"available,omitempty"`
}

// SignalAvailability 逐信号可用性标记。
//
// 字段语义是"**已设置**"标记：该维被显式标为不可用时置 false，其余为 true。
// 零值（未设置）由 IsZero 判定为"全部可用"，见 FiveSignals.Available 的说明。
type SignalAvailability struct {
	// Set 区分"未设置"与"显式全不可用"。
	Set        bool `json:"set,omitempty"`
	IntentConf bool `json:"intent_conf"`
	EntityComp bool `json:"entity_comp"`
	CtxRelev   bool `json:"ctx_relev"`
	RAGQual    bool `json:"rag_qual"`
	LLMEntropy bool `json:"llm_entropy"`
}

// AllAvailable 五维信号全部可用。
func AllAvailable() SignalAvailability {
	return SignalAvailability{
		Set:        true,
		IntentConf: true,
		EntityComp: true,
		CtxRelev:   true,
		RAGQual:    true,
		LLMEntropy: true,
	}
}

// NoneAvailable 五维信号全部不可用。
func NoneAvailable() SignalAvailability {
	return SignalAvailability{Set: true}
}

// IsZero 是否为"零值"（= 未设置 ⇒ 全部可用）。
func (a SignalAvailability) IsZero() bool {
	return !a.Set
}

// SignalCollectionInput 5 维信号采集入参
//
// 由 SalesEngine / SmartCSOrchestrator 在每轮对话时构造并传给 ConfidenceAggregator
type SignalCollectionInput struct {
	SessionID  string `json:"session_id"`
	CustomerID string `json:"customer_id"`
	MessageID  string `json:"message_id"`
	Text       string `json:"text"`
	IntentType string `json:"intent_type"`

	RawIntentConf     float64        `json:"raw_intent_conf"`
	RawLogits         []float64      `json:"raw_logits,omitempty"`
	ExtractedEntities map[string]any `json:"extracted_entities,omitempty"`
	ExpectedEntities  map[string]any `json:"expected_entities,omitempty"`
	LLMLogprobs       []float64      `json:"llm_logprobs,omitempty"`
	RAGChunks         []RAGChunk     `json:"rag_chunks,omitempty"`
	RAGExecuted       bool           `json:"rag_executed,omitempty"`
	LastTurns         []string       `json:"last_turns,omitempty"`

	// RAGTopK 本次检索实际配置的期望召回条数（= 智能体的 rag_top_k）。
	//
	// 它是 RAG 覆盖率的**分母**：覆盖率 = 实际召回条数 / 期望条数。
	// 早先覆盖率的分母写死 5，而智能体普遍配 3 ⇒ 任何一次正常召回都被平白折成 0.6 倍，
	// 把"召回了 3 条好知识"说成"只覆盖了 60%"。0 表示调用方没给（按实际召回条数兜底）。
	RAGTopK int `json:"rag_top_k,omitempty"`

	CustomerLevel     string  `json:"customer_level,omitempty"`
	AgentAvailability float64 `json:"agent_availability,omitempty"`
}

// ConfidenceDecision 置信度决策结果
//
// 由 ConfidenceAggregator.Aggregate 返回，包含 5 维信号快照、聚合置信度、动态阈值和决策区间
type ConfidenceDecision struct {
	SignalID         string      `json:"signal_id"`
	AggregatedConf   float64     `json:"aggregated_conf"`
	DynamicThreshold float64     `json:"dynamic_threshold"`
	DecisionBand     string      `json:"decision_band"`
	VetoTriggered    string      `json:"veto_triggered,omitempty"`
	Signals          FiveSignals `json:"signals"`
	CalculatedAt     time.Time   `json:"calculated_at"`
}

// 决策区间常量
const (
	BandHandoff     = "handoff"
	BandLLMFallback = "llm_fallback"
	BandReview      = "review"
	BandAuto        = "auto"
)

// CalibrationResult 校准结果（dto 层暴露给上游）
type CalibrationResult struct {
	Temperature float64 `json:"temperature"`
	ECEBefore   float64 `json:"ece_before"`
	ECEAfter    float64 `json:"ece_after"`
	NLLBefore   float64 `json:"nll_before"`
	NLLAfter    float64 `json:"nll_after"`
	SampleSize  int     `json:"sample_size"`
}

// ThresholdPolicyRequest 阈值策略 Upsert 请求 DTO
type ThresholdPolicyRequest struct {
	PolicyID                string  `json:"policy_id" binding:"required,min=1,max=64"`
	IntentType              string  `json:"intent_type" binding:"required,min=1,max=64"`
	BaseThreshold           float64 `json:"base_threshold" binding:"required,min=0,max=1"`
	CustomerLevelWeight     float64 `json:"customer_level_weight" binding:"min=0,max=1"`
	TimeslotWeight          float64 `json:"timeslot_weight" binding:"min=0,max=1"`
	AgentAvailabilityWeight float64 `json:"agent_availability_weight" binding:"min=0,max=1"`
	BandHandoffUpper        float64 `json:"band_handoff_upper" binding:"min=0,max=1"`
	BandFallbackUpper       float64 `json:"band_fallback_upper" binding:"min=0,max=1"`
	BandReviewUpper         float64 `json:"band_review_upper" binding:"min=0,max=1"`
	ReviewSLASeconds        int     `json:"review_sla_seconds" binding:"min=1"`
	Version                 int     `json:"version" binding:"min=1"`
}
