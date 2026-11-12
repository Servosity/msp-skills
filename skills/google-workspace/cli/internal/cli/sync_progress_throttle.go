// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "time"

// syncProgressEvery is the least time between two sync_progress lines of one
// resource's walk. A variable so a test can set it. Hand-fix (handfixes.json
// sync-progress-throttled); tools/maintainer/replay_handfixes.py writes this
// file from handfix_replay/sync_progress_throttle.go.tmpl after a reprint.
// Every write to stdout or stderr costs a full credential check of the write
// plus the stream's tail, whatever the write's size. Sync wrote one progress
// line per page, and at HubSpot's 10 rows a page and a real server's pace
// that check was 74% of sync's CPU (#696). Every line written is still
// checked; sync writes fewer of them.
var syncProgressEvery = 5 * time.Second

// syncProgressDue reports whether a progress line is due, and starts the next
// interval if so. The first page of a resource always reports. The closing
// sync_complete event (or the human "synced (done)" line) carries the final
// count.
func syncProgressDue(last *time.Time) bool {
	now := time.Now()
	if now.Sub(*last) < syncProgressEvery { // a zero last is centuries ago
		return false
	}
	*last = now
	return true
}
