// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func openFDCount(t *testing.T) int {
	t.Helper()
	d, err := os.Open("/dev/fd")
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		t.Skipf("no /dev/fd: %v", err)
	}
	return len(names)
}

// Hand-fix (handfixes.json temp-handle-closed-before-kit-write): every
// CreateTemp handle is closed before the kit rewrites the file and before it
// is renamed, linked or removed. Windows refuses those on an open file.
func TestTempFileWritersCloseTheirHandle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("covered on Windows by the rename in the download tests")
	}
	t.Run("download", func(t *testing.T) {
		dir := t.TempDir()
		before := openFDCount(t)
		if err := writeDownloadUnder(dir, "clip-1.mp3", []byte("audio-bytes")); err != nil {
			t.Fatalf("writeDownloadUnder: %v", err)
		}
		if after := openFDCount(t); after != before {
			t.Fatalf("open descriptors %d -> %d", before, after)
		}
	})
	t.Run("db owner marker", func(t *testing.T) {
		dir := t.TempDir()
		unscoped := filepath.Join(dir, "data.db")
		if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		before := openFDCount(t)
		claimLegacyUnscopedDB(dir, unscoped, "scopehash-0123456789", "test")
		if after := openFDCount(t); after != before {
			t.Fatalf("open descriptors %d -> %d: the marker temp file was left open", before, after)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".data-db-owner-") {
				t.Fatalf("temp marker left behind: %s", e.Name())
			}
		}
	})
}
