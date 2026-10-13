// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadTranscript_LocalStoreErrorSurfaced is the FIX L guard: when
// openGranolaCache returns a REAL store error (corrupt/unreadable DB) and the
// user asked for --data-source local (no live fallback), loadTranscript must
// surface that error rather than dereferencing a nil/partial cache and
// returning a misleading "not in local store" (or panicking).
func TestLoadTranscript_LocalStoreErrorSurfaced(t *testing.T) {
	// A file that EXISTS (so it is not treated as "missing") but is not a valid
	// SQLite database → openGranolaStoreRead / EnsureSchema fails with a real error.
	corrupt := filepath.Join(t.TempDir(), "data.db")
	if err := os.WriteFile(corrupt, []byte("this is not a sqlite database, it is garbage bytes\n"), 0o600); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}
	prev := granolaStoreOverride
	granolaStoreOverride = corrupt
	t.Cleanup(func() { granolaStoreOverride = prev })

	_, _, err := loadTranscript("not_x", "local")
	if err == nil {
		t.Fatal("a corrupt local store must surface an error, not a nil-cache not-found")
	}
	// It must be the real store error, not the generic "not in local store" hint.
	if strings.Contains(err.Error(), "not in local store") {
		t.Fatalf("expected the underlying store error to be surfaced, got the misleading hint: %v", err)
	}
	if !strings.Contains(err.Error(), "read local transcript store") {
		t.Fatalf("expected a wrapped store-read error, got: %v", err)
	}
}

// TestLoadTranscript_AutoFallsThroughOnStoreError proves the auto mode does NOT
// fail on a local store problem — it deliberately falls through to the live REST
// path (which then fails for its OWN reason, not the store error, and never
// panics on a nil cache).
func TestLoadTranscript_AutoFallsThroughOnStoreError(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no real config/creds
	corrupt := filepath.Join(t.TempDir(), "data.db")
	if err := os.WriteFile(corrupt, []byte("garbage-not-sqlite\n"), 0o600); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}
	prev := granolaStoreOverride
	granolaStoreOverride = corrupt
	t.Cleanup(func() { granolaStoreOverride = prev })

	// auto mode ("") — must not panic; returns an error from the live path (no
	// creds / no network), which must NOT be the local store-read error.
	_, _, err := loadTranscript("not_x", "")
	if err != nil && strings.Contains(err.Error(), "read local transcript store") {
		t.Fatalf("auto mode must fall through to live, not surface the local store error: %v", err)
	}
}
