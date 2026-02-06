package converter

import (
	"fmt"
	"strings"

	"kiro/config"

	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// agenticSystemPrompt 用于防止大文件写入超时的系统提示
const agenticSystemPrompt = `
# CRITICAL: CHUNKED WRITE PROTOCOL (MANDATORY)

- **MAXIMUM 350 LINES** per single write/edit operation
- AWS Kiro API has a 2-3 minute timeout for large file write operations
- If you need to write more than 350 lines, split into multiple operations
- For new files: Create with first chunk, then append remaining chunks
- For edits: Make multiple targeted edits instead of one large replacement
`

// ValidateAssistantResponseEvent 验证助手响应事件
// ConvertToAssistantResponseEvent 转换任意数据为标准的AssistantResponseEvent
// NormalizeAssistantResponseEvent 标准化助手响应事件（填充默认值等）
// normalizeWebLinks 标准化网页链接
// normalizeReferences 标准化引用
// CodeWhisperer格式转换器

// getLastUserMessageContent 获取最后一条用户消息的文本内容
func getLastUserMessageContent(messages []types.AnthropicRequestMessage) string {
	// 从后向前查找最后一条用户消息
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return extractTextFromContent(messages[i].Content)
		}
	}
	return ""
}

// extractTextFromContent 从消息内容中提取文本（支持 string 和 []ContentBlock）
func extractTextFromContent(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		// 处理 []any 格式（JSON 解析后的格式）
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if blockType, exists := block["type"].(string); exists && blockType == "text" {
					if text, exists := block["text"].(string); exists {
						return text
					}
				}
			}
		}
	case []types.ContentBlock:
		// 处理 []ContentBlock 格式
		for _, block := range v {
			if block.Type == "text" && block.Text != nil {
				return *block.Text
			}
		}
	}
	return ""
}

// isAgenticMode 检查是否应启用 Agentic 模式（最后一条用户消息以 "-agent" 开头）
func isAgenticMode(messages []types.AnthropicRequestMessage) bool {
	content := getLastUserMessageContent(messages)
	return strings.HasPrefix(strings.TrimSpace(content), "-agent")
}

// buildEnhancedSystemPrompt 构建增强的系统提示（包含 Thinking、Agentic 注入）
func buildEnhancedSystemPrompt(anthropicReq types.AnthropicRequest) string {
	var systemPrompt strings.Builder

	// 1. 添加原有的系统提示
	if len(anthropicReq.System) > 0 {
		for _, sysMsg := range anthropicReq.System {
			content, err := utils.GetMessageContent(sysMsg)
			if err == nil && content != "" {
				systemPrompt.WriteString(content)
				systemPrompt.WriteString("\n")
			}
		}
	}

	// 2. 注入 Agentic 模式提示（条件：最后一条用户消息以 "-agent" 开头）
	if isAgenticMode(anthropicReq.Messages) {
		systemPrompt.WriteString("\n")
		systemPrompt.WriteString(agenticSystemPrompt)
	}

	// 3. 注入 Thinking 模式提示（默认禁用，除非显式启用）
	shouldEnableThinking := false
	budgetTokens := 16000 // 默认值

	// 检查是否显式启用了 Thinking 模式
	if anthropicReq.Thinking != nil && anthropicReq.Thinking.Type == "enabled" {
		shouldEnableThinking = true
	}

	// 如果显式启用并指定了 budget_tokens，使用指定值
	if anthropicReq.Thinking != nil && anthropicReq.Thinking.BudgetTokens > 0 {
		budgetTokens = anthropicReq.Thinking.BudgetTokens
	}

	if shouldEnableThinking {
		systemPrompt.WriteString("\n")
		systemPrompt.WriteString(fmt.Sprintf("<thinking_mode>interleaved</thinking_mode><max_thinking_length>%d</max_thinking_length>", budgetTokens))
	}

	return strings.TrimSpace(systemPrompt.String())
}

