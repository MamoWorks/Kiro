package config

import (
	"os"
	"strconv"
)

// ModelMap 模型映射表（映射到 CodeWhisperer 实际支持的模型 ID）
// 注意：当模型不在映射表中时，将直接透传原始模型ID
var ModelMap = map[string]string{
	"claude-opus-4-6":            "claude-opus-4-6",
	"claude-sonnet-4-6":          "claude-sonnet-4-6",
	"claude-opus-4-5-20251101":   "claude-opus-4.5",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4.5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4.5",
}

// RefreshTokenURL Kiro 刷新token的URL (Kiro Desktop 端点，用于原生 Kiro refresh token)
const RefreshTokenURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

// KiroRefreshHeaders Kiro 原生 refresh token 请求头 (伪装 Kiro CLI 1.27.2)
var KiroRefreshHeaders = map[string]string{
	"content-type": "application/json",
	"user-agent":   "aws-sdk-rust/1.3.10 os/macos lang/rust/1.86.0",
}

// AmazonQTokenURL AmazonQ OIDC token刷新URL
const AmazonQTokenURL = "https://oidc.us-east-1.amazonaws.com/token"

// AmazonQOIDCHeaders AmazonQ OIDC 认证请求头 (伪装 Kiro CLI 1.27.2 Rust SDK)
var AmazonQOIDCHeaders = map[string]string{
	"content-type":     "application/json",
	"user-agent":       "aws-sdk-rust/1.3.10 os/macos lang/rust/1.86.0",
	"x-amz-user-agent": "aws-sdk-rust/1.3.10 ua/2.1 api/ssooidc/1.92.0 os/macos lang/rust/1.86.0 m/E app/AmazonQ-For-KIRO_CLI",
	"amz-sdk-request":  "attempt=1; max=3",
}

// CodeWhispererURL Kiro API 的 URL (使用根路径，通过 x-amz-target 头路由)
const CodeWhispererURL = "https://q.us-east-1.amazonaws.com"

// MCPURL MCP 端点 URL
const MCPURL = "https://q.us-east-1.amazonaws.com/mcp"

// KiroCLIVersion Kiro CLI 版本号 (从最新二进制 BUILD-INFO 提取)
const KiroCLIVersion = "1.27.2"

// MaxToolDescriptionLength 工具描述的最大长度（字符数）
// 可通过环境变量 MAX_TOOL_DESCRIPTION_LENGTH 配置，默认 10000
var MaxToolDescriptionLength = getEnvIntWithDefault("MAX_TOOL_DESCRIPTION_LENGTH", 10000)

// getEnvIntWithDefault 获取整数类型环境变量（带默认值）
func getEnvIntWithDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
