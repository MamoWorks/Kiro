package parser

import (
	"fmt"
	"kiro/types"
	"kiro/utils"
	"strings"
	"time"
)

// ValueType AWS Event Stream 值类型
type ValueType byte

const (
	ValueType_BOOL_TRUE  ValueType = 0
	ValueType_BOOL_FALSE ValueType = 1
	ValueType_BYTE       ValueType = 2
	ValueType_SHORT      ValueType = 3
	ValueType_INTEGER    ValueType = 4
	ValueType_LONG       ValueType = 5
	ValueType_BYTE_ARRAY ValueType = 6
	ValueType_STRING     ValueType = 7
	ValueType_TIMESTAMP  ValueType = 8
	ValueType_UUID       ValueType = 9
)

// HeaderValue 头部值结构
type HeaderValue struct {
	Type  ValueType
	Value any
}

// EventStreamMessage 符合规范的事件流消息
type EventStreamMessage struct {
	Headers     map[string]HeaderValue
	Payload     []byte
	MessageType string
	EventType   string
	ContentType string
}

// GetMessageType 获取消息类型
func (esm *EventStreamMessage) GetMessageType() string {
	if header, exists := esm.Headers[":message-type"]; exists {
		if msgType, ok := header.Value.(string); ok {
			return msgType
		}
	}
	return "event" // 默认为事件类型
}

// GetEventType 获取事件类型
func (esm *EventStreamMessage) GetEventType() string {
	if header, exists := esm.Headers[":event-type"]; exists {
		if eventType, ok := header.Value.(string); ok {
			return eventType
		}
	}
	return ""
}

// GetContentType 获取内容类型
func (esm *EventStreamMessage) GetContentType() string {
	if header, exists := esm.Headers[":content-type"]; exists {
		if contentType, ok := header.Value.(string); ok {
			return contentType
		}
	}
	return "application/json" // 默认为JSON
}

// MessageTypes 规范定义的消息类型
var MessageTypes = struct {
	EVENT     string
	ERROR     string
	EXCEPTION string
}{
	EVENT:     "event",
	ERROR:     "error",
	EXCEPTION: "exception",
}

// EventTypes 规范定义的事件类型
var EventTypes = struct {
	// 代码补全
	COMPLETION       string
	COMPLETION_CHUNK string

	// 工具调用相关
	TOOL_CALL_REQUEST    string
	TOOL_CALL_RESULT     string
	TOOL_CALL_ERROR      string
	TOOL_EXECUTION_START string
	TOOL_EXECUTION_END   string

	// 会话管理
	SESSION_START string
	SESSION_END   string

	// 兼容旧格式
	ASSISTANT_RESPONSE_EVENT string
	TOOL_USE_EVENT           string
}{
	COMPLETION:       "completion",
	COMPLETION_CHUNK: "completion_chunk",

	TOOL_CALL_REQUEST:    "tool_call_request",
	TOOL_CALL_RESULT:     "tool_call_result",
	TOOL_CALL_ERROR:      "tool_call_error",
	TOOL_EXECUTION_START: "tool_execution_start",
	TOOL_EXECUTION_END:   "tool_execution_end",

	SESSION_START: "session_start",
	SESSION_END:   "session_end",

	ASSISTANT_RESPONSE_EVENT: "assistantResponseEvent",
	TOOL_USE_EVENT:           "toolUseEvent",
}

// ToolExecution 工具执行状态
type ToolExecution struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	StartTime  time.Time           `json:"start_time"`
	EndTime    *time.Time          `json:"end_time,omitempty"`
	Status     ToolExecutionStatus `json:"status"`
	Arguments  map[string]any      `json:"arguments"`
	Result     any                 `json:"result,omitempty"`
	Error      string              `json:"error,omitempty"`
	BlockIndex int                 `json:"block_index"`
}

// ToolExecutionStatus 工具执行状态枚举
type ToolExecutionStatus int

const (
	ToolStatusPending ToolExecutionStatus = iota
	ToolStatusRunning
	ToolStatusCompleted
	ToolStatusError
)

