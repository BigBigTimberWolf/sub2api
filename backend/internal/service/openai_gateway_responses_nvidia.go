package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/util/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// ForwardAsResponsesViaNvidiaChatCompletions forwards OpenAI /v1/responses
// requests to NVIDIA-compatible /v1/chat/completions and converts the
// upstream response back to Responses shape.
func (s *OpenAIGatewayService) ForwardAsResponsesViaNvidiaChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	reqBody map[string]any,
	originalModel string,
	billingModel string,
	upstreamModel string,
	promptCacheKey string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	var responsesReq apicompat.ResponsesRequest
	if err := json.Unmarshal(body, &responsesReq); err != nil {
		return nil, fmt.Errorf("parse responses request: %w", err)
	}

	chatReq, err := responsesRequestToChatCompletions(&responsesReq)
	if err != nil {
		return nil, fmt.Errorf("convert responses to chat completions: %w", err)
	}
	chatReq.Model = upstreamModel
	if chatReq.Stream {
		chatReq.StreamOptions = &apicompat.ChatStreamOptions{IncludeUsage: true}
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("marshal chat completions request: %w", err)
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, chatBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, policyErr
	}
	chatBody = updatedBody

	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	baseURL := account.GetOpenAIBaseURL()
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	validatedURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}
	targetURL := buildOpenAIChatCompletionsURL(validatedURL)

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	upstreamReq, err := http.NewRequestWithContext(upstreamCtx, http.MethodPost, targetURL, bytes.NewReader(chatBody))
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Authorization", "Bearer "+token)
	if chatReq.Stream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	if promptCacheKey != "" {
		apiKeyID := getAPIKeyIDFromContext(c)
		upstreamReq.Header.Set("session_id", generateSessionUUID(isolateOpenAISessionID(apiKeyID, promptCacheKey)))
	}

	for key, values := range c.Request.Header {
		if openaiCCRawAllowedHeaders[strings.ToLower(key)] {
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		upstreamReq.Header.Set("user-agent", customUA)
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "request_error",
			Message:            safeErr,
		})
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "upstream_error",
				"message": "Upstream request failed",
			},
		})
		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(respBody))

		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
		if s.shouldFailoverOpenAIUpstreamResponse(resp.StatusCode, upstreamMsg, respBody) {
			upstreamDetail := ""
			if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
				maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
				if maxBytes <= 0 {
					maxBytes = 2048
				}
				upstreamDetail = truncateString(string(respBody), maxBytes)
			}
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform:           account.Platform,
				AccountID:          account.ID,
				AccountName:        account.Name,
				UpstreamStatusCode: resp.StatusCode,
				UpstreamRequestID:  resp.Header.Get("x-request-id"),
				Kind:               "failover",
				Message:            upstreamMsg,
				Detail:             upstreamDetail,
			})
			if s.rateLimitService != nil {
				s.rateLimitService.HandleUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody)
			}
			return nil, &UpstreamFailoverError{
				StatusCode:             resp.StatusCode,
				ResponseBody:           respBody,
				RetryableOnSameAccount: account.IsPoolMode() && (isPoolModeRetryableStatus(resp.StatusCode) || isOpenAITransientProcessingError(resp.StatusCode, upstreamMsg, respBody)),
			}
		}
		return s.handleResponsesNvidiaErrorResponse(resp, c, account)
	}

	var result *OpenAIForwardResult
	if chatReq.Stream {
		result, err = s.handleNvidiaResponsesStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	} else {
		result, err = s.handleNvidiaResponsesBufferedResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}
	if err != nil {
		return nil, err
	}

	result.ServiceTier = extractOpenAIServiceTier(reqBody)
	result.ReasoningEffort = extractOpenAIReasoningEffort(reqBody, originalModel)
	return result, nil
}

func responsesRequestToChatCompletions(req *apicompat.ResponsesRequest) (*apicompat.ChatCompletionsRequest, error) {
	if req == nil {
		return nil, errors.New("responses request is nil")
	}
	messages, err := responsesInputToChatMessages(req.Instructions, req.Input)
	if err != nil {
		return nil, err
	}
	out := &apicompat.ChatCompletionsRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		ServiceTier: req.ServiceTier,
	}
	if req.MaxOutputTokens != nil && *req.MaxOutputTokens > 0 {
		v := *req.MaxOutputTokens
		out.MaxTokens = &v
	}
	if req.Reasoning != nil && req.Reasoning.Effort != "" {
		out.ReasoningEffort = req.Reasoning.Effort
	}
	if len(req.Tools) > 0 {
		out.Tools = responsesToolsToChatTools(req.Tools)
	}
	if len(req.ToolChoice) > 0 {
		out.ToolChoice = req.ToolChoice
	}
	return out, nil
}

