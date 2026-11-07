// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening.

package learn_test

import (
	"datto-saas-pp-cli/internal/credkit"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datto-saas-pp-cli/internal/cliutil"
	"datto-saas-pp-cli/internal/cliutil/testenv"
	"datto-saas-pp-cli/internal/learn"
)

// Credential stand-ins an operator could put where a session label, a flag
// name or an operand goes: a Blackpoint-shaped API key, a short numeric one
// (a base-URL password like https://u:482915@...), and one that reads as a
// typo of the declared --compact, so Cobra suggests a flag for it.
const (
	journalHeldLong  = "df_live_7c3e9a1b5d2f4a6e8b0c"
	journalHeldShort = "482915"
	journalHeldTypo  = "compactx1"
)

// hardeningHome isolates every user directory the CLI resolves (config,
// data, state, cache) in one throwaway home, clears every journal switch
// and returns the home, so a scan of it sees every file a run wrote.
func hardeningHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, name := range []string{
		cliutil.VerifyEnvVar, cliutil.DogfoodEnvVar,
		"DATTO_SAAS_NO_LEARN", "DATTO_SAAS_LEARN_NO_CAPTURE", "DATTO_SAAS_LEARN_SESSION",
		"DATTO_SAAS_FEEDBACK_ENDPOINT", "DATTO_SAAS_FEEDBACK_AUTO_SEND",
	} {
		t.Setenv(name, "")
	}
	for _, name := range learn.JournalHarnessSessionEnvVars {
		t.Setenv(name, "")
	}
	return home
}

// filesCarrying lists every file under root whose bytes contain needle.
func filesCarrying(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- temp test path
		if err != nil {
			return err
		}
		if strings.Contains(string(b), needle) {
			rel, _ := filepath.Rel(root, p)
			hits = append(hits, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

// TestJournalCredentialHardening drives every path a credential could take
// into the learn journal (the session override, a flag name, a flag after
// "--", an operand, a label) and into the teach log, the learnings audit,
// the learn store and the feedback ledger, and fails if the credential
// reaches any file under the test home. The structural paths (session key,
// argv shape, teach.log argument echo) must hold even for a value nobody
// holds. Every run must journal exactly one entry, so a run that writes nothing
// cannot pass. An operand or label that repeats a held value is the credential
// kit's: TestJournalLineThatEchoesAHeldValueIsNotWritten.
func TestJournalCredentialHardening(t *testing.T) {
	type hardeningCase struct {
		name     string
		override bool                         // the value is DATTO_SAAS_LEARN_SESSION
		args     func(secret string) []string // the argv
		noShape  bool                         // every token after "--" is an operand
		secret   string                       // a fixed value instead of each held one
	}
	cases := []hardeningCase{
		{name: "learn session override", override: true, args: func(string) []string { return []string{"version"} }},
		{name: "credential typed as a flag name", args: func(s string) []string { return []string{"version", "--" + s} }},
		{name: "credential typed as a flag name with a value", args: func(s string) []string { return []string{"version", "--" + s + "=x"} }},
		{name: "credential typed as a short flag", args: func(s string) []string { return []string{"version", "-" + s} }},
		{name: "credential after the end of flags", args: func(s string) []string { return []string{"version", "--", "--" + s} }, noShape: true},
		{name: "teach with no query", args: func(s string) []string { return []string{"teach", s} }},
		{name: "playbook amend with no query", args: func(s string) []string { return []string{"playbook", "amend", s} }},
	}
	for _, secret := range []string{journalHeldLong, journalHeldShort} {
		for _, tc := range cases {
			value := secret
			if tc.secret != "" {
				if secret != journalHeldLong {
					continue
				}
				value = tc.secret
			}
			name := tc.name + "/" + value
			t.Run(name, func(t *testing.T) {
				home := hardeningHome(t)
				if tc.override {
					t.Setenv("DATTO_SAAS_LEARN_SESSION", value)
				}
				_ = runCLI(t, tc.args(value)...)
				entries := readAllJournalEntries(t)
				if len(entries) != 1 {
					t.Fatalf("want 1 journal entry, got %d (a run that journals nothing proves nothing)", len(entries))
				}
				if tc.override && !strings.HasPrefix(entries[0].SessionKey, "o:") {
					t.Errorf("session_key = %q, want the hashed override (o:)", entries[0].SessionKey)
				}
				if tc.noShape && len(entries[0].ArgvShape) != 0 {
					t.Errorf("argv_shape = %v, want empty: every token after -- is an operand", entries[0].ArgvShape)
				}
				if hits := filesCarrying(t, home, value); len(hits) > 0 {
					t.Errorf("the credential reached %v; journal:\n%s", hits, journalRawBytes(t))
				}
			})
		}
	}
}

// TestJournalLineThatEchoesAHeldValueIsNotWritten: the journal writes through
// the credential kit's file sink, so a held value in any field fails closed:
// nothing is written and the append reports the refusal.
func TestJournalLineThatEchoesAHeldValueIsNotWritten(t *testing.T) {
	for _, secret := range []string{journalHeldLong, journalHeldShort} {
		t.Run(secret, func(t *testing.T) {
			home := hardeningHome(t)
			credkit.Hold("test:journal", secret)
			err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"items", "x" + secret}, ExitCode: 0})
			if err != nil && !errors.Is(err, credkit.ErrWithheld) {
				t.Errorf("AppendJournalEntry = %v", err)
			}
			if hits := filesCarrying(t, home, secret); len(hits) > 0 {
				t.Errorf("the held value reached %v", hits)
			}
			if n := len(readAllJournalEntries(t)); n != 0 {
				t.Errorf("want no journal entry, got %d", n)
			}
		})
	}
}

// TestJournalFailedFlagKeepsOnlyNearMisses: a failed flag is kept as typed
// only when it is a near miss of a declared flag (so the flag-correction
// loop still pairs it with the retry), never when it is no typo of anything
// the command declares, which is what a credential typed as a flag looks like.
func TestJournalFailedFlagKeepsOnlyNearMisses(t *testing.T) {
	declared := []string{"date", "limit", "json", "compact"}
	cases := []struct {
		failed, suggested, want string
	}{
		{failed: "--dat", want: "--dat"},
		{failed: "--limt", want: "--limt"},
		{failed: "--jso", suggested: "--json", want: "--jso"},
		{failed: "--zzz", suggested: "--json", want: "--zzz"},
		{failed: "--" + journalHeldLong, want: ""},
		{failed: "--" + journalHeldShort, want: ""},
		{failed: "--sport", want: ""},
		{failed: "", want: ""},
	}
	for _, tc := range cases {
		if got := learn.JournalFailedFlag(tc.failed, tc.suggested, declared); got != tc.want {
			t.Errorf("JournalFailedFlag(%q, %q) = %q, want %q", tc.failed, tc.suggested, got, tc.want)
		}
	}
}
