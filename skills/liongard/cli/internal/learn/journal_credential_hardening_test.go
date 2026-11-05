// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: guards handfixes.json journal-credential-hardening.

package learn_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"liongard-pp-cli/internal/cliutil"
	"liongard-pp-cli/internal/cliutil/testenv"
	"liongard-pp-cli/internal/credkit"
	"liongard-pp-cli/internal/learn"
)

// journalHardeningSecret stands in for a credential an operator put where a
// session label, a flag name or a learn-loop field goes. It shares no
// 8-character piece with anything the journal writes on its own (it has no
// run of 8 hex digits, so no hashed session key can carry one).
const journalHardeningSecret = "LgQx7R" + "k2Wv9Pz4NmTy6Hs3Jd"

// journalHardeningHome isolates every press directory in a fresh home with
// each journal switch cleared, so a run journals exactly once into it.
func journalHardeningHome(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, name := range []string{
		"PRINTING_PRESS_VERIFY", "PRINTING_PRESS_DOGFOOD",
		"LIONGARD_NO_LEARN", "LIONGARD_LEARN_NO_CAPTURE", "LIONGARD_LEARN_SESSION",
	} {
		t.Setenv(name, "")
	}
	for _, name := range learn.JournalHarnessSessionEnvVars {
		t.Setenv(name, "")
	}
	return home
}

// credentialPieces lists every file under root that carries an 8-character
// piece of needle (the kit's match unit), compared case-insensitively.
func credentialPieces(t *testing.T, root, needle string) []string {
	t.Helper()
	needle = strings.ToLower(needle)
	var pieces []string
	if len(needle) <= 8 {
		pieces = []string{needle}
	}
	for i := 0; i+8 <= len(needle); i++ {
		pieces = append(pieces, needle[i:i+8])
	}
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p) // #nosec G304 -- walks the test's own temp home
		if err != nil {
			return err
		}
		text := strings.ToLower(string(b))
		for _, piece := range pieces {
			if strings.Contains(text, piece) {
				hits = append(hits, p+" carries "+piece)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

func hashedOverride(v string) string {
	sum := sha256.Sum256([]byte(v))
	return "o:" + hex.EncodeToString(sum[:8])
}

// TestJournalCredentialHardening drives each path a credential could take into
// the learn journal through the real command tree, first not held and then
// held by the shared credential kit, and fails if any 8-character piece of it
// reaches any file under the test home. Each run must journal one entry, so a
// run that writes nothing cannot pass. It then proves every file this package
// writes goes through the kit: a held credential in a field the journal keeps
// by design (query family, unresolved entities), in a teach-log warning and in
// the derivation offset is withheld, not written.
func TestJournalCredentialHardening(t *testing.T) {
	cases := []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; the secret unless set
		noShape  bool   // nothing after "--" is a flag, so argv_shape stays empty
	}{
		{name: "learn session override", override: true, args: []string{"version"}},
		{name: "credential typed as a flag name", args: []string{"version", "--" + journalHardeningSecret}},
		{name: "credential typed as a flag name with a value", args: []string{"version", "--" + journalHardeningSecret + "=x"}},
		{name: "credential typed as a short flag", args: []string{"version", "-" + journalHardeningSecret}},
		{name: "credential after the end of flags", args: []string{"version", "--", "--" + journalHardeningSecret}, noShape: true},
		// Within 3 edits of the declared --compact, so a did-you-mean
		// suggestion exists: the typed name is still never kept.
		{name: "credential one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x"},
		{name: "declared flag name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
	}
	for _, held := range []bool{false, true} {
		if held {
			// The held set is process-global, so the held pass runs second.
			credkit.Hold("env:LIONGARD_API_KEY", journalHardeningSecret)
		}
		for _, tc := range cases {
			name := tc.name
			if held {
				name += " (held)"
			}
			t.Run(name, func(t *testing.T) {
				home := journalHardeningHome(t)
				if tc.override {
					t.Setenv("LIONGARD_LEARN_SESSION", journalHardeningSecret)
				}
				_ = runCLI(t, tc.args...)
				entries := readAllJournalEntries(t)
				if len(entries) != 1 {
					t.Fatalf("want 1 journal entry, got %d (a run that journals nothing proves nothing)", len(entries))
				}
				if tc.override && entries[0].SessionKey != hashedOverride(journalHardeningSecret) {
					t.Errorf("session_key = %q, want the hashed override %q", entries[0].SessionKey, hashedOverride(journalHardeningSecret))
				}
				if tc.noShape && len(entries[0].ArgvShape) != 0 {
					t.Errorf("argv_shape = %v, want empty: every token after -- is an operand", entries[0].ArgvShape)
				}
				needle := tc.needle
				if needle == "" {
					needle = journalHardeningSecret
				}
				if hits := credentialPieces(t, home, needle); len(hits) > 0 {
					t.Errorf("the credential reached a file: %v", hits)
				}
			})
		}
	}

	// The credential is held from here on: every write below must be routed
	// through the kit, which withholds it.
	notice := credkit.Notice(credkit.SinkFile)

	t.Run("journal fields kept by design go through the kit", func(t *testing.T) {
		home := journalHardeningHome(t)
		learn.SetJournalLearnContext("family "+journalHardeningSecret, []string{journalHardeningSecret})
		learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"recall"}, ExitCode: 0})
		if raw := journalRawBytes(t); !strings.Contains(raw, notice) {
			t.Errorf("journal = %q, want the kit's notice in place of the withheld entry", raw)
		}
		if hits := credentialPieces(t, home, journalHardeningSecret); len(hits) > 0 {
			t.Errorf("the credential reached a file: %v", hits)
		}
	})

	t.Run("teach log goes through the kit", func(t *testing.T) {
		home := journalHardeningHome(t)
		w := learn.Warning{Code: "unresolved", Resource: journalHardeningSecret, Detail: "detail " + journalHardeningSecret}
		if err := learn.AppendTeachLogWarning("teach", "a query", w); err != nil {
			t.Fatalf("AppendTeachLogWarning: %v", err)
		}
		p, err := learn.TeachLogPath()
		if err != nil {
			t.Fatalf("TeachLogPath: %v", err)
		}
		b, err := os.ReadFile(p) // #nosec G304 -- temp test path
		if err != nil {
			t.Fatalf("read teach.log: %v", err)
		}
		if !strings.Contains(string(b), notice) {
			t.Errorf("teach.log = %q, want the kit's notice in place of the withheld warning", b)
		}
		if hits := credentialPieces(t, home, journalHardeningSecret); len(hits) > 0 {
			t.Errorf("the credential reached a file: %v", hits)
		}
	})

	t.Run("derivation offset goes through the kit", func(t *testing.T) {
		home := journalHardeningHome(t)
		if err := learn.StoreJournalOffset(learn.JournalOffset{Segment: "journal-20260101.jsonl", Byte: 7}); err != nil {
			t.Fatalf("store a clean offset: %v", err)
		}
		err := learn.StoreJournalOffset(learn.JournalOffset{Segment: journalHardeningSecret, Byte: 1})
		if !errors.Is(err, credkit.ErrWithheld) {
			t.Errorf("StoreJournalOffset = %v, want the kit's withheld error", err)
		}
		if got, err := learn.LoadJournalOffset(); err != nil || got.Segment != "journal-20260101.jsonl" || got.Byte != 7 {
			t.Errorf("offset = %+v, %v; a withheld write must leave the stored offset as it was", got, err)
		}
		if hits := credentialPieces(t, home, journalHardeningSecret); len(hits) > 0 {
			t.Errorf("the credential reached a file: %v", hits)
		}
	})
}

