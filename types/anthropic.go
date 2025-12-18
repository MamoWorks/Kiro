package types

import "encoding/json"

// AnthropicTool 表示 Anthropic API 的工具结构
type AnthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ToolChoice 表示工具选择策略
type ToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "tool"
	Name string `json:"name,omitempty"` // 当type为"tool"时指定的工具名称
}

// AnthropicRequest 表示 Anthropic API 的请求结构
type AnthropicRequest struct {
	Model       string                    `json:"model"`
	MaxTokens   int                       `json:"max_tokens"`
	Messages    []AnthropicRequestMessage `json:"messages"`
	System      SystemMessages            `json:"system,omitempty"`
	Tools       []AnthropicTool           `json:"tools,omitempty"`
	ToolChoice  any                       `json:"tool_choice,omitempty"` // 可以是string或ToolChoice对象
	Stream      bool                      `json:"stream"`
	Temperature *float64                  `json:"temperature,omitempty"`
	Metadata    map[string]any            `json:"metadata,omitempty"`
	Thinking    *ThinkingConfig           `json:"thinking,omitempty"` // Thinking 模式配置
}

// ThinkingConfig 表示 Thinking 模式配置
type ThinkingConfig struct {
	Type         string `json:"type"`                    // "enabled" 或 "disabled"
	BudgetTokens int    `json:"budget_tokens,omitempty"` // 思考预算 token 数
}

// AnthropicStreamResponse 表示 Anthropic 流式响应的结构
type AnthropicStreamResponse struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentDelta struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"delta,omitempty"`
	Content []struct {
		Text string `json:"text"`
		Type string `json:"type"`
	} `json:"content,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
	Usage        *Usage `json:"usage,omitempty"`
}

// AnthropicRequestMessage 表示 Anthropic API 的消息结构
type AnthropicRequestMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // 可以是 string 或 []ContentBlock
}

// AnthropicSystemMessage 表示系统消息结构
type AnthropicSystemMessage struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SystemMessages 封装系统消息，支持字符串和数组两种格式
type SystemMessages []AnthropicSystemMessage

// UnmarshalJSON 实现自定义 JSON 解析，兼容字符串和数组格式
func (s *SystemMessages) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		// 空字符串视为无系统提示
		if str == "" {
			*s = []AnthropicSystemMessage{}
			return nil
		}
		*s = []AnthropicSystemMessage{{Type: "text", Text: str}}
		return nil
	}

	// 尝试解析为数组
	var messages []AnthropicSystemMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return err
	}
	*s = messages
	return nil
}

// ContentBlock 表示消息内容块的结构
type ContentBlock struct {
	Type      string       `json:"type"`
	Text      *string      `json:"text,omitempty"`
	ToolUseId *string      `json:"tool_use_id,omitempty"`
	Content   any          `json:"content,omitempty"`  // tool_result的内容，可以是string、[]any或map[string]any
	Name      *string      `json:"name,omitempty"`     // tool_use的名称
	Input     *any         `json:"input,omitempty"`    // tool_use的输入参数
	ID        *string      `json:"id,omitempty"`       // tool_use的唯一标识符
	IsError   *bool        `json:"is_error,omitempty"` // tool_result是否表示错误
	Source    *ImageSource `json:"source,omitempty"`   // 图片数据源
}

// ImageSource 表示图片数据源的结构
type ImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg", "image/png", "image/gif", "image/webp"
	Data      string `json:"data"`       // base64编码的图片数据
}
