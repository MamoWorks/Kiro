package server

import (
	"crypto/rand"
	"encoding/base64"
	"regexp"
	"strings"
)

// ThinkingExtractor 从文本流中提取 <thinking> 标签内容
// 支持增量流式输出：收到一条转发一条
type ThinkingExtractor struct {
	buffer          strings.Builder // 文本缓冲区（用于处理部分标签）
	inThinkingBlock bool            // 是否在 thinking 块内
	thinkingContent strings.Builder // 累积的 thinking 内容（用于生成签名）
}

// StreamingExtractResult 流式提取结果
type StreamingExtractResult struct {
	// 事件类型
	ThinkingStarted bool   // 是否刚进入 thinking 块
	ThinkingEnded   bool   // 是否刚结束 thinking 块
	ThinkingDelta   string // thinking 增量内容（立即转发）
	TextDelta       string // 普通文本增量（立即转发）
	Signature       string // thinking 块结束时的签名
	HasPending      bool   // 是否有未完成的内容（部分标签）
}

// ExtractedContent 提取结果（兼容旧接口）
type ExtractedContent struct {
	ThinkingBlocks []string // 完整的 thinking 内容列表
	TextContent    string   // 非 thinking 的文本内容
	HasPending     bool     // 是否有未完成的 thinking 块
}

// NewThinkingExtractor 创建新的 thinking 提取器
func NewThinkingExtractor() *ThinkingExtractor {
	return &ThinkingExtractor{}
}

// Reset 重置提取器状态
func (te *ThinkingExtractor) Reset() {
	te.buffer.Reset()
	te.thinkingContent.Reset()
	te.inThinkingBlock = false
}

// ProcessTextStreaming 流式处理文本增量
// 返回可以立即转发的内容
func (te *ThinkingExtractor) ProcessTextStreaming(text string) StreamingExtractResult {
	result := StreamingExtractResult{}

	// 将新文本添加到缓冲区
	te.buffer.WriteString(text)
	content := te.buffer.String()
	te.buffer.Reset()

	for len(content) > 0 {
		if te.inThinkingBlock {
			// 在 thinking 块内，查找结束标签
			endIdx := strings.Index(content, "</thinking>")
			if endIdx >= 0 {
				// 找到结束标签，发送剩余内容并结束
				if endIdx > 0 {
					delta := content[:endIdx]
					result.ThinkingDelta += delta
					te.thinkingContent.WriteString(delta)
				}
				// 生成签名并结束 thinking 块
				result.ThinkingEnded = true
				result.Signature = te.generateSignature()
				te.thinkingContent.Reset()
				te.inThinkingBlock = false
				content = content[endIdx+len("</thinking>"):]
			} else {
				// 检查是否有部分结束标签
				partialEnd := findPartialEndTag(content)
				if partialEnd > 0 {
					// 有部分结束标签，发送前面的内容，保留部分标签
					if len(content) > partialEnd {
						delta := content[:len(content)-partialEnd]
						result.ThinkingDelta += delta
						te.thinkingContent.WriteString(delta)
					}
					te.buffer.WriteString(content[len(content)-partialEnd:])
					result.HasPending = true
				} else {
					// 没有结束标签，全部作为 thinking delta 发送
					result.ThinkingDelta += content
					te.thinkingContent.WriteString(content)
				}
				content = ""
			}
		} else {
			// 不在 thinking 块内，查找开始标签
			startIdx := strings.Index(content, "<thinking>")
			if startIdx >= 0 {
				// 找到开始标签
				if startIdx > 0 {
					result.TextDelta += content[:startIdx]
				}
				result.ThinkingStarted = true
				te.inThinkingBlock = true
				content = content[startIdx+len("<thinking>"):]
			} else {
				// 检查是否有部分开始标签
				partialStart := findPartialStartTag(content)
				if partialStart > 0 {
					// 有部分开始标签，发送前面的内容，保留部分标签
					if len(content) > partialStart {
						result.TextDelta += content[:len(content)-partialStart]
					}
					te.buffer.WriteString(content[len(content)-partialStart:])
					result.HasPending = true
				} else {
					// 没有开始标签，全部作为普通文本发送
					result.TextDelta += content
				}
				content = ""
			}
		}
	}

	return result
}

// Flush 强制刷新所有缓冲内容
func (te *ThinkingExtractor) Flush() ExtractedContent {
	result := ExtractedContent{}

	// 如果有未完成的 thinking 块，将其作为 thinking 内容
	if te.inThinkingBlock && te.thinkingContent.Len() > 0 {
		result.ThinkingBlocks = append(result.ThinkingBlocks, strings.TrimSpace(te.thinkingContent.String()))
		te.thinkingContent.Reset()
	}

	// 缓冲区中的内容作为普通文本
	if te.buffer.Len() > 0 {
		result.TextContent = te.buffer.String()
		te.buffer.Reset()
	}

	te.inThinkingBlock = false
	return result
}

