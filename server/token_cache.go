package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"kiro/config"
	"kiro/types"
	"kiro/utils"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

/**
 * TokenCache 存储用户的 Token 缓存信息
 */
type TokenCache struct {
	AccessToken  string
	RefreshToken string
	LastRefresh  time.Time
	TokenType    types.TokenType
	// AmazonQ 专用字段
	ClientID     string
	ClientSecret string
}

var (
	// tokenMap Token 缓存映射（key: token hash）
	tokenMap = make(map[string]*TokenCache)
	// tokenMutex Token 缓存互斥锁
	tokenMutex sync.RWMutex
	// refreshGroup 用于防止并发刷新同一个 token
	refreshGroup singleflight.Group
)

/**
 * sha256Hash 计算输入文本的 SHA256 哈希值
 */
func sha256Hash(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

/**
 * ParseToken 解析 token 格式，判断是 Kiro 还是 AmazonQ
 * AmazonQ 格式: clientId:clientSecret:refreshToken
 * Kiro 格式: refreshToken (单段)
 */
func ParseToken(token string) (tokenType types.TokenType, clientID, clientSecret, refreshToken string) {
	parts := strings.SplitN(token, ":", 3)
	if len(parts) == 3 && parts[0] != "" && parts[2] != "" {
		return types.TokenTypeAmazonQ, parts[0], parts[1], parts[2]
	}
	return types.TokenTypeKiro, "", "", token
}

/**
 * RefreshAmazonQToken 刷新 AmazonQ token
 */
func RefreshAmazonQToken(clientID, clientSecret, refreshToken string) (string, error) {
	refreshReq := types.AmazonQRefreshRequest{
		GrantType:    "refresh_token",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RefreshToken: refreshToken,
	}

	reqBody, err := utils.FastMarshal(refreshReq)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", config.AmazonQTokenURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	for k, v := range config.AmazonQOIDCHeaders {
		req.Header.Set(k, v)
	}
	req.Header.Set("amz-sdk-invocation-id", utils.GenerateUUID())

	// token 刷新也走代理（用 refreshToken 的 hash 做 key）
	tokenHash := sha256Hash(refreshToken)
	resp, err := utils.DoRequestWithProxy(req, tokenHash)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("刷新失败: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	var refreshResp types.RefreshResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if err := utils.SafeUnmarshal(body, &refreshResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	return refreshResp.AccessToken, nil
}

/**
 * RefreshKiroToken 刷新 Kiro token
 */
func RefreshKiroToken(refreshToken string) (string, error) {
	refreshReq := types.RefreshRequest{
		RefreshToken: refreshToken,
	}

	reqBody, err := utils.FastMarshal(refreshReq)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", config.RefreshTokenURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %v", err)
	}

	for k, v := range config.KiroRefreshHeaders {
		req.Header.Set(k, v)
	}

	// Kiro token 刷新也走代理
	tokenHash := sha256Hash(refreshToken)
	resp, err := utils.DoRequestWithProxy(req, tokenHash)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("刷新失败: 状态码 %d, 响应: %s", resp.StatusCode, string(body))
	}

	var refreshResp types.RefreshResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	if err := utils.SafeUnmarshal(body, &refreshResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	return refreshResp.AccessToken, nil
}

/**
 * GetOrRefreshToken 获取或刷新 token，自动识别 Kiro 或 AmazonQ 格式
 * 使用 singleflight 确保同一个 token 的并发请求只刷新一次
 */
func GetOrRefreshToken(token string) (string, error) {
	tokenHash := sha256Hash(token)

	// 检查缓存
	tokenMutex.RLock()
	cached, exists := tokenMap[tokenHash]
	tokenMutex.RUnlock()

	if exists {
		return cached.AccessToken, nil
	}

	// 使用 singleflight 确保同一个 token 只刷新一次
	result, err, _ := refreshGroup.Do(tokenHash, func() (interface{}, error) {
		// 双重检查：可能在等待期间已被其他 goroutine 刷新
		tokenMutex.RLock()
		cached, exists := tokenMap[tokenHash]
		tokenMutex.RUnlock()
		if exists {
			return cached.AccessToken, nil
		}

		// 解析 token 类型
		tokenType, clientID, clientSecret, refreshToken := ParseToken(token)

		var accessToken string
		var refreshErr error

		switch tokenType {
		case types.TokenTypeAmazonQ:
			accessToken, refreshErr = RefreshAmazonQToken(clientID, clientSecret, refreshToken)
		default:
			accessToken, refreshErr = RefreshKiroToken(refreshToken)
		}

		// 获取类型名称用于日志
		typeName := "Kiro"
		if tokenType == types.TokenTypeAmazonQ {
			typeName = "AmazonQ"
		}

		if refreshErr != nil {
			utils.Error("AT 刷新失败 [%s]: %v", typeName, refreshErr)
			return "", refreshErr
		}

		utils.Info("AT 刷新成功 [%s]", typeName)

		// 缓存
		tokenMutex.Lock()
		tokenMap[tokenHash] = &TokenCache{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			LastRefresh:  time.Now(),
			TokenType:    tokenType,
			ClientID:     clientID,
			ClientSecret: clientSecret,
		}
		tokenMutex.Unlock()

		return accessToken, nil
	})

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

/**
 * InvalidateToken 使指定的 token 缓存失效
 * 当上游返回 403 表示 token 已过期时调用
 */
func InvalidateToken(token string) {
	tokenHash := sha256Hash(token)
	tokenMutex.Lock()
	delete(tokenMap, tokenHash)
	tokenMutex.Unlock()
}

/**
 * RefreshAllTokens 全局刷新器，遍历并刷新所有缓存的 token
 */
func RefreshAllTokens() {
	tokenMutex.RLock()
	count := len(tokenMap)
	tokenMutex.RUnlock()

	if count == 0 {
		return
	}

	refreshCount := 0

	tokenMutex.RLock()
	tokens := make(map[string]*TokenCache)
	for k, v := range tokenMap {
		tokens[k] = v
	}
	tokenMutex.RUnlock()

	for hash, cache := range tokens {
		var newToken string
		var err error

		switch cache.TokenType {
		case types.TokenTypeAmazonQ:
			newToken, err = RefreshAmazonQToken(cache.ClientID, cache.ClientSecret, cache.RefreshToken)
		default:
			newToken, err = RefreshKiroToken(cache.RefreshToken)
		}

		if err != nil {
			utils.Error("刷新 token 失败: %v", err)
			tokenMutex.Lock()
			delete(tokenMap, hash)
			tokenMutex.Unlock()
			continue
		}

		tokenMutex.Lock()
		if tokenMap[hash] != nil {
			tokenMap[hash].AccessToken = newToken
			tokenMap[hash].LastRefresh = time.Now()
		}
		tokenMutex.Unlock()

		refreshCount++
	}

	utils.Info("Token 刷新完成: %d/%d", refreshCount, count)
}

/**
 * StartTokenRefresher 启动定时 token 刷新器
 * 在后台 goroutine 中每 45 分钟自动刷新所有缓存的 token
 */
func StartTokenRefresher() {
	go func() {
		ticker := time.NewTicker(45 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			RefreshAllTokens()
		}
	}()

	utils.Info("Token 自动刷新器已启动 (间隔: 45分钟)")
}
