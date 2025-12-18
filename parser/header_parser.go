package parser

import (
	"encoding/binary"
	"fmt"
	"kiro/utils"
)

// ParsePhase 解析阶段枚举
type ParsePhase int

const (
	PhaseReadNameLength ParsePhase = iota
	PhaseReadName
	PhaseReadValueType
	PhaseReadValueLength
	PhaseReadValue
)

// HeaderParseState 头部解析状态，支持断点续传
type HeaderParseState struct {
	Phase         ParsePhase             // 当前解析阶段
	CurrentHeader int                    // 当前处理的头部索引
	NameLength    int                    // 当前头部名称长度
	ValueType     ValueType              // 当前头部值类型
	ValueLength   int                    // 当前头部值长度
	PartialName   []byte                 // 部分读取的名称数据
	PartialValue  []byte                 // 部分读取的值数据
	PartialLength []byte                 // 部分读取的长度字节（2字节big-endian）
	ParsedHeaders map[string]HeaderValue // 已解析完成的头部
}

// NewHeaderParseState 创建新的解析状态
func NewHeaderParseState() *HeaderParseState {
	return &HeaderParseState{
		Phase:         PhaseReadNameLength,
		ParsedHeaders: make(map[string]HeaderValue),
	}
}

// Reset 重置解析状态
func (hps *HeaderParseState) Reset() {
	hps.Phase = PhaseReadNameLength
	hps.CurrentHeader = 0
	hps.NameLength = 0
	hps.ValueType = 0
	hps.ValueLength = 0
	hps.PartialName = nil
	hps.PartialValue = nil
	hps.PartialLength = nil
	hps.ParsedHeaders = make(map[string]HeaderValue)
}

// IsComplete 检查是否解析完成
func (hps *HeaderParseState) IsComplete() bool {
	return hps.Phase == PhaseReadNameLength && len(hps.ParsedHeaders) > 0
}

// HeaderParser AWS Event Stream 头部解析器，支持断点续传
type HeaderParser struct {
	state *HeaderParseState
}

// NewHeaderParser 创建头部解析器
func NewHeaderParser() *HeaderParser {
	return &HeaderParser{
		state: NewHeaderParseState(),
	}
}

// ParseHeaders 解析头部数据，支持断点续传
func (hp *HeaderParser) ParseHeaders(data []byte) (map[string]HeaderValue, error) {
	if len(data) == 0 {
		return make(map[string]HeaderValue), nil
	}

	return hp.ParseHeadersWithState(data, hp.state)
}

