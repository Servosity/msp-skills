// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): the client half of the
// extra-round classes. A cache entry an older binary wrote under an
// unprotected path is never served; an asset field value is withheld under any
// envelope; a layout's field types are read again for every body. These tests
// use only client APIs that predate the fix, so they also run (and fail)
// against the unfixed client.

package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// Codex extra P1 (cache): an older binary followed /companies/77's redirect
// into the vault and cached the raw answer under the /companies/77 key, as
// "<key>.json". Before the fix GetWithHeaders served it unfiltered, because
// the requested path is unprotected and a cache hit has no answered URL.
func TestProtectLegacyCacheEntryNeverServed(t *testing.T) {
	const pw = "SENTINEL-CACHE-PW"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"company":{"id":77,"name":"Acme"}}`))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	c.NoCache = false

	const path = "/companies/77"
	dir := c.cacheResourceDir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, c.cacheKeyFor(http.MethodGet, path, nil, nil, nil)+".json")
	if err := os.WriteFile(legacy, []byte(`{"asset_password":{"id":1,"name":"vpn","password":"`+pw+`","otp_secret":"`+pw+`"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := c.Get(context.Background(), path, nil)
	assertNoSentinel(t, "legacy cache entry", pw, string(data), errText(err))
	if err != nil || !strings.Contains(string(data), "Acme") || hits.Load() != 1 {
		t.Fatalf("want the live answer, got hits=%d err=%v body=%s", hits.Load(), err, data)
	}
	// This client's own entry is served on the next read.
	if data, err = c.Get(context.Background(), path, nil); err != nil || !strings.Contains(string(data), "Acme") || hits.Load() != 1 {
		t.Fatalf("want this client's cache entry, got hits=%d err=%v body=%s", hits.Load(), err, data)
	}
}

// Codex extra P1 (asset envelope): a 200 from an asset path whose asset sits
// under an envelope the filter did not list was returned unchanged.
func TestProtectAssetFieldsUnderAnyEnvelope(t *testing.T) {
	const af = "SENTINEL-ENV-AF"
	asset := `{"id":5,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + af + `"},{"label":"Hostname","value":"dc01"}]}`
	bodies := map[string]string{
		"/assets/5":            `{"data":{"asset":` + asset + `}}`,
		"/assets/6":            `{"data":{"assets":[` + asset + `]}}`,
		"/assets/7":            `{"results":[{"item":` + asset + `}]}`,
		"/companies/3/assets":  `[{"wrapper":[` + asset + `]}]`,
		"/assets/8":            `{"asset":{"id":8,"fields":[{"label":"Admin Password","value":"` + af + `"}]}}`,
		"/assets/9/no-layout":  `{"page":{"asset":{"fields":[{"label":"Admin Password","value":"` + af + `"}]}}}`,
		"/assets/10/map-shape": `{"data":{"asset":{"asset_layout_id":3,"custom_fields":[{"admin_password":"` + af + `"}]}}}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/asset_layouts/3" {
			_, _ = w.Write([]byte(`{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"Password"},{"label":"Hostname","field_type":"Text"}]}}`))
			return
		}
		body, ok := bodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)

	for path := range bodies {
		data, err := c.Get(context.Background(), path, nil)
		assertNoSentinel(t, path, af, string(data), errText(err))
		if err == nil && strings.Contains(bodies[path], "dc01") && !strings.Contains(string(data), "dc01") {
			t.Errorf("%s: a Text field value the layout declares was not kept: %s", path, data)
		}
	}
}

// Codex extra P1 (layout memo): during a multipage sync, layout 3's "Admin
// Password" is Text when page one is filtered, then becomes Password and is
// filled before page two. Before the fix the client reused page one's Text
// answer for the life of the client and returned page two's value.
func TestProtectLayoutTypesReadForEveryBody(t *testing.T) {
	const af = "SENTINEL-PAGE2-AF"
	var layoutReads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/asset_layouts/3":
			fieldType := "Text"
			if layoutReads.Add(1) > 1 {
				fieldType = "Password"
			}
			_, _ = w.Write([]byte(`{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"` + fieldType + `"}]}}`))
		case "/assets":
			value := "not-yet-a-secret"
			if r.URL.Query().Get("page") == "2" {
				value = af
			}
			_, _ = w.Write([]byte(`{"assets":[{"id":5,"asset_layout_id":3,"fields":[{"label":"Admin Password","value":"` + value + `"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	ctx := context.Background()

	page1, err := c.Get(ctx, "/assets", map[string]string{"page": "1"})
	if err != nil || !strings.Contains(string(page1), "not-yet-a-secret") {
		t.Fatalf("page 1: want the Text value, got err=%v body=%s", err, page1)
	}
	page2, err := c.Get(ctx, "/assets", map[string]string{"page": "2"})
	assertNoSentinel(t, "page 2 after the layout changed", af, string(page2), errText(err))
	if layoutReads.Load() < 2 {
		t.Fatalf("the layout was read %d time(s) for two bodies", layoutReads.Load())
	}
}
