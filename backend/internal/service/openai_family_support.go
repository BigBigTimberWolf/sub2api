package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

func normalizeOpenAICompatRoutingPlatform(platform string) string {
	switch strings.TrimSpace(platform) {
	case PlatformOpenAI:
		return PlatformOpenAI
	case PlatformNvidia:
		return PlatformNvidia
	default:
		return ""
	}
}

func openAICompatRequestedModelFamily(requestedModel string) string {
	trimmed := strings.TrimSpace(requestedModel)
	if trimmed == "" {
		return ""
	}

	normalized := strings.TrimSpace(NormalizeOpenAICompatRequestedModel(trimmed))
	if normalized == "" {
		return ""
	}

	lower := strings.ToLower(normalized)
	switch {
	case isOpenAIImageGenerationModel(lower):
		return PlatformOpenAI
	case strings.HasPrefix(lower, "meta/"), strings.HasPrefix(lower, "nvidia/"):
		return PlatformNvidia
	}

	modelID := strings.ToLower(lastOpenAIModelSegment(normalized))
	if modelID == "" {
		return ""
	}
	if strings.HasPrefix(modelID, "gpt-") {
		return PlatformOpenAI
	}
	if strings.HasPrefix(modelID, "gpt") {
		if strings.HasPrefix(normalizeCodexModel(modelID), "gpt-") {
			return PlatformOpenAI
		}
	}

	return ""
}

func openAICompatKnownPlatformForModel(requestedModel string) string {
	normalized := strings.TrimSpace(NormalizeOpenAICompatRequestedModel(strings.TrimSpace(requestedModel)))
	if normalized == "" {
		return ""
	}

	lower := strings.ToLower(normalized)
	switch {
	case isOpenAIImageGenerationModel(lower):
		return PlatformOpenAI
	case strings.HasPrefix(lower, "meta/"), strings.HasPrefix(lower, "nvidia/"):
		return PlatformNvidia
	}

	for _, model := range openai.DefaultModels {
		if strings.EqualFold(model.ID, normalized) {
			return PlatformOpenAI
		}
	}
	for _, model := range DefaultNvidiaModels {
		if strings.EqualFold(model.ID, normalized) {
			return PlatformNvidia
		}
	}

	return openAICompatRequestedModelFamily(normalized)
}

func openAICompatAccountHasExplicitModelSupport(account *Account, requestedModel string) bool {
	if account == nil {
		return false
	}
	if len(account.GetModelMapping()) == 0 {
		return false
	}
	return account.IsModelSupported(requestedModel)
}

func inferOpenAICompatPlatformFromAccounts(accounts []Account, requestedModel string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ""
	}

	hasOpenAIExplicitSupport := false
	hasNvidiaExplicitSupport := false
	for i := range accounts {
		account := &accounts[i]
		switch account.Platform {
		case PlatformOpenAI:
			if openAICompatAccountHasExplicitModelSupport(account, requestedModel) {
				hasOpenAIExplicitSupport = true
			}
		case PlatformNvidia:
			if openAICompatAccountHasExplicitModelSupport(account, requestedModel) {
				hasNvidiaExplicitSupport = true
			}
		}
	}

	if hasOpenAIExplicitSupport != hasNvidiaExplicitSupport {
		if hasOpenAIExplicitSupport {
			return PlatformOpenAI
		}
		return PlatformNvidia
	}

	return openAICompatKnownPlatformForModel(requestedModel)
}

func resolveOpenAICompatForcedPlatform(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	forcedPlatform, ok := ctx.Value(ctxkey.ForcePlatform).(string)
	if !ok {
		return ""
	}
	return normalizeOpenAICompatRoutingPlatform(forcedPlatform)
}

func isOpenAICompatAccountModelSupported(account *Account, requestedModel string) bool {
	return isOpenAICompatAccountModelSupportedForSelection(nil, account, requestedModel, "")
}

func isOpenAICompatAccountModelSupportedForSelection(ctx context.Context, account *Account, requestedModel string, platform string) bool {
	if account == nil {
		return false
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return true
	}

	strictPlatform := resolveOpenAICompatForcedPlatform(ctx)
	if strictPlatform == "" && normalizeOpenAICompatRoutingPlatform(platform) == PlatformNvidia {
		strictPlatform = PlatformNvidia
	}
	if strictPlatform != "" {
		if account.Platform != strictPlatform {
			return false
		}
		if len(account.GetModelMapping()) > 0 {
			return account.IsModelSupported(requestedModel)
		}
		return true
	}

	if len(account.GetModelMapping()) > 0 {
		return account.IsModelSupported(requestedModel)
	}

	switch openAICompatKnownPlatformForModel(requestedModel) {
	case "":
		return true
	case PlatformOpenAI:
		return account.Platform == PlatformOpenAI
	case PlatformNvidia:
		return account.Platform == PlatformNvidia
	default:
		return true
	}
}
