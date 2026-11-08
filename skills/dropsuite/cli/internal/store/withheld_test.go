// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Hand-fix (handfixes.json sync-skips-withheld-row).

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dropsuite-pp-cli/internal/credkit"
)

// One row carrying a held credential is withheld and named; the rows around
// it are stored, and the credential is nowhere in the store file (#612).
// It runs in a process of its own: a value stays held until exit, and this
// package's other tests must not run with it held.
func TestUpsertBatchSkipsWithheldRow(t *testing.T) {
	if os.Getenv("WITHHELD_ROW_TEST_PROCESS") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestUpsertBatchSkipsWithheldRow$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "WITHHELD_ROW_TEST_PROCESS=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !bytes.Contains(out, []byte("--- PASS: TestUpsertBatchSkipsWithheldRow")) {
			t.Fatalf("in a process of its own: %v\n%s", err, out)
		}
		return
	}
	secret := "kitmem-" + strings.Repeat("Qx7v", 10)
	credkit.Hold("minted:kitmem-test", secret)
	path := filepath.Join(t.TempDir(), "store.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	items := []json.RawMessage{
		json.RawMessage(`{"id":"1","name":"first"}`),
		json.RawMessage(`{"id":"2","name":"note ` + secret + `"}`),
		json.RawMessage(`{"id":"3","name":"third"}`),
	}
	stored, _, _, err := s.UpsertBatchDetailed("kitmem_rows", items)
	var w *WithheldRowsError
	if !errors.As(err, &w) || len(w.IDs) != 1 || !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("err = %v, want the one withheld row named", err)
	}
	if stored != 2 {
		t.Fatalf("stored %d rows, want the 2 around the withheld one", stored)
	}
	if n, err := s.Count("kitmem_rows"); err != nil || n != 2 {
		t.Fatalf("Count = %d, %v; want 2", n, err)
	}
	// A clean batch names nothing.
	if _, _, _, err := s.UpsertBatchDetailed("kitmem_rows", items[:1]); err != nil {
		t.Fatalf("a clean batch: %v", err)
	}
	s.Close()
	for _, f := range []string{path, path + "-wal"} {
		if b, err := os.ReadFile(f); err == nil && bytes.Contains(b, []byte(secret)) {
			t.Fatalf("%s carries the held credential", filepath.Base(f))
		}
	}
}
