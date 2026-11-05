// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"backup-radar-pp-cli/internal/cliutil/testenv"
	"backup-radar-pp-cli/internal/store"
)

// Class incomplete-paging-read-as-complete (Codex r1 4.34.2 review): a
// collection whose last sync attempt did not finish (a --max-pages cap, a
// failed page, an interrupted run) is never read as complete because it kept
// rows. Every report that reads it names it in sources.incomplete and says so
// in sources.note: `sync --resources devices --max-pages 1` over a documented
// ClientsDevicesPage (total_count 2, next_cursor set) stored one mapped device,
// and `unwatched --reason unmapped` reported zero unmapped devices with no note.
// The incomplete list is not narrowed to the collections a report names as
// its sources: a report can read a collection indirectly (Codex r2: the
// device mapping behind `unwatched --reason ticketless --client`), so any
// unfinished sync is named (fail closed).
func TestIncompleteSyncIsNeverReadAsComplete(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// health: a completed sync of an empty page.
	if err := db.SaveSyncState("health", "", 0); err != nil {
		t.Fatal(err)
	}
	// devices: page one of two (a documented BackupDeviceData row), then the
	// --max-pages cap: sync records the attempt as incomplete.
	if _, _, err := db.UpsertBatch("devices", []json.RawMessage{json.RawMessage(
		`{"device_id":"dev-1","br_device_id":"Q2c4N1RhM06xY1k9","client":{"name":"Corp","br_company_name":"Corp BR","client_id":"cli-1"},"device_name":"Server-01","br_device_name":"SERVER-01","br_job_name":"Nightly Backup","last_result":[],"history":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncProgress("devices", "after", 1); err != nil {
		t.Fatal(err)
	}
	db.Close()

	for _, args := range [][]string{
		{"unwatched", "--reason", "unmapped", "--json", "--db", dbPath},
		// Codex r2: a report that reads devices only indirectly (v2-only
		// reasons resolve --client through the device mapping) still names it.
		{"unwatched", "--reason", "ticketless", "--client", "Corp", "--json", "--db", dbPath},
		{"check", "--json", "--db", dbPath},
		{"stale", "--json", "--db", dbPath},
		{"scorecard", "--json", "--db", dbPath},
		{"timeline", "Server-01", "--json", "--db", dbPath},
		{"coverage", "diff", "--names", "SERVER-01,NEW-02", "--json", "--db", dbPath},
	} {
		out, err := runRadarCmd(t, "", args...)
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		var v struct {
			Sources struct {
				Incomplete []string `json:"incomplete"`
				Note       string   `json:"note"`
			} `json:"sources"`
		}
		decodeJSON(t, out, &v)
		if !slices.Contains(v.Sources.Incomplete, "devices") {
			t.Errorf("%s: sources.incomplete = %q, want devices named:\n%s", args[0], v.Sources.Incomplete, out)
		}
		if !strings.Contains(v.Sources.Note, "devices") || !strings.Contains(v.Sources.Note, "did not finish") {
			t.Errorf("%s: sources.note does not say the devices sync did not finish: %q", args[0], v.Sources.Note)
		}
	}
}
