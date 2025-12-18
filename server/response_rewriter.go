package server

import (
	"bytes"
	"net/http"
	"strings"

	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// ResponseRewriter 响应重写器，参考 CLIProxyAPIPlus 的实现
// 用于拦截和处理响应体，支持流式和非流式响应
type ResponseRewriter struct {
	gin.ResponseWriter
	body          *bytes.Buffer
	originalModel string
	isStreaming   bool
}

// NewResponseRewriter 创建响应重写器
func NewResponseRewriter(w gin.ResponseWriter, originalModel string) *ResponseRewriter {
	return &ResponseRewriter{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		originalModel:  originalModel,
	}
}

const maxBufferedResponseBytes = 2 * 1024 * 1024 // 2MB 安全上限

// looksLikeSSEChunk 检测数据是否看起来像 SSE 块
func looksLikeSSEChunk(data []byte) bool {
	return bytes.Contains(data, []byte("data:")) ||
		bytes.Contains(data, []byte("event:")) ||
		bytes.Contains(data, []byte("message_start")) ||
		bytes.Contains(data, []byte("message_delta")) ||
		bytes.Contains(data, []byte("content_block_start")) ||
		bytes.Contains(data, []byte("content_block_delta")) ||
		bytes.Contains(data, []byte("content_block_stop")) ||
		bytes.Contains(data, []byte("\n\n"))
}

// enableStreaming 启用流式模式
func (rw *ResponseRewriter) enableStreaming(reason string) error {
	if rw.isStreaming {
		return nil
	}
	rw.isStreaming = true

	// 刷新之前缓冲的数据
	if rw.body != nil && rw.body.Len() > 0 {
		buf := rw.body.Bytes()
		toFlush := make([]byte, len(buf))
		copy(toFlush, buf)
		rw.body.Reset()

		if _, err := rw.ResponseWriter.Write(toFlush); err != nil {
			return err
		}
		if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	utils.Log("响应重写器: 切换到流式模式", utils.LogString("reason", reason))
	return nil
}

// Write 拦截写入操作
func (rw *ResponseRewriter) Write(data []byte) (int, error) {
	// 首次写入时检测流式
	if !rw.isStreaming && rw.body.Len() == 0 {
		contentType := rw.Header().Get("Content-Type")
		rw.isStreaming = strings.Contains(contentType, "text/event-stream") ||
			strings.Contains(contentType, "stream")
	}

	if !rw.isStreaming {
		// 内容检测：即使 Content-Type 缺失/错误，也检测 SSE 特征
		if looksLikeSSEChunk(data) {
			if err := rw.enableStreaming("sse heuristic"); err != nil {
				return 0, err
			}
		} else if rw.body.Len()+len(data) > maxBufferedResponseBytes {
			// 缓冲区超限，切换到流式
			utils.Log("响应重写器: 缓冲区超过限制，切换到流式",
				utils.LogInt("buffer_size", rw.body.Len()+len(data)))
			if err := rw.enableStreaming("buffer limit"); err != nil {
				return 0, err
			}
		}
	}

	if rw.isStreaming {
		n, err := rw.ResponseWriter.Write(data)
		if err == nil {
			if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		return n, err
	}
	return rw.body.Write(data)
}

// Flush 刷新缓冲的响应
func (rw *ResponseRewriter) Flush() {
	if rw.isStreaming {
		if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	if rw.body.Len() > 0 {
		if _, err := rw.ResponseWriter.Write(rw.body.Bytes()); err != nil {
			utils.Log("响应重写器: 写入缓冲响应失败", utils.LogErr(err))
		}
	}
}

// IsStreaming 检查是否为流式模式
func (rw *ResponseRewriter) IsStreaming() bool {
	return rw.isStreaming
}

// GetBufferedBody 获取缓冲的响应体
func (rw *ResponseRewriter) GetBufferedBody() []byte {
	return rw.body.Bytes()
}