// determineChatTriggerType 智能确定聊天触发类型 (SOLID-SRP: 单一责任)
func determineChatTriggerType(anthropicReq types.AnthropicRequest) string {
	// 如果有工具调用，通常是自动触发的
	if len(anthropicReq.Tools) > 0 {
		// 检查tool_choice是否强制要求使用工具
		if anthropicReq.ToolChoice != nil {
			if tc, ok := anthropicReq.ToolChoice.(*types.ToolChoice); ok && tc != nil {
				if tc.Type == "any" || tc.Type == "tool" {
					return "AUTO" // 自动工具调用
				}
			} else if tcMap, ok := anthropicReq.ToolChoice.(map[string]any); ok {
				if tcType, exists := tcMap["type"].(string); exists {
					if tcType == "any" || tcType == "tool" {
						return "AUTO" // 自动工具调用
					}
				}
			}
		}
	}

	// 默认为手动触发
	return "MANUAL"
}

// validateCodeWhispererRequest 验证CodeWhisperer请求的完整性 (SOLID-SRP: 单一责任验证)
func validateCodeWhispererRequest(cwReq *types.CodeWhispererRequest) error {
	// 验证必需字段
	if cwReq.ConversationState.CurrentMessage.UserInputMessage.ModelId == "" {
		return fmt.Errorf("ModelId不能为空")
	}

	if cwReq.ConversationState.ConversationId == "" {
		return fmt.Errorf("ConversationId不能为空")
	}

	// 验证内容完整性 (KISS: 简化内容验证)
	trimmedContent := strings.TrimSpace(cwReq.ConversationState.CurrentMessage.UserInputMessage.Content)
	hasImages := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.Images) > 0
	hasTools := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools) > 0
	hasToolResults := len(cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults) > 0

	// 如果有工具结果，允许内容为空（这是工具执行后的反馈请求）
	if hasToolResults {
		return nil
	}

	// 如果没有内容但有工具，注入占位内容 (YAGNI: 只在需要时处理)
	if trimmedContent == "" && !hasImages && hasTools {
		placeholder := "执行工具任务"
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = placeholder
		trimmedContent = placeholder
	}

	// 验证至少有内容或图片
	if trimmedContent == "" && !hasImages {
		return fmt.Errorf("用户消息内容和图片都为空")
	}

	return nil
}

// extractToolResultsFromMessage 从消息内容中提取工具结果
func extractToolResultsFromMessage(content any) []types.ToolResult {
	var toolResults []types.ToolResult

	switch v := content.(type) {
	case []any:
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if blockType, exists := block["type"]; exists {
					if typeStr, ok := blockType.(string); ok && typeStr == "tool_result" {
						toolResult := types.ToolResult{}

						// 提取 tool_use_id
						if toolUseId, ok := block["tool_use_id"].(string); ok {
							toolResult.ToolUseId = toolUseId
						}

						// 提取 content - 转换为数组格式
						if content, exists := block["content"]; exists {
							// 将 content 转换为 []map[string]any 格式
							var contentArray []map[string]any

							// 处理不同的 content 格式
							switch c := content.(type) {
							case string:
								// 如果是字符串，包装成标准格式
								contentArray = []map[string]any{
									{"text": c},
								}
							case []any:
								// 如果已经是数组，只保留 text 字段，移除 type 等其他字段
								for _, item := range c {
									if m, ok := item.(map[string]any); ok {
										cleanedItem := make(map[string]any)
										if text, hasText := m["text"]; hasText {
											cleanedItem["text"] = text
										}
										if len(cleanedItem) > 0 {
											contentArray = append(contentArray, cleanedItem)
										}
									}
								}
							case map[string]any:
								// 如果是单个对象，包装成数组
								contentArray = []map[string]any{c}
							default:
								// 其他格式，尝试转换为字符串
								contentArray = []map[string]any{
									{"text": fmt.Sprintf("%v", c)},
								}
							}

							toolResult.Content = contentArray
						}

						// 确保 Content 不为空（上游 API 要求）
						if len(toolResult.Content) == 0 {
							toolResult.Content = []map[string]any{{"text": ""}}
						}

						// 提取 status (默认为 success)
						toolResult.Status = "success"
						if isError, ok := block["is_error"].(bool); ok && isError {
							toolResult.Status = "error"
							toolResult.IsError = true
						}

						toolResults = append(toolResults, toolResult)

						// utils.Log("提取到工具结果",
						// 	utils.LogString("tool_use_id", toolResult.ToolUseId),
						// 	utils.LogString("status", toolResult.Status),
						// 	utils.LogInt("content_items", len(toolResult.Content)))
					}
				}
			}
		}
	case []types.ContentBlock:
		for _, block := range v {
			if block.Type == "tool_result" {
				toolResult := types.ToolResult{}

				if block.ToolUseId != nil {
					toolResult.ToolUseId = *block.ToolUseId
				}

				// 处理 content
				if block.Content != nil {
					var contentArray []map[string]any

					switch c := block.Content.(type) {
					case string:
						contentArray = []map[string]any{
							{"text": c},
						}
					case []any:
						// 只保留 text 字段，移除 type 等其他字段
						for _, item := range c {
							if m, ok := item.(map[string]any); ok {
								cleanedItem := make(map[string]any)
								if text, hasText := m["text"]; hasText {
									cleanedItem["text"] = text
								}
								if len(cleanedItem) > 0 {
									contentArray = append(contentArray, cleanedItem)
								}
							}
						}
					case map[string]any:
						// 只保留 text 字段
						cleanedItem := make(map[string]any)
						if text, hasText := c["text"]; hasText {
							cleanedItem["text"] = text
						}
						if len(cleanedItem) > 0 {
							contentArray = []map[string]any{cleanedItem}
						}
					default:
						contentArray = []map[string]any{
							{"text": fmt.Sprintf("%v", c)},
						}
					}

					toolResult.Content = contentArray
				}

				// 确保 Content 不为空（上游 API 要求）
				if len(toolResult.Content) == 0 {
					toolResult.Content = []map[string]any{{"text": ""}}
				}

				// 设置 status
				toolResult.Status = "success"
				if block.IsError != nil && *block.IsError {
					toolResult.Status = "error"
					toolResult.IsError = true
				}

				toolResults = append(toolResults, toolResult)
			}
		}
	}

	return toolResults
}

