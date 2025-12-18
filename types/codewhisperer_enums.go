package types

// MessageStatus 消息状态枚举
type MessageStatus string

const (
	MessageStatusCompleted  MessageStatus = "COMPLETED"
	MessageStatusInProgress MessageStatus = "IN_PROGRESS"
	MessageStatusError      MessageStatus = "ERROR"
)

// UserIntent 用户意图枚举
type UserIntent string

const (
	UserIntentExplainCodeSelection     UserIntent = "EXPLAIN_CODE_SELECTION"
	UserIntentSuggestAlternateImpl     UserIntent = "SUGGEST_ALTERNATE_IMPLEMENTATION"
	UserIntentApplyCommonBestPractices UserIntent = "APPLY_COMMON_BEST_PRACTICES"
	UserIntentImproveCode              UserIntent = "IMPROVE_CODE"
	UserIntentShowExamples             UserIntent = "SHOW_EXAMPLES"
	UserIntentCiteSources              UserIntent = "CITE_SOURCES"
	UserIntentExplainLineByLine        UserIntent = "EXPLAIN_LINE_BY_LINE"
)

// ContentType 内容类型枚举
type ContentType string

const (
	ContentTypeMarkdown ContentType = "text/markdown"
	ContentTypePlain    ContentType = "text/plain"
	ContentTypeJSON     ContentType = "application/json"
)
