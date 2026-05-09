package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

var DefaultNvidiaModels = []openai.Model{
	{ID: defaultNvidiaTextTestModel, Object: "model", Type: "model", OwnedBy: "nvidia", DisplayName: defaultNvidiaTextTestModel},
	{ID: "meta/llama-3.3-70b-instruct", Object: "model", Type: "model", OwnedBy: "nvidia", DisplayName: "meta/llama-3.3-70b-instruct"},
	{ID: "meta/llama-3.1-70b-instruct", Object: "model", Type: "model", OwnedBy: "nvidia", DisplayName: "meta/llama-3.1-70b-instruct"},
	{ID: "nvidia/llama-3.1-nemotron-70b-instruct", Object: "model", Type: "model", OwnedBy: "nvidia", DisplayName: "nvidia/llama-3.1-nemotron-70b-instruct"},
}

func buildOpenAIModelsURL(base string) string {
	normalized := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(normalized, "/models") {
		return normalized
	}
	if strings.HasSuffix(normalized, "/v1") {
		return normalized + "/models"
	}
	return normalized + "/v1/models"
}

func NvidiaModelForID(id string) openai.Model {
	trimmed := strings.TrimSpace(id)
	for _, model := range DefaultNvidiaModels {
		if model.ID == trimmed {
			return model
		}
	}
	return openai.Model{
		ID:          trimmed,
		Object:      "model",
		Type:        "model",
		OwnedBy:     "nvidia",
		DisplayName: trimmed,
	}
}

func NvidiaModelsFromMapping(mapping map[string]string) []openai.Model {
	if len(mapping) == 0 {
		return append([]openai.Model(nil), DefaultNvidiaModels...)
	}
	keys := make([]string, 0, len(mapping))
	for requestedModel := range mapping {
		if strings.TrimSpace(requestedModel) != "" {
			keys = append(keys, requestedModel)
		}
	}
	sort.Strings(keys)
	models := make([]openai.Model, 0, len(keys))
	for _, id := range keys {
		models = append(models, NvidiaModelForID(id))
	}
	return models
}

// FetchNvidiaModels calls NVIDIA's OpenAI-compatible model list endpoint.
// Official NVIDIA NIM endpoint shape is GET {base}/models with Bearer API key.
func (s *AccountTestService) FetchNvidiaModels(ctx context.Context, account *Account, baseURLOverride string, apiKeyOverride string) ([]openai.Model, error) {
	if s == nil || s.httpUpstream == nil {
		return nil, errors.New("account test service is not configured")
	}
	if account != nil && account.Platform != PlatformNvidia {
		return nil, fmt.Errorf("account platform is not NVIDIA: %s", account.Platform)
	}

	apiKey := strings.TrimSpace(apiKeyOverride)
	if apiKey == "" && account != nil {
		apiKey = account.GetOpenAIApiKey()
	}
	if apiKey == "" {
		return nil, errors.New("api_key is required")
	}

	baseURL := strings.TrimSpace(baseURLOverride)
	if baseURL == "" && account != nil {
		baseURL = account.GetOpenAIBaseURL()
	}
	if baseURL == "" {
		baseURL = "https://integrate.api.nvidia.com/v1"
	}
	normalizedBaseURL, err := s.validateUpstreamBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base_url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buildOpenAIModelsURL(normalizedBaseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	proxyURL := ""
	accountID := int64(0)
	accountConcurrency := 0
	if account != nil {
		accountID = account.ID
		accountConcurrency = account.Concurrency
		if account.ProxyID != nil && account.Proxy != nil {
			proxyURL = account.Proxy.URL()
		}
	}

	var tlsProfile *tlsfingerprint.Profile
	if s.tlsFPProfileService != nil {
		tlsProfile = s.tlsFPProfileService.ResolveTLSProfile(account)
	}
	resp, err := s.httpUpstream.DoWithTLS(req, proxyURL, accountID, accountConcurrency, tlsProfile)
	if err != nil {
		return nil, fmt.Errorf("request NVIDIA models failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read NVIDIA models response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(extractUpstreamErrorMessage(body))
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("NVIDIA models API returned %d: %s", resp.StatusCode, message)
	}

	models, err := parseNvidiaModelsResponse(body)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("NVIDIA models API returned no models")
	}
	return models, nil
}

func parseNvidiaModelsResponse(body []byte) ([]openai.Model, error) {
	var list struct {
		Data []openai.Model `json:"data"`
	}
	if err := json.Unmarshal(body, &list); err == nil && len(list.Data) > 0 {
		return normalizeNvidiaModels(list.Data), nil
	}

	var direct []openai.Model
	if err := json.Unmarshal(body, &direct); err != nil {
		return nil, fmt.Errorf("parse NVIDIA models response: %w", err)
	}
	return normalizeNvidiaModels(direct), nil
}

func normalizeNvidiaModels(input []openai.Model) []openai.Model {
	seen := make(map[string]bool, len(input))
	models := make([]openai.Model, 0, len(input))
	for _, model := range input {
		id := strings.TrimSpace(model.ID)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		model.ID = id
		if model.Object == "" {
			model.Object = "model"
		}
		if model.Type == "" {
			model.Type = "model"
		}
		if model.OwnedBy == "" {
			model.OwnedBy = "nvidia"
		}
		if model.DisplayName == "" {
			model.DisplayName = id
		}
		models = append(models, model)
	}
	return models
}
