package apicompat

import (
	"encoding/json"
	"strings"
	"time"
)

// ChatCompletionsToAnthropicResponse converts a Chat Completions response into
// an Anthropic Messages response.
func ChatCompletionsToAnthropicResponse(resp *ChatCompletionsResponse, model string) *AnthropicResponse {
	out := &AnthropicResponse{
		ID:      generateAnthropicRespID(),
		Type:    "message",
		Role:    "assistant",
		Model:   model,
		Content: []AnthropicContentBlock{},
	}
	if resp == nil || len(resp.Choices) == 0 {
		out.Content = []AnthropicContentBlock{{Type: "text", Text: ""}}
		return out
	}

	choice := resp.Choices[0]
	if len(choice.Message.Content) > 0 {
		if content, err := chatContentToAnthropicBlocks(choice.Message.Content); err == nil && len(content) > 0 {
			out.Content = append(out.Content, content...)
		}
	}
	if choice.Message.ReasoningContent != "" {
		out.Content = append(out.Content, AnthropicContentBlock{
			Type:     "thinking",
			Thinking: choice.Message.ReasoningContent,
		})
	}
	for _, tc := range choice.Message.ToolCalls {
		out.Content = append(out.Content, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	if len(out.Content) == 0 {
		out.Content = []AnthropicContentBlock{{Type: "text", Text: ""}}
	}

	out.StopReason = chatFinishReasonToAnthropicStopReason(choice.FinishReason)
	if resp.Usage != nil {
		out.Usage = AnthropicUsage{
			InputTokens:          resp.Usage.PromptTokens,
			OutputTokens:         resp.Usage.CompletionTokens,
			CacheReadInputTokens: 0,
		}
		if resp.Usage.PromptTokensDetails != nil {
			out.Usage.CacheReadInputTokens = resp.Usage.PromptTokensDetails.CachedTokens
		}
	}
	return out
}

func chatContentToAnthropicBlocks(content json.RawMessage) ([]AnthropicContentBlock, error) {
	parsed, err := parseChatMessageContent(content)
	if err != nil {
		return nil, err
	}
	if parsed.Text != nil {
		if *parsed.Text == "" {
			return []AnthropicContentBlock{{Type: "text", Text: ""}}, nil
		}
		return []AnthropicContentBlock{{Type: "text", Text: *parsed.Text}}, nil
	}
	var blocks []AnthropicContentBlock
	for _, part := range parsed.Parts {
		switch part.Type {
		case "text":
			if part.Text != "" {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: part.Text})
			}
		case "image_url":
			if part.ImageURL != nil && part.ImageURL.URL != "" {
				blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: part.ImageURL.URL})
			}
		}
	}
	if len(blocks) == 0 {
		blocks = append(blocks, AnthropicContentBlock{Type: "text", Text: ""})
	}
	return blocks, nil
}

func chatFinishReasonToAnthropicStopReason(finishReason string) string {
	switch strings.TrimSpace(finishReason) {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "content_filter"
	default:
		return "end_turn"
	}
}

func chatChoiceFinishReason(finishReason *string) string {
	if finishReason == nil {
		return ""
	}
	return strings.TrimSpace(*finishReason)
}

// ChatEventToAnthropicState tracks a Chat Completions SSE stream while it is
// translated into Anthropic SSE events or a final Anthropic response.
type ChatEventToAnthropicState struct {
	ResponseID string
	Model      string
	Created    int64

	MessageStartSent bool
	MessageStopSent  bool

	Blocks             []AnthropicContentBlock
	ContentBlockOpen   bool
	CurrentBlockType   string // text | thinking | tool_use
	CurrentToolIndex   int
	CurrentToolName    string
	CurrentToolArgs    strings.Builder
	ToolIndexToBlockID map[int]int

	PendingStopReason string

	InputTokens          int
	OutputTokens         int
	CacheReadInputTokens int
}

