// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// --- GAP 2: oversized stdin token is rejected, not silently truncated --------

func TestReadSetTokenValueRejectsOversizedStdin(t *testing.T) {
	cmd := &cobra.Command{}
	// One byte past the cap: a plain LimitReader(max) would hand back exactly
	// maxTokenBytes and silently drop the rest — a corrupt, unusable secret.
	cmd.SetIn(strings.NewReader(strings.Repeat("a", maxTokenBytes+1)))
	if _, err := readSetTokenValue(cmd); err == nil {
		t.Fatalf("expected rejection of oversized stdin token, got nil (secret would be silently truncated)")
	} else if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("error = %v, want an over-limit rejection", err)
	}
}

func TestReadSetTokenValueAcceptsNormalStdin(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("normal-piped-token-123\n"))
	got, err := readSetTokenValue(cmd)
	if err != nil {
		t.Fatalf("readSetTokenValue: %v", err)
	}
	if got != "normal-piped-token-123" {
		t.Fatalf("token = %q, want normal-piped-token-123", got)
	}
}

// --- GAP 3: export writes must not follow a pre-existing symlink -------------

func TestWriteSafeFileRefusesPreExistingSymlink(t *testing.T) {
	dir := t.TempDir()
	// The real file the symlink points at — the write must NOT reach it.
	outside := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(outside, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "note.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if err := writeSafeFile(link, []byte("SECRET-NOTES")); err == nil {
		t.Fatalf("writeSafeFile wrote through a pre-existing symlink; want a refusal")
	}
	// The symlink target must be untouched (no write-through).
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "original" {
		t.Fatalf("symlink target was modified through the link: %q", data)
	}
}

func TestWriteSafeFileOverwriteYields0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	// Pre-existing world-readable regular file must end up 0600 after overwrite.
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSafeFile(path, []byte("new")); err != nil {
		t.Fatalf("writeSafeFile: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file perm = %o, want 600", perm)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q, want new", data)
	}
}

// --- GAP 4: notes get must percent-encode a slash in the opaque note id ------

func TestNotesGetEscapesSlashInID(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	t.Cleanup(server.Close)

	// Isolate from any real config; env overrides win for base URL + token.
	t.Setenv("GRANOLA_CONFIG", filepath.Join(t.TempDir(), "cfg.toml"))
	t.Setenv("GRANOLA_BASE_URL", server.URL)
	t.Setenv("GRANOLA_API_KEY", "test-token")

	flags := &rootFlags{noCache: true, timeout: 5 * time.Second}
	cmd := newNotesGetCmd(flags)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetContext(context.Background())

	if err := cmd.RunE(cmd, []string{"a/b"}); err != nil {
		t.Fatalf("notes get RunE: %v", err)
	}
	if gotPath != "/v1/notes/a%2Fb" {
		t.Fatalf("request path = %q, want /v1/notes/a%%2Fb (slash must be percent-encoded into one segment)", gotPath)
	}
}

// --- GAP 6: the read path tightens the data dir to 0700 ----------------------

func TestOpenGranolaStoreReadTightensDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sub", "data.db")
	prev := granolaStoreOverride
	granolaStoreOverride = dbPath
	t.Cleanup(func() { granolaStoreOverride = prev })

	// openGranolaStore creates 'sub' at 0700 and the DB inside it.
	s, err := openGranolaStore(context.Background())
	if err != nil {
		t.Fatalf("openGranolaStore: %v", err)
	}
	s.Close()

	subDir := filepath.Dir(dbPath)
	// Loosen the dir to prove the read path re-tightens it.
	if err := os.Chmod(subDir, 0o777); err != nil {
		t.Fatal(err)
	}

	s2, err := openGranolaStoreRead(context.Background())
	if err != nil {
		t.Fatalf("openGranolaStoreRead: %v", err)
	}
	if s2 != nil {
		s2.Close()
	}

	fi, err := os.Stat(subDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir perm after read = %o, want 700 (read path must tighten)", perm)
	}
}
