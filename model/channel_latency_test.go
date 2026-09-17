package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func setupChannelLatencyTest(t *testing.T) {
	t.Helper()
	channelLatencyStatesMu.Lock()
	channelLatencyStates = make(map[channelLatencyKey]*channelLatencyState)
	modelLatencyStates = make(map[string]*channelLatencyState)
	channelLatencyStatesMu.Unlock()
	common.ChannelLatencyWeightingEnabled = true
	t.Cleanup(func() {
		channelLatencyStatesMu.Lock()
		channelLatencyStates = make(map[channelLatencyKey]*channelLatencyState)
		modelLatencyStates = make(map[string]*channelLatencyState)
		channelLatencyStatesMu.Unlock()
		channelLatencyClock = func() int64 { return time.Now().Unix() }
	})
}

func TestChannelLatencyWeight(t *testing.T) {
	setupChannelLatencyTest(t)

	// Without data every pair is neutral.
	assert.Equal(t, 1.0, ChannelLatencyWeight(42, "gpt-4o"))

	// Model-wide aggregate over ten channels at ~2000ms.
	for id := 1; id <= 10; id++ {
		RecordChannelTtft(id, "gpt-4o", 2*time.Second)
	}
	// Channel 42 converges to ~6000ms, three times the average.
	for i := 0; i < 3; i++ {
		RecordChannelTtft(42, "gpt-4o", 6*time.Second)
	}
	// Channel 43 tracks the model average.
	for i := 0; i < 3; i++ {
		RecordChannelTtft(43, "gpt-4o", 2*time.Second)
	}

	weightSlow := ChannelLatencyWeight(42, "gpt-4o")
	assert.Less(t, weightSlow, 0.5, "a 3x slower pair must be clearly demoted")
	assert.GreaterOrEqual(t, weightSlow, 0.1, "the demotion floor keeps a share of traffic")
	assert.Equal(t, 1.0, ChannelLatencyWeight(43, "gpt-4o"), "an average pair keeps its full weight")
	assert.Equal(t, 1.0, ChannelLatencyWeight(44, "gpt-4o"), "a pair without its own samples stays neutral")

	// Disabling the switch restores neutral weights.
	common.ChannelLatencyWeightingEnabled = false
	assert.Equal(t, 1.0, ChannelLatencyWeight(42, "gpt-4o"))
}
