package server

import (
	"net/http"
	"os"
	"strings"

	"kiro/config"

	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

/**
 * StartServer 启动HTTP代理服务器
 */
func StartServer(port string) {
	// 设置 gin 模式
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)

	r := gin.New()

	// 添加中间件
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(RequestIDMiddleware())
	r.Use(corsMiddleware())

	// 根路径重定向（无需认证）
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "https://www.bilibili.com/video/BV1cp4y1Q7yn")
	})

	r.Use(AuthMiddleware()) // 应用到所有 API 端点

	// GET /v1/models 端点
	r.GET("/v1/models", func(c *gin.Context) {
		// 构建模型列表
		models := []types.Model{}
		for anthropicModel := range config.ModelMap {
			model := types.Model{
				ID:          anthropicModel,
				Object:      "model",
				Created:     1234567890,
				OwnedBy:     "anthropic",
				DisplayName: anthropicModel,
				Type:        "text",
				MaxTokens:   200000,
			}
			models = append(models, model)
		}

		response := types.ModelsResponse{
			Object: "list",
			Data:   models,
		}

		c.JSON(http.StatusOK, response)
	})

	// POST /v1/messages 端点
	r.POST("/v1/messages", func(c *gin.Context) {
		// 从上下文获取 access token
		accessToken, exists := c.Get("accessToken")
		if !exists {
			respondError(c, http.StatusUnauthorized, "%s", "未找到访问令牌")
			return
		}

		tokenInfo := types.TokenInfo{
			AccessToken: accessToken.(string),
		}

		// 读取请求体
		body, err := c.GetRawData()
		if err != nil {
			utils.Error("读取请求体失败: %v", err)
			respondError(c, http.StatusBadRequest, "读取请求体失败: %v", err)
			return
		}

		// 先解析为通用map以便处理工具格式
		var rawReq map[string]any
		if err := utils.SafeUnmarshal(body, &rawReq); err != nil {
			utils.Error("解析请求体失败: %v", err)
			respondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
			return
		}

		// 标准化工具格式处理
		if tools, exists := rawReq["tools"]; exists && tools != nil {
			if toolsArray, ok := tools.([]any); ok {
				normalizedTools := make([]map[string]any, 0, len(toolsArray))
				for _, tool := range toolsArray {
					if toolMap, ok := tool.(map[string]any); ok {
						if name, hasName := toolMap["name"]; hasName {
							if description, hasDesc := toolMap["description"]; hasDesc {
								if inputSchema, hasSchema := toolMap["input_schema"]; hasSchema {
									normalizedTool := map[string]any{
										"name":         name,
										"description":  description,
										"input_schema": inputSchema,
									}
									normalizedTools = append(normalizedTools, normalizedTool)
									continue
								}
							}
						}
						normalizedTools = append(normalizedTools, toolMap)
					}
				}
				rawReq["tools"] = normalizedTools
			}
		}

		// 重新序列化并解析为AnthropicRequest
		normalizedBody, err := utils.SafeMarshal(rawReq)
		if err != nil {
			utils.Error("重新序列化请求失败: %v", err)
			respondError(c, http.StatusBadRequest, "处理请求格式失败: %v", err)
			return
		}

		var anthropicReq types.AnthropicRequest
		if err := utils.SafeUnmarshal(normalizedBody, &anthropicReq); err != nil {
			utils.Error("解析标准化请求体失败: %v", err)
			respondError(c, http.StatusBadRequest, "解析请求体失败: %v", err)
			return
		}

		// 验证请求的有效性
		if len(anthropicReq.Messages) == 0 {
			utils.Error("请求中没有消息")
			respondError(c, http.StatusBadRequest, "%s", "messages 数组不能为空")
			return
		}

		// 验证最后一条消息有有效内容
		lastMsg := anthropicReq.Messages[len(anthropicReq.Messages)-1]
		content, err := utils.GetMessageContent(lastMsg.Content)
		if err != nil {
			utils.Error("获取消息内容失败: %v", err)
			respondError(c, http.StatusBadRequest, "获取消息内容失败: %v", err)
			return
		}

		trimmedContent := strings.TrimSpace(content)
		if trimmedContent == "" || trimmedContent == "answer for user question" {
			respondError(c, http.StatusBadRequest, "%s", "消息内容不能为空")
			return
		}

		if anthropicReq.Stream {
			handleStreamRequest(c, anthropicReq, tokenInfo)
			return
		}

		handleNonStreamRequest(c, anthropicReq, tokenInfo)
	})

	// Token计数端点
	r.POST("/v1/messages/count_tokens", handleCountTokens)

	r.NoRoute(func(c *gin.Context) {
		respondError(c, http.StatusNotFound, "%s", "404 未找到")
	})

	// 创建自定义HTTP服务器以支持长时间请求
	server := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		utils.Error("启动服务器失败: %v, port: %s", err, port)
		os.Exit(1)
	}
}

/**
 * corsMiddleware CORS中间件
 */
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, x-api-key, X-CSRF-Token")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}
