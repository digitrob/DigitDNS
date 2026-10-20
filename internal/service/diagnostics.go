package service

import (
	"math"
	"net"
	"sync"
	"time"

	"digitdns/internal/database"
)

// DiagnosticResult is one row of a diagnostic run.
type DiagnosticResult struct {
	Name      string  `json:"name"`
	Target    string  `json:"target"`
	Success   bool    `json:"success"`
	LatencyMs float64 `json:"latency_ms"`
}

// DiagnosticReport is the whole answer to one run: the per-target rows plus the
// derived score, quality band and average latency over the targets that answered.
type DiagnosticReport struct {
	Results        []DiagnosticResult `json:"results"`
	OverallScore   int                `json:"overall_score"`
	OverallQuality string             `json:"overall_quality"`
	AvgLatencyMs   float64            `json:"avg_latency_ms"`
	Reachable      int                `json:"reachable"`
	Total          int                `json:"total"`
	Timestamp      time.Time          `json:"timestamp"`
}

// DiagnosticTimeout bounds a single probe.
const DiagnosticTimeout = 2500 * time.Millisecond

// RunDiagnostics dials every target in parallel and summarises the result. It is
// shared by the dashboard's /api/diagnostics/run and the v2 REST route so the two
// cannot drift on what a "reachable target" or a score means.
//
// Each target is a host:port (the handlers normalise the operator's input to that
// form before storing it). A dial that does not complete in DiagnosticTimeout is
// an unreachable target with a 0 ms latency; the row is still returned, because
// the unreachable row is the one an operator most needs to see.
func RunDiagnostics(targets []database.DiagnosticTarget) DiagnosticReport {
	results := make([]DiagnosticResult, len(targets))

	var wg sync.WaitGroup
	for i, t := range targets {
		wg.Add(1)
		go func(idx int, tgt database.DiagnosticTarget) {
			defer wg.Done()
			start := time.Now()
			conn, err := net.DialTimeout("tcp", tgt.Host, DiagnosticTimeout)
			dur := float64(time.Since(start).Microseconds()) / 1000.0
			if err != nil {
				results[idx] = DiagnosticResult{Name: tgt.Name, Target: tgt.Host, Success: false, LatencyMs: 0}
				return
			}
			_ = conn.Close()
			results[idx] = DiagnosticResult{Name: tgt.Name, Target: tgt.Host, Success: true, LatencyMs: dur}
		}(i, t)
	}
	wg.Wait()

	return SummarizeDiagnostics(results)
}

// SummarizeDiagnostics derives the score, band and average from a set of rows. It
// is separate from RunDiagnostics so the arithmetic can be tested without opening
// sockets, and so a caller that already has rows (or wants to render a stored
// report) shares the exact scoring.
func SummarizeDiagnostics(results []DiagnosticResult) DiagnosticReport {
	successCount := 0
	totalLatency := 0.0
	for _, r := range results {
		if r.Success {
			successCount++
			totalLatency += r.LatencyMs
		}
	}

	// An empty list scores 100 rather than 0: "nothing to test" is not "everything
	// failed", and it is the only division by zero here.
	score := 100
	if len(results) > 0 {
		// math.Round, not int() truncation: 7 of 8 is 87.5%, which truncates to 87
		// while the panel's "7/8 reachable" line rounds to 88. Rounding keeps the
		// score agreeing with the count it is derived from.
		score = int(math.Round((float64(successCount) / float64(len(results))) * 100))
	}

	avg := 0.0
	if successCount > 0 {
		avg = totalLatency / float64(successCount)
	}

	quality := "EXCELLENT (A+)"
	if score < 70 {
		quality = "MODERATE (B)"
	}
	if score < 50 {
		quality = "POOR (C)"
	}

	if results == nil {
		results = []DiagnosticResult{}
	}

	return DiagnosticReport{
		Results:        results,
		OverallScore:   score,
		OverallQuality: quality,
		AvgLatencyMs:   math.Round(avg*100) / 100,
		Reachable:      successCount,
		Total:          len(results),
		Timestamp:      time.Now(),
	}
}
