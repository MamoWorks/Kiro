package utils

import (
	"fmt"
	"os"
	"time"
)

// LogLevel 日志级别
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelError
)

var (
	// 当前日志级别，release 模式只输出 ERROR
	currentLevel = func() LogLevel {
		if os.Getenv("GIN_MODE") == "release" {
			return LevelError
		}
		// 开发模式下，检查是否要启用 DEBUG
		if os.Getenv("DEBUG") == "1" || os.Getenv("DEBUG") == "true" {
			return LevelDebug
		}
		return LevelInfo
	}()
)

// SetLogLevel 设置日志级别
func SetLogLevel(level LogLevel) {
	currentLevel = level
}

// timestamp 获取时间戳
func timestamp() string {
	return time.Now().Format("15:04:05")
}

// Debug 调试日志（仅在 DEBUG 模式下输出）
func Debug(format string, args ...any) {
	if currentLevel <= LevelDebug {
		fmt.Printf("[%s] [DEBUG] %s\n", timestamp(), fmt.Sprintf(format, args...))
	}
}

// Info 信息日志
func Info(format string, args ...any) {
	if currentLevel <= LevelInfo {
		fmt.Printf("[%s] %s\n", timestamp(), fmt.Sprintf(format, args...))
	}
}

// Error 错误日志（始终输出）
func Error(format string, args ...any) {
	fmt.Printf("[%s] [ERROR] %s\n", timestamp(), fmt.Sprintf(format, args...))
}

// === 兼容旧 API（逐步废弃） ===

// LogField 日志字段（保留兼容性）
type LogField struct {
	Key   string
	Value any
}

// Log 兼容旧 API，映射到 Debug
func Log(msg string, fields ...LogField) {
	if currentLevel > LevelDebug {
		return
	}
	if len(fields) == 0 {
		Debug("%s", msg)
		return
	}
	// 简化输出：只输出消息
	Debug("%s", msg)
}

// LogAlways 兼容旧 API，映射到 Info
func LogAlways(msg string, fields ...LogField) {
	if len(fields) == 0 {
		Info("%s", msg)
		return
	}
	Info("%s", msg)
}

// 字段构造函数（保留兼容性，但不再使用）
func LogString(key, val string) LogField { return LogField{Key: key, Value: val} }
func LogInt(key string, val int) LogField { return LogField{Key: key, Value: val} }
func LogBool(key string, val bool) LogField { return LogField{Key: key, Value: val} }
func LogAny(key string, val any) LogField  { return LogField{Key: key, Value: val} }

func LogErr(err error) LogField {
	if err == nil {
		return LogField{Key: "error", Value: nil}
	}
	return LogField{Key: "error", Value: err.Error()}
}

