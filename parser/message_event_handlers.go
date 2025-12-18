package parser

import (
	"kiro/utils"
	"strings"
)

// === 辅助函数 ===

// convertInputToString 将any类型的input转换为JSON字符串
func convertInputToString(input any) string {
	if input == nil {
		return "{}"
	}

	// 如果已经是字符串，直接返回
	if str, ok := input.(string); ok {
		return str
	}

	// 将对象转换为JSON字符串
	jsonBytes, err := utils.FastMarshal(input)
	if err != nil {
		utils.Log("转换input为JSON字符串失败", utils.LogErr(err))
		return "{}"
	}

	return string(jsonBytes)
}

// isToolCallEvent 检查是否为工具调用事件
func isToolCallEvent(payload []byte) bool {
	payloadStr := string(payload)
	return strings.Contains(payloadStr, "\"toolUseId\":") ||
		strings.Contains(payloadStr, "\"tool_use_id\":") ||
		strings.Contains(payloadStr, "\"name\":") && strings.Contains(payloadStr, "\"input\":")
}

// isStreamingResponse 检查是否为流式响应
func isStreamingResponse(event *FullAssistantResponseEvent) bool {
	// 检查是否包含部分内容或状态为进行中
	return event != nil && (event.MessageStatus == "IN_PROGRESS" || event.Content != "")
}

// === 事件处理器实现 ===

// CompletionEventHandler 处理代码补全事件
type CompletionEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *CompletionEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	content := ""
	if c, ok := data["content"].(string); ok {
		content = c
	}

	finishReason := ""
	if fr, ok := data["finish_reason"].(string); ok {
		finishReason = fr
	}

	// 处理工具调用
	var toolCalls []ToolCall
	if tcData, ok := data["tool_calls"].([]any); ok {
		for _, tc := range tcData {
			if tcMap, ok := tc.(map[string]any); ok {
				toolCall := ToolCall{}
				if id, ok := tcMap["id"].(string); ok {
					toolCall.ID = id
				}
				if tcType, ok := tcMap["type"].(string); ok {
					toolCall.Type = tcType
				}
				if function, ok := tcMap["function"].(map[string]any); ok {
					if name, ok := function["name"].(string); ok {
						toolCall.Function.Name = name
					}
					if args, ok := function["arguments"].(string); ok {
						toolCall.Function.Arguments = args
					}
				}
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}

	events := []SSEEvent{
		{
			Event: "completion",
			Data: map[string]any{
				"type":          "completion",
				"content":       content,
				"finish_reason": finishReason,
				"tool_calls":    toolCalls,
				"raw_data":      data,
			},
		},
	}

	return events, nil
}

// CompletionChunkEventHandler 处理流式补全事件
type CompletionChunkEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *CompletionChunkEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	content := ""
	if c, ok := data["content"].(string); ok {
		content = c
	}

	delta := ""
	if d, ok := data["delta"].(string); ok {
		delta = d
	}

	finishReason := ""
	if fr, ok := data["finish_reason"].(string); ok {
		finishReason = fr
	}

	// 累积完整内容
	h.processor.completionBuffer = append(h.processor.completionBuffer, content)

	// 使用delta作为实际的文本增量，如果没有则使用content
	textDelta := delta
	if textDelta == "" {
		textDelta = content
	}

	events := []SSEEvent{
		{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": textDelta,
				},
			},
		},
	}

	// 如果有完成原因，添加完成事件
	if finishReason != "" {
		events = append(events, SSEEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":          "content_block_stop",
				"index":         0,
				"finish_reason": finishReason,
			},
		})
	}

	return events, nil
}

// ToolCallRequestHandler 处理工具调用请求
type ToolCallRequestHandler struct {
	toolManager *ToolLifecycleManager
}

func (h *ToolCallRequestHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	// 从标准AWS事件格式解析工具调用信息
	toolCallID, _ := data["toolCallId"].(string)
	toolName, _ := data["toolName"].(string)

	// 如果没有直接的toolCallId，尝试解析input字段
	input := map[string]any{}
	if inputData, ok := data["input"].(map[string]any); ok {
		input = inputData
	}

	// 创建标准格式的工具调用请求
	toolCall := ToolCall{
		ID:   toolCallID,
		Type: "function",
		Function: ToolCallFunction{
			Name:      toolName,
			Arguments: "{}",
		},
	}

	// 将input转换为JSON字符串
	if len(input) > 0 {
		if argsJSON, err := utils.FastMarshal(input); err == nil {
			toolCall.Function.Arguments = string(argsJSON)
		}
	}

	request := ToolCallRequest{
		ToolCalls: []ToolCall{toolCall},
	}

	utils.Log("标准工具调用请求处理",
		utils.LogString("tool_id", toolCallID),
		utils.LogString("tool_name", toolName),
		utils.LogAny("input", input))

	return h.toolManager.HandleToolCallRequest(request), nil
}

