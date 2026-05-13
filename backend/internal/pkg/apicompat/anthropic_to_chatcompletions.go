package apicompat

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// AnthropicToChatCompletions converts an Anthropic Messages request directly
// into a Chat Completions request.
func AnthropicToChatCompletions(req *AnthropicRequest) (*ChatCompletionsRequest, error) {
	messages, err := convertAnthropicMessagesToChatMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	out := &ChatCompletionsRequest{
		Model:           req.Model,
		Messages:        messages,
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		Stream:          req.Stream,
		ReasoningEffort: anthropicEffortToChatReasoningEffort(req.OutputConfig),
	}

	if len(req.System) > 0 {
		content, err := marshalAnthropicSystemToChatContent(req.System)
		if err != nil {
			return nil, err
		}
		if len(content) > 0 {
			out.Messages = append([]ChatMessage{{Role: "system", Content: content}}, out.Messages...)
		}
	}

	if req.MaxTokens > 0 {
		v := req.MaxTokens
		out.MaxTokens = &v
	}
	if req.Stream {
		out.StreamOptions = &ChatStreamOptions{IncludeUsage: true}
	}
	if len(req.StopSeqs) > 0 {
		out.Stop = mustMarshalJSON(req.StopSeqs)
	}

	if len(req.Tools) > 0 {
		out.Tools = convertAnthropicToolsToChatTools(req.Tools)
	}

	if len(req.ToolChoice) > 0 {
		tc, err := convertAnthropicToolChoiceToChatToolChoice(req.ToolChoice)
		if err != nil {
			return nil, err
		}
		out.ToolChoice = tc
	}

	return out, nil
}

func marshalAnthropicSystemToChatContent(raw json.RawMessage) (json.RawMessage, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if strings.TrimSpace(s) == "" || isAnthropicBillingHeaderText(s) {
			return nil, nil
		}
		return json.Marshal(s)
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var parts []ChatContentPart
	for _, block := range blocks {
		if block.Type == "text" && block.Text != "" && !isAnthropicBillingHeaderText(block.Text) {
			parts = append(parts, ChatContentPart{Type: "text", Text: block.Text})
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return json.Marshal(parts)
}

func anthropicEffortToChatReasoningEffort(cfg *AnthropicOutputConfig) string {
	if cfg == nil {
		return ""
	}
	switch strings.TrimSpace(cfg.Effort) {
	case "low", "medium", "high":
		return cfg.Effort
	case "max":
		return "xhigh"
	default:
		return ""
	}
}

func convertAnthropicMessagesToChatMessages(messages []AnthropicMessage) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(messages))
	for _, msg := range messages {
		converted, err := convertAnthropicMessageToChatMessage(msg)
		if err != nil {
			return nil, err
		}
		out = append(out, converted...)
	}
	return out, nil
}