// BuildCodeWhispererRequest 构建 CodeWhisperer 请求
func BuildCodeWhispererRequest(anthropicReq types.AnthropicRequest, ctx *gin.Context) (types.CodeWhispererRequest, error) {
	cwReq := types.CodeWhispererRequest{}

	// 智能设置ChatTriggerType (KISS: 简化逻辑但保持准确性)
	cwReq.ConversationState.ChatTriggerType = determineChatTriggerType(anthropicReq)

	// 使用 UUID 作为 conversationId
	if ctx != nil {
		cwReq.ConversationState.ConversationId = utils.GenerateStableConversationID(ctx)
	} else {
		cwReq.ConversationState.ConversationId = utils.GenerateUUID()
	}

	// 处理最后一条消息，包括图片
	if len(anthropicReq.Messages) == 0 {
		return cwReq, fmt.Errorf("消息列表为空")
	}

	lastMessage := anthropicReq.Messages[len(anthropicReq.Messages)-1]

	// 调试：记录原始消息内容
	// utils.Log("处理用户消息",
	// 	utils.LogString("role", lastMessage.Role),
	// 	utils.LogString("content_type", fmt.Sprintf("%T", lastMessage.Content)))

	textContent, images, err := processMessageContent(lastMessage.Content)
	if err != nil {
		return cwReq, fmt.Errorf("处理消息内容失败: %v", err)
	}

	// 构建增强的系统提示（包含 Thinking, Agentic 注入）
	enhancedSystemPrompt := buildEnhancedSystemPrompt(anthropicReq)

	// 只在当前消息带系统提示（用 <system_mode> 标签包裹）
	var finalContent strings.Builder
	if enhancedSystemPrompt != "" {
		finalContent.WriteString("<system_mode>")
		finalContent.WriteString(enhancedSystemPrompt)
		finalContent.WriteString("</system_mode>\n\n")
	}
	finalContent.WriteString(textContent)

	cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = finalContent.String()
	// 确保Images字段始终是数组，即使为空
	if len(images) > 0 {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Images = images
	} else {
		cwReq.ConversationState.CurrentMessage.UserInputMessage.Images = []types.CodeWhispererImage{}
	}

	// 新增：检查并处理 ToolResults
	if lastMessage.Role == "user" {
		toolResults := extractToolResultsFromMessage(lastMessage.Content)
		if len(toolResults) > 0 {
			cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.ToolResults = toolResults
			// 对于包含 tool_result 的请求，保留系统提示
			if enhancedSystemPrompt != "" {
				cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = "<system_mode>" + enhancedSystemPrompt + "</system_mode>"
			} else {
				cwReq.ConversationState.CurrentMessage.UserInputMessage.Content = ""
			}
		}
	}

	// 获取模型映射，如果不存在则直接透传原始模型ID
	modelId := config.ModelMap[anthropicReq.Model]
	if modelId == "" {
		modelId = anthropicReq.Model
	}
	cwReq.ConversationState.CurrentMessage.UserInputMessage.ModelId = modelId
	cwReq.ConversationState.CurrentMessage.UserInputMessage.Origin = "AI_EDITOR" // v0.4兼容性：固定使用AI_EDITOR

	// 处理 tools 信息 - 根据req.json实际结构优化工具转换
	if len(anthropicReq.Tools) > 0 {
		// utils.Log("开始处理工具配置",
		// 	utils.LogInt("tools_count", len(anthropicReq.Tools)),
		// 	utils.LogString("conversation_id", cwReq.ConversationState.ConversationId))

		var tools []types.CodeWhispererTool
		for _, tool := range anthropicReq.Tools {
			// 验证工具定义的完整性 (SOLID-SRP: 单一责任验证)
			if tool.Name == "" {
				continue
			}

			// 过滤不支持的工具：web_search (静默过滤，不发送到上游)
			if tool.Name == "web_search" || tool.Name == "websearch" {
				continue
			}

			// utils.Log("转换工具定义",
			// 	utils.LogInt("tool_index", i),
			// 	utils.LogString("tool_name", tool.Name),
			// utils.LogString("tool_description", tool.Description)
			// )

			// 根据req.json的实际结构，确保JSON Schema完整性
			cwTool := types.CodeWhispererTool{}
			cwTool.ToolSpecification.Name = tool.Name

			// 限制 description 长度为 10000 字符
			if len(tool.Description) > config.MaxToolDescriptionLength {
				cwTool.ToolSpecification.Description = tool.Description[:config.MaxToolDescriptionLength]
			} else {
				cwTool.ToolSpecification.Description = tool.Description
			}

			// 直接使用原始的InputSchema，避免过度处理 (恢复v0.4兼容性)
			cwTool.ToolSpecification.InputSchema = types.InputSchema{
				Json: tool.InputSchema,
			}
			tools = append(tools, cwTool)
		}

		// 工具配置放在 UserInputMessageContext.Tools 中 (符合req.json结构)
		cwReq.ConversationState.CurrentMessage.UserInputMessage.UserInputMessageContext.Tools = tools
	}

	// 构建历史消息（不带系统提示，系统提示只在当前消息中）
	if len(anthropicReq.Messages) > 1 || len(anthropicReq.Tools) > 0 {
		var history []any

		// 处理常规消息历史 (修复配对逻辑：合并连续user消息，然后与assistant配对)
		// 关键修复：收集连续的user消息并合并，遇到assistant时配对添加
		var userMessagesBuffer []types.AnthropicRequestMessage // 累积连续的user消息

		// 决定历史消息的循环边界
		// 关键修复：如果最后一条消息是assistant，应该将它加入历史（与前面的user配对）
		// 如果最后一条是user，它作为currentMessage，不加入历史
		historyEndIndex := len(anthropicReq.Messages) - 1
		if lastMessage.Role == "assistant" {
			historyEndIndex = len(anthropicReq.Messages) // 包含最后一条assistant
		}

		for i := 0; i < historyEndIndex; i++ {
			msg := anthropicReq.Messages[i]

			if msg.Role == "user" {
				// 收集user消息到缓冲区
				userMessagesBuffer = append(userMessagesBuffer, msg)
				continue
			}
			if msg.Role == "assistant" {
				// 遇到assistant，只有当有对应的user消息时才处理
				if len(userMessagesBuffer) > 0 {
					// 合并所有累积的user消息
					mergedUserMsg := types.HistoryUserMessage{}
					var contentParts []string
					var allImages []types.CodeWhispererImage
					var allToolResults []types.ToolResult

					for _, userMsg := range userMessagesBuffer {
						// 处理每个user消息的内容和图片
						messageContent, messageImages, err := processMessageContent(userMsg.Content)
						if err == nil && messageContent != "" {
							contentParts = append(contentParts, messageContent)
							if len(messageImages) > 0 {
								allImages = append(allImages, messageImages...)
							}
						}

						// 收集工具结果
						toolResults := extractToolResultsFromMessage(userMsg.Content)
						if len(toolResults) > 0 {
							allToolResults = append(allToolResults, toolResults...)
						}
					}

					// 设置合并后的内容
					mergedContent := strings.Join(contentParts, "\n")

					mergedUserMsg.UserInputMessage.Content = mergedContent
					if len(allImages) > 0 {
						mergedUserMsg.UserInputMessage.Images = allImages
					}
					if len(allToolResults) > 0 {
						mergedUserMsg.UserInputMessage.UserInputMessageContext.ToolResults = allToolResults
						// 如果历史用户消息包含工具结果，也将 content 设置为空字符串
						mergedUserMsg.UserInputMessage.Content = ""
						// utils.Log("历史用户消息包含工具结果",
						// 	utils.LogInt("merged_messages", len(userMessagesBuffer)),
						// 	utils.LogInt("tool_results_count", len(allToolResults)))
					}

					mergedUserMsg.UserInputMessage.ModelId = modelId
					mergedUserMsg.UserInputMessage.Origin = "AI_EDITOR"
					history = append(history, mergedUserMsg)

					// 清空缓冲区
					userMessagesBuffer = nil

					// 添加assistant消息（只在有配对的user时添加）
					assistantMsg := types.HistoryAssistantMessage{}
					assistantContent, err := utils.GetMessageContent(msg.Content)
					if err == nil {
						assistantMsg.AssistantResponseMessage.Content = assistantContent
					} else {
						assistantMsg.AssistantResponseMessage.Content = ""
					}

					// 提取助手消息中的工具调用
					toolUses := extractToolUsesFromMessage(msg.Content)
					if len(toolUses) > 0 {
						assistantMsg.AssistantResponseMessage.ToolUses = toolUses
					} else {
						assistantMsg.AssistantResponseMessage.ToolUses = nil
					}

					history = append(history, assistantMsg)
				} else if len(history) > 0 {
					// 孤立的assistant消息：合并到上一个assistant消息中
					lastHistoryIdx := len(history) - 1
					if lastAssistant, ok := history[lastHistoryIdx].(types.HistoryAssistantMessage); ok {
						// 合并内容
						additionalContent, err := utils.GetMessageContent(msg.Content)
						if err == nil && additionalContent != "" {
							if lastAssistant.AssistantResponseMessage.Content != "" {
								lastAssistant.AssistantResponseMessage.Content += "\n" + additionalContent
							} else {
								lastAssistant.AssistantResponseMessage.Content = additionalContent
							}
						}

						// 合并工具调用
						additionalToolUses := extractToolUsesFromMessage(msg.Content)
						if len(additionalToolUses) > 0 {
							lastAssistant.AssistantResponseMessage.ToolUses = append(
								lastAssistant.AssistantResponseMessage.ToolUses,
								additionalToolUses...,
							)
						}

						history[lastHistoryIdx] = lastAssistant
					}
				}
				// 如果history为空且buffer为空，完全孤立的assistant消息被忽略
			}
		}

		// 处理结尾的孤立user消息
		// 如果最后一条是user（作为currentMessage），buffer中可能还有倒数第二条及之前的孤立user消息
		// 这些孤立的user消息应该配对一个"OK"的assistant
		if len(userMessagesBuffer) > 0 {
			// 合并所有孤立的user消息
			mergedOrphanUserMsg := types.HistoryUserMessage{}
			var contentParts []string
			var allImages []types.CodeWhispererImage
			var allToolResults []types.ToolResult

			for _, userMsg := range userMessagesBuffer {
				messageContent, messageImages, err := processMessageContent(userMsg.Content)
				if err == nil && messageContent != "" {
					contentParts = append(contentParts, messageContent)
					if len(messageImages) > 0 {
						allImages = append(allImages, messageImages...)
					}
				}

				toolResults := extractToolResultsFromMessage(userMsg.Content)
				if len(toolResults) > 0 {
					allToolResults = append(allToolResults, toolResults...)
				}
			}

			mergedOrphanUserMsg.UserInputMessage.Content = strings.Join(contentParts, "\n")
			if len(allImages) > 0 {
				mergedOrphanUserMsg.UserInputMessage.Images = allImages
			}
			if len(allToolResults) > 0 {
				mergedOrphanUserMsg.UserInputMessage.UserInputMessageContext.ToolResults = allToolResults
				mergedOrphanUserMsg.UserInputMessage.Content = ""
			}

			mergedOrphanUserMsg.UserInputMessage.ModelId = modelId
			mergedOrphanUserMsg.UserInputMessage.Origin = "AI_EDITOR"
			history = append(history, mergedOrphanUserMsg)

			// 自动配对一个"OK"的assistant响应
			autoAssistantMsg := types.HistoryAssistantMessage{}
			autoAssistantMsg.AssistantResponseMessage.Content = "OK"
			autoAssistantMsg.AssistantResponseMessage.ToolUses = nil
			history = append(history, autoAssistantMsg)
		}

		cwReq.ConversationState.History = history
	}

	// 设置 InferenceConfig（参考 CLIProxyAPIPlus 格式）
	if anthropicReq.MaxTokens > 0 {
		cwReq.InferenceConfig = &types.InferenceConfig{
			MaxTokens: anthropicReq.MaxTokens,
		}
		// 如果指定了温度参数，也设置它
		if anthropicReq.Temperature != nil {
			cwReq.InferenceConfig.Temperature = *anthropicReq.Temperature
		}
	}

	// 最终验证请求完整性 (KISS: 简化验证逻辑)
	if err := validateCodeWhispererRequest(&cwReq); err != nil {
		return cwReq, fmt.Errorf("请求验证失败: %v", err)
	}

	return cwReq, nil
}

