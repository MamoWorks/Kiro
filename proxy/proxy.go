package proxy

import (
	"bufio"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

// dataDir 数据文件根目录
var dataDir = "data"

// ConfigBinding 持久化绑定条目
type ConfigBinding struct {
	Key   string `json:"key"`
	Proxy string `json:"proxy"`
}

// ProxyManager 代理分配管理器
type ProxyManager struct {
	mu             sync.RWMutex
	enabled        bool
	proxies        []string
	configBindings []ConfigBinding
	keyProxyMap    map[string]string
	keyLastSeen    map[string]time.Time
	errorProxies   map[string]struct{}
	clients        map[string]*http.Client
	skipTLS        bool
	// 文件修改时间，用于热重载检测
	proxiesModTime time.Time
	configModTime  time.Time
}

// 全局单例
var manager = &ProxyManager{
	keyProxyMap:  make(map[string]string),
	keyLastSeen:  make(map[string]time.Time),
	errorProxies: make(map[string]struct{}),
	clients:      make(map[string]*http.Client),
}

// keyTTL key 绑定过期时间
const keyTTL = 60 * time.Minute

// Init 启动时初始化代理管理器
func Init(skipTLS bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.skipTLS = skipTLS

	// 创建 data 目录
	os.MkdirAll(dataDir, 0755)

	// 加载 proxies.txt
	proxiesPath := filepath.Join(dataDir, "proxies.txt")
	proxies, err := readLines(proxiesPath)
	if err != nil || len(proxies) == 0 {
		manager.enabled = false
		fmt.Fprintf(os.Stderr, "[Proxy] 已禁用: %s 中没有有效代理\n", proxiesPath)
		return
	}
	manager.proxies = proxies
	manager.enabled = true
	fmt.Fprintf(os.Stderr, "[Proxy] 已加载 %d 个代理\n", len(proxies))

	// 加载 error_proxies.txt
	errorPath := filepath.Join(dataDir, "error_proxies.txt")
	errorLines, _ := readLines(errorPath)
	for _, line := range errorLines {
		manager.errorProxies[line] = struct{}{}
	}

	// 构建代理集合
	proxySet := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		proxySet[p] = struct{}{}
	}

	// 加载 proxies_config.json
	configPath := filepath.Join(dataDir, "proxies_config.json")
	raw, err := os.ReadFile(configPath)
	if err == nil {
		var bindings []ConfigBinding
		if json.Unmarshal(raw, &bindings) == nil {
			var cleaned []ConfigBinding
			for _, b := range bindings {
				if _, ok := proxySet[b.Proxy]; ok {
					cleaned = append(cleaned, b)
				}
			}
			manager.configBindings = cleaned
			writeConfigBindings(cleaned)
		}
	}

	// 记录文件修改时间
	if info, err := os.Stat(proxiesPath); err == nil {
		manager.proxiesModTime = info.ModTime()
	}
	if info, err := os.Stat(configPath); err == nil {
		manager.configModTime = info.ModTime()
	}
}

// Enabled 返回代理是否启用
func Enabled() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.enabled
}

