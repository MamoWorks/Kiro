package server

import (
	"kiro/types"
	"kiro/utils"
)

// StopReasonManager 管理符合Claude规范的stop_reason决策
type StopReasonManager struct {
	hasActiveToolCalls bool
	hasCompletedTools  bool
}

// NewStopReasonManager 创建stop_reason管理器
func NewStopReasonManager(anthropicReq types.AnthropicRequest) *StopReasonManager {
	return &StopReasonManager{
		hasActiveToolCalls: false,
		hasCompletedTools:  false,
	}
}

// UpdateToolCallStatus 更新工具调用状态
func (srm *StopReasonManager) UpdateToolCallStatus(hasActiveCalls, hasCompleted bool) {
	srm.hasActiveToolCalls = hasActiveCalls
	srm.hasCompletedTools = hasCompleted

	utils.Log("更新工具调用状态",
		utils.LogBool("has_active_tools", hasActiveCalls),
		utils.LogBool("has_completed_tools", hasCompleted))
}

// DetermineStopReason 根据Claude官方规范确定stop_reason
func (srm *StopReasonManager) DetermineStopReason() string {

	// 检查是否有工具调用（活跃或已完成）
	// *** 关键修复：根据Claude规范，只要消息包含tool_use块，stop_reason就应该是tool_use ***
	// 根据 Anthropic API 文档 (https://docs.anthropic.com/en/api/messages-streaming):
	//   stop_reason: "tool_use" - The model wants to use a tool
	//
	// 只要消息中包含任何 tool_use 内容块（无论是正在流式传输还是已完成），
	// stop_reason 就应该是 "tool_use"。这与工具的"生命周期状态"无关。
	//
	// 之前的BUG: 只检查 hasActiveToolCalls（正在流式传输的工具）
	// 问题场景: 工具块关闭后被移到 completedTools，导致 hasActiveToolCalls=false，
	//          错误返回 "end_turn" 而非 "tool_use"
	//
	// 修复: 检查 hasActiveToolCalls OR hasCompletedTools
	if srm.hasActiveToolCalls || srm.hasCompletedTools {
		return "tool_use"
	}

	// 规则3: 默认情况 - 自然完成响应
	return "end_turn"
}

// DetermineStopReasonFromUpstream 从上游响应中提取stop_reason
// 用于当上游已经提供了stop_reason时的情况
func (srm *StopReasonManager) DetermineStopReasonFromUpstream(upstreamStopReason string) string {
	if upstreamStopReason == "" {
		return srm.DetermineStopReason()
	}

	// 验证上游stop_reason是否符合Claude规范
	validStopReasons := map[string]bool{
		"end_turn":      true,
		"max_tokens":    true,
		"stop_sequence": true,
		"tool_use":      true,
		"pause_turn":    true,
		"refusal":       true,
	}

	if !validStopReasons[upstreamStopReason] {
		utils.Log("上游提供了无效的stop_reason，使用本地逻辑",
			utils.LogString("upstream_stop_reason", upstreamStopReason))
		return srm.DetermineStopReason()
	}

	utils.Log("使用上游stop_reason",
		utils.LogString("upstream_stop_reason", upstreamStopReason))
	return upstreamStopReason
}

// GetStopReasonDescription 获取stop_reason的描述（用于调试）
func GetStopReasonDescription(stopReason string) string {
	descriptions := map[string]string{
		"end_turn":      "Claude自然完成了响应",
		"max_tokens":    "达到了token限制",
		"stop_sequence": "遇到了自定义停止序列",
		"tool_use":      "Claude正在调用工具并期待执行",
		"pause_turn":    "服务器工具操作暂停",
		"refusal":       "Claude拒绝生成响应",
	}

	if desc, exists := descriptions[stopReason]; exists {
		return desc
	}
	return "未知的stop_reason"
}