// ToolCallErrorHandler 处理工具调用错误
type ToolCallErrorHandler struct {
	toolManager *ToolLifecycleManager
}

func (h *ToolCallErrorHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var errorInfo ToolCallError
	if err := utils.FastUnmarshal(message.Payload, &errorInfo); err != nil {
		return nil, err
	}

	return h.toolManager.HandleToolCallError(errorInfo), nil
}

// SessionStartHandler 处理会话开始事件
type SessionStartHandler struct {
	sessionManager *SessionManager
}

func (h *SessionStartHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	// 尝试多种会话ID字段名
	sessionID := ""
	if sid, ok := data["sessionId"].(string); ok {
		sessionID = sid
	} else if sid, ok := data["session_id"].(string); ok {
		sessionID = sid
	}

	if sessionID != "" {
		h.sessionManager.SetSessionID(sessionID)
		// 触发实际的会话开始
		h.sessionManager.StartSession()
	}

	return []SSEEvent{
		{
			Event: EventTypes.SESSION_START,
			Data:  data,
		},
	}, nil
}

// SessionEndHandler 处理会话结束事件
type SessionEndHandler struct {
	sessionManager *SessionManager
}

func (h *SessionEndHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(message.Payload, &data); err != nil {
		return nil, err
	}

	// 实际结束会话
	endEvents := h.sessionManager.EndSession()

	// 合并事件数据
	result := []SSEEvent{
		{
			Event: EventTypes.SESSION_END,
			Data:  data,
		},
	}

	// 添加会话管理器生成的结束事件
	result = append(result, endEvents...)

	return result, nil
}

// StandardAssistantResponseEventHandler 标准assistantResponseEvent处理器
type StandardAssistantResponseEventHandler struct {
	processor *CompliantMessageProcessor
}

func (h *StandardAssistantResponseEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	// 首先检查是否是工具调用相关的事件
	if isToolCallEvent(message.Payload) {
		utils.Log("检测到工具调用事件，使用聚合器处理")
		return h.handleToolCallEvent(message)
	}

	// 作为标准事件，优先尝试解析完整格式
	if fullEvent, err := parseFullAssistantResponseEvent(message.Payload); err == nil {
		// 对于流式响应，放宽验证要求
		if isStreamingResponse(fullEvent) {
			// utils.Log("检测到流式格式assistantResponseEvent，使用宽松验证")
			return h.handleStreamingEvent(fullEvent)
		}

		// utils.Log("检测到完整格式assistantResponseEvent，使用标准处理器")
		return h.handleFullAssistantEvent(fullEvent)
	}

	// 如果完整格式解析失败，回退到legacy格式处理
	utils.Log("完整格式解析失败，回退到legacy格式处理")
	return h.handleLegacyFormat(message.Payload)
}

// handleToolCallEvent 处理工具调用事件
func (h *StandardAssistantResponseEventHandler) handleToolCallEvent(message *EventStreamMessage) ([]SSEEvent, error) {
	// 直接处理工具调用事件
	var evt toolUseEvent
	if err := utils.FastUnmarshal(message.Payload, &evt); err != nil {
		utils.Log("解析工具调用事件失败", utils.LogErr(err))
		return []SSEEvent{}, nil
	}

	// 创建工具调用
	toolCall := ToolCall{
		ID:   evt.ToolUseId,
		Type: "function",
		Function: ToolCallFunction{
			Name:      evt.Name,
			Arguments: convertInputToString(evt.Input),
		},
	}

	request := ToolCallRequest{
		ToolCalls: []ToolCall{toolCall},
	}

	return h.processor.toolManager.HandleToolCallRequest(request), nil
}

