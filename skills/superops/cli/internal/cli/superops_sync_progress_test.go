// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-fix (handfixes.json sync-progress-throttled). This connector's
// syncResource takes a concrete GraphQL client and writes its events to the
// process stderr, so its walk runs through the CLI against fakeSuperOps.

import (
	"strings"
	"testing"
	"time"
)

// superopsProgressLines syncs 120 tickets (three pages of 50) and counts the
// sync_progress lines written.
func superopsProgressLines(t *testing.T, every time.Duration) int {
	t.Helper()
	saved := syncProgressEvery
	syncProgressEvery = every
	defer func() { syncProgressEvery = saved }()
	startFakeSuperOps(t, &fakeSuperOps{data: map[string][]map[string]any{"getTicketList": fakeTickets(120, time.Now())}})
	var out string
	var err error
	events := captureProcessStderr(t, func() { out, err = runSuperOpsCLI(t, "sync", "--resources", "tickets", "--full", "--json") })
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := superopsStoreRows(t, "tickets"); got != 120 {
		t.Fatalf("stored %d tickets, want 120", got)
	}
	return strings.Count(events, `"event":"sync_progress"`)
}

func TestSuperopsSyncWritesProgressAtMostOncePerInterval(t *testing.T) {
	if got := superopsProgressLines(t, time.Hour); got != 1 {
		t.Fatalf("3 pages inside one interval wrote %d progress lines, want 1 (the first page)", got)
	}
}

func TestSuperopsSyncWritesProgressPerPageWithNoInterval(t *testing.T) {
	if got := superopsProgressLines(t, 0); got != 3 {
		t.Fatalf("wrote %d progress lines, want one for each of 3 pages", got)
	}
}