// ParseHeadersWithState 使用指定状态进行断点续传解析
func (hp *HeaderParser) ParseHeadersWithState(data []byte, state *HeaderParseState) (map[string]HeaderValue, error) {
	if len(data) == 0 {
		if len(state.ParsedHeaders) > 0 {
			return state.ParsedHeaders, nil
		}
		return make(map[string]HeaderValue), nil
	}

	offset := 0
	consecutiveDataInsufficientCount := 0 // 追踪连续的数据不足错误
	initialPhase := state.Phase           // 记录初始阶段

	for offset < len(data) {
		var needMoreData bool
		var err error
		currentPhase := state.Phase // 记录当前阶段

		switch state.Phase {
		case PhaseReadNameLength:
			needMoreData, err = hp.processNameLengthPhase(data, &offset, state)

		case PhaseReadName:
			needMoreData, err = hp.processNamePhase(data, &offset, state)

		case PhaseReadValueType:
			needMoreData, err = hp.processValueTypePhase(data, &offset, state)

		case PhaseReadValueLength:
			needMoreData, err = hp.processValueLengthPhase(data, &offset, state)

		case PhaseReadValue:
			needMoreData, err = hp.processValuePhase(data, &offset, state)
		}

		if err != nil {
			return state.ParsedHeaders, err
		}

		if needMoreData {
			consecutiveDataInsufficientCount++

			// 检查是否在同一阶段陷入无限循环 - 降低阈值，更快触发恢复
			if currentPhase == initialPhase && consecutiveDataInsufficientCount > 2 {
				utils.Log("检测到头部解析循环，强制完成解析",
					utils.LogInt("consecutive_count", consecutiveDataInsufficientCount),
					utils.LogInt("current_phase", int(currentPhase)),
					utils.LogInt("parsed_headers", len(state.ParsedHeaders)))
				return hp.ForceCompleteHeaderParsing(state), nil
			}

			// 如果已经有部分解析结果且连续失败次数过多，强制完成 - 降低阈值
			if len(state.ParsedHeaders) > 0 && consecutiveDataInsufficientCount > 3 {
				utils.Log("连续数据不足错误过多，强制完成头部解析",
					utils.LogInt("consecutive_count", consecutiveDataInsufficientCount),
					utils.LogInt("parsed_headers", len(state.ParsedHeaders)))
				return hp.ForceCompleteHeaderParsing(state), nil
			}

			// 新增：如果没有任何解析结果但连续失败，也要强制跳出
			if len(state.ParsedHeaders) == 0 && consecutiveDataInsufficientCount > 5 {
				utils.Log("长期无法解析任何头部，强制使用默认头部",
					utils.LogInt("consecutive_count", consecutiveDataInsufficientCount))
				return hp.ForceCompleteHeaderParsing(state), nil
			}

			utils.Log("头部解析需要更多数据（状态已保存）",
				utils.LogInt("current_phase", int(state.Phase)),
				utils.LogInt("processed_bytes", offset),
				utils.LogInt("parsed_count", len(state.ParsedHeaders)),
				utils.LogInt("consecutive_insufficient", consecutiveDataInsufficientCount))

			// 确保状态完整保存
			return state.ParsedHeaders, NewParseError("数据不足：需要更多数据继续解析", nil)
		}

		consecutiveDataInsufficientCount = 0 // 重置计数器
		initialPhase = state.Phase           // 更新初始阶段
	}

	// 检查是否还有未处理完的状态
	if state.Phase != PhaseReadNameLength {
		utils.Log("数据结束但解析未完成",
			utils.LogInt("current_phase", int(state.Phase)),
			utils.LogInt("processed_bytes", offset),
			utils.LogInt("parsed_headers", len(state.ParsedHeaders)))

		// 如果已经有一些解析结果，尝试强制完成
		if len(state.ParsedHeaders) > 0 {
			utils.Log("数据不完整但已有解析结果，强制完成头部解析")
			return hp.ForceCompleteHeaderParsing(state), nil
		}

		return state.ParsedHeaders, NewParseError("数据不足：需要更多数据继续解析", nil)
	}

	return state.ParsedHeaders, nil
}

// processNameLengthPhase 处理名称长度读取阶段
func (hp *HeaderParser) processNameLengthPhase(data []byte, offset *int, state *HeaderParseState) (needMoreData bool, err error) {
	if *offset >= len(data) {
		return true, nil
	}

	nameLength := int(data[*offset])
	*offset++

	// 验证名称长度合理性
	if nameLength == 0 || nameLength > 255 {
		return false, NewParseError(fmt.Sprintf("名称长度异常: %d", nameLength), nil)
	}

	state.NameLength = nameLength
	state.PartialName = make([]byte, 0, nameLength)
	state.Phase = PhaseReadName

	return false, nil
}

// processNamePhase 处理名称读取阶段
func (hp *HeaderParser) processNamePhase(data []byte, offset *int, state *HeaderParseState) (needMoreData bool, err error) {
	remainingNameBytes := state.NameLength - len(state.PartialName)
	availableBytes := len(data) - *offset

	if availableBytes < remainingNameBytes {
		// 数据不足，累积部分名称数据
		state.PartialName = append(state.PartialName, data[*offset:]...)
		*offset = len(data)
		utils.Log("部分名称数据累积",
			utils.LogInt("partial_len", len(state.PartialName)),
			utils.LogInt("need_len", state.NameLength))
		return true, nil
	}

	// 读取剩余名称数据
	state.PartialName = append(state.PartialName, data[*offset:*offset+remainingNameBytes]...)
	*offset += remainingNameBytes
	state.Phase = PhaseReadValueType

	return false, nil
}

// processValueTypePhase 处理值类型读取阶段
func (hp *HeaderParser) processValueTypePhase(data []byte, offset *int, state *HeaderParseState) (needMoreData bool, err error) {
	if *offset >= len(data) {
		return true, nil
	}

	state.ValueType = ValueType(data[*offset])
	*offset++
	state.Phase = PhaseReadValueLength

	return false, nil
}

