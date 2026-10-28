// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

// Review round 1 (Codex, 2026-10-28), one P1 class: every file the program
// writes outside the credential store passes the credential kit.
// writePrivateFile is the one writer behind run receipts (which record argv),
// the audit index, client profiles, the default-profile config and the rate
// limit ledger.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kaseya-vsa-pp-cli/internal/credkit"
)

func TestR1CPrivateFilesWithholdHeldValues(t *testing.T) {
	secret := "Secret" + "Alpha901" // split: a synthetic fixture, not a credential for secret scanners
	credkit.Hold("config:vsa_token_secret", secret)
	path := filepath.Join(t.TempDir(), "receipts", "run.json")
	if err := writePrivateFile(path, []byte(`{"argv":["--`+secret+`"],"status":"ok"}`+"\n")); !errors.Is(err, credkit.ErrWithheld) {
		t.Fatalf("want the write withheld, got %v", err)
	}
	if got, err := os.ReadFile(path); err == nil && bytes.Contains(got, []byte(secret)) {
		t.Fatalf("receipt holds the credential: %s", got)
	}
	if err := writePrivateFile(path, []byte(`{"status":"ok"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(got, []byte(`"status":"ok"`)) {
		t.Fatalf("an ordinary receipt was not written (%v): %s", err, got)
	}
}