// handleStreamingEvent 处理流式事件
func (h *StandardAssistantResponseEventHandler) handleStreamingEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
	// 处理流式响应事件
	var events []SSEEvent // 提取内容
	if event.Content != "" {
		events = append(events, SSEEvent{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": event.Content,
				},
			},
		})
	}

	return events, nil
}

// handleFullAssistantEvent 处理完整的assistant事件
func (h *StandardAssistantResponseEventHandler) handleFullAssistantEvent(event *FullAssistantResponseEvent) ([]SSEEvent, error) {
	// 处理完整的assistant响应事件
	var events []SSEEvent // 提取文本内容
	if event.Content != "" {
		events = append(events, SSEEvent{
			Event: "content_block_start",
			Data: map[string]any{
				"type":  "content_block_start",
				"index": 0,
				"content_block": map[string]any{
					"type": "text",
					"text": event.Content,
				},
			},
		})

		events = append(events, SSEEvent{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": event.Content,
				},
			},
		})

		events = append(events, SSEEvent{
			Event: "content_block_stop",
			Data: map[string]any{
				"type":  "content_block_stop",
				"index": 0,
			},
		})
	}

	return events, nil
}

// handleLegacyFormat 处理旧格式数据
func (h *StandardAssistantResponseEventHandler) handleLegacyFormat(payload []byte) ([]SSEEvent, error) {
	// 尝试作为简单文本处理
	payloadStr := strings.TrimSpace(string(payload))
	if payloadStr != "" && !strings.HasPrefix(payloadStr, "{") {
		// 简单文本内容
		return []SSEEvent{{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": payloadStr,
				},
			},
		}}, nil
	}

	// 尝试解析为JSON
	var data map[string]any
	if err := utils.FastUnmarshal(payload, &data); err != nil {
		utils.Log("无法解析legacy格式数据", utils.LogErr(err))
		return []SSEEvent{}, nil
	}

	// 基本处理
	var events []SSEEvent
	if content, ok := data["content"].(string); ok && content != "" {
		events = append(events, SSEEvent{
			Event: "content_block_delta",
			Data: map[string]any{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]any{
					"type": "text_delta",
					"text": content,
				},
			},
		})
	}

	return events, nil
}

// LegacyToolUseEventHandler 处理旧格式的工具使用事件
type LegacyToolUseEventHandler struct {
	toolManager *ToolLifecycleManager
	aggregator  *SonicStreamingJSONAggregator
}

// Handle 实现EventHandler接口
func (h *LegacyToolUseEventHandler) Handle(message *EventStreamMessage) ([]SSEEvent, error) {
	return h.handleToolCallEvent(message)
}

