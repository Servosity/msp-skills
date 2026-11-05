// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guards for handfixes.json entry
// "no-local-data-covers-hand-written-writers" (class local-store).

package cli

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/store"
)

// noLocalDataRefusal is the refusal applyLocalDataEnv gives a command whose
// job is to copy API data into the local store (sync, workflow archive).
func noLocalDataRefusal(path string) string {
	return path + " is disabled while " + noLocalDataEnvVar + "=1: it would save API data to the local store"
}

// storedRows counts rows of resourceType across every SQLite store under root.
func storedRows(t *testing.T, root, resourceType string) (dbs, rows int) {
	t.Helper()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".db") {
			return nil
		}
		dbs++
		db, oerr := store.Open(path)
		if oerr != nil {
			t.Fatalf("open %s: %v", path, oerr)
		}
		defer db.Close()
		n, cerr := db.Count(resourceType)
		if cerr != nil {
			t.Fatalf("count %s in %s: %v", resourceType, path, cerr)
		}
		rows += n
		return nil
	})
	return dbs, rows
}

// TestMirrorRefusesUnderNoLocalData: AVANAN_NO_LOCAL_DATA=1 promises no local
// copy of API responses. sync and workflow archive refuse under it, but the
// hand-written mirror still opened the store and wrote every event, entity
// and exception it fetched. It must refuse exactly as sync does, before any
// request or store write; --dry-run still previews, as it does for sync.
func TestMirrorRefusesUnderNoLocalData(t *testing.T) {
	home := isolateAvananEnv(t)
	s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){}}
	srv := httptest.NewServer(s)
	defer srv.Close()
	t.Setenv("AVANAN_BASE_URL", srv.URL)
	t.Setenv(noLocalDataEnvVar, "1")

	_, _, syncErr := executeCLI(t, "sync")
	if syncErr == nil || syncErr.Error() != noLocalDataRefusal("sync") {
		t.Fatalf("sync under %s=1: got %v, want the documented refusal", noLocalDataEnvVar, syncErr)
	}

	dbPath := filepath.Join(home, "mirror.db")
	stdout, stderr, err := executeCLI(t, "mirror", "--since", "1d", "--db", dbPath, "--json")
	if err == nil || err.Error() != noLocalDataRefusal("mirror") {
		t.Errorf("mirror under %s=1: got %v, want %q\nstdout:\n%s\nstderr:\n%s", noLocalDataEnvVar, err, noLocalDataRefusal("mirror"), stdout, stderr)
	}
	s.mu.Lock()
	calls := append([]string(nil), s.calls...)
	s.mu.Unlock()
	if len(calls) != 0 {
		t.Errorf("mirror under %s=1 sent %d request(s): %v", noLocalDataEnvVar, len(calls), calls)
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Errorf("mirror under %s=1 created the local store %s (stat: %v)", noLocalDataEnvVar, dbPath, statErr)
	}

	if _, _, err := executeCLI(t, "mirror", "--since", "1d", "--db", dbPath, "--dry-run"); err != nil {
		t.Errorf("mirror --dry-run under %s=1 refused: %v (sync's refusal exempts a dry run)", noLocalDataEnvVar, err)
	}
}

// TestRemediateKeepsNoActionLogUnderNoLocalData: remediate's recordActionLog
// wrote the task-to-entity link (task ids from the API response) into the
// local store without checking AVANAN_NO_LOCAL_DATA. Under the switch the
// confirmed action is still submitted and reported, but nothing is stored.
// The control run without the switch proves the test can see the log.
func TestRemediateKeepsNoActionLogUnderNoLocalData(t *testing.T) {
	for _, tc := range []struct {
		name     string
		switchOn bool
	}{
		{"control: switch off records the log", false},
		{"switch on keeps no log", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := isolateAvananEnv(t)
			s := &reviewServer{handlers: map[string]func(http.ResponseWriter, *http.Request){
				"POST /v1.0/action/entity": jsonReply(`{"responseEnvelope":{"responseCode":0},"responseData":[{"entityId":"aaa","taskId":"T-1"}]}`),
			}}
			srv := httptest.NewServer(s)
			defer srv.Close()
			t.Setenv("AVANAN_BASE_URL", srv.URL)
			if tc.switchOn {
				t.Setenv(noLocalDataEnvVar, "1")
			}

			stdout, stderr, err := executeCLI(t, "remediate", "quarantine", "--entity", "aaa",
				"--scope", "farm1:tenant-a", "--entity-type", "office365_emails_email", "--yes", "--json")
			if err != nil {
				t.Fatalf("remediate: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
			}
			if n := s.count("POST /v1.0/action/entity"); n != 1 {
				t.Fatalf("remediate sent %d action request(s), want 1", n)
			}
			if !strings.Contains(stdout, `"T-1"`) {
				t.Errorf("remediate did not report the submitted task:\n%s", stdout)
			}
			_, rows := storedRows(t, home, avananActionResource)
			if tc.switchOn && rows != 0 {
				t.Errorf("remediate under %s=1 stored %d action log row(s)", noLocalDataEnvVar, rows)
			}
			if !tc.switchOn && rows == 0 {
				t.Errorf("control run stored no action log row, so this test cannot see the log")
			}
		})
	}
}
