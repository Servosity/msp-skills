// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"time"
)

// syncThroughputWindow is how far back sync's measured rate looks, so the
// number follows the current pace instead of averaging the whole run.
const syncThroughputWindow = 30 * time.Second

type syncThroughputSample struct {
	at   time.Time
	rows int64
}

// syncThroughput measures the rows one sync walk stored per second over a
// recent window. Hand-fix (handfixes.json sync-progress-measured-rate);
// tools/maintainer/replay_handfixes.py writes this file from
// handfix_replay/sync_throughput.go.tmpl after a reprint. sync_progress used
// to print the adaptive limiter's ceiling as rate_rps, which is the most the
// limiter would allow, not what the sync did (4.9 while storing 33 rows a
// second, #694). One walk owns one syncThroughput, so it takes no lock.
type syncThroughput struct {
	samples []syncThroughputSample
}

func newSyncThroughput(start time.Time) *syncThroughput {
	return &syncThroughput{samples: []syncThroughputSample{{at: start}}}
}

// rowsPerSec records rows (the walk's running total) at now and returns the
// rate since the oldest sample still inside the window. One sample at or
// before the window's edge is kept, so a slow walk still measures a span.
func (t *syncThroughput) rowsPerSec(now time.Time, rows int64) float64 {
	edge := now.Add(-syncThroughputWindow)
	for len(t.samples) > 1 && !t.samples[1].at.After(edge) {
		t.samples = t.samples[1:]
	}
	first := t.samples[0]
	t.samples = append(t.samples, syncThroughputSample{at: now, rows: rows})
	seconds := now.Sub(first.at).Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(rows-first.rows) / seconds
}

// syncProgressRateJSON is the rate part of a sync_progress event. rate_rps
// keeps its name for existing readers and carries the measured value;
// limiter_ceiling_rps is the limiter's ceiling, when one is active.
func syncProgressRateJSON(rowsPerSec, limiterCeiling float64) string {
	out := fmt.Sprintf(`,"rows_per_sec":%.1f,"rate_rps":%.1f`, rowsPerSec, rowsPerSec)
	if limiterCeiling > 0 {
		out += fmt.Sprintf(`,"limiter_ceiling_rps":%.1f`, limiterCeiling)
	}
	return out
}

// syncProgressRateText is the same two numbers for the human progress line.
func syncProgressRateText(rowsPerSec, limiterCeiling float64) string {
	if limiterCeiling > 0 {
		return fmt.Sprintf("[%.1f rows/s, limit %.1f req/s]", rowsPerSec, limiterCeiling)
	}
	return fmt.Sprintf("[%.1f rows/s]", rowsPerSec)
}
