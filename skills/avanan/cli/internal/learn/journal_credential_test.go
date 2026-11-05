// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry "journal-credential-hardening".

package learn_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/credkit"
	"avanan-pp-cli/internal/learn"
)

// Low-entropy stand-ins for credentials. Only journalHeldCredential is ever
// held (credkit.Hold is process-wide and never forgets), so no other
// test may use it; the other two are never held.
const (
	journalSessionCredential = "pp2sess-eee-fff-3333"
	journalHeldCredential    = "zq7held-aaa-bbb-1111"
	journalUnheldFlagName    = "mm4flag-ccc-ddd-2222"
)

// journalCredentialForms is v as written and in the obvious encodings a
// reader could undo: hex in either case, base64 in either alphabet with or
// without padding, and URL escaping.
func journalCredentialForms(v string) []string {
	h := hex.EncodeToString([]byte(v))
	return []string{
		v,
		h,
		strings.ToUpper(h),
		base64.StdEncoding.EncodeToString([]byte(v)),
		base64.RawStdEncoding.EncodeToString([]byte(v)),
		base64.URLEncoding.EncodeToString([]byte(v)),
		base64.RawURLEncoding.EncodeToString([]byte(v)),
		url.QueryEscape(v),
	}
}

// journalFilesCarrying lists every file under home whose bytes carry v in
// any journalCredentialForms form, or (when v is held) in any form the
// output filter decodes (cliutil.HeldScan).
func journalFilesCarrying(t *testing.T, home, v string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- temp test path
		if err != nil {
			return err
		}
		text := string(b)
		for _, form := range journalCredentialForms(v) {
			if strings.Contains(text, form) {
				hits = append(hits, p+" ("+form+")")
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", home, err)
	}
	return hits
}

// TestJournalCredential_SessionOverrideIsHashed proves the
// AVANAN_LEARN_SESSION override never reaches the journal raw or in an
// obvious encoding: it is stored as the harness-id hash under its own "o:"
// prefix, so entries of one session still share a key.
func TestJournalCredential_SessionOverrideIsHashed(t *testing.T) {
	home := withJournalHome(t)
	t.Setenv("AVANAN_LEARN_SESSION", journalSessionCredential)
	for i := 0; i < 2; i++ {
		if err := runCLI(t, "version"); err != nil {
			t.Fatalf("version: %v", err)
		}
	}
	t.Setenv("AVANAN_LEARN_SESSION", "")
	t.Setenv("CODEX_SESSION_ID", journalSessionCredential)
	if err := runCLI(t, "version"); err != nil {
		t.Fatalf("version: %v", err)
	}

	sum := sha256.Sum256([]byte(journalSessionCredential))
	hashed := hex.EncodeToString(sum[:8])
	entries := readAllJournalEntries(t)
	if len(entries) != 3 {
		t.Fatalf("want 3 journal entries, got %d (a run that journals nothing proves nothing)", len(entries))
	}
	if entries[0].SessionKey != "o:"+hashed || entries[1].SessionKey != entries[0].SessionKey {
		t.Errorf("override session keys = %q, %q; want both o:%s (hashed like a harness id, still grouped)",
			entries[0].SessionKey, entries[1].SessionKey, hashed)
	}
	if entries[2].SessionKey != "h:"+hashed {
		t.Errorf("harness session key = %q, want h:%s (the override keeps its own prefix)", entries[2].SessionKey, hashed)
	}
	if hits := journalFilesCarrying(t, home, journalSessionCredential); len(hits) > 0 {
		t.Errorf("the session override reached %v", hits)
	}
}

// TestJournalCredential_UndeclaredFlagIsPlaceholder proves argv_shape keeps
// only flag names the invoked command declares: an undeclared name, or the
// value after one, held or not, is recorded only as the fixed placeholder,
// and nothing after "--" is a flag name. A held credential in either place
// reaches no file.
func TestJournalCredential_UndeclaredFlagIsPlaceholder(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("config:unclassified", journalHeldCredential)
	cases := []struct {
		name        string
		args        []string
		placeholder bool // an undeclared flag precedes "--", so the placeholder is recorded
		held        bool // the held credential is typed, so it must reach no file
	}{
		{name: "held credential as a flag name", args: []string{"version", "--" + journalHeldCredential}, placeholder: true, held: true},
		{name: "held credential as a flag name with a value", args: []string{"version", "--" + journalHeldCredential + "=x"}, placeholder: true, held: true},
		{name: "held credential as a short flag", args: []string{"version", "-" + journalHeldCredential}, placeholder: true, held: true},
		{name: "held credential as an undeclared flag's value", args: []string{"version", "--not-a-declared-flag", journalHeldCredential}, placeholder: true, held: true},
		{name: "held credential as an undeclared flag's inline value", args: []string{"version", "--not-a-declared-flag=" + journalHeldCredential}, placeholder: true, held: true},
		{name: "held credential after the end of flags", args: []string{"version", "--", "--" + journalHeldCredential}, held: true},
		{name: "unheld undeclared flag name", args: []string{"version", "--" + journalUnheldFlagName}, placeholder: true},
		{name: "declared flag beside an undeclared one", args: []string{"version", "--json", "--" + journalUnheldFlagName}, placeholder: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := withJournalHome(t)
			_ = runCLI(t, tc.args...)
			if tc.held { // the kit withholds the whole entry; nothing may carry the value
				if hits := journalFilesCarrying(t, home, journalHeldCredential); len(hits) > 0 {
					t.Errorf("the held credential reached %v", hits)
				}
				return
			}
			entries := readAllJournalEntries(t)
			if len(entries) != 1 {
				t.Fatalf("want 1 journal entry, got %d (a run that journals nothing proves nothing)", len(entries))
			}
			shape := entries[0].ArgvShape
			for name := range shape {
				if name != "<undeclared>" && name != "json" {
					t.Errorf("argv_shape key %q is not a name the command declares; shape = %v", name, shape)
				}
			}
			if got := shape["<undeclared>"]; tc.placeholder && got != "redacted" {
				t.Errorf("argv_shape[<undeclared>] = %q, want redacted; shape = %v", got, shape)
			}
			if !tc.placeholder && len(shape) != 0 {
				t.Errorf("argv_shape = %v, want empty: every token after -- is an operand", shape)
			}
		})
	}
}

