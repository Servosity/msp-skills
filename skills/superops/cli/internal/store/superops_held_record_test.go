// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): credkit-wiring store rows.

package store

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/credkit"
)

// A vendor batch in which any record repeats a held credential, as written or
// encoded, at any length, is refused whole: no row, no search index entry,
// nothing of it in the database file.
func TestUpsertBatchRefusesARecordThatRepeatsAHeldCredential(t *testing.T) {
	if !heldChild(t) {
		return
	}
	const long, short = "storeHeldTok5530", "j~W9xR"
	credkit.Hold("minted:test", long, short)
	for _, echo := range []string{long, base64.StdEncoding.EncodeToString([]byte(short))} {
		dir := t.TempDir()
		db, err := Open(filepath.Join(dir, "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		clean, _ := json.Marshal(map[string]any{"ticketId": "41", "subject": "printer"})
		leak, _ := json.Marshal(map[string]any{"ticketId": "42", "subject": "token " + echo})
		_, _, err = db.UpsertBatch("tickets", []json.RawMessage{clean, leak})
		if !errors.Is(err, credkit.ErrWithheld) || strings.Contains(err.Error(), echo) {
			t.Fatalf("UpsertBatch = %v, want credkit.ErrWithheld naming no value", err)
		}
		var n int
		if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources`).Scan(&n); err != nil || n != 0 {
			t.Fatalf("stored %d rows (%v), want none", n, err)
		}
		_ = db.Close()
		files, _ := filepath.Glob(filepath.Join(dir, "data.db*"))
		for _, p := range files {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), echo) {
				t.Fatalf("%s keeps %q", filepath.Base(p), echo)
			}
		}
	}
}