// NewChatEventToAnthropicState returns an initialised stream state.
func NewChatEventToAnthropicState() *ChatEventToAnthropicState {
	return &ChatEventToAnthropicState{
		ResponseID:         generateAnthropicRespID(),
		Created:            time.Now().Unix(),
		ToolIndexToBlockID: make(map[int]int),
	}
}

// ChatCompletionsChunkToAnthropicEvents converts a single Chat Completions SSE
// chunk into zero or more Anthropic SSE events.
func ChatCompletionsChunkToAnthropicEvents(chunk *ChatCompletionsChunk, state *ChatEventToAnthropicState) []AnthropicStreamEvent {
	if chunk == nil || state == nil {
		return nil
	}

	if chunk.ID != "" && !state.MessageStartSent && len(state.Blocks) == 0 && strings.HasPrefix(state.ResponseID, "msg_") {
		state.ResponseID = chunk.ID
	}
	if state.Model == "" && chunk.Model != "" {
		state.Model = chunk.Model
	}
	if len(chunk.Choices) == 0 {
		return chatChunkUsageOnlyToAnthropicEvents(chunk, state)
	}

	choice := chunk.Choices[0]
	events := make([]AnthropicStreamEvent, 0, 4)

	if choice.Delta.Role != "" && !state.MessageStartSent {
		events = append(events, emitChatAnthropicMessageStart(state))
		state.MessageStartSent = true
	}
	if !state.MessageStartSent {
		events = append(events, emitChatAnthropicMessageStart(state))
		state.MessageStartSent = true
	}

	if choice.Delta.Content != nil && *choice.Delta.Content != "" {
		events = append(events, chatAnthropicEnsureBlockStart(state, "text")...)
		appendAnthropicTextBlock(state, *choice.Delta.Content)
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: intPtr(state.ContentBlockIndex()),
			Delta: &AnthropicDelta{
				Type: "text_delta",
				Text: *choice.Delta.Content,
			},
		})
	}
	if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
		events = append(events, chatAnthropicEnsureBlockStart(state, "thinking")...)
		appendAnthropicThinkingBlock(state, *choice.Delta.ReasoningContent)
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: intPtr(state.ContentBlockIndex()),
			Delta: &AnthropicDelta{
				Type:     "thinking_delta",
				Thinking: *choice.Delta.ReasoningContent,
			},
		})
	}
	if len(choice.Delta.ToolCalls) > 0 {
		for _, tc := range choice.Delta.ToolCalls {
			events = append(events, chatAnthropicHandleToolCallDelta(state, tc)...)
		}
	}

	if finishReason := chatChoiceFinishReason(choice.FinishReason); finishReason != "" {
		events = append(events, chatAnthropicCloseCurrentBlock(state)...)
		state.PendingStopReason = chatFinishReasonToAnthropicStopReason(finishReason)
	}
	if chunk.Usage != nil {
		usage := chatAnthropicUsageFromChatUsage(chunk.Usage)
		state.InputTokens = usage.InputTokens
		state.OutputTokens = usage.OutputTokens
		state.CacheReadInputTokens = usage.CacheReadInputTokens
		if state.PendingStopReason != "" && !state.MessageStopSent {
			events = append(events, chatAnthropicFinalizeMessage(state)...)
		}
	}

	return events
}

// FinalizeChatAnthropicStream emits any missing Anthropic terminal events if
// the Chat stream ended without a final finish_reason/usage chunk.
func FinalizeChatAnthropicStream(state *ChatEventToAnthropicState) []AnthropicStreamEvent {
	if state == nil || state.MessageStopSent {
		return nil
	}
	var events []AnthropicStreamEvent
	if !state.MessageStartSent {
		events = append(events, emitChatAnthropicMessageStart(state))
		state.MessageStartSent = true
	}
	events = append(events, chatAnthropicCloseCurrentBlock(state)...)
	if state.PendingStopReason == "" {
		state.PendingStopReason = "end_turn"
	}
	events = append(events, chatAnthropicFinalizeMessage(state)...)
	return events
}

