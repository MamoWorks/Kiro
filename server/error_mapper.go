package server

import (
	"encoding/json"
	"fmt"
	"kiro/utils"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorMappingStrategy 错误映射策略接口 (DIP原则)
type ErrorMappingStrategy interface {
	MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool)
	GetErrorType() string
}

// ClaudeErrorResponse Claude API规范的错误响应结构
type ClaudeErrorResponse struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	StopReason string `json:"stop_reason,omitempty"` // 用于内容长度超限等情况
}

// CodeWhispererErrorBody AWS CodeWhisperer错误响应体
type CodeWhispererErrorBody struct {
	Message string `json:"message"`
	Reason  string `json:"reason"`
}

// ContentLengthExceedsStrategy 内容长度超限错误映射策略 (SRP原则)
type ContentLengthExceedsStrategy struct{}

func (s *ContentLengthExceedsStrategy) MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool) {
	if statusCode != http.StatusBadRequest {
		return nil, false
	}

	var errorBody CodeWhispererErrorBody
	if err := json.Unmarshal(responseBody, &errorBody); err != nil {
		return nil, false
	}

	// 检查是否为内容长度超限错误
	if errorBody.Reason == "CONTENT_LENGTH_EXCEEDS_THRESHOLD" {
		return &ClaudeErrorResponse{
			Type:       "message_delta", // 按Claude规范发送message_delta事件
			StopReason: "max_tokens",    // 映射为max_tokens stop_reason
			Message:    "Content length exceeds threshold, response truncated",
		}, true
	}

	return nil, false
}

func (s *ContentLengthExceedsStrategy) GetErrorType() string {
	return "content_length_exceeds"
}

// DefaultErrorStrategy 默认错误映射策略 (YAGNI原则)
type DefaultErrorStrategy struct{}

func (s *DefaultErrorStrategy) MapError(statusCode int, responseBody []byte) (*ClaudeErrorResponse, bool) {
	return &ClaudeErrorResponse{
		Type:    "error",
		Message: fmt.Sprintf("Upstream error: %s", string(responseBody)),
	}, true
}

func (s *DefaultErrorStrategy) GetErrorType() string {
	return "default"
}

// ErrorMapper 错误映射器 (Strategy Pattern + Factory Pattern)
type ErrorMapper struct {
	strategies []ErrorMappingStrategy
}

// NewErrorMapper 创建错误映射器 (Factory Pattern)
func NewErrorMapper() *ErrorMapper {
	return &ErrorMapper{
		strategies: []ErrorMappingStrategy{
			&ContentLengthExceedsStrategy{}, // 优先处理特定错误
			&DefaultErrorStrategy{},         // 默认处理器
		},
	}
}

// MapCodeWhispererError 映射CodeWhisperer错误到Claude格式 (Template Method Pattern)
func (em *ErrorMapper) MapCodeWhispererError(statusCode int, responseBody []byte) *ClaudeErrorResponse {
	// 依次尝试各种映射策略
	for _, strategy := range em.strategies {
		if response, handled := strategy.MapError(statusCode, responseBody); handled {
			utils.Log("错误映射成功",
				utils.LogString("strategy", strategy.GetErrorType()),
				utils.LogInt("status_code", statusCode),
				utils.LogString("mapped_type", response.Type),
				utils.LogString("stop_reason", response.StopReason))
			return response
		}
	}

	// 理论上不会到达这里，因为DefaultErrorStrategy总是返回true
	return &ClaudeErrorResponse{
		Type:    "error",
		Message: "Unknown error",
	}
}

// SendClaudeError 发送Claude规范的错误响应 (KISS原则)
func (em *ErrorMapper) SendClaudeError(c *gin.Context, claudeError *ClaudeErrorResponse) {
	// 根据错误类型决定发送格式
	if claudeError.StopReason == "max_tokens" {
		// 发送message_delta事件，符合Claude规范
		em.sendMaxTokensResponse(c, claudeError)
	} else {
		// 发送标准错误事件
		em.sendStandardError(c, claudeError)
	}
}

// sendMaxTokensResponse 发送max_tokens类型的响应 (SRP原则)
func (em *ErrorMapper) sendMaxTokensResponse(c *gin.Context, claudeError *ClaudeErrorResponse) {
	// 按照Anthropic规范，当内容长度超限时，应该发送一个带有stop_reason: max_tokens的message_delta事件
	response := map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "max_tokens",
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":  0, // 实际项目中应该从请求中获取
			"output_tokens": 0,
		},
	}

	// 发送SSE事件
	sender := &AnthropicStreamSender{}
	if err := sender.SendEvent(c, response); err != nil {
		utils.Log("发送max_tokens响应失败",
			utils.LogErr(err),
			utils.LogString("original_message", claudeError.Message))
	}

	utils.Log("已发送max_tokens stop_reason响应",
		addReqFields(c,
			utils.LogString("stop_reason", "max_tokens"),
			utils.LogString("original_message", claudeError.Message))...)
}

// sendStandardError 发送标准错误响应 (SRP原则)
func (em *ErrorMapper) sendStandardError(c *gin.Context, claudeError *ClaudeErrorResponse) {
	errorResp := map[string]any{
		"type": "error",
		"error": map[string]any{
			"type":    "overloaded_error",
			"message": claudeError.Message,
		},
	}

	sender := &AnthropicStreamSender{}
	if err := sender.SendEvent(c, errorResp); err != nil {
		utils.Log("发送标准错误响应失败", utils.LogErr(err))
	}
}
