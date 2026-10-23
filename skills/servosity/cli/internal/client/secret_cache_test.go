// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package client

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/config"
)

const cacheSentinel = "cachesentinel7f3a"

// cacheFilesContaining walks the response cache and returns every file whose
// body contains needle, plus the total number of cached files.
func cacheFilesContaining(t *testing.T, root, needle string) (hits []string, total int) {
	t.Helper()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		total++
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(body), needle) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits, total
}

// TestSecretResponsesAreNotCached pins that an encryption-key or credential
// read is never written to the on-disk response cache, while an ordinary read
// still is.
func TestSecretResponsesAreNotCached(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SERVOSITY_MSP_CACHE_DIR", cacheDir)
	t.Setenv("XDG_CACHE_HOME", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/restic-backups/21/encryption-key/":
			_, _ = w.Write([]byte(`{"key":"` + cacheSentinel + `","version":1,"locked":false}`))
		case "/backups/11/config.ini/":
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("password=" + cacheSentinel + "\n"))
		case "/restic-backups/21/restic-env/":
			_, _ = w.Write([]byte(`{"RESTIC_PASSWORD":"` + cacheSentinel + `"}`))
		case "/credentials/":
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"nas","password":"` + cacheSentinel + `"}]}`))
		default:
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"ordinary"}]}`))
		}
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}, 10*time.Second, 0)
	for _, path := range []string{"/restic-backups/21/encryption-key/", "/credentials/", "/restic-backups/21/restic-env/", "/issues/"} {
		if _, err := c.Get(context.Background(), path, nil); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
	}
	// A non-JSON body fails the read, but it must not reach the cache first.
	_, _ = c.Get(context.Background(), "/backups/11/config.ini/", nil)
	hits, total := cacheFilesContaining(t, cacheDir, cacheSentinel)
	if len(hits) != 0 {
		t.Fatalf("secret responses were cached: %v", hits)
	}
	if total == 0 {
		t.Fatal("the ordinary read was not cached either; the check above proves nothing")
	}
}

// TestSecretCachePurgeKeepsOrdinaryEntries pins the one-time upgrade cleanup:
// cached responses that carry a secret are deleted, every other entry stays,
// and a later run does not repeat the sweep.
func TestSecretCachePurgeKeepsOrdinaryEntries(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SERVOSITY_MSP_CACHE_DIR", cacheDir)
	t.Setenv("XDG_CACHE_HOME", "")
	root := filepath.Join(cacheDir, "http", "resources")
	seed := map[string]string{
		"restic-backups/key.json":  `{"key":"` + cacheSentinel + `","version":1}`,
		"credentials/list.json":    `{"results":[{"id":1,"password":"` + cacheSentinel + `"}]}`,
		"issues/list.json":         `{"results":[{"id":"safe"}]}`,
		"companies/empty-tok.json": `{"id":4421,"name":"Acme Co","token":""}`,
		"backups/config-ini.json":  "password=" + cacheSentinel,
	}
	for rel, body := range seed {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatalf("seed %s: %v", rel, err)
		}
	}

	New(&config.Config{}, time.Second, 0)

	for rel, wantKept := range map[string]bool{
		"restic-backups/key.json":  false,
		"credentials/list.json":    false,
		"issues/list.json":         true,
		"companies/empty-tok.json": true,
		"backups/config-ini.json":  false,
	} {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if kept := err == nil; kept != wantKept {
			t.Fatalf("%s kept=%v, want %v (%v)", rel, kept, wantKept, err)
		}
	}
	later := filepath.Join(root, "restic-backups", "later.json")
	if err := os.WriteFile(later, []byte(`{"key":"`+cacheSentinel+`"}`), 0o600); err != nil {
		t.Fatalf("seed later: %v", err)
	}
	New(&config.Config{}, time.Second, 0)
	if _, err := os.Stat(later); err != nil {
		t.Fatalf("the one-time sweep ran again: %v", err)
	}
}
