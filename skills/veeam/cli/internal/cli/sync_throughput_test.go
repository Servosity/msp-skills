// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-progress-measured-rate): tools/maintainer/
// replay_handfixes.py writes this file from
// handfix_replay/sync_throughput_test.go.tmpl after a reprint.

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"veeam-pp-cli/internal/store"
)

func TestSyncThroughputMeasuresARecentWindow(t *testing.T) {
	start := time.Unix(1000, 0)
	rate := newSyncThroughput(start)
	if got := rate.rowsPerSec(start.Add(10*time.Second), 100); got != 10 {
		t.Fatalf("100 rows in 10s = %v rows/s, want 10", got)
	}
	// 100 rows in the first 10s, then 1,000 in the next 30s: the window has
	// moved past the slow start, so the rate is 1,000 / 30, not 1,100 / 40.
	got := rate.rowsPerSec(start.Add(40*time.Second), 1100)
	if want := 1000.0 / 30.0; math.Abs(got-want) > 0.01 {
		t.Fatalf("rate over the window = %v, want %v", got, want)
	}
}

func TestSyncProgressRateNamesWhatWasMeasured(t *testing.T) {
	got := syncProgressRateJSON(33, 4.9)
	want := `,"rows_per_sec":33.0,"rate_rps":33.0,"limiter_ceiling_rps":4.9`
	if got != want {
		t.Fatalf("event fields = %s, want %s", got, want)
	}
	if got := syncProgressRateJSON(33, 0); strings.Contains(got, "limiter_ceiling_rps") {
		t.Fatalf("no active limiter, but the event names a ceiling: %s", got)
	}
	if got, want := syncProgressRateText(33, 4.9), "[33.0 rows/s, limit 4.9 req/s]"; got != want {
		t.Fatalf("human line = %s, want %s", got, want)
	}
}

// syncRateFake answers its first read with two rows and later reads with an
// empty page, at a limiter ceiling the walk never reaches.
type syncRateFake struct{ calls int }

func (f *syncRateFake) reply() json.RawMessage {
	f.calls++
	if f.calls > 1 {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(`[{"id":"1","name":"Acme Corp"},{"id":"2","name":"Acme Corp"}]`)
}

func (f *syncRateFake) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	return f.reply(), nil
}

func (f *syncRateFake) PostWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error) {
	return f.reply(), 200, nil
}

func (f *syncRateFake) PostQueryWithParams(context.Context, string, map[string]string, any) (json.RawMessage, int, error) {
	return f.reply(), 200, nil
}

func (f *syncRateFake) RateLimit() float64 { return 4.9 }

// TestSyncProgressReportsMeasuredRowsNotTheLimiter walks real resources and
// reads the sync_progress event: its rate must be the measured one, with the
// limiter's 4.9 under its own name.
func TestSyncProgressReportsMeasuredRowsNotTheLimiter(t *testing.T) {
	saved := humanFriendly
	humanFriendly = false
	defer func() { humanFriendly = saved }()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("opening the local store: %v", err)
	}
	defer db.Close()
	for _, resource := range defaultSyncResources() {
		c := &syncRateFake{}
		var events bytes.Buffer
		_ = syncResource(context.Background(), c, db, resource, "", false, 0, false, false, &syncUserParams{}, &events)
		for _, line := range strings.Split(events.String(), "\n") {
			if !strings.Contains(line, `"event":"sync_progress"`) {
				continue
			}
			var event struct {
				Fetched int64    `json:"fetched"`
				Rows    *float64 `json:"rows_per_sec"`
				Rate    *float64 `json:"rate_rps"`
				Ceiling *float64 `json:"limiter_ceiling_rps"`
			}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("%s: event is not JSON: %v: %s", resource, err, line)
			}
			if event.Fetched == 0 {
				continue // a page that stored nothing measures nothing
			}
			if event.Rows == nil || event.Rate == nil || event.Ceiling == nil {
				t.Fatalf("%s: event lacks rows_per_sec, rate_rps or limiter_ceiling_rps: %s", resource, line)
			}
			if *event.Rate != *event.Rows || *event.Rate == 4.9 || *event.Ceiling != 4.9 {
				t.Fatalf("%s: rate_rps must be the measured rows_per_sec and the ceiling its own field: %s", resource, line)
			}
			return
		}
	}
	t.Skip("no default resource stored a row from the fake page, so no measured rate was emitted")
}
