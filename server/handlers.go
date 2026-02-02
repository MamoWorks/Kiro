package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro/cache"
	"kiro/config"

	"kiro/parser"
	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// extractRelevantHeaders 提取相关的请求头信息
func extractRelevantHeaders(c *gin.Context) map[string]string {
	relevantHeaders := map[string]string{}

	// 提取关键的请求头
	headerKeys := []string{
		"Content-Type",
		"Authorization",
		"X-API-Key",
		"X-Request-ID",
		"X-Forwarded-For",
		"Accept",
		"Accept-Encoding",
	}

	for _, key := range headerKeys {
		if value := c.GetHeader(key); value != "" {
			// 对敏感信息进行脱敏处理
			if key == "Authorization" && len(value) > 20 {
				relevantHeaders[key] = value[:10] + "***" + value[len(value)-7:]
			} else if key == "X-API-Key" && len(value) > 10 {
				relevantHeaders[key] = value[:5] + "***" + value[len(value)-3:]
			} else {
				relevantHeaders[key] = value
			}
		}
	}

	return relevantHeaders
}

// handleStreamRequest 处理流式请求
func handleStreamRequest(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	sender := &AnthropicStreamSender{}
	handleGenericStreamRequest(c, anthropicReq, token, sender, createAnthropicStreamEvents)
}

// handleGenericStreamRequest 通用流式请求处理
func handleGenericStreamRequest(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo, sender StreamEventSender, eventCreator func(string, int, string, *cache.CacheResult) []map[string]any) {
	// 计算输入tokens（基于实际发送给上游的数据）
	estimator := utils.NewTokenEstimator()
	countReq := &types.CountTokensRequest{
		Model:    anthropicReq.Model,
		System:   anthropicReq.System,
		Messages: anthropicReq.Messages,
		Tools:    filterSupportedTools(anthropicReq.Tools), // 过滤不支持的工具后计算
	}
	inputTokens := estimator.EstimateTokens(countReq)

	// 执行缓存处理
	cacheResult := cache.ProcessRequest(anthropicReq, inputTokens)

	// 生成消息ID并注入上下文
	messageID := fmt.Sprintf(config.MessageIDFormat, time.Now().Format(config.MessageIDTimeFormat))
	c.Set("message_id", messageID)

	// 先执行上游请求，确保成功后再建立 SSE 连接
	resp, err := execCWRequest(c, anthropicReq, token, true)
	if err != nil {
		var modelNotFoundErrorType *types.ModelNotFoundErrorType
		if errors.As(err, &modelNotFoundErrorType) {
			return
		}
		// 上游请求失败，返回 HTTP 错误（不建立 SSE 连接）
		var upstreamErr *UpstreamError
		if errors.As(err, &upstreamErr) {
			respondErrorWithCode(c, upstreamErr.StatusCode, "upstream_error", "%s", upstreamErr.Message)
		} else {
			respondError(c, http.StatusBadGateway, "%s", err.Error())
		}
		return
	}
	defer resp.Body.Close()

	// 上游成功，初始化 SSE 响应
	if err := initializeSSEResponse(c); err != nil {
		resp.Body.Close()
		respondError(c, http.StatusInternalServerError, "连接不支持SSE: %v", err)
		return
	}

	// 创建流处理上下文
	ctx := NewStreamProcessorContext(c, anthropicReq, token, sender, messageID, inputTokens, cacheResult)
	defer ctx.Cleanup()

	// 发送初始事件
	if err := ctx.sendInitialEvents(eventCreator); err != nil {
		return
	}

	// 处理事件流
	processor := NewEventStreamProcessor(ctx)
	if err := processor.ProcessEventStream(resp.Body); err != nil {
		utils.Log("事件流处理失败", utils.LogErr(err))
		return
	}

	// 发送结束事件
	if err := ctx.sendFinalEvents(); err != nil {
		utils.Log("发送结束事件失败", utils.LogErr(err))
		return
	}

	// 日志输出缓存统计
	logCacheResult(cacheResult, inputTokens, ctx.totalOutputTokens, true)
}

// createAnthropicStreamEvents 创建Anthropic流式初始事件
func createAnthropicStreamEvents(messageId string, inputTokens int, model string, cacheResult *cache.CacheResult) []map[string]any {
	// 构建 usage 对象
	usage := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": 0,
	}
	if cacheResult != nil {
		if cacheResult.CacheCreationTokens > 0 {
			usage["cache_creation_input_tokens"] = cacheResult.CacheCreationTokens
		}
		if cacheResult.CacheReadTokens > 0 {
			usage["cache_read_input_tokens"] = cacheResult.CacheReadTokens
		}
	}

	// 创建基础初始事件序列
	// 注意：ping 事件在 sse_state_manager 中第一个 content_block_start 之后发送
	// 这与官方 Claude API 顺序一致：message_start -> content_block_start -> ping
	events := []map[string]any{
		{
			"type": "message_start",
			"message": map[string]any{
				"id":            messageId,
				"type":          "message",
				"role":          "assistant",
				"content":       []any{},
				"model":         model,
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage":         usage,
			},
		},
	}
	return events
}

