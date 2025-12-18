package parser

import (
	"bytes"

	"kiro/utils"
	"strings"
	"sync"
	"time"
)

// SonicStreamingJSONAggregator 基于Sonic的高性能流式JSON聚合器
// ToolParamsUpdateCallback 工具参数更新回调函数类型
type ToolParamsUpdateCallback func(toolUseId string, fullParams string)

// AWS EventStream流式传输配置
// 由于EventStream按字节边界分片传输，导致UTF-8字符截断，
// 因此只在收到停止信号时进行JSON解析，避免解析损坏的片段

type SonicStreamingJSONAggregator struct {
	activeStreamers map[string]*SonicJSONStreamer
	mu              sync.RWMutex
	updateCallback  ToolParamsUpdateCallback
}

// SonicJSONStreamer 单个工具调用的Sonic流式解析器
type SonicJSONStreamer struct {
	toolUseId      string
	toolName       string
	buffer         *bytes.Buffer
	state          SonicParseState
	lastUpdate     time.Time
	isComplete     bool
	result         map[string]any
	fragmentCount  int
	totalBytes     int
	incompleteUTF8 string // 用于存储跨片段的不完整UTF-8字符
}

// SonicParseState Sonic JSON解析状态
type SonicParseState struct {
	hasValidJSON bool
}

// NewSonicStreamingJSONAggregatorWithCallback 创建带回调的Sonic流式JSON聚合器
func NewSonicStreamingJSONAggregatorWithCallback(callback ToolParamsUpdateCallback) *SonicStreamingJSONAggregator {
	// utils.Log("创建Sonic流式JSON聚合器")

	return &SonicStreamingJSONAggregator{
		activeStreamers: make(map[string]*SonicJSONStreamer),
		updateCallback:  callback,
	}
}

// ProcessToolData 处理工具调用数据片段（Sonic版本）
func (ssja *SonicStreamingJSONAggregator) ProcessToolData(toolUseId, name, input string, stop bool, fragmentIndex int) (complete bool, fullInput string) {
	ssja.mu.Lock()
	defer ssja.mu.Unlock()

	// 获取或创建Sonic流式解析器
	streamer, exists := ssja.activeStreamers[toolUseId]
	if !exists {
		streamer = ssja.createSonicJSONStreamer(toolUseId, name)
		ssja.activeStreamers[toolUseId] = streamer
	}

	// 处理输入片段
	if input != "" {
		if err := streamer.appendFragment(input); err != nil {
			utils.Error("追加JSON片段到Sonic解析器失败: toolUseId=%s, err=%v", toolUseId, err)
		}
	}

	// AWS EventStream按字节边界分片传输，导致UTF-8中文字符截断问题
	// 只有在收到停止信号时才进行最终解析，避免中途解析损坏的JSON片段
	if !stop {
		return false, ""
	}

	// 收到停止信号，使用Sonic尝试解析当前缓冲区
	parseResult := streamer.tryParseWithSonic()

	streamer.isComplete = true

	if streamer.state.hasValidJSON && streamer.result != nil {
		// 使用Sonic序列化结果
		if jsonBytes, err := utils.FastMarshal(streamer.result); err == nil {
			fullInput = string(jsonBytes)
		} else {
			utils.Error("Sonic序列化失败: toolName=%s, err=%v", streamer.toolName, err)
			// 使用空JSON对象，让工具调用失败
			fullInput = "{}"
		}
	} else {
		// 区分真正的错误和无参数工具
		if streamer.fragmentCount == 0 && streamer.totalBytes == 0 {
			// 无参数工具，正常情况
		} else {
			// 真正的解析失败
			utils.Error("流式解析失败: toolName=%s, status=%s", streamer.toolName, parseResult)
		}
		// 使用空JSON对象
		fullInput = "{}"
	}

	// 清理完成的流式解析器，归还对象到池中
	ssja.cleanupStreamer(streamer)
	delete(ssja.activeStreamers, toolUseId)

	// 触发回调
	ssja.onAggregationComplete(toolUseId, fullInput)

	return true, fullInput
}

