package types

import (
	"time"
)

// Token 统一的token管理结构，合并了TokenInfo、RefreshResponse、RefreshRequest的功能
type Token struct {
	// 核心token信息
	AccessToken  string    `json:"accessToken,omitempty"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`

	// API响应字段
	ExpiresIn  int    `json:"expiresIn,omitempty"`  // 多少秒后失效，来自RefreshResponse
	ProfileArn string `json:"profileArn,omitempty"` // 来自RefreshResponse
}

// FromRefreshResponse 从RefreshResponse创建Token
func (t *Token) FromRefreshResponse(resp RefreshResponse, originalRefreshToken string) {
	t.AccessToken = resp.AccessToken
	t.RefreshToken = originalRefreshToken // 保持原始refresh token
	t.ExpiresIn = resp.ExpiresIn
	t.ProfileArn = resp.ProfileArn
	t.ExpiresAt = time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
}

// IsExpired 检查token是否已过期
func (t *Token) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// TokenInfo Token的类型别名
type TokenInfo = Token
// RefreshResponse token刷新响应结构
type RefreshResponse struct {
	AccessToken  string `json:"accessToken"`
	ExpiresIn    int    `json:"expiresIn"`
	RefreshToken string `json:"refreshToken,omitempty"`
	ProfileArn   string `json:"profileArn,omitempty"`
	TokenType    string `json:"tokenType,omitempty"`
}

// RefreshRequest 刷新请求结构
type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// AmazonQRefreshRequest AmazonQ token刷新请求结构
type AmazonQRefreshRequest struct {
	GrantType    string `json:"grantType"`
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	RefreshToken string `json:"refreshToken"`
}

// TokenType 认证类型枚举
type TokenType int

const (
	TokenTypeKiro    TokenType = iota // Kiro 单段式 refreshToken
	TokenTypeAmazonQ                  // AmazonQ 三段式 clientId:clientSecret:refreshToken
)
