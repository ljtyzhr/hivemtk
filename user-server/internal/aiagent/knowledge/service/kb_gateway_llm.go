// Package service T4：GatewayLLMClient —— QA 拆分接模型网关。
//
// 走全局 Dispatcher 的 low_cost 场景（拆分是批量离线任务，不必占高质量路由），
// 计入租户用量（与 T1/T5 归因口径一致，§8.2）。
package service

import (
	"context"
	"errors"

	"hivemtk-user/internal/aiagent/llm"
)

// GatewayLLMClient 基于 LLM 网关的 Complete 实现
type GatewayLLMClient struct{}

// NewGatewayLLMClient 构造
func NewGatewayLLMClient() *GatewayLLMClient { return &GatewayLLMClient{} }

// Complete 单轮补全：low_cost 场景 dispatch，返回文本内容
func (c *GatewayLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
	d := llm.GetGlobalDispatcher()
	if d == nil {
		return "", errors.New("LLM 网关未就绪")
	}
	res, err := d.Dispatch(ctx, llm.DispatchRequest{
		Scenario:    llm.ScenarioLowCost,
		Prompt:      prompt,
		JSONMode:    true,
		MaxTokens:   4096,
		Temperature: 0.3,
	})
	if err != nil {
		return "", err
	}
	return res.Content, nil
}
