package server

import (
	"net/http"
	"strings"

	"kiro/utils"

	"github.com/gin-gonic/gin"
)

/**
 * AuthMiddleware 认证中间件，支持 x-api-key 和 Authorization Bearer 两种格式
 */
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 优先使用 x-api-key（Claude 格式）
		token := c.GetHeader("x-api-key")

		// 如果没有 x-api-key，尝试从 Authorization header 获取（Bearer 格式）
		if token == "" {
			authHeader := c.GetHeader("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Missing authentication. Provide Authorization header or x-api-key",
				},
			})
			c.Abort()
			return
		}

		// 获取或刷新 access token
		accessToken, err := GetOrRefreshToken(token)
		if err != nil {
			utils.Error("Token 认证失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"type":    "authentication_error",
					"message": "Identity verification fails, please check its validity",
				},
			})
			c.Abort()
			return
		}

		// 将 access token 和原始 refresh token 存入上下文
		c.Set("accessToken", accessToken)
		c.Set("refreshToken", token)
		c.Next()
	}
}

/**
 * RequestIDMiddleware 为每个请求注入 request_id 并通过响应头返回
 */
func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = "req_" + utils.GenerateUUID()
		}
		c.Set("request_id", rid)
		c.Writer.Header().Set("X-Request-ID", rid)
		c.Next()
	}
}

/**
 * GetRequestID 从上下文读取 request_id
 */
func GetRequestID(c *gin.Context) string {
	if v, ok := c.Get("request_id"); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

/**
 * GetMessageID 从上下文读取 message_id
 */
func GetMessageID(c *gin.Context) string {
	if v, ok := c.Get("message_id"); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

/**
 * addReqFields 注入标准请求字段，统一日志追踪
 */
func addReqFields(c *gin.Context, fields ...utils.LogField) []utils.LogField {
	rid := GetRequestID(c)
	mid := GetMessageID(c)
	out := make([]utils.LogField, 0, len(fields)+2)
	if rid != "" {
		out = append(out, utils.LogString("request_id", rid))
	}
	if mid != "" {
		out = append(out, utils.LogString("message_id", mid))
	}
	out = append(out, fields...)
	return out
}
