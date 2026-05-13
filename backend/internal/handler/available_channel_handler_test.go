//go:build unit

package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUserAvailableChannel_Unauthenticated401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AvailableChannelHandler{}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/channels/available", nil)

	h.List(c)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestFilterUserVisibleGroups_IntersectionOnly(t *testing.T) {
	groups := []service.AvailableGroupRef{
		{ID: 1, Name: "g1", Platform: "anthropic"},
		{ID: 2, Name: "g2", Platform: "anthropic"},
		{ID: 3, Name: "g3", Platform: "openai"},
	}
	allowed := map[int64]struct{}{1: {}, 3: {}}

	visible := filterUserVisibleGroups(groups, allowed)
	require.Len(t, visible, 2)
	ids := []int64{visible[0].ID, visible[1].ID}
	require.ElementsMatch(t, []int64{1, 3}, ids)
	require.Equal(t, "anthropic", visible[0].Platform)
	require.Equal(t, "openai", visible[1].Platform)
}

func TestToUserSupportedModels_FiltersByAllowedPlatforms(t *testing.T) {
	src := []service.SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic", Pricing: nil},
		{Name: "gpt-4o", Platform: "openai", Pricing: nil},
	}
	allowed := map[string]struct{}{"anthropic": {}}
	out := toUserSupportedModels(src, allowed)
	require.Len(t, out, 1)
	require.Equal(t, "claude-sonnet-4-6", out[0].Name)
	require.Equal(t, "anthropic", out[0].Platform)
}

func TestToUserSupportedModels_NilAllowedPlatformsKeepsAll(t *testing.T) {
	src := []service.SupportedModel{
		{Name: "a", Platform: "anthropic"},
		{Name: "b", Platform: "openai"},
	}
	require.Len(t, toUserSupportedModels(src, nil), 2)
}

func TestFilterSupportedModelsByVisibleGroups_FiltersByPlatformWithoutExposingPlatform(t *testing.T) {
	visible := []userAvailableGroup{
		{ID: 1, Name: "g-openai", Platform: "openai"},
		{ID: 2, Name: "g-ant", Platform: "anthropic"},
	}
	src := []service.SupportedModel{
		{Name: "claude-sonnet-4-6", Platform: "anthropic"},
		{Name: "gpt-4o", Platform: "openai"},
		{Name: "gemini-2.5-pro", Platform: "gemini"},
	}
	out := filterSupportedModelsByVisibleGroups(src, visible)
	require.Len(t, out, 2)
	require.Equal(t, "claude-sonnet-4-6", out[0].Name)
	require.Equal(t, "gpt-4o", out[1].Name)
	require.Equal(t, "anthropic", out[0].Platform)
	require.Equal(t, "openai", out[1].Platform)
}

func TestUserAvailableChannel_FieldWhitelist(t *testing.T) {
	row := userAvailableChannel{
		Name:            "ch",
		Description:     "d",
		Groups:          []userAvailableGroup{{ID: 1, Name: "g1", Platform: "anthropic"}},
		SupportedModels: []userSupportedModel{{Name: "claude-sonnet-4-6", Platform: "anthropic"}},
	}
	raw, err := json.Marshal(row)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	for _, key := range []string{"id", "status", "billing_model_source", "restrict_models", "platform", "platforms"} {
		_, exists := decoded[key]
		require.Falsef(t, exists, "user DTO must not expose %q", key)
	}
	for _, key := range []string{"name", "description", "groups", "supported_models"} {
		_, exists := decoded[key]
		require.Truef(t, exists, "user DTO must expose %q", key)
	}

	rawGroup, err := json.Marshal(row.Groups[0])
	require.NoError(t, err)
	var groupDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawGroup, &groupDecoded))
	for _, key := range []string{"id", "name", "subscription_type", "rate_multiplier", "is_exclusive"} {
		_, exists := groupDecoded[key]
		require.Truef(t, exists, "group DTO must expose %q", key)
	}
	_, hasGroupPlatform := groupDecoded["platform"]
	require.False(t, hasGroupPlatform, "group DTO must not expose platform")

	rawModel, err := json.Marshal(row.SupportedModels[0])
	require.NoError(t, err)
	var modelDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawModel, &modelDecoded))
	for _, key := range []string{"name", "pricing"} {
		_, exists := modelDecoded[key]
		require.Truef(t, exists, "model DTO must expose %q", key)
	}
	_, hasModelPlatform := modelDecoded["platform"]
	require.False(t, hasModelPlatform, "model DTO must not expose platform")

	pricing := toUserPricing(&service.ChannelModelPricing{
		BillingMode: service.BillingModeToken,
		Intervals: []service.PricingInterval{
			{ID: 7, MinTokens: 0, MaxTokens: nil, SortOrder: 3},
		},
	})
	require.NotNil(t, pricing)
	require.Len(t, pricing.Intervals, 1)
	rawIv, err := json.Marshal(pricing.Intervals[0])
	require.NoError(t, err)
	var ivDecoded map[string]any
	require.NoError(t, json.Unmarshal(rawIv, &ivDecoded))
	for _, key := range []string{"id", "pricing_id", "sort_order"} {
		_, exists := ivDecoded[key]
		require.Falsef(t, exists, "user pricing interval must not expose %q", key)
	}
}
