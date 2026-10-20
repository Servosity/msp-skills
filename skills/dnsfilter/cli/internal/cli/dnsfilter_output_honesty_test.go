// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Output-honesty tests: fleet commands must never print an all-clear when
// nothing was compared (unsynced mirror, unreadable release feed).

package cli

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

func runFleetFixture(t *testing.T, f *fixtureAPI) func(args ...string) string {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	t.Cleanup(func() { humanFriendly = false })
	return func(args ...string) string {
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errOut.String())
		}
		return out.String()
	}
}

func TestDriftNoAllClearWhenNothingCompared(t *testing.T) {
	// The feed answers without a release list, so it is unreadable, and the
	// mirror holds no roaming clients: nothing can be compared.
	f := &fixtureAPI{releases: []string{`{}`}}
	run := runFleetFixture(t, f)
	db := t.TempDir() + "/mirror.db"
	run("sync", "--resources", "user-agents", "--db", db, "--json")

	human := run("agents", "drift", "--db", db, "--human-friendly")
	if strings.Contains(human, "No client org has roaming clients behind") {
		t.Fatalf("drift printed an all-clear with 0 clients scanned and no feed:\n%s", human)
	}
	if !strings.Contains(human, "nothing was compared") {
		t.Fatalf("drift did not say nothing was compared:\n%s", human)
	}
	if strings.Contains(human, "map[") {
		t.Fatalf("drift printed a raw Go map:\n%s", human)
	}

	var v driftView
	if err := json.Unmarshal([]byte(run("agents", "drift", "--db", db, "--json")), &v); err != nil {
		t.Fatalf("drift json: %v", err)
	}
	if v.ReleasesSource != "unavailable" {
		t.Fatalf("releases_source = %q, want unavailable", v.ReleasesSource)
	}
	if !strings.Contains(v.Note, "Release feed unavailable") || strings.Contains(v.Note, "lists no non-white-label") {
		t.Fatalf("note = %q: an unread feed must not be described as an empty feed", v.Note)
	}
}

func TestDriftAllClearOnlyAfterRealComparison(t *testing.T) {
	f := &fixtureAPI{
		agents:   []string{`{"id":"a1","attributes":{"hostname":"PC-1","agent_type":"windows","agent_version":"3.2.0","status":"active"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`},
		releases: []string{`{"user_agent_releases":[{"agent_type":"windows","architecture":"x86_64","agent_version":"3.2.0","white_label":false,"release_channels":[{"value":"stable"}]}]}`},
	}
	run := runFleetFixture(t, f)
	db := t.TempDir() + "/mirror.db"
	run("sync", "--resources", "user-agents", "--db", db, "--json")

	human := run("agents", "drift", "--db", db, "--human-friendly")
	if !strings.Contains(human, "latest windows 3.2.0") {
		t.Fatalf("drift did not render latest as type/version pairs:\n%s", human)
	}
	if !strings.Contains(human, "No client org has roaming clients behind the latest release.") {
		t.Fatalf("a compared, current fleet should get the all-clear:\n%s", human)
	}
}

func TestStaleNoAllClearOnEmptyMirror(t *testing.T) {
	run := runFleetFixture(t, &fixtureAPI{})
	db := t.TempDir() + "/mirror.db"
	run("sync", "--resources", "user-agents", "--db", db, "--json")

	human := run("agents", "stale", "--db", db, "--human-friendly")
	if strings.Contains(human, "No roaming clients silent for more than") {
		t.Fatalf("stale printed an all-clear over an empty mirror:\n%s", human)
	}
	if !strings.Contains(human, "nothing was checked") {
		t.Fatalf("stale did not say nothing was checked:\n%s", human)
	}
}
