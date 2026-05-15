package service

import "strings"

const (
	defaultOpenAIMessagesDispatchOpusMappedModel   = "gpt-5.4"
	defaultOpenAIMessagesDispatchSonnetMappedModel = "gpt-5.3-codex"
	defaultOpenAIMessagesDispatchHaikuMappedModel  = "gpt-5.4-mini"
)

type OpenAICompatRoutingTarget struct {
	Model    string
	Platform string
}

func normalizeOpenAIMessagesDispatchMappedModel(model string) string {
	model = NormalizeOpenAICompatRequestedModel(strings.TrimSpace(model))
	return strings.TrimSpace(model)
}

func normalizeOpenAIMessagesDispatchMappedPlatform(platform string) string {
	return normalizeOpenAICompatRoutingPlatform(platform)
}

func normalizeOpenAIMessagesDispatchModelConfig(cfg OpenAIMessagesDispatchModelConfig) OpenAIMessagesDispatchModelConfig {
	out := OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:      normalizeOpenAIMessagesDispatchMappedModel(cfg.OpusMappedModel),
		OpusMappedPlatform:   normalizeOpenAIMessagesDispatchMappedPlatform(cfg.OpusMappedPlatform),
		SonnetMappedModel:    normalizeOpenAIMessagesDispatchMappedModel(cfg.SonnetMappedModel),
		SonnetMappedPlatform: normalizeOpenAIMessagesDispatchMappedPlatform(cfg.SonnetMappedPlatform),
		HaikuMappedModel:     normalizeOpenAIMessagesDispatchMappedModel(cfg.HaikuMappedModel),
		HaikuMappedPlatform:  normalizeOpenAIMessagesDispatchMappedPlatform(cfg.HaikuMappedPlatform),
	}

	if len(cfg.ExactModelMappings) > 0 {
		out.ExactModelMappings = make(map[string]string, len(cfg.ExactModelMappings))
		for requestedModel, mappedModel := range cfg.ExactModelMappings {
			requestedModel = strings.TrimSpace(requestedModel)
			mappedModel = normalizeOpenAIMessagesDispatchMappedModel(mappedModel)
			if requestedModel == "" || mappedModel == "" {
				continue
			}
			out.ExactModelMappings[requestedModel] = mappedModel
		}
		if len(out.ExactModelMappings) == 0 {
			out.ExactModelMappings = nil
		}
	}
	if len(cfg.ExactModelMappingPlatforms) > 0 {
		out.ExactModelMappingPlatforms = make(map[string]string, len(cfg.ExactModelMappingPlatforms))
		for requestedModel, platform := range cfg.ExactModelMappingPlatforms {
			requestedModel = strings.TrimSpace(requestedModel)
			platform = normalizeOpenAIMessagesDispatchMappedPlatform(platform)
			if requestedModel == "" || platform == "" {
				continue
			}
			if out.ExactModelMappings != nil {
				if _, exists := out.ExactModelMappings[requestedModel]; !exists {
					continue
				}
			}
			out.ExactModelMappingPlatforms[requestedModel] = platform
		}
		if len(out.ExactModelMappingPlatforms) == 0 {
			out.ExactModelMappingPlatforms = nil
		}
	}

	return out
}

func claudeMessagesDispatchFamily(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if !strings.HasPrefix(normalized, "claude") {
		return ""
	}
	switch {
	case strings.Contains(normalized, "opus"):
		return "opus"
	case strings.Contains(normalized, "sonnet"):
		return "sonnet"
	case strings.Contains(normalized, "haiku"):
		return "haiku"
	default:
		return ""
	}
}

func (g *Group) ResolveMessagesDispatchTarget(requestedModel string) OpenAICompatRoutingTarget {
	if g == nil {
		return OpenAICompatRoutingTarget{}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return OpenAICompatRoutingTarget{}
	}

	cfg := normalizeOpenAIMessagesDispatchModelConfig(g.MessagesDispatchModelConfig)
	if mappedModel := strings.TrimSpace(cfg.ExactModelMappings[requestedModel]); mappedModel != "" {
		return OpenAICompatRoutingTarget{
			Model:    mappedModel,
			Platform: strings.TrimSpace(cfg.ExactModelMappingPlatforms[requestedModel]),
		}
	}

	switch claudeMessagesDispatchFamily(requestedModel) {
	case "opus":
		if mappedModel := strings.TrimSpace(cfg.OpusMappedModel); mappedModel != "" {
			return OpenAICompatRoutingTarget{
				Model:    mappedModel,
				Platform: cfg.OpusMappedPlatform,
			}
		}
		return OpenAICompatRoutingTarget{
			Model:    defaultOpenAIMessagesDispatchOpusMappedModel,
			Platform: cfg.OpusMappedPlatform,
		}
	case "sonnet":
		if mappedModel := strings.TrimSpace(cfg.SonnetMappedModel); mappedModel != "" {
			return OpenAICompatRoutingTarget{
				Model:    mappedModel,
				Platform: cfg.SonnetMappedPlatform,
			}
		}
		return OpenAICompatRoutingTarget{
			Model:    defaultOpenAIMessagesDispatchSonnetMappedModel,
			Platform: cfg.SonnetMappedPlatform,
		}
	case "haiku":
		if mappedModel := strings.TrimSpace(cfg.HaikuMappedModel); mappedModel != "" {
			return OpenAICompatRoutingTarget{
				Model:    mappedModel,
				Platform: cfg.HaikuMappedPlatform,
			}
		}
		return OpenAICompatRoutingTarget{
			Model:    defaultOpenAIMessagesDispatchHaikuMappedModel,
			Platform: cfg.HaikuMappedPlatform,
		}
	default:
		return OpenAICompatRoutingTarget{}
	}
}

func (g *Group) ResolveMessagesDispatchModel(requestedModel string) string {
	return strings.TrimSpace(g.ResolveMessagesDispatchTarget(requestedModel).Model)
}

func sanitizeGroupMessagesDispatchFields(g *Group) {
	if g == nil || g.Platform == PlatformOpenAI {
		return
	}
	g.AllowMessagesDispatch = false
	g.DefaultMappedModel = ""
	g.MessagesDispatchModelConfig = OpenAIMessagesDispatchModelConfig{}
}