// ChatCompletionsToAnthropicResponseFromState converts the final stream state
// into a single Anthropic response object.
func ChatCompletionsToAnthropicResponseFromState(state *ChatEventToAnthropicState) *AnthropicResponse {
	if state == nil {
		return &AnthropicResponse{
			ID:      generateAnthropicRespID(),
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{{Type: "text", Text: ""}},
		}
	}
	blocks := append([]AnthropicContentBlock(nil), state.Blocks...)
	if len(blocks) == 0 {
		blocks = []AnthropicContentBlock{{Type: "text", Text: ""}}
	}
	return &AnthropicResponse{
		ID:         state.ResponseID,
		Type:       "message",
		Role:       "assistant",
		Content:    blocks,
		Model:      state.Model,
		StopReason: chatAnthropicStopReasonForFinalize(state),
		Usage: AnthropicUsage{
			InputTokens:          state.InputTokens,
			OutputTokens:         state.OutputTokens,
			CacheReadInputTokens: state.CacheReadInputTokens,
		},
	}
}

func chatAnthropicStopReasonForFinalize(state *ChatEventToAnthropicState) string {
	if state == nil || state.PendingStopReason == "" {
		return "end_turn"
	}
	return state.PendingStopReason
}

func chatAnthropicFinalizeMessage(state *ChatEventToAnthropicState) []AnthropicStreamEvent {
	if state == nil || state.MessageStopSent {
		return nil
	}
	state.MessageStopSent = true
	return []AnthropicStreamEvent{
		{
			Type: "message_delta",
			Delta: &AnthropicDelta{
				StopReason: state.PendingStopReason,
			},
			Usage: &AnthropicUsage{
				InputTokens:          state.InputTokens,
				OutputTokens:         state.OutputTokens,
				CacheReadInputTokens: state.CacheReadInputTokens,
			},
		},
		{Type: "message_stop"},
	}
}

func chatAnthropicUsageFromChatUsage(usage *ChatUsage) AnthropicUsage {
	if usage == nil {
		return AnthropicUsage{}
	}
	out := AnthropicUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
	if usage.PromptTokensDetails != nil {
		out.CacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
	}
	return out
}

func emitChatAnthropicMessageStart(state *ChatEventToAnthropicState) AnthropicStreamEvent {
	return AnthropicStreamEvent{
		Type: "message_start",
		Message: &AnthropicResponse{
			ID:      state.ResponseID,
			Type:    "message",
			Role:    "assistant",
			Content: []AnthropicContentBlock{},
			Model:   state.Model,
			Usage:   AnthropicUsage{},
		},
	}
}

func chatAnthropicEnsureBlockStart(state *ChatEventToAnthropicState, blockType string) []AnthropicStreamEvent {
	if state.ContentBlockOpen && state.CurrentBlockType == blockType {
		return nil
	}
	events := chatAnthropicCloseCurrentBlock(state)
	idx := len(state.Blocks)
	state.ContentBlockOpen = true
	state.CurrentBlockType = blockType
	switch blockType {
	case "text":
		state.Blocks = append(state.Blocks, AnthropicContentBlock{Type: "text", Text: ""})
	case "thinking":
		state.Blocks = append(state.Blocks, AnthropicContentBlock{Type: "thinking", Thinking: ""})
	case "tool_use":
		state.Blocks = append(state.Blocks, AnthropicContentBlock{Type: "tool_use", Input: json.RawMessage("{}")})
	}
	return append(events, AnthropicStreamEvent{
		Type:         "content_block_start",
		Index:        intPtr(idx),
		ContentBlock: &state.Blocks[len(state.Blocks)-1],
	})
}

