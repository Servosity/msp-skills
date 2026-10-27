// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-fix (handfixes.json vault-secrets-withheld): proves the client decides
// by RESOURCE, not by the shape of a body. Each test feeds one input that the
// shape-based filter let through: an error body whose secret sits inside an
// array, a redirect into the vault or assets from an unprotected path, plain
// text and HTML bodies, a vault key ending in _at, and password-type asset
// field values. These tests use only client APIs that predate the fix, so
// they also run (and fail) against the unfixed client.

package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func assertNoSentinel(t *testing.T, label string, sentinel string, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(text, sentinel) {
			t.Errorf("%s leaked %s: %s", label, sentinel, text)
		}
	}
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Codex r2 P1 #2: a vault validation error echoing the rejected value inside
// an array under a secret-named key.
func TestProtectVaultValidationErrorWithheld(t *testing.T) {
	const sentinel = "SENTINEL-ERR"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"errors":{"password":["rejected value: ` + sentinel + `"]}}`))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)

	data, status, err := c.Post(context.Background(), "/asset_passwords", map[string]any{"company_id": 1, "name": "vpn", "password": "typed-by-operator"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity || status != http.StatusUnprocessableEntity {
		t.Fatalf("want a 422 APIError, got status=%d err=%v", status, err)
	}
	assertNoSentinel(t, "vault 422 error", sentinel, errText(err), string(data), apiErr.Body)
}

// Redirect: an unprotected request path that a same-origin redirect answers
// from the vault or assets is filtered by the URL that answered.
func TestProtectRedirectIntoProtectedResource(t *testing.T) {
	const pw, af, echo = "SENTINEL-REDIR-PW", "SENTINEL-REDIR-AF", "SENTINEL-REDIR-ERR"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/companies/77":
			http.Redirect(w, r, "/asset_passwords/1", http.StatusFound)
		case "/asset_passwords/1":
			_, _ = w.Write([]byte(`{"asset_password":{"id":1,"name":"vpn","username":"u","password":"` + pw + `","otp_secret":"` + pw + `"}}`))
		case "/companies/78":
			http.Redirect(w, r, "/assets/5", http.StatusFound)
		case "/assets/5":
			_, _ = w.Write([]byte(`{"asset":{"id":5,"asset_layout_id":3,"fields":[{"id":10,"label":"Admin Password","value":"` + af + `","position":1},{"id":11,"label":"Hostname","value":"dc01","position":2}]}}`))
		case "/asset_layouts/3":
			_, _ = w.Write([]byte(`{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"Password"},{"label":"Hostname","field_type":"Text"}]}}`))
		case "/companies/79":
			http.Redirect(w, r, "/asset_passwords/2", http.StatusFound)
		case "/asset_passwords/2":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"` + echo + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	ctx := context.Background()

	data, err := c.Get(ctx, "/companies/77", nil)
	assertNoSentinel(t, "redirect into the vault", pw, string(data), errText(err))
	if err != nil || !strings.Contains(string(data), `"username":"u"`) {
		t.Fatalf("redirect into the vault: want the filtered entry, got err=%v body=%s", err, data)
	}

	data, err = c.Get(ctx, "/companies/78", nil)
	assertNoSentinel(t, "redirect into assets", af, string(data), errText(err))
	if err != nil || !strings.Contains(string(data), "dc01") {
		t.Fatalf("redirect into assets: want the filtered asset, got err=%v body=%s", err, data)
	}

	data, err = c.Get(ctx, "/companies/79", nil)
	if err == nil {
		t.Fatalf("redirect into a vault error: want an error, got %s", data)
	}
	assertNoSentinel(t, "redirect into a vault error", echo, string(data), errText(err))
}

// Text bodies from a vault URL: a plain-text 500, an HTML 422 whose title
// carries the value, and a 200 HTML page with that title.
func TestProtectVaultTextBodiesWithheld(t *testing.T) {
	const txt, html = "SENTINEL-TXT", "SENTINEL-HTML"
	page := `<!doctype html><html><head><title>` + html + `</title></head><body>vault</body></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/asset_passwords/text-error":
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("password=" + txt + "\n"))
		case "/asset_passwords/html-error":
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(page))
		case "/asset_passwords/html-page":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(page))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	ctx := context.Background()

	// POST so the 500 is not retried with backoff.
	data, _, err := c.Post(ctx, "/asset_passwords/text-error", map[string]any{"name": "x"})
	if err == nil {
		t.Fatalf("text 500: want an error, got %s", data)
	}
	assertNoSentinel(t, "text/plain 500", txt, errText(err), string(data))

	data, _, err = c.Post(ctx, "/asset_passwords/html-error", map[string]any{"name": "x"})
	if err == nil {
		t.Fatalf("html 422: want an error, got %s", data)
	}
	assertNoSentinel(t, "text/html 422", html, errText(err), string(data))

	data, err = c.Get(ctx, "/asset_passwords/html-page", nil)
	if err == nil {
		t.Fatalf("html 200: want an error, got %s", data)
	}
	assertNoSentinel(t, "text/html 200", html, errText(err), string(data))
}

// The vault allowlist is exact: a key ending in _at that is not on it, such as
// otp_secret_at, is dropped.
func TestProtectVaultExactAllowlistDropsAtSuffix(t *testing.T) {
	const sentinel = "SENTINEL-AT"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"asset_password":{"id":9,"name":"vpn","otp_secret_at":"` + sentinel + `","updated_at":"2026-02-03T04:05:06Z"}}`))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)

	data, err := c.Get(context.Background(), "/asset_passwords/9", nil)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	assertNoSentinel(t, "vault entry", sentinel, string(data))
	if !strings.Contains(string(data), `"updated_at":"2026-02-03T04:05:06Z"`) {
		t.Fatalf("an allowlisted timestamp was dropped: %s", data)
	}
}

