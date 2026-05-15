//go:build unit

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
)

var openAIChallengeRegex = regexp.MustCompile(`Q:\s*(\d+)\s*([+-])\s*(\d+)\s*=\s*\?`)

type openAICaptureHandler struct {
	lastPath    string
	lastBody    map[string]any
	lastHeaders http.Header
	status      int
}

func (h *openAICaptureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.lastPath = r.URL.Path
	h.lastHeaders = r.Header.Clone()
	defer func() { _ = r.Body.Close() }()

	var parsed map[string]any
	_ = json.NewDecoder(r.Body).Decode(&parsed)
	h.lastBody = parsed

	if h.status == 0 {
		h.status = http.StatusOK
	}

	answer := solveMonitorChallengeFromRequestBody(extractMonitorPrompt(r.URL.Path, parsed))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(h.status)

	if r.URL.Path == providerOpenAIResponsesPath {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": parsed["model"],
			"output": []map[string]any{
				{
					"type": "message",
					"content": []map[string]any{
						{
							"type": "output_text",
							"text": answer,
						},
					},
				},
			},
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"choices": []map[string]any{
			{
				"message": map[string]any{
					"content": answer,
				},
			},
		},
	})
}

func setupFakeOpenAI(t *testing.T, handler *openAICaptureHandler) string {
	t.Helper()
	swapMonitorHTTPClient(t)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func solveMonitorChallengeFromRequestBody(prompt string) string {
	matches := openAIChallengeRegex.FindAllStringSubmatch(prompt, -1)
	if len(matches) == 0 {
		return ""
	}

	last := matches[len(matches)-1]
	left, err := strconv.Atoi(last[1])
	if err != nil {
		return ""
	}
	right, err := strconv.Atoi(last[3])
	if err != nil {
		return ""
	}

	if last[2] == "+" {
		return strconv.Itoa(left + right)
	}
	return strconv.Itoa(left - right)
}

func extractMonitorPrompt(path string, body map[string]any) string {
	switch path {
	case providerOpenAIResponsesPath:
		input, ok := body["input"].([]any)
		if !ok || len(input) == 0 {
			return ""
		}
		message, ok := input[0].(map[string]any)
		if !ok {
			return ""
		}
		content, ok := message["content"].([]any)
		if !ok || len(content) == 0 {
			return ""
		}
		part, ok := content[0].(map[string]any)
		if !ok {
			return ""
		}
		text, _ := part["text"].(string)
		return text
	default:
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) == 0 {
			return ""
		}
		message, ok := messages[0].(map[string]any)
		if !ok {
			return ""
		}
		text, _ := message["content"].(string)
		return text
	}
}

func TestRunCheckForModel_OpenAINonCodex_UsesChatCompletionsPath(t *testing.T) {
	handler := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, handler)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-fake", "gpt-4.1", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("expected operational status, got status=%s message=%q", res.Status, res.Message)
	}
	if handler.lastPath != providerOpenAIPath {
		t.Fatalf("expected path %q, got %q", providerOpenAIPath, handler.lastPath)
	}
	if _, ok := handler.lastBody["messages"]; !ok {
		t.Fatal("expected chat completions payload to contain messages")
	}
	if _, ok := handler.lastBody["input"]; ok {
		t.Fatal("chat completions payload should not contain input")
	}
	if _, ok := handler.lastBody["instructions"]; ok {
		t.Fatal("chat completions payload should not contain instructions by default")
	}
	if got := handler.lastHeaders.Get("Authorization"); got != "Bearer sk-fake" {
		t.Fatalf("expected Authorization header to be preserved, got %q", got)
	}
}

func TestRunCheckForModel_OpenAICodexAlias_UsesResponsesPath(t *testing.T) {
	handler := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, handler)

	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-fake", "gpt-5.3", nil)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("expected operational status, got status=%s message=%q", res.Status, res.Message)
	}
	if handler.lastPath != providerOpenAIResponsesPath {
		t.Fatalf("expected path %q, got %q", providerOpenAIResponsesPath, handler.lastPath)
	}
	if _, ok := handler.lastBody["messages"]; ok {
		t.Fatal("responses payload should not contain messages")
	}
	if _, ok := handler.lastBody["input"]; !ok {
		t.Fatal("responses payload should contain input")
	}
	instructions, _ := handler.lastBody["instructions"].(string)
	if instructions == "" {
		t.Fatal("responses payload should contain default instructions")
	}
	if got := handler.lastHeaders.Get("Authorization"); got != "Bearer sk-fake" {
		t.Fatalf("expected Authorization header to be preserved, got %q", got)
	}
}

func TestRunCheckForModel_OpenAICodexMergeMode_ProtectsInputAndAllowsInstructionsOverride(t *testing.T) {
	handler := &openAICaptureHandler{}
	endpoint := setupFakeOpenAI(t, handler)

	opts := &CheckOptions{
		BodyOverrideMode: MonitorBodyOverrideModeMerge,
		BodyOverride: map[string]any{
			"instructions": "You are a custom codex monitor.",
			"input":        "hacked-input",
		},
	}
	res := runCheckForModel(context.Background(), MonitorProviderOpenAI, endpoint, "sk-fake", "gpt-5.3-codex", opts)

	if res.Status != MonitorStatusOperational {
		t.Fatalf("expected operational status, got status=%s message=%q", res.Status, res.Message)
	}
	if handler.lastPath != providerOpenAIResponsesPath {
		t.Fatalf("expected path %q, got %q", providerOpenAIResponsesPath, handler.lastPath)
	}
	if got := handler.lastBody["instructions"]; got != "You are a custom codex monitor." {
		t.Fatalf("expected instructions override to win, got %v", got)
	}
	if _, ok := handler.lastBody["input"].(string); ok {
		t.Fatalf("expected input to remain structured, got %T", handler.lastBody["input"])
	}
	input, ok := handler.lastBody["input"].([]any)
	if !ok || len(input) == 0 {
		t.Fatalf("expected protected structured input, got %T", handler.lastBody["input"])
	}
	if prompt := extractMonitorPrompt(handler.lastPath, handler.lastBody); prompt == "" {
		t.Fatal("expected structured input to retain challenge prompt")
	}
}
