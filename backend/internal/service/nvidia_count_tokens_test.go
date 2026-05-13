package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestEstimateAnthropicCountTokens_TextAndTools(t *testing.T) {
	req := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: json.RawMessage(`"You are a concise coding assistant."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: json.RawMessage(`"hello world"`)},
			{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"I can help."}]`)},
		},
		Tools: []apicompat.AnthropicTool{
			{
				Name:        "run_command",
				Description: "Execute a command",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"cmd":{"type":"string"}}}`),
			},
		},
		StopSeqs: []string{"STOP"},
	}

	got := EstimateAnthropicCountTokens(req)
	require.Greater(t, got, 0)
	// Includes system, messages, tool metadata, and stop sequence.
	require.GreaterOrEqual(t, got, estimateTokensForText("hello world")+estimateTokensForText("run_command"))
}

func TestEstimateAnthropicCountTokens_ImageUsesBoundedEstimate(t *testing.T) {
	req := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-5",
		Messages: []apicompat.AnthropicMessage{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"describe this image"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
				]`),
			},
		},
	}

	got := EstimateAnthropicCountTokens(req)
	require.GreaterOrEqual(t, got, estimatedImageInputTokens)
	// The image contribution should be bounded, not proportional to raw base64 length.
	require.Less(t, got, estimatedImageInputTokens+estimateTokensForText("describe this image")+128)
}

