package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsModelMissingError(t *testing.T) {
	tests := []struct {
		name     string
		err      *NewAPIError
		expected bool
	}{
		{
			name: "nil error",
		},
		{
			name:     "upstream model_not_found code",
			err:      WithOpenAIError(OpenAIError{Message: "The model `gpt-x` does not exist", Code: "model_not_found"}, http.StatusNotFound),
			expected: true,
		},
		{
			name:     "404 mentioning model",
			err:      NewErrorWithStatusCode(errors.New("model claude-x not found"), ErrorCodeBadResponse, http.StatusNotFound),
			expected: true,
		},
		{
			name:     "404 no endpoints",
			err:      NewErrorWithStatusCode(errors.New("No endpoints found for this model"), ErrorCodeBadResponse, http.StatusNotFound),
			expected: true,
		},
		{
			name: "404 without model mention",
			err:  NewErrorWithStatusCode(errors.New("resource not found"), ErrorCodeBadResponse, http.StatusNotFound),
		},
		{
			name: "500 mentioning model",
			err:  NewErrorWithStatusCode(errors.New("model overloaded"), ErrorCodeBadResponse, http.StatusInternalServerError),
		},
		{
			name:     "gateway routing abort",
			err:      NewError(errors.New("no available channel"), ErrorCodeModelNotFound),
			expected: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, IsModelMissingError(test.err))
		})
	}
}

func TestIsStreamStallError(t *testing.T) {
	assert.False(t, IsStreamStallError(nil))
	assert.False(t, IsStreamStallError(NewError(errors.New("boom"), ErrorCodeBadResponse)))
	assert.True(t, IsStreamStallError(NewErrorWithStatusCode(
		errors.New("upstream stream stalled before first byte"),
		ErrorCodeChannelStreamTimeout, http.StatusBadGateway)))
}
