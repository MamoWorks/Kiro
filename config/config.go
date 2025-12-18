package config

import (
	"os"
	"strconv"
)

// ModelMap 模型映射表（使用 CLIProxyAPIPlus 格式）
var ModelMap = map[string]string{
	"claude-opus-4-5":            "claude-opus-4.5",
	"claude-opus-4-5-20251101":   "claude-opus-4.5",
	"claude-sonnet-4-5":          "claude-sonnet-4.5",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4.5",
	"claude-haiku-4-5":           "claude-haiku-4.5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4.5",
}

// RefreshTokenURL Kiro 刷新token的URL
const RefreshTokenURL = "https://prod.us-east-1.auth.desktop.kiro.dev/refreshToken"

// AmazonQTokenURL AmazonQ OIDC token刷新URL
const AmazonQTokenURL = "https://oidc.us-east-1.amazonaws.com/token"

// AmazonQOIDCHeaders AmazonQ OIDC 认证请求头
var AmazonQOIDCHeaders = map[string]string{
	"content-type":     "application/json",
	"user-agent":       "aws-sdk-rust/1.3.9 os/windows lang/rust/1.87.0",
	"x-amz-user-agent": "aws-sdk-rust/1.3.9 ua/2.1 api/ssooidc/1.88.0 os/windows lang/rust/1.87.0 m/E app/AmazonQ-For-CLI",
	"amz-sdk-request":  "attempt=1; max=3",
}

// CodeWhispererURL CodeWhisperer API的URL
const CodeWhispererURL = "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse"

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
