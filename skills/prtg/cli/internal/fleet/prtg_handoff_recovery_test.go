// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"testing"
	"time"
)

// handoff-recovery: a sensor that is up now, switched to up inside the window
// and has a recorded down at or before that switch is a recovery, whatever
// interrupted the way from down to up (a pause and resume, an acknowledgement,
// a rescheduled scan interval). Its path says whether the down sample was
// the scan right before the switch ("direct") or older ("after_gap").
func TestHandoffNeverOmitsAnInterruptedRecovery(t *testing.T) {
	at := time.Date(2026, 10, 27, 16, 5, 0, 0, time.UTC)
	nodes := []Node{
		{ID: "42", Name: "WAN Ping", Status: "UP", StatusSince: "2026-10-27T16:00:00Z", LastDown: "2026-10-27T15:00:00Z", ScanningInterval: 3600},
		{ID: "43", Name: "Rescheduled", Status: "UP", StatusSince: "2026-10-27T16:00:00Z", LastDown: "2026-10-27T15:00:00Z", ScanningInterval: 60},
		{ID: "44", Name: "Resumed", Status: "UP", StatusSince: "2026-10-27T16:01:00Z", LastDown: "2025-09-22T10:00:00Z", ScanningInterval: 60},
		{ID: "45", Name: "Acked then up", Status: "UP", StatusSince: "2026-10-27T15:50:00Z", LastDown: "2026-10-27T13:10:00Z", ScanningInterval: 300},
		{ID: "46", Name: "New", Status: "UP", StatusSince: "2026-10-27T16:00:00Z"},
		{ID: "47", Name: "Steady", Status: "UP", StatusSince: "2026-10-26T16:00:00Z", LastDown: "2026-10-26T15:00:00Z"},
	}
	rows := Handoff("acme", nodes, at.Add(-30*time.Minute), at)
	got := map[string]HandoffRow{}
	for _, r := range rows {
		got[r.ID] = r
	}
	want := map[string]string{"42": "direct", "43": "after_gap", "44": "after_gap", "45": "after_gap"}
	for id, path := range want {
		r, ok := got[id]
		if !ok || r.Event != "recovered" || r.Via != path || r.LastDown == "" {
			t.Errorf("%s: want recovered via %s, got %+v", id, path, r)
		}
	}
	for _, id := range []string{"46", "47"} {
		if _, ok := got[id]; ok {
			t.Errorf("%s: not a recovery inside the window, got %+v", id, got[id])
		}
	}
}