// Assets: a password-type field value is replaced by the marker, a Text field
// value is kept; when the layout lookup fails every non-empty value is
// replaced; an asset error body echoing a value is withheld.
func TestProtectAssetPasswordFieldValues(t *testing.T) {
	const sentinel = "SENTINEL-AF"
	assetJSON := func(id, layoutID, hostname string) string {
		return `{"id":` + id + `,"asset_layout_id":` + layoutID + `,"name":"dc","fields":[{"id":10,"label":"Admin Password","value":"` + sentinel + `","position":1},{"id":11,"label":"Hostname","value":"` + hostname + `","position":2},{"id":12,"label":"Notes","value":"","position":3}]}`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/assets/5":
			_, _ = w.Write([]byte(`{"asset":` + assetJSON("5", "3", "dc01") + `}`))
		case r.URL.Path == "/companies/3/assets":
			_, _ = w.Write([]byte(`{"assets":[` + assetJSON("5", "3", "dc01") + `]}`))
		case r.URL.Path == "/assets/6":
			_, _ = w.Write([]byte(`{"asset":` + assetJSON("6", "4", "dc02") + `}`))
		case r.URL.Path == "/asset_layouts/3":
			_, _ = w.Write([]byte(`{"asset_layout":{"id":3,"fields":[{"label":"Admin Password","field_type":"Password"},{"label":"Hostname","field_type":"Text"},{"label":"Notes","field_type":"ConfidentialText"}]}}`))
		case r.URL.Path == "/assets" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"errors":{"fields":["Admin Password ` + sentinel + ` is invalid"]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	ctx := context.Background()

	fieldValues := func(label string, data json.RawMessage) map[string]any {
		t.Helper()
		var env struct {
			Asset  map[string]any   `json:"asset"`
			Assets []map[string]any `json:"assets"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("%s: not an asset envelope: %v: %s", label, err, data)
		}
		asset := env.Asset
		if asset == nil && len(env.Assets) > 0 {
			asset = env.Assets[0]
		}
		out := map[string]any{}
		fields, _ := asset["fields"].([]any)
		for _, f := range fields {
			entry, _ := f.(map[string]any)
			lbl, _ := entry["label"].(string)
			out[lbl] = entry["value"]
		}
		return out
	}

	for _, p := range []string{"/assets/5", "/companies/3/assets"} {
		data, err := c.Get(ctx, p, nil)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		assertNoSentinel(t, p, sentinel, string(data))
		values := fieldValues(p, data)
		if values["Hostname"] != "dc01" {
			t.Fatalf("%s: a Text field value was not kept: %s", p, data)
		}
		if v, _ := values["Admin Password"].(string); v == "" {
			t.Fatalf("%s: a filled password field no longer reads as filled: %s", p, data)
		}
		if values["Notes"] != "" {
			t.Fatalf("%s: an empty field was changed: %s", p, data)
		}
	}

	// The layout lookup fails (404): every non-empty value is replaced.
	data, err := c.Get(ctx, "/assets/6", nil)
	if err != nil {
		t.Fatalf("/assets/6: %v", err)
	}
	assertNoSentinel(t, "/assets/6 with no layout", sentinel, string(data))
	assertNoSentinel(t, "/assets/6 with no layout", "dc02", string(data))

	// An asset error body echoing a value is withheld.
	data, _, err = c.Post(ctx, "/assets", map[string]any{"name": "dc"})
	if err == nil {
		t.Fatalf("asset 422: want an error, got %s", data)
	}
	assertNoSentinel(t, "asset 422 error", sentinel, errText(err), string(data))
}

// A body read through this CLI carries the withheld-field placeholder; sending
// it back would overwrite the real password-type value in Hudu, so the client
// refuses before the wire.
func TestProtectWithheldMarkerNeverSent(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"asset":{"id":5}}`))
	}))
	t.Cleanup(srv.Close)
	c := newVaultTestClient(t, srv.URL)
	body := map[string]any{"asset": map[string]any{"custom_fields": []any{map[string]any{"admin_password": "[withheld: password field]"}}}}
	if _, _, err := c.Put(context.Background(), "/companies/3/assets/5", body); err == nil {
		t.Fatalf("a body carrying the placeholder was sent")
	}
	if hits != 0 {
		t.Fatalf("the placeholder reached the server: %d requests", hits)
	}
	if _, _, err := c.Put(context.Background(), "/companies/3/assets/5", map[string]any{"asset": map[string]any{"name": "dc01"}}); err != nil || hits != 1 {
		t.Fatalf("an ordinary update was refused: err=%v hits=%d", err, hits)
	}
}
