package utils

import (
	"encoding/json"
	"fmt"
	"strings"

	"kiro/types"
)

// ParseToolResultContent 解析tool_result的content字段
// 参考Python实现：parse_tool_result_content函数和Anthropic官方文档
func ParseToolResultContent(content any) string {
	if content == nil {
		return "No content provided"
	}

	switch v := content.(type) {
	case string:
		if v == "" {
			return "Tool executed with no output"
		}
		return v

	case []any:
		if len(v) == 0 {
			return "Tool executed with empty result list"
		}

		// 直接使用strings.Builder，Go编译器会优化栈分配
		var result strings.Builder
		for _, item := range v {
			switch itemVal := item.(type) {
			case map[string]any:
				// 处理结构化内容块，如 {"type": "text", "text": "..."}
				if itemType, ok := itemVal["type"].(string); ok && itemType == "text" {
					if text, ok := itemVal["text"].(string); ok && text != "" {
						result.WriteString(text + "\n")
					}
				} else if text, ok := itemVal["text"].(string); ok && text != "" {
					// 处理包含text字段但没有type的对象
					result.WriteString(text + "\n")
				} else {
					// 其他结构化数据序列化为JSON
					if data, err := json.Marshal(itemVal); err == nil {
						result.WriteString(string(data) + "\n")
					} else {
						result.WriteString(fmt.Sprintf("%v\n", itemVal))
					}
				}
			case string:
				if itemVal != "" {
					result.WriteString(itemVal + "\n")
				}
			default:
				// 处理其他类型（数字、布尔值等）
				result.WriteString(fmt.Sprintf("%v\n", itemVal))
			}
		}

		resultStr := strings.TrimSpace(result.String())
		if resultStr == "" {
			return "Tool executed with empty content"
		}
		return resultStr

	case map[string]any:
		// 处理单个结构化对象
		if contentType, ok := v["type"].(string); ok && contentType == "text" {
			if text, ok := v["text"].(string); ok {
				if text == "" {
					return "Tool executed with empty text"
				}
				return text
			}
		}

		// 检查是否有直接的text字段
		if text, ok := v["text"].(string); ok {
			if text == "" {
				return "Tool executed with empty text field"
			}
			return text
		}

		// 序列化整个对象
		if data, err := json.Marshal(v); err == nil {
			return string(data)
		}
		return fmt.Sprintf("%v", v)

	default:
		// 处理其他基本类型（数字、布尔值等）
		return fmt.Sprintf("%v", content)
	}
}

// GetMessageContent 从消息中提取文本内容的辅助函数，支持图片内容
func GetMessageContent(content any) (string, error) {
	switch v := content.(type) {
	case types.AnthropicSystemMessage:
		return v.Text, nil
	case string:
		if len(v) == 0 {
			return "answer for user question", nil
		}
		return v, nil
	case []any:
		var texts []string
		hasImage := false
		for _, block := range v {
			if m, ok := block.(map[string]any); ok {
				var cb types.ContentBlock
				if data, err := json.Marshal(m); err == nil {
					if err := json.Unmarshal(data, &cb); err == nil {
						switch cb.Type {
						case "tool_result":
							if cb.Content != nil {
								toolResultContent := ParseToolResultContent(cb.Content)

								// 检查是否为错误结果
								if cb.IsError != nil && *cb.IsError {
									toolResultContent = "Tool Error: " + toolResultContent
								}

								// 添加tool_use_id信息以便追踪
								if cb.ToolUseId != nil && *cb.ToolUseId != "" {
									toolResultContent = fmt.Sprintf("Tool result for %s: %s", *cb.ToolUseId, toolResultContent)
								}

								texts = append(texts, toolResultContent)
							}
						case "text":
							if cb.Text != nil {
								texts = append(texts, *cb.Text)
							}
						case "image":
							hasImage = true
							if cb.Source != nil {
								texts = append(texts, fmt.Sprintf("[图片: %s格式]", cb.Source.MediaType))
							} else {
								texts = append(texts, "[图片]")
							}
						}
					}
				}
			}
		}
		if len(texts) == 0 && hasImage {
			return "请描述这张图片的内容", nil
		}
		if len(texts) == 0 {
			return "answer for user question", nil
		}
		return strings.Join(texts, "\n"), nil
	case []types.ContentBlock:
		var texts []string
		hasImage := false
		for _, cb := range v {
			switch cb.Type {
			case "tool_result":
				if cb.Content != nil {
					toolResultContent := ParseToolResultContent(cb.Content)

					// 检查是否为错误结果
					if cb.IsError != nil && *cb.IsError {
						toolResultContent = "Tool Error: " + toolResultContent
					}

					// 添加tool_use_id信息以便追踪
					if cb.ToolUseId != nil && *cb.ToolUseId != "" {
						toolResultContent = fmt.Sprintf("Tool result for %s: %s", *cb.ToolUseId, toolResultContent)
					}

					texts = append(texts, toolResultContent)
				}
			case "text":
				if cb.Text != nil {
					texts = append(texts, *cb.Text)
				}
			case "image":
				hasImage = true
				if cb.Source != nil {
					texts = append(texts, fmt.Sprintf("[图片: %s格式]", cb.Source.MediaType))
				} else {
					texts = append(texts, "[图片]")
				}
			}
		}
		if len(texts) == 0 && hasImage {
			return "请描述这张图片的内容", nil
		}
		if len(texts) == 0 {
			return "answer for user question", nil
		}
		return strings.Join(texts, "\n"), nil
	default:
		return "", fmt.Errorf("unsupported content type: %T", v)
	}
}
