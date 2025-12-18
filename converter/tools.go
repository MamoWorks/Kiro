package converter

import (
	"fmt"

	"kiro/types"
	"kiro/utils"
)

// 工具处理器

// cleanAndValidateToolParameters 清理和验证工具参数
func cleanAndValidateToolParameters(params map[string]any) (map[string]any, error) {
	if params == nil {
		return nil, fmt.Errorf("参数不能为nil")
	}

	// 深拷贝避免修改原始数据
	cleanedParams, _ := utils.SafeMarshal(params)
	var tempParams map[string]any
	if err := utils.SafeUnmarshal(cleanedParams, &tempParams); err != nil {
		return nil, fmt.Errorf("参数序列化失败: %v", err)
	}

	// 移除不支持的顶级字段
	delete(tempParams, "additionalProperties")
	delete(tempParams, "strict")
	delete(tempParams, "$schema")
	delete(tempParams, "$id")
	delete(tempParams, "$ref")
	delete(tempParams, "definitions")
	delete(tempParams, "$defs")

	// 处理超长参数名 - CodeWhisperer限制参数名长度；保留原名映射
	if properties, ok := tempParams["properties"].(map[string]any); ok {
		cleanedProperties := make(map[string]any)
		for paramName, paramDef := range properties {
			cleanedName := paramName
			// 如果参数名超过64字符，进行简化
			if len(paramName) > 64 {
				// 保留前缀和后缀，中间用下划线连接
				if len(paramName) > 80 {
					cleanedName = paramName[:20] + "_" + paramName[len(paramName)-20:]
				} else {
					cleanedName = paramName[:30] + "_param"
				}
			}
			cleanedProperties[cleanedName] = paramDef
		}
		tempParams["properties"] = cleanedProperties

		// 同时更新required字段中的参数名
		if required, ok := tempParams["required"].([]any); ok {
			var cleanedRequired []any
			for _, req := range required {
				if reqStr, ok := req.(string); ok {
					if len(reqStr) > 64 {
						if len(reqStr) > 80 {
							cleanedRequired = append(cleanedRequired, reqStr[:20]+"_"+reqStr[len(reqStr)-20:])
						} else {
							cleanedRequired = append(cleanedRequired, reqStr[:30]+"_param")
						}
					} else {
						cleanedRequired = append(cleanedRequired, reqStr)
					}
				}
			}
			tempParams["required"] = cleanedRequired
		}
	}

	// 确保 schema 明确声明顶级 type=object，符合 CodeWhisperer 工具schema约定
	if _, exists := tempParams["type"]; !exists {
		tempParams["type"] = "object"
	}

	// 验证必需的字段
	if schemaType, exists := tempParams["type"]; exists {
		if typeStr, ok := schemaType.(string); ok && typeStr == "object" {
			// 对象类型应该有properties字段
			if _, hasProps := tempParams["properties"]; !hasProps {
				return nil, fmt.Errorf("对象类型缺少properties字段")
			}
		}
	}

	// CodeWhisperer 对 schema 的兼容性处理：
	// - 仅允许标准 JSON Schema 字段：type, properties, required, description
	// - 去除潜在不兼容的字段（上面已经逐步移除）
	// - 保证 required 是字符串数组，properties 为对象
	if req, ok := tempParams["required"]; ok && req != nil {
		if arr, ok := req.([]any); ok {
			cleaned := make([]string, 0, len(arr))
			for _, v := range arr {
				if s, ok := v.(string); ok && s != "" {
					cleaned = append(cleaned, s)
				}
			}
			tempParams["required"] = cleaned
		} else {
			delete(tempParams, "required")
		}
	}
	if props, ok := tempParams["properties"]; ok {
		if _, ok := props.(map[string]any); !ok {
			delete(tempParams, "properties")
			tempParams["properties"] = map[string]any{}
		}
	} else {
		tempParams["properties"] = map[string]any{}
	}

	return tempParams, nil
}

// convertAnthropicToolChoiceToAnthropic 处理 Anthropic 格式的 tool_choice
// 支持的格式：
// - string: "auto", "any", "none"
// - map[string]any: {"type": "tool", "name": "tool_name"}
// - *types.ToolChoice: 结构化类型
func convertAnthropicToolChoiceToAnthropic(toolChoice any) any {
	if toolChoice == nil {
		return nil
	}

	switch choice := toolChoice.(type) {
	case string:
		// 处理字符串类型："auto", "any", "none"
		switch choice {
		case "auto":
			return &types.ToolChoice{Type: "auto"}
		case "any":
			return &types.ToolChoice{Type: "any"}
		case "none":
			// 返回nil表示不强制使用工具
			return nil
		default:
			// 未知字符串，默认为auto
			return &types.ToolChoice{Type: "auto"}
		}

	case map[string]any:
		// 处理对象类型：{"type": "tool", "name": "tool_name"}
		if choiceType, ok := choice["type"].(string); ok {
			if choiceType == "tool" {
				if name, ok := choice["name"].(string); ok {
					return &types.ToolChoice{
						Type: "tool",
						Name: name,
					}
				}
			} else if choiceType == "auto" || choiceType == "any" {
				return &types.ToolChoice{Type: choiceType}
			}
		}
		// 如果无法解析，返回auto
		return &types.ToolChoice{Type: "auto"}

	case *types.ToolChoice:
		// 已经是正确类型，直接返回
		return choice

	case types.ToolChoice:
		// 值类型，转为指针返回
		return &choice

	default:
		// 未知类型，默认为auto
		return &types.ToolChoice{Type: "auto"}
	}
}
