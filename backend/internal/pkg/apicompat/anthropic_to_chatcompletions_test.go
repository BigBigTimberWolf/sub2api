package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnthropicToChatCompletions_UserToolResultBecomesToolMessage(t *testing.T) {
	req := &AnthropicRequest{
		Model:     "gpt-5.2",
		MaxTokens: 128,
		Messages: []AnthropicMessage{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"Before"},
					{"type":"tool_result","tool_use_id":"call_1","content":[{"type":"text","text":"ok"}]},
					{"type":"text","text":"After"}
				]`),
			},
		},
	}

	chatReq, err := AnthropicToChatCompletions(req)
	require.NoError(t, err)
	require.Len(t, chatReq.Messages, 3)

	assert.Equal(t, "user", chatReq.Messages[0].Role)
	assert.JSONEq(t, `"Before"`, string(chatReq.Messages[0].Content))

	assert.Equal(t, "tool", chatReq.Messages[1].Role)
	assert.Equal(t, "call_1", chatReq.Messages[1].ToolCallID)
	assert.JSONEq(t, `"ok"`, string(chatReq.Messages[1].Content))

	assert.Equal(t, "user", chatReq.Messages[2].Role)
	assert.JSONEq(t, `"After"`, string(chatReq.Messages[2].Content))
}

func TestChatCompletionsToAnthropicResponse_FinishReasonPointer(t *testing.T) {
	finishReason := "length"
	resp := &ChatCompletionsResponse{
		ID:     "chatcmpl_123",
		Object: "chat.completion",
		Model:  "gpt-5.2",
		Choices: []ChatChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: mustMarshalJSONString("Hello"),
				},
				FinishReason: finishReason,
			},
		},
	}

	anth := ChatCompletionsToAnthropicResponse(resp, "claude-sonnet-4-5")
	require.NotNil(t, anth)
	assert.Equal(t, "claude-sonnet-4-5", anth.Model)
	assert.Equal(t, "max_tokens", anth.StopReason)
	require.Len(t, anth.Content, 1)
	assert.Equal(t, "text", anth.Content[0].Type)
	assert.Equal(t, "Hello", anth.Content[0].Text)
}

func TestChatCompletionsChunkToAnthropicEvents_FinishReasonPointer(t *testing.T) {
	finishReason := "tool_calls"
	state := NewChatEventToAnthropicState()
	state.Model = "claude-sonnet-4-5"

	events := ChatCompletionsChunkToAnthropicEvents(&ChatCompletionsChunk{
		ID:    "chatcmpl_stream_1",
		Model: "gpt-5.2",
		Choices: []ChatChunkChoice{
			{
				Delta: ChatDelta{
					Role: "assistant",
				},
				FinishReason: &finishReason,
			},
		},
		Usage: &ChatUsage{PromptTokens: 10, CompletionTokens: 2},
	}, state)

	require.NotEmpty(t, events)
	var sawStop bool
	for _, evt := range events {
		if evt.Type == "message_delta" {
			assert.Equal(t, "tool_use", evt.Delta.StopReason)
			sawStop = true
		}
	}
	assert.True(t, sawStop)
}
