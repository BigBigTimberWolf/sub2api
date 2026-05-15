package domain

// OpenAIMessagesDispatchModelConfig controls how Anthropic /v1/messages
// requests are mapped onto OpenAI/Codex models.
type OpenAIMessagesDispatchModelConfig struct {
	OpusMappedModel            string            `json:"opus_mapped_model,omitempty"`
	OpusMappedPlatform         string            `json:"opus_mapped_platform,omitempty"`
	SonnetMappedModel          string            `json:"sonnet_mapped_model,omitempty"`
	SonnetMappedPlatform       string            `json:"sonnet_mapped_platform,omitempty"`
	HaikuMappedModel           string            `json:"haiku_mapped_model,omitempty"`
	HaikuMappedPlatform        string            `json:"haiku_mapped_platform,omitempty"`
	ExactModelMappings         map[string]string `json:"exact_model_mappings,omitempty"`
	ExactModelMappingPlatforms map[string]string `json:"exact_model_mapping_platforms,omitempty"`
}
