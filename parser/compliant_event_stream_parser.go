package parser

import (
	"fmt"
	"kiro/utils"
)

// CompliantEventStreamParser 符合AWS规范的完整事件流解析器
type CompliantEventStreamParser struct {
	robustParser     *RobustEventStreamParser
	messageProcessor *CompliantMessageProcessor
}

// NewCompliantEventStreamParser 创建符合规范的事件流解析器
func NewCompliantEventStreamParser() *CompliantEventStreamParser {
	return &CompliantEventStreamParser{
		robustParser:     NewRobustEventStreamParser(),
		messageProcessor: NewCompliantMessageProcessor(),
	}
}

// SetMaxErrors 设置最大错误次数
func (cesp *CompliantEventStreamParser) SetMaxErrors(maxErrors int) {
	cesp.robustParser.SetMaxErrors(maxErrors)
}

// Reset 重置解析器状态
func (cesp *CompliantEventStreamParser) Reset() {
	cesp.robustParser.Reset()
	cesp.messageProcessor.Reset()
}

// ParseResponse 解析完整的 CodeWhisperer 响应
func (cesp *CompliantEventStreamParser) ParseResponse(streamData []byte) (*ParseResult, error) {
	// 1. 解析二进制事件流
	messages, err := cesp.robustParser.ParseStream(streamData)
	if err != nil {
		utils.Log("事件流解析部分失败", utils.LogErr(err))
	}

	// 2. 处理消息
	var allEvents []SSEEvent
	var errors []error

	for i, message := range messages {
		events, processErr := cesp.messageProcessor.ProcessMessage(message)
		if processErr != nil {
			errMsg := fmt.Errorf("处理消息 %d 失败: %w", i, processErr)
			errors = append(errors, errMsg)
			utils.Log("消息处理失败",
				utils.LogInt("message_index", i),
				utils.LogString("message_type", message.GetMessageType()),
				utils.LogString("event_type", message.GetEventType()),
				utils.LogErr(processErr))
			continue
		}

		allEvents = append(allEvents, events...)
	}

	// 3. 构建结果
	result := &ParseResult{
		Messages:       messages,
		Events:         allEvents,
		ToolExecutions: cesp.messageProcessor.toolManager.GetCompletedTools(),
		ActiveTools:    cesp.messageProcessor.toolManager.GetActiveTools(),
		SessionInfo:    cesp.messageProcessor.sessionManager.GetSessionInfo(),
		Summary:        cesp.generateSummary(messages, allEvents),
		Errors:         errors,
	}

	if len(errors) > 0 {
		utils.Log("解析完成，但有部分错误",
			utils.LogInt("success_messages", len(messages)),
			utils.LogInt("total_events", len(allEvents)),
			utils.LogInt("error_count", len(errors)))
	}

	return result, nil
}

// ParseStream 解析流式数据（增量解析）
func (cesp *CompliantEventStreamParser) ParseStream(data []byte) ([]SSEEvent, error) {
	// 解析新的消息
	messages, err := cesp.robustParser.ParseStream(data)
	if err != nil {
		utils.Log("流式解析部分失败", utils.LogErr(err))
	}

	var allEvents []SSEEvent

	// 处理每个消息
	for _, message := range messages {
		events, processErr := cesp.messageProcessor.ProcessMessage(message)
		if processErr != nil {
			utils.Log("流式处理消息失败", utils.LogErr(processErr))
			continue
		}

		allEvents = append(allEvents, events...)
	}

	return allEvents, nil
}

