package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var (
	ErrReActParseFailed = fmt.Errorf("react parse failed")
	ErrReActNoAction    = fmt.Errorf("react no action or final answer")
)

// ReActParseResult ReAct 解析结果
type ReActParseResult struct {
	Thought     string
	Action      string
	ActionInput string
	FinalAnswer string
	IsFinal     bool
	RawContent  string
}

var reactActionRe = regexp.MustCompile(`(?i)Action\s*:\s*([a-zA-Z0-9_.\-]+)`)

var reactActionInputRe = regexp.MustCompile(`(?is)Action\s*Input\s*:\s*(\{.*?\})`)

var reactFinalAnswerRe = regexp.MustCompile(`(?is)Final\s*Answer\s*:\s*(.+)`)

var reactThoughtRe = regexp.MustCompile(`(?is)Thought\s*:\s*(.+?)(?:\n\s*(?:Action|Final)|$)`)

// ParseReActResponse 解析 LLM ReAct 文本响应
//
// 返回值：
//   - 若含 Final Answer：IsFinal=true，FinalAnswer 非空
//   - 若含 Action：IsFinal=false，Action + ActionInput 非空
//   - 都不含：返回 ErrReActNoAction
func ParseReActResponse(content string) (*ReActParseResult, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: empty content", ErrReActParseFailed)
	}

	result := &ReActParseResult{RawContent: content}

	if m := reactFinalAnswerRe.FindStringSubmatch(content); m != nil {
		result.IsFinal = true
		result.FinalAnswer = strings.TrimSpace(m[1])
		if tm := reactThoughtRe.FindStringSubmatch(content); tm != nil {
			result.Thought = strings.TrimSpace(tm[1])
		}
		return result, nil
	}

	if m := reactActionRe.FindStringSubmatch(content); m != nil {
		result.Action = strings.TrimSpace(m[1])
		if im := reactActionInputRe.FindStringSubmatch(content); im != nil {
			result.ActionInput = strings.TrimSpace(im[1])
		} else {
			result.ActionInput = "{}"
		}
		if tm := reactThoughtRe.FindStringSubmatch(content); tm != nil {
			result.Thought = strings.TrimSpace(tm[1])
		}
		return result, nil
	}

	return &ReActParseResult{
		IsFinal:     true,
		FinalAnswer: content,
		RawContent:  content,
	}, nil
}

// ToToolCall 将 ReAct Action 转换为 OpenAI 兼容的 ToolCall 结构，
// 让上层 Agent Loop 统一处理 FC 和 ReAct 两种路径。
func (r *ReActParseResult) ToToolCall(id string) *ToolCall {
	if r == nil || r.IsFinal || r.Action == "" {
		return nil
	}
	return &ToolCall{
		ID:   id,
		Type: "function",
		Function: ToolCallFunction{
			Name:      r.Action,
			Arguments: r.ActionInput,
		},
	}
}

// ReActAdapter 将无 FC 能力的 LLM 适配为 ReAct 协议：Dispatch 前把 Tools 转为
// ReAct 系统提示词，Dispatch 后解析输出——含 Action 则构造 ToolCall 注入
// DispatchResult.ToolCalls，含 Final Answer 则直接作为 Content 返回。
// 适配器无状态，可安全并发使用。
type ReActAdapter struct {
	toolCallSeq uint64
}

// NewReActAdapter 创建 ReAct 适配器
func NewReActAdapter() *ReActAdapter {
	return &ReActAdapter{}
}

// WrapSystemPrompt 在原 SystemPrompt 后追加 ReAct 工具描述与协议说明（原提示词可为空）。
func (a *ReActAdapter) WrapSystemPrompt(originalSystemPrompt string, tools []ToolDefinition) string {
	var sb strings.Builder
	if originalSystemPrompt != "" {
		sb.WriteString(originalSystemPrompt)
		sb.WriteString("\n\n")
	}

	sb.WriteString("你是一个可以使用工具的智能体。请严格按照以下 ReAct 协议输出：\n\n")
	sb.WriteString("【可用工具】\n")
	for _, t := range tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
		if t.Function.Parameters != nil {
			if b, err := json.Marshal(t.Function.Parameters); err == nil {
				sb.WriteString(fmt.Sprintf("  参数: %s\n", string(b)))
			}
		}
	}

	sb.WriteString("\n【输出格式】\n")
	sb.WriteString("需要调用工具时：\n")
	sb.WriteString("Thought: <你的思考过程>\n")
	sb.WriteString("Action: <工具名>\n")
	sb.WriteString("Action Input: <JSON 参数>\n\n")
	sb.WriteString("获得工具结果后，最终回复时：\n")
	sb.WriteString("Thought: <你的思考过程>\n")
	sb.WriteString("Final Answer: <最终回复给用户的内容>\n\n")
	sb.WriteString("注意：\n")
	sb.WriteString("1. Action 必须是【可用工具】列表中的工具名\n")
	sb.WriteString("2. Action Input 必须是合法 JSON\n")
	sb.WriteString("3. 收到 Observation 后才能输出 Final Answer\n")
	sb.WriteString("4. 不要编造工具未返回的数据\n")

	return sb.String()
}

// AdaptResult 将 LLM 文本响应适配为 DispatchResult：
//   - 含 Action：在 result.ToolCalls 注入对应 ToolCall，FinishReason 改为 "tool_calls"
//   - 含 Final Answer：result.Content 改为 Final Answer 文本
//   - 都未匹配：保持原 result 不变（兼容性降级）
func (a *ReActAdapter) AdaptResult(result *DispatchResult) *DispatchResult {
	if result == nil || result.Content == "" {
		return result
	}

	parsed, err := ParseReActResponse(result.Content)
	if err != nil {
		return result
	}

	if parsed.IsFinal {
		result.Content = parsed.FinalAnswer
		return result
	}

	toolCallID := a.nextToolCallID()
	if tc := parsed.ToToolCall(toolCallID); tc != nil {
		result.ToolCalls = []ToolCall{*tc}
		result.FinishReason = "tool_calls"
		if parsed.Thought != "" {
			result.Content = parsed.Thought
		}
	}
	return result
}

func (a *ReActAdapter) nextToolCallID() string {
	seq := atomic.AddUint64(&a.toolCallSeq, 1)
	return fmt.Sprintf("react_%d_%d", time.Now().UnixNano(), seq)
}

// BuildObservationMessage 构造 ReAct Observation 消息。
// 工具结果回灌时，原生 FC 路径用 role=tool，ReAct 路径用 role=user
// （无 FC 能力 LLM 不识别 role=tool）。格式：Observation: <工具返回 JSON>
func BuildObservationMessage(toolName, toolCallID, observation string) ChatMessage {
	content := fmt.Sprintf("Observation: %s\n", observation)
	return ChatMessage{
		Role:       "user",
		Content:    content,
		ToolCallID: toolCallID,
		Name:       toolName,
	}
}

// IsReActMode 判断当前 DispatchRequest 是否走 ReAct 模式：
// provider 标记为 NoFC 且请求含 Tools（智能体模式）时启用。
// provider 标记由 Dispatcher 处理，此处仅做工具存在性判断。
func IsReActMode(req *DispatchRequest, providerNoFC bool) bool {
	if providerNoFC && req != nil && len(req.Tools) > 0 {
		return true
	}
	return false
}
