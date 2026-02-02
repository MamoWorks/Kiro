package cache

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"kiro/types"
	"kiro/utils"
)

// CacheEntry 表示单个缓存条目
type CacheEntry struct {
	Tokens  int       // 该内容的 token 数
	ExpTime time.Time // 过期时间
	TTL     string    // "5m" 或 "1h"，用于刷新
}

// CacheResult 表示缓存处理结果
type CacheResult struct {
	TotalTokens         int // 总 token 数（等于 inputTokens）
	CacheCreationTokens int // 新创建缓存的 token 数
	CacheReadTokens     int // 命中缓存的 token 数
}

// PromptCache 提示缓存管理器
type PromptCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
}

// globalCache 全局缓存实例
var globalCache *PromptCache

// InitGlobalCache 初始化全局缓存并启动清理协程
func InitGlobalCache(cleanInterval time.Duration) {
	globalCache = NewPromptCache()
	globalCache.StartCleaner(cleanInterval)
	utils.Log("Prompt Cache 已初始化",
		utils.LogString("clean_interval", cleanInterval.String()))
}

// GetGlobalCache 获取全局缓存实例
func GetGlobalCache() *PromptCache {
	return globalCache
}

// NewPromptCache 创建新的缓存实例
func NewPromptCache() *PromptCache {
	return &PromptCache{
		entries: make(map[string]*CacheEntry),
	}
}

// Get 获取缓存条目并刷新 TTL
func (c *PromptCache) Get(hash string) (*CacheEntry, bool) {
	c.mu.RLock()
	entry, exists := c.entries[hash]
	c.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Now().After(entry.ExpTime) {
		// 已过期，删除条目
		c.mu.Lock()
		delete(c.entries, hash)
		c.mu.Unlock()
		return nil, false
	}

	// 刷新 TTL
	c.mu.Lock()
	entry.ExpTime = calculateExpTime(entry.TTL)
	c.mu.Unlock()

	return entry, true
}

// Set 创建缓存条目
func (c *PromptCache) Set(hash string, tokens int, ttl string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[hash] = &CacheEntry{
		Tokens:  tokens,
		ExpTime: calculateExpTime(ttl),
		TTL:     ttl,
	}
}

// CleanExpired 清理所有过期条目
func (c *PromptCache) CleanExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	cleaned := 0
	for hash, entry := range c.entries {
		if now.After(entry.ExpTime) {
			delete(c.entries, hash)
			cleaned++
		}
	}

	if cleaned > 0 {
		utils.Log("Prompt Cache 清理完成",
			utils.LogInt("cleaned", cleaned),
			utils.LogInt("remaining", len(c.entries)))
	}
}

// StartCleaner 启动定期清理协程
func (c *PromptCache) StartCleaner(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			c.CleanExpired()
		}
	}()
}