// GetClient 为指定 key (token hash) 获取代理 HTTP Client
// 返回 client, proxyURL, error
func GetClient(key string) (*http.Client, string, error) {
	if !manager.enabled {
		return nil, "", nil // 返回 nil client 表示使用默认
	}

	// 快速路径：读锁查缓存
	manager.mu.RLock()
	if p, ok := manager.keyProxyMap[key]; ok {
		client := manager.clients[p]
		manager.mu.RUnlock()
		go func() {
			manager.mu.Lock()
			manager.keyLastSeen[key] = time.Now()
			manager.mu.Unlock()
		}()
		return client, p, nil
	}
	manager.mu.RUnlock()

	// 慢路径：写锁分配
	manager.mu.Lock()
	defer manager.mu.Unlock()

	// double-check
	if p, ok := manager.keyProxyMap[key]; ok {
		manager.keyLastSeen[key] = time.Now()
		return manager.clients[p], p, nil
	}

	// 查 configBindings
	for _, b := range manager.configBindings {
		if b.Key == key {
			if _, isErr := manager.errorProxies[b.Proxy]; !isErr {
				manager.keyProxyMap[key] = b.Proxy
				manager.keyLastSeen[key] = time.Now()
				manager.ensureClient(b.Proxy)
				return manager.clients[b.Proxy], b.Proxy, nil
			}
		}
	}

	// 收集已分配的代理
	usedProxies := make(map[string]struct{}, len(manager.keyProxyMap))
	for _, p := range manager.keyProxyMap {
		usedProxies[p] = struct{}{}
	}

	// 优先分配未使用且非故障的代理
	var available []string
	for _, p := range manager.proxies {
		if _, used := usedProxies[p]; used {
			continue
		}
		if _, isErr := manager.errorProxies[p]; isErr {
			continue
		}
		available = append(available, p)
	}

	var chosen string
	if len(available) > 0 {
		chosen = available[rand.Intn(len(available))]
	} else {
		// 复用已分配的非故障代理
		var reusable []string
		for _, p := range manager.proxies {
			if _, isErr := manager.errorProxies[p]; !isErr {
				reusable = append(reusable, p)
			}
		}
		if len(reusable) == 0 {
			return nil, "", fmt.Errorf("所有代理均故障，无可用代理")
		}
		chosen = reusable[rand.Intn(len(reusable))]
	}

	manager.keyProxyMap[key] = chosen
	manager.keyLastSeen[key] = time.Now()
	manager.ensureClient(chosen)

	// 持久化绑定
	manager.configBindings = append(manager.configBindings, ConfigBinding{Key: key, Proxy: chosen})
	go writeConfigBindings(manager.configBindings)

	fmt.Fprintf(os.Stderr, "[Proxy] 分配 %s → %s\n", key[:8], chosen)
	return manager.clients[chosen], chosen, nil
}

// ReportError 报告代理故障
func ReportError(key string, proxyURL string) {
	if !manager.enabled || proxyURL == "" {
		return
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if _, exists := manager.errorProxies[proxyURL]; !exists {
		manager.errorProxies[proxyURL] = struct{}{}
		go appendLine(filepath.Join(dataDir, "error_proxies.txt"), proxyURL)
		fmt.Fprintf(os.Stderr, "[Proxy] 标记故障: %s\n", proxyURL)
	}

	if current, ok := manager.keyProxyMap[key]; ok && current == proxyURL {
		delete(manager.keyProxyMap, key)
	}

	var cleaned []ConfigBinding
	for _, b := range manager.configBindings {
		if b.Key == key && b.Proxy == proxyURL {
			continue
		}
		cleaned = append(cleaned, b)
	}
	manager.configBindings = cleaned
	go writeConfigBindings(cleaned)
}

// IsProxyError 判断是否为代理自身的连接错误
func IsProxyError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "socks") ||
		strings.Contains(errMsg, "proxyconnect") ||
		strings.Contains(errMsg, "proxy authentication")
}

// StartCleanupTicker 启动定时清理和热重载
func StartCleanupTicker() {
	go func() {
		reloadTicker := time.NewTicker(30 * time.Second)
		cleanupTicker := time.NewTicker(5 * time.Minute)
		for {
			select {
			case <-reloadTicker.C:
				checkAndReload()
			case <-cleanupTicker.C:
				cleanupBindings()
			}
		}
	}()
}

// --- 内部方法 ---

func (m *ProxyManager) ensureClient(proxyURL string) {
	if _, exists := m.clients[proxyURL]; !exists {
		m.clients[proxyURL] = m.createClientForProxy(proxyURL)
	}
}

func (m *ProxyManager) createClientForProxy(proxyURL string) *http.Client {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return createDirectClient(m.skipTLS)
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: m.skipTLS,
		MinVersion:         tls.VersionTLS12,
	}

	switch u.Scheme {
	case "socks5", "socks5h":
		return m.createSocks5Client(u, tlsConfig)
	case "http", "https":
		return m.createHTTPProxyClient(u, tlsConfig)
	default:
		return createDirectClient(m.skipTLS)
	}
}

func (m *ProxyManager) createSocks5Client(u *url.URL, tlsConfig *tls.Config) *http.Client {
	var auth *proxy.Auth
	if u.User != nil {
		password, _ := u.User.Password()
		auth = &proxy.Auth{
			User:     u.User.Username(),
			Password: password,
		}
	}

	dialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return createDirectClient(m.skipTLS)
	}

	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return createDirectClient(m.skipTLS)
	}

	transport := &http.Transport{
		DialContext:         contextDialer.DialContext,
		TLSClientConfig:    tlsConfig,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{Transport: transport}
}

