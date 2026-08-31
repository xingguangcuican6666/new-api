package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

func buildBillingQueryRequest(channel *model.Channel, config *dto.BillingQueryConfig) (string, http.Header, error) {
	if channel == nil {
		return "", nil, errors.New("channel is nil")
	}
	if config == nil {
		return "", nil, errors.New("billing query config is nil")
	}
	if err := config.Validate(); err != nil {
		return "", nil, err
	}

	requestURL := config.NormalizedBaseURL() + dto.BillingQueryNewAPITokenUsagePath
	headers := http.Header{}
	if config.UsesAPIKey() {
		if key := strings.TrimSpace(channel.Key); key != "" {
			headers.Set("Authorization", "Bearer "+key)
		}
	} else if token := strings.TrimSpace(config.BearerToken); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	return requestURL, headers, nil
}

func fetchBillingQueryBalance(channel *model.Channel, config *dto.BillingQueryConfig) (channelBalanceResult, error) {
	secrets := make([]string, 0, 4)
	if channel != nil {
		secrets = append(secrets, channel.Key)
	}
	if config != nil {
		secrets = append(secrets, config.BearerToken, config.BaseURL, config.NormalizedBaseURL())
	}

	requestURL, headers, err := buildBillingQueryRequest(channel, config)
	if err != nil {
		return channelBalanceResult{}, sanitizeBillingQueryError(err, secrets...)
	}

	body, err := getBillingQueryResponseBody(http.MethodGet, requestURL, channel, headers)
	if err != nil {
		return channelBalanceResult{}, sanitizeBillingQueryError(err, secrets...)
	}
	balance, err := parseNewAPITokenUsageBalance(body)
	if err != nil {
		return channelBalanceResult{}, err
	}

	channel.UpdateBalance(balance)
	return channelBalanceResult{Balance: balance}, nil
}

func getBillingQueryResponseBody(method string, requestURL string, channel *model.Channel, headers http.Header) ([]byte, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	request, err := http.NewRequest(method, requestURL, nil)
	if err != nil {
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	client, err := service.GetHttpClientWithProxy(channel.GetSetting().Proxy)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxChannelBalanceResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxChannelBalanceResponseBytes {
		return nil, fmt.Errorf("balance response exceeds %d bytes", maxChannelBalanceResponseBytes)
	}
	return body, nil
}

func parseNewAPITokenUsageBalance(body []byte) (float64, error) {
	var validated json.RawMessage
	if err := common.Unmarshal(body, &validated); err != nil {
		return 0, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	if common.GetJsonType(validated) != "object" {
		return 0, errors.New("invalid balance JSON response: expected an object")
	}

	var response struct {
		Code bool `json:"code"`
		Data *struct {
			TotalAvailable json.RawMessage `json:"total_available"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &response); err != nil {
		return 0, fmt.Errorf("invalid balance JSON response: %w", err)
	}
	if !response.Code {
		return 0, errors.New("invalid balance response: code must be true")
	}
	if response.Data == nil {
		return 0, errors.New("invalid balance response: data is required")
	}
	if len(response.Data.TotalAvailable) == 0 || strings.EqualFold(string(response.Data.TotalAvailable), "null") {
		return 0, errors.New("invalid balance response: total_available is required")
	}
	if common.GetJsonType(response.Data.TotalAvailable) != "number" {
		return 0, errors.New("invalid balance response: total_available must be a number")
	}

	var balance float64
	if err := common.Unmarshal(response.Data.TotalAvailable, &balance); err != nil {
		return 0, fmt.Errorf("invalid balance response: total_available: %w", err)
	}
	if balance < 0 {
		return 0, errors.New("invalid balance response: total_available must not be negative")
	}
	// JSON cannot normally represent NaN or infinity, but keep the check here
	// because the value is converted to float64 before it is persisted.
	if math.IsNaN(balance) || math.IsInf(balance, 0) {
		return 0, errors.New("invalid balance response: total_available must be finite")
	}
	return balance, nil
}

func sanitizeBillingQueryError(err error, secrets ...string) error {
	if err == nil {
		return nil
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}
	message := err.Error()
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret == "" {
			continue
		}
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(secret), "[REDACTED]")
		message = strings.ReplaceAll(message, url.PathEscape(secret), "[REDACTED]")
	}
	return errors.New(message)
}