// createAnthropicFinalEvents 创建Anthropic流式结束事件
func createAnthropicFinalEvents(outputTokens, inputTokens int, stopReason string) []map[string]any {
	// 删除硬编码的content_block_stop，依赖sendFinalEvents的动态保护机制
	// sendFinalEvents在调用本函数前已经自动关闭所有未关闭的content_block（stream_processor.go:353-365）
	// 这样避免了重复发送content_block_stop导致的违规错误
	//
	// 三重保护机制确保不会缺失content_block_stop：
	// 1. ProcessEventStream正常转发上游的stop事件（99%场景）
	// 2. sendFinalEvents遍历所有activeBlocks并补发缺失的stop（容错机制，100%覆盖）
	// 3. handleMessageDelta在发送message_delta前的最后检查（最后保险）
	events := []map[string]any{
		{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   stopReason,
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"output_tokens": outputTokens,
				"input_tokens":  inputTokens,
			},
		},
		{
			"type": "message_stop",
		},
	}

	return events
}

// handleNonStreamRequest 处理非流式请求
func handleNonStreamRequest(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	// 计算输入tokens（基于实际发送给上游的数据）
	estimator := utils.NewTokenEstimator()
	countReq := &types.CountTokensRequest{
		Model:    anthropicReq.Model,
		System:   anthropicReq.System,
		Messages: anthropicReq.Messages,
		Tools:    filterSupportedTools(anthropicReq.Tools), // 过滤不支持的工具后计算
	}
	inputTokens := estimator.EstimateTokens(countReq)

	// 执行缓存处理
	cacheResult := cache.ProcessRequest(anthropicReq, inputTokens)

	resp, err := executeCodeWhispererRequest(c, anthropicReq, token, false)
	if err != nil {
		return
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// 读取响应体
	body, err := utils.ReadHTTPResponse(resp.Body)
	if err != nil {
		handleResponseReadError(c, err)
		return
	}

	// 使用新的符合AWS规范的解析器，但在非流式模式下增加超时保护
	compliantParser := parser.NewCompliantEventStreamParser()
	compliantParser.SetMaxErrors(config.ParserMaxErrors) // 限制最大错误次数以防死循环

	// 为非流式解析添加超时保护
	result, err := func() (*parser.ParseResult, error) {
		done := make(chan struct{})
		var result *parser.ParseResult
		var err error

		go func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("解析器panic: %v", r)
				}
				close(done)
			}()
			result, err = compliantParser.ParseResponse(body)
		}()

		select {
		case <-done:
			return result, err
		case <-time.After(600 * time.Second): // 600秒超时
			utils.Log("非流式解析超时")
			return nil, fmt.Errorf("解析超时")
		}
	}()

	if err != nil {
		utils.Log("非流式解析失败",
			utils.LogErr(err),
			utils.LogString("model", anthropicReq.Model),
			utils.LogInt("response_size", len(body)))

		// 提供更详细的错误信息和建议
		errorResp := gin.H{
			"error":   "响应解析失败",
			"type":    "parsing_error",
			"message": "无法解析AWS CodeWhisperer响应格式",
		}

		// 根据错误类型提供不同的HTTP状态码
		statusCode := http.StatusInternalServerError
		if strings.Contains(err.Error(), "解析超时") {
			statusCode = http.StatusRequestTimeout
			errorResp["message"] = "请求处理超时，请稍后重试"
		} else if strings.Contains(err.Error(), "格式错误") {
			statusCode = http.StatusBadRequest
			errorResp["message"] = "请求格式不正确"
		}

		c.JSON(statusCode, errorResp)
		return
	}

	// 转换为Anthropic格式
	var contexts []map[string]any
	textAgg := result.GetCompletionText()

	// 检查是否启用了 thinking 模式
	thinkingEnabled := anthropicReq.Thinking != nil && anthropicReq.Thinking.Type == "enabled"

	// 先获取工具管理器的所有工具，确保sawToolUse的判断基于实际工具
	toolManager := compliantParser.GetToolManager()
	allTools := make([]*parser.ToolExecution, 0)

	// 获取活跃工具
	for _, tool := range toolManager.GetActiveTools() {
		allTools = append(allTools, tool)
	}

	// 获取已完成工具
	for _, tool := range toolManager.GetCompletedTools() {
		allTools = append(allTools, tool)
	}

	// 基于实际工具数量判断是否包含工具调用
	sawToolUse := len(allTools) > 0

	// utils.Log("非流式响应处理完成",
	// 	addReqFields(c,
	// 		utils.LogString("text_content", textAgg[:utils.IntMin(config.LogPreviewMaxLength, len(textAgg))]),
	// 		utils.LogInt("tool_calls_count", len(allTools)),
	// 		utils.LogBool("saw_tool_use", sawToolUse),
	// 	)...)

	// 添加文本内容（如果启用 thinking 模式，需要提取 thinking 块）
	if textAgg != "" {
		if thinkingEnabled {
			// 提取 thinking 内容
			thinkingBlocks, cleanText := ExtractThinkingFromFinalText(textAgg)

			// 合并所有 thinking 块为一个
			if len(thinkingBlocks) > 0 {
				mergedThinking := strings.Join(thinkingBlocks, "\n\n")
				if mergedThinking != "" {
					contexts = append(contexts, map[string]any{
						"type":      "thinking",
						"thinking":  mergedThinking,
						"signature": GenerateFakeSignature(len(mergedThinking)),
					})
				}
			}

			// 添加清理后的文本（如果有）
			if cleanText != "" {
				contexts = append(contexts, map[string]any{
					"type": "text",
					"text": cleanText,
				})
			}
		} else {
			// 非 thinking 模式，直接添加文本
			contexts = append(contexts, map[string]any{
				"type": "text",
				"text": textAgg,
			})
		}
	}

	// 添加工具调用
	// 工具已经在前面从toolManager获取到allTools中
	// utils.Log("从工具生命周期管理器获取工具调用",
	// 	utils.LogInt("total_tools", len(allTools)),
	// 	utils.LogInt("parse_result_tools", len(result.GetToolCalls())))

	for _, tool := range allTools {
		// utils.Log("添加工具调用到响应",
		// 	utils.LogString("tool_id", tool.ID),
		// 	utils.LogString("tool_name", tool.Name),
		// 	utils.LogString("tool_status", tool.Status.String()),
		// 	utils.LogAny("tool_arguments", tool.Arguments))

		// 创建标准的tool_use块，确保包含完整的状态信息
		toolUseBlock := map[string]any{
			"type":  "tool_use",
			"id":    tool.ID,
			"name":  tool.Name,
			"input": tool.Arguments,
		}

		// 如果工具参数为空或nil，确保为空对象而不是nil
		if tool.Arguments == nil {
			toolUseBlock["input"] = map[string]any{}
		}

		// 添加详细的调试日志，验证tool_use块格式
		// if toolUseBlockJSON, err := utils.SafeMarshal(toolUseBlock); err == nil {
		// 	utils.Log("发送给Claude CLI的tool_use块详细结构",
		// 		utils.LogString("tool_id", tool.ID),
		// 		utils.LogString("tool_name", tool.Name),
		// 		utils.LogString("tool_use_json", string(toolUseBlockJSON)),
		// 		utils.LogString("input_type", fmt.Sprintf("%T", tool.Arguments)),
		// 		utils.LogAny("arguments_value", tool.Arguments))
		// }

		contexts = append(contexts, toolUseBlock)

		// 记录工具调用完成状态，帮助客户端识别工具调用已完成
		// utils.Log("工具调用已添加到响应",
		// 	utils.LogString("tool_id", tool.ID),
		// 	utils.LogString("tool_name", tool.Name))
	}

	// 使用新的stop_reason管理器，确保符合Claude官方规范
	stopReasonManager := NewStopReasonManager(anthropicReq)

	// *** 关键修复：基于实际发送给客户端的内容计算 token ***
	// 设计原则：token 计费应该基于实际下发的内容，而不是上游原始数据
	// 原因：
	// 1. 格式转换：CodeWhisperer → Claude 格式可能有差异
	// 2. 计费准确性：客户端消费的是 contexts，而不是 textAgg/allTools
	// 3. 一致性：确保 token 计算与实际响应内容完全一致
	outputTokens := 0
	for _, contentBlock := range contexts {
		blockType, _ := contentBlock["type"].(string)

		switch blockType {
		case "text":
			// 文本块：基于实际发送的文本内容
			if text, ok := contentBlock["text"].(string); ok {
				outputTokens += estimator.EstimateTextTokens(text)
			}

		case "tool_use":
			// 工具调用块：基于实际发送的工具名称和参数
			// 这里使用与 SSE 响应相同的 token 计算逻辑
			toolName, _ := contentBlock["name"].(string)
			toolInput, _ := contentBlock["input"].(map[string]any)
			outputTokens += estimator.EstimateToolUseTokens(toolName, toolInput)
		}
	}

	// 最小 token 保护：确保非空响应至少有 1 token
	if outputTokens < 1 && len(contexts) > 0 {
		outputTokens = 1
	}

	stopReasonManager.UpdateToolCallStatus(sawToolUse, sawToolUse)
	stopReason := stopReasonManager.DetermineStopReason()

	// utils.Log("非流式响应stop_reason决策",
	// 	utils.LogString("stop_reason", stopReason),
	// 	utils.LogString("description", GetStopReasonDescription(stopReason)),
	// 	utils.LogBool("saw_tool_use", sawToolUse),
	// 	utils.LogInt("output_tokens", outputTokens))

	// 构建 usage 对象
	usageMap := map[string]any{
		"input_tokens":  inputTokens,
		"output_tokens": outputTokens,
	}
	if cacheResult != nil {
		if cacheResult.CacheCreationTokens > 0 {
			usageMap["cache_creation_input_tokens"] = cacheResult.CacheCreationTokens
		}
		if cacheResult.CacheReadTokens > 0 {
			usageMap["cache_read_input_tokens"] = cacheResult.CacheReadTokens
		}
	}

	anthropicResp := map[string]any{
		"content":       contexts,
		"model":         anthropicReq.Model,
		"role":          "assistant",
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"type":          "message",
		"usage":         usageMap,
	}

	// utils.Log("非流式响应最终数据",
	// 	utils.LogString("stop_reason", stopReason),
	// 	utils.LogInt("content_blocks", len(contexts)))

	utils.Log("下发非流式响应",
		addReqFields(c,
			utils.LogString("direction", "downstream_send"),
			utils.LogAny("contexts", contexts),
			utils.LogBool("saw_tool_use", sawToolUse),
			utils.LogInt("content_count", len(contexts)),
		)...)
	c.JSON(http.StatusOK, anthropicResp)

	// 日志输出缓存统计
	logCacheResult(cacheResult, inputTokens, outputTokens, false)
}

