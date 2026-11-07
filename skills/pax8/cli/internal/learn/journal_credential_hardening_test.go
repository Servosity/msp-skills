// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (not generated): guards the pax8 hand-fix
// journal-credential-hardening in skills/pax8/handfixes.json.

package learn_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pax8-pp-cli/internal/credkit"
	"pax8-pp-cli/internal/learn"
	"pax8-pp-cli/internal/learn/entities"
	"pax8-pp-cli/internal/learn/lookups"
	"pax8-pp-cli/internal/learn/patterns"
	"pax8-pp-cli/internal/store"
)

// jhSecret stands in for a credential an operator put where a session label
// or a flag name goes. The held pass holds it through the shared credential
// kit (handfixes.json credkit-wiring).
const jhSecret = "Zq7f3a9c0e5b1d42xJhW" // gitleaks:allow

// jhName stands in for a credential put where the learn loop stores a name
// or an operand by design (a query family, an alias, a pattern). Lowercase
// letters and digits only, so normalization cannot change it.
const jhName = "zq7f3a9c0e5b1d42xjhw"

// jhProbe is never held: a sink that stores it proves the sink was reached.
const jhProbe = "jhprobe7kq2xwzm"

// jhModeEnv selects the pass a child of this test binary runs.
const jhModeEnv = "JOURNAL_HARDENING_MODE"

// jhRunChild runs one test of this binary again in a child process with
// mode set. A held value is process-wide and permanent, so a pass that holds
// one never runs in the process that runs the rest of the package.
func jhRunChild(t *testing.T, test, mode string) {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^"+test+"$", "-test.v", "-test.count=1") // #nosec G204 -- this test binary
	child.Env = append(os.Environ(), jhModeEnv+"="+mode)
	out, err := child.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--- PASS: "+test+"/") {
		t.Errorf("%s pass: %v\n%s", mode, err, out)
	}
}

