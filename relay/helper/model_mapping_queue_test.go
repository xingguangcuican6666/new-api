package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveModelMappingQueue(t *testing.T) {
	tests := []struct {
		name     string
		mapping  string
		model    string
		expected []ModelMappingQueueEntry
	}{
		{name: "absent mapping", mapping: "", model: "gpt-4o"},
		{name: "empty mapping", mapping: "{}", model: "gpt-4o"},
		{name: "string value stays 1:1", mapping: `{"gpt-4o":"upstream-a"}`, model: "gpt-4o"},
		{name: "unknown model", mapping: `{"other":["a","b"]}`, model: "gpt-4o"},
		{
			name:    "plain list",
			mapping: `{"gpt-4o":["upstream-a","upstream-b"]}`,
			model:   "gpt-4o",
			expected: []ModelMappingQueueEntry{
				{UpstreamModel: "upstream-a"},
				{UpstreamModel: "upstream-b"},
			},
		},
		{
			name:    "objects with retry counts",
			mapping: `{"gpt-4o":[{"model":"upstream-a","retry":2},{"model":"upstream-b"}]}`,
			model:   "gpt-4o",
			expected: []ModelMappingQueueEntry{
				{UpstreamModel: "upstream-a", Retry: 2},
				{UpstreamModel: "upstream-b"},
			},
		},
		{
			name:    "mixed items and blanks",
			mapping: `{"gpt-4o":["",{"model":"upstream-a"},"upstream-b",{"retry":3}]}`,
			model:   "gpt-4o",
			expected: []ModelMappingQueueEntry{
				{UpstreamModel: "upstream-a"},
				{UpstreamModel: "upstream-b"},
			},
		},
		{
			name:    "reasoning base model fallback",
			mapping: `{"gpt-4o":["upstream-4o"]}`,
			model:   "gpt-4o-high",
			expected: []ModelMappingQueueEntry{
				{UpstreamModel: "upstream-4o"},
			},
		},
		{
			name:    "broken json",
			mapping: `{"gpt-4o":`,
			model:   "gpt-4o",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queue := ResolveModelMappingQueue(test.mapping, test.model)
			assert.Equal(t, test.expected, queue)
		})
	}
}

func TestModelMappedHelperAppliesQueueOverride(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("model_mapping", `{"client-model":["upstream-a","upstream-b"],"plain-model":"plain-upstream"}`)
	request := &dto.GeneralOpenAIRequest{Model: "client-model"}
	info := &relaycommon.RelayInfo{OriginModelName: "client-model"}

	c.Set("model_mapping_override", "upstream-b")
	require.NoError(t, ModelMappedHelper(c, info, request))
	assert.Equal(t, "upstream-b", info.UpstreamModelName)
	assert.Equal(t, "upstream-b", request.Model)

	// Without the override the queue value is not a 1:1 mapping; the request
	// keeps the origin model and other keys still resolve their strings.
	c.Set("model_mapping_override", "")
	request2 := &dto.GeneralOpenAIRequest{Model: "plain-model"}
	info2 := &relaycommon.RelayInfo{OriginModelName: "plain-model"}
	require.NoError(t, ModelMappedHelper(c, info2, request2))
	assert.Equal(t, "plain-upstream", info2.UpstreamModelName)
	assert.Equal(t, "plain-upstream", request2.Model)
}
