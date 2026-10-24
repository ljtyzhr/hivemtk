package ragretrieval

import (
	"context"
	"fmt"

	"hivemtk-user/internal/aiagent/llm"
)

// LLMChatOptions LLM Chat 调用选项
type LLMChatOptions struct {
	Temperature  float64
	MaxTokens    int
	SystemPrompt string
}

type LLMChatClient interface {
	Chat(ctx context.Context, prompt string, opts LLMChatOptions) (string, error)
}

// DispatcherChatAdapter 把 *llm.Dispatcher 适配为 LLMChatClient。
// scenario 默认 ScenarioLowCost（低成本批量场景），由 Dispatcher 内部路由到具体 provider。
type DispatcherChatAdapter struct {
	dispatcher *llm.Dispatcher
	scenario   llm.DispatchScenario
}

func NewDispatcherChatAdapter(dispatcher *llm.Dispatcher) *DispatcherChatAdapter {
	return &DispatcherChatAdapter{
		dispatcher: dispatcher,
		scenario:   llm.ScenarioLowCost,
	}
}

// WithScenario 设置调度场景。HyDE / Multi-Query 用默认低成本场景即可；
// Contextual Retrieval 建议使用 ScenarioLongSummary（长上下文）。
func (a *DispatcherChatAdapter) WithScenario(s llm.DispatchScenario) *DispatcherChatAdapter {
	a.scenario = s
	return a
}

func (a *DispatcherChatAdapter) Chat(ctx context.Context, prompt string, opts LLMChatOptions) (string, error) {
	if a == nil || a.dispatcher == nil {
		return "", fmt.Errorf("LLM dispatcher 未初始化")
	}
	req := llm.DispatchRequest{
		Scenario:     a.scenario,
		Prompt:       prompt,
		SystemPrompt: opts.SystemPrompt,
		MaxTokens:    opts.MaxTokens,
		Temperature:  opts.Temperature,
	}
	result, err := a.dispatcher.Dispatch(ctx, req)
	if err != nil {
		return "", fmt.Errorf("LLM dispatch 失败: %w", err)
	}
	return result.Content, nil
}

// NoopLLMChatClient 空实现（用于禁用查询改写场景）：
// Chat 始终返回错误，使上层走「未启用」分支。
type NoopLLMChatClient struct{}

func (NoopLLMChatClient) Chat(_ context.Context, _ string, _ LLMChatOptions) (string, error) {
	return "", fmt.Errorf("LLM chat disabled (noop client)")
}

var _ LLMChatClient = (*DispatcherChatAdapter)(nil)
var _ LLMChatClient = NoopLLMChatClient{}
