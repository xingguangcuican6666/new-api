package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogOtherScopesAndMerges(t *testing.T) {
	var other LogOther

	assert.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	other.MergePublic(map[string]any{
		"zero": 0,
	})
	assert.True(t, other.SetAdmin("use_channel", []string{"channel-a"}))
	other.MergeAdmin(map[string]any{
		"rejected": false,
	})
	assert.True(t, other.SetRoot("upstream_request_id", "upstream-private"))
	other.MergeRoot(map[string]any{
		"generation": 0,
	})
	assert.True(t, other.SetAudit("method", "POST"))
	other.MergeAudit(map[string]any{
		"success": false,
	})

	require.JSONEq(t, `{
		"request_path": "/v1/chat/completions",
		"zero": 0,
		"admin_info": {
			"use_channel": ["channel-a"],
			"rejected": false
		},
		"root_info": {
			"upstream_request_id": "upstream-private",
			"generation": 0
		},
		"audit_info": {
			"method": "POST",
			"success": false
		}
	}`, other.JSONString())
}

func TestLogOtherRejectsSensitivePublicFields(t *testing.T) {
	other := NewLogOther()

	for _, key := range []string{
		"admin_info",
		"root_info",
		"audit_info",
		"channel_id",
		"channel_name",
		"channel_type",
		"reject_reason",
	} {
		assert.False(t, other.SetPublic(key, "must-not-leak"), key)
	}
	other.MergePublic(map[string]any{
		"request_path": "/v1/responses",
		"channel_name": "still-must-not-leak",
		"admin_info":   map[string]any{"secret": true},
	})

	require.JSONEq(t, `{"request_path":"/v1/responses"}`, other.JSONString())
	require.JSONEq(t, `{}`, NewLogOther().JSONString())
}

func TestLogOtherJSONStringDoesNotMutateReceiver(t *testing.T) {
	other := NewLogOther()
	require.True(t, other.SetPublic("request_path", "/v1/chat/completions"))
	require.True(t, other.SetAdmin("rejected", false))

	before := other.Snapshot()
	first := other.JSONString()
	after := other.Snapshot()
	second := other.JSONString()

	require.Equal(t, before, after)
	require.Equal(t, first, second)
}

func TestFormatLogOtherJSONModelMappingPrivacy(t *testing.T) {
	origPrivacy := setting.UpstreamPrivacyProtectionEnabled
	defer func() { setting.UpstreamPrivacyProtectionEnabled = origPrivacy }()

	mappedOther := func() *LogOther {
		other := NewLogOther()
		require.True(t, other.SetPublic("is_model_mapped", true))
		require.True(t, other.SetPublic("upstream_model_name", "upstream-secret-model"))
		require.True(t, other.SetPublic("model_ratio", 1.5))
		return other
	}

	setting.UpstreamPrivacyProtectionEnabled = false
	assert.Contains(t, formatLogOtherJSON(mappedOther().JSONString(), logOtherVisibilityUser), "upstream-secret-model")

	setting.UpstreamPrivacyProtectionEnabled = true
	userJSON := formatLogOtherJSON(mappedOther().JSONString(), logOtherVisibilityUser)
	assert.NotContains(t, userJSON, "is_model_mapped")
	assert.NotContains(t, userJSON, "upstream_model_name")
	assert.NotContains(t, userJSON, "upstream-secret-model")
	assert.Contains(t, userJSON, "model_ratio")

	adminJSON := formatLogOtherJSON(mappedOther().JSONString(), logOtherVisibilityAdmin)
	assert.Contains(t, adminJSON, "upstream-secret-model")

	rootJSON := formatLogOtherJSON(mappedOther().JSONString(), logOtherVisibilityRoot)
	assert.Contains(t, rootJSON, "upstream-secret-model")
}