func TestJournalCredential_HeldOperatorTextIsRefused(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("config:unclassified", journalHeldCredential)
	encoded := base64.StdEncoding.EncodeToString([]byte(journalHeldCredential))

	t.Run("recall family and unresolved entities", func(t *testing.T) {
		home := withJournalHome(t)
		learn.SetJournalLearnContext("widget "+journalHeldCredential, []string{journalHeldCredential, "WIDGET-X", encoded})
		learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"recall"}, ExitCode: 0})
		if hits := journalFilesCarrying(t, home, journalHeldCredential); len(hits) > 0 {
			t.Errorf("the held credential reached %v", hits)
		}
	})

	t.Run("failed flag, flag name, verb and error class", func(t *testing.T) {
		home := withJournalHome(t)
		learn.JournalInvocation(learn.JournalEntry{
			Cmd:           []string{"items", journalHeldCredential},
			ArgvShape:     map[string]string{journalHeldCredential: "str", "limit": "int"},
			ExitCode:      2,
			ErrorClass:    encoded,
			FailedFlag:    "--" + journalHeldCredential,
			SuggestedFlag: "--json",
		})
		if hits := journalFilesCarrying(t, home, journalHeldCredential); len(hits) > 0 {
			t.Errorf("the held credential reached %v", hits)
		}
	})

	t.Run("a field no refusal names is withheld whole", func(t *testing.T) {
		home := withJournalHome(t)
		learn.JournalInvocation(learn.JournalEntry{TS: journalHeldCredential, Cmd: []string{"version"}})
		if entries := readAllJournalEntries(t); len(entries) != 0 {
			t.Errorf("want the entry withheld whole, got %+v", entries)
		}
		if hits := journalFilesCarrying(t, home, journalHeldCredential); len(hits) > 0 {
			t.Errorf("the held credential reached %v", hits)
		}
	})

	t.Run("teach.log resource", func(t *testing.T) {
		home := withJournalHome(t)
		probe := learn.Warning{Code: "no_entity_overlap", Resource: "PREFIX-PROBE", Detail: "probe"}
		if err := learn.AppendTeachLogWarning("teach", "alpha widget", probe); err != nil {
			t.Fatalf("an unheld warning must be written: %v", err)
		}
		held := learn.Warning{Code: "no_entity_overlap", Resource: journalHeldCredential, Detail: "query entities [" + encoded + "]"}
		_ = learn.AppendTeachLogWarning("teach", "alpha widget", held)
		if hits := journalFilesCarrying(t, home, journalHeldCredential); len(hits) > 0 {
			t.Errorf("the held credential reached %v", hits)
		}
	})
}
