package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kiro/config"
	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// MCP JSON-RPC 请求/响应结构
type mcpRequest struct {
	ID      string    `json:"id"`
	JSONRPC string    `json:"jsonrpc"`
	Method  string    `json:"method"`
	Params  mcpParams `json:"params"`
}

type mcpParams struct {
	Name      string       `json:"name"`
	Arguments mcpArguments `json:"arguments"`
}

type mcpArguments struct {
	Query string `json:"query"`
}

type mcpResponse struct {
	ID      string     `json:"id"`
	JSONRPC string     `json:"jsonrpc"`
	Result  *mcpResult `json:"result,omitempty"`
	Error   *mcpError  `json:"error,omitempty"`
}

type mcpResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type webSearchResults struct {
	Results      []webSearchResult `json:"results"`
	TotalResults int               `json:"totalResults,omitempty"`
	Query        string            `json:"query,omitempty"`
}

type webSearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Snippet     string `json:"snippet,omitempty"`
	Domain      string `json:"domain,omitempty"`
	PublishedAt int64  `json:"published_date,omitempty"`
}

// hasWebSearchTool 检查请求中是否包含 web_search 工具
func hasWebSearchTool(req types.AnthropicRequest) bool {
	for _, tool := range req.Tools {
		if tool.Name == "web_search" || tool.Name == "websearch" {
			return true
		}
	}
	return false
}

// getWebSearchMaxUses 获取 web_search 工具的 max_uses 限制
func getWebSearchMaxUses(req types.AnthropicRequest) int {
	// Anthropic web_search tool 没有标准的 max_uses 字段
	// 默认返回 5
	return 5
}

// extractSearchQuery 从请求中提取搜索查询
func extractSearchQuery(req types.AnthropicRequest) string {
	if len(req.Messages) == 0 {
		return ""
	}

	// 从最后一条用户消息中提取
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			text := extractTextFromUserMessage(req.Messages[i].Content)
			// 去掉常见前缀
			prefix := "Perform a web search for the query: "
			if strings.HasPrefix(text, prefix) {
				return strings.TrimSpace(text[len(prefix):])
			}
			return text
		}
	}
	return ""
}

