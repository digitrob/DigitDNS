package service

import (
	"fmt"
	"sync"
	"testing"

	"digitdns/internal/database"
)

// The live-log ring used to write a slot before a CAS on the count. Two writers
// could both write the same slot, only one CAS won, and a reader could observe a
// torn QueryLogItem (it carries slices and strings). These tests pin the
// claim-then-publish design under -race and check that Clear cannot leak an
// entry written before it into a later history snapshot.
func TestRecentLogsConcurrentPublishAndClear(t *testing.T) {
	s := &StatsService{}

	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 400; i++ {
				s.PushQueryLog(database.QueryLogItem{
					Domain:   fmt.Sprintf("w%d-i%d.example", w, i),
					ClientIP: "192.0.2.1",
				})
				if i%64 == 0 {
					s.ClearRecentLogs()
					_ = s.GetRecentLogs()
				}
			}
		}(w)
	}
	wg.Wait()

	got := s.GetRecentLogs()
	if len(got) > recentLogsCap {
		t.Fatalf("history length = %d, want <= %d", len(got), recentLogsCap)
	}
	for _, item := range got {
		if item.Domain == "" {
			t.Error("empty domain in the ring: a slot was read before it was published")
		}
	}
}

func TestRecentLogsClearDropsOldEntriesAndWraparoundKeepsNewest(t *testing.T) {
	s := &StatsService{}

	for i := 0; i < recentLogsCap+25; i++ {
		s.PushQueryLog(database.QueryLogItem{Domain: fmt.Sprintf("old-%03d.example", i)})
	}
	got := s.GetRecentLogs()
	if len(got) != recentLogsCap {
		t.Fatalf("history after wrap = %d, want %d", len(got), recentLogsCap)
	}
	if got[0].Domain != "old-025.example" {
		t.Fatalf("oldest after wrap = %q, want old-025.example", got[0].Domain)
	}
	if got[len(got)-1].Domain != fmt.Sprintf("old-%03d.example", recentLogsCap+24) {
		t.Fatalf("newest after wrap = %q", got[len(got)-1].Domain)
	}

	s.ClearRecentLogs()
	if got := s.GetRecentLogs(); len(got) != 0 {
		t.Fatalf("history after Clear = %d, want 0", len(got))
	}

	s.PushQueryLog(database.QueryLogItem{Domain: "new.example"})
	got = s.GetRecentLogs()
	if len(got) != 1 || got[0].Domain != "new.example" {
		t.Fatalf("history after Clear + push = %+v, want only new.example", got)
	}
}
