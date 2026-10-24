package app

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/pkg/utils/logger"
)

// =============================================================================
// 推理降级档（C2）：本地 LLM 不可达 ⇒ 回退云端 LLM，Embedding 保持本地。
//
// 为什么这一层放在 app 而不是 aiagent/llm：
//   - 档位选择是**部署决策**，不是调度决策。它的输入（本地 LLM 端点、云端 LLM
//     是否配置、降级是否被允许）全部来自 config.AppConfig / 配置项，输出是一句
//     明确的"当前跑在哪一档"。aiagent/llm 的 Dispatcher 已经负责"候选 provider
//     逐个试、熔断、模板兜底"这套运行时动作，那套逻辑不该再叠一层档位概念；
//   - 本包正好是装配层（wiring）：它读 config、组装组件，并把决策记下来供
//     启动日志与状态面读取。放这里不引入新的依赖方向（app 已 import config 与 llm）。
//
// 为什么不把"选择结果"直接改 Dispatcher 路由：本地 default 候选失败后，Dispatcher
// 的 pickEnabledFallback 已经会按 QualityScore 挑一个 enabled 的云端 provider；
// ProviderFailover 的健康检查会把连不上的本地 provider 熔断掉。运行时回落链已经存在，
// 本档位要补的是"**在启动时把档位显式定下来并说出来**"这个缺口——让"本地推理挂了、
// 现在跑在云端 LLM 降级档"这件事从一堆 warn 日志变成一条可判定的状态。
// =============================================================================

// InferenceTier 推理档位。
type InferenceTier string

const (
	// TierFullLocal 本地全栈档：LLM / Embedding / Rerank 全走本地 llama.cpp。
	TierFullLocal InferenceTier = "full_local"
	// TierCloudLLMLocalEmbedding 降级档：本地 LLM 不可达时回退云端 LLM，
	// Embedding 仍走本地（数据不出域，合规基线不破）。
	TierCloudLLMLocalEmbedding InferenceTier = "cloud_llm_local_embedding"
	// TierLocalDownNoCloud 本地 LLM 不可达且未配置云端 LLM：无降级档可切，
	// LLM 请求走 Dispatcher 运行时的模板兜底。
	TierLocalDownNoCloud InferenceTier = "local_down_no_cloud"
)

// TierDecision 一次档位选择的结果，字段是自检报告的事实面而不是判定面：
// 上层拿它打日志、状态面拿它出读数，判定顺序只在 Select 一处。
type TierDecision struct {
	Tier               InferenceTier `json:"tier"`
	LocalLLMUp         bool          `json:"local_llm_up"`
	LocalEmbeddingUp   bool          `json:"local_embedding_up"`
	CloudLLMConfigured bool          `json:"cloud_llm_configured"`
	DegradedAllowed    bool          `json:"degraded_allowed"`
	Reason             string        `json:"reason"`
}

// InferenceReachProbe 推理端点可用性探测接口。
//
// 为什么是接口而不是直接写 http.Get：选择逻辑必须能在不连真服务的单测里验证，
// 探测动作本身（连不连得上）与"连上与否该选哪一档"是两件事。实现见
// httpInferenceProbe，测试注入 fake。
type InferenceReachProbe interface {
	ProbeLLM(ctx context.Context, baseURL string) error
	ProbeEmbedding(ctx context.Context, baseURL string) error
}

// TierPolicy 档位选择的输入（全部来自配置，本层不做任何网络/DB 读取）。
type TierPolicy struct {
	// DegradedAllowed 是否允许降级档（配置项 inference.tier.fallback_to_cloud_llm）。
	DegradedAllowed bool
	// LocalLLMBaseURL 本地 LLM 端点（config.Inference.LLM.BaseURL）。
	LocalLLMBaseURL string
	// LocalEmbeddingBaseURL 本地 Embedding 端点（config.Inference.Embedding.BaseURL）。
	LocalEmbeddingBaseURL string
	// CloudLLMConfigured 是否已配置可用的云端 LLM（enabled 且带 api_key 且带 base_url）。
	CloudLLMConfigured bool
}

// InferenceTierSelector 档位选择器：纯决策，无状态副作用（除了把最近一次
// 决策存给状态面读）。
type InferenceTierSelector struct {
	policy TierPolicy
	probe  InferenceReachProbe
}

// NewInferenceTierSelector 构造档位选择器；probe 传 nil 时使用默认 HTTP 探测。
func NewInferenceTierSelector(policy TierPolicy, probe InferenceReachProbe) *InferenceTierSelector {
	if probe == nil {
		probe = NewHTTPInferenceProbe(3 * time.Second)
	}
	return &InferenceTierSelector{policy: policy, probe: probe}
}

// Select 执行一次档位选择。判定顺序固定（不许乱序，乱序会改变语义）：
//
//  1. 本地 LLM 可达          ⇒ full_local（本地优先，永远优先）；
//  2. 本地 LLM 不可达且
//     允许降级 + 已配云端 LLM ⇒ cloud_llm_local_embedding（Embedding 仍本地）；
//  3. 其余                    ⇒ local_down_no_cloud（运行时模板兜底）。
func (s *InferenceTierSelector) Select(ctx context.Context) TierDecision {
	if s == nil {
		return TierDecision{Tier: TierLocalDownNoCloud, Reason: "selector is nil"}
	}

	llmUp := s.probe.ProbeLLM(ctx, s.policy.LocalLLMBaseURL) == nil
	embUp := s.probe.ProbeEmbedding(ctx, s.policy.LocalEmbeddingBaseURL) == nil

	d := TierDecision{
		LocalLLMUp:         llmUp,
		LocalEmbeddingUp:   embUp,
		CloudLLMConfigured: s.policy.CloudLLMConfigured,
		DegradedAllowed:    s.policy.DegradedAllowed,
	}

	switch {
	case llmUp:
		d.Tier = TierFullLocal
		d.Reason = "本地 LLM 可达，全本地档"
	case s.policy.DegradedAllowed && s.policy.CloudLLMConfigured:
		d.Tier = TierCloudLLMLocalEmbedding
		d.Reason = "本地 LLM 不可达，已配置云端 LLM 且降级允许，回退云端 LLM（Embedding 仍本地）"
	default:
		d.Tier = TierLocalDownNoCloud
		d.Reason = "本地 LLM 不可达且无可用云端降级（未配置云端 LLM 或降级被禁用），LLM 走运行时模板兜底"
	}
	return d
}

