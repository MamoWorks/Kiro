package utils

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kiro/types"

	"github.com/sugarme/tokenizer"
	"github.com/sugarme/tokenizer/pretrained"
)

//go:embed claude_tokenizer.json
var embeddedTokenizer embed.FS

var (
	claudeTokenizer *tokenizer.Tokenizer
	initOnce        sync.Once
	initErr         error
)

// getClaudeTokenizer 获取 Claude tokenizer（单例）
func getClaudeTokenizer() (*tokenizer.Tokenizer, error) {
	initOnce.Do(func() {
		// 从嵌入的文件系统读取 tokenizer.json
		data, err := embeddedTokenizer.ReadFile("claude_tokenizer.json")
		if err != nil {
			initErr = err
			return
		}

		// 写入临时文件（pretrained.FromFile 需要文件路径）
		tmpDir := os.TempDir()
		tmpFile := filepath.Join(tmpDir, "claude_tokenizer.json")
		if err := os.WriteFile(tmpFile, data, 0644); err != nil {
			initErr = err
			return
		}

		claudeTokenizer, initErr = pretrained.FromFile(tmpFile)
	})
	return claudeTokenizer, initErr
}

// TokenEstimator Claude token 计算器
type TokenEstimator struct {
	tokenizer *tokenizer.Tokenizer
}

// NewTokenEstimator 创建 token 估算器实例
func NewTokenEstimator() *TokenEstimator {
	tk, err := getClaudeTokenizer()
	if err != nil {
		panic("failed to initialize Claude tokenizer: " + err.Error())
	}
	return &TokenEstimator{tokenizer: tk}
}

// EstimateTokens 计算消息的 token 数量
func (e *TokenEstimator) EstimateTokens(req *types.CountTokensRequest) int {
	totalTokens := 0

	// 1. 系统提示词
	for _, sysMsg := range req.System {
		if sysMsg.Text != "" {
			totalTokens += e.EstimateTextTokens(sysMsg.Text)
			totalTokens += 2 // 系统提示固定开销
		}
	}

	// 2. 消息内容
	for _, msg := range req.Messages {
		totalTokens += 3 // 角色标记开销

		switch content := msg.Content.(type) {
		case string:
			totalTokens += e.EstimateTextTokens(content)
		case []any:
			for _, block := range content {
				totalTokens += e.estimateContentBlock(block)
			}
		case []types.ContentBlock:
			for _, block := range content {
				totalTokens += e.estimateTypedContentBlock(block)
			}
		default:
			if jsonBytes, err := SafeMarshal(content); err == nil {
				totalTokens += e.countTokens(string(jsonBytes))
			}
		}
	}

	// 3. 工具定义
	if len(req.Tools) > 0 {
		totalTokens += 100 // 工具数组基础开销

		for _, tool := range req.Tools {
			totalTokens += e.EstimateTextTokens(tool.Name)
			totalTokens += e.EstimateTextTokens(tool.Description)

			if tool.InputSchema != nil {
				if jsonBytes, err := SafeMarshal(tool.InputSchema); err == nil {
					totalTokens += e.countTokens(string(jsonBytes))
				}
			}
			totalTokens += 50 // 每个工具的结构开销
		}
	}

	// 4. 基础请求开销
	totalTokens += 4

	return totalTokens
}

// EstimateTextTokens 计算纯文本的 token 数量
func (e *TokenEstimator) EstimateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	return e.countTokens(text)
}

// EstimateToolUseTokens 计算工具调用的 token 数量
func (e *TokenEstimator) EstimateToolUseTokens(toolName string, toolInput map[string]any) int {
	totalTokens := 0

	// JSON 结构字段开销
	totalTokens += 3  // "type": "tool_use"
	totalTokens += 8  // "id": "toolu_xxx..."
	totalTokens += 1  // "name" 关键字
	totalTokens += e.countTokens(toolName)
	totalTokens += 1 // "input" 关键字

	if len(toolInput) > 0 {
		if jsonBytes, err := SafeMarshal(toolInput); err == nil {
			totalTokens += e.countTokens(string(jsonBytes))
		}
	} else {
		totalTokens += 1 // 空对象 {}
	}

	return totalTokens
}

// countTokens 使用 Claude tokenizer 计算 token 数量
func (e *TokenEstimator) countTokens(text string) int {
	en, err := e.tokenizer.EncodeSingle(text, true)
	if err != nil {
		// 降级到字符估算
		return len([]rune(text))
	}
	return len(en.Ids)
}

// estimateContentBlock 计算单个内容块的 token 数量
func (e *TokenEstimator) estimateContentBlock(block any) int {
	blockMap, ok := block.(map[string]any)
	if !ok {
		return 10
	}

	blockType, _ := blockMap["type"].(string)

	switch blockType {
	case "text":
		if text, ok := blockMap["text"].(string); ok {
			return e.EstimateTextTokens(text)
		}
		return 10

	case "image":
		return 1500

	case "document":
		return 500

	case "tool_use":
		toolName, _ := blockMap["name"].(string)
		toolInput, _ := blockMap["input"].(map[string]any)
		return e.EstimateToolUseTokens(toolName, toolInput)

	case "tool_result":
		content := blockMap["content"]
		switch c := content.(type) {
		case string:
			return e.EstimateTextTokens(c)
		case []any:
			total := 0
			for _, item := range c {
				total += e.estimateContentBlock(item)
			}
			return total
		default:
			return 50
		}

	default:
		if jsonBytes, err := SafeMarshal(block); err == nil {
			return e.countTokens(string(jsonBytes))
		}
		return 10
	}
}

// estimateTypedContentBlock 计算类型化内容块的 token 数量
func (e *TokenEstimator) estimateTypedContentBlock(block types.ContentBlock) int {
	switch block.Type {
	case "text":
		if block.Text != nil {
			return e.EstimateTextTokens(*block.Text)
		}
		return 10

	case "image":
		return 1500

	case "tool_use":
		toolName := ""
		if block.Name != nil {
			toolName = *block.Name
		}
		toolInput := make(map[string]any)
		if block.Input != nil {
			if input, ok := (*block.Input).(map[string]any); ok {
				toolInput = input
			}
		}
		return e.EstimateToolUseTokens(toolName, toolInput)

	case "tool_result":
		switch content := block.Content.(type) {
		case string:
			return e.EstimateTextTokens(content)
		case []any:
			total := 0
			for _, item := range content {
				total += e.estimateContentBlock(item)
			}
			return total
		default:
			return 50
		}

	default:
		return 10
	}
}

// IsValidClaudeModel 验证是否为有效的 Claude 模型
func IsValidClaudeModel(model string) bool {
	if model == "" {
		return false
	}

	model = strings.ToLower(model)

	validPrefixes := []string{
		"claude-",
		"anthropic.claude",
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}

	return false
}