// handleToolCallEvent 在LegacyToolUseEventHandler中处理工具调用事件
func (h *LegacyToolUseEventHandler) handleToolCallEvent(message *EventStreamMessage) ([]SSEEvent, error) {
	// 尝试解析为工具使用事件
	var evt toolUseEvent
	if err := utils.FastUnmarshal(message.Payload, &evt); err != nil {
		utils.Log("解析工具调用事件失败",
			utils.LogErr(err),
			utils.LogString("payload", string(message.Payload)))
		return []SSEEvent{}, nil
	}

	// 验证必要字段
	if evt.Name == "" || evt.ToolUseId == "" {
		utils.Log("工具调用事件缺少必要字段",
			utils.LogString("name", evt.Name),
			utils.LogString("toolUseId", evt.ToolUseId))

		// 即使缺少字段，也尝试处理，避免完全丢弃
		if evt.Name == "" && evt.ToolUseId == "" {
			return []SSEEvent{}, nil // 完全无效的事件，直接跳过
		}
	}

	// 🔥 统一转换input，避免重复调用
	inputStr := convertInputToString(evt.Input)

	// *** 核心修复：区分一次性完整数据和流式分片数据 ***

	// 第一步：检查工具是否已经注册
	_, toolExists := h.toolManager.GetActiveTools()[evt.ToolUseId]

	if !toolExists {
		// 首次收到工具调用，先注册工具
		utils.Log("首次收到工具调用片段，先注册工具",
			utils.LogString("toolUseId", evt.ToolUseId),
			utils.LogString("name", evt.Name))

		// 创建初始工具调用请求（使用完整参数）
		toolCall := ToolCall{
			ID:   evt.ToolUseId,
			Type: "function",
			Function: ToolCallFunction{
				Name:      evt.Name,
				Arguments: inputStr, // 使用已转换的input参数
			},
		}

		request := ToolCallRequest{
			ToolCalls: []ToolCall{toolCall},
		}

		// 先注册工具到管理器
		events := h.toolManager.HandleToolCallRequest(request)

		// 🔥 核心修复：如果是stop事件且是首次注册，说明这是一次性完整数据
		// 已经在注册时使用了完整参数，无需再通过聚合器处理，直接返回
		if evt.Stop {
			utils.Log("首次注册即收到stop信号，使用完整参数，跳过聚合器",
				utils.LogString("toolUseId", evt.ToolUseId),
				utils.LogString("arguments", inputStr))
			return events, nil
		}

		// 🔥 关键修复：如果不是stop事件，说明后续还有数据片段
		// 但首次注册时的数据已经是完整的JSON对象（来自evt.Input），不是片段
		// 因此不应该通过聚合器处理，聚合器只处理后续的字符串片段

		// 如果不是stop事件，说明后续还有数据片段，返回注册事件，等待后续片段
		return events, nil
	}

	// 第二步：工具已存在，使用聚合器处理流式分片数据
	// 🔥 关键修复：只有在工具已注册且不是首次的情况下，才使用聚合器
	// 这避免了对已经完整的一次性数据进行二次处理

	// 🔥 核心修复：区分"无参数工具"和"stop信号无新数据"
	// 场景1：无参数工具 - 从头到尾都没有数据
	// 场景2：stop信号无新数据 - 已有完整数据，stop事件不带新数据

	if evt.Stop {
		// 收到stop信号，需要完成聚合
		// 🔥 关键：只传递空字符串，不传递"{}"，避免污染buffer
		complete, fullInput := h.aggregator.ProcessToolData(evt.ToolUseId, evt.Name, "", evt.Stop, -1)

		if complete {
			// 聚合完成，更新工具参数
			if fullInput != "" && fullInput != "{}" {
				var testArgs map[string]any
				if err := utils.FastUnmarshal([]byte(fullInput), &testArgs); err != nil {
					utils.Log("聚合后的工具调用参数JSON格式无效",
						utils.LogString("toolUseId", evt.ToolUseId),
						utils.LogString("fullInput", fullInput),
						utils.LogErr(err))
				} else {
					h.toolManager.UpdateToolArguments(evt.ToolUseId, testArgs)
				}
			}

			// 处理工具完成
			result := ToolCallResult{
				ToolCallID: evt.ToolUseId,
				Result:     "Tool execution completed via toolUseEvent",
			}
			return h.toolManager.HandleToolCallResult(result), nil
		}
	}

	// 如果是空数据但不是stop，返回空事件
	if inputStr == "" || inputStr == "{}" {
		return []SSEEvent{}, nil
	}

	// 🔥 使用聚合器处理流式JSON片段
	complete, _ := h.aggregator.ProcessToolData(evt.ToolUseId, evt.Name, inputStr, evt.Stop, -1)

	// 处理未完整数据时发送增量事件
	if !complete {
		// 如果有新的输入片段，发送参数增量事件
		if inputStr != "" && inputStr != "{}" {
			// 边界情况检查：确保工具ID有效
			if evt.ToolUseId == "" {
				utils.Log("工具调用片段缺少有效的toolUseId，跳过增量事件发送",
					utils.LogString("inputFragment", inputStr))
				return []SSEEvent{}, nil
			}

			// 获取工具的块索引
			toolIndex := h.toolManager.GetBlockIndex(evt.ToolUseId)
			if toolIndex >= 0 {
				return []SSEEvent{{
					Event: "content_block_delta",
					Data: map[string]any{
						"type":  "content_block_delta",
						"index": toolIndex,
						"delta": map[string]any{
							"type":         "input_json_delta",
							"partial_json": inputStr,
						},
					},
				}}, nil
			} else {
				// 工具未注册的边界情况（理论上不应该发生，因为上面已经检查过）
				utils.Log("尝试发送增量事件但工具未注册，可能存在时序问题",
					utils.LogString("toolUseId", evt.ToolUseId),
					utils.LogString("name", evt.Name),
					utils.LogString("inputFragment", inputStr))
			}
		}

		// 无新数据或无法获取索引，返回空事件
		return []SSEEvent{}, nil
	}

	// 非stop事件的流式片段处理完成，返回空事件
	return []SSEEvent{}, nil
}
