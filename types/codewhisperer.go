package types

import (
	"encoding/json"
	"fmt"
)

// CodeWhispererRequest 表示 CodeWhisperer API 的请求结构
// 格式参考 CLIProxyAPIPlus 项目
type CodeWhispererRequest struct {
	ConversationState struct {
		ChatTriggerType string `json:"chatTriggerType"`
		ConversationId  string `json:"conversationId"`
		CurrentMessage  struct {
			UserInputMessage struct {
				Content                 string               `json:"content"`
				ModelId                 string               `json:"modelId"`
				Origin                  string               `json:"origin"`
				Images                  []CodeWhispererImage `json:"images,omitempty"`
				UserInputMessageContext struct {
					Tools       []CodeWhispererTool `json:"tools,omitempty"`
					ToolResults []ToolResult        `json:"toolResults,omitempty"`
				} `json:"userInputMessageContext,omitempty"`
			} `json:"userInputMessage"`
		} `json:"currentMessage"`
		History []any `json:"history,omitempty"`
	} `json:"conversationState"`
	ProfileArn      string           `json:"profileArn,omitempty"`
	InferenceConfig *InferenceConfig `json:"inferenceConfig,omitempty"`
}

// InferenceConfig 推理配置参数
type InferenceConfig struct {
	MaxTokens   int     `json:"maxTokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// CodeWhispererImage 表示 CodeWhisperer API 的图片结构
type CodeWhispererImage struct {
	Format string `json:"format"` // "jpeg", "png", "gif", "webp"
	Source struct {
		Bytes string `json:"bytes"` // base64编码的图片数据
	} `json:"source"`
}

// CodeWhispererEvent 表示 CodeWhisperer 的事件响应
type CodeWhispererEvent struct {
	ContentType string `json:"content-type"`
	MessageType string `json:"message-type"`
	Content     string `json:"content"`
	EventType   string `json:"event-type"`
}

// ToolSpecification 表示工具规范的结构
type ToolSpecification struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

// CodeWhispererTool 表示 CodeWhisperer API 的工具结构
type CodeWhispererTool struct {
	ToolSpecification ToolSpecification `json:"toolSpecification"`
}

// HistoryUserMessage 表示历史记录中的用户消息
type HistoryUserMessage struct {
	UserInputMessage struct {
		Content                 string               `json:"content"`
		ModelId                 string               `json:"modelId"`
		Origin                  string               `json:"origin"`
		Images                  []CodeWhispererImage `json:"images,omitempty"`
		UserInputMessageContext struct {
			ToolResults []ToolResult        `json:"toolResults,omitempty"`
			Tools       []CodeWhispererTool `json:"tools,omitempty"`
		} `json:"userInputMessageContext"`
	} `json:"userInputMessage"`
}

// HistoryAssistantMessage 表示历史记录中的助手消息
type HistoryAssistantMessage struct {
	AssistantResponseMessage struct {
		Content  string         `json:"content"`
		ToolUses []ToolUseEntry `json:"toolUses"`
	} `json:"assistantResponseMessage"`
}

// ToolUseEntry 表示工具使用条目
type ToolUseEntry struct {
	ToolUseId string         `json:"toolUseId"`
	Name      string         `json:"name"`
	Input     map[string]any `json:"input"`
}

// InputSchema 表示工具输入模式的结构
type InputSchema struct {
	Json map[string]any `json:"json"`
}

// ToolResult 表示工具执行结果的结构
type ToolResult struct {
	ToolUseId string           `json:"toolUseId"`
	Content   []map[string]any `json:"content"` // 根据req.json，content是数组格式
	Status    string           `json:"status"`  // "success" 或 "error"
	IsError   bool             `json:"isError,omitempty"`
}

// ========== AWS CodeWhisperer assistantResponseEvent 完整结构定义 ==========

// ContentSpan 内容范围
type ContentSpan struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// SupplementaryWebLink 补充网络链接
type SupplementaryWebLink struct {
	URL     string   `json:"url"`
	Title   *string  `json:"title,omitempty"`
	Snippet *string  `json:"snippet,omitempty"`
	Score   *float64 `json:"score,omitempty"`
}

// MostRelevantMissedAlternative 最相关的错过替代方案
type MostRelevantMissedAlternative struct {
	URL         string  `json:"url"`
	LicenseName *string `json:"licenseName,omitempty"`
	Repository  *string `json:"repository,omitempty"`
}

// Reference 引用
type Reference struct {
	LicenseName                   *string                        `json:"licenseName,omitempty"`
	Repository                    *string                        `json:"repository,omitempty"`
	URL                           *string                        `json:"url,omitempty"`
	Information                   *string                        `json:"information,omitempty"`
	RecommendationContentSpan     *ContentSpan                   `json:"recommendationContentSpan,omitempty"`
	MostRelevantMissedAlternative *MostRelevantMissedAlternative `json:"mostRelevantMissedAlternative,omitempty"`
}

// FollowupPrompt 后续提示
type FollowupPrompt struct {
	Content    string      `json:"content"`
	UserIntent *UserIntent `json:"userIntent,omitempty"`
}

// ProgrammingLanguage 编程语言
type ProgrammingLanguage struct {
	LanguageName string `json:"languageName"`
}

// Customization 自定义模型
type Customization struct {
	ARN  string  `json:"arn"`
	Name *string `json:"name,omitempty"`
}

// CodeQuery 代码查询
type CodeQuery struct {
	CodeQueryID         string               `json:"codeQueryId"`
	ProgrammingLanguage *ProgrammingLanguage `json:"programmingLanguage,omitempty"`
	UserInputMessageID  *string              `json:"userInputMessageId,omitempty"`
}

// AssistantResponseEvent AWS CodeWhisperer 助手响应事件完整结构
type AssistantResponseEvent struct {
	// 核心字段
	ConversationID string        `json:"conversationId"`
	MessageID      string        `json:"messageId"`
	Content        string        `json:"content"`
	ContentType    ContentType   `json:"contentType,omitempty"`
	MessageStatus  MessageStatus `json:"messageStatus,omitempty"`

	// 引用和链接字段
	SupplementaryWebLinks []SupplementaryWebLink `json:"supplementaryWebLinks,omitempty"`
	References            []Reference            `json:"references,omitempty"`
	CodeReference         []Reference            `json:"codeReference,omitempty"`

	// 交互字段
	FollowupPrompt *FollowupPrompt `json:"followupPrompt,omitempty"`

	// 上下文字段
	ProgrammingLanguage *ProgrammingLanguage `json:"programmingLanguage,omitempty"`
	Customizations      []Customization      `json:"customizations,omitempty"`
	UserIntent          *UserIntent          `json:"userIntent,omitempty"`
	CodeQuery           *CodeQuery           `json:"codeQuery,omitempty"`
}

// FromDict 从map[string]any创建AssistantResponseEvent
func (are *AssistantResponseEvent) FromDict(data map[string]any) error {
	// 对于流式响应，这些字段可能为空，只需要有content
	if convID, ok := data["conversationId"].(string); ok {
		are.ConversationID = convID
	}

	if msgID, ok := data["messageId"].(string); ok {
		are.MessageID = msgID
	}

	if content, ok := data["content"].(string); ok {
		are.Content = content
	}

	// 可选字段
	if contentType, ok := data["contentType"].(string); ok {
		are.ContentType = ContentType(contentType)
	} else {
		are.ContentType = ContentTypeMarkdown // 默认值
	}

	if msgStatus, ok := data["messageStatus"].(string); ok {
		are.MessageStatus = MessageStatus(msgStatus)
	} else {
		are.MessageStatus = MessageStatusCompleted // 默认值
	}

	// 处理补充网页链接
	if linksData, ok := data["supplementaryWebLinks"].([]any); ok {
		for _, linkData := range linksData {
			if linkMap, ok := linkData.(map[string]any); ok {
				link := SupplementaryWebLink{}
				if url, ok := linkMap["url"].(string); ok {
					link.URL = url
				}
				if title, ok := linkMap["title"].(string); ok {
					link.Title = &title
				}
				if snippet, ok := linkMap["snippet"].(string); ok {
					link.Snippet = &snippet
				}
				if score, ok := linkMap["score"].(float64); ok {
					link.Score = &score
				}
				are.SupplementaryWebLinks = append(are.SupplementaryWebLinks, link)
			}
		}
	}

	// 处理引用
	if refsData, ok := data["references"].([]any); ok {
		are.References = parseReferences(refsData)
	}

	if codeRefsData, ok := data["codeReference"].([]any); ok {
		are.CodeReference = parseReferences(codeRefsData)
	}

	// 处理后续提示
	if promptData, ok := data["followupPrompt"].(map[string]any); ok {
		prompt := &FollowupPrompt{}
		if content, ok := promptData["content"].(string); ok {
			prompt.Content = content
		}
		if userIntent, ok := promptData["userIntent"].(string); ok {
			intent := UserIntent(userIntent)
			prompt.UserIntent = &intent
		}
		are.FollowupPrompt = prompt
	}

	// 处理编程语言
	if langData, ok := data["programmingLanguage"].(map[string]any); ok {
		if langName, ok := langData["languageName"].(string); ok {
			are.ProgrammingLanguage = &ProgrammingLanguage{
				LanguageName: langName,
			}
		}
	}

	// 处理自定义模型
	if customsData, ok := data["customizations"].([]any); ok {
		for _, customData := range customsData {
			if customMap, ok := customData.(map[string]any); ok {
				custom := Customization{}
				if arn, ok := customMap["arn"].(string); ok {
					custom.ARN = arn
				}
				if name, ok := customMap["name"].(string); ok {
					custom.Name = &name
				}
				are.Customizations = append(are.Customizations, custom)
			}
		}
	}

	// 处理用户意图
	if userIntent, ok := data["userIntent"].(string); ok {
		intent := UserIntent(userIntent)
		are.UserIntent = &intent
	}

	// 处理代码查询
	if queryData, ok := data["codeQuery"].(map[string]any); ok {
		query := &CodeQuery{}
		if queryID, ok := queryData["codeQueryId"].(string); ok {
			query.CodeQueryID = queryID
		}
		if msgID, ok := queryData["userInputMessageId"].(string); ok {
			query.UserInputMessageID = &msgID
		}
		if langData, ok := queryData["programmingLanguage"].(map[string]any); ok {
			if langName, ok := langData["languageName"].(string); ok {
				query.ProgrammingLanguage = &ProgrammingLanguage{
					LanguageName: langName,
				}
			}
		}
		are.CodeQuery = query
	}

	return nil
}

// ToDict 转换为map[string]any
func (are *AssistantResponseEvent) ToDict() map[string]any {
	result := make(map[string]any)

	// 核心字段
	result["conversationId"] = are.ConversationID
	result["messageId"] = are.MessageID
	result["content"] = are.Content

	if are.ContentType != "" {
		result["contentType"] = string(are.ContentType)
	}

	if are.MessageStatus != "" {
		result["messageStatus"] = string(are.MessageStatus)
	}

	// 引用和链接
	if len(are.SupplementaryWebLinks) > 0 {
		links := make([]map[string]any, 0, len(are.SupplementaryWebLinks))
		for _, link := range are.SupplementaryWebLinks {
			linkMap := map[string]any{
				"url": link.URL,
			}
			if link.Title != nil {
				linkMap["title"] = *link.Title
			}
			if link.Snippet != nil {
				linkMap["snippet"] = *link.Snippet
			}
			if link.Score != nil {
				linkMap["score"] = *link.Score
			}
			links = append(links, linkMap)
		}
		result["supplementaryWebLinks"] = links
	}

	if len(are.References) > 0 {
		result["references"] = referencesToDict(are.References)
	}

	if len(are.CodeReference) > 0 {
		result["codeReference"] = referencesToDict(are.CodeReference)
	}

	// 交互字段
	if are.FollowupPrompt != nil {
		promptMap := map[string]any{
			"content": are.FollowupPrompt.Content,
		}
		if are.FollowupPrompt.UserIntent != nil {
			promptMap["userIntent"] = string(*are.FollowupPrompt.UserIntent)
		}
		result["followupPrompt"] = promptMap
	}

	// 上下文字段
	if are.ProgrammingLanguage != nil {
		result["programmingLanguage"] = map[string]any{
			"languageName": are.ProgrammingLanguage.LanguageName,
		}
	}

	if len(are.Customizations) > 0 {
		customs := make([]map[string]any, 0, len(are.Customizations))
		for _, custom := range are.Customizations {
			customMap := map[string]any{
				"arn": custom.ARN,
			}
			if custom.Name != nil {
				customMap["name"] = *custom.Name
			}
			customs = append(customs, customMap)
		}
		result["customizations"] = customs
	}

	if are.UserIntent != nil {
		result["userIntent"] = string(*are.UserIntent)
	}

	if are.CodeQuery != nil {
		queryMap := map[string]any{
			"codeQueryId": are.CodeQuery.CodeQueryID,
		}
		if are.CodeQuery.UserInputMessageID != nil {
			queryMap["userInputMessageId"] = *are.CodeQuery.UserInputMessageID
		}
		if are.CodeQuery.ProgrammingLanguage != nil {
			queryMap["programmingLanguage"] = map[string]any{
				"languageName": are.CodeQuery.ProgrammingLanguage.LanguageName,
			}
		}
		result["codeQuery"] = queryMap
	}

	return result
}

// Validate 验证AssistantResponseEvent的完整性
func (are *AssistantResponseEvent) Validate() error {
	// 对于流式响应，如果只有content，则认为是有效的
	if are.ConversationID == "" && are.MessageID == "" && are.Content != "" {
		// 流式响应，只验证有content即可
		return nil
	}

	// 对于工具调用事件，如果只有工具相关字段，也认为是有效的
	if are.ConversationID == "" && are.MessageID == "" && are.CodeQuery != nil {
		// 工具调用事件，只验证工具字段即可
		return nil
	}

	// 对于部分响应或工具事件，放宽验证要求
	// 只要有任何有效内容，就认为是有效的
	hasValidContent := are.Content != "" ||
		are.CodeQuery != nil ||
		len(are.SupplementaryWebLinks) > 0 ||
		len(are.References) > 0 ||
		len(are.CodeReference) > 0 ||
		are.FollowupPrompt != nil

	if hasValidContent {
		// 有有效内容，跳过严格的ID验证
		// 但仍然验证枚举值
		goto ValidateEnums
	}

	// 完整响应需要验证所有必需字段
	if are.ConversationID == "" {
		// 对于没有任何内容的响应，才要求conversationId
		if !hasValidContent {
			return fmt.Errorf("conversationId不能为空")
		}
	}

	if are.MessageID == "" {
		// 对于没有任何内容的响应，才要求messageId
		if !hasValidContent {
			return fmt.Errorf("messageId不能为空")
		}
	}

ValidateEnums:

	// 验证枚举值
	if are.MessageStatus != "" {
		switch are.MessageStatus {
		case MessageStatusCompleted, MessageStatusInProgress, MessageStatusError:
		default:
			return fmt.Errorf("无效的messageStatus: %s", are.MessageStatus)
		}
	}

	if are.ContentType != "" {
		switch are.ContentType {
		case ContentTypeMarkdown, ContentTypePlain, ContentTypeJSON:
		default:
			return fmt.Errorf("无效的contentType: %s", are.ContentType)
		}
	}

	if are.UserIntent != nil {
		switch *are.UserIntent {
		case UserIntentExplainCodeSelection, UserIntentSuggestAlternateImpl,
			UserIntentApplyCommonBestPractices, UserIntentImproveCode,
			UserIntentShowExamples, UserIntentCiteSources, UserIntentExplainLineByLine:
		default:
			return fmt.Errorf("无效的userIntent: %s", *are.UserIntent)
		}
	}

	return nil
}

// parseReferences 解析引用列表
func parseReferences(refsData []any) []Reference {
	var refs []Reference
	for _, refData := range refsData {
		if refMap, ok := refData.(map[string]any); ok {
			ref := Reference{}
			if licenseName, ok := refMap["licenseName"].(string); ok {
				ref.LicenseName = &licenseName
			}
			if repository, ok := refMap["repository"].(string); ok {
				ref.Repository = &repository
			}
			if url, ok := refMap["url"].(string); ok {
				ref.URL = &url
			}
			if info, ok := refMap["information"].(string); ok {
				ref.Information = &info
			}

			// 处理推荐内容范围
			if spanData, ok := refMap["recommendationContentSpan"].(map[string]any); ok {
				span := &ContentSpan{}
				if start, ok := spanData["start"].(float64); ok {
					span.Start = int(start)
				}
				if end, ok := spanData["end"].(float64); ok {
					span.End = int(end)
				}
				ref.RecommendationContentSpan = span
			}

			// 处理最相关遗漏替代方案
			if altData, ok := refMap["mostRelevantMissedAlternative"].(map[string]any); ok {
				alt := &MostRelevantMissedAlternative{}
				if url, ok := altData["url"].(string); ok {
					alt.URL = url
				}
				if licenseName, ok := altData["licenseName"].(string); ok {
					alt.LicenseName = &licenseName
				}
				if repository, ok := altData["repository"].(string); ok {
					alt.Repository = &repository
				}
				ref.MostRelevantMissedAlternative = alt
			}

			refs = append(refs, ref)
		}
	}
	return refs
}

// referencesToDict 将引用列表转换为字典
func referencesToDict(refs []Reference) []map[string]any {
	result := make([]map[string]any, 0, len(refs))
	for _, ref := range refs {
		refMap := make(map[string]any)

		if ref.LicenseName != nil {
			refMap["licenseName"] = *ref.LicenseName
		}
		if ref.Repository != nil {
			refMap["repository"] = *ref.Repository
		}
		if ref.URL != nil {
			refMap["url"] = *ref.URL
		}
		if ref.Information != nil {
			refMap["information"] = *ref.Information
		}

		if ref.RecommendationContentSpan != nil {
			refMap["recommendationContentSpan"] = map[string]any{
				"start": ref.RecommendationContentSpan.Start,
				"end":   ref.RecommendationContentSpan.End,
			}
		}

		if ref.MostRelevantMissedAlternative != nil {
			altMap := map[string]any{
				"url": ref.MostRelevantMissedAlternative.URL,
			}
			if ref.MostRelevantMissedAlternative.LicenseName != nil {
				altMap["licenseName"] = *ref.MostRelevantMissedAlternative.LicenseName
			}
			if ref.MostRelevantMissedAlternative.Repository != nil {
				altMap["repository"] = *ref.MostRelevantMissedAlternative.Repository
			}
			refMap["mostRelevantMissedAlternative"] = altMap
		}

		result = append(result, refMap)
	}
	return result
}

// MarshalJSON 自定义JSON序列化
func (are *AssistantResponseEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(are.ToDict())
}

// UnmarshalJSON 自定义JSON反序列化
func (are *AssistantResponseEvent) UnmarshalJSON(data []byte) error {
	var dict map[string]any
	if err := json.Unmarshal(data, &dict); err != nil {
		return fmt.Errorf("JSON反序列化失败: %w", err)
	}

	return are.FromDict(dict)
}