// extractToolUsesFromMessage 从助手消息内容中提取工具调用
func extractToolUsesFromMessage(content any) []types.ToolUseEntry {
	var toolUses []types.ToolUseEntry

	switch v := content.(type) {
	case []any:
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if blockType, exists := block["type"]; exists {
					if typeStr, ok := blockType.(string); ok && typeStr == "tool_use" {
						toolUse := types.ToolUseEntry{}

						// 提取 id 作为 ToolUseId
						if id, ok := block["id"].(string); ok {
							toolUse.ToolUseId = id
						}

						// 提取 name
						if name, ok := block["name"].(string); ok {
							toolUse.Name = name
						}

						// 过滤不支持的工具：web_search (静默过滤)
						if toolUse.Name == "web_search" || toolUse.Name == "websearch" {
							continue
						}

						// 提取 input
						if input, ok := block["input"].(map[string]any); ok {
							toolUse.Input = input
						} else {
							// 如果 input 不是 map 或不存在，设置为空对象
							toolUse.Input = map[string]any{}
						}

						toolUses = append(toolUses, toolUse)

						// utils.Log("提取到历史工具调用", utils.LogString("tool_id", toolUse.ToolUseId), utils.LogString("tool_name", toolUse.Name))
					}
				}
			}
		}
	case []types.ContentBlock:
		for _, block := range v {
			if block.Type == "tool_use" {
				toolUse := types.ToolUseEntry{}

				if block.ID != nil {
					toolUse.ToolUseId = *block.ID
				}

				if block.Name != nil {
					toolUse.Name = *block.Name
				}

				// 过滤不支持的工具：web_search (静默过滤)
				if toolUse.Name == "web_search" || toolUse.Name == "websearch" {
					continue
				}

				if block.Input != nil {
					switch inp := (*block.Input).(type) {
					case map[string]any:
						toolUse.Input = inp
					default:
						toolUse.Input = map[string]any{
							"value": inp,
						}
					}
				} else {
					toolUse.Input = map[string]any{}
				}

				toolUses = append(toolUses, toolUse)
			}
		}
	case string:
		// 如果是纯文本，不包含工具调用
		return nil
	}

	return toolUses
}
