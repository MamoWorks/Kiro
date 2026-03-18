package utils

import (
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"time"

	"kiro/config"
	"kiro/proxy"
)

var (
	// SharedHTTPClient 共享的HTTP客户端实例（无代理时使用）
	SharedHTTPClient *http.Client
)

func init() {
	skipTLS := shouldSkipTLSVerify()
	if skipTLS {
		os.Stderr.WriteString("[WARNING] TLS证书验证已禁用 - 仅适用于开发/调试环境\n")
	}

	SharedHTTPClient = &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: config.HTTPClientKeepAlive,
				DualStack: true,
			}).DialContext,
			TLSHandshakeTimeout: config.HTTPClientTLSHandshakeTimeout,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: skipTLS,
				MinVersion:         tls.VersionTLS12,
				MaxVersion:         tls.VersionTLS13,
				CipherSuites: []uint16{
					tls.TLS_AES_256_GCM_SHA384,
					tls.TLS_CHACHA20_POLY1305_SHA256,
					tls.TLS_AES_128_GCM_SHA256,
				},
			},
			ForceAttemptHTTP2:  false,
			DisableCompression: false,
		},
	}
}

func shouldSkipTLSVerify() bool {
	return os.Getenv("GIN_MODE") == "debug"
}

// DoRequest 执行HTTP请求（使用默认直连客户端）
func DoRequest(req *http.Request) (*http.Response, error) {
	return SharedHTTPClient.Do(req)
}

// DoRequestWithProxy 执行HTTP请求，通过代理管理器按 key 路由
// key 通常是 token hash，用于绑定代理
// 如果代理未启用或获取失败，回退到直连
func DoRequestWithProxy(req *http.Request, key string) (*http.Response, error) {
	if !proxy.Enabled() || key == "" {
		return SharedHTTPClient.Do(req)
	}

	client, proxyURL, err := proxy.GetClient(key)
	if err != nil {
		// 获取代理失败，回退直连
		Error("获取代理失败: %v，回退直连", err)
		return SharedHTTPClient.Do(req)
	}
	if client == nil {
		return SharedHTTPClient.Do(req)
	}

	resp, err := client.Do(req)
	if err != nil && proxy.IsProxyError(err) {
		// 代理自身故障，报告并重试直连
		proxy.ReportError(key, proxyURL)
		Error("代理故障 %s: %v，回退直连", proxyURL, err)
		return SharedHTTPClient.Do(req)
	}

	return resp, err
}
