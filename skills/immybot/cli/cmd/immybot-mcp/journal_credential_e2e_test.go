// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json journal-credential-hardening).

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/learn"
)

// e2eJournalBytes is every journal segment's bytes, so a run is proven to have
// reached the journal.
func e2eJournalBytes(t *testing.T) string {
	t.Helper()
	state, err := cliutil.StateDir()
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	matches, _ := filepath.Glob(filepath.Join(state, "learn", "journal-*.jsonl"))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(b)
	}
	return all.String()
}

// TestTheLearnJournalKeepsNoCredential drives each path a credential can take
// into the learn journal, not held and then held (as IMMYBOT_CLIENT_SECRET), and
// fails if it reaches any file under the run's home. Every run must reach the
// journal, so a run that journals nothing cannot pass. Operator-named text (a
// teach query, a profile's name, a recall operand) is stored by design, so it is
// driven only held: then it must be refused.
func TestTheLearnJournalKeepsNoCredential(t *testing.T) {
	e2eSkipOnWindows(t)
	cli, _ := buildE2EBinaries(t)
	secret := e2eSecret("learn-journal")
	type run struct {
		name     string
		override bool
		args     []string
		needle   string
		heldOnly bool
	}
	runs := []run{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "a flag name", args: []string{"version", "--" + secret}},
		{name: "a flag name with a value", args: []string{"version", "--" + secret + "=x"}},
		{name: "a short flag", args: []string{"version", "-" + secret}},
		{name: "after the end of flags", args: []string{"version", "--", "--" + secret}},
		{name: "one typo from a declared flag", args: []string{"version", "--jsonn"}, needle: "jsonn"},
		{name: "teach with no query", args: []string{"teach", "--", "--" + secret}},
		{name: "playbook amend with no query", args: []string{"playbook", "amend", "--", "--" + secret}},
		{name: "a teach query", args: []string{"teach", "--query", secret, "--resource", "r1", "--resource-type", "computer"}, heldOnly: true},
		{name: "a recall operand", args: []string{"recall", secret}, heldOnly: true},
		{name: "a profile's name", args: []string{"profile", "save", secret, "--select", "id"}, heldOnly: true},
		{name: "a feedback note", args: []string{"feedback", "see " + secret}, heldOnly: true},
		{name: "a learn session label that is held", override: true, args: []string{"version"}, heldOnly: true},
	}
	for _, held := range []bool{false, true} {
		for _, r := range runs {
			if r.heldOnly && !held {
				continue
			}
			name := r.name
			if held {
				name += " (held)"
			}
			t.Run(name, func(t *testing.T) {
				s := newStubImmy(t)
				home := e2eHome(t, s)
				for _, k := range append([]string{"IMMYBOT_NO_LEARN", "IMMYBOT_LEARN_NO_CAPTURE", "IMMYBOT_LEARN_SESSION", "PRINTING_PRESS_DOGFOOD"}, learn.JournalHarnessSessionEnvVars...) {
					t.Setenv(k, "")
				}
				if r.override {
					t.Setenv("IMMYBOT_LEARN_SESSION", secret)
				}
				if held {
					t.Setenv("IMMYBOT_CLIENT_SECRET", secret)
				}
				out := e2eRun(t, cli, "", r.args...)
				if e2eJournalBytes(t) == "" {
					t.Fatalf("the run reached no journal (a run that journals nothing proves nothing):\n%s", out)
				}
				needle := r.needle
				if needle == "" {
					needle = secret
				}
				if hits := e2eFilesCarrying(t, home, needle); len(hits) > 0 {
					t.Errorf("the credential reached %v", hits)
				}
			})
		}
	}
}
