// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the store through the credential kit (credkit-wiring).

package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/credkit"
)

// Every statement of a store passes the kit, whatever code writes it: a held
// vendor response value in an argument (a checkpoint cursor) is refused whole
// and never reaches the file; clean writes still work. (synccpu, owner GO
// 2026-11-09: the store reads no statement text and decodes nothing.)
func TestStoreRefusesEveryStatementThatRepeatsAHeldValue(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const held = "driverHeldTok7120"
	credkit.Hold("response:token", held) // synccpu: the store refuses only a held vendor response value
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSyncState("tickets", "page-2", 5); err != nil {
		t.Fatalf("a clean checkpoint was refused: %v", err)
	}
	if err := db.SaveSyncState("tickets", "next:"+held, 5); !errors.Is(err, credkit.ErrWithheld) {
		t.Errorf("a checkpoint repeating a held value: %v", err)
	}
	_ = db.Close()
	files, _ := filepath.Glob(filepath.Join(dir, "data.db*"))
	for _, p := range files {
		if b, _ := os.ReadFile(p); strings.Contains(string(b), held) {
			t.Fatalf("%s keeps the value", filepath.Base(p))
		}
	}
}
