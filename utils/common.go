package utils

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
)

// ==================== UUID ====================

// GenerateUUID generates a simple UUID v4
func GenerateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant bits
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// ==================== Math ====================

// IntMin 返回两个整数的最小值
func IntMin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IntMax 返回两个整数的最大值
func IntMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ==================== HTTP ====================

// ReadHTTPResponse 通用的HTTP响应体读取函数
func ReadHTTPResponse(body io.Reader) ([]byte, error) {
	buffer := bytes.NewBuffer(nil)
	buf := make([]byte, 1024)

	for {
		n, err := body.Read(buf)
		if n > 0 {
			buffer.Write(buf[:n])
		}
		if err != nil {
			result := buffer.Bytes()
			if result == nil {
				result = []byte{}
			}
			if err == io.EOF {
				return result, nil
			}
			return result, err
		}
	}
}

// ==================== JSON ====================

// FastMarshal 高性能JSON序列化
func FastMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// FastUnmarshal 高性能JSON反序列化
func FastUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// SafeMarshal 安全JSON序列化
func SafeMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// SafeUnmarshal 安全JSON反序列化
func SafeUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// MarshalIndent 带缩进的JSON序列化
func MarshalIndent(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}
