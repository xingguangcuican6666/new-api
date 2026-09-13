package service

import (
	"sync"
	"time"
)

// Per-channel error-rate breaker. Every real upstream attempt new-api makes
// (retries included) feeds a sliding window of the last
// channelErrorRateWindowSize outcomes; once more than
// channelErrorRateThreshold of them failed, every request temporarily skips
// the channel for channelErrorCooldownSeconds. The channel status itself is
// never changed — the skip is advisory and expires on its own.
const (
	channelErrorRateWindowSize  = 30
	channelErrorCooldownSeconds = 10
	// >80% of the window: errors*5 > windowSize*4 keeps the comparison integral.
	channelErrorRateNum = 5
	channelErrorRateDen = 4
)

type channelErrorWindow struct {
	outcomes      [channelErrorRateWindowSize]bool // true = failed attempt
	filled        int
	next          int
	cooldownUntil int64 // unix seconds
}

var (
	channelErrorWindowsMu sync.Mutex
	channelErrorWindows   = make(map[int]*channelErrorWindow)
)

// Test seam for cooldown expiry; production always uses wall-clock time.
var channelErrorClock = func() int64 { return time.Now().Unix() }

// RecordChannelAttemptOutcome feeds one real upstream attempt outcome into
// the channel's sliding window and re-arms the cooldown while the error rate
// stays above the threshold.
func RecordChannelAttemptOutcome(channelId int, failed bool) {
	if channelId <= 0 {
		return
	}
	channelErrorWindowsMu.Lock()
	defer channelErrorWindowsMu.Unlock()
	window := channelErrorWindows[channelId]
	if window == nil {
		window = &channelErrorWindow{}
		channelErrorWindows[channelId] = window
	}
	window.outcomes[window.next] = failed
	window.next = (window.next + 1) % channelErrorRateWindowSize
	if window.filled < channelErrorRateWindowSize {
		window.filled++
	}
	if window.filled == channelErrorRateWindowSize && window.errorCount()*channelErrorRateNum > channelErrorRateWindowSize*channelErrorRateDen {
		window.cooldownUntil = channelErrorClock() + channelErrorCooldownSeconds
	}
}

// ChannelInErrorCooldown reports whether the channel is temporarily skipped
// for all requests.
func ChannelInErrorCooldown(channelId int) bool {
	if channelId <= 0 {
		return false
	}
	channelErrorWindowsMu.Lock()
	defer channelErrorWindowsMu.Unlock()
	window := channelErrorWindows[channelId]
	if window == nil {
		return false
	}
	return channelErrorClock() < window.cooldownUntil
}

// CooldownExcludedChannelIds lists the channels currently skipped by the
// error-rate breaker.
func CooldownExcludedChannelIds() []int {
	channelErrorWindowsMu.Lock()
	defer channelErrorWindowsMu.Unlock()
	now := channelErrorClock()
	var excluded []int
	for channelId, window := range channelErrorWindows {
		if now < window.cooldownUntil {
			excluded = append(excluded, channelId)
		}
	}
	return excluded
}

func (window *channelErrorWindow) errorCount() int {
	count := 0
	for i := 0; i < window.filled; i++ {
		if window.outcomes[i] {
			count++
		}
	}
	return count
}
