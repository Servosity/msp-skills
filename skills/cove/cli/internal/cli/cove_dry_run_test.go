// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"cove-pp-cli/internal/cliutil/testenv"
)

// TestHandBuiltDryRunJSONIsJSON: under --dry-run --json every hand-built
// command prints the dry-run envelope, never prose, so an agent parsing stdout
// gets JSON (live dogfood's dry_run_json check failed on prose).
func TestHandBuiltDryRunJSONIsJSON(t *testing.T) {
	testenv.Isolate(t)
	for _, path := range [][]string{
		{"billing", "changes"}, {"billing", "usage"},
		{"devices", "changes"}, {"devices", "failures"}, {"devices", "sources"}, {"devices", "stale"},
		{"fleet", "activity"}, {"fleet", "health"}, {"storage", "growth"},
		{"auth", "login"}, {"auth", "status"}, {"auth", "token"}, {"auth", "logout"},
		{"call", "GetServerInfo"}, {"snapshot"},
	} {
		args := append(append([]string{}, path...), "--dry-run", "--json")
		out, err := runCoveCmd(t, args...)
		if err != nil {
			t.Errorf("%s: %v\n%s", strings.Join(path, " "), err, out)
			continue
		}
		var env struct {
			DryRun bool   `json:"dry_run"`
			Would  string `json:"would"`
		}
		if json.Unmarshal([]byte(out), &env) != nil || !env.DryRun || env.Would == "" {
			t.Errorf("%s --dry-run --json printed %q, want the dry-run JSON envelope", strings.Join(path, " "), out)
		}
	}
}