// generateSummary 生成解析摘要
func (cesp *CompliantEventStreamParser) generateSummary(messages []*EventStreamMessage, events []SSEEvent) *ParseSummary {
	summary := &ParseSummary{
		TotalMessages:    len(messages),
		TotalEvents:      len(events),
		MessageTypes:     make(map[string]int),
		EventTypes:       make(map[string]int),
		HasToolCalls:     false,
		HasCompletions:   false,
		HasErrors:        false,
		HasSessionEvents: false,
	}

	// 统计消息类型
	for _, message := range messages {
		msgType := message.GetMessageType()
		summary.MessageTypes[msgType]++

		if msgType == MessageTypes.ERROR || msgType == MessageTypes.EXCEPTION {
			summary.HasErrors = true
		}

		eventType := message.GetEventType()
		if eventType != "" {
			summary.EventTypes[eventType]++

			switch eventType {
			case EventTypes.TOOL_CALL_REQUEST, EventTypes.TOOL_CALL_ERROR:
				summary.HasToolCalls = true
			case EventTypes.COMPLETION, EventTypes.COMPLETION_CHUNK:
				summary.HasCompletions = true
			case EventTypes.SESSION_START, EventTypes.SESSION_END:
				summary.HasSessionEvents = true
			case EventTypes.ASSISTANT_RESPONSE_EVENT:
				// 旧格式的助手响应事件也算作补全内容
				summary.HasCompletions = true
			}
		}
	}

	// 统计事件类型
	for _, event := range events {
		summary.EventTypes[event.Event]++

		eventType := event.Event
		if eventType == "content_block_start" || eventType == "content_block_stop" ||
			eventType == "content_block_delta" {
			// 检查是否是工具相关的内容块
			if data, ok := event.Data.(map[string]any); ok {
				if contentBlock, exists := data["content_block"]; exists {
					if block, ok := contentBlock.(map[string]any); ok {
						if blockType, ok := block["type"].(string); ok && blockType == "tool_use" {
							summary.HasToolCalls = true
						}
					}
				}
			}
		}
	}

	// 工具执行统计
	toolSummary := cesp.messageProcessor.toolManager.GenerateToolSummary()
	summary.ToolSummary = toolSummary

	return summary
}

// GetToolManager 获取工具管理器
func (cesp *CompliantEventStreamParser) GetToolManager() *ToolLifecycleManager {
	return cesp.messageProcessor.GetToolManager()
}

// ParseResult 解析结果
type ParseResult struct {
	Messages       []*EventStreamMessage     `json:"messages"`
	Events         []SSEEvent                `json:"events"`
	ToolExecutions map[string]*ToolExecution `json:"tool_executions"`
	ActiveTools    map[string]*ToolExecution `json:"active_tools"`
	SessionInfo    SessionInfo               `json:"session_info"`
	Summary        *ParseSummary             `json:"summary"`
	Errors         []error                   `json:"errors,omitempty"`
}

// ParseSummary 解析摘要
type ParseSummary struct {
	TotalMessages    int            `json:"total_messages"`
	TotalEvents      int            `json:"total_events"`
	MessageTypes     map[string]int `json:"message_types"`
	EventTypes       map[string]int `json:"event_types"`
	HasToolCalls     bool           `json:"has_tool_calls"`
	HasCompletions   bool           `json:"has_completions"`
	HasErrors        bool           `json:"has_errors"`
	HasSessionEvents bool           `json:"has_session_events"`
	ToolSummary      map[string]any `json:"tool_summary"`
}

// GetCompletionText 获取完整的补全文本
func (pr *ParseResult) GetCompletionText() string {
	var text string

	for _, event := range pr.Events {
		if event.Event == "content_block_delta" {
			if data, ok := event.Data.(map[string]any); ok {
				if delta, ok := data["delta"].(map[string]any); ok {
					if deltaText, ok := delta["text"].(string); ok {
						text += deltaText
					}
				}
			}
		}
	}

	return text
}

// GetToolCalls 获取所有工具调用
func (pr *ParseResult) GetToolCalls() []*ToolExecution {
	var tools []*ToolExecution

	for _, tool := range pr.ToolExecutions {
		tools = append(tools, tool)
	}
	for _, tool := range pr.ActiveTools {
		tools = append(tools, tool)
	}

	return tools
}
