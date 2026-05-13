package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

const estimatedImageInputTokens = 256

type anthropicCountTokensResponse struct {
	InputTokens int `json:"input_tokens"`
}

// estimateAnthropicCountTokens returns a best-effort estimate for Claude-style
// input tokens without calling an upstream count_tokens endpoint. This is used
// for OpenAI-compatible groups where upstream support is inconsistent or absent.
func EstimateAnthropicCountTokens(req *apicompat.AnthropicRequest) int {
	if req == nil {
		return 0
	}

	total := 0
	total += estimateAnthropicSystemTokens(req.System)

	for _, msg := range req.Messages {
		total += estimateTokensForText(msg.Role)
		total += estimateAnthropicMessageContentTokens(msg.Content)
	}

	for _, tool := range req.Tools {
		total += estimateTokensForText(tool.Type)
		total += estimateTokensForText(tool.Name)
		total += estimateTokensForText(tool.Description)
		total += estimateJSONTextTokens(tool.InputSchema)
	}

	total += estimateJSONTextTokens(req.ToolChoice)
	total += estimateJSONTextTokens(req.Metadata)
	total += estimateTokensForText(strings.Join(req.StopSeqs, "\n"))
	if req.Thinking != nil {
		total += estimateTokensForText(req.Thinking.Type)
	}
	if req.OutputConfig != nil {
		total += estimateTokensForText(req.OutputConfig.Effort)
	}

	if total < 0 {
		return 0
	}
	return total
}

func estimateAnthropicSystemTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return estimateTokensForText(text)
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return estimateJSONTextTokens(raw)
	}

	total := 0
	for _, block := range blocks {
		total += estimateAnthropicContentBlockTokens(block)
	}
	return total
}

func estimateAnthropicMessageContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return estimateTokensForText(text)
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return estimateJSONTextTokens(raw)
	}

	total := 0
	for _, block := range blocks {
		total += estimateAnthropicContentBlockTokens(block)
	}
	return total
}

func estimateAnthropicContentBlockTokens(block apicompat.AnthropicContentBlock) int {
	total := 0
	total += estimateTokensForText(block.Type)
	total += estimateTokensForText(block.Text)
	total += estimateTokensForText(block.Thinking)
	total += estimateTokensForText(block.ID)
	total += estimateTokensForText(block.Name)
	total += estimateTokensForText(block.ToolUseID)
	total += estimateJSONTextTokens(block.Input)
	total += estimateAnthropicToolResultContentTokens(block.Content)

	if block.CacheControl != nil {
		total += estimateTokensForText(block.CacheControl.Type)
		total += estimateTokensForText(block.CacheControl.TTL)
	}
	if block.Source != nil {
		total += estimateTokensForText(block.Source.Type)
		total += estimateTokensForText(block.Source.MediaType)
		if strings.TrimSpace(block.Source.Data) != "" {
			total += estimatedImageInputTokens
		}
	}

	return total
}

func estimateAnthropicToolResultContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return estimateTokensForText(text)
	}

	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return estimateJSONTextTokens(raw)
	}

	total := 0
	for _, block := range blocks {
		total += estimateAnthropicContentBlockTokens(block)
	}
	return total
}