// processValueLengthPhase 处理值长度读取阶段，支持部分长度数据累积
func (hp *HeaderParser) processValueLengthPhase(data []byte, offset *int, state *HeaderParseState) (needMoreData bool, err error) {
	// 初始化部分长度缓冲区
	if state.PartialLength == nil {
		state.PartialLength = make([]byte, 0, 2)
	}

	// 计算还需要多少字节
	remainingBytes := 2 - len(state.PartialLength)
	availableBytes := len(data) - *offset

	if availableBytes < remainingBytes {
		// 数据不足，累积可用的字节
		state.PartialLength = append(state.PartialLength, data[*offset:]...)
		*offset = len(data)
		utils.Log("部分值长度数据累积",
			utils.LogInt("partial_len", len(state.PartialLength)),
			utils.LogInt("need_len", 2),
			utils.LogString("name", string(state.PartialName)))
		return true, nil
	}

	// 读取剩余的长度字节
	state.PartialLength = append(state.PartialLength, data[*offset:*offset+remainingBytes]...)
	*offset += remainingBytes

	// 现在有完整的2字节长度数据，解析它
	valueLength := int(binary.BigEndian.Uint16(state.PartialLength))

	// 验证值长度合理性
	if valueLength < 0 || valueLength > 65535 {
		return false, NewParseError(fmt.Sprintf("值长度异常: %d", valueLength), nil)
	}

	state.ValueLength = valueLength
	state.PartialValue = make([]byte, 0, valueLength)
	state.PartialLength = nil // 清除长度缓冲区
	state.Phase = PhaseReadValue

	return false, nil
}

// processValuePhase 处理值数据读取阶段
func (hp *HeaderParser) processValuePhase(data []byte, offset *int, state *HeaderParseState) (needMoreData bool, err error) {
	remainingValueBytes := state.ValueLength - len(state.PartialValue)
	availableBytes := len(data) - *offset

	if availableBytes < remainingValueBytes {
		// 数据不足，累积部分值数据
		state.PartialValue = append(state.PartialValue, data[*offset:]...)
		*offset = len(data)
		utils.Log("部分值数据累积",
			utils.LogInt("partial_len", len(state.PartialValue)),
			utils.LogInt("need_len", state.ValueLength),
			utils.LogString("name", string(state.PartialName)))
		return true, nil
	}

	// 读取剩余值数据
	state.PartialValue = append(state.PartialValue, data[*offset:*offset+remainingValueBytes]...)
	*offset += remainingValueBytes

	// 解析完整的头部值
	value, err := hp.parseHeaderValue(state.ValueType, state.PartialValue)
	if err != nil {
		utils.Log("解析头部值失败",
			utils.LogString("header_name", string(state.PartialName)),
			utils.LogInt("value_type", int(state.ValueType)),
			utils.LogErr(err))
		// 跳过当前头部，继续处理下一个
	} else {
		// 保存解析成功的头部
		headerName := string(state.PartialName)
		state.ParsedHeaders[headerName] = HeaderValue{
			Type:  state.ValueType,
			Value: value,
		}
	}

	// 重置状态，准备解析下一个头部
	state.CurrentHeader++
	state.NameLength = 0
	state.ValueType = 0
	state.ValueLength = 0
	state.PartialName = nil
	state.PartialValue = nil
	state.PartialLength = nil
	state.Phase = PhaseReadNameLength

	return false, nil
}

// Reset 重置解析器状态
func (hp *HeaderParser) Reset() {
	if hp.state != nil {
		hp.state.Reset()
	} else {
		hp.state = NewHeaderParseState()
	}
}

// GetState 获取当前解析状态
func (hp *HeaderParser) GetState() *HeaderParseState {
	return hp.state
}

