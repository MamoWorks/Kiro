package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"kiro/config"
	"kiro/converter"

	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// UpstreamError 上游 API 错误类型
type UpstreamError struct {
	StatusCode int
	Message    string
}

func (e *UpstreamError) Error() string {
	return e.Message
}

// respondErrorWithCode 标准化的错误响应结构
// 统一返回: {"error": {"message": string, "code": string}}
func respondErrorWithCode(c *gin.Context, statusCode int, code string, format string, args ...any) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": fmt.Sprintf(format, args...),
			"code":    code,
		},
	})
}

// respondError 简化封装，依据statusCode映射默认code
func respondError(c *gin.Context, statusCode int, format string, args ...any) {
	var code string
	switch statusCode {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusUnauthorized:
		code = "unauthorized"
	case http.StatusForbidden:
		code = "forbidden"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusTooManyRequests:
		code = "rate_limited"
	default:
		code = "internal_error"
	}
	respondErrorWithCode(c, statusCode, code, format, args...)
}

// 通用请求处理错误函数
func handleRequestBuildError(c *gin.Context, err error) {
	utils.Error("构建请求失败: %v", err)
	respondError(c, http.StatusInternalServerError, "构建请求失败: %v", err)
}

func handleRequestSendError(c *gin.Context, err error) {
	utils.Error("发送请求失败: %v", err)
	respondError(c, http.StatusInternalServerError, "发送请求失败: %v", err)
}

func handleResponseReadError(c *gin.Context, err error) {
	utils.Error("读取响应体失败: %v", err)
	respondError(c, http.StatusInternalServerError, "读取响应体失败: %v", err)
}

// 通用请求执行函数
// filterSupportedTools 过滤掉不支持的工具（与上游转换逻辑保持一致）
// 设计原则：
// - DRY: 统一过滤逻辑，确保计费与上游请求一致
// - KISS: 简单直接的过滤规则
func filterSupportedTools(tools []types.AnthropicTool) []types.AnthropicTool {
	if len(tools) == 0 {
		return tools
	}

	filtered := make([]types.AnthropicTool, 0, len(tools))
	for _, tool := range tools {
		// 过滤不支持的工具：web_search（与 converter/codewhisperer.go 保持一致）
		if tool.Name == "web_search" || tool.Name == "websearch" {
			continue
		}
		filtered = append(filtered, tool)
	}

	return filtered
}

func executeCodeWhispererRequest(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Response, error) {
	req, err := buildCodeWhispererRequest(c, anthropicReq, tokenInfo, isStream)
	if err != nil {
		// 检查是否是模型未找到错误，如果是，则响应已经发送，不需要再次处理
		if _, ok := err.(*types.ModelNotFoundErrorType); ok {
			return nil, err
		}
		if !isStream {
			handleRequestBuildError(c, err)
		}
		return nil, err
	}

	resp, err := utils.DoRequest(req)
	if err != nil {
		if !isStream {
			handleRequestSendError(c, err)
		}
		return nil, err
	}

	upstreamErr := handleCodeWhispererError(c, resp, isStream)
	if upstreamErr != nil {
		resp.Body.Close()
		return nil, upstreamErr
	}

	return resp, nil
}

// execCWRequest 供测试覆盖的请求执行入口（可在测试中替换）
var execCWRequest = executeCodeWhispererRequest

// buildCodeWhispererRequest 构建通用的CodeWhisperer请求
func buildCodeWhispererRequest(c *gin.Context, anthropicReq types.AnthropicRequest, tokenInfo types.TokenInfo, isStream bool) (*http.Request, error) {
	cwReq, err := converter.BuildCodeWhispererRequest(anthropicReq, c)
	if err != nil {
		// 检查是否是模型未找到错误
		if modelNotFoundErr, ok := err.(*types.ModelNotFoundErrorType); ok {
			// 直接返回用户期望的JSON格式
			c.JSON(http.StatusBadRequest, modelNotFoundErr.ErrorData)
			return nil, err
		}
		return nil, fmt.Errorf("构建CodeWhisperer请求失败: %v", err)
	}

	cwReqBody, err := utils.SafeMarshal(cwReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %v", err)
	}

	utils.Info("上游请求: size=%d, tools=%d",
		len(cwReqBody),
		len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools))

	req, err := http.NewRequest("POST", config.CodeWhispererURL, bytes.NewReader(cwReqBody))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenInfo.AccessToken)
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererStreamingService.GenerateAssistantResponse")
	req.Header.Set("User-Agent", "aws-sdk-rust/1.3.9 os/macos lang/rust/1.87.0")
	req.Header.Set("X-Amz-User-Agent", "aws-sdk-rust/1.3.9 ua/2.1 api/codewhispererstreaming/1.0.0 os/macos lang/rust/1.87.0 m/E")

	return req, nil
}

