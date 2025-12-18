package parser

import (
	"kiro/utils"
	"time"
)

// SessionManager 会话管理器
type SessionManager struct {
	sessionID string
	startTime time.Time
	endTime   *time.Time
	isActive  bool
}

// NewSessionManager 创建会话管理器
func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessionID: utils.GenerateUUID(),
		startTime: time.Now(),
		isActive:  false,
	}
}

// SetSessionID 设置会话ID
func (sm *SessionManager) SetSessionID(sessionID string) {
	sm.sessionID = sessionID
}

// StartSession 开始会话
func (sm *SessionManager) StartSession() []SSEEvent {
	sm.isActive = true
	sm.startTime = time.Now()

	return []SSEEvent{
		{
			Event: EventTypes.SESSION_START,
			Data: map[string]any{
				"type":       EventTypes.SESSION_START,
				"session_id": sm.sessionID,
				"timestamp":  sm.startTime.Format(time.RFC3339),
			},
		},
	}
}

// EndSession 结束会话
func (sm *SessionManager) EndSession() []SSEEvent {
	now := time.Now()
	sm.endTime = &now
	sm.isActive = false

	return []SSEEvent{
		{
			Event: EventTypes.SESSION_END,
			Data: map[string]any{
				"type":       EventTypes.SESSION_END,
				"session_id": sm.sessionID,
				"timestamp":  now.Format(time.RFC3339),
				"duration":   now.Sub(sm.startTime).Milliseconds(),
			},
		},
	}
}

// IsActive 检查会话是否活跃
func (sm *SessionManager) IsActive() bool {
	return sm.isActive
}

// GetSessionInfo 获取会话信息
func (sm *SessionManager) GetSessionInfo() SessionInfo {
	return SessionInfo{
		SessionID: sm.sessionID,
		StartTime: sm.startTime,
		EndTime:   sm.endTime,
	}
}

// Reset 重置会话管理器
func (sm *SessionManager) Reset() {
	sm.sessionID = utils.GenerateUUID()
	sm.startTime = time.Now()
	sm.endTime = nil
	sm.isActive = false
}
