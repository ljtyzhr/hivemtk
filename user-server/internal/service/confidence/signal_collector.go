package confidence

import (
	"context"
	"math"
	"strings"

	"hivemtk-user/internal/dto"
)

// Embedder 向量化接口（解耦对 llm.EmbeddingServiceInterface 的依赖）
//
// SignalCollector 仅需要 Embed(text) → []float32 能力
// 生产环境注入 llm.EmbeddingService；测试注入 mock
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// SignalCollector 5 维信号采集器
type SignalCollector struct {
	embedder Embedder
}

// NewSignalCollector 构造采集器
//
// embedder 可为 nil（CtxRelev 信号将降级为中性值 0.5）
func NewSignalCollector(embedder Embedder) *SignalCollector {
	return &SignalCollector{embedder: embedder}
}

// Collect 采集 5 维信号
//
// 步骤：
//  1. IntentConf：从意图识别器输出读取（已在 input.RawIntentConf 中）
//  2. EntityComp：实体完整性 = |extracted ∩ expected| / |expected|
//  3. CtxRelev：query 与最近 3 轮对话均值的余弦相似度
//  4. RAGQual：top-k chunk 余弦均值 * 覆盖率
//  5. LLMEntropy：1 - normalize(entropy(top-20 logprobs))
func (c *SignalCollector) Collect(ctx context.Context, in *dto.SignalCollectionInput) (*dto.FiveSignals, error) {
	signals := &dto.FiveSignals{
		IntentConf: clamp01(in.RawIntentConf),
	}

	// 逐维标出"真取到了证据"还是"只是中性占位值"。
	//
	// 占位值 0.5 的含义是"这里没数据"，不是"这一维是中等水平"。让它们参与
	// 等权聚合，等于凭空给这两维各 0.5 的信用或扣分 —— 首轮对话里 ctx_relev
	// 与 llm_entropy 恒为占位值，两项占 35% 权重，把总置信度按在阈值之下，
	// 官网客服对"介绍下项目"这种能答的问题也判低置信转人工。
	signals.Available = dto.SignalAvailability{
		Set:        true,
		IntentConf: in.RawIntentConf > 0,
		EntityComp: true,
		CtxRelev:   false,
		RAGQual:    false,
		LLMEntropy: false,
	}

	signals.EntityComp = c.computeEntityComp(in.ExtractedEntities, in.ExpectedEntities)

	ctxRelev, ctxOK, err := c.computeCtxRelev(ctx, in.Text, in.LastTurns)
	if err == nil {
		signals.CtxRelev = ctxRelev
		signals.Available.CtxRelev = ctxOK
	} else {
		signals.CtxRelev = 0.5
	}

	signals.RAGQual, signals.Available.RAGQual = c.computeRAGQual(in)

	signals.LLMEntropy, signals.Available.LLMEntropy = c.computeLLMEntropy(in.LLMLogprobs)

	return signals, nil
}

func (c *SignalCollector) computeEntityComp(extracted, expected map[string]any) float64 {
	if len(expected) == 0 {
		return 1.0
	}
	hit := 0
	for k, v := range expected {
		if ev, ok := extracted[k]; ok && ev == v {
			hit++
		}
	}
	return clamp01(float64(hit) / float64(len(expected)))
}

// computeCtxRelev 算上下文相关性，第二个返回值表示**是否真取到了证据**。
//
// 没有历史对话（首轮）、没有 embedder、或向量化失败时，返回的中性 0.5 只是占位值，
// available=false ⇒ 该维不参与加权。
func (c *SignalCollector) computeCtxRelev(ctx context.Context, query string, lastTurns []string) (float64, bool, error) {
	if c.embedder == nil {
		return 0.5, false, nil
	}
	if len(lastTurns) == 0 || strings.TrimSpace(query) == "" {
		return 0.5, false, nil
	}
	queryVec, err := c.embedder.Embed(ctx, query)
	if err != nil {
		return 0.5, false, err
	}
	if len(queryVec) == 0 {
		return 0.5, false, nil
	}

	meanVec := make([]float32, len(queryVec))
	validCount := 0
	for _, turn := range lastTurns {
		if strings.TrimSpace(turn) == "" {
			continue
		}
		v, err := c.embedder.Embed(ctx, turn)
		if err != nil || len(v) != len(meanVec) {
			continue
		}
		for i := range v {
			meanVec[i] += v[i]
		}
		validCount++
	}
	if validCount == 0 {
		return 0.5, false, nil
	}
	for i := range meanVec {
		meanVec[i] /= float32(validCount)
	}
	return clamp01(cosineSim(queryVec, meanVec)), true, nil
}

// computeRAGQual 算 RAG 检索质量，第二个返回值表示**是否真取到了证据**。
//
// 未执行检索（没有召回也没有执行标记）⇒ 0.5 中性占位值、available=false。
// 执行了检索但一条没召回 ⇒ 0.0、available=true（这是"确实没有相关知识"的真实结论，
// 与"没查"是两回事，不能混同）。
func (c *SignalCollector) computeRAGQual(in *dto.SignalCollectionInput) (float64, bool) {
	if !in.RAGExecuted && len(in.RAGChunks) == 0 {
		return 0.5, false
	}
	chunks := in.RAGChunks
	if len(chunks) == 0 {
		return 0.0, true
	}
	sum := 0.0
	for _, ch := range chunks {
		sum += ch.Score
	}
	meanScore := sum / float64(len(chunks))

	// 覆盖率 = 实际召回条数 / 本次配置的期望条数。
	//
	// 分母早先写死 5.0，而智能体普遍配 rag_top_k=3 ⇒ 每次正常召回都被折成 0.6 倍。
	// 召回条数少是**配置**（限上下文预算），不是缺陷，不能拿来当质量折扣。
	expectedK := in.RAGTopK
	if expectedK <= 0 {
		// 调用方没给期望条数：以实际召回条数为分母（全覆盖 ⇒ 1.0）。
		expectedK = len(chunks)
	}
	coverage := math.Min(1.0, float64(len(chunks))/float64(expectedK))
	return clamp01(meanScore * coverage), true
}

// computeLLMEntropy 算 LLM 生成熵，第二个返回值表示**是否真取到了证据**。
//
// 没有 logprobs 时返回 0.5 中性占位值、available=false —— 那不是"生成熵是中等水平"，
// 而是"没测到熵"。
func (c *SignalCollector) computeLLMEntropy(logprobs []float64) (float64, bool) {
	if len(logprobs) == 0 {
		return 0.5, false
	}
	maxLp := logprobs[0]
	for _, lp := range logprobs {
		if lp > maxLp {
			maxLp = lp
		}
	}
	expSum := 0.0
	expLp := make([]float64, len(logprobs))
	for i, lp := range logprobs {
		expLp[i] = math.Exp(lp - maxLp)
		expSum += expLp[i]
	}
	if expSum == 0 {
		return 0.5, false
	}
	entropy := 0.0
	for _, e := range expLp {
		p := e / expSum
		if p > 0 {
			entropy -= p * math.Log(p)
		}
	}
	k := float64(len(logprobs))
	if k <= 1 {
		return 1.0, true
	}
	normalizedEntropy := entropy / math.Log(k)
	return clamp01(1.0 - normalizedEntropy), true
}

func cosineSim(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	dot := float64(0)
	na := float64(0)
	nb := float64(0)
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0.0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