// handleCodeWhispererError 处理CodeWhisperer API错误响应
// 对于流式请求，只返回错误信息；对于非流式请求，发送JSON响应
func handleCodeWhispererError(c *gin.Context, resp *http.Response, isStream bool) *UpstreamError {
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Error("读取错误响应失败: %v", err)
		if !isStream {
			respondError(c, http.StatusInternalServerError, "%s", "读取响应失败")
		}
		return &UpstreamError{StatusCode: resp.StatusCode, Message: "读取响应失败"}
	}

	utils.Error("上游错误: status=%d", resp.StatusCode)

	// 尝试解析上游错误信息
	errorMsg := string(body)
	var errorResp map[string]any
	if err := utils.SafeUnmarshal(body, &errorResp); err == nil {
		if msg, ok := errorResp["message"].(string); ok && msg != "" {
			errorMsg = msg
		}
	}

	// 特殊处理：403错误表示账号被封禁
	if resp.StatusCode == http.StatusForbidden {
		// 清除失效的 token 缓存
		if refreshToken, exists := c.Get("refreshToken"); exists {
			if token, ok := refreshToken.(string); ok {
				InvalidateToken(token)
			}
		}

		if !isStream {
			respondErrorWithCode(c, http.StatusForbidden, "forbidden", "%s", errorMsg)
		}
		return &UpstreamError{StatusCode: resp.StatusCode, Message: errorMsg}
	}

	// 使用错误映射器处理错误
	errorMapper := NewErrorMapper()
	claudeError := errorMapper.MapCodeWhispererError(resp.StatusCode, body)

	if !isStream {
		// 非流式请求：发送JSON响应
		if claudeError.StopReason == "max_tokens" {
			errorMapper.SendClaudeError(c, claudeError)
		} else {
			respondErrorWithCode(c, http.StatusInternalServerError, "cw_error", "%s", errorMsg)
		}
	}

	return &UpstreamError{StatusCode: resp.StatusCode, Message: errorMsg}
}

// StreamEventSender 统一的流事件发送接口
type StreamEventSender interface {
	SendEvent(c *gin.Context, data any) error
	SendError(c *gin.Context, message string, err error) error
}

// AnthropicStreamSender Anthropic格式的流事件发送器
type AnthropicStreamSender struct{}

func (s *AnthropicStreamSender) SendEvent(c *gin.Context, data any) error {
	var eventType string
	var orderedData any = data

	// 如果是 map，转换为有序 struct
	if dataMap, ok := data.(map[string]any); ok {
		if t, exists := dataMap["type"]; exists {
			eventType, _ = t.(string)
		}
		orderedData = convertToOrderedStruct(dataMap)
	} else {
		// 从 struct 中提取 type
		switch v := data.(type) {
		case *types.MessageStartEvent:
			eventType = v.Type
		case *types.ContentBlockStartEvent:
			eventType = v.Type
		case *types.ContentBlockDeltaEvent:
			eventType = v.Type
		case *types.ContentBlockStopEvent:
			eventType = v.Type
		case *types.MessageDeltaEvent:
			eventType = v.Type
		case *types.MessageStopEvent:
			eventType = v.Type
		case *types.PingEvent:
			eventType = v.Type
		case *types.GenericOrderedEvent:
			eventType = v.Type
		case *types.ErrorEvent:
			eventType = v.Type
		}
	}

	json, err := utils.SafeMarshal(orderedData)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.Writer, "event: %s\n", eventType)
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(json))
	c.Writer.Flush()
	return nil
}

// convertToOrderedStruct 将 map 转换为有序 struct（保证 type 在最前）
func convertToOrderedStruct(m map[string]any) any {
	eventType, _ := m["type"].(string)

	switch eventType {
	case "message_start":
		return convertMessageStart(m)
	case "content_block_start":
		return convertContentBlockStart(m)
	case "content_block_delta":
		return convertContentBlockDelta(m)
	case "content_block_stop":
		return convertContentBlockStop(m)
	case "message_delta":
		return convertMessageDelta(m)
	case "message_stop":
		return types.NewMessageStopEvent()
	case "ping":
		return types.NewPingEvent()
	case "error":
		return convertError(m)
	default:
		// 未知类型使用 GenericOrderedEvent 确保 type 在前
		return types.NewGenericOrderedEvent(eventType, m)
	}
}

