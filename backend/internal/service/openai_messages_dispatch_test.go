package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	t.Parallel()

	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:      " gpt-5.4-high ",
		SonnetMappedModel:    "gpt-5.3-codex",
		SonnetMappedPlatform: " nvidia ",
		HaikuMappedModel:     " gpt-5.4-mini-medium ",
		ExactModelMappings: map[string]string{
			" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			"":                             "gpt-5.4",
			"claude-opus-4-6":              " ",
		},
		ExactModelMappingPlatforms: map[string]string{
			" claude-sonnet-4-5-20250929 ": " openai ",
			"claude-opus-4-6":              "nvidia",
			"":                             "openai",
		},
	})

	require.Equal(t, "gpt-5.4", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, PlatformNvidia, cfg.SonnetMappedPlatform)
	require.Equal(t, "gpt-5.4-mini", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": "gpt-5.2",
	}, cfg.ExactModelMappings)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": PlatformOpenAI,
	}, cfg.ExactModelMappingPlatforms)
}

func TestGroupResolveMessagesDispatchTarget_PreservesMappedPlatform(t *testing.T) {
	t.Parallel()

	group := &Group{
		MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
			SonnetMappedModel:    " gpt-5.4-high ",
			SonnetMappedPlatform: " nvidia ",
			ExactModelMappings: map[string]string{
				"claude-opus-4-6": " gpt-5.4-high ",
			},
			ExactModelMappingPlatforms: map[string]string{
				"claude-opus-4-6": " nvidia ",
			},
		},
	}

	exactTarget := group.ResolveMessagesDispatchTarget("claude-opus-4-6")
	require.Equal(t, "gpt-5.4", exactTarget.Model)
	require.Equal(t, PlatformNvidia, exactTarget.Platform)

	familyTarget := group.ResolveMessagesDispatchTarget("claude-sonnet-4-5-20250929")
	require.Equal(t, "gpt-5.4", familyTarget.Model)
	require.Equal(t, PlatformNvidia, familyTarget.Platform)
}