func convertAnthropicMessageToChatMessage(msg AnthropicMessage) ([]ChatMessage, error) {
	if has, err := anthropicContentHasImage(msg.Content); err != nil {
		return nil, err
	} else if has {
		return nil, fmt.Errorf("images are not supported for this Nvidia messages path")
	}

	switch msg.Role {
	case "assistant":
		return convertAnthropicAssistantMessageToChat(msg.Content)
	case "user":
		return convertAnthropicUserMessageToChat(msg.Content)
	case "system":
		return convertAnthropicSystemMessageToChat(msg.Content)
	default:
		return convertAnthropicUserMessageToChat(msg.Content)
	}
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func convertAnthropicSystemMessageToChat(raw json.RawMessage) ([]ChatMessage, error) {
	text, parts, err := parseAnthropicContent(raw)
	if err != nil {
		return nil, err
	}
	if text == "" && len(parts) == 0 {
		return nil, nil
	}
	content, err := marshalChatContent(text, parts)
	if err != nil {
		return nil, err
	}
	return []ChatMessage{{Role: "system", Content: content}}, nil
}

func convertAnthropicUserMessageToChat(raw json.RawMessage) ([]ChatMessage, error) {
	if len(raw) == 0 {
		return []ChatMessage{{Role: "user", Content: mustMarshalJSONString("")}}, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []ChatMessage{{Role: "user", Content: mustMarshalJSONString(s)}}, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var out []ChatMessage
	var userParts []ChatContentPart

	flushUser := func() error {
		if len(userParts) == 0 {
			return nil
		}
		content, err := marshalChatContentFromChatParts(userParts)
		if err != nil {
			return err
		}
		out = append(out, ChatMessage{Role: "user", Content: content})
		userParts = nil
		return nil
	}

	for _, block := range blocks {
		switch block.Type {
		case "tool_result":
			if err := flushUser(); err != nil {
				return nil, err
			}
			toolMsgs, err := convertAnthropicToolResultToChatMessages(block)
			if err != nil {
				return nil, err
			}
			out = append(out, toolMsgs...)
		case "text":
			if block.Text != "" {
				userParts = append(userParts, ChatContentPart{Type: "text", Text: block.Text})
			}
		case "thinking":
			if block.Thinking != "" {
				userParts = append(userParts, ChatContentPart{Type: "text", Text: "<thinking>" + block.Thinking + "</thinking>"})
			}
		case "image":
			if block.Source != nil && block.Source.Type == "base64" && block.Source.MediaType != "" && block.Source.Data != "" {
				userParts = append(userParts, ChatContentPart{
					Type: "image_url",
					ImageURL: &ChatImageURL{
						URL: "data:" + block.Source.MediaType + ";base64," + block.Source.Data,
					},
				})
			}
		}
	}

	if err := flushUser(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return []ChatMessage{{Role: "user", Content: mustMarshalJSONString("")}}, nil
	}
	return out, nil
}

func convertAnthropicToolResultToChatMessages(block AnthropicContentBlock) ([]ChatMessage, error) {
	outputText, imageParts := convertToolResultOutput(block)
	if len(imageParts) > 0 {
		return nil, fmt.Errorf("images are not supported for this Nvidia messages path")
	}
	return []ChatMessage{{
		Role:       "tool",
		ToolCallID: block.ToolUseID,
		Content:    mustMarshalJSONString(outputText),
	}}, nil
}

func convertAnthropicAssistantMessageToChat(raw json.RawMessage) ([]ChatMessage, error) {
	var out ChatMessage
	out.Role = "assistant"

	text, parts, err := parseAnthropicContent(raw)
	if err != nil {
		return nil, err
	}

	if text != "" || len(parts) > 0 {
		content, err := marshalChatContent(text, parts)
		if err != nil {
			return nil, err
		}
		out.Content = content
	}

	if len(parts) > 0 {
		for _, block := range parts {
			switch block.Type {
			case "tool_use":
				out.ToolCalls = append(out.ToolCalls, ChatToolCall{
					ID:   block.ID,
					Type: "function",
					Function: ChatFunctionCall{
						Name:      block.Name,
						Arguments: anthropicToolInputToArguments(block.Input),
					},
				})
			case "thinking":
				if block.Thinking != "" {
					out.ReasoningContent += block.Thinking
				}
			case "text":
				if block.Text != "" {
					out.Content = appendChatContentText(out.Content, block.Text)
				}
			}
		}
	}

	if len(out.Content) == 0 && len(out.ToolCalls) == 0 && out.ReasoningContent == "" {
		return []ChatMessage{{Role: "assistant", Content: mustMarshalJSONString("")}}, nil
	}
	return []ChatMessage{out}, nil
}

func parseAnthropicContent(raw json.RawMessage) (string, []AnthropicContentBlock, error) {
	if len(raw) == 0 {
		return "", nil, nil
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", nil, err
	}

	var textParts []string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textParts = append(textParts, block.Text)
			}
		case "thinking":
			if block.Thinking != "" {
				textParts = append(textParts, "<thinking>"+block.Thinking+"</thinking>")
			}
		}
	}

	return strings.Join(textParts, ""), blocks, nil
}

func marshalChatContent(text string, parts []AnthropicContentBlock) (json.RawMessage, error) {
	if len(parts) > 0 {
		chatParts := make([]ChatContentPart, 0, len(parts))
		for _, block := range parts {
			switch block.Type {
			case "text":
				if block.Text != "" {
					chatParts = append(chatParts, ChatContentPart{Type: "text", Text: block.Text})
				}
			case "image":
				if block.Source != nil && block.Source.Type == "base64" && block.Source.MediaType != "" && block.Source.Data != "" {
					chatParts = append(chatParts, ChatContentPart{
						Type: "image_url",
						ImageURL: &ChatImageURL{
							URL: "data:" + block.Source.MediaType + ";base64," + block.Source.Data,
						},
					})
				}
			}
		}
		if len(chatParts) > 0 {
			return json.Marshal(chatParts)
		}
	}
	return json.Marshal(text)
}

