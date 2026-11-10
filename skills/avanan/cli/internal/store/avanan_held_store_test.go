// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/credkit"
)

// TestStoreRefusesAHeldValue: a write carrying a held vendor response value
// whole is refused and never reaches the database files (synccpu: the store
// reads no other source and decodes nothing).
func TestStoreRefusesAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const v = "store-held-value-ST1"
	credkit.Hold("response:password", v) // synccpu: the store refuses only a held vendor response value
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("msp-tenants", "ok-1", json.RawMessage(`{"name":"ordinary"}`)); err != nil {
		t.Fatalf("an ordinary write was refused: %v", err)
	}
	if body := `{"key":"` + v + `"}`; s.Upsert("msp-tenants", "held-1", json.RawMessage(body)) == nil {
		t.Errorf("a write carrying the held value was accepted: %s", body)
	}
	_ = s.Close()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(b), v) {
			t.Errorf("%s carries the held value", e.Name())
		}
	}
}
