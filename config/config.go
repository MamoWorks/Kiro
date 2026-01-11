package config

import (
	"os"
	"strconv"
)

// ModelMap 模型映射表（映射到 CodeWhisperer 实际支持的模型 ID）
var ModelMap = map[string]string{
	// ===== Claude 4.5 系列（原生支持）=====
	"claude-opus-4-5":            "claude-opus-4.5",
	"claude-opus-4-5-20251101":   "claude-opus-4.5",
	"claude-sonnet-4-5":          "claude-sonnet-4.5",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4.5",
	"claude-haiku-4-5":           "claude-haiku-4.5",
	"claude-haiku-4-5-20251001":  "claude-haiku-4.5",

	// Claude 4.5 思维链模式
	"claude-opus-4-5-thinking":            "claude-opus-4.5",
	"claude-opus-4-5-20251101-thinking":   "claude-opus-4.5",
	"claude-sonnet-4-5-thinking":          "claude-sonnet-4.5",
	"claude-sonnet-4-5-20250929-thinking": "claude-sonnet-4.5",
	"claude-haiku-4-5-thinking":           "claude-haiku-4.5",
	"claude-haiku-4-5-20251001-thinking":  "claude-haiku-4.5",

	// ===== Claude 4.0 Sonnet 系列（已验证支持）=====
	"claude-sonnet-4-20250514":         "claude-sonnet-4.5", // 映射到 4.5
	"claude-sonnet-4":                  "claude-sonnet-4.5",
	"claude-4-sonnet":                  "claude-sonnet-4.5",
	"claude-sonnet-4-20250514-thinking": "claude-sonnet-4.5",
	"claude-sonnet-4-thinking":          "claude-sonnet-4.5",
	"claude-4-sonnet-thinking":          "claude-sonnet-4.5",

	// ===== Claude 4.1 Opus / 4.0 Opus 系列（映射到 4.5 Opus）=====
	"claude-opus-4-1-20250805":         "claude-opus-4.5", // CodeWhisperer 不支持 4.1，映射到 4.5
	"claude-opus-4-1":                  "claude-opus-4.5",
	"claude-opus-4-1-20250805-thinking": "claude-opus-4.5",
	"claude-opus-4-1-thinking":          "claude-opus-4.5",

	"claude-opus-4-20250514":         "claude-opus-4.5", // CodeWhisperer 不支持单独的 4.0，映射到 4.5
	"claude-opus-4":                  "claude-opus-4.5",
	"claude-4-opus":                  "claude-opus-4.5",
	"claude-opus-4-20250514-thinking": "claude-opus-4.5",
	"claude-opus-4-thinking":          "claude-opus-4.5",
	"claude-4-opus-thinking":          "claude-opus-4.5",

	// ===== Claude 3.7 Sonnet 系列（映射到 4.5 Sonnet）=====
	"claude-3-7-sonnet-20250219":         "claude-sonnet-4.5", // CodeWhisperer 不支持 3.7，映射到 4.5
	"claude-3-7-sonnet":                  "claude-sonnet-4.5",
	"claude-sonnet-3-7":                  "claude-sonnet-4.5",
	"claude-3-7-sonnet-20250219-thinking": "claude-sonnet-4.5",
	"claude-3-7-sonnet-thinking":          "claude-sonnet-4.5",
	"claude-sonnet-3-7-thinking":          "claude-sonnet-4.5",

	// ===== Claude 3.5 Haiku 系列（映射到 4.5 Haiku）=====
	"claude-3-5-haiku-20241022":         "claude-haiku-4.5", // CodeWhisperer 不支持 3.5 Haiku，映射到 4.5
	"claude-3-5-haiku":                  "claude-haiku-4.5",
	"claude-haiku-3-5":                  "claude-haiku-4.5",
	"claude-3-5-haiku-20241022-thinking": "claude-haiku-4.5",
	"claude-3-5-haiku-thinking":          "claude-haiku-4.5",
	"claude-haiku-3-5-thinking":          "claude-haiku-4.5",

	// ===== Claude 3.5 Sonnet 系列（映射到 4.5 Sonnet）=====
	"claude-3-5-sonnet-20241022": "claude-sonnet-4.5",
	"claude-3-5-sonnet":          "claude-sonnet-4.5",
	"claude-sonnet-3-5":          "claude-sonnet-4.5",
	"claude-3-5-sonnet-20241022-thinking": "claude-sonnet-4.5",
	"claude-3-5-sonnet-thinking":          "claude-sonnet-4.5",
	"claude-sonnet-3-5-thinking":          "claude-sonnet-4.5",
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
