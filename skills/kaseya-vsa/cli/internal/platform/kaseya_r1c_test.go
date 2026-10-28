// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

// Review round 1 (Codex, 2026-10-28) [P1] class: every file the program
// writes outside the credential store passes the output chokepoint.
// writePrivateFile is the one writer behind run receipts (which record argv),
// the audit index, client profiles, the default-profile config and the rate
// limit ledger.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

func TestR1CPrivateFilesWithholdHeldValues(t *testing.T) {
	secret := "Secret" + "Alpha901" // split: a synthetic fixture, not a credential for secret scanners
	cliutil.FunnelHold(secret)
	path := filepath.Join(t.TempDir(), "receipts", "run.json")
	if err := writePrivateFile(path, []byte(`{"argv":["--`+secret+`"],"status":"ok"}`+"\n")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte(secret)) {
		t.Fatalf("receipt holds the credential: %s", got)
	}
	if !bytes.Contains(got, []byte(`"status":"ok"`)) {
		t.Fatalf("receipt lost its other fields: %s", got)
	}
}
