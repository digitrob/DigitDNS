package service

import (
	"sync"
	"time"
)

// BenchmarkRunner collapses concurrent benchmark requests onto one run. Start
// runs the probe asynchronously; Run runs it on the caller's goroutine so the
// Adaptive scheduler can read the fresh measurements before deciding. Both
// share the same gate.
type BenchmarkRunner struct {
	run func()

	mu      sync.Mutex
	running bool
	done    chan struct{}
}

func NewBenchmarkRunner(run func()) *BenchmarkRunner {
	return &BenchmarkRunner{run: run}
}

// begin claims the gate and returns the channel closed when the run finishes.
// The channel is installed before running is set, so a concurrent Wait can
// never observe a claimed gate with no channel to wait on.
func (b *BenchmarkRunner) begin() (chan struct{}, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.running {
		return nil, false
	}
	done := make(chan struct{})
	b.done = done
	b.running = true
	return done, true
}

func (b *BenchmarkRunner) finish(done chan struct{}) {
	b.mu.Lock()
	if b.done == done {
		close(done)
		b.done = nil
	}
	b.running = false
	b.mu.Unlock()
}

func (b *BenchmarkRunner) Start() bool {
	if b == nil || b.run == nil {
		return false
	}
	done, ok := b.begin()
	if !ok {
		return false
	}
	go func() {
		defer b.finish(done)
		b.run()
	}()
	return true
}

// Run executes the benchmark on the calling goroutine and reports whether it
// acquired the shared gate. The Adaptive scheduler uses it because it must read
// the fresh measurements before deciding; a background Start would return
// before the probes landed. A manual benchmark already in flight makes Run
// return false, exactly like Start.
func (b *BenchmarkRunner) Run() bool {
	if b == nil || b.run == nil {
		return false
	}
	done, ok := b.begin()
	if !ok {
		return false
	}
	defer b.finish(done)
	b.run()
	return true
}

func (b *BenchmarkRunner) Running() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// Wait blocks until the in-flight run finishes or timeout elapses, and reports
// whether the gate is free. It remains available to callers that need the
// measurements a run is already taking; the Force path no longer waits and
// selects from the latest measurements immediately.
func (b *BenchmarkRunner) Wait(timeout time.Duration) bool {
	if b == nil {
		return true
	}
	deadline := time.Now().Add(timeout)
	for {
		b.mu.Lock()
		done := b.done
		b.mu.Unlock()
		if done == nil {
			return true
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false
		}
		timer := time.NewTimer(remaining)
		select {
		case <-done:
			timer.Stop()
		case <-timer.C:
			return false
		}
	}
}