// httpInferenceProbe 默认探测实现：GET {baseURL}/health，2xx 视为可达。
//
// 与 ProviderFailover.HTTPHealthChecker 同口径（/health 是 llama-server 的健康端点），
// 但独立于那套熔断器：这里只回答"这一刻通不通"，不累计、不熔断。
type httpInferenceProbe struct {
	client *http.Client
}

// NewHTTPInferenceProbe 构造 HTTP 探测（timeout 由装配层传入，不设包级常量）。
func NewHTTPInferenceProbe(timeout time.Duration) *httpInferenceProbe {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &httpInferenceProbe{client: &http.Client{Timeout: timeout}}
}

func (h *httpInferenceProbe) ProbeLLM(ctx context.Context, baseURL string) error {
	return h.ping(ctx, baseURL)
}

func (h *httpInferenceProbe) ProbeEmbedding(ctx context.Context, baseURL string) error {
	return h.ping(ctx, baseURL)
}

func (h *httpInferenceProbe) ping(ctx context.Context, baseURL string) error {
	if h == nil || h.client == nil {
		return fmt.Errorf("inference probe: nil client")
	}
	base := strings.TrimSpace(baseURL)
	if base == "" {
		return fmt.Errorf("inference probe: base_url 为空")
	}
	// 健康探测钉在服务根路径 /health，而不是 base_url 直接拼 /health：
	// 配置里的 base_url 带 OpenAI 兼容前缀 /v1（如 http://127.0.0.1:8208/v1），
	// 而 llama-server 的健康端点在根路径（与 scripts/inference-host 的 wait_health
	// 同口径），拼 /v1/health 在非 llama.cpp 网关（如 8787 freeapi）上会 404 误报。
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("inference probe: base_url 不是合法绝对地址: %q", base)
	}
	healthURL := u.Scheme + "://" + u.Host + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("inference probe: build request %s: %w", healthURL, err)
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("inference probe: %s: %w", healthURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("inference probe: %s returned status %d", healthURL, resp.StatusCode)
	}
	return nil
}

// BuildTierPolicyFromConfig 从 AppConfig 组装档位策略。
//
// CloudLLMConfigured 的判据是"enabled 且 api_key 非空且 base_url 非空"——与
// dispatcher.registerCloudProvidersFromConfig 的启用口径一致（只注册这三样齐的）。
// 单写一个 api_key 不叫"已配置云端"：缺 base_url 发不出去、缺 enabled 不会被注册。
func BuildTierPolicyFromConfig(cfg config.AppConfig) TierPolicy {
	p := TierPolicy{
		DegradedAllowed:       cfg.Inference.Tier.FallbackToCloudLLM,
		LocalLLMBaseURL:       cfg.Inference.LLM.BaseURL,
		LocalEmbeddingBaseURL: cfg.Inference.Embedding.BaseURL,
	}
	for _, cp := range cfg.Inference.LLM.CloudProviders {
		if cp.Enabled && strings.TrimSpace(cp.APIKey) != "" && strings.TrimSpace(cp.BaseURL) != "" {
			p.CloudLLMConfigured = true
			break
		}
	}
	return p
}

var (
	tierDecisionMu     sync.RWMutex
	lastTierDecision   *TierDecision
	tierDecisionWasSet bool
)

// recordTierDecision 记下最近一次选择结果（供状态面/日志读取，只写内存）。
func recordTierDecision(d TierDecision) {
	tierDecisionMu.Lock()
	defer tierDecisionMu.Unlock()
	cp := d
	lastTierDecision = &cp
	tierDecisionWasSet = true
}

// GetLastTierDecision 返回最近一次档位选择结果；尚未选择过时返回 nil。
func GetLastTierDecision() *TierDecision {
	tierDecisionMu.RLock()
	defer tierDecisionMu.RUnlock()
	if !tierDecisionWasSet || lastTierDecision == nil {
		return nil
	}
	cp := *lastTierDecision
	return &cp
}

// SelectAndRecordInferenceTier 启动期装配入口：做一次档位选择、记下来、打日志。
//
// 只读探测（GET /health），不写库、不改配置。返回决策供启动流程继续使用。
func SelectAndRecordInferenceTier(ctx context.Context, policy TierPolicy, probe InferenceReachProbe) TierDecision {
	d := NewInferenceTierSelector(policy, probe).Select(ctx)
	recordTierDecision(d)

	switch d.Tier {
	case TierFullLocal:
		logger.Infof("[inference-tier] 档位=full_local（本地 LLM/Embedding 全通）")
	case TierCloudLLMLocalEmbedding:
		logger.Warnf("[inference-tier] 档位=cloud_llm_local_embedding：本地 LLM 不可达，已回退云端 LLM；Embedding 仍本地。%s", d.Reason)
	default:
		logger.Warnf("[inference-tier] 档位=local_down_no_cloud：本地 LLM 不可达且无云端降级，LLM 走运行时模板兜底。%s", d.Reason)
	}
	return d
}
