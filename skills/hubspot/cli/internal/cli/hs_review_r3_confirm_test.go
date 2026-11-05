// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex round 1 (publish lane, 2026-10-24): a confirmation is never taken
// from a saved run profile, on any command that asks for one.

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
)

func TestR3ProfileYesNeverConfirmsBulkUpdate(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"status":"COMPLETE","results":[{"id":"123","properties":{"firstname":"Changed"}}]}`)
	path := r1BulkStore(t)
	if _, err := r1Run(t, "", "profile", "save", "always-yes", "--yes", "--compact"); err != nil {
		t.Fatalf("profile save: %v", err)
	}
	if _, err := r1Run(t, `{"id":"123","firstname":"Changed"}`+"\n", "contacts", "bulk-update", "--jsonl", "--profile", "always-yes", "--db", path); err == nil || rs.writes() != 0 {
		t.Fatalf("err=%v writes=%d: a --yes saved in a profile approved a live bulk update", err, rs.writes())
	}
	if _, err := r1Run(t, `{"id":"123","firstname":"Changed"}`+"\n", "contacts", "bulk-update", "--jsonl", "--yes", "--db", path); err != nil || rs.writes() != 1 {
		t.Fatalf("err=%v writes=%d: a typed --yes must still send", err, rs.writes())
	}
}

func TestR3ProfileNeverStoresYes(t *testing.T) {
	testenv.Isolate(t)
	if _, err := r1Run(t, "", "profile", "save", "p", "--yes", "--compact"); err != nil {
		t.Fatalf("profile save: %v", err)
	}
	p, err := GetProfile("p")
	if err != nil {
		t.Fatal(err)
	}
	if p != nil {
		for k := range p.Values {
			if strings.EqualFold(k, "yes") {
				t.Fatalf("profile stored %q; a confirmation must not be saved", k)
			}
		}
	}
}

// Adversarial P2 (publish lane): a JSONL line that does not parse is a
// rejected row, so it fails the run like any other rejected row.
func TestR3UnparseableJSONLLineFailsTheRun(t *testing.T) {
	testenv.Isolate(t)
	newR1Server(t, 200, `{"status":"COMPLETE","results":[{"id":"1","properties":{"firstname":"X"}}]}`)
	path := r1BulkStore(t)
	in := `{"id":"1","firstname":"X"}` + "\n" + `not json` + "\n"
	if out, err := r1Run(t, in, "contacts", "bulk-update", "--jsonl", "--yes", "--db", path); err == nil {
		t.Fatalf("an unparseable line must fail the run; out=%s", out)
	}
}

// Twoclass REC 1 (2026-10-26): a run profile never carries an approval flag.
// A profile file holding --confirm and --digest for a bulk update over 100
// rows must not approve it; the same values typed on the command do.
func TestR3ProfileConfirmDigestNeverApproveBulkUpdate(t *testing.T) {
	testenv.Isolate(t)
	rs := newR1Server(t, 200, `{"status":"COMPLETE","results":[]}`)
	path := r1BulkStore(t)
	var in strings.Builder
	for i := 1; i <= 101; i++ {
		fmt.Fprintf(&in, `{"id":"%d","firstname":"X"}`+"\n", i)
	}
	out, err := r1Run(t, in.String(), "contacts", "bulk-update", "--jsonl", "--db", path, "--json")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	digest := regexp.MustCompile(`blast-[0-9a-f]+`).FindString(out)
	if digest == "" || rs.writes() != 0 {
		t.Fatalf("want a persisted digest and no writes; writes=%d out=%s", rs.writes(), out)
	}
	s, err := loadProfileStore()
	if err != nil {
		t.Fatal(err)
	}
	s.Profiles["approve"] = Profile{Name: "approve", Values: map[string]string{"confirm": "101", "digest": digest, "compact": "true"}}
	if err := saveProfileStore(s); err != nil {
		t.Fatal(err)
	}
	_, _ = r1Run(t, in.String(), "contacts", "bulk-update", "--jsonl", "--profile", "approve", "--db", path, "--json")
	if n := rs.writes(); n != 0 {
		t.Fatalf("a profile's --confirm/--digest approved a %d-row bulk update: %d write request(s) sent", 101, n)
	}
	// Control: the same digest typed on the command is a valid approval, so
	// the refusal above came from the profile rule, not a stale digest.
	_, _ = r1Run(t, in.String(), "contacts", "bulk-update", "--jsonl", "--confirm", "101", "--digest", digest, "--db", path, "--json")
	if rs.writes() == 0 {
		t.Fatal("control: a typed --confirm/--digest must send the update")
	}
}

// Every flag outside the allowlist is ignored when a profile file carries it.
func TestR3ProfileAppliesOnlyAllowlistedFlags(t *testing.T) {
	testenv.Isolate(t)
	for _, approval := range []string{"yes", "confirm", "digest", "dry-run", "force", "allow-partial-failure", "idempotent", "ignore-missing", "type", "archived", "properties", "deliver"} {
		if runProfileAllowedFlags[approval] {
			t.Errorf("--%s is on the run-profile allowlist; a profile must not approve, shape or redirect a write", approval)
		}
	}
}
