package server

import (
	"fmt"
	"net/http"

	"kiro/types"
	"kiro/utils"

	"github.com/gin-gonic/gin"
)

// handleCountTokens 本地实现token计数接口
// 设计原则：
// - KISS: 简单高效的估算算法，避免引入复杂的tokenizer库
// - 向后兼容: 支持所有Claude模型和消息格式
// - 性能优先: 本地计算，响应时间<5ms
func handleCountTokens(c *gin.Context) {
	var req types.CountTokensRequest

	// 解析请求体
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Log("token计数请求解析失败",
			addReqFields(c,
				utils.LogErr(err),
			)...)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid request body: %v", err),
			},
		})
		return
	}

	// 验证模型参数（支持所有Claude模型）
	if !utils.IsValidClaudeModel(req.Model) {
		utils.Log("无效的模型参数",
			addReqFields(c,
				utils.LogString("model", req.Model),
			)...)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"type":    "invalid_request_error",
				"message": fmt.Sprintf("Invalid model: %s", req.Model),
			},
		})
		return
	}

	// 创建token估算器
	estimator := utils.NewTokenEstimator()

	// 计算token数量
	tokenCount := estimator.EstimateTokens(&req)

	// 返回符合官方API格式的响应
	c.JSON(http.StatusOK, types.CountTokensResponse{
		InputTokens: tokenCount,
	})
}
