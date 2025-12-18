package types

import (
	"bytes"
	"encoding/json"
)

// ==================== SSE 事件结构（保证 type 字段在最前） ====================

// MessageStartEvent message_start 事件
type MessageStartEvent struct {
	Type    string       `json:"type"`
	Message *MessageInfo `json:"message"`
}

// MessageInfo 消息信息
type MessageInfo struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Role         string    `json:"role"`
	Content      []any     `json:"content"`
	Model        string    `json:"model"`
	StopReason   *string   `json:"stop_reason"`
	StopSequence *string   `json:"stop_sequence"`
	Usage        *UsageInfo `json:"usage"`
}

// UsageInfo 使用量信息（与官方 Claude API 一致）
type UsageInfo struct {
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
}

// ContentBlockStartEvent content_block_start 事件
// 字段顺序: type, index, content_block (与官方 Claude API 一致)
type ContentBlockStartEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock any    `json:"content_block"`
}

// SSEContentBlock SSE 事件专用内容块（与 anthropic.ContentBlock 区分）
// 文本块必须包含 text 字段（即使为空字符串）
type SSEContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

// SSEToolUseContentBlock 工具使用内容块（input 字段始终显示）
type SSEToolUseContentBlock struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input any    `json:"input"`
}

// SSETextContentBlock 文本内容块（text 字段始终显示）
type SSETextContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ContentBlockDeltaEvent content_block_delta 事件
// 字段顺序: type, index, delta (与官方 Claude API 一致)
type ContentBlockDeltaEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta any    `json:"delta"`
}

// DeltaBlock delta 块
type DeltaBlock struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// TextDeltaBlock 文本增量块（text 字段始终显示）
type TextDeltaBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// InputJSONDeltaBlock JSON 增量块（partial_json 字段始终显示）
type InputJSONDeltaBlock struct {
	Type        string `json:"type"`
	PartialJSON string `json:"partial_json"`
}

// ContentBlockStopEvent content_block_stop 事件
type ContentBlockStopEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
}

// MessageDeltaEvent message_delta 事件
type MessageDeltaEvent struct {
	Type  string            `json:"type"`
	Delta *MessageDeltaInfo `json:"delta"`
	Usage *UsageInfo        `json:"usage,omitempty"`
}

// MessageDeltaInfo message delta 信息
// stop_sequence 字段始终显示（即使为 null）
type MessageDeltaInfo struct {
	StopReason   string  `json:"stop_reason"`
	StopSequence *string `json:"stop_sequence"`
}

// MessageStopEvent message_stop 事件
type MessageStopEvent struct {
	Type string `json:"type"`
}

// PingEvent ping 事件
type PingEvent struct {
	Type string `json:"type"`
}

// GenericOrderedEvent 通用有序事件（确保 type 在最前面）
type GenericOrderedEvent struct {
	Type string         `json:"type"`
	Data map[string]any `json:"-"`
}

// MarshalJSON 自定义 JSON 序列化，确保 type 在最前面
func (e *GenericOrderedEvent) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(`{"type":`)

	typeJSON, err := json.Marshal(e.Type)
	if err != nil {
		return nil, err
	}
	buf.Write(typeJSON)

	// 添加其他字段
	for k, v := range e.Data {
		if k == "type" {
			continue
		}
		buf.WriteString(",")
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteString(":")
		valJSON, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		buf.Write(valJSON)
	}

	buf.WriteString("}")
	return buf.Bytes(), nil
}

// NewGenericOrderedEvent 创建通用有序事件
func NewGenericOrderedEvent(eventType string, data map[string]any) *GenericOrderedEvent {
	return &GenericOrderedEvent{
		Type: eventType,
		Data: data,
	}
}

// ErrorEvent error 事件
type ErrorEvent struct {
	Type  string     `json:"type"`
	Error *ErrorInfo `json:"error"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ==================== 构造函数 ====================

// NewMessageStartEvent 创建 message_start 事件
func NewMessageStartEvent(msg *MessageInfo) *MessageStartEvent {
	return &MessageStartEvent{
		Type:    "message_start",
		Message: msg,
	}
}

// NewContentBlockStartEvent 创建 content_block_start 事件
func NewContentBlockStartEvent(index int, block any) *ContentBlockStartEvent {
	return &ContentBlockStartEvent{
		Type:         "content_block_start",
		Index:        index,
		ContentBlock: block,
	}
}

// NewTextContentBlock 创建文本内容块
func NewTextContentBlock(text string) *SSEContentBlock {
	return &SSEContentBlock{
		Type: "text",
		Text: text,
	}
}

// NewToolUseContentBlock 创建工具使用内容块
func NewToolUseContentBlock(id, name string, input any) *SSEContentBlock {
	if input == nil {
		input = map[string]any{}
	}
	return &SSEContentBlock{
		Type:  "tool_use",
		ID:    id,
		Name:  name,
		Input: input,
	}
}

// NewContentBlockDeltaEvent 创建 content_block_delta 事件
func NewContentBlockDeltaEvent(index int, delta any) *ContentBlockDeltaEvent {
	return &ContentBlockDeltaEvent{
		Type:  "content_block_delta",
		Index: index,
		Delta: delta,
	}
}

// NewTextDelta 创建文本 delta
func NewTextDelta(text string) *DeltaBlock {
	return &DeltaBlock{
		Type: "text_delta",
		Text: text,
	}
}

// NewInputJSONDelta 创建 JSON delta
func NewInputJSONDelta(partialJSON string) *DeltaBlock {
	return &DeltaBlock{
		Type:        "input_json_delta",
		PartialJSON: partialJSON,
	}
}

// NewContentBlockStopEvent 创建 content_block_stop 事件
func NewContentBlockStopEvent(index int) *ContentBlockStopEvent {
	return &ContentBlockStopEvent{
		Type:  "content_block_stop",
		Index: index,
	}
}

// NewMessageDeltaEvent 创建 message_delta 事件
func NewMessageDeltaEvent(stopReason string, usage *UsageInfo) *MessageDeltaEvent {
	return &MessageDeltaEvent{
		Type: "message_delta",
		Delta: &MessageDeltaInfo{
			StopReason: stopReason,
		},
		Usage: usage,
	}
}

// NewMessageStopEvent 创建 message_stop 事件
func NewMessageStopEvent() *MessageStopEvent {
	return &MessageStopEvent{
		Type: "message_stop",
	}
}

// NewPingEvent 创建 ping 事件
func NewPingEvent() *PingEvent {
	return &PingEvent{
		Type: "ping",
	}
}

// NewErrorEvent 创建 error 事件
func NewErrorEvent(errType, message string) *ErrorEvent {
	return &ErrorEvent{
		Type: "error",
		Error: &ErrorInfo{
			Type:    errType,
			Message: message,
		},
	}
}
