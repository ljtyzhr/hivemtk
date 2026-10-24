package ragcustomerservice

import (
	"context"

	"hivemtk-user/internal/aiagent/llm"
)

// LLMServiceAdapter 适配 *llm.LLMService → LLMServiceInterface
// （LLMService 的方法签名与本包接口不一致）
type LLMServiceAdapter struct {
	svc *llm.LLMService
}

func NewLLMServiceAdapter(svc *llm.LLMService) *LLMServiceAdapter {
	return &LLMServiceAdapter{svc: svc}
}

func (a *LLMServiceAdapter) Generate(ctx context.Context, config any, prompt string) (string, error) {
	if llmCfg, ok := config.(*llm.LLMConfig); ok {
		return a.svc.Generate(ctx, llmCfg, prompt)
	}
	return a.svc.Generate(ctx, &llm.LLMConfig{}, prompt)
}

func (a *LLMServiceAdapter) GenerateStructured(ctx context.Context, config any, prompt string, schema any) (any, error) {
	out, err := a.Generate(ctx, config, prompt)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (a *LLMServiceAdapter) ValidateConfig(config any) error {
	return nil
}

func (a *LLMServiceAdapter) GetDefaultConfig() any {
	return &llm.LLMConfig{}
}
