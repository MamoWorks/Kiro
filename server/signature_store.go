package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"kiro/types"
	"kiro/utils"

	_ "modernc.org/sqlite"
)

// signatureStore 已发出签名的持久化注册表（SQLite）
type signatureStore struct {
	db   *sql.DB
	mu   sync.RWMutex // 保护并发写入
}

var sigStore *signatureStore

// InitSignatureStore 初始化签名存储（SQLite）
func InitSignatureStore() {
	// 数据目录
	dir := "data"
	os.MkdirAll(dir, 0755)
	dbPath := filepath.Join(dir, "signatures.db")

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		utils.Error("签名存储初始化失败: %v", err)
		// 回退到内存模式
		db, _ = sql.Open("sqlite", ":memory:")
	}

	// 连接池设置
	db.SetMaxOpenConns(1) // SQLite 单写
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(0) // 不过期

	// 建表
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS signatures (
			hash TEXT PRIMARY KEY,
			created_at INTEGER NOT NULL
		)
	`)
	if err != nil {
		utils.Error("创建签名表失败: %v", err)
	}

	// 索引加速过期清理
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_sig_created ON signatures(created_at)`)

	sigStore = &signatureStore{db: db}

	// 启动时清理过期数据
	sigStore.cleanup()

	utils.Info("签名存储已初始化 (%s)", dbPath)
}

// RegisterSignature 注册一个已发出的签名
func RegisterSignature(sig string) {
	if sig == "" || sigStore == nil {
		return
	}
	hash := hashSignature(sig)
	now := time.Now().Unix()

	sigStore.mu.Lock()
	defer sigStore.mu.Unlock()

	sigStore.db.Exec(
		`INSERT OR REPLACE INTO signatures (hash, created_at) VALUES (?, ?)`,
		hash, now,
	)
}

// IsValidSignature 检查签名是否由本服务生成
func IsValidSignature(sig string) bool {
	if sig == "" || sigStore == nil {
		return false
	}
	hash := hashSignature(sig)

	sigStore.mu.RLock()
	defer sigStore.mu.RUnlock()

	var count int
	err := sigStore.db.QueryRow(
		`SELECT COUNT(1) FROM signatures WHERE hash = ?`, hash,
	).Scan(&count)

	return err == nil && count > 0
}

// validateThinkingSignatures 校验请求中历史消息的 thinking 签名
func validateThinkingSignatures(req types.AnthropicRequest) error {
	for _, msg := range req.Messages {
		if msg.Role != "assistant" {
			continue
		}

		contentArr, ok := msg.Content.([]any)
		if !ok {
			continue
		}

		for _, block := range contentArr {
			blockMap, ok := block.(map[string]any)
			if !ok {
				continue
			}

			blockType, _ := blockMap["type"].(string)
			if blockType != "thinking" {
				continue
			}

			signature, _ := blockMap["signature"].(string)
			if signature == "" {
				continue
			}

			if !IsValidSignature(signature) {
				return fmt.Errorf("Thinking signature verification failed: the signature on a thinking block in messages[].content is invalid. Please ensure you are sending the unmodified `signature` field from the original assistant response.")
			}
		}
	}
	return nil
}

// StartSignatureCleanup 定时清理过期签名（保留 7 天）
func StartSignatureCleanup() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if sigStore != nil {
				sigStore.cleanup()
			}
		}
	}()
}

func (s *signatureStore) cleanup() {
	cutoff := time.Now().Add(-7 * 24 * time.Hour).Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(`DELETE FROM signatures WHERE created_at < ?`, cutoff)
	if err == nil {
		if n, _ := result.RowsAffected(); n > 0 {
			utils.Info("清理过期签名: %d 条", n)
			s.db.Exec(`VACUUM`) // 回收空间
		}
	}
}

func hashSignature(sig string) string {
	h := sha256.Sum256([]byte(sig))
	return hex.EncodeToString(h[:])
}