func responsesInputToChatMessages(instructions string, inputRaw json.RawMessage) ([]apicompat.ChatMessage, error) {
	trimmedInstructions := strings.TrimSpace(instructions)
	var messages []apicompat.ChatMessage
	if trimmedInstructions != "" {
		content, err := json.Marshal(trimmedInstructions)
		if err != nil {
			return nil, fmt.Errorf("marshal instructions: %w", err)
		}
		messages = append(messages, apicompat.ChatMessage{Role: "system", Content: content})
	}

	if len(inputRaw) == 0 || string(inputRaw) == "null" {
		if len(messages) == 0 {
			return nil, errors.New("responses input is required")
		}
		return messages, nil
	}

	var inputStr string
	if err := json.Unmarshal(inputRaw, &inputStr); err == nil {
		content, mErr := json.Marshal(inputStr)
		if mErr != nil {
			return nil, fmt.Errorf("marshal string input: %w", mErr)
		}
		messages = append(messages, apicompat.ChatMessage{Role: "user", Content: content})
		return messages, nil
	}

	var items []apicompat.ResponsesInputItem
	if err := json.Unmarshal(inputRaw, &items); err != nil {
		return nil, fmt.Errorf("parse responses input: %w", err)
	}

	for _, item := range items {
		switch item.Type {
		case "function_call":
			messages = append(messages, apicompat.ChatMessage{
				Role: "assistant",
				ToolCalls: []apicompat.ChatToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: apicompat.ChatFunctionCall{
						Name:      item.Name,
						Arguments: normalizeJSONArguments(item.Arguments),
					},
				}},
			})
		case "function_call_output":
			output := item.Output
			if output == "" {
				output = "(empty)"
			}
			content, err := json.Marshal(output)
			if err != nil {
				return nil, fmt.Errorf("marshal function_call_output: %w", err)
			}
			messages = append(messages, apicompat.ChatMessage{
				Role:       "tool",
				ToolCallID: item.CallID,
				Content:    content,
			})
		case "item_reference":
			continue
		default:
			role := normalizeResponsesInputRole(item.Role)
			content, err := normalizeResponsesMessageContentForChat(item.Content, role)
			if err != nil {
				return nil, err
			}
			messages = append(messages, apicompat.ChatMessage{Role: role, Content: content})
		}
	}

	if len(messages) == 0 {
		content, err := json.Marshal("")
		if err != nil {
			return nil, fmt.Errorf("marshal empty input: %w", err)
		}
		messages = append(messages, apicompat.ChatMessage{Role: "user", Content: content})
	}
	return messages, nil
}

func normalizeResponsesInputRole(role string) string {
	role = strings.TrimSpace(role)
	switch role {
	case "system", "developer":
		return "system"
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	default:
		return "user"
	}
}

func normalizeResponsesMessageContentForChat(raw json.RawMessage, role string) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.Marshal("")
	}

	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return json.Marshal(s)
	}

	var parts []apicompat.ResponsesContentPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		chatParts := make([]apicompat.ChatContentPart, 0, len(parts))
		for _, part := range parts {
			switch part.Type {
			case "input_text", "output_text", "text":
				if part.Text != "" {
					chatParts = append(chatParts, apicompat.ChatContentPart{Type: "text", Text: part.Text})
				}
			case "input_image":
				if role != "assistant" && part.ImageURL != "" {
					chatParts = append(chatParts, apicompat.ChatContentPart{
						Type:     "image_url",
						ImageURL: &apicompat.ChatImageURL{URL: part.ImageURL},
					})
				}
			}
		}
		if len(chatParts) == 0 {
			return json.Marshal("")
		}
		if len(chatParts) == 1 && chatParts[0].Type == "text" && chatParts[0].ImageURL == nil {
			return json.Marshal(chatParts[0].Text)
		}
		return json.Marshal(chatParts)
	}

	return raw, nil
}

func responsesToolsToChatTools(tools []apicompat.ResponsesTool) []apicompat.ChatTool {
	out := make([]apicompat.ChatTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Type != "function" || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		out = append(out, apicompat.ChatTool{
			Type: "function",
			Function: &apicompat.ChatFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.Parameters,
				Strict:      tool.Strict,
			},
		})
	}
	return out
}

func normalizeJSONArguments(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	return trimmed
}

func (s *OpenAIGatewayService) handleResponsesNvidiaErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, func(c *gin.Context, statusCode int, errType, message string) {
		c.JSON(statusCode, gin.H{
			"error": gin.H{
				"type":    errType,
				"message": message,
			},
		})
	})
}

