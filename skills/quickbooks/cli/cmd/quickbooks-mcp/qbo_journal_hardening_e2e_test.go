// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written: guards handfixes.json journal-credential-hardening. These run
// the real binary, with the credential kit started, because the refusal of a
// held value at a file sink is the kit's and an in-process test never starts it.

package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/learn"
)

const (
	// qboJournalPlanted stands in for a credential put where a session label, a flag
	// name or a stored name goes. Mixed case, because the learn loop lowercases
	// what it stores.
	qboJournalPlanted = "Zq7F3a9C0e5B1d42jhCredX"
	// qboJournalProbe is held by nothing, so it is stored: it proves a sink is reached.
	qboJournalProbe = "jhprobe7kq2xwzm"
)

// qboJournalHome is a fresh home with the learn loop on.
func qboJournalHome(t *testing.T) (string, []string) {
	t.Helper()
	home := testenv.Isolate(t)
	return home, []string{
		"QUICKBOOKS_BASE_URL=" + qboE2EBaseURL, "QUICKBOOKS_NO_LEARN=", "QUICKBOOKS_LEARN_NO_CAPTURE=",
		"QUICKBOOKS_LEARN_SESSION=", "PRINTING_PRESS_VERIFY=", "PRINTING_PRESS_DOGFOOD=",
	}
}

// qboFilesCarrying lists every file under root holding needle, in any case or
// in an encoding qboLeaks knows.
func qboFilesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(needle)) || qboLeaks(string(b), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// qboJournalEntries reads every entry the run journaled.
func qboJournalEntries(t *testing.T, root string) []learn.JournalEntry {
	t.Helper()
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasPrefix(d.Name(), "journal-") && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	var out []learn.JournalEntry
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var e learn.JournalEntry
			if json.Unmarshal([]byte(line), &e) == nil {
				out = append(out, e)
			}
		}
	}
	return out
}

// Each path a credential could take into the journal, held by the kit and not:
// a value that is not held has no output filter to stop it, so the journal must
// not keep the text at all.
func TestQBOJournalKeepsNoTypedCredential(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; qboJournalPlanted unless set
		noShape  bool   // every token after "--" is an operand
		placed   bool   // argv_shape keeps only the placeholder
	}{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "typed as a flag name", args: []string{"version", "--" + qboJournalPlanted}, placed: true},
		{name: "typed as a flag name with a value", args: []string{"version", "--" + qboJournalPlanted + "=x"}, placed: true},
		{name: "typed as a short flag", args: []string{"version", "-" + qboJournalPlanted}, placed: true},
		{name: "after the end of flags", args: []string{"version", "--", "--" + qboJournalPlanted}, noShape: true},
		// Within 3 edits of a declared flag, so Cobra suggests it.
		{name: "one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x", placed: true},
		{name: "a declared name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
		{name: "teach with no query", args: []string{"teach", "--", "--" + qboJournalPlanted}},
		{name: "playbook amend with no query", args: []string{"playbook", "amend", "--", "--" + qboJournalPlanted}},
	} {
		for _, held := range []bool{false, true} {
			name := tc.name
			if held {
				name += " (held)"
			}
			t.Run(name, func(t *testing.T) {
				home, env := qboJournalHome(t)
				if held {
					env = append(env, "QUICKBOOKS_ACCESS_TOKEN="+qboJournalPlanted)
				}
				if tc.override {
					env = append(env, "QUICKBOOKS_LEARN_SESSION="+qboJournalPlanted)
				}
				qboRun(t, env, "", tc.args...)
				entries := qboJournalEntries(t, home)
				if len(entries) != 1 {
					t.Fatalf("want 1 journal entry, got %d: a run that journals nothing proves nothing", len(entries))
				}
				e := entries[0]
				if tc.override && !strings.HasPrefix(e.SessionKey, "o:") {
					t.Errorf("session_key = %q, want the hashed override", e.SessionKey)
				}
				if tc.noShape && len(e.ArgvShape) != 0 {
					t.Errorf("argv_shape = %v, want none: a token after -- is an operand", e.ArgvShape)
				}
				if tc.placed && e.ArgvShape["<undeclared>"] != "redacted" {
					t.Errorf("argv_shape = %v, want the placeholder", e.ArgvShape)
				}
				needle := tc.needle
				if needle == "" {
					needle = qboJournalPlanted
				}
				if hits := qboFilesCarrying(t, home, needle); len(hits) > 0 {
					t.Errorf("the credential reached %v", hits)
				}
			})
		}
	}
}

// Two override values stay two sessions, and one value stays one.
func TestQBOJournalOverrideStillGroups(t *testing.T) {
	home, env := qboJournalHome(t)
	for _, session := range []string{"session-one", "session-one", "session-two"} {
		qboRun(t, append(env, "QUICKBOOKS_LEARN_SESSION="+session), "", "version")
	}
	e := qboJournalEntries(t, home)
	if len(e) != 3 || e[0].SessionKey != e[1].SessionKey || e[0].SessionKey == e[2].SessionKey {
		t.Errorf("session keys %+v: want one group of two and one of one", e)
	}
}

// Text the program stores as a name by design is refused when it is a held
// credential, in every form the output filter knows. The probe proves each
// command reaches a file, so a command that stores nothing cannot pass.
func TestQBOHeldCredentialIsRefusedAsAStoredName(t *testing.T) {
	for _, tc := range []struct {
		name string
		args func(v string) []string
	}{
		{"profile name", func(v string) []string { return []string{"profile", "save", "--json", "--", v} }},
		{"profile description", func(v string) []string {
			return []string{"profile", "save", "jh-profile", "--json", "--description", v}
		}},
		{"recall query", func(v string) []string { return []string{"recall", "--", v} }},
		{"teach query", func(v string) []string {
			return []string{"teach", "--query", v, "--resource-type", "items", "--resource", "jh-resource"}
		}},
		{"teach resource", func(v string) []string {
			return []string{"teach", "--query", "find items", "--resource-type", "items", "--resource", v}
		}},
		{"teach notes", func(v string) []string {
			return []string{"teach", "--query", "find items", "--resource-type", "items", "--resource", "jh-resource", "--notes", v}
		}},
		{"teach pattern", func(v string) []string {
			return []string{"teach-pattern", "--query-template", v + " in {entity}", "--resource-template", "GROUP-{entity:category}", "--resource-type", "items", "--entity-kind", "category"}
		}},
		{"entity alias", func(v string) []string {
			return []string{"teach-lookup", "--kind", "category", "--canonical", "jh canonical", "--value", v}
		}},
		{"feedback text", func(v string) []string { return []string{"feedback", v} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, env := qboJournalHome(t)
			out := qboRun(t, env, "", tc.args(qboJournalProbe)...)
			if len(qboFilesCarrying(t, home, qboJournalProbe)) == 0 {
				t.Fatalf("the probe reached no file, so this proves nothing about the sink: %s", out)
			}
			home, env = qboJournalHome(t)
			out = qboRun(t, append(env, "QUICKBOOKS_ACCESS_TOKEN="+qboJournalPlanted), "", tc.args(qboJournalPlanted)...)
			if hits := qboFilesCarrying(t, home, qboJournalPlanted); len(hits) > 0 {
				t.Errorf("the held credential reached %v: %s", hits, out)
			}
		})
	}
}
