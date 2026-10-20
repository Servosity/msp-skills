// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestUnsyncedRowsAreNullInEveryReport is the review's repro end to end:
// before any sync, `abb trend --since 30d --json` answered the NAS's
// nas_not_synced row with id 0, runs 0 ... success_rate_pct 0, fleet status
// with tasks 0 ... stale_devices 0, and a fleet sync row for a NAS whose
// credentials are not set with tasks 0, results 0. Absence is never zero:
// those fields are null.
func TestUnsyncedRowsAreNullInEveryReport(t *testing.T) {
	sweepIsolate(t)
	t.Setenv("SYNOLOGY_BASE_URL", "https://192.0.2.10:5001")
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "PW-NULL-1111")
	rows := func(args ...string) []map[string]any {
		t.Helper()
		out, errText, _ := runCLIErr(t, args...)
		var r []map[string]any
		if err := json.Unmarshal([]byte(out), &r); err != nil || len(r) == 0 {
			t.Fatalf("%v: %v\nout=%s\nerr=%s", args, err, out, errText)
		}
		return r
	}
	check := func(what string, row map[string]any, keys ...string) {
		t.Helper()
		for _, k := range keys {
			if v, ok := row[k]; !ok || v != nil {
				t.Errorf("%s: %s = %v (present %v), want null: %v", what, k, v, ok, row)
			}
		}
	}
	tr := rows("abb", "trend", "--since", "30d", "--json")
	if tr[0]["kind"] != "nas_not_synced" {
		t.Fatalf("abb trend: %v", tr)
	}
	check("abb trend nas_not_synced", tr[0], "id", "runs", "successes", "partials", "failures", "success_rate_pct")
	st := rows("fleet", "status", "--json")
	check("fleet status of an unsynced NAS", st[0], "tasks", "tasks_ok", "tasks_partial", "tasks_failed", "tasks_no_run",
		"tasks_running", "tasks_running_too_long", "tasks_unscheduled", "stale_devices")

	if _, errText, code := runCLIErr(t, "fleet", "add", "nocreds", "--url", "https://192.0.2.20:5001", "--json"); code != 0 {
		t.Fatalf("fleet add: %d %s", code, errText)
	}
	sy := rows("fleet", "sync", "--nas", "nocreds", "--json")
	if sy[0]["ok"] != false || !strings.Contains(sy[0]["error"].(string), "credentials not set") {
		t.Fatalf("fleet sync: %v", sy)
	}
	check("fleet sync row that fetched nothing", sy[0], "tasks", "results", "device_runs", "storage_items")
}