// FlushStreaming 流式刷新
func (te *ThinkingExtractor) FlushStreaming() StreamingExtractResult {
	result := StreamingExtractResult{}

	// 如果在 thinking 块内，结束它
	if te.inThinkingBlock {
		result.ThinkingEnded = true
		result.Signature = te.generateSignature()
		te.inThinkingBlock = false
	}

	// 缓冲区中的内容作为普通文本
	if te.buffer.Len() > 0 {
		result.TextDelta = te.buffer.String()
		te.buffer.Reset()
	}

	te.thinkingContent.Reset()
	return result
}

// IsInThinkingBlock 检查是否在 thinking 块内
func (te *ThinkingExtractor) IsInThinkingBlock() bool {
	return te.inThinkingBlock
}

// generateSignature 生成 thinking 签名并注册到签名表
func (te *ThinkingExtractor) generateSignature() string {
	sig := generateProtobufLikeSignature(te.thinkingContent.Len())
	RegisterSignature(sig)
	return sig
}

// ProcessText 处理文本增量（兼容旧接口）
func (te *ThinkingExtractor) ProcessText(text string) ExtractedContent {
	result := ExtractedContent{}
	streamResult := te.ProcessTextStreaming(text)

	// 如果 thinking 块完成了，添加到结果
	if streamResult.ThinkingEnded {
		// 这里需要累积内容，但新的流式接口不再累积完整块
		// 旧接口不再推荐使用
	}

	result.TextContent = streamResult.TextDelta
	result.HasPending = streamResult.HasPending || te.inThinkingBlock

	return result
}

// findPartialStartTag 查找部分开始标签
func findPartialStartTag(s string) int {
	tag := "<thinking>"
	for i := 1; i < len(tag) && i <= len(s); i++ {
		suffix := s[len(s)-i:]
		if strings.HasPrefix(tag, suffix) {
			return i
		}
	}
	return 0
}

// findPartialEndTag 查找部分结束标签
func findPartialEndTag(s string) int {
	tag := "</thinking>"
	for i := 1; i < len(tag) && i <= len(s); i++ {
		suffix := s[len(s)-i:]
		if strings.HasPrefix(tag, suffix) {
			return i
		}
	}
	return 0
}

// ExtractThinkingFromFinalText 从完整文本中提取 thinking 内容
// 用于非流式响应
func ExtractThinkingFromFinalText(text string) (thinkingBlocks []string, cleanText string) {
	re := regexp.MustCompile(`(?s)<thinking>(.*?)</thinking>`)

	// 提取所有 thinking 块
	matches := re.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) > 1 {
			thinkingBlocks = append(thinkingBlocks, strings.TrimSpace(match[1]))
		}
	}

	// 移除 thinking 标签，保留普通文本
	cleanText = re.ReplaceAllString(text, "")
	cleanText = strings.TrimSpace(cleanText)

	return thinkingBlocks, cleanText
}

// GenerateFakeSignature 生成签名并注册到签名表（公开方法）
func GenerateFakeSignature(contentLen int) string {
	sig := generateProtobufLikeSignature(contentLen)
	RegisterSignature(sig)
	return sig
}

// generateProtobufLikeSignature 生成模拟官方 protobuf 格式的签名
// 官方签名特征：以 "Ev"/"Eu" 开头，含 "CkYI" 子串，Base64 编码，末尾 "=="
func generateProtobufLikeSignature(contentLen int) string {
	// 官方签名长度约 400-700 bytes (编码后)，与 thinking 内容长度正相关
	signatureLen := contentLen * 2 / 3
	if signatureLen < 300 {
		signatureLen = 300
	}
	if signatureLen > 700 {
		signatureLen = 700
	}

	// 构建模拟 protobuf 头：固定前缀模拟真实结构
	// field 1 (tag 0x12), varint length, "CkYI" (嵌套 message header)
	prefix := []byte{0x12} // field 2, wire type 2 (length-delimited) → Base64 "Ev" 前缀

	// 随机负载
	payloadLen := signatureLen - 1
	randomBytes := make([]byte, payloadLen)
	rand.Read(randomBytes)

	// 注入 "CkYI" 特征在前 4-8 字节位置
	// CkYI 的原始字节: 0x0A 0x46 0x08
	if payloadLen > 8 {
		randomBytes[0] = 0x83 // 使 base64 输出接近 "g"/"h" 范围
		randomBytes[1] = 0x0A // "CkYI" proto field
		randomBytes[2] = 0x46
		randomBytes[3] = 0x08
		// 后面跟 varint + 随机数据
		randomBytes[4] = 0x0B // field number hint
		randomBytes[5] = 0x18
		randomBytes[6] = 0x02
	}

	fullBytes := append(prefix, randomBytes...)
	encoded := base64.StdEncoding.EncodeToString(fullBytes)

	// 确保末尾有 "=="
	if !strings.HasSuffix(encoded, "==") {
		encoded += "=="
	}

	return encoded
}