func chatAnthropicHandleToolCallDelta(state *ChatEventToAnthropicState, tc ChatToolCall) []AnthropicStreamEvent {
	if state == nil {
		return nil
	}
	idx := 0
	if tc.Index != nil {
		idx = *tc.Index
	} else if state.ContentBlockOpen && state.CurrentBlockType == "tool_use" {
		idx = state.CurrentToolIndex
	} else {
		idx = len(state.ToolIndexToBlockID)
	}

	events := make([]AnthropicStreamEvent, 0, 2)
	blockIdx, ok := state.ToolIndexToBlockID[idx]
	if !ok || !state.ContentBlockOpen || state.CurrentBlockType != "tool_use" || state.CurrentToolIndex != idx {
		events = append(events, chatAnthropicCloseCurrentBlock(state)...)
		state.CurrentToolIndex = idx
		state.CurrentToolName = tc.Function.Name
		state.CurrentToolArgs.Reset()
		state.ContentBlockOpen = true
		state.CurrentBlockType = "tool_use"
		state.Blocks = append(state.Blocks, AnthropicContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage("{}"),
		})
		blockIdx = len(state.Blocks) - 1
		state.ToolIndexToBlockID[idx] = blockIdx
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_start",
			Index: intPtr(blockIdx),
			ContentBlock: &AnthropicContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: json.RawMessage("{}"),
			},
		})
	}
	if tc.Function.Arguments != "" {
		_, _ = state.CurrentToolArgs.WriteString(tc.Function.Arguments)
		block := state.Blocks[blockIdx]
		block.Input = json.RawMessage(state.CurrentToolArgs.String())
		state.Blocks[blockIdx] = block
		events = append(events, AnthropicStreamEvent{
			Type:  "content_block_delta",
			Index: intPtr(blockIdx),
			Delta: &AnthropicDelta{
				Type:        "input_json_delta",
				PartialJSON: tc.Function.Arguments,
			},
		})
	}
	state.SawToolCall()
	return events
}

func (state *ChatEventToAnthropicState) SawToolCall() {
	// Marker method so the final stop reason can be derived from tool-call
	// streams even when the upstream omits an explicit finish_reason.
	if state != nil && state.PendingStopReason == "" {
		state.PendingStopReason = "tool_use"
	}
}

func chatAnthropicCloseCurrentBlock(state *ChatEventToAnthropicState) []AnthropicStreamEvent {
	if state == nil || !state.ContentBlockOpen {
		return nil
	}
	idx := state.ContentBlockIndex()
	state.ContentBlockOpen = false
	state.CurrentBlockType = ""
	state.CurrentToolName = ""
	state.CurrentToolArgs.Reset()
	return []AnthropicStreamEvent{{Type: "content_block_stop", Index: intPtr(idx)}}
}

func chatChunkUsageOnlyToAnthropicEvents(chunk *ChatCompletionsChunk, state *ChatEventToAnthropicState) []AnthropicStreamEvent {
	if chunk == nil || state == nil || chunk.Usage == nil {
		return nil
	}
	usage := chatAnthropicUsageFromChatUsage(chunk.Usage)
	state.InputTokens = usage.InputTokens
	state.OutputTokens = usage.OutputTokens
	state.CacheReadInputTokens = usage.CacheReadInputTokens
	if state.PendingStopReason != "" && !state.MessageStopSent {
		return chatAnthropicFinalizeMessage(state)
	}
	return nil
}

func (state *ChatEventToAnthropicState) ContentBlockIndex() int {
	if state == nil {
		return 0
	}
	return len(state.Blocks) - 1
}

func appendAnthropicTextBlock(state *ChatEventToAnthropicState, text string) {
	if state == nil || len(state.Blocks) == 0 {
		return
	}
	idx := len(state.Blocks) - 1
	block := state.Blocks[idx]
	block.Text += text
	state.Blocks[idx] = block
}

func appendAnthropicThinkingBlock(state *ChatEventToAnthropicState, text string) {
	if state == nil || len(state.Blocks) == 0 {
		return
	}
	idx := len(state.Blocks) - 1
	block := state.Blocks[idx]
	block.Thinking += text
	state.Blocks[idx] = block
}

func intPtr(v int) *int {
	return &v
}
