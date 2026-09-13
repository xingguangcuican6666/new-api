package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupChannelErrorRateTest(t *testing.T) *int64 {
	t.Helper()
	channelErrorWindowsMu.Lock()
	channelErrorWindows = make(map[int]*channelErrorWindow)
	channelErrorWindowsMu.Unlock()
	now := int64(1_000_000)
	channelErrorClock = func() int64 { return now }
	t.Cleanup(func() {
		channelErrorWindowsMu.Lock()
		channelErrorWindows = make(map[int]*channelErrorWindow)
		channelErrorWindowsMu.Unlock()
		channelErrorClock = func() int64 { return time.Now().Unix() }
	})
	return &now
}

func TestChannelErrorRateCooldownTriggersOnlyAboveEightyPercent(t *testing.T) {
	now := setupChannelErrorRateTest(t)

	// Fill the 30-attempt window with exactly 80% errors.
	for i := 0; i < 6; i++ {
		RecordChannelAttemptOutcome(42, false)
	}
	for i := 0; i < 24; i++ {
		RecordChannelAttemptOutcome(42, true)
	}
	assert.False(t, ChannelInErrorCooldown(42), "exactly 80% errors must not trigger the skip")

	RecordChannelAttemptOutcome(42, true)
	assert.True(t, ChannelInErrorCooldown(42), "25/30 errors is above the 80% threshold")
	assert.Contains(t, CooldownExcludedChannelIds(), 42)

	// The skip expires on its own without changing any channel state.
	*now += channelErrorCooldownSeconds + 1
	assert.False(t, ChannelInErrorCooldown(42))
	assert.NotContains(t, CooldownExcludedChannelIds(), 42)
}

func TestChannelErrorRateRecoversThroughSuccessfulProbes(t *testing.T) {
	now := setupChannelErrorRateTest(t)

	for i := 0; i < 30; i++ {
		RecordChannelAttemptOutcome(42, true)
	}
	require.True(t, ChannelInErrorCooldown(42))

	// After every expiry the channel is reachable again and a successful
	// probe slides one failure out of the window. While the window still
	// holds more than 80% errors, the skip re-arms; once it drops to exactly
	// 80% the channel stays reachable.
	for i := 0; i < 6; i++ {
		*now += channelErrorCooldownSeconds + 1
		require.False(t, ChannelInErrorCooldown(42))
		RecordChannelAttemptOutcome(42, false)
	}
	assert.False(t, ChannelInErrorCooldown(42))

	// Failures replace old failures one-for-one while the window holds
	// exactly 80% errors; the skip re-arms once the successful probes slide
	// out of the window.
	*now += channelErrorCooldownSeconds + 1
	for i := 0; i < 24; i++ {
		RecordChannelAttemptOutcome(42, true)
	}
	assert.False(t, ChannelInErrorCooldown(42), "exactly 80% must not re-arm the skip")
	RecordChannelAttemptOutcome(42, true)
	assert.True(t, ChannelInErrorCooldown(42), "the failure that drops a success from the window re-arms the skip")
}

func TestChannelErrorRateIsolatesChannelsAndIgnoresInvalidIds(t *testing.T) {
	now := setupChannelErrorRateTest(t)

	for i := 0; i < 30; i++ {
		RecordChannelAttemptOutcome(42, true)
	}
	assert.True(t, ChannelInErrorCooldown(42))
	assert.False(t, ChannelInErrorCooldown(43), "the skip must stay per channel")
	assert.False(t, ChannelInErrorCooldown(0))

	RecordChannelAttemptOutcome(0, true)
	assert.Equal(t, []int{42}, CooldownExcludedChannelIds())

	// Channel 43 arms its own skip after 42's has expired.
	*now += channelErrorCooldownSeconds + 1
	assert.False(t, ChannelInErrorCooldown(42))
	for i := 0; i < 30; i++ {
		RecordChannelAttemptOutcome(43, true)
	}
	assert.True(t, ChannelInErrorCooldown(43))
	assert.False(t, ChannelInErrorCooldown(42))
}
