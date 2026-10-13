package service

// lead_intent_llm.go 线索判定的 LLM 复核层
//
// 背景：UnifiedLeadMiner 是通用电商关键词打分，词库硬编码、换行业就失灵。
// 本层把「这条消息算不算线索/商机」交给 LLM 做行业无关的结构化判别：
//   - 行业画像来自参数中心 lead.industry_profile（一段自由文本，任何行业
//     改这一段即可适配，不需要改代码）；
//   - 评分规则是通用销售语义（询价/购买意向高、能力咨询中、闲聊低），
//     不依赖任何行业词表；
//   - 关键词打分降级为「LLM 不可用时的兜底」，词库不再决定结果。
//
// 约束：
//   - 失败永不阻断主流程：任何错误都回落关键词结果（线索挖掘不能因为
//     LLM 抖动而丢线索）；
//   - LLM 输出非 JSON / 解析失败同样回落。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/llm"
)

// DefaultLeadLLMRefineEnabled / DefaultLeadIndustryProfile 是参数中心不可用时的兜底。
// 两处都必须与 config_param_seeds.go 里 lead 组的 DefaultValue 逐字一致：
// scripts/check-config-param-defaults.py 会比对这两个常量与种子行，不一致即判红。
const (
	DefaultLeadLLMRefineEnabled = true
	DefaultLeadIndustryProfile  = "通用零售/服务咨询：客户咨询产品或服务的价格、功能与购买流程"
)

// LLM 复核开关与行业画像都走参数中心（ConfigParam），这里用 seam 注入，
// 避免本包直接依赖装配层。初始值取上面的常量，这样「没接参数中心」和
// 「参数中心还没读到值」两种情况下，行为都和种子默认一致。
var (
	leadLLMRefineEnabledProvider = func() bool { return DefaultLeadLLMRefineEnabled }
	leadIndustryProfileProvider  = func() string { return DefaultLeadIndustryProfile }
)

// SetLeadLLMRefineProviders 由装配层注入参数读取口（nil 忽略）。
func SetLeadLLMRefineProviders(enabled func() bool, profile func() string) {
	if enabled != nil {
		leadLLMRefineEnabledProvider = enabled
	}
	if profile != nil {
		leadIndustryProfileProvider = profile
	}
}

// llmRefineTimeout 单次复核预算：线索挖掘在入站热路径上，超时立即回落。
const llmRefineTimeout = 12 * time.Second

// refineLeadWithLLM 用 LLM 复核关键词打分结果；不可用/失败时原样返回。
func refineLeadWithLLM(ctx context.Context, text string, score int, signals []string, isOpp bool) (int, []string, bool) {
	if os.Getenv("K_TRACE_LLM_REFINE") == "" && !leadLLMRefineEnabledProvider() {
		return score, signals, isOpp
	}
	dispatcher := llm.GetGlobalDispatcher()
	if dispatcher == nil {
		return score, signals, isOpp
	}
	profile := strings.TrimSpace(leadIndustryProfileProvider())

	ctx, cancel := context.WithTimeout(ctx, llmRefineTimeout)
	defer cancel()

	prompt := fmt.Sprintf(`你是销售线索评审员。根据行业背景与客户消息，输出 JSON 评分。

【行业背景】
%s

【通用评分规则】
- 70-95：明确询价/求报价/想购买/留联系方式
- 40-69：产品或服务能力咨询（了解、能不能、怎么收费的笼统问法）
- 15-39：弱相关闲聊（提到行业但无意向）
- 0-14：无关内容、寒暄
is_opportunity = intent_score >= %d

【客户消息】
%s

只输出 JSON：{"intent_score":0到100的整数,"is_opportunity":true或false,"intent_type":"price|consult|contact|chat|other","reason":"20字内依据"}`, profile, unifiedMinerOpportunityThreshold(), truncateForPrompt(text, 600))

	result, err := dispatcher.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioIntentRecognize,
		Prompt:      prompt,
		JSONMode:    true,
		MaxTokens:   200,
		Temperature: 0,
	})
	if err != nil {
		return score, signals, isOpp
	}
	var parsed struct {
		IntentScore *int  `json:"intent_score"`
		IsOpp       *bool `json:"is_opportunity"`
	}
	if err := json.Unmarshal([]byte(extractJSONFromStr(result.Content)), &parsed); err != nil || parsed.IntentScore == nil {
		return score, signals, isOpp
	}
	newScore := *parsed.IntentScore
	if newScore < 0 {
		newScore = 0
	}
	if newScore > 100 {
		newScore = 100
	}
	var newOpp bool
	if parsed.IsOpp != nil {
		newOpp = *parsed.IsOpp
	} else {
		newOpp = newScore >= unifiedMinerOpportunityThreshold()
	}
	signals = append(signals, "llm复核")
	return newScore, signals, newOpp
}

func truncateForPrompt(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ProbeLeadLLMRefineEnabled 暴露开关读取口当前值，供装配层测试断言接线确实生效
// （与 ProbeUnifiedMinerOpportunityThreshold 同用途）。
func ProbeLeadLLMRefineEnabled() bool { return leadLLMRefineEnabledProvider() }

// ProbeLeadIndustryProfile 暴露行业画像读取口当前值。
func ProbeLeadIndustryProfile() string { return leadIndustryProfileProvider() }
