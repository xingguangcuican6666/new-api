package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildBillingQueryRequestAuthModes(t *testing.T) {
	channel := &model.Channel{Key: " channel-api-key "}
	baseURL := "https://billing.example/base///"

	useAPIKey := true
	doNotUseAPIKey := false
	tests := []struct {
		name       string
		config     *dto.BillingQueryConfig
		wantHeader string
	}{
		{
			name: "omitted switch uses channel key",
			config: &dto.BillingQueryConfig{
				Type:    dto.BillingQueryTypeNewAPI,
				BaseURL: baseURL,
			},
			wantHeader: "Bearer channel-api-key",
		},
		{
			name: "enabled switch uses channel key",
			config: &dto.BillingQueryConfig{
				Type:        dto.BillingQueryTypeNewAPI,
				BaseURL:     baseURL,
				BearerToken: "custom-token-is-ignored",
				UseAPIKey:   &useAPIKey,
			},
			wantHeader: "Bearer channel-api-key",
		},
		{
			name: "disabled switch uses custom token",
			config: &dto.BillingQueryConfig{
				Type:        dto.BillingQueryTypeNewAPI,
				BaseURL:     baseURL,
				BearerToken: " custom-token ",
				UseAPIKey:   &doNotUseAPIKey,
			},
			wantHeader: "Bearer custom-token",
		},
		{
			name: "disabled switch with empty token sends no auth",
			config: &dto.BillingQueryConfig{
				Type:      dto.BillingQueryTypeNewAPI,
				BaseURL:   baseURL,
				UseAPIKey: &doNotUseAPIKey,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestURL, headers, err := buildBillingQueryRequest(channel, tt.config)
			require.NoError(t, err)
			assert.Equal(t, "https://billing.example/base/dashboard/billing/credit_grants", requestURL)
			assert.Equal(t, tt.wantHeader, headers.Get("Authorization"))
		})
	}
}

func TestUpdateChannelBalanceUsesConfiguredQueryAndPersistsRawAmount(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	requestPath := make(chan string, 1)
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath <- request.URL.Path
		authorization <- request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"object":"credit_summary","total_granted":20.5,"total_used":8.25,"total_available":12.25}`))
	}))
	defer server.Close()

	baseURL := "https://relay.example.invalid"
	channel := &model.Channel{
		Type:    constant.ChannelTypeOpenAI,
		Key:     "channel-api-key",
		Name:    "configured billing channel",
		BaseURL: &baseURL,
		Balance: 99,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		BillingQuery: &dto.BillingQueryConfig{
			Type:      dto.BillingQueryTypeNewAPI,
			BaseURL:   server.URL + "/billing///",
			UseAPIKey: boolPointer(true),
		},
	})
	require.NoError(t, db.Create(channel).Error)

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	assert.Equal(t, 12.25, result.Balance)
	assert.Empty(t, result.RawResponse)
	assert.Equal(t, "/billing/dashboard/billing/credit_grants", <-requestPath)
	assert.Equal(t, "Bearer channel-api-key", <-authorization)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, 12.25, stored.Balance)
}

func TestUpdateChannelBalanceKeepsAdvancedCustomFallbackWithoutBillingQuery(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	requestPath := make(chan string, 1)
	authorization := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestPath <- request.URL.Path
		authorization <- request.Header.Get("Authorization")
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"balance":4.5}`))
	}))
	defer server.Close()

	baseURL := server.URL
	channel := &model.Channel{
		Type:    constant.ChannelTypeAdvancedCustom,
		Key:     "advanced-custom-key",
		Name:    "advanced custom fallback",
		BaseURL: &baseURL,
		Balance: 8,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		AdvancedCustom: &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{
				{
					IncomingPath: dto.AdvancedCustomBalancePath,
					UpstreamPath: "/provider/balance",
					Converter:    "none",
				},
			},
		},
	})
	require.NoError(t, db.Create(channel).Error)

	result, err := updateChannelBalance(channel)
	require.NoError(t, err)
	assert.Equal(t, "/provider/balance", <-requestPath)
	assert.Equal(t, "Bearer advanced-custom-key", <-authorization)
	assert.Equal(t, "{\n  \"balance\": 4.5\n}", result.RawResponse)

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, 8.0, stored.Balance)
}

func TestFetchBillingQueryBalanceDoesNotUpdateForInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "non-200 response",
			statusCode: http.StatusUnauthorized,
			body:       `{"object":"credit_summary","total_available":12.25}`,
		},
		{
			name:       "invalid JSON",
			statusCode: http.StatusOK,
			body:       "not-json",
		},
		{
			name:       "negative balance",
			statusCode: http.StatusOK,
			body:       `{"object":"credit_summary","total_available":-1}`,
		},
		{
			name:       "oversized response",
			statusCode: http.StatusOK,
			body:       strings.Repeat("x", maxChannelBalanceResponseBytes+1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupModelListControllerTestDB(t)
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.WriteHeader(tt.statusCode)
				_, _ = writer.Write([]byte(tt.body))
			}))
			defer server.Close()

			baseURL := "https://relay.example.invalid"
			channel := &model.Channel{
				Type:    constant.ChannelTypeOpenAI,
				Key:     "channel-api-key",
				Name:    "invalid billing response",
				BaseURL: &baseURL,
				Balance: 7,
			}
			require.NoError(t, db.Create(channel).Error)

			config := &dto.BillingQueryConfig{
				Type:      dto.BillingQueryTypeNewAPI,
				BaseURL:   server.URL,
				UseAPIKey: boolPointer(false),
			}
			_, err := fetchBillingQueryBalance(channel, config)
			require.Error(t, err)

			var stored model.Channel
			require.NoError(t, db.First(&stored, channel.Id).Error)
			assert.Equal(t, 7.0, stored.Balance)
		})
	}
}

func TestSanitizeBillingQueryErrorRemovesCredentials(t *testing.T) {
	secret := "query-secret"
	err := sanitizeBillingQueryError(errors.New("request failed for "+secret), secret)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret)
	assert.Contains(t, err.Error(), "[REDACTED]")
}

func TestGetTokenStatusReturnsCreditSummaryWithoutQuotaConversion(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := &model.Token{
		Id:          301,
		UserId:      17,
		Key:         "token-key",
		RemainQuota: 12345,
		ExpiredTime: -1,
	}
	require.NoError(t, db.Create(token).Error)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("token_id", token.Id)
	context.Set("id", token.UserId)

	GetTokenStatus(context)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Object         string `json:"object"`
		TotalGranted   int    `json:"total_granted"`
		TotalUsed      int    `json:"total_used"`
		TotalAvailable int    `json:"total_available"`
		ExpiresAt      int64  `json:"expires_at"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "credit_summary", response.Object)
	assert.Equal(t, token.RemainQuota, response.TotalGranted)
	assert.Equal(t, 0, response.TotalUsed)
	assert.Equal(t, token.RemainQuota, response.TotalAvailable)
	assert.Equal(t, int64(0), response.ExpiresAt)
}

func boolPointer(value bool) *bool {
	return &value
}
