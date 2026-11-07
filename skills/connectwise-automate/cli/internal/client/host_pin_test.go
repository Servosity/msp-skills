// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectwise-automate-pp-cli/internal/config"
)

// After the client is built, no change to Config.TemplateVars can move the
// credentials to another server (handfix base-url-pinned-at-construction).
func TestClientRefusesABaseURLThatMovedAfterConstruction(t *testing.T) {
	var tenantHits, otherHits atomic.Int32
	serve := func(n *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			_, _ = w.Write([]byte(`[]`))
		}))
	}
	tenant, other := serve(&tenantHits), serve(&otherHits)
	defer tenant.Close()
	defer other.Close()
	t.Setenv("CONNECTWISE_AUTOMATE_BASE_URL", "http://{server}/cwa/api/v1")
	t.Setenv("CONNECTWISE_AUTOMATE_SERVER", strings.TrimPrefix(tenant.URL, "http://"))
	t.Setenv("CONNECTWISE_AUTOMATE_TOKEN", "tenant-secret")
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	if _, err := c.Get(context.Background(), "/Computers", nil); err != nil {
		t.Fatalf("request to the configured server: %v", err)
	}
	cfg.TemplateVars["server"] = strings.TrimPrefix(other.URL, "http://")
	if _, err := c.Get(context.Background(), "/Computers", nil); err == nil || !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("want a refusal, got %v", err)
	}
	if tenantHits.Load() != 1 || otherHits.Load() != 0 {
		t.Fatalf("tenant=%d other=%d, want 1 and 0", tenantHits.Load(), otherHits.Load())
	}
}
