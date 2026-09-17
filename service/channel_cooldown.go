package service

import (
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Per-(channel, model) cooldown breaker. Every real upstream attempt (retries
// included) feeds its outcome into the (channel, model) pair it used — when a
// channel's model mapping resolves one requested name to an ordered queue of
// upstream models, that is the upstream model the attempt actually used, so
// each mapped model cools down independently; after
// common.ChannelCooldownFailureThreshold consecutive bad outcomes the pair is
// skipped for an escalating cooldown that starts at
// common.ChannelCooldownBaseSeconds and doubles per extra bad outcome up to
// common.ChannelCooldownMaxSeconds. A measured streaming first-byte latency
// above common.ChannelSlowFirstByteSeconds is a bad outcome too and arms the
// base cooldown immediately, even below the failure threshold. Any successful
// attempt resets the streak and clears the cooldown. The channel status itself
// is never changed — the skip is advisory and expires on its own.
//
// Stale entries are dropped by a background janitor so the map cannot grow
// with retired channel/model combinations.

type channelModelKey struct {
	channelId int
	model     string
}

type channelModelState struct {
	badStreak     int
	lastBadAt     int64 // unix seconds
	cooldownUntil int64 // unix seconds
}

var (
	channelCooldownStatesMu sync.Mutex
	channelCooldownStates   = make(map[channelModelKey]*channelModelState)
)

// Test seam for cooldown expiry; production always uses wall-clock time.
var channelCooldownClock = func() int64 { return time.Now().Unix() }

// channelCooldownJanitorTTL is how long a quiet, uncooled entry survives.
const channelCooldownJanitorTTL int64 = 1800

func init() {
	go cleanupChannelCooldownStates()
}

func cleanupChannelCooldownStates() {
	for range time.Tick(10 * time.Minute) {
		now := channelCooldownClock()
		channelCooldownStatesMu.Lock()
		for key, state := range channelCooldownStates {
			if now >= state.cooldownUntil && now-state.lastBadAt > channelCooldownJanitorTTL {
				delete(channelCooldownStates, key)
			}
		}
		channelCooldownStatesMu.Unlock()
	}
}

// RecordChannelAttemptOutcome feeds one real upstream attempt outcome into the
// (channel, model) cooldown breaker.
func RecordChannelAttemptOutcome(channelId int, modelName string, failed bool) {
	if channelId <= 0 {
		return
	}
	if failed {
		recordChannelBadOutcome(channelId, modelName, false)
		return
	}
	resetChannelCooldown(channelId, modelName)
}

// RecordChannelSlowFirstByte arms the cooldown when a measured streaming
// first-byte latency exceeds common.ChannelSlowFirstByteSeconds; slow bytes
// count toward the same escalation streak as failures and arm the base
// cooldown immediately, without waiting for the failure threshold.
func RecordChannelSlowFirstByte(channelId int, modelName string, ttft time.Duration) {
	limit := common.ChannelSlowFirstByteSeconds
	if channelId <= 0 || limit <= 0 || ttft <= time.Duration(limit)*time.Second {
		return
	}
	recordChannelBadOutcome(channelId, modelName, true)
}

func recordChannelBadOutcome(channelId int, modelName string, immediate bool) {
	channelCooldownStatesMu.Lock()
	defer channelCooldownStatesMu.Unlock()
	key := channelModelKey{channelId: channelId, model: modelName}
	state := channelCooldownStates[key]
	if state == nil {
		state = &channelModelState{}
		channelCooldownStates[key] = state
	}
	state.badStreak++
	now := channelCooldownClock()
	if cooldown := channelCooldownDuration(state.badStreak, immediate); cooldown > 0 {
		state.cooldownUntil = now + int64(cooldown.Seconds())
	}
	state.lastBadAt = now
}

// channelCooldownDuration returns the cooldown owed by a streak of bad
// outcomes. Failures only arm it once the streak reaches the threshold;
// immediate (slow first byte) never arms less than the base. The shift is
// clamped so long streaks cannot overflow the duration.
func channelCooldownDuration(streak int, immediate bool) time.Duration {
	base := time.Duration(common.ChannelCooldownBaseSeconds) * time.Second
	if base <= 0 {
		return 0
	}
	if !immediate && streak < common.ChannelCooldownFailureThreshold {
		return 0
	}
	shift := max(streak-common.ChannelCooldownFailureThreshold, 0)
	if shift > 16 {
		shift = 16
	}
	duration := base << uint(shift)
	if max := time.Duration(common.ChannelCooldownMaxSeconds) * time.Second; max > 0 && duration > max {
		duration = max
	}
	return duration
}

func resetChannelCooldown(channelId int, modelName string) {
	channelCooldownStatesMu.Lock()
	defer channelCooldownStatesMu.Unlock()
	state := channelCooldownStates[channelModelKey{channelId: channelId, model: modelName}]
	if state == nil {
		return
	}
	state.badStreak = 0
	state.cooldownUntil = 0
}

// ChannelInErrorCooldown reports whether the channel is temporarily skipped
// for the given model.
func ChannelInErrorCooldown(channelId int, modelName string) bool {
	if channelId <= 0 {
		return false
	}
	channelCooldownStatesMu.Lock()
	defer channelCooldownStatesMu.Unlock()
	state := channelCooldownStates[channelModelKey{channelId: channelId, model: modelName}]
	return state != nil && channelCooldownClock() < state.cooldownUntil
}

// CooldownExcludedChannelIds lists the channels currently skipped for the
// given model by the cooldown breaker.
func CooldownExcludedChannelIds(modelName string) []int {
	channelCooldownStatesMu.Lock()
	defer channelCooldownStatesMu.Unlock()
	now := channelCooldownClock()
	var excluded []int
	for key, state := range channelCooldownStates {
		if key.model == modelName && now < state.cooldownUntil {
			excluded = append(excluded, key.channelId)
		}
	}
	return excluded
}
