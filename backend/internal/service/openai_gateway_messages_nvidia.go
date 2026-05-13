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

// ForwardAsAnthropicViaNvidiaChatCompletions forwards Anthropic /v1/messages to
// NVIDIA-compatible /v1/chat/completions and converts the response back to
// Anthropic shape.
func (s *OpenAIGatewayService) ForwardAsAnthropicViaNvidiaChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	displayModel string,
) (*OpenAIForwardResult, error) {
	startTime := time.Now()

	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	if hasImage, err := hasAnthropicMessageImages(anthropicReq.Messages); err != nil {
		return nil, fmt.Errorf("inspect anthropic messages: %w", err)
	} else if hasImage {
		return nil, fmt.Errorf("images are not supported for this Nvidia messages path")
	}

	requestModel := strings.TrimSpace(anthropicReq.Model)
	if requestModel == "" {
		return nil, fmt.Errorf("model is required")
	}
	originalModel := strings.TrimSpace(displayModel)
	if originalModel == "" {
		originalModel = requestModel
	}
	reqStream := anthropicReq.Stream

	billingModel, upstreamModel := resolveNvidiaMessagesModels(account, requestModel)
	apiKeyID := getAPIKeyIDFromContext(c)
	if promptCacheKey != "" {
		// Keep session isolation aligned with the existing OpenAI-compatible paths.
		promptCacheKey = strings.TrimSpace(promptCacheKey)
	}

	chatReq, err := apicompat.AnthropicToChatCompletions(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("convert anthropic to chat completions: %w", err)
	}
	chatReq.Model = upstreamModel
	if reqStream {
		chatReq.Stream = true
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
			writeAnthropicError(c, http.StatusForbidden, "forbidden_error", blocked.Message)
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
	if reqStream {
		upstreamReq.Header.Set("Accept", "text/event-stream")
	} else {
		upstreamReq.Header.Set("Accept", "application/json")
	}
	if promptCacheKey != "" {
		upstreamReq.Header.Set("session_id", generateSessionUUID(isolateOpenAISessionID(apiKeyID, promptCacheKey)))
	}

	// Keep the same safe header pass-through behavior as raw chat completions.
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
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream request failed")
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
		return s.handleAnthropicErrorResponse(resp, c, account)
	}

	if reqStream {
		return s.handleNvidiaAnthropicStreamingResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
	}
	return s.handleNvidiaAnthropicBufferedResponse(resp, c, originalModel, billingModel, upstreamModel, startTime)
}

func resolveNvidiaMessagesModels(account *Account, requestedModel string) (string, string) {
	mapped := NormalizeOpenAICompatRequestedModel(requestedModel)
	if mapped == "" {
		mapped = strings.TrimSpace(requestedModel)
	}
	if account != nil {
		if candidate, matched := account.ResolveMappedModel(mapped); matched {
			mapped = strings.TrimSpace(candidate)
		}
	}
	if mapped == "" {
		mapped = strings.TrimSpace(requestedModel)
	}
	return mapped, mapped
}

func hasAnthropicMessageImages(messages []apicompat.AnthropicMessage) (bool, error) {
	for _, msg := range messages {
		var blocks []apicompat.AnthropicContentBlock
		if err := json.Unmarshal(msg.Content, &blocks); err != nil {
			continue
		}
		for _, block := range blocks {
			if block.Type == "image" {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *OpenAIGatewayService) handleNvidiaAnthropicBufferedResponse(
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
		if !errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", "Failed to read upstream response")
		}
		return nil, err
	}
	var ccResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(body, &ccResp); err != nil {
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream returned invalid response")
		return nil, fmt.Errorf("parse upstream chat completions response: %w", err)
	}
	anthropicResp := apicompat.ChatCompletionsToAnthropicResponse(&ccResp, originalModel)
	if s.responseHeaderFilter != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}
	c.JSON(http.StatusOK, anthropicResp)
	return &OpenAIForwardResult{
		RequestID:    requestID,
		ResponseID:   ccResp.ID,
		Usage:        copyOpenAIUsageFromChatUsage(ccResp.Usage),
		Model:        originalModel,
		BillingModel: billingModel,
		UpstreamModel: upstreamModel,
		Stream:       false,
		Duration:     time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) handleNvidiaAnthropicStreamingResponse(
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

	state := apicompat.NewChatEventToAnthropicState()
	state.Model = originalModel
	var usage OpenAIUsage
	var firstTokenMs *int
	observedFirstChunk := false
	clientDisconnected := false
	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanner.Buffer(make([]byte, 0, 64*1024), maxLineSize)

	for scanner.Scan() {
		line := scanner.Text()
		if payload, ok := extractOpenAISSEDataLine(line); ok {
			if !observedFirstChunk && strings.TrimSpace(payload) != "[DONE]" {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
				observedFirstChunk = true
			}
			if strings.TrimSpace(payload) == "[DONE]" {
				break
			}
			var chunk apicompat.ChatCompletionsChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				logger.L().Warn("openai messages nvidia stream: failed to parse chunk",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}
			if chunk.Usage != nil {
				usage = copyOpenAIUsageFromChatUsage(chunk.Usage)
			}
			events := apicompat.ChatCompletionsChunkToAnthropicEvents(&chunk, state)
			if !clientDisconnected {
				for _, evt := range events {
					sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
					if err != nil {
						continue
					}
					if _, err := fmt.Fprint(c.Writer, sse); err != nil {
						clientDisconnected = true
						break
					}
				}
			}
			if len(events) > 0 && !clientDisconnected {
				c.Writer.Flush()
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		logger.L().Warn("openai messages nvidia stream: read error",
			zap.Error(err),
			zap.String("request_id", requestID),
		)
	}
	finalEvents := apicompat.FinalizeChatAnthropicStream(state)
	if !clientDisconnected {
		for _, evt := range finalEvents {
			sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprint(c.Writer, sse); err != nil {
				clientDisconnected = true
				break
			}
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}
	return &OpenAIForwardResult{
		RequestID:    requestID,
		ResponseID:   state.ResponseID,
		Usage:        usage,
		Model:        originalModel,
		BillingModel: billingModel,
		UpstreamModel: upstreamModel,
		Stream:       true,
		Duration:     time.Since(startTime),
		FirstTokenMs: firstTokenMs,
	}, nil
}

func copyOpenAIUsageFromChatUsage(usage *apicompat.ChatUsage) OpenAIUsage {
	if usage == nil {
		return OpenAIUsage{}
	}
	result := OpenAIUsage{
		InputTokens:  usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
	}
	if usage.PromptTokensDetails != nil {
		result.CacheReadInputTokens = usage.PromptTokensDetails.CachedTokens
	}
	return result
}
