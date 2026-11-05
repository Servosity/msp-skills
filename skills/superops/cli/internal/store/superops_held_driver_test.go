// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the store through the credential kit (credkit-wiring).

package store

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/credkit"
)

// Every statement of a store passes the kit, whatever code
// writes it: a held value in an argument (a checkpoint cursor) or in the
// statement text (a savepoint built from a vendor id), in any encoding, is
// refused and never reaches the file; clean writes still work.
func TestStoreRefusesEveryStatementThatRepeatsAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const held = "driverHeldTok7120"
	credkit.Hold("minted:test", held)
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("tickets", "page-2", 5); err != nil {
		t.Fatalf("a clean checkpoint was refused: %v", err)
	}
	if err := db.SaveSyncState("tickets", hex.EncodeToString([]byte(held)), 5); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("a checkpoint repeating a held value: %v", err)
	}
	if _, err := db.DB().Exec("SAVEPOINT " + held); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("a statement text repeating a held value: %v", err)
	}
	_ = db.Close()
	files, _ := filepath.Glob(filepath.Join(dir, "data.db*"))
	for _, p := range files {
		if b, _ := os.ReadFile(p); strings.Contains(string(b), hex.EncodeToString([]byte(held))) {
			t.Fatalf("%s keeps the value", filepath.Base(p))
		}
	}
}
