// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. The fleet mirror records a NAS address only in MirrorAddress
// form; an address an earlier build recorded raw is scrubbed on open.

package fleet

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"synology-pp-cli/internal/store"
)

// TestMirrorScrubsAnAddressRecordedWithAPassword: a build before this fix
// stored a hand-edited registry URL raw in fleet_sync.url, password and all.
// Opening the mirror for writing rewrites it as MirrorAddress, and the
// password is gone from the database file and its write-ahead log; a clean
// address recorded the same way is left exactly as it was, so its mirror is
// still this box's.
func TestMirrorScrubsAnAddressRecordedWithAPassword(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "fleet.db")
	secret := fmt.Sprintf("UI-PASS-%x", time.Now().UnixNano()) // a per-run sentinel, never a literal
	raw := "http://admin:" + secret + "@127.0.0.1:5000/?token=" + secret + "#" + secret
	clean := "https://nas.acme.lan:5001"

	s, err := store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, s.DB()); err != nil {
		t.Fatal(err)
	}
	for nas, u := range map[string]string{"acme": raw, "beta": clean} {
		if _, err := s.DB().Exec(`INSERT INTO fleet_sync (nas, url, attempted_at, succeeded_at, mirror_format) VALUES (?, ?, ?, ?, ?)`,
			nas, u, time.Now().Unix(), time.Now().Unix(), MirrorFormat); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.Close()
	if !fileHolds(t, path, secret) {
		t.Fatal("fixture: the raw address is not in the database file")
	}

	// A read-only open (no migration) never reads the password back.
	s, err = store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	unmigrated, err := LoadSyncStates(ctx, s.DB())
	stale, serr := NeedsMigration(ctx, s.DB())
	_ = s.Close()
	if err != nil || unmigrated["acme"].URL != "http://127.0.0.1:5000/" {
		t.Fatalf("an unscrubbed row reads back as %q (%v)", unmigrated["acme"].URL, err)
	}
	if !stale || serr != nil {
		t.Fatalf("NeedsMigration = %v, %v for a mirror holding a raw address: a fleet report would not scrub it", stale, serr)
	}

	s, err = store.OpenWithContext(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureSchema(ctx, s.DB()); err != nil {
		t.Fatal(err)
	}
	states, err := LoadSyncStates(ctx, s.DB())
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := s.DB().QueryRow(`SELECT url FROM fleet_sync WHERE nas = 'acme'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	// Checked while the mirror is still open (a long-running synology-mcp
	// keeps it open): the scrub itself must leave nothing behind, not the
	// checkpoint a close would run.
	for _, p := range []string{path, path + "-wal"} {
		if fileHolds(t, p, secret) {
			t.Fatalf("%s still holds the password after the scrub (mirror still open)", filepath.Base(p))
		}
	}
	stale, serr = NeedsMigration(ctx, s.DB())
	_ = s.Close()
	if stale || serr != nil {
		t.Fatalf("NeedsMigration = %v, %v after the scrub", stale, serr)
	}
	if stored != "http://127.0.0.1:5000/" || states["acme"].URL != stored {
		t.Fatalf("scrubbed address = %q (read back %q), want http://127.0.0.1:5000/", stored, states["acme"].URL)
	}
	if states["beta"].URL != clean || !states["beta"].FreshFor(NAS{Name: "beta", URL: clean}, time.Now(), time.Hour) {
		t.Fatalf("a clean recorded address changed or no longer matches its registry URL: %+v", states["beta"])
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if fileHolds(t, p, secret) {
			t.Fatalf("%s still holds the password after the scrub", filepath.Base(p))
		}
	}
}

func fileHolds(t *testing.T, path, secret string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Contains(b, []byte(secret))
}

// TestMirrorAddressHasNoCredentials: every address the mirror records or
// compares goes through MirrorAddress, which keeps no user info, query or
// fragment, and two spellings that differ only in those are one address.
func TestMirrorAddressHasNoCredentials(t *testing.T) {
	for raw, want := range map[string]string{
		"http://admin:pw@nas:5000":          "http://nas:5000",
		" https://nas:5001/dsm?sid=x#frag ": "https://nas:5001/dsm",
		"https://nas:5001":                  "https://nas:5001",
	} {
		if got := MirrorAddress(raw); got != want {
			t.Errorf("MirrorAddress(%q) = %q, want %q", raw, got, want)
		}
	}
	if !SameMirrorAddress("http://admin:pw@nas:5000", "http://nas:5000") || SameMirrorAddress("http://nas:5000", "http://nas2:5000") {
		t.Fatal("SameMirrorAddress")
	}
}
