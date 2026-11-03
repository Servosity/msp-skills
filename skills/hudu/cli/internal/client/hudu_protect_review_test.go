// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): the client half of the
// next-batch review classes. What the filter returns is the tree it inspected;
// a vault or asset answer is never cached; only a 2xx protected body, or a 2xx
// layout read, is trusted. These tests use only client APIs that predate the
// fix, so they also run (and fail) against the unfixed client.

package client

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const reviewLayout3 = `{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"Password"},{"label":"Hostname","field_type":"Text"}]}}`

// Duplicate and loosely spelled members: before the fix the filter returned
// the input bytes when it changed nothing, so a secret in a member the decoder
// dropped (a duplicate "fields") reached output and the cache, and a loosely
// spelled key ("Fields", "Value") was never inspected.
func TestProtectAssetOutputIsTheInspectedTree(t *testing.T) {
	const af = "SENTINEL-TREE-AF"
	bodies := map[string]string{
		"/assets/5": `{"asset":{"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + af + `"}],"fields":[]}}`,
		"/assets/6": `{"asset":{"asset_layout_id":3,"Fields":[{"label":"Admin Password","value":"` + af + `"}]}}`,
		"/assets/7": `{"asset":{"asset_layout_id":3,"fields":[{"label":"Hostname","value":"dc01","Value":"` + af + `"}]}}`,
		"/assets/8": `{"asset":{"asset_layout_id":3,"Custom-Fields":[{"admin_password":"` + af + `"}]}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/asset_layouts/3" {
			_, _ = w.Write([]byte(reviewLayout3))
			return
		}
		_, _ = w.Write([]byte(bodies[r.URL.Path]))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	for path := range bodies {
		data, err := c.Get(context.Background(), path, nil)
		assertNoSentinel(t, path, af, string(data), errText(err))
	}
}

// Cache provenance: /companies/78 redirected to an asset whose field was Text,
// then the layout made it Password. Before the fix the second read came from
// the cache, classified by /companies/78 alone, and showed the value.
func TestProtectRedirectedAnswerNeverCached(t *testing.T) {
	const af = "SENTINEL-REDIR-AF"
	for name, first := range map[string]func(*Client, context.Context, string) ([]byte, error){
		"Get": func(c *Client, ctx context.Context, p string) ([]byte, error) { return c.Get(ctx, p, nil) },
		"GetNoCache": func(c *Client, ctx context.Context, p string) ([]byte, error) {
			return c.GetNoCache(ctx, p, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var password atomic.Bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/companies/78":
					http.Redirect(w, r, "/assets/5", http.StatusFound)
				case "/assets/5":
					_, _ = w.Write([]byte(`{"asset":{"id":5,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + af + `"}]}}`))
				case "/asset_layouts/3":
					fieldType := "Text"
					if password.Load() {
						fieldType = "Password"
					}
					_, _ = w.Write([]byte(`{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"` + fieldType + `"}]}}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)
			c := newVaultTestClient(t, srv.URL)
			c.NoCache = false
			ctx := context.Background()
			if _, err := first(c, ctx, "/companies/78"); err != nil {
				t.Fatalf("first read: %v", err)
			}
			password.Store(true)
			data, err := c.Get(ctx, "/companies/78", nil)
			assertNoSentinel(t, "read after the layout changed", af, string(data), errText(err))
			_ = filepath.WalkDir(c.cacheDir, func(path string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() {
					if b, rerr := os.ReadFile(path); rerr == nil && strings.Contains(string(b), af) {
						t.Errorf("cache file %s holds a protected answer", path)
					}
				}
				return nil
			})
		})
	}
}

// Non-success status: a protected answer outside 2xx that the client does not
// treat as an error (a 3xx Hudu did not redirect) was filtered as a success
// and shown, and a layout read answered that way was trusted.
func TestProtectNonSuccessStatusWithheld(t *testing.T) {
	const af = "SENTINEL-3XX-AF"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/assets/5":
			w.WriteHeader(http.StatusFound)
			_, _ = w.Write([]byte(`{"errors":{"password":["rejected value: ` + af + `"]}}`))
		case "/asset_passwords/7":
			w.WriteHeader(http.StatusMultipleChoices)
			_, _ = w.Write([]byte(`{"asset_password":{"id":7,"name":"vpn","password":"` + af + `"}}`))
		case "/assets/9":
			_, _ = w.Write([]byte(`{"asset":{"id":9,"asset_layout_id":4,"fields":[{"label":"Admin Password","value":"` + af + `"}]}}`))
		case "/asset_layouts/4":
			w.WriteHeader(http.StatusMultipleChoices)
			_, _ = w.Write([]byte(`{"asset_layout":{"id":4,"fields":[{"label":"Admin Password","field_type":"Text"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	for _, path := range []string{"/assets/5", "/asset_passwords/7", "/assets/9"} {
		data, err := c.Get(context.Background(), path, nil)
		assertNoSentinel(t, path, af, string(data), errText(err))
	}
	if _, err := c.Get(context.Background(), "/assets/5", nil); err == nil {
		t.Error("a protected 302 body was shown as a success")
	}
}