// TestJournalArgvShapeRecordsOnlyDeclaredNames proves the declared-flags
// filter on its own, without the held-value layers: with no declared-flag
// lookup no typed name is kept, only the placeholder; with one, only the
// names it declares are; and nothing after "--" is read as a flag.
func TestJournalArgvShapeRecordsOnlyDeclaredNames(t *testing.T) {
	args := []string{"--" + journalHardeningSecret, "x", "-" + journalHardeningSecret, "--limit", "3", "--", "--json"}
	got := learn.JournalArgvShape(args, nil)
	if len(got) != 1 || got["<undeclared>"] != "redacted" {
		t.Errorf("no lookup: shape = %v, want only the <undeclared> placeholder", got)
	}
	declared := func(name string) bool { return name == "limit" || name == "json" }
	got = learn.JournalArgvShape(args, nil, declared)
	if len(got) != 2 || got["<undeclared>"] != "redacted" || got["limit"] != "int" {
		t.Errorf("declared lookup: shape = %v, want <undeclared>:redacted and limit:int only", got)
	}
	// A nil lookup declares nothing, so it cannot widen what is kept.
	if got := learn.JournalArgvShape([]string{"--limit", "3"}, nil, declared, nil); len(got) != 1 || got["<undeclared>"] != "redacted" {
		t.Errorf("nil lookup: shape = %v, want only the <undeclared> placeholder", got)
	}
}

// TestJournalTornTailRepairedThroughKit drives the tail repair, now a kit
// write: a segment another writer left without its final newline gets one
// before the next entry, so that entry stays a line of its own.
func TestJournalTornTailRepairedThroughKit(t *testing.T) {
	journalHardeningHome(t)
	dir, err := learn.JournalDir()
	if err != nil {
		t.Fatalf("JournalDir: %v", err)
	}
	today := "journal-" + time.Now().UTC().Format("20060102") + ".jsonl"
	if err := os.WriteFile(filepath.Join(dir, today), []byte(`{"ts":"2026-01-01T00:00:00Z","cmd":["torn"`), 0o600); err != nil {
		t.Fatalf("plant torn segment: %v", err)
	}
	if err := learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"fresh"}, ExitCode: 0}); err != nil {
		t.Fatalf("AppendJournalEntry: %v", err)
	}
	entries := readAllJournalEntries(t)
	if len(entries) != 1 || len(entries[0].Cmd) != 1 || entries[0].Cmd[0] != "fresh" {
		t.Fatalf("entries = %+v, want the fresh entry on a line of its own", entries)
	}
	if raw := journalRawBytes(t); !strings.HasPrefix(raw, `{"ts":"2026-01-01T00:00:00Z","cmd":["torn"`+"\n{") {
		t.Errorf("journal = %q, want the torn line ended by a newline before the fresh entry", raw)
	}
}
