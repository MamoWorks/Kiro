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

	// 使用单次 time.Now() 调用
	now := time.Now()

	// 检查是否过期
	if now.After(entry.ExpTime) {
		// 已过期，删除条目
		c.mu.Lock()
		delete(c.entries, hash)
		c.mu.Unlock()
		return nil, false
	}

	// 刷新 TTL（使用已获取的 now）
	c.mu.Lock()
	entry.ExpTime = calculateExpTimeFrom(now, entry.TTL)
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

// ProcessRequest 处理请求的缓存逻辑（官方前缀累计方式）
// 官方逻辑：cache_control 是断点标记，缓存的是从头到断点的所有内容的累计前缀。
// 断点处用前缀 hash 做 key，命中时 cache_read = 累计 token 数。
// 只有最后一个命中的断点生效（最长前缀匹配）。
func ProcessRequest(req types.AnthropicRequest, inputTokens int) *CacheResult {
	pc := GetGlobalCache()
	if pc == nil {
		return &CacheResult{TotalTokens: inputTokens}
	}

	estimator := utils.NewTokenEstimator()
	result := &CacheResult{TotalTokens: inputTokens}
	minTokens := GetMinCacheTokens(req.Model)

	// 收集所有内容块，按顺序构建前缀
	type contentItem struct {
		hash   string // 这个块自身内容的 hash
		tokens int    // 这个块的 token 数
		hasCc  bool   // 是否有 cache_control 断点
		ttl    string // ephemeral TTL
	}
	var items []contentItem

	// 处理 system 消息
	for _, sysMsg := range req.System {
		if sysMsg.Text == "" {
			continue
		}
		hash := computeHash(sysMsg.Text)
		tokens := estimator.EstimateTextTokens(sysMsg.Text) + 2
		hasCc := sysMsg.CacheControl != nil && sysMsg.CacheControl.Type == "ephemeral"
		ttl := ""
		if hasCc && sysMsg.CacheControl.TTL != "" {
			ttl = sysMsg.CacheControl.TTL
		}
		items = append(items, contentItem{hash: hash, tokens: tokens, hasCc: hasCc, ttl: ttl})
	}

	// 处理 tools（在 system 之后、messages 之前，参与前缀累计）
	for _, tool := range req.Tools {
		if tool.Name == "" {
			continue
		}
		data, err := json.Marshal(tool)
		if err != nil {
			continue
		}
		hash := computeHashBytes(data)
		tokens := estimator.EstimateToolUseTokens(tool.Name, tool.InputSchema)
		hasCc := tool.CacheControl != nil && tool.CacheControl.Type == "ephemeral"
		ttl := ""
		if hasCc && tool.CacheControl.TTL != "" {
			ttl = tool.CacheControl.TTL
		}
		items = append(items, contentItem{hash: hash, tokens: tokens, hasCc: hasCc, ttl: ttl})
	}

	// 处理 messages（按顺序遍历所有内容块）
	for _, msg := range req.Messages {
		switch content := msg.Content.(type) {
		case string:
			if content != "" {
				items = append(items, contentItem{
					hash: computeHash(content), tokens: estimator.EstimateTextTokens(content),
				})
			}
		case []any:
			for _, block := range content {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				item := extractContentItem(estimator, blockMap)
				if item != nil {
					items = append(items, *item)
				}
			}
		case []types.ContentBlock:
			for _, block := range content {
				item := extractTypedContentItem(estimator, block)
				if item != nil {
					items = append(items, *item)
				}
			}
		}
	}

	// 构建前缀 hash 并在断点处检查缓存
	// 前缀 hash = hash(block1.hash + block2.hash + ... + blockN.hash)
	var prefixParts []string
	var cumulativeTokens int

	// 记录最后一个命中的断点
	var lastReadTokens int
	var lastCreateTokens int
	var lastCreateTTL string
	var hasRead bool
	var hasCreate bool

	for _, item := range items {
		prefixParts = append(prefixParts, item.hash)
		cumulativeTokens += item.tokens

		if !item.hasCc {
			continue
		}

		// 到达断点，用前缀 hash 检查缓存
		prefixHash := computeHash(joinHashes(prefixParts))

		entry, exists := pc.Get(prefixHash)
		if exists {
			// 命中：记录这个断点的累计 token（后面的断点可能覆盖）
			lastReadTokens = entry.Tokens
			hasRead = true
			// 清除之前可能标记的 create（更长前缀命中了）
			hasCreate = false
		} else if cumulativeTokens >= minTokens {
			// 未命中且达到最小 token 要求：标记为待创建
			lastCreateTokens = cumulativeTokens
			lastCreateTTL = item.ttl
			if lastCreateTTL == "" {
				lastCreateTTL = "5m"
			}
			hasCreate = true
			// 不立即写入，等确定最终状态

			// 写入缓存
			pc.Set(prefixHash, cumulativeTokens, lastCreateTTL)
		}
	}

	// 应用最终结果：只报最后一个有效断点
	if hasRead {
		result.CacheReadTokens = lastReadTokens
	}
	if hasCreate {
		result.CacheCreationTokens = lastCreateTokens
	}

	return result
}