// createTokenPreview 创建token预览显示格式 (***+后10位)
func createTokenPreview(token string) string {
	if len(token) <= 10 {
		// 如果token太短，全部用*代替
		return strings.Repeat("*", len(token))
	}

	// 3个*号 + 后10位
	suffix := token[len(token)-10:]
	return "***" + suffix
}

// maskEmail 对邮箱进行脱敏处理
// 规则：
// - 用户名部分：保留前2位和后2位，中间用星号替换
// - 域名部分：保留顶级域名和二级域名后缀，其他用星号替换
// 示例：
//   - caidaoli@gmail.com -> ca****li@*****.com
//   - caidaolihz888@sun.edu.pl -> ca*********88@***.**.pl
func maskEmail(email string) string {
	if email == "" {
		return ""
	}

	// 分割邮箱为用户名和域名
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		// 不是有效的邮箱格式，返回原值
		return email
	}

	username := parts[0]
	domain := parts[1]

	// 处理用户名部分：保留前2位和后2位
	var maskedUsername string
	if len(username) <= 4 {
		// 用户名太短，全部用星号替换
		maskedUsername = strings.Repeat("*", len(username))
	} else {
		prefix := username[:2]
		suffix := username[len(username)-2:]
		middleLen := len(username) - 4
		maskedUsername = prefix + strings.Repeat("*", middleLen) + suffix
	}

	// 处理域名部分：保留顶级域名和二级域名后缀
	domainParts := strings.Split(domain, ".")
	var maskedDomain string

	if len(domainParts) == 1 {
		// 只有一级域名（不常见），全部用星号替换
		maskedDomain = strings.Repeat("*", len(domain))
	} else if len(domainParts) == 2 {
		// 二级域名（如 gmail.com）
		// 主域名用星号替换，保留顶级域名
		maskedDomain = strings.Repeat("*", len(domainParts[0])) + "." + domainParts[1]
	} else {
		// 三级或更多级域名（如 sun.edu.pl）
		// 保留后两级域名，其他用星号替换
		maskedParts := make([]string, len(domainParts))
		for i := 0; i < len(domainParts)-2; i++ {
			maskedParts[i] = strings.Repeat("*", len(domainParts[i]))
		}
		// 保留最后两级
		maskedParts[len(domainParts)-2] = domainParts[len(domainParts)-2]
		maskedParts[len(domainParts)-1] = domainParts[len(domainParts)-1]
		maskedDomain = strings.Join(maskedParts, ".")
	}

	return maskedUsername + "@" + maskedDomain
}

// logCacheResult 输出缓存统计日志
func logCacheResult(cacheResult *cache.CacheResult, inputTokens, outputTokens int, isStream bool) {
	mode := "非流式"
	if isStream {
		mode = "流式"
	}

	cacheCreation := 0
	cacheRead := 0
	if cacheResult != nil {
		cacheCreation = cacheResult.CacheCreationTokens
		cacheRead = cacheResult.CacheReadTokens
	}

	utils.Info("请求完成 [%s] | input: %d, output: %d, cache_creation: %d, cache_read: %d",
		mode, inputTokens, outputTokens, cacheCreation, cacheRead)
}