func convertMessageStart(m map[string]any) *types.MessageStartEvent {
	msg := &types.MessageInfo{
		Content: []any{}, // 确保 Content 始终是空数组而不是 nil
	}
	if message, ok := m["message"].(map[string]any); ok {
		msg.ID, _ = message["id"].(string)
		msg.Type, _ = message["type"].(string)
		msg.Role, _ = message["role"].(string)
		msg.Model, _ = message["model"].(string)
		if content, ok := message["content"].([]any); ok && content != nil {
			// 过滤 content 数组，移除 thinking 块中的 signature 字段
			cleanedContent := make([]any, 0, len(content))
			for _, item := range content {
				if contentBlock, ok := item.(map[string]any); ok {
					// 如果是 thinking 类型，移除 signature 字段
					if blockType, _ := contentBlock["type"].(string); blockType == "thinking" {
						cleanedBlock := make(map[string]any)
						for k, v := range contentBlock {
							if k != "signature" {
								cleanedBlock[k] = v
							}
						}
						cleanedContent = append(cleanedContent, cleanedBlock)
					} else {
						// 其他类型直接保留
						cleanedContent = append(cleanedContent, item)
					}
				} else {
					// 非 map 类型直接保留
					cleanedContent = append(cleanedContent, item)
				}
			}
			msg.Content = cleanedContent
		}
		if usage, ok := message["usage"].(map[string]any); ok {
			msg.Usage = &types.UsageInfo{}
			// cache 相关字段
			if v, ok := usage["cache_creation_input_tokens"].(int); ok {
				msg.Usage.CacheCreationInputTokens = v
			} else if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
				msg.Usage.CacheCreationInputTokens = int(v)
			}
			if v, ok := usage["cache_read_input_tokens"].(int); ok {
				msg.Usage.CacheReadInputTokens = v
			} else if v, ok := usage["cache_read_input_tokens"].(float64); ok {
				msg.Usage.CacheReadInputTokens = int(v)
			}
			// 基础 token 字段
			if v, ok := usage["input_tokens"].(int); ok {
				msg.Usage.InputTokens = v
			} else if v, ok := usage["input_tokens"].(float64); ok {
				msg.Usage.InputTokens = int(v)
			}
			if v, ok := usage["output_tokens"].(int); ok {
				msg.Usage.OutputTokens = v
			} else if v, ok := usage["output_tokens"].(float64); ok {
				msg.Usage.OutputTokens = int(v)
			}
		}
	}
	return types.NewMessageStartEvent(msg)
}


func convertContentBlockStart(m map[string]any) *types.ContentBlockStartEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}

	// 默认为空文本块，确保 content_block 不为 null
	var block any = &types.SSETextContentBlock{
		Type: "text",
		Text: "",
	}
	if cb, ok := m["content_block"].(map[string]any); ok {
		blockType, _ := cb["type"].(string)
		if blockType == "text" {
			// 文本块：始终包含 text 字段（即使为空）
			text, _ := cb["text"].(string)
			block = &types.SSETextContentBlock{
				Type: "text",
				Text: text,
			}
		} else if blockType == "tool_use" {
			// 工具使用块：使用专用结构体确保 input 字段始终存在
			toolBlock := &types.SSEToolUseContentBlock{
				Type:  "tool_use",
				Input: map[string]any{}, // 确保 input 不为 null
			}
			toolBlock.ID, _ = cb["id"].(string)
			toolBlock.Name, _ = cb["name"].(string)
			if input, exists := cb["input"]; exists && input != nil {
				toolBlock.Input = input
			}
			block = toolBlock
		} else if blockType != "" {
			// 其他已知类型
			sseBlock := &types.SSEContentBlock{}
			sseBlock.Type = blockType
			sseBlock.Text, _ = cb["text"].(string)
			sseBlock.ID, _ = cb["id"].(string)
			sseBlock.Name, _ = cb["name"].(string)
			if input, exists := cb["input"]; exists {
				sseBlock.Input = input
			}
			block = sseBlock
		}
	}
	return types.NewContentBlockStartEvent(index, block)
}