// parseHeaderValue 根据类型解析头部值
func (hp *HeaderParser) parseHeaderValue(valueType ValueType, data []byte) (any, error) {
	switch valueType {
	case ValueType_BOOL_TRUE:
		return true, nil

	case ValueType_BOOL_FALSE:
		return false, nil

	case ValueType_BYTE:
		if len(data) != 1 {
			return nil, fmt.Errorf("BYTE类型长度错误: 期望1字节，实际%d字节", len(data))
		}
		return int8(data[0]), nil

	case ValueType_SHORT:
		if len(data) != 2 {
			return nil, fmt.Errorf("SHORT类型长度错误: 期望2字节，实际%d字节", len(data))
		}
		return int16(binary.BigEndian.Uint16(data)), nil

	case ValueType_INTEGER:
		if len(data) != 4 {
			return nil, fmt.Errorf("INTEGER类型长度错误: 期望4字节，实际%d字节", len(data))
		}
		return int32(binary.BigEndian.Uint32(data)), nil

	case ValueType_LONG:
		if len(data) != 8 {
			return nil, fmt.Errorf("LONG类型长度错误: 期望8字节，实际%d字节", len(data))
		}
		return int64(binary.BigEndian.Uint64(data)), nil

	case ValueType_BYTE_ARRAY:
		// 返回字节数组的副本，避免共享底层数组
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil

	case ValueType_STRING:
		return string(data), nil

	case ValueType_TIMESTAMP:
		if len(data) != 8 {
			return nil, fmt.Errorf("TIMESTAMP类型长度错误: 期望8字节，实际%d字节", len(data))
		}
		timestamp := int64(binary.BigEndian.Uint64(data))
		return timestamp, nil

	case ValueType_UUID:
		if len(data) == 16 {
			// 标准UUID格式 (16字节)
			return fmt.Sprintf("%x-%x-%x-%x-%x",
				data[0:4], data[4:6], data[6:8], data[8:10], data[10:16]), nil
		} else if len(data) > 0 {
			// 字符串格式的UUID
			return string(data), nil
		} else {
			return "", nil
		}

	default:
		utils.Log("未知的值类型", utils.LogInt("value_type", int(valueType)))
		// 对于未知类型，返回原始字节数据
		result := make([]byte, len(data))
		copy(result, data)
		return result, nil
	}
}

// GetMessageTypeFromHeaders 从头部提取消息类型
func GetMessageTypeFromHeaders(headers map[string]HeaderValue) string {
	if header, exists := headers[":message-type"]; exists {
		if msgType, ok := header.Value.(string); ok {
			return msgType
		}
	}
	return MessageTypes.EVENT // 默认为事件类型
}

// GetEventTypeFromHeaders 从头部提取事件类型
func GetEventTypeFromHeaders(headers map[string]HeaderValue) string {
	if header, exists := headers[":event-type"]; exists {
		if eventType, ok := header.Value.(string); ok {
			return eventType
		}
	}
	return ""
}

// GetContentTypeFromHeaders 从头部提取内容类型
func GetContentTypeFromHeaders(headers map[string]HeaderValue) string {
	if header, exists := headers[":content-type"]; exists {
		if contentType, ok := header.Value.(string); ok {
			return contentType
		}
	}
	return "application/json" // 默认为JSON
}

// IsHeaderParseRecoverable 检查头部解析错误是否可恢复
func (hp *HeaderParser) IsHeaderParseRecoverable(state *HeaderParseState) bool {
	// 如果在读取名称长度阶段且已有解析成功的头部，则可以终止解析
	if state.Phase == PhaseReadNameLength && len(state.ParsedHeaders) >= 1 {
		return true
	}
	// 如果在名称或值读取阶段但有基本头部信息，也可以终止
	return len(state.ParsedHeaders) > 0
}

// ForceCompleteHeaderParsing 强制完成头部解析（容错处理）
func (hp *HeaderParser) ForceCompleteHeaderParsing(state *HeaderParseState) map[string]HeaderValue {
	if len(state.ParsedHeaders) == 0 {
		// 没有任何头部信息，返回默认头部
		return map[string]HeaderValue{
			":message-type": {Type: ValueType_STRING, Value: MessageTypes.EVENT},
			":event-type":   {Type: ValueType_STRING, Value: EventTypes.ASSISTANT_RESPONSE_EVENT},
			":content-type": {Type: ValueType_STRING, Value: "application/json"},
		}
	}

	// 补充缺失的关键头部
	result := make(map[string]HeaderValue)
	for k, v := range state.ParsedHeaders {
		result[k] = v
	}

	if _, exists := result[":message-type"]; !exists {
		result[":message-type"] = HeaderValue{Type: ValueType_STRING, Value: MessageTypes.EVENT}
	}
	if _, exists := result[":event-type"]; !exists {
		result[":event-type"] = HeaderValue{Type: ValueType_STRING, Value: EventTypes.ASSISTANT_RESPONSE_EVENT}
	}
	if _, exists := result[":content-type"]; !exists {
		result[":content-type"] = HeaderValue{Type: ValueType_STRING, Value: "application/json"}
	}

	return result
}
