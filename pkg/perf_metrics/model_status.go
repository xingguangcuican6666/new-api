package perfmetrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// modelLiveStatus tracks per-model request counters and the most recent
// outcome since process start. It backs the model-square card footer
// (success/failure counts, last success time, last first-byte latency) and is
// independent of the perf-metrics bucket persistence.
type modelLiveStatus struct {
	successCount  atomic.Int64
	failureCount  atomic.Int64
	lastSuccessAt atomic.Int64 // unix seconds, 0 = none
	lastRequestAt atomic.Int64 // unix seconds
	lastTtftMs    atomic.Int64 // 0 = unknown / non-stream
}

var modelLiveStatuses sync.Map // model name -> *modelLiveStatus

type ModelLiveStatusSnapshot struct {
	SuccessCount  int64
	FailureCount  int64
	LastSuccessAt int64
	LastRequestAt int64
	LastTtftMs    int64
}

func recordModelLiveStatus(sample Sample) {
	if sample.Model == "" {
		return
	}
	statusAny, _ := modelLiveStatuses.LoadOrStore(sample.Model, &modelLiveStatus{})
	status := statusAny.(*modelLiveStatus)
	now := time.Now().Unix()
	status.lastRequestAt.Store(now)
	if sample.Success {
		status.successCount.Add(1)
		status.lastSuccessAt.Store(now)
	} else {
		status.failureCount.Add(1)
	}
	if sample.HasTtft && sample.TtftMs >= 0 {
		status.lastTtftMs.Store(sample.TtftMs)
	}
}

func modelLiveStatusSnapshot(modelName string) (ModelLiveStatusSnapshot, bool) {
	value, ok := modelLiveStatuses.Load(modelName)
	if !ok {
		return ModelLiveStatusSnapshot{}, false
	}
	status := value.(*modelLiveStatus)
	return ModelLiveStatusSnapshot{
		SuccessCount:  status.successCount.Load(),
		FailureCount:  status.failureCount.Load(),
		LastSuccessAt: status.lastSuccessAt.Load(),
		LastRequestAt: status.lastRequestAt.Load(),
		LastTtftMs:    status.lastTtftMs.Load(),
	}, true
}
