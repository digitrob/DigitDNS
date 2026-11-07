package safego

import (
	"sync"
	"testing"
)

func TestRecoverContainsPanicAndCounts(t *testing.T) {
	before := Count()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer Recover("test")
		panic("boom")
	}()
	wg.Wait()

	if got := Count(); got != before+1 {
		t.Fatalf("Count() = %d, want %d", got, before+1)
	}
}

func TestHandleIgnoresNil(t *testing.T) {
	before := Count()
	Handle("test", nil)
	if got := Count(); got != before {
		t.Fatalf("nil panic changed count to %d", got)
	}
}