// extractTextFromUserMessage 从用户消息内容中提取文本
func extractTextFromUserMessage(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if blockType, _ := block["type"].(string); blockType == "text" {
					if text, ok := block["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}

// handleMCPWebSearch 处理包含 web_search 的请求（支持流式 SSE 和非流式 JSON）
func handleMCPWebSearch(c *gin.Context, anthropicReq types.AnthropicRequest, token types.TokenInfo) {
	estimator := utils.NewTokenEstimator()
	countReq := &types.CountTokensRequest{
		Model:    anthropicReq.Model,
		System:   anthropicReq.System,
		Messages: anthropicReq.Messages,
		Tools:    anthropicReq.Tools,
	}
	inputTokens := estimator.EstimateTokens(countReq)

	query := extractSearchQuery(anthropicReq)
	if query == "" {
		respondError(c, http.StatusBadRequest, "无法从请求中提取搜索查询")
		return
	}

	maxUses := getWebSearchMaxUses(anthropicReq)

	// 构建 MCP 请求
	mcpReqID := fmt.Sprintf("web_search_%s_%d", utils.GenerateUUID()[:22], time.Now().UnixMilli())
	mcpReq := mcpRequest{
		ID:      mcpReqID,
		JSONRPC: "2.0",
		Method:  "tools/call",
		Params: mcpParams{
			Name:      "web_search",
			Arguments: mcpArguments{Query: query},
		},
	}

	jsonBytes, err := json.Marshal(mcpReq)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "序列化 MCP 请求失败: %v", err)
		return
	}

	// 发送 MCP 请求
	httpReq, err := http.NewRequest("POST", config.MCPURL, bytes.NewReader(jsonBytes))
	if err != nil {
		respondError(c, http.StatusInternalServerError, "创建 MCP 请求失败: %v", err)
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	httpReq.Header.Set("user-agent", "aws-sdk-rust/1.3.10 ua/2.1 api/codewhispererstreaming/0.1.10231 os/macos lang/rust/1.86.0 md/appVersion-"+config.KiroCLIVersion+" app/AmazonQ-For-CLI")
	httpReq.Header.Set("x-amz-user-agent", "aws-sdk-rust/1.3.10 ua/2.1 api/codewhispererstreaming/0.1.10231 os/macos lang/rust/1.86.0 m/F app/AmazonQ-For-CLI")

	proxyKey, _ := c.Get("tokenHash")
	proxyKeyStr, _ := proxyKey.(string)
	resp, err := utils.DoRequestWithProxy(httpReq, proxyKeyStr)
	if err != nil {
		utils.Error("MCP 请求失败: %v", err)
		respondError(c, http.StatusBadGateway, "MCP 服务不可用")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var mcpResp mcpResponse
	if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &mcpResp) != nil || mcpResp.Error != nil {
		errMsg := "MCP 请求失败"
		if mcpResp.Error != nil {
			errMsg = mcpResp.Error.Message
		}
		utils.Error("MCP 响应错误: status=%d, body=%s", resp.StatusCode, string(body))
		respondError(c, http.StatusBadGateway, "%s", errMsg)
		return
	}

	// 解析搜索结果
	var searchResults *webSearchResults
	if mcpResp.Result != nil && len(mcpResp.Result.Content) > 0 {
		for _, content := range mcpResp.Result.Content {
			if content.Type == "text" {
				var results webSearchResults
				if json.Unmarshal([]byte(content.Text), &results) == nil {
					searchResults = &results
					break
				}
			}
		}
	}

	// 构建通用数据
	toolUseID := "srvtoolu_" + strings.ReplaceAll(utils.GenerateUUID(), "-", "")[:32]
	msgID := fmt.Sprintf(config.MessageIDFormat, utils.GenerateBase62ID(22))

	// 构建搜索结果内容
	searchContent := []any{}
	if searchResults != nil {
		limit := len(searchResults.Results)
		if maxUses > 0 && maxUses < limit {
			limit = maxUses
		}
		for i := 0; i < limit; i++ {
			result := searchResults.Results[i]
			searchContent = append(searchContent, map[string]any{
				"type":              "web_search_result",
				"title":             result.Title,
				"url":               result.URL,
				"encrypted_content": result.Snippet,
				"page_age":          nil,
			})
		}
	}

	// 生成文本摘要
	summary := fmt.Sprintf("以下是关于 \"%s\" 的搜索结果：\n\n", query)
	if searchResults != nil && len(searchResults.Results) > 0 {
		limit := len(searchResults.Results)
		if maxUses > 0 && maxUses < limit {
			limit = maxUses
		}
		for i := 0; i < limit; i++ {
			result := searchResults.Results[i]
			summary += fmt.Sprintf("%d. **%s**\n", i+1, result.Title)
			if result.Snippet != "" {
				snippet := result.Snippet
				if len(snippet) > 200 {
					snippet = snippet[:200] + "..."
				}
				summary += fmt.Sprintf("   %s\n", snippet)
			}
			summary += fmt.Sprintf("   来源: %s\n\n", result.URL)
		}
	} else {
		summary += "未找到相关结果。\n"
	}

	outputTokens := estimator.EstimateTextTokens(summary)

	// 非流式响应：返回完整 JSON
	if !anthropicReq.Stream {
		contentBlocks := []any{
			map[string]any{
				"type":  "server_tool_use",
				"id":    toolUseID,
				"name":  "web_search",
				"input": map[string]string{"query": query},
			},
			map[string]any{
				"type":        "web_search_tool_result",
				"tool_use_id": toolUseID,
				"content":     searchContent,
			},
			map[string]any{
				"type": "text",
				"text": summary,
			},
		}

		c.JSON(http.StatusOK, map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         anthropicReq.Model,
			"content":       contentBlocks,
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": outputTokens,
				"service_tier":  "standard",
			},
		})

		utils.Info("MCP web_search 完成 (非流式): query=%s, results=%d", query, len(searchContent))
		return
	}

	// 流式响应：SSE 输出
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	sender := &AnthropicStreamSender{}

	// 1. message_start
	sender.SendEvent(c, map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"model":         anthropicReq.Model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  inputTokens,
				"output_tokens": 0,
				"service_tier":  "standard",
			},
		},
	})

	// 2. content_block_start: server_tool_use (web_search)
	sender.SendEvent(c, map[string]any{
		"type":  "content_block_start",
		"index": 0,
		"content_block": map[string]any{
			"id":    toolUseID,
			"type":  "server_tool_use",
			"name":  "web_search",
			"input": map[string]any{},
		},
	})

	// 3. content_block_delta: input_json
	inputJSON, _ := json.Marshal(map[string]string{"query": query})
	sender.SendEvent(c, map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": string(inputJSON),
		},
	})

	// 4. content_block_stop
	sender.SendEvent(c, map[string]any{
		"type":  "content_block_stop",
		"index": 0,
	})

	// 5. content_block_start: web_search_tool_result
	sender.SendEvent(c, map[string]any{
		"type":  "content_block_start",
		"index": 1,
		"content_block": map[string]any{
			"type":        "web_search_tool_result",
			"tool_use_id": toolUseID,
			"content":     searchContent,
		},
	})

	sender.SendEvent(c, map[string]any{
		"type":  "content_block_stop",
		"index": 1,
	})

	// 6. 文本摘要
	sender.SendEvent(c, map[string]any{
		"type":  "content_block_start",
		"index": 2,
		"content_block": map[string]any{
			"type": "text",
			"text": "",
		},
	})

	sender.SendEvent(c, map[string]any{
		"type":  "content_block_delta",
		"index": 2,
		"delta": map[string]any{
			"type": "text_delta",
			"text": summary,
		},
	})

	sender.SendEvent(c, map[string]any{
		"type":  "content_block_stop",
		"index": 2,
	})

	// 7. message_delta + message_stop
	sender.SendEvent(c, map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"output_tokens": outputTokens,
			"service_tier":  "standard",
		},
	})

	sender.SendEvent(c, map[string]any{
		"type": "message_stop",
	})

	utils.Info("MCP web_search 完成 (流式): query=%s, results=%d", query, len(searchContent))
}
