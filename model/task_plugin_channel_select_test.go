package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskPluginChannelSelectionFiltersBothCachePaths(t *testing.T) {
	truncateTables(t)
	priority := int64(0)
	weight := uint(1)
	baseURL := "https://example.com"
	alphaSetting := `{"task_plugin_key":"alpha"}`
	betaSetting := `{"task_plugin_key":"beta"}`
	channels := []Channel{
		{Id: 900001, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "alpha", Models: "shared", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &alphaSetting},
		{Id: 900002, Type: constant.ChannelTypeTaskPlugin, Status: common.ChannelStatusEnabled, Name: "beta", Models: "shared", Group: "default", Priority: &priority, Weight: &weight, BaseURL: &baseURL, Setting: &betaSetting},
		{Id: 900003, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "ordinary", Models: "ordinary", Group: "default", Priority: &priority, Weight: &weight},
		{Id: 900004, Type: constant.ChannelTypeKling, Status: common.ChannelStatusEnabled, Name: "legacy-alpha", Models: "legacy", Group: "default", Priority: &priority, Weight: &weight},
		{Id: 900005, Type: constant.ChannelTypeJimeng, Status: common.ChannelStatusEnabled, Name: "legacy-beta", Models: "legacy", Group: "default", Priority: &priority, Weight: &weight},
	}
	for i := range channels {
		require.NoError(t, channels[i].Insert())
	}

	selected, err := GetChannel("default", "shared", 0, identityFilters("alpha", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "alpha", selected.Name)
	selected, err = GetChannel("default", "shared", 0, identityFilters("", nil))
	require.NoError(t, err)
	assert.Nil(t, selected)
	selected, err = GetChannel("default", "ordinary", 0, identityFilters("", nil))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "ordinary", selected.Name)
	selected, err = GetChannel("default", "legacy", 0, identityFilters("legacy-alpha", []int{constant.ChannelTypeKling}))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "legacy-alpha", selected.Name)
	selected, err = GetChannel("default", "legacy", 0, identityFilters("legacy-alpha", []int{constant.ChannelTypeKling, constant.ChannelTypeJimeng}))
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Contains(t, []string{"legacy-alpha", "legacy-beta"}, selected.Name)
}

func TestUserChannelExclusionFiltersBothCachePaths(t *testing.T) {
	truncateTables(t)
	priority := int64(0)
	weight := uint(1)
	channels := []Channel{
		{Id: 920001, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "alpha", Models: "shared-user-model", Group: "default", Priority: &priority, Weight: &weight},
		{Id: 920002, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Name: "beta", Models: "shared-user-model", Group: "default", Priority: &priority, Weight: &weight},
	}
	for i := range channels {
		require.NoError(t, channels[i].Insert())
	}
	excludeAlpha := []dto.ChannelFilter{{Kind: dto.FilterExcludeChannelIds, ExcludeChannelIds: []int{920001}}}
	excludeAll := []dto.ChannelFilter{{Kind: dto.FilterExcludeChannelIds, ExcludeChannelIds: []int{920001, 920002}}}

	previousMemoryCache := common.MemoryCacheEnabled
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	// Database selection path (memory cache disabled).
	common.MemoryCacheEnabled = false
	selected, err := GetChannel("default", "shared-user-model", 0, excludeAlpha)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "beta", selected.Name)

	selected, err = GetChannel("default", "shared-user-model", 0, excludeAll)
	require.ErrorIs(t, err, ErrUserChannelsExhausted)
	assert.Nil(t, selected)

	selected, err = GetChannel("default", "shared-user-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)

	// In-memory cache selection path.
	common.MemoryCacheEnabled = true
	InitChannelCache()
	selected, err = GetRandomSatisfiedChannel("default", "shared-user-model", 0, excludeAlpha)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "beta", selected.Name)

	selected, err = GetRandomSatisfiedChannel("default", "shared-user-model", 0, excludeAll)
	require.ErrorIs(t, err, ErrUserChannelsExhausted)
	assert.Nil(t, selected)

	selected, err = GetRandomSatisfiedChannel("default", "shared-user-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
}

func identityFilters(key string, channelTypes []int) []dto.ChannelFilter {
	return []dto.ChannelFilter{{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          key,
		TaskPluginChannelTypes: channelTypes,
	}}
}