// createSonicJSONStreamer 创建Sonic JSON流式解析器（使用对象池优化）
func (ssja *SonicStreamingJSONAggregator) createSonicJSONStreamer(toolUseId, toolName string) *SonicJSONStreamer {
	// 直接分配Buffer，Go GC会自动管理
	buffer := bytes.NewBuffer(nil)

	return &SonicJSONStreamer{
		toolUseId:  toolUseId,
		toolName:   toolName,
		buffer:     buffer,
		lastUpdate: time.Now(),
		result:     make(map[string]any),
	}
}

// appendFragment 追加JSON片段
func (sjs *SonicJSONStreamer) appendFragment(fragment string) error {
	// 确保UTF-8字符完整性
	safeFragment := sjs.ensureUTF8Integrity(fragment)

	sjs.buffer.WriteString(safeFragment)
	sjs.lastUpdate = time.Now()
	sjs.fragmentCount++
	sjs.totalBytes += len(fragment) // 使用原始长度统计

	return nil
}

// ensureUTF8Integrity 确保UTF-8字符完整性
func (sjs *SonicJSONStreamer) ensureUTF8Integrity(fragment string) string {
	if fragment == "" {
		return fragment
	}

	// 检查片段是否以不完整的UTF-8字符结尾
	bytes := []byte(fragment)
	n := len(bytes)
	if n == 0 {
		return fragment
	}

	// 从末尾开始检查UTF-8字符边界
	for i := n - 1; i >= 0 && i >= n-4; i-- {
		b := bytes[i]

		// 检查是否为UTF-8多字节序列的开始
		if b&0x80 == 0 {
			// ASCII字符，边界正确
			break
		} else if b&0xE0 == 0xC0 {
			// 2字节UTF-8序列开始
			if n-i < 2 {
				// 保存截断的字符到下一个片段处理
				sjs.incompleteUTF8 = string(bytes[i:])
				return string(bytes[:i])
			}
			break
		} else if b&0xF0 == 0xE0 {
			// 3字节UTF-8序列开始
			if n-i < 3 {
				sjs.incompleteUTF8 = string(bytes[i:])
				return string(bytes[:i])
			}
			break
		} else if b&0xF8 == 0xF0 {
			// 4字节UTF-8序列开始
			if n-i < 4 {
				sjs.incompleteUTF8 = string(bytes[i:])
				return string(bytes[:i])
			}
			break
		}
		// 继续字符(10xxxxxx)，继续向前检查
	}

	// 检查是否有之前的不完整UTF-8字符需要拼接
	if sjs.incompleteUTF8 != "" {
		combined := sjs.incompleteUTF8 + fragment
		sjs.incompleteUTF8 = ""                  // 清空
		return sjs.ensureUTF8Integrity(combined) // 递归处理合并结果
	}

	return fragment
}

// tryParseWithSonic 使用Sonic尝试解析当前缓冲区
func (sjs *SonicJSONStreamer) tryParseWithSonic() string {
	content := sjs.buffer.Bytes()
	if len(content) == 0 {
		return "empty"
	}

	// 🔥 核心修复：快速检测空对象/空数组（无参数工具）
	contentStr := strings.TrimSpace(string(content))
	if contentStr == "{}" || contentStr == "[]" {
		// 空对象/数组是完全有效的 JSON，无需进一步解析
		var emptyResult map[string]any
		if contentStr == "{}" {
			emptyResult = make(map[string]any)
		}
		sjs.result = emptyResult
		sjs.state.hasValidJSON = true
		return "complete"
	}

	// 尝试使用Sonic完整JSON解析
	var result map[string]any
	if err := utils.FastUnmarshal(content, &result); err == nil {
		sjs.result = result
		sjs.state.hasValidJSON = true
		return "complete"
	}

	return "invalid"
}

// onAggregationComplete 聚合完成回调
func (ssja *SonicStreamingJSONAggregator) onAggregationComplete(toolUseId string, fullInput string) {
	if ssja.updateCallback != nil {
		ssja.updateCallback(toolUseId, fullInput)
	}
}

// cleanupStreamer 清理单个流式解析器，归还对象到池中
func (ssja *SonicStreamingJSONAggregator) cleanupStreamer(streamer *SonicJSONStreamer) {
	if streamer == nil {
		return
	}

	// Buffer 和 Map 由 GC 自动回收，无需手动清理
	streamer.buffer = nil
	streamer.result = nil
}
