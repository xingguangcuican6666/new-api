package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newEmptyResponseGuardTest(t *testing.T, enabled bool) (*gin.Context, *httptest.ResponseRecorder, *EmptyResponseGuard) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{
				EmptyResponseRetryOverrideEnabled: true,
				EmptyResponseRetryEnabled:         enabled,
			},
		},
	}

	return c, recorder, BeginEmptyResponseGuard(c, info)
}

func TestEmptyResponseGuardRetractsEmptyResponse(t *testing.T) {
	c, recorder, guard := newEmptyResponseGuardTest(t, true)

	SetEventStreamHeaders(c)
	require.NoError(t, StringData(c, `{"choices":[{"delta":{"role":"assistant","content":""},"finish_reason":"stop"}]}`))
	Done(c)

	apiErr := guard.Commit(&dto.Usage{PromptTokens: 12})
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeEmptyResponseRetry, apiErr.GetErrorCode())
	assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)

	assert.Empty(t, recorder.Body.String(), "an empty upstream response must not reach the client")
	assert.Empty(t, recorder.Header().Get("Content-Type"))
	// The retry attempt writes a fresh response, so it must be able to set the
	// SSE headers again.
	_, headersMarked := c.Get(eventStreamHeadersSetKey)
	assert.False(t, headersMarked)
}

func TestEmptyResponseGuardForwardsResponseWithContent(t *testing.T) {
	c, recorder, guard := newEmptyResponseGuardTest(t, true)

	body := `{"choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"completion_tokens":1}}`
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(http.StatusOK)
	_, err := c.Writer.WriteString(body)
	require.NoError(t, err)

	require.Nil(t, guard.Commit(&dto.Usage{PromptTokens: 12, CompletionTokens: 1}))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	assert.JSONEq(t, body, recorder.Body.String())
}

func TestEmptyResponseGuardStopsWithholdingAfterProbeLimit(t *testing.T) {
	c, recorder, guard := newEmptyResponseGuardTest(t, true)

	oversized := strings.Repeat("x", emptyResponseProbeLimit+1)
	_, err := c.Writer.WriteString(oversized)
	require.NoError(t, err)
	assert.Equal(t, oversized, recorder.Body.String(), "a response past the probe limit streams through")

	// Zero output tokens no longer matter: the client already has the bytes.
	require.Nil(t, guard.Commit(&dto.Usage{}))
}

func TestEmptyResponseGuardIsInertWhenDisabled(t *testing.T) {
	c, recorder, guard := newEmptyResponseGuardTest(t, false)

	_, err := c.Writer.WriteString("passed through")
	require.NoError(t, err)
	require.Nil(t, guard.Commit(&dto.Usage{}))
	assert.Equal(t, "passed through", recorder.Body.String())
}

func TestPingDataStopsWithholding(t *testing.T) {
	c, recorder, guard := newEmptyResponseGuardTest(t, true)

	SetEventStreamHeaders(c)
	require.NoError(t, PingData(c))
	assert.Equal(t, ": PING\n\n", recorder.Body.String())
	assert.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))

	require.Nil(t, guard.Commit(&dto.Usage{}), "a client that has been pinged can no longer be retried")
}

func TestIsEmptyUpstreamResponse(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		usage *dto.Usage
		empty bool
	}{
		{
			name:  "openai empty message",
			body:  `{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":9}}`,
			usage: &dto.Usage{PromptTokens: 9},
			empty: true,
		},
		{
			name:  "openai message with content",
			body:  `{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`,
			usage: &dto.Usage{PromptTokens: 9},
			empty: false,
		},
		{
			name:  "openai empty message but reported output tokens",
			body:  `{"choices":[{"message":{"role":"assistant","content":""}}]}`,
			usage: &dto.Usage{PromptTokens: 9, CompletionTokens: 4},
			empty: false,
		},
		{
			name:  "openai stream without content",
			body:  "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n",
			usage: &dto.Usage{},
			empty: true,
		},
		{
			name:  "openai stream with tool call",
			body:  "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"name\":\"get_weather\",\"arguments\":\"\"}}]}}]}\n\ndata: [DONE]\n\n",
			usage: &dto.Usage{},
			empty: false,
		},
		{
			name:  "claude stream without content",
			body:  "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"role\":\"assistant\",\"content\":[]}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n",
			usage: &dto.Usage{},
			empty: true,
		},
		{
			name:  "claude stream with content delta",
			body:  "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n",
			usage: &dto.Usage{},
			empty: false,
		},
		{
			name:  "gemini empty candidates",
			body:  `{"candidates":[{"content":{"role":"model"},"finishReason":"STOP"}]}`,
			usage: &dto.Usage{},
			empty: true,
		},
		{
			name:  "gemini candidate with part text",
			body:  `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]}}]}`,
			usage: &dto.Usage{},
			empty: false,
		},
		{
			name:  "missing usage and empty body",
			body:  "",
			usage: nil,
			empty: true,
		},
		{
			name:  "unparsable body counts as content-free",
			body:  "not json at all",
			usage: &dto.Usage{},
			empty: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			assert.Equal(t, testCase.empty, isEmptyUpstreamResponse([]byte(testCase.body), testCase.usage))
		})
	}
}
