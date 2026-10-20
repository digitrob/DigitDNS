package service

import (
	"testing"

	"digitdns/internal/database"
)

func TestSummarizeDiagnosticsScore(t *testing.T) {
	rows := func(successes, failures int) []DiagnosticResult {
		out := make([]DiagnosticResult, 0, successes+failures)
		for i := 0; i < successes; i++ {
			out = append(out, DiagnosticResult{Name: "ok", Target: "x:1", Success: true, LatencyMs: 10})
		}
		for i := 0; i < failures; i++ {
			out = append(out, DiagnosticResult{Name: "bad", Target: "y:1", Success: false})
		}
		return out
	}

	for _, tc := range []struct {
		name        string
		ok, bad     int
		wantScore   int
		wantReach   int
		wantTotal   int
		avgWhenNone bool
	}{
		{"all reachable", 8, 0, 100, 8, 8, false},
		{"none reachable", 0, 8, 0, 0, 8, false},
		{"seven of eight rounds", 7, 1, 88, 7, 8, false},
		{"empty list scores 100", 0, 0, 100, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := SummarizeDiagnostics(rows(tc.ok, tc.bad))
			if rep.OverallScore != tc.wantScore {
				t.Errorf("score = %d, want %d", rep.OverallScore, tc.wantScore)
			}
			if rep.Reachable != tc.wantReach || rep.Total != tc.wantTotal {
				t.Errorf("reachable/total = %d/%d, want %d/%d", rep.Reachable, rep.Total, tc.wantReach, tc.wantTotal)
			}
			if len(rep.Results) != tc.wantTotal {
				t.Errorf("results = %d rows, want %d", len(rep.Results), tc.wantTotal)
			}
		})
	}
}

func TestSummarizeDiagnosticsAverageOverReachableOnly(t *testing.T) {
	rep := SummarizeDiagnostics([]DiagnosticResult{
		{Name: "a", Target: "a:1", Success: true, LatencyMs: 10},
		{Name: "b", Target: "b:1", Success: true, LatencyMs: 20},
		{Name: "c", Target: "c:1", Success: false},
	})
	if rep.AvgLatencyMs != 15 {
		t.Fatalf("avg_latency_ms = %v, want 15 (the failed dial's 0 must not be averaged in)", rep.AvgLatencyMs)
	}
}

func TestSummarizeDiagnosticsQualityBands(t *testing.T) {
	bands := func(score int) string {
		rows := make([]DiagnosticResult, 0, 100)
		for i := 0; i < 100; i++ {
			rows = append(rows, DiagnosticResult{Success: i < score})
		}
		return SummarizeDiagnostics(rows).OverallQuality
	}
	if bands(100) != "EXCELLENT (A+)" {
		t.Errorf("100%% = %q, want EXCELLENT (A+)", bands(100))
	}
	if bands(60) != "MODERATE (B)" {
		t.Errorf("60%% = %q, want MODERATE (B)", bands(60))
	}
	if bands(40) != "POOR (C)" {
		t.Errorf("40%% = %q, want POOR (C)", bands(40))
	}
}

func TestRunDiagnosticsEmptyList(t *testing.T) {
	rep := RunDiagnostics(nil)
	if rep.Total != 0 || len(rep.Results) != 0 {
		t.Fatalf("empty run = %d total / %d results, want none", rep.Total, len(rep.Results))
	}
	if rep.Results == nil {
		t.Fatal("Results is nil; the JSON would be null rather than []")
	}
}

// A closed loopback port refuses instantly, so this exercises the dial path
// without depending on the internet or waiting out DiagnosticTimeout.
func TestRunDiagnosticsDialsAndMarksUnreachable(t *testing.T) {
	db := database.DiagnosticTarget{Name: "closed", Host: "127.0.0.1:1"}
	rep := RunDiagnostics([]database.DiagnosticTarget{db})
	if rep.Total != 1 {
		t.Fatalf("total = %d, want 1", rep.Total)
	}
	if rep.Results[0].Success {
		t.Fatal("a closed port reported as reachable")
	}
	if rep.Reachable != 0 || rep.OverallScore != 0 {
		t.Fatalf("reachable/score = %d/%d, want 0/0", rep.Reachable, rep.OverallScore)
	}
}
