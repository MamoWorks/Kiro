package types

// CountTokensRequest 符合Anthropic官方API规范的token计数请求结构
// 参考: https://docs.anthropic.com/en/api/messages-count-tokens
type CountTokensRequest struct {
	Model    string                    `json:"model" binding:"required"`
	Messages []AnthropicRequestMessage `json:"messages" binding:"required"`
	System   SystemMessages            `json:"system,omitempty"`
	Tools    []AnthropicTool           `json:"tools,omitempty"`
}

// CountTokensResponse 符合Anthropic官方API规范的token计数响应结构
type CountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}
