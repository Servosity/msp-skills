// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/sophos-central/handfixes.json
// under the "secret-responses-never-persisted" entry.

package client

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

	"sophos-central-pp-cli/internal/config"
)

const cacheSentinel = "SENTINEL-TAMPER-PW-cache"

// TestSecretResponsesAreNeverCached drives the real GET path against a stub
// and greps the cache directory: the tamper password must not land on disk,
// while an ordinary response still does (so the cache is not simply off).
func TestSecretResponsesAreNeverCached(t *testing.T) {
	home := t.TempDir()
	t.Setenv("SOPHOS_CENTRAL_HOME", home)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tamper-protection") {
			_, _ = io.WriteString(w, `{"enabled":true,"password":"`+cacheSentinel+`","previousPasswords":[]}`)
			return
		}
		_, _ = io.WriteString(w, `{"items":[{"id":"ordinary-row-marker"}]}`)
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, AuthHeaderVal: "Bearer stub"}, 5*time.Second, 0)
	if c.cacheDir == "" {
		t.Fatal("response cache is not enabled for this client; the test would prove nothing")
	}
	ctx := context.Background()
	if _, err := c.Get(ctx, "/endpoint/v1/endpoints/ep1/tamper-protection", nil); err != nil {
		t.Fatalf("tamper GET: %v", err)
	}
	if _, err := c.Get(ctx, "/endpoint/v1/endpoints", nil); err != nil {
		t.Fatalf("endpoints GET: %v", err)
	}
	sawOrdinary := false
	_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), cacheSentinel) {
			t.Fatalf("tamper protection password persisted to %s", path)
		}
		if strings.Contains(string(b), "ordinary-row-marker") {
			sawOrdinary = true
		}
		return nil
	})
	if !sawOrdinary {
		t.Fatal("ordinary GET response was not cached; the guard must only skip secret-bearing bodies")
	}
}

func TestResponseCarriesSecret(t *testing.T) {
	cases := map[string]bool{
		`{"password":"x"}`:                                       true,
		`{"previousPasswords":[{"password":"y"}]}`:               true,
		`{"items":[{"id":"m1","token":"z"}]}`:                    true,
		`{"items":[{"id":"e1","tamperProtectionEnabled":true}]}`: false,
		`not json`: false,
	}
	for body, want := range cases {
		if got := responseCarriesSecret([]byte(body)); got != want {
			t.Errorf("responseCarriesSecret(%s) = %v, want %v", body, got, want)
		}
	}
}