func (m *ProxyManager) createHTTPProxyClient(u *url.URL, tlsConfig *tls.Config) *http.Client {
	transport := &http.Transport{
		Proxy:               http.ProxyURL(u),
		TLSClientConfig:    tlsConfig,
		MaxIdleConns:        256,
		MaxIdleConnsPerHost: 64,
		IdleConnTimeout:     90 * time.Second,
	}

	return &http.Client{Transport: transport}
}

func createDirectClient(skipTLS bool) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: skipTLS,
				MinVersion:         tls.VersionTLS12,
			},
			MaxIdleConns:        256,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

func checkAndReload() {
	proxiesPath := filepath.Join(dataDir, "proxies.txt")
	configPath := filepath.Join(dataDir, "proxies_config.json")

	proxiesChanged := false
	configChanged := false

	if info, err := os.Stat(proxiesPath); err == nil {
		manager.mu.RLock()
		if info.ModTime().After(manager.proxiesModTime) {
			proxiesChanged = true
		}
		manager.mu.RUnlock()
	}

	if info, err := os.Stat(configPath); err == nil {
		manager.mu.RLock()
		if info.ModTime().After(manager.configModTime) {
			configChanged = true
		}
		manager.mu.RUnlock()
	}

	if proxiesChanged {
		reloadProxies(proxiesPath)
	}
	if configChanged && !proxiesChanged {
		reloadConfig(configPath)
	}
}

func reloadProxies(proxiesPath string) {
	proxies, err := readLines(proxiesPath)
	if err != nil {
		return
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	oldCount := len(manager.proxies)
	manager.proxies = proxies
	manager.enabled = len(proxies) > 0

	if info, err := os.Stat(proxiesPath); err == nil {
		manager.proxiesModTime = info.ModTime()
	}

	proxySet := make(map[string]struct{}, len(proxies))
	for _, p := range proxies {
		proxySet[p] = struct{}{}
	}

	for key, p := range manager.keyProxyMap {
		if _, ok := proxySet[p]; !ok {
			delete(manager.keyProxyMap, key)
		}
	}

	fmt.Fprintf(os.Stderr, "[Proxy] 热重载: 代理 %d→%d\n", oldCount, len(proxies))
}

func reloadConfig(configPath string) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return
	}

	var bindings []ConfigBinding
	if json.Unmarshal(raw, &bindings) != nil {
		return
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	proxySet := make(map[string]struct{}, len(manager.proxies))
	for _, p := range manager.proxies {
		proxySet[p] = struct{}{}
	}

	var cleaned []ConfigBinding
	for _, b := range bindings {
		if _, ok := proxySet[b.Proxy]; ok {
			cleaned = append(cleaned, b)
		}
	}
	manager.configBindings = cleaned

	if info, err := os.Stat(configPath); err == nil {
		manager.configModTime = info.ModTime()
	}
}

func cleanupBindings() {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	now := time.Now()
	expired := 0
	for key, lastSeen := range manager.keyLastSeen {
		if now.Sub(lastSeen) > keyTTL {
			delete(manager.keyProxyMap, key)
			delete(manager.keyLastSeen, key)
			expired++
		}
	}

	if expired > 0 {
		var bindings []ConfigBinding
		for key, p := range manager.keyProxyMap {
			bindings = append(bindings, ConfigBinding{Key: key, Proxy: p})
		}
		manager.configBindings = bindings
		writeConfigBindings(bindings)
		fmt.Fprintf(os.Stderr, "[Proxy] 清理: 过期 %d 个绑定\n", expired)
	}
}

// --- 文件 I/O ---

func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, normalizeProxy(line))
	}
	return lines, scanner.Err()
}

func normalizeProxy(raw string) string {
	if strings.Contains(raw, "://") {
		return raw
	}
	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 4:
		return fmt.Sprintf("http://%s:%s@%s:%s", parts[2], parts[3], parts[0], parts[1])
	case 2:
		return fmt.Sprintf("http://%s:%s", parts[0], parts[1])
	default:
		return raw
	}
}

func writeConfigBindings(bindings []ConfigBinding) {
	if bindings == nil {
		bindings = []ConfigBinding{}
	}
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dataDir, "proxies_config.json"), data, 0644)
}

func appendLine(path string, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}