// extractContentItem 从 map 格式内容块提取缓存信息
func extractContentItem(estimator *utils.TokenEstimator, blockMap map[string]any) *struct {
	hash   string
	tokens int
	hasCc  bool
	ttl    string
} {
	blockType, _ := blockMap["type"].(string)

	var hash string
	var tokens int

	switch blockType {
	case "text":
		text, _ := blockMap["text"].(string)
		if text == "" {
			return nil
		}
		hash = computeHash(text)
		tokens = estimator.EstimateTextTokens(text)
	case "tool_use":
		data, err := json.Marshal(blockMap)
		if err != nil {
			return nil
		}
		hash = computeHashBytes(data)
		toolName, _ := blockMap["name"].(string)
		toolInput, _ := blockMap["input"].(map[string]any)
		tokens = estimator.EstimateToolUseTokens(toolName, toolInput)
	case "tool_result":
		data, err := json.Marshal(blockMap)
		if err != nil {
			return nil
		}
		hash = computeHashBytes(data)
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}
	case "image":
		data, err := json.Marshal(blockMap)
		if err != nil {
			return nil
		}
		hash = computeHashBytes(data)
		if source, ok := blockMap["source"].(map[string]any); ok {
			if imgData, ok := source["data"].(string); ok && imgData != "" {
				tokens = utils.EstimateImageTokensFromBase64(imgData)
			} else {
				tokens = 1500
			}
		} else {
			tokens = 1500
		}
	case "thinking":
		// thinking 块不参与缓存计算但参与前缀
		if thinking, ok := blockMap["thinking"].(string); ok && thinking != "" {
			hash = computeHash(thinking)
			tokens = len(thinking) / 4
		} else {
			return nil
		}
	default:
		data, err := json.Marshal(blockMap)
		if err != nil {
			return nil
		}
		hash = computeHashBytes(data)
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}
	}

	hasCc := false
	ttl := ""
	if ccRaw, ok := blockMap["cache_control"]; ok && ccRaw != nil {
		if ccMap, ok := ccRaw.(map[string]any); ok {
			if getStr(ccMap, "type") == "ephemeral" {
				hasCc = true
				ttl = getStr(ccMap, "ttl")
			}
		}
	}

	return &struct {
		hash   string
		tokens int
		hasCc  bool
		ttl    string
	}{hash: hash, tokens: tokens, hasCc: hasCc, ttl: ttl}
}

// extractTypedContentItem 从结构化内容块提取缓存信息
func extractTypedContentItem(estimator *utils.TokenEstimator, block types.ContentBlock) *struct {
	hash   string
	tokens int
	hasCc  bool
	ttl    string
} {
	var hash string
	var tokens int

	switch block.Type {
	case "text":
		if block.Text == nil || *block.Text == "" {
			return nil
		}
		hash = computeHash(*block.Text)
		tokens = estimator.EstimateTextTokens(*block.Text)
	case "tool_use":
		data, _ := json.Marshal(block)
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
	case "image":
		data, _ := json.Marshal(block)
		hash = computeHashBytes(data)
		if block.Source != nil && block.Source.Data != "" {
			tokens = utils.EstimateImageTokensFromBase64(block.Source.Data)
		} else {
			tokens = 1500
		}
	default:
		data, _ := json.Marshal(block)
		hash = computeHashBytes(data)
		tokens = len(data) / 4
		if tokens < 1 {
			tokens = 1
		}
	}

	hasCc := block.CacheControl != nil && block.CacheControl.Type == "ephemeral"
	ttl := ""
	if hasCc && block.CacheControl.TTL != "" {
		ttl = block.CacheControl.TTL
	}

	return &struct {
		hash   string
		tokens int
		hasCc  bool
		ttl    string
	}{hash: hash, tokens: tokens, hasCc: hasCc, ttl: ttl}
}

// joinHashes 拼接 hash 列表用于前缀 hash
func joinHashes(hashes []string) string {
	result := ""
	for _, h := range hashes {
		result += h + "|"
	}
	return result
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
	return calculateExpTimeFrom(time.Now(), ttl)
}

// calculateExpTimeFrom 基于给定时间计算过期时间（避免重复调用 time.Now()）
func calculateExpTimeFrom(now time.Time, ttl string) time.Time {
	switch ttl {
	case "1h":
		return now.Add(1 * time.Hour)
	default:
		// 默认 5 分钟
		return now.Add(5 * time.Minute)
	}
}

// getStr 从 map 中安全获取字符串
func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// GetMinCacheTokens 根据模型返回最小可缓存 token 数
// 参考: https://platform.claude.com/docs/en/build-with-claude/prompt-caching#cache-limitations
func GetMinCacheTokens(model string) int {
	// Claude Opus 4.5: 4096
	// Claude Opus 4.1/4, Sonnet 4.5/4/3.7: 1024
	// Claude Haiku 4.5: 4096
	// Claude Haiku 3.5/3: 2048
	switch {
	case utils.ContainsAny(model, "opus-4-5", "opus-4.5"):
		return 4096
	case utils.ContainsAny(model, "opus"):
		return 1024
	case utils.ContainsAny(model, "haiku-4-5", "haiku-4.5"):
		return 4096
	case utils.ContainsAny(model, "haiku"):
		return 2048
	case utils.ContainsAny(model, "sonnet"):
		return 1024
	default:
		return 1024 // 默认使用最小值
	}
}