func marshalChatContentFromChatParts(parts []ChatContentPart) (json.RawMessage, error) {
	if len(parts) == 0 {
		return mustMarshalJSONString(""), nil
	}
	if len(parts) == 1 && parts[0].Type == "text" && parts[0].ImageURL == nil {
		return mustMarshalJSONString(parts[0].Text), nil
	}
	return json.Marshal(parts)
}

func appendChatContentText(raw json.RawMessage, text string) json.RawMessage {
	if text == "" {
		return raw
	}
	if len(raw) == 0 {
		b, _ := json.Marshal(text)
		return b
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		b, _ := json.Marshal(s + text)
		return b
	}

	var parts []ChatContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		parts = append(parts, ChatContentPart{Type: "text", Text: text})
		b, _ := json.Marshal(parts)
		return b
	}

	b, _ := json.Marshal(text)
	return b
}

func anthropicToolInputToArguments(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func convertAnthropicToolsToChatTools(tools []AnthropicTool) []ChatTool {
	out := make([]ChatTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, ChatTool{
			Type: "function",
			Function: &ChatFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
				Strict:      nil,
			},
		})
	}
	return out
}

func convertAnthropicToolChoiceToChatToolChoice(raw json.RawMessage) (json.RawMessage, error) {
	var tc struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}

	switch tc.Type {
	case "auto":
		return json.Marshal("auto")
	case "any":
		return json.Marshal("required")
	case "none":
		return json.Marshal("none")
	case "tool":
		return json.Marshal(map[string]any{
			"type": "function",
			"name": tc.Name,
		})
	default:
		return raw, nil
	}
}

func mustMarshalJSONString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

func generateAnthropicMsgID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func generateAnthropicRespID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "msg_" + hex.EncodeToString(b)
}

func generateChatMessageID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "chatcmpl-" + hex.EncodeToString(b)
}

// AnthropicToChatCompletionsResponse converts an Anthropic response into a
// Chat Completions response.
func AnthropicToChatCompletionsResponse(resp *AnthropicResponse, model string) *ChatCompletionsResponse {
	id := generateChatMessageID()
	if resp != nil && resp.ID != "" {
		id = resp.ID
	}
	out := &ChatCompletionsResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
	}
	if resp == nil {
		out.Choices = []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: mustMarshalJSONString("")},
			FinishReason: "stop",
		}}
		return out
	}

	var contentText string
	var reasoningText string
	var toolCalls []ChatToolCall
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			contentText += block.Text
		case "thinking":
			reasoningText += block.Thinking
		case "tool_use":
			toolCalls = append(toolCalls, ChatToolCall{
				ID:   block.ID,
				Type: "function",
				Function: ChatFunctionCall{
					Name:      block.Name,
					Arguments: anthropicToolInputToArguments(block.Input),
				},
			})
		}
	}

	msg := ChatMessage{Role: "assistant"}
	if contentText != "" {
		msg.Content = mustMarshalJSONString(contentText)
	}
	if reasoningText != "" {
		msg.ReasoningContent = reasoningText
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	finishReason := "stop"
	switch resp.StopReason {
	case "max_tokens":
		finishReason = "length"
	case "tool_use":
		finishReason = "tool_calls"
	}
	out.Choices = []ChatChoice{{
		Index:        0,
		Message:      msg,
		FinishReason: finishReason,
	}}
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		out.Usage = &ChatUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		}
		if resp.Usage.CacheReadInputTokens > 0 {
			out.Usage.PromptTokensDetails = &ChatTokenDetails{CachedTokens: resp.Usage.CacheReadInputTokens}
		}
	}
	return out
}

func anthropicContentHasImage(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}

	var blocks []AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return false, nil
	}
	for _, block := range blocks {
		if block.Type == "image" {
			return true, nil
		}
	}
	return false, nil
}