// jhFilesCarrying lists every file under root whose bytes contain needle, in
// any case, except files whose base name starts with one of skip (inputs a
// test planted with the needle on purpose).
func jhFilesCarrying(t *testing.T, root, needle string, skip ...string) []string {
	t.Helper()
	needle = strings.ToLower(needle)
	var hits []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		for _, prefix := range skip {
			if strings.HasPrefix(d.Name(), prefix) {
				return nil
			}
		}
		b, err := os.ReadFile(p) // #nosec G304 -- temp test path
		if err != nil {
			return err
		}
		if strings.Contains(strings.ToLower(string(b)), needle) {
			hits = append(hits, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return hits
}

// TestJournalCredentialHardening drives each path an operator-typed credential
// takes into the learn journal (the LEARN_SESSION override, a flag name, a flag
// name after "--", a typo Cobra corrects) through the real command tree, first
// with nothing held and then with the credential held, and fails if the
// credential reaches any file under the test home. Each run must journal one
// entry, so a run that writes nothing cannot pass.
func TestJournalCredentialHardening(t *testing.T) {
	mode := os.Getenv(jhModeEnv)
	if mode == "" {
		jhRunChild(t, "TestJournalCredentialHardening", "unheld")
		jhRunChild(t, "TestJournalCredentialHardening", "held")
		return
	}
	if mode == "held" {
		credkit.Hold("test:journal", jhSecret)
	}
	cases := []struct {
		name     string
		override bool
		args     []string
		needle   string // what must reach no file; jhSecret unless set
		noShape  bool   // nothing after "--" is a flag, so argv_shape stays empty
	}{
		{name: "credential typed as a flag name", args: []string{"version", "--" + jhSecret}},
		{name: "credential typed as a flag name with a value", args: []string{"version", "--" + jhSecret + "=x"}},
		{name: "credential typed as a flag name before a value", args: []string{"version", "--" + jhSecret, "x"}},
		{name: "credential typed as a short flag", args: []string{"version", "-" + jhSecret}},
		{name: "credential after the end of flags", args: []string{"version", "--", "--" + jhSecret}, noShape: true},
		// Within 3 edits of the declared --compact, so Cobra suggests it: the
		// typed name is still never kept.
		{name: "credential one typo from a declared flag", args: []string{"version", "--compactQ7x"}, needle: "compactQ7x"},
		{name: "declared flag name after the end of flags", args: []string{"version", "--", "--json"}, noShape: true},
		{name: "learn session override", override: true, args: []string{"version"}},
	}
	for _, tc := range cases {
		t.Run(tc.name+" ("+mode+")", func(t *testing.T) {
			home := withJournalHome(t)
			if tc.override {
				t.Setenv("PAX8_LEARN_SESSION", jhSecret)
			}
			_ = runCLI(t, tc.args...)
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
			needle := tc.needle
			if needle == "" {
				needle = jhSecret
			}
			if hits := jhFilesCarrying(t, home, needle); len(hits) > 0 {
				t.Errorf("the credential reached %v", hits)
			}
		})
	}
}

// jhSink is one writer under internal/learn that stores operator-supplied
// text. write stores v through it and reports whether the sink stored a
// record at all (not whether v survived: a record rewritten to a notice is
// still a record the refusal should have prevented).
type jhSink struct {
	name  string
	write func(t *testing.T, home, v string) bool
	// planted names the files (base-name prefixes) the sink's input was
	// planted in with the text as an older build wrote it; only those may
	// carry the held credential afterwards.
	planted []string
}

func jhOpenStore(t *testing.T, home string) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(home, "jh-learn.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func jhCount(t *testing.T, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0
		}
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func jhTodaySegment() string {
	return "journal-" + time.Now().UTC().Format("20060102") + ".jsonl"
}

func jhSinks() []jhSink {
	ctx := context.Background()
	return []jhSink{
		{name: "journal entry (staged learn context)", write: func(t *testing.T, home, v string) bool {
			learn.SetJournalLearnContext("jh family "+v, []string{v})
			learn.JournalInvocation(learn.JournalEntry{Cmd: []string{"recall"}})
			return len(readAllJournalEntries(t)) > 0
		}},
		{name: "journal entry (direct append)", write: func(t *testing.T, home, v string) bool {
			_ = learn.AppendJournalEntry(learn.JournalEntry{Cmd: []string{"recall"}, QueryFamily: "jh family " + v})
			return len(readAllJournalEntries(t)) > 0
		}},
		{name: "teach log warning", write: func(t *testing.T, home, v string) bool {
			_ = learn.AppendTeachLogWarning("teach", "jh query", learn.Warning{Code: "jh-code", Resource: v, Detail: "jh detail " + v})
			got, err := learn.ReadTeachLogWarnings()
			if err != nil {
				t.Fatalf("read teach log: %v", err)
			}
			return len(got) > 0
		}},
		{name: "entity alias (lookups.Upsert)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			_ = lookups.Upsert(s.DB(), "jhkind", "jh canonical", v, "")
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM entity_lookups WHERE kind = 'jhkind'`) > 0
		}},
		{name: "entity seed (lookups.SeedBatch)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			tx, err := s.DB().Begin()
			if err != nil {
				t.Fatalf("begin: %v", err)
			}
			_, _ = lookups.SeedBatch(tx, []lookups.LookupRow{{Kind: "jhkind", Canonical: "jh seed", Value: v}})
			if err := tx.Commit(); err != nil {
				t.Fatalf("commit: %v", err)
			}
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM entity_lookups WHERE kind = 'jhkind'`) > 0
		}},
		{name: "recall miss (lookups.RecordMisses)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			_ = lookups.RecordMisses(ctx, s.DB(), []string{v})
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM learn_recall_misses`) > 0
		}},
		{name: "synced alias (lookups.RefreshFromSynced)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			// An older build recorded the miss and a synced row carries it:
			// planted below the store's own writer, as that build left them.
			raw, err := sql.Open("sqlite", filepath.Join(home, "jh-learn.db"))
			if err != nil {
				t.Fatalf("open raw: %v", err)
			}
			defer raw.Close()
			for _, q := range []string{
				`CREATE TABLE IF NOT EXISTS learn_recall_misses (entity TEXT PRIMARY KEY, miss_count INTEGER NOT NULL DEFAULT 1, first_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
				`INSERT INTO learn_recall_misses (entity) VALUES ('` + v + `')`,
				`INSERT INTO resources (id, resource_type, data) VALUES ('jh-1', 'jhkind', '{"name":"` + v + `"}')`,
			} {
				if _, err := raw.Exec(q); err != nil {
					t.Fatalf("plant %q: %v", q, err)
				}
			}
			_, _ = lookups.RefreshFromSynced(ctx, s.DB(), lookups.RefreshOpts{Extract: func(string, []byte) []string { return []string{v} }})
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM entity_lookups WHERE source = 'synced'`) > 0
		}, planted: []string{"jh-learn.db"}},
		{name: "pattern (patterns.Upsert)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			_, _, _ = patterns.Upsert(s.DB(), patterns.Pattern{
				QueryTemplate:    v + " {category}",
				ResourceTemplate: "GROUP-{category}",
				ResourceType:     "items",
				Strategy:         patterns.StrategySubstitute,
				EntityKind:       "jhkind",
				ExampleQuery:     v + " widgets",
			})
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM search_patterns WHERE entity_kind = 'jhkind'`) > 0
		}},
		{name: "preseed learning (learn.RunWith)", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			learn.ResetScannersForTest()
			t.Cleanup(learn.ResetScannersForTest)
			learn.Register("jh-scanner", func(context.Context, *sql.DB) ([]learn.PreseedRow, error) {
				return []learn.PreseedRow{{QueryPattern: "jh " + v, ResourceID: "jh-res", ResourceType: "jhitems"}}, nil
			})
			_, _ = learn.RunWith(ctx, s.DB(), learn.RunOpts{RowCap: 0})
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM search_learnings WHERE resource_type = 'jhitems'`) > 0
		}},
		{name: "playbook family rekey", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			pb, err := learn.MarshalPlaybook(learn.Playbook{Steps: []learn.PlaybookStep{{Cmd: "items list"}}, ExpectedToolCalls: 1})
			if err != nil {
				t.Fatalf("marshal playbook: %v", err)
			}
			if _, _, err := s.UpsertPlaybook(store.UpsertPlaybookInput{QueryFamily: "jh old family", PlaybookJSON: pb}); err != nil {
				t.Fatalf("seed playbook: %v", err)
			}
			_, _, _ = learn.LookupPlaybookByFamily(ctx, s.DB(), entities.NewConfig(), "jh family "+v, "jh old family")
			// Stored means the row moved off its old key to the new text.
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM learning_playbooks WHERE query_family = 'jh old family'`) == 0
		}},
		{name: "playbook candidate synthesis", write: func(t *testing.T, home, v string) bool {
			s := jhOpenStore(t, home)
			family := "jh family " + v
			// An episode an older build journaled with the text as typed.
			writeRawSegment(t, jhTodaySegment(), synthesisLines(t,
				learn.JournalEntry{SessionKey: "jh-sess", Cmd: []string{"recall"}, QueryFamily: family},
				learn.JournalEntry{SessionKey: "jh-sess", Cmd: []string{"items", "list"}, ArgvShape: map[string]string{"limit": "int"}},
			)...)
			_, _, _ = learn.SynthesizePlaybookCandidate(s, family, "jh-sess")
			return jhCount(t, s.DB(), `SELECT COUNT(*) FROM learn_candidates`) > 0
		}, planted: []string{"journal-"}},
		{name: "flag correction candidate (derive)", write: func(t *testing.T, home, v string) bool {
			dbPath := filepath.Join(home, "jh-learn.db")
			now := time.Now().UTC()
			// A failed flag an older build journaled as typed, then its retry.
			writeRawSegment(t, jhTodaySegment(), synthesisLines(t,
				learn.JournalEntry{TS: now.Add(-2 * time.Minute).Format(time.RFC3339), SessionKey: "jh-sess", Cmd: []string{"items", "list"}, ExitCode: 2, ErrorClass: "usage", FailedFlag: "--" + v, SuggestedFlag: "--dates"},
				learn.JournalEntry{TS: now.Add(-time.Minute).Format(time.RFC3339), SessionKey: "jh-sess", Cmd: []string{"items", "list"}, ArgvShape: map[string]string{"dates": "str"}},
			)...)
			_ = learn.DeriveFlagCorrections(deriveOpener(dbPath), nil)
			if _, err := os.Stat(dbPath); err != nil {
				return false
			}
			return len(listCandidateRows(t, dbPath)) > 0
		}, planted: []string{"journal-"}},
	}
}

