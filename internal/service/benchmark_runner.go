package service

import "sync/atomic"

// BenchmarkRunner provides one asynchronous benchmark gate shared by the web
// panel and the root-local control plane.
type BenchmarkRunner struct {
	run     func()
	running atomic.Bool
}

func NewBenchmarkRunner(run func()) *BenchmarkRunner {
	return &BenchmarkRunner{run: run}
}

func (b *BenchmarkRunner) Start() bool {
	if b == nil || b.run == nil || !b.running.CompareAndSwap(false, true) {
		return false
	}
	go func() {
		defer b.running.Store(false)
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
	if b == nil || b.run == nil || !b.running.CompareAndSwap(false, true) {
		return false
	}
	defer b.running.Store(false)
	b.run()
	return true
}

func (b *BenchmarkRunner) Running() bool {
	return b != nil && b.running.Load()
}
