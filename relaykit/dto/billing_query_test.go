package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingQueryConfigValidate(t *testing.T) {
	valid := &BillingQueryConfig{
		Type:    BillingQueryTypeNewAPI,
		BaseURL: "https://billing.example/api///",
	}
	require.NoError(t, valid.Validate())
	assert.Equal(t, "https://billing.example/api", valid.NormalizedBaseURL())

	tests := []struct {
		name    string
		config  BillingQueryConfig
		message string
	}{
		{
			name:    "missing type",
			config:  BillingQueryConfig{BaseURL: "https://billing.example"},
			message: "billing query type is required",
		},
		{
			name:    "unsupported type",
			config:  BillingQueryConfig{Type: "openai", BaseURL: "https://billing.example"},
			message: "unsupported billing query type",
		},
		{
			name:    "relative URL",
			config:  BillingQueryConfig{Type: BillingQueryTypeNewAPI, BaseURL: "/billing"},
			message: "absolute HTTP or HTTPS URL",
		},
		{
			name:    "unsupported scheme",
			config:  BillingQueryConfig{Type: BillingQueryTypeNewAPI, BaseURL: "ftp://billing.example"},
			message: "HTTP or HTTPS",
		},
		{
			name:    "credentials",
			config:  BillingQueryConfig{Type: BillingQueryTypeNewAPI, BaseURL: "https://user:pass@billing.example"},
			message: "credentials",
		},
		{
			name:    "query string",
			config:  BillingQueryConfig{Type: BillingQueryTypeNewAPI, BaseURL: "https://billing.example?tenant=one"},
			message: "query parameters",
		},
		{
			name:    "fragment",
			config:  BillingQueryConfig{Type: BillingQueryTypeNewAPI, BaseURL: "https://billing.example#billing"},
			message: "absolute HTTP or HTTPS URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.message)
		})
	}
}

func TestBillingQueryConfigUsesAPIKeyCompatibilityDefault(t *testing.T) {
	config := &BillingQueryConfig{}
	assert.True(t, config.UsesAPIKey())

	useAPIKey := true
	config.UseAPIKey = &useAPIKey
	assert.True(t, config.UsesAPIKey())

	useAPIKey = false
	assert.False(t, config.UsesAPIKey())
}
