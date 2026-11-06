// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (handfixes.json credkit-wiring).

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sophos-central-pp-cli/internal/credkit"
)

// The store keeps each vendor response as received (resources.data, the FTS
// index and the typed tables fleet refresh fills). A response that echoes a
// held credential must not reach the SQLite file: every statement runs
// through the credential kit's SQL driver (internal/credsql), which withholds
// it before SQLite sees it. The value is held the way Start holds
// SOPHOS_CENTRAL_CLIENT_SECRET; this package's other tests never write it.
func TestStoreNeverWritesAHeldCredential(t *testing.T) {
	const secret = "qZ7vN" + "2xLp9Wd4Sc"
	credkit.Hold("env:SOPHOS_CENTRAL_CLIENT_SECRET", secret)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	raw := json.RawMessage(`{"id":"a1b2c3d4-0000-4000-8000-000000000001","tenant":{"id":"48bebbb0-f7b6-49c5-aec0-315858f73457"},"description":"` + secret + `"}`)
	if err := s.Upsert("alerts", "a1b2c3d4-0000-4000-8000-000000000001", raw); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("Upsert of a response that echoes a held credential: got %v, want the kit's withhold", err)
	}
	if err := s.UpsertAlerts(raw); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("UpsertAlerts of a response that echoes a held credential: got %v, want the kit's withhold", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if data, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil && bytes.Contains(data, []byte(secret)) {
			t.Errorf("%s holds the credential the vendor echoed", e.Name())
		}
	}
}
