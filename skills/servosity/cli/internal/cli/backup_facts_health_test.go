// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func TestBackupHealth(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		last     string
		hydrated bool
		want     string
	}{
		{"", false, "unknown"},
		{"2026-06-04T00:00:00Z", false, "unknown"},
		{"", true, "stale"},
		{"2026-06-04T00:00:00Z", true, "ok"},
		{"2026-05-01T00:00:00Z", true, "stale"},
		{"not-a-time", true, "stale"},
	}
	for _, c := range cases {
		if got := backupHealth(c.last, c.hydrated, now); got != c.want {
			t.Errorf("backupHealth(%q, %v) = %q, want %q", c.last, c.hydrated, got, c.want)
		}
	}
}