func convertContentBlockDelta(m map[string]any) *types.ContentBlockDeltaEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}

	// 解析 delta 类型
	deltaType := "text_delta"
	text := ""
	partialJSON := ""
	if d, ok := m["delta"].(map[string]any); ok {
		if t, ok := d["type"].(string); ok && t != "" {
			deltaType = t
		}
		text, _ = d["text"].(string)
		partialJSON, _ = d["partial_json"].(string)
	}

	// 根据 delta 类型返回相应的结构体（确保字段始终存在）
	var delta any
	switch deltaType {
	case "input_json_delta":
		delta = &types.InputJSONDeltaBlock{
			Type:        deltaType,
			PartialJSON: partialJSON,
		}
	default:
		// text_delta 或其他类型
		delta = &types.TextDeltaBlock{
			Type: deltaType,
			Text: text,
		}
	}

	return types.NewContentBlockDeltaEvent(index, delta)
}

func convertContentBlockStop(m map[string]any) *types.ContentBlockStopEvent {
	index := 0
	if v, ok := m["index"].(int); ok {
		index = v
	} else if v, ok := m["index"].(float64); ok {
		index = int(v)
	}
	return types.NewContentBlockStopEvent(index)
}

func convertMessageDelta(m map[string]any) *types.MessageDeltaEvent {
	stopReason := ""
	if delta, ok := m["delta"].(map[string]any); ok {
		stopReason, _ = delta["stop_reason"].(string)
	}

	var usage *types.UsageInfo
	if u, ok := m["usage"].(map[string]any); ok {
		usage = &types.UsageInfo{}
		// cache 相关字段
		if v, ok := u["cache_creation_input_tokens"].(int); ok {
			usage.CacheCreationInputTokens = v
		} else if v, ok := u["cache_creation_input_tokens"].(float64); ok {
			usage.CacheCreationInputTokens = int(v)
		}
		if v, ok := u["cache_read_input_tokens"].(int); ok {
			usage.CacheReadInputTokens = v
		} else if v, ok := u["cache_read_input_tokens"].(float64); ok {
			usage.CacheReadInputTokens = int(v)
		}
		// 基础 token 字段
		if v, ok := u["input_tokens"].(int); ok {
			usage.InputTokens = v
		} else if v, ok := u["input_tokens"].(float64); ok {
			usage.InputTokens = int(v)
		}
		if v, ok := u["output_tokens"].(int); ok {
			usage.OutputTokens = v
		} else if v, ok := u["output_tokens"].(float64); ok {
			usage.OutputTokens = int(v)
		}
	}
	return types.NewMessageDeltaEvent(stopReason, usage)
}

func convertError(m map[string]any) *types.ErrorEvent {
	errType := "error"
	errMsg := ""
	if e, ok := m["error"].(map[string]any); ok {
		errType, _ = e["type"].(string)
		errMsg, _ = e["message"].(string)
	}
	return types.NewErrorEvent(errType, errMsg)
}

func (s *AnthropicStreamSender) SendError(c *gin.Context, message string, _ error) error {
	return s.SendEvent(c, types.NewErrorEvent("overloaded_error", message))
}

// RequestContext 请求处理上下文，封装通用的请求处理逻辑
type RequestContext struct {
	GinContext  *gin.Context
	AuthService interface {
		GetToken() (types.TokenInfo, error)
		GetTokenWithUsage() (*types.TokenWithUsage, error)
	}
	RequestType string // "Anthropic"
}

// GetTokenAndBody 通用的token获取和请求体读取
// 返回: tokenInfo, requestBody, error
func (rc *RequestContext) GetTokenAndBody() (types.TokenInfo, []byte, error) {
	// 获取token
	tokenInfo, err := rc.AuthService.GetToken()
	if err != nil {
		utils.Error("获取token失败: %v", err)
		respondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	// 读取请求体
	body, err := rc.GinContext.GetRawData()
	if err != nil {
		utils.Error("读取请求体失败: %v", err)
		respondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return types.TokenInfo{}, nil, err
	}

	return tokenInfo, body, nil
}

// GetTokenWithUsageAndBody 获取token（包含使用信息）和请求体
// 返回: tokenWithUsage, requestBody, error
func (rc *RequestContext) GetTokenWithUsageAndBody() (*types.TokenWithUsage, []byte, error) {
	// 获取token（包含使用信息）
	tokenWithUsage, err := rc.AuthService.GetTokenWithUsage()
	if err != nil {
		utils.Error("获取token失败: %v", err)
		respondError(rc.GinContext, http.StatusInternalServerError, "获取token失败: %v", err)
		return nil, nil, err
	}

	// 读取请求体
	body, err := rc.GinContext.GetRawData()
	if err != nil {
		utils.Error("读取请求体失败: %v", err)
		respondError(rc.GinContext, http.StatusBadRequest, "读取请求体失败: %v", err)
		return nil, nil, err
	}

	return tokenWithUsage, body, nil
}
