package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/tidwall/gjson"
)

func buildOpenAIChannelMonitorPath(model string) string {
	if shouldUseOpenAIResponsesMonitorPath(model) {
		return providerOpenAIResponsesPath
	}
	return providerOpenAIPath
}

func buildOpenAIChannelMonitorBody(model, prompt string) ([]byte, error) {
	if shouldUseOpenAIResponsesMonitorPath(model) {
		return json.Marshal(map[string]any{
			"model":        model,
			"instructions": openai.DefaultInstructions,
			"input": []map[string]any{
				{
					"type": "message",
					"role": "user",
					"content": []map[string]any{
						{
							"type": "input_text",
							"text": prompt,
						},
					},
				},
			},
			"max_output_tokens": monitorChallengeMaxTokens,
			"stream":            false,
		})
	}
	return json.Marshal(map[string]any{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens": monitorChallengeMaxTokens,
		"stream":     false,
	})
}

func extractOpenAIChannelMonitorText(model string, respBytes []byte) string {
	if shouldUseOpenAIResponsesMonitorPath(model) {
		return extractOpenAIResponsesMonitorText(respBytes)
	}
	return gjson.GetBytes(respBytes, "choices.0.message.content").String()
}

func extractOpenAIResponsesMonitorText(respBytes []byte) string {
	if len(respBytes) == 0 {
		return ""
	}

	var resp apicompat.ResponsesResponse
	if err := json.Unmarshal(respBytes, &resp); err == nil {
		chatResp := apicompat.ResponsesToChatCompletions(&resp, resp.Model)
		if chatResp != nil && len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.Content) > 0 {
			var text string
			if err := json.Unmarshal(chatResp.Choices[0].Message.Content, &text); err == nil {
				return strings.TrimSpace(text)
			}
		}
	}

	var builder strings.Builder
	for _, output := range gjson.GetBytes(respBytes, "output").Array() {
		for _, part := range output.Get("content").Array() {
			partType := strings.TrimSpace(part.Get("type").String())
			text := part.Get("text").String()
			if text == "" {
				continue
			}
			if partType != "output_text" && partType != "text" {
				continue
			}
			_, _ = builder.WriteString(text)
		}
	}
	return strings.TrimSpace(builder.String())
}

func shouldUseOpenAIResponsesMonitorPath(model string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(model))
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "codex") {
		return true
	}
	normalized := strings.ToLower(strings.TrimSpace(normalizeCodexModel(trimmed)))
	return strings.Contains(normalized, "codex")
}
