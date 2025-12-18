package types

import (
	"fmt"
)

// Usage 表示API使用统计的通用结构
type Usage struct {
	// Anthropic格式的字段
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

// ModelNotFoundError 模型未找到错误结构
type ModelNotFoundError struct {
	Error ModelNotFoundErrorDetail `json:"error"`
}

// ModelNotFoundErrorDetail 模型未找到错误详细信息
type ModelNotFoundErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// NewModelNotFoundError 创建模型未找到错误
func NewModelNotFoundError(model, requestId string) *ModelNotFoundError {
	return &ModelNotFoundError{
		Error: ModelNotFoundErrorDetail{
			Code: "model_not_found",
			Message: fmt.Sprintf("分组 default 下模型 %s 无可用渠道（distributor） (request id: %s)",
				model, requestId),
			Type: "new_api_error",
		},
	}
}

// ModelNotFoundErrorType 模型未找到错误的类型包装器，用于在错误处理中识别
type ModelNotFoundErrorType struct {
	ErrorData *ModelNotFoundError
}

// Error 实现 error 接口
func (e *ModelNotFoundErrorType) Error() string {
	return fmt.Sprintf("model not found: %s", e.ErrorData.Error.Message)
}

// NewModelNotFoundErrorType 创建模型未找到错误类型
func NewModelNotFoundErrorType(model, requestId string) *ModelNotFoundErrorType {
	return &ModelNotFoundErrorType{
		ErrorData: NewModelNotFoundError(model, requestId),
	}
}
