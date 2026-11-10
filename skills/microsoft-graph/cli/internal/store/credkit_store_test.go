// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credkit-sqlite-every-handle):
// the local store opens through the credential kit's SQL driver, so a record
// that carries a held vendor response value is refused whole and an
// ordinary one is kept.

package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"microsoft-graph-pp-cli/internal/credkit"
)

func TestStoreRefusesAHeldValue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "kit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	credkit.Hold("response:access_token", "StoreHeldTok-4e7a91") // synccpu: the store refuses only a held vendor response value
	if err := s.Upsert("users", "u1", json.RawMessage(`{"id":"u1","displayName":"Ordinary Person"}`)); err != nil {
		t.Fatalf("an ordinary record was refused: %v", err)
	}
	err = s.Upsert("users", "u2", json.RawMessage(`{"id":"u2","displayName":"StoreHeldTok-4e7a91"}`))
	if !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("a record carrying a held value was not refused by the kit: %v", err)
	}
}
