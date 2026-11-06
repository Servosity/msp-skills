// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"domain-cert-tracker-pp-cli/internal/cliutil"
)

// Under MCP, a write runs only with confirm: true. Review input: with
// default MCP settings teach_playbook {"query":"find domains","notes":
// "example note"} persisted a playbook without confirm.
func TestTeachPlaybookUnderMCPNeedsConfirm(t *testing.T) {
	withTempLearnHome(t)
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	out, _, err := runRootArgs(t, "--json", "teach-playbook", "--query", "find domains", "--notes", "example note")
	if err == nil || !strings.Contains(out, "confirm_required") {
		t.Fatalf("teach-playbook under MCP without confirm ran: err=%v out=%s", err, out)
	}
	list, _, err := runRootArgs(t, "--json", "playbook", "list")
	if err != nil || strings.Contains(list, "example note") {
		t.Fatalf("unconfirmed teach-playbook reached the store: err=%v %s", err, list)
	}
	if _, _, err := runRootArgs(t, "teach-playbook", "--query", "find domains", "--notes", "example note", "--confirm"); err != nil {
		t.Fatalf("teach-playbook --confirm under MCP: %v", err)
	}
	list, _, _ = runRootArgs(t, "--json", "playbook", "list")
	if !strings.Contains(list, "example note") {
		t.Fatalf("confirmed teach-playbook did not persist: %s", list)
	}
}

// The command line keeps its behaviour: no MCP child marker, no confirm.
func TestTeachPlaybookOnCommandLineNeedsNoConfirm(t *testing.T) {
	withTempLearnHome(t)
	if _, _, err := runRootArgs(t, "teach-playbook", "--query", "find domains", "--notes", "example note"); err != nil {
		t.Fatalf("teach-playbook on the command line: %v", err)
	}
}

// Codex publish pass 3 round 1: an unconfirmed MCP write changes nothing, the
// store's schema and seed rows included. Every write command previews before
// the store exists; a read of it would create nothing either.
func TestUnconfirmedMCPWritesLeaveNoStore(t *testing.T) {
	cases := map[string][]string{
		"teach playbook": {"teach-playbook", "--query", "find domains", "--notes", "example note"},
		"clients set":    {"clients", "set", "Acme", "--domain", "example.com"},
		"clients remove": {"clients", "remove", "Acme", "--domain", "example.com"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			trackerHome(t)
			t.Setenv(cliutil.MCPChildEnvVar, "1")
			_, _, _ = runRootArgs(t, args...)
			db := defaultDBPath(trackerCLIName)
			if _, err := os.Stat(db); !os.IsNotExist(err) {
				t.Fatalf("an unconfirmed write touched the store %s: stat err=%v", db, err)
			}
		})
	}
}

// An existing store keeps its schema version through an unconfirmed write.
func TestUnconfirmedMCPRemovePreviewsWithoutWriteHandle(t *testing.T) {
	trackerHome(t)
	if _, _, err := runRootArgs(t, "clients", "set", "Acme", "--domain", "example.com", "--yes"); err != nil {
		t.Fatal(err)
	}
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	before, err := os.ReadFile(defaultDBPath(trackerCLIName))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := runRootArgs(t, "--json", "clients", "remove", "Acme", "--domain", "example.com")
	if err == nil || !strings.Contains(out, `"confirm_required": true`) {
		t.Fatalf("clients remove without confirm: err=%v out=%s", err, out)
	}
	after, err := os.ReadFile(defaultDBPath(trackerCLIName))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("an unconfirmed clients remove changed the store file")
	}
}
