package service

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestShouldDisableChannelUsesChannelOverrideAsCompleteReplacement(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	originalRanges := operation_setting.AutomaticDisableStatusCodeRanges
	originalKeywords := operation_setting.AutomaticDisableKeywords
	t.Cleanup(func() {
		common.AutomaticDisableChannelEnabled = originalEnabled
		operation_setting.AutomaticDisableStatusCodeRanges = originalRanges
		operation_setting.AutomaticDisableKeywords = originalKeywords
	})

	common.AutomaticDisableChannelEnabled = true
	operation_setting.AutomaticDisableStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 403}}
	operation_setting.AutomaticDisableKeywords = []string{"global balance error"}
	override := dto.ChannelOtherSettings{
		AutomaticDisableOverrideEnabled: true,
		AutomaticDisableStatusCodes:     "429",
		AutomaticDisableKeywords:        "channel balance error",
	}

	require.False(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("global balance error"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden), &override))
	require.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("CHANNEL BALANCE ERROR"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest), &override))
	require.True(t, ShouldDisableChannel(types.NewOpenAIError(errors.New("rate limited"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), &override))
}

func TestShouldDisableChannelDefaultsIncludeForbidden(t *testing.T) {
	originalEnabled := common.AutomaticDisableChannelEnabled
	t.Cleanup(func() { common.AutomaticDisableChannelEnabled = originalEnabled })
	common.AutomaticDisableChannelEnabled = true

	err := types.NewOpenAIError(errors.New("forbidden"), types.ErrorCodeBadResponseStatusCode, http.StatusForbidden)
	require.True(t, ShouldDisableChannel(err, nil))
}
