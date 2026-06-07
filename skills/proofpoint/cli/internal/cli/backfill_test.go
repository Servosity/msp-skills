// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the backfill novel feature.

package cli

import (
	"encoding/json"
	"testing"
	"time"
)

func TestChunkBackfillWindows(t *testing.T) {
	base := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		start     time.Time
		end       time.Time
		want      int
		lastShort bool
	}{
		{name: "exact 24 hours", start: base, end: base.Add(24 * time.Hour), want: 24},
		{name: "partial tail window", start: base, end: base.Add(90 * time.Minute), want: 2, lastShort: true},
		{name: "sub-30s sliver dropped", start: base, end: base.Add(time.Hour + 10*time.Second), want: 1},
		{name: "empty range", start: base, end: base, want: 0},
		{name: "single short window", start: base, end: base.Add(5 * time.Minute), want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			windows := chunkBackfillWindows(tc.start, tc.end, time.Hour)
			if len(windows) != tc.want {
				t.Fatalf("got %d windows, want %d", len(windows), tc.want)
			}
			for i, w := range windows {
				if w.End.Sub(w.Start) > time.Hour {
					t.Errorf("window %d exceeds the API 1-hour cap: %v", i, w.End.Sub(w.Start))
				}
				if w.End.Sub(w.Start) < 30*time.Second {
					t.Errorf("window %d shorter than the API 30s floor: %v", i, w.End.Sub(w.Start))
				}
				if i > 0 && !windows[i-1].End.Equal(w.Start) {
					t.Errorf("window %d not contiguous with prior", i)
				}
			}
			if tc.lastShort && len(windows) > 0 {
				last := windows[len(windows)-1]
				if last.End.Sub(last.Start) >= time.Hour {
					t.Errorf("expected short tail window, got %v", last.End.Sub(last.Start))
				}
			}
		})
	}
}

func TestExtractEnvelopeEvents(t *testing.T) {
	envelope := json.RawMessage(`{
		"queryEndTime": "2026-06-06T01:00:00Z",
		"clicksPermitted": [{"id":"c1","recipient":"a@example.test"},{"id":"c2","recipient":"b@example.test"}],
		"messagesDelivered": []
	}`)

	items, err := extractEnvelopeEvents(envelope, "clicksPermitted")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d events, want 2", len(items))
	}

	empty, err := extractEnvelopeEvents(envelope, "messagesDelivered")
	if err != nil {
		t.Fatalf("unexpected error for empty array: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty array must produce zero events, got %d", len(empty))
	}

	missing, err := extractEnvelopeEvents(envelope, "clicksBlocked")
	if err != nil || missing != nil {
		t.Fatalf("absent key must produce nil, nil; got %v, %v", missing, err)
	}

	if _, err := extractEnvelopeEvents(json.RawMessage(`not json`), "clicksPermitted"); err == nil {
		t.Fatal("malformed envelope must error")
	}
}

func TestBackfillResourcePathsCoverAllSingleTypeFeeds(t *testing.T) {
	for _, name := range backfillResourceOrder {
		rp, ok := backfillResourcePaths[name]
		if !ok {
			t.Fatalf("resource %s missing from path map", name)
		}
		if rp.Path == "" || rp.Key == "" {
			t.Fatalf("resource %s has incomplete mapping: %+v", name, rp)
		}
	}
}