// Size 返回当前缓存条目数（用于调试）
func (c *PromptCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// ProcessRequest 处理请求的缓存逻辑，返回缓存命中/创建的 token 统计
func ProcessRequest(req types.AnthropicRequest, inputTokens int) *CacheResult {
	pc := GetGlobalCache()
	if pc == nil {
		return &CacheResult{TotalTokens: inputTokens}
	}

	estimator := utils.NewTokenEstimator()
	result := &CacheResult{TotalTokens: inputTokens}

	// 处理 system 消息
	for _, sysMsg := range req.System {
		if sysMsg.Text == "" {
			continue
		}
		hash := computeHash(sysMsg.Text)
		tokens := estimator.EstimateTextTokens(sysMsg.Text) + 2 // 系统提示固定开销

		processContentBlock(pc, hash, tokens, sysMsg.CacheControl, result)
	}

	// 处理 messages
	for _, msg := range req.Messages {
		switch content := msg.Content.(type) {
		case string:
			if content == "" {
				continue
			}
			hash := computeHash(content)
			tokens := estimator.EstimateTextTokens(content)
			// 无 cache_control 标记的字符串消息，仅自动命中
			processContentBlock(pc, hash, tokens, nil, result)

		case []any:
			for _, block := range content {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				processRawContentBlock(pc, estimator, blockMap, result)
			}

		case []types.ContentBlock:
			for _, block := range content {
				processTypedContentBlock(pc, estimator, block, result)
			}
		}
	}

	return result
}

// processContentBlock 处理单个内容块的缓存逻辑
func processContentBlock(pc *PromptCache, hash string, tokens int, cc *types.CacheControl, result *CacheResult) {
	hasCacheControl := cc != nil && cc.Type == "ephemeral"

	entry, exists := pc.Get(hash)
	if exists {
		// 缓存命中（无论是否带 cache_control 标记）
		result.CacheReadTokens += entry.Tokens
	} else if hasCacheControl {
		// 有 cache_control 标记且缓存不存在 → 创建缓存
		ttl := cc.TTL
		if ttl == "" {
			ttl = "5m" // 默认 5 分钟
		}
		pc.Set(hash, tokens, ttl)
		result.CacheCreationTokens += tokens
	}
	// 无 cache_control 且缓存不存在 → 不创建缓存，正常计算
}

// processRawContentBlock 处理原始 map 格式的内容块
func processRawContentBlock(pc *PromptCache, estimator *utils.TokenEstimator, blockMap map[string]any, result *CacheResult) {
	blockType, _ := blockMap["type"].(string)

	// 提取 cache_control
	var cc *types.CacheControl
	if ccRaw, ok := blockMap["cache_control"]; ok && ccRaw != nil {
		if ccMap, ok := ccRaw.(map[string]any); ok {
			cc = &types.CacheControl{
				Type: getStr(ccMap, "type"),
				TTL:  getStr(ccMap, "ttl"),
			}
		}
	}

	// 计算哈希和 tokens
	var hash string
	var tokens int

	switch blockType {
	case "text":
		text, _ := blockMap["text"].(string)
		if text == "" {
			return
		}
		hash = computeHash(text)
		tokens = estimator.EstimateTextTokens(text)

	case "tool_use":
		// 序列化整个块计算哈希
		data, err := json.Marshal(blockMap)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
		toolName, _ := blockMap["name"].(string)
		toolInput, _ := blockMap["input"].(map[string]any)
		tokens = estimator.EstimateToolUseTokens(toolName, toolInput)

	case "tool_result":
		data, err := json.Marshal(blockMap)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
		// 粗略估算 tool_result token
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}

	case "image":
		// 图片使用 source hash
		data, err := json.Marshal(blockMap)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
		tokens = 1500 // 图片固定估算

	default:
		data, err := json.Marshal(blockMap)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}
	}

	processContentBlock(pc, hash, tokens, cc, result)
}

// processTypedContentBlock 处理类型化内容块
func processTypedContentBlock(pc *PromptCache, estimator *utils.TokenEstimator, block types.ContentBlock, result *CacheResult) {
	var hash string
	var tokens int

	switch block.Type {
	case "text":
		if block.Text == nil || *block.Text == "" {
			return
		}
		hash = computeHash(*block.Text)
		tokens = estimator.EstimateTextTokens(*block.Text)

	case "tool_use":
		data, err := json.Marshal(block)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
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
		tokens = estimator.EstimateToolUseTokens(toolName, toolInput)

	default:
		data, err := json.Marshal(block)
		if err != nil {
			return
		}
		hash = computeHashBytes(data)
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}
	}

	processContentBlock(pc, hash, tokens, block.CacheControl, result)
}

// computeHash 计算字符串内容的 SHA-256 哈希
func computeHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// computeHashBytes 计算字节内容的 SHA-256 哈希
func computeHashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}

// calculateExpTime 根据 TTL 字符串计算过期时间
func calculateExpTime(ttl string) time.Time {
	switch ttl {
	case "1h":
		return time.Now().Add(1 * time.Hour)
	default:
		// 默认 5 分钟
		return time.Now().Add(5 * time.Minute)
	}
}

// getStr 从 map 中安全获取字符串
func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
