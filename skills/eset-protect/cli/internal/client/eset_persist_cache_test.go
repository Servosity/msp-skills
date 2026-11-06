// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/config"
)

const cacheLinkSentinel = "CACHE-INSTALLER-LINK-SENTINEL"

func cacheDirHolds(t *testing.T, dir, needle string) []string {
	t.Helper()
	var hits []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, rerr := os.ReadFile(path); rerr == nil && bytes.Contains(b, []byte(needle)) {
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// TestResponseCacheNeverHoldsCredentialTierBodies guards the HTTP-cache half
// of credential-tier-never-persisted: a GET whose body carries an installer
// download link is returned live with the link but never written to the
// response cache (so every call goes back to ESET), an entry an earlier build
// left behind is deleted instead of served, and ordinary bodies still cache.
func TestResponseCacheNeverHoldsCredentialTierBodies(t *testing.T) {
	var installerCalls, deviceCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/installers":
			atomic.AddInt32(&installerCalls, 1)
			_, _ = w.Write([]byte(`{"installers":[{"uuid":"b1","downloadUrl":"https://download.example/` + cacheLinkSentinel + `"}],"nextPageToken":""}`))
		default:
			atomic.AddInt32(&deviceCalls, 1)
			_, _ = w.Write([]byte(`{"devices":[{"uuid":"d1","displayName":"FRONTDESK-01"}],"nextPageToken":""}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = t.TempDir()

	for i := 0; i < 2; i++ {
		got, err := c.Get(context.Background(), "/v1/installers", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), cacheLinkSentinel) {
			t.Fatalf("live output must keep the installer link: %s", got)
		}
	}
	if n := atomic.LoadInt32(&installerCalls); n != 2 {
		t.Fatalf("the installer list was served from the response cache: %d live calls for 2 reads", n)
	}
	if hits := cacheDirHolds(t, c.cacheDir, cacheLinkSentinel); len(hits) > 0 {
		t.Fatalf("an installer download link reached the response cache: %v", hits)
	}

	// Ordinary bodies still cache: the second read is a hit.
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), "/v1/devices", nil); err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt32(&deviceCalls); n != 1 {
		t.Fatalf("an ordinary GET must still cache: %d live calls for 2 reads", n)
	}

	// A leftover entry (written before this rule) is deleted, not served.
	path := "/v1/installers/b1"
	file := filepath.Join(c.cacheResourceDir(path), c.cacheKeyFor(http.MethodGet, path, nil, nil, nil)+".json")
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(`{"installer":{"uuid":"b1","downloadUrl":"https://download.example/`+cacheLinkSentinel+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.readCache(path, nil); ok {
		t.Fatal("a cached body holding an installer link was served")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("the cached body holding an installer link was left on disk: %v", err)
	}
}
