// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"connectwise-automate-pp-cli/internal/store"
)

// seedStore points HOME at a temp dir so defaultDBPath resolves there, then
// writes a small fixture fleet into the generic resources table (which the
// transcendence commands read via store.List). Returns nothing; the commands
// resolve the same path through defaultDBPath.
func seedStore(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	dbPath := defaultDBPath("connectwise-automate-cli")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	recentISO := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	staleISO := time.Now().UTC().AddDate(0, 0, -90).Format(time.RFC3339)

	put := func(rt, id, body string) {
		if err := s.Upsert(rt, id, json.RawMessage(body)); err != nil {
			t.Fatalf("seed %s/%s: %v", rt, id, err)
		}
	}
	put("clients", "1", `{"Id":1,"Name":"Acme Corp"}`)
	put("clients", "2", `{"Id":2,"Name":"Globex"}`)
	put("locations", "5", `{"Id":5,"ClientId":1,"Name":"Acme HQ"}`)
	put("computers", "10", `{"Id":10,"ComputerName":"ACME-PC1","ClientId":1,"Status":"Online","OperatingSystemName":"Windows 11 Pro","LastContact":"`+recentISO+`"}`)
	put("computers", "11", `{"Id":11,"ComputerName":"ACME-OLD","ClientId":1,"Status":"Offline","OperatingSystemName":"Windows 7 Pro","LastContact":"`+staleISO+`"}`)
	put("computers", "20", `{"Id":20,"ComputerName":"GLOBEX-SRV","ClientId":2,"Status":"Online","OperatingSystemName":"Windows Server 2012 R2","LastContact":"`+recentISO+`"}`)
	put("alerts", "100", `{"Id":100,"ComputerId":11,"Priority":4,"Message":"disk full","CreatedDate":"`+recentISO+`"}`)
	put("patching", "200", `{"Id":200,"ComputerId":10,"Status":"Installed","InstallDate":"`+recentISO+`"}`)
	put("patching", "201", `{"Id":201,"ComputerId":11,"Status":"Failed","InstallDate":"`+recentISO+`"}`)
}

// nospace strips whitespace so numeric "key": value assertions are insensitive
// to the output formatter's pretty-printing (space after colon, indentation).
func nospace(s string) string {
	return strings.NewReplacer(" ", "", "\n", "", "\t", "").Replace(s)
}

// run executes a transcendence command (built from its constructor) with
// agent-mode output and returns stdout.
func run(t *testing.T, build func(*rootFlags) *cobra.Command, args ...string) string {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := build(flags)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s: %v\noutput: %s", cmd.Name(), err, buf.String())
	}
	return buf.String()
}

func TestFleetHealthCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelFleetHealthCmd)
	if !strings.Contains(out, "Acme Corp") {
		t.Fatalf("fleet-health output missing Acme Corp: %s", out)
	}
	// Acme has the offline+stale Win7 box and the open alert.
	if !strings.Contains(nospace(out), `"offline":1`) || !strings.Contains(nospace(out), `"open_alerts":1`) {
		t.Fatalf("fleet-health did not roll up Acme offline/alert counts: %s", out)
	}
}

func TestStaleAgentsCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelStaleAgentsCmd, "--days", "30")
	if !strings.Contains(out, "ACME-OLD") {
		t.Fatalf("stale-agents missing ACME-OLD: %s", out)
	}
	if strings.Contains(out, "ACME-PC1") {
		t.Fatalf("stale-agents should not list the freshly-contacted ACME-PC1: %s", out)
	}
}

func TestPatchComplianceCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelPatchComplianceCmd)
	// Acme: 1 installed / 2 total = 50%.
	if !strings.Contains(out, "Acme Corp") || !strings.Contains(nospace(out), `"compliance_pct":50`) {
		t.Fatalf("patch-compliance did not compute Acme 50%%: %s", out)
	}
}

func TestClientRollupCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelClientRollupCmd, "Acme")
	if !strings.Contains(out, "Acme Corp") {
		t.Fatalf("client-rollup missing Acme: %s", out)
	}
	if strings.Contains(out, "Globex") {
		t.Fatalf("client-rollup filter 'Acme' leaked Globex: %s", out)
	}
}

func TestAlertTriageCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelAlertTriageCmd, "--min-priority", "3")
	if !strings.Contains(out, "disk full") || !strings.Contains(out, "Acme Corp") {
		t.Fatalf("alert-triage missing the priority-4 Acme alert: %s", out)
	}
}

func TestOSInventoryCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelOsInventoryCmd, "--eol-only")
	if !strings.Contains(out, "Windows 7 Pro") || !strings.Contains(out, "Windows Server 2012 R2") {
		t.Fatalf("os-inventory --eol-only missing EOL OSes: %s", out)
	}
	if strings.Contains(out, "Windows 11 Pro") {
		t.Fatalf("os-inventory --eol-only leaked the non-EOL Windows 11: %s", out)
	}
}

func TestSinceCommand(t *testing.T) {
	seedStore(t)
	out := run(t, newNovelSinceCmd, "--hours", "24")
	if !strings.Contains(nospace(out), `"patches_installed":1`) {
		t.Fatalf("since did not count the recent installed patch: %s", out)
	}
	if !strings.Contains(out, "disk full") {
		t.Fatalf("since missing the recent alert: %s", out)
	}
}

func TestTranscendCommandsTolerateEmptyStore(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no seed: empty/just-created store
	builders := []func(*rootFlags) *cobra.Command{
		newNovelFleetHealthCmd, newNovelStaleAgentsCmd, newNovelPatchComplianceCmd,
		newNovelClientRollupCmd, newNovelAlertTriageCmd, newNovelOsInventoryCmd, newNovelSinceCmd,
	}
	for _, b := range builders {
		flags := &rootFlags{asJSON: true}
		cmd := b(flags)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs(nil)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s errored on empty store (want graceful exit): %v", cmd.Name(), err)
		}
	}
}