func (s ToolExecutionStatus) String() string {
	switch s {
	case ToolStatusPending:
		return "pending"
	case ToolStatusRunning:
		return "running"
	case ToolStatusCompleted:
		return "completed"
	case ToolStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// ToolCallRequest 工具调用请求
type ToolCallRequest struct {
	ToolCalls []ToolCall `json:"tool_calls"`
}

// ToolCall 单个工具调用
type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction 工具调用函数信息
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON字符串
}

// ToolCallResult 工具调用结果
type ToolCallResult struct {
	ToolCallID    string `json:"tool_call_id"`
	Result        any    `json:"result"`
	ExecutionTime int64  `json:"execution_time,omitempty"` // 毫秒
}

// ToolCallError 工具调用错误
type ToolCallError struct {
	ToolCallID string `json:"tool_call_id"`
	Error      string `json:"error"`
}

// SessionInfo 会话信息
type SessionInfo struct {
	SessionID string     `json:"session_id"`
	StartTime time.Time  `json:"start_time"`
	EndTime   *time.Time `json:"end_time,omitempty"`
}

// ParseError 解析错误类型
type ParseError struct {
	Message string
	Cause   error
}

func (e *ParseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("解析错误: %s, 原因: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("解析错误: %s", e.Message)
}

// NewParseError 创建解析错误
func NewParseError(message string, cause error) *ParseError {
	return &ParseError{
		Message: message,
		Cause:   cause,
	}
}

// SSEEvent Server-Sent Event结构
type SSEEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// toolIndexState 工具索引状态（用于legacy格式处理）
type toolIndexState struct {
	collectingToolText bool
	toolTextBuffer     strings.Builder
	currentToolName    string
	currentToolId      string
	nextBlockIndex     int
}

// fullReset 完全重置状态
func (tis *toolIndexState) fullReset() {
	tis.collectingToolText = false
	tis.toolTextBuffer.Reset()
	tis.currentToolName = ""
	tis.currentToolId = ""
	tis.nextBlockIndex = 1
}

// assistantResponseEvent 助手响应事件（legacy格式，保持向后兼容）
type assistantResponseEvent struct {
	Content   string  `json:"content"`
	Name      string  `json:"name"`
	ToolUseId string  `json:"toolUseId"`
	Input     *string `json:"input,omitempty"`
	Stop      bool    `json:"stop"`
}

// FullAssistantResponseEvent 完整的助手响应事件结构（符合AWS规范）
type FullAssistantResponseEvent struct {
	types.AssistantResponseEvent
}

// NewFullAssistantResponseEventFromLegacy 从legacy格式创建完整事件
func NewFullAssistantResponseEventFromLegacy(legacy assistantResponseEvent) *FullAssistantResponseEvent {
	full := &FullAssistantResponseEvent{}

	// 映射legacy字段到完整结构
	full.Content = legacy.Content
	full.MessageStatus = types.MessageStatusCompleted
	full.ContentType = types.ContentTypeMarkdown

	// 如果是工具相关的事件
	if legacy.ToolUseId != "" && legacy.Name != "" {
		// 可以在这里添加工具相关的逻辑
		// 但为了保持简单性，我们主要映射基本字段
	}

	return full
}

// NewFullAssistantResponseEventFromDict 从字典创建完整事件
func NewFullAssistantResponseEventFromDict(data map[string]any) (*FullAssistantResponseEvent, error) {
	full := &FullAssistantResponseEvent{}

	if err := full.FromDict(data); err != nil {
		return nil, err
	}

	return full, nil
}

// toolUseEvent 工具使用事件（legacy格式）
type toolUseEvent struct {
	Name      string `json:"name"`
	ToolUseId string `json:"toolUseId"`
	Input     any    `json:"input"` // 修复：支持对象和字符串格式
	Stop      bool   `json:"stop"`
}

// parseFullAssistantResponseEvent 解析完整的助手响应事件
func parseFullAssistantResponseEvent(payload []byte) (*FullAssistantResponseEvent, error) {
	var data map[string]any
	if err := utils.FastUnmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("解析JSON失败: %w", err)
	}

	// 检查是否是嵌套在assistantResponseEvent中
	if eventData, ok := data["assistantResponseEvent"].(map[string]any); ok {
		data = eventData
	}

	// 检查是否真的是完整格式（而不是工具调用片段）
	// 工具调用片段通常只有 name, toolUseId, input, stop 等字段
	// 完整格式应该至少有 content, conversationId, messageId 中的一个主要字段
	isToolFragment := false
	hasMainFields := false

	// 检查是否是工具调用片段
	if _, hasToolUseId := data["toolUseId"]; hasToolUseId {
		if _, hasName := data["name"]; hasName {
			isToolFragment = true
		}
	}

	// 检查是否有主要字段
	if _, hasContent := data["content"]; hasContent && data["content"] != "" {
		hasMainFields = true
	}
	if _, hasConvId := data["conversationId"]; hasConvId && data["conversationId"] != "" {
		hasMainFields = true
	}
	if _, hasMsgId := data["messageId"]; hasMsgId && data["messageId"] != "" {
		hasMainFields = true
	}

	// 如果是工具片段且没有主要字段，则不是完整格式
	if isToolFragment && !hasMainFields {
		return nil, fmt.Errorf("不是完整格式的assistantResponseEvent，而是工具调用片段")
	}

	return NewFullAssistantResponseEventFromDict(data)
}
