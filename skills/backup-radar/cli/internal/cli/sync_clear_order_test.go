// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"backup-radar-pp-cli/internal/cliutil/testenv"
	"backup-radar-pp-cli/internal/store"
)

// HAND-FIX(sync-clear-never-marks-complete): a sync that fails flag
// validation, or names an unknown resource, must not touch sync_state.
func TestSyncUsageErrorsLeaveSyncStateUntouched(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("BACKUP_RADAR_BASE_URL", "http://127.0.0.1:1/v3")
	t.Setenv("BACKUP_RADAR_API_KEY", "test-key")
	t.Setenv("BACKUP_RADAR_V2_API_KEY", "")
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSyncStateAt("devices", "KEEP", 7, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	state := func() string {
		s, err := store.OpenWithContext(context.Background(), dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		var b bytes.Buffer
		rows, err := s.DB().Query(`SELECT resource_type, last_cursor, last_attempt_complete FROM sync_state ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var r, c string
			var done int
			_ = rows.Scan(&r, &c, &done)
			fmt.Fprintf(&b, "%s|%s|%d;", r, c, done)
		}
		return b.String()
	}
	want := state()
	for _, args := range [][]string{
		{"sync", "--full", "--since", "bogus", "--db", dbPath},
		{"sync", "--full", "--param", "date=2026-01-01", "--db", dbPath},
		{"sync", "--latest-only", "--param", "Date=2026-01-01", "--db", dbPath},
		{"sync", "--full", "--resources", "devcies", "--db", dbPath},
	} {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		_ = cmd.Execute()
		if got := state(); got != want {
			t.Fatalf("%v rewrote sync_state:\n got %s\nwant %s\n%s", args, got, want, out.String())
		}
	}
}