func (s *OpenAIGatewayService) handleNvidiaResponsesBufferedResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, err
	}

	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Upstream returned invalid response",
			},
		})
		return nil, fmt.Errorf("parse upstream chat completions response: %w", err)
	}

	responsesResp := chatCompletionsToResponsesResponse(&chatResp, originalModel)
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, responsesResp)

	return &OpenAIForwardResult{
		RequestID:     requestID,
		ResponseID:    responsesResp.ID,
		Usage:         copyOpenAIUsageFromChatUsage(chatResp.Usage),
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        false,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) handleNvidiaResponsesStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	chatState := apicompat.NewChatEventToAnthropicState()
	chatState.Model = originalModel
	responsesState := apicompat.NewAnthropicEventToResponsesState()
	responsesState.Model = originalModel

	var usage OpenAIUsage
	var responseID string
	var firstTokenMs *int
	firstChunk := true
	clientDisconnected := false

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	processChunk := func(payload string) {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		var chunk apicompat.ChatCompletionsChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			logger.L().Warn("openai responses nvidia stream: failed to parse chunk",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return
		}

		if chunk.Usage != nil {
			usage = copyOpenAIUsageFromChatUsage(chunk.Usage)
		}

		anthEvents := apicompat.ChatCompletionsChunkToAnthropicEvents(&chunk, chatState)
		for _, anthEvt := range anthEvents {
			respEvents := apicompat.AnthropicEventToResponsesEvents(&anthEvt, responsesState)
			for _, respEvt := range respEvents {
				if respEvt.Response != nil && respEvt.Response.ID != "" {
					responseID = respEvt.Response.ID
				}
				if respEvt.Response != nil && respEvt.Response.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(respEvt.Response.Usage)
				}
				if !clientDisconnected {
					sse, err := apicompat.ResponsesEventToSSE(respEvt)
					if err != nil {
						logger.L().Warn("openai responses nvidia stream: failed to marshal event",
							zap.Error(err),
							zap.String("request_id", requestID),
						)
						continue
					}
					if _, err := fmt.Fprint(c.Writer, sse); err != nil {
						clientDisconnected = true
						break
					}
				}
			}
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		payload, ok := extractOpenAISSEDataLine(line)
		if !ok {
			continue
		}
		if strings.TrimSpace(payload) == "[DONE]" {
			break
		}
		processChunk(payload)
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.L().Warn("openai responses nvidia stream: read error",
			zap.Error(err),
			zap.String("request_id", requestID),
		)
	}

	finalAnthEvents := apicompat.FinalizeChatAnthropicStream(chatState)
	for _, anthEvt := range finalAnthEvents {
		respEvents := apicompat.AnthropicEventToResponsesEvents(&anthEvt, responsesState)
		for _, respEvt := range respEvents {
			if respEvt.Response != nil && respEvt.Response.ID != "" {
				responseID = respEvt.Response.ID
			}
			if respEvt.Response != nil && respEvt.Response.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(respEvt.Response.Usage)
			}
			if clientDisconnected {
				continue
			}
			sse, err := apicompat.ResponsesEventToSSE(respEvt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
			}
		}
	}
	if !clientDisconnected {
		finalRespEvents := apicompat.FinalizeAnthropicResponsesStream(responsesState)
		for _, respEvt := range finalRespEvents {
			if respEvt.Response != nil && respEvt.Response.ID != "" {
				responseID = respEvt.Response.ID
			}
			if respEvt.Response != nil && respEvt.Response.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(respEvt.Response.Usage)
			}
			sse, err := apicompat.ResponsesEventToSSE(respEvt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
			}
		}
	}
	if !clientDisconnected {
		_, _ = fmt.Fprint(c.Writer, "data: [DONE]\n\n")
		c.Writer.Flush()
	}

	return &OpenAIForwardResult{
		RequestID:     requestID,
		ResponseID:    responseID,
		Usage:         usage,
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        true,
		Duration:      time.Since(startTime),
		FirstTokenMs:  firstTokenMs,
	}, nil
}

func chatCompletionsToResponsesResponse(resp *apicompat.ChatCompletionsResponse, model string) *apicompat.ResponsesResponse {
	anthropicResp := apicompat.ChatCompletionsToAnthropicResponse(resp, model)
	responsesResp := apicompat.AnthropicToResponsesResponse(anthropicResp)
	if model != "" {
		responsesResp.Model = model
	}
	if resp != nil && resp.ID != "" {
		responsesResp.ID = resp.ID
	}
	return responsesResp
}
