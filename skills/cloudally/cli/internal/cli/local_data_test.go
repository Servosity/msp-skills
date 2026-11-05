// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cloudally-pp-cli/internal/cliutil"
	"cloudally-pp-cli/internal/cliutil/testenv"
	"cloudally-pp-cli/internal/learn"
)

// Every purge kind resolves to where this connector really writes it, and
// credentials stay.
func TestLocalDataPurgePreviewsThenDeletes(t *testing.T) {
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	dataDir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	cacheDir, err := cliutil.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	journalDir, err := learn.JournalDir()
	if err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dataDir, "credentials.toml")
	targets := []string{
		filepath.Join(dataDir, "data.db"),
		filepath.Join(dataDir, "data-abc123.db-wal"),
		filepath.Join(cacheDir, "http", "entry"),
		filepath.Join(journalDir, "journal-20261105.jsonl"),
		filepath.Join(journalDir, "journal.offset"),
	}
	for _, p := range append(targets, keep) {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	stdout, stderr, err := runRootArgs(t, "local-data", "purge", "--json")
	if err != nil {
		t.Fatalf("preview: %v (stderr=%q)", err, stderr)
	}
	for _, p := range targets {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("preview deleted %s", p)
		}
	}
	for _, listed := range []string{targets[0], targets[1], filepath.Dir(targets[2]), targets[3], targets[4]} {
		// the preview is JSON: a Windows path appears with its backslashes escaped
		quoted, _ := json.Marshal(listed)
		if !strings.Contains(stdout, string(quoted)) {
			t.Errorf("preview does not list %s: %s", listed, stdout)
		}
	}

	if _, stderr, err := runRootArgs(t, "local-data", "purge", "--yes", "--json"); err != nil {
		t.Fatalf("apply: %v (stderr=%q)", err, stderr)
	}
	for _, p := range targets {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("--yes kept %s", p)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("purge deleted the credentials file: %v", err)
	}
}

// The switch refuses the commands that copy API data locally before any
// request, and a read under it saves nothing.
func TestNoLocalDataSwitch(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u-1","name":"Example MSP"}`))
	}))
	defer srv.Close()
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("CLOUDALLY_BASE_URL", srv.URL)
	t.Setenv(noLocalDataEnvVar, "1")

	for _, args := range [][]string{{"sync", "--json"}, {"workflow", "archive", "--json"}} {
		_, _, err := runRootArgs(t, args...)
		if err == nil || !strings.Contains(err.Error(), noLocalDataEnvVar) {
			t.Errorf("%v ran under the switch: err=%v", args, err)
		}
	}
	if requests != 0 {
		t.Fatalf("a refused writer reached the API (%d requests)", requests)
	}

	stdout, stderr, err := runRootArgs(t, "user", "me", "--json")
	if err != nil || !strings.Contains(stdout, "Example MSP") || requests != 1 {
		t.Fatalf("read under the switch: err=%v requests=%d stdout=%q stderr=%q", err, requests, stdout, stderr)
	}
	for name, dirOf := range map[string]func() (string, error){"data": cliutil.DataDir, "cache": cliutil.CacheDir} {
		dir, err := dirOf()
		if err != nil {
			t.Fatal(err)
		}
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.Type().IsRegular() {
				t.Errorf("read under the switch wrote %s file %s", name, p)
			}
			return nil
		})
	}

	// An agent-supplied flag cannot undo the operator's choice.
	if _, _, err := runRootArgs(t, "user", "me", "--data-source", "auto", "--json"); err == nil || !strings.Contains(err.Error(), noLocalDataEnvVar) {
		t.Errorf("--data-source auto ran under the switch: err=%v", err)
	}
}