// TestLearnSinksRefuseHeldCredential drives every writer under internal/learn
// that stores operator-supplied text: a probe that is not held must be stored
// (the sink was reached), then a held credential in the same place must be
// withheld by the kit, not stored in any form, and reach no file under the test home.
func TestLearnSinksRefuseHeldCredential(t *testing.T) {
	if os.Getenv(jhModeEnv) != "held" {
		jhRunChild(t, "TestLearnSinksRefuseHeldCredential", "held")
		return
	}
	credkit.Hold("test:journal", jhName)
	for _, sink := range jhSinks() {
		t.Run(sink.name, func(t *testing.T) {
			home := withJournalHome(t)
			if !sink.write(t, home, jhProbe) {
				t.Fatalf("the probe was not stored, so this run proves nothing about the sink")
			}
			if len(jhFilesCarrying(t, home, jhProbe)) == 0 {
				t.Fatalf("the probe reached no file, so this run proves nothing about the sink")
			}
			home = withJournalHome(t)
			if sink.write(t, home, jhName) {
				t.Errorf("a record carrying the held credential was stored; want it refused")
			}
			if hits := jhFilesCarrying(t, home, jhName, sink.planted...); len(hits) > 0 {
				t.Errorf("the held credential reached %v", hits)
			}
		})
	}
}
