// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored: the response cache is a credential-tier sink. Every GET the
// CLI or the MCP server issues goes through Client.GetWithHeaders, which writes
// the body to <cacheDir>/resources/<tag>/<sha>.json for five minutes and serves
// the next identical request from that file. For the two reads that return an
// agent-enrollment credential that is a plaintext secret on disk and a second
// path that answers without the API ever being asked. The paths below are this
// test's own oracle, not read from the production allowlist.

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"threatlocker-pp-cli/internal/config"
)

var tlCredentialTierPaths = []string{
	"/Organization/OrganizationGetAuthKeyById",
	"/Computer/ComputerGetForNewComputer",
}

func tlCredentialClient(t *testing.T, handler func(path string) any) (*Client, func() int64) {
	t.Helper()
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(handler(r.URL.Path))
	}))
	t.Cleanup(srv.Close)
	cfg := &config.Config{
		BaseURL:            srv.URL,
		AuthSource:         "env:THREATLOCKER_API_KEY",
		ThreatlockerApiKey: "0000000000000000000000000000000000000000000000000000000000000003",
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	return c, func() int64 { return atomic.LoadInt64(&hits) }
}

func tlCacheFilesContaining(t *testing.T, root, needle string) []string {
	t.Helper()
	var hits []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if data, readErr := os.ReadFile(path); readErr == nil && bytes.Contains(data, []byte(needle)) { // #nosec G304 -- test temp dir.
			hits = append(hits, path)
		}
		return nil
	})
	return hits
}

// Control: an ordinary read IS cached, so the absence checks below are real.
func TestCredentialTierCacheControlOrdinaryReadIsCached(t *testing.T) {
	const marker = "ordinary-cache-marker-51c3"
	c, hits := tlCredentialClient(t, func(string) any { return map[string]any{"name": marker} })
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), "/ComputerGroup/ComputerGroupGetGroupAndComputer", nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := hits(); got != 1 {
		t.Fatalf("ordinary read reached the wire %d times, want 1 (second served from cache)", got)
	}
	if len(tlCacheFilesContaining(t, c.cacheDir, marker)) == 0 {
		t.Fatal("ordinary read left no cache file, so the credential-tier checks prove nothing")
	}
}

// The cache never stores a credential-tier response, never answers one from
// disk, and a query string, a case change, a doubled slash, a trailing slash
// or a percent-escaped letter (all of which the Portal API routes to the same
// endpoint) does not route around it.
func TestCredentialTierResponseNeverEntersTheCache(t *testing.T) {
	for _, path := range tlCredentialTierPaths {
		escaped := "/%" + strings.ToUpper(fmt.Sprintf("%x", path[1])) + path[2:]
		for _, variant := range []string{path, path + "?organizationId=org-1", strings.ToLower(path), "/" + path, path + "/", escaped} {
			variant := variant
			t.Run(variant, func(t *testing.T) {
				secret := "SENTINEL-cache-" + strings.Trim(strings.ReplaceAll(variant, "/", "-"), "-")
				c, hits := tlCredentialClient(t, func(string) any { return map[string]any{"authKey": secret, "installKey": secret} })
				for i := 0; i < 2; i++ {
					data, err := c.Get(context.Background(), variant, nil)
					if err != nil {
						t.Fatalf("GET %s: %v", variant, err)
					}
					if !strings.Contains(string(data), secret) {
						t.Fatalf("GET %s did not return the live body: %s", variant, data)
					}
				}
				if got := hits(); got != 2 {
					t.Fatalf("GET %s reached the wire %d time(s) in two calls; the second was served from the cache", variant, got)
				}
				if leaked := tlCacheFilesContaining(t, c.cacheDir, secret); len(leaked) > 0 {
					t.Fatalf("credential written to the response cache: %v", leaked)
				}
			})
		}
	}
}

