package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRuntimeDisableChannelUsesChannelOverrideAsCompleteReplacement(t *testing.T) {
	originalEnabled := common.RuntimeAutomaticDisableChannelEnabled
	originalRanges := operation_setting.RuntimeAutomaticDisableStatusCodeRanges
	originalKeywords := operation_setting.RuntimeAutomaticDisableKeywords
	t.Cleanup(func() {
		common.RuntimeAutomaticDisableChannelEnabled = originalEnabled
		operation_setting.RuntimeAutomaticDisableStatusCodeRanges = originalRanges
		operation_setting.RuntimeAutomaticDisableKeywords = originalKeywords
	})

	common.RuntimeAutomaticDisableChannelEnabled = true
	operation_setting.RuntimeAutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 403}}
	operation_setting.RuntimeAutomaticDisableKeywords = []string{"global balance error"}
	override := dto.ChannelOtherSettings{
		RuntimeAutomaticDisableOverrideEnabled: true,
		RuntimeAutomaticDisableStatusCodes:     "429",
		RuntimeAutomaticDisableKeywords:        "channel balance error",
	}

	require.False(t, ShouldRuntimeDisableChannel(types.NewOpenAIError(errors.New("global balance error"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden), &override))
	require.True(t, ShouldRuntimeDisableChannel(types.NewOpenAIError(errors.New("CHANNEL BALANCE ERROR"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest), &override))
	require.True(t, ShouldRuntimeDisableChannel(types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), &override))
}

func TestShouldDisableChannelDefaultsIncludeForbidden(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalRanges := operation_setting.AutomaticDisableStatusCodeRanges
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalRanges
	})
	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}

	err := types.NewOpenAIError(errors.New("forbidden"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	assert.False(t, ShouldDisableChannel(err))
}

func TestShouldRuntimeDisableChannelOverrideOnlyDisablesMatchingRuntimeErrors(t *testing.T) {
	originalEnabled := common.RuntimeAutomaticDisableChannelEnabled
	t.Cleanup(func() { common.RuntimeAutomaticDisableChannelEnabled = originalEnabled })
	common.RuntimeAutomaticDisableChannelEnabled = true

	override := dto.ChannelOtherSettings{
		RuntimeAutomaticDisableOverrideEnabled: true,
		RuntimeAutomaticDisableStatusCodes:     "403",
		RuntimeAutomaticDisableKeywords:        "account suspended",
	}

	unmatchedChannelError := types.NewErrorWithStatusCode(
		errors.New("invalid local channel configuration"),
		types.ErrorCodeChannelInvalidKey,
		http.StatusBadRequest,
	)
	matchedStatus := types.NewErrorWithStatusCode(
		errors.New("forbidden"),
		types.ErrorCodeChannelInvalidKey,
		http.StatusForbidden,
	)
	matchedKeyword := types.NewOpenAIError(
		errors.New("ACCOUNT SUSPENDED by provider"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	assert.False(t, ShouldRuntimeDisableChannel(unmatchedChannelError, &override))
	assert.True(t, ShouldRuntimeDisableChannel(matchedStatus, &override))
	assert.True(t, ShouldRuntimeDisableChannel(matchedKeyword, &override))
}

func TestShouldRuntimeDisableChannelGlobalRulesDoNotDisableUnmatchedChannelErrors(t *testing.T) {
	originalEnabled := common.RuntimeAutomaticDisableChannelEnabled
	originalRanges := operation_setting.RuntimeAutomaticDisableStatusCodeRanges
	originalKeywords := operation_setting.RuntimeAutomaticDisableKeywords
	t.Cleanup(func() {
		common.RuntimeAutomaticDisableChannelEnabled = originalEnabled
		operation_setting.RuntimeAutomaticDisableStatusCodeRanges = originalRanges
		operation_setting.RuntimeAutomaticDisableKeywords = originalKeywords
	})
	common.RuntimeAutomaticDisableChannelEnabled = true
	operation_setting.RuntimeAutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 401}}
	operation_setting.RuntimeAutomaticDisableKeywords = []string{"account suspended"}

	unmatched := types.NewErrorWithStatusCode(
		errors.New("invalid local channel configuration"),
		types.ErrorCodeChannelInvalidKey,
		http.StatusBadRequest,
	)

	assert.False(t, ShouldRuntimeDisableChannel(unmatched, nil))
}
