// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): regression guard for handfixes.json entry
// "credkit-wiring" (class secret-echo).

package store

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"avanan-pp-cli/internal/credkit"
)

// TestStoreRefusesAHeldValue: a write carrying a held credential, in any
// spelling the check decodes, is refused and never reaches the database files.
func TestStoreRefusesAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const v = "store-held-value-ST1"
	credkit.Hold("config:unclassified", v)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("msp-tenants", "ok-1", json.RawMessage(`{"name":"ordinary"}`)); err != nil {
		t.Fatalf("an ordinary write was refused: %v", err)
	}
	for _, body := range []string{`{"key":"` + v + `"}`, `{"blob":"` + strings.ToUpper(hex.EncodeToString([]byte(v))[:6]) + hex.EncodeToString([]byte(v))[6:] + `"}`} {
		if err := s.Upsert("msp-tenants", "held-1", json.RawMessage(body)); err == nil {
			t.Errorf("a write carrying the held value was accepted: %s", body)
		}
	}
	_ = s.Close()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(b), v) || strings.Contains(strings.ToLower(string(b)), hex.EncodeToString([]byte(v))) {
			t.Errorf("%s carries the held value", e.Name())
		}
	}
}