// A cache file a previous binary already wrote must not be served after the
// upgrade, and the next call purges it.
func TestCredentialTierPreexistingCacheEntryIsNotServed(t *testing.T) {
	for _, path := range tlCredentialTierPaths {
		path := path
		t.Run(path, func(t *testing.T) {
			const stale = "SENTINEL-stale-cache-entry"
			c, hits := tlCredentialClient(t, func(string) any { return map[string]any{"authKey": "fresh-live-value"} })
			dir := c.cacheResourceDir(path)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			planted := filepath.Join(dir, c.cacheKeyFor(http.MethodGet, path, nil, nil, nil)+".json")
			if err := os.WriteFile(planted, []byte(`{"authKey":"`+stale+`"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			data, err := c.Get(context.Background(), path, nil)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), stale) || hits() != 1 {
				t.Fatalf("a credential was answered from a cache file (wire hits=%d): %s", hits(), data)
			}
			if _, err := os.Stat(planted); !os.IsNotExist(err) {
				t.Fatalf("the stale credential cache file is still on disk: %v", err)
			}
		})
	}
}

// Inside a process the MCP server spawned, the transport itself refuses a
// credential-tier path, so no command, novel or generated, can carry the
// value back to an MCP client. The same client in an operator's terminal
// still gets it.
func TestCredentialTierTransportRefusesInsideAnMCPChild(t *testing.T) {
	for _, path := range tlCredentialTierPaths {
		path := path
		t.Run(path, func(t *testing.T) {
			const secret = "SENTINEL-transport-mcp-child"
			c, hits := tlCredentialClient(t, func(string) any { return map[string]any{"authKey": secret} })
			c.NoCache = true
			t.Setenv("THREATLOCKER_MCP_CHILD", "1")
			data, err := c.Get(context.Background(), path, nil)
			if err == nil || strings.Contains(string(data), secret) {
				t.Fatalf("MCP child read %s: data=%s err=%v", path, data, err)
			}
			if !strings.Contains(err.Error(), "terminal") {
				t.Fatalf("refusal does not point at the terminal: %v", err)
			}
			if hits() != 0 {
				t.Fatalf("refused request still reached the wire %d time(s)", hits())
			}
			t.Setenv("THREATLOCKER_MCP_CHILD", "")
			data, err = c.Get(context.Background(), path, nil)
			if err != nil || !strings.Contains(string(data), secret) {
				t.Fatalf("operator terminal read %s: data=%s err=%v", path, data, err)
			}
		})
	}
}

// A redirect is a second way to reach a credential read: the request asks for
// an ordinary path and the server moves it onto the key. The response would be
// cached under the ordinary path's key (and, inside MCP, returned to the
// client), so a hop onto a credential read is refused in every process unless
// the request already asked for that same credential from a terminal.
func TestCredentialTierRedirectNeverLandsOnACredentialRead(t *testing.T) {
	const secret = "SENTINEL-redirected-authkey"
	var memberHits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/Organization/OrganizationGetForMoveComputers":
			http.Redirect(w, r, "/Organization/OrganizationGetAuthKeyById", http.StatusFound)
			return
		case "/Organization/OrganizationGetAuthKeyById":
			if r.URL.Query().Get("moved") == "" {
				http.Redirect(w, r, "/portalapi/Organization/OrganizationGetAuthKeyById?moved=1", http.StatusFound)
				return
			}
		case "/portalapi/Organization/OrganizationGetAuthKeyById":
		default:
			http.NotFound(w, r)
			return
		}
		atomic.AddInt64(&memberHits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"organizationId":"org-1","authKey":"` + secret + `"}`))
	}))
	t.Cleanup(srv.Close)
	newClient := func(t *testing.T) *Client {
		cfg := &config.Config{
			BaseURL:            srv.URL,
			AuthSource:         "env:THREATLOCKER_API_KEY",
			ThreatlockerApiKey: "0000000000000000000000000000000000000000000000000000000000000005",
		}
		c := New(cfg, 5*time.Second, 0)
		c.cacheDir = t.TempDir()
		return c
	}
	for _, child := range []string{"", "1"} {
		child := child
		t.Run("ordinary-path-redirected/mcp_child="+child, func(t *testing.T) {
			t.Setenv("THREATLOCKER_MCP_CHILD", child)
			atomic.StoreInt64(&memberHits, 0)
			c := newClient(t)
			data, err := c.Get(context.Background(), "/Organization/OrganizationGetForMoveComputers", nil)
			if strings.Contains(string(data), secret) || err == nil {
				t.Fatalf("a redirect from an ordinary path returned the credential: data=%s err=%v", data, err)
			}
			if leaked := tlCacheFilesContaining(t, c.cacheDir, secret); len(leaked) > 0 {
				t.Fatalf("a redirected credential was cached: %v", leaked)
			}
			if got := atomic.LoadInt64(&memberHits); got != 0 {
				t.Fatalf("the credential endpoint was reached %d time(s) through a redirect", got)
			}
		})
	}
	t.Run("operator-request-moved-by-the-api", func(t *testing.T) {
		t.Setenv("THREATLOCKER_MCP_CHILD", "")
		c := newClient(t)
		data, err := c.Get(context.Background(), "/Organization/OrganizationGetAuthKeyById", nil)
		if err != nil || !strings.Contains(string(data), secret) {
			t.Fatalf("the operator's own credential read, moved by the API, failed: data=%s err=%v", data, err)
		}
		if leaked := tlCacheFilesContaining(t, c.cacheDir, secret); len(leaked) > 0 {
			t.Fatalf("credential cached: %v", leaked)
		}
	})
}
