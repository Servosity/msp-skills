// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "redirect-strips-every-custom-credential",
// "dry-run-never-prints-a-credential" and
// "response-text-in-errors-is-masked" entries.

package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/config"
)

// An off-origin 307/308 that would replay a credential-bearing body (a new
// password, an SSO client secret) or a write method is refused before the
// other host sees it; an off-origin GET, a POST that a 302 turns into a
// bodiless GET, and a same-origin 307 are still followed.
func TestRedirectOffOriginNeverReplaysTheBody(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	var mu sync.Mutex
	var foreign []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		foreign = append(foreign, r.Method+" "+r.URL.Path+" "+string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(other.Close)
	// A hostname, not an IP literal: the private-address refusal must not be
	// what stops the hop.
	collect := strings.Replace(other.URL, "127.0.0.1", "localhost", 1) + "/collect"
	var landed string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/users/change_password", "/v1/networks/9", "/v1/organizations":
			w.Header().Set("Location", collect)
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "/v1/enterprise_connections":
			w.Header().Set("Location", collect)
			w.WriteHeader(http.StatusPermanentRedirect)
		case "/v1/policies":
			w.Header().Set("Location", collect)
			w.WriteHeader(http.StatusFound)
		case "/v1/same":
			w.Header().Set("Location", "/v1/landed")
			w.WriteHeader(http.StatusTemporaryRedirect)
		case "/v1/landed":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			landed = r.Method + " " + string(body)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	t.Cleanup(origin.Close)
	c := New(&config.Config{BaseURL: origin.URL, DnsfilterApiKey: "r18-key"}, 5*time.Second, 0)
	c.NoCache = true
	ctx := context.Background()

	if _, _, err := c.Patch(ctx, "/v1/users/change_password", map[string]any{"new_password": "R18-NEW-PASSWORD"}); !errors.Is(err, ErrRedirectBodyReplay) {
		t.Fatalf("PATCH change_password followed an off-origin 307: %v", err)
	}
	if _, _, err := c.Post(ctx, "/v1/enterprise_connections", map[string]any{"options": map[string]any{"client_secret": "R18-CLIENT-SECRET"}}); !errors.Is(err, ErrRedirectBodyReplay) {
		t.Fatalf("POST enterprise_connections followed an off-origin 308: %v", err)
	}
	if _, _, err := c.Delete(ctx, "/v1/networks/9"); !errors.Is(err, ErrRedirectBodyReplay) {
		t.Fatalf("DELETE followed an off-origin 307: %v", err)
	}
	mu.Lock()
	for _, got := range foreign {
		t.Errorf("a body- or method-replaying request reached the other host: %q", got)
	}
	foreign = nil
	mu.Unlock()

	if _, err := c.Get(ctx, "/v1/organizations", nil); err != nil {
		t.Fatalf("an off-origin GET redirect was refused: %v", err)
	}
	if _, _, err := c.Post(ctx, "/v1/policies", map[string]any{"name": "r18"}); err != nil {
		t.Fatalf("a POST that a 302 turns into a bodiless GET was refused: %v", err)
	}
	if _, _, err := c.Patch(ctx, "/v1/same", map[string]any{"name": "same-origin"}); err != nil {
		t.Fatalf("a same-origin 307 was refused: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(foreign, "|") != "GET /collect |GET /collect " {
		t.Fatalf("other host saw %q, want two bodiless GETs", foreign)
	}
	if landed != `PATCH {"name":"same-origin"}` {
		t.Fatalf("same-origin 307 landed as %q, want the PATCH with its body", landed)
	}
}

// --dry-run never prints a credential: credential-named body fields at any
// depth, credential text in a string, a credential-named query parameter (in
// params or in the path) and the configured key are shown as ****, while the
// field names and every ordinary value stay readable.
func TestDryRunNeverPrintsACredential(t *testing.T) {
	testenv.Isolate(t, cliutil.CacheDir)
	c := New(&config.Config{BaseURL: "https://api.dnsfilter.test", DnsfilterApiKey: "R18-CONFIGURED-KEY"}, 5*time.Second, 0)
	c.DryRun = true
	body := map[string]any{
		"organization_id": 1,
		"strategy":        "oidc",
		"options":         map[string]any{"client_secret": "R18-CLIENT-SECRET", "domain": "sso.example.com"},
		"payment_token":   "R18-PAYMENT-TOKEN",
		"new_password":    "R18-NEW-PASSWORD",
		"notes":           []any{"rotate with password: R18-NOTE-PASSWORD", "plain note"},
		"echo":            "key R18-CONFIGURED-KEY",
		"secret_key":      nil,
		"enabled":         true,
	}
	stderr := captureR17Stderr(t, func() {
		ctx := context.Background()
		if _, _, err := c.Post(ctx, "/v1/enterprise_connections", body); err != nil {
			t.Errorf("dry-run POST: %v", err)
		}
		if _, err := c.Get(ctx, "/v1/networks", map[string]string{"token": "R18-PARAM-TOKEN", "page": "2"}); err != nil {
			t.Errorf("dry-run GET: %v", err)
		}
		if _, err := c.Get(ctx, "/v1/networks?api_key=R18-QUERY-KEY&page=3", nil); err != nil {
			t.Errorf("dry-run GET with a query: %v", err)
		}
	})
	for _, secret := range []string{"R18-CLIENT-SECRET", "R18-PAYMENT-TOKEN", "R18-NEW-PASSWORD", "R18-NOTE-PASSWORD", "R18-CONFIGURED-KEY", "R18-PARAM-TOKEN", "R18-QUERY-KEY"} {
		if strings.Contains(stderr, secret) {
			t.Errorf("--dry-run printed %s:\n%s", secret, stderr)
		}
	}
	for _, kept := range []string{`"client_secret": "****"`, `"payment_token": "****"`, `"new_password": "****"`, `"sso.example.com"`, `"oidc"`, `"plain note"`, `"enabled": true`, `"secret_key": null`, "page=2", "page=3", "token=****", "api_key=****"} {
		if !strings.Contains(stderr, kept) {
			t.Errorf("--dry-run preview lost %s:\n%s", kept, stderr)
		}
	}
}

func r18ErrorServer(t *testing.T, status int, contentType, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func r18GetError(t *testing.T, cfg *config.Config) string {
	t.Helper()
	c := New(cfg, 5*time.Second, 0)
	c.NoCache = true
	_, err := c.Get(context.Background(), "/v1/networks/7", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	return err.Error()
}

// The 4 KiB cut never splits a credential past the masker or a credential-
// named JSON key past the filters: the configured key is masked in the whole
// body first, and a body the cut would leave undecodable (JSON, or text with
// JSON in it) loses its credential-named keys before the cut.
func TestErrorBodyIsMaskedBeforeTheCut(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	key := "R18KEY-" + strings.Repeat("k", 25) // 32 bytes
	pad := strings.Repeat("x", 5000)
	cases := []struct {
		name, contentType, body, leak string
	}{
		{"configured key across the cut", "text/plain", strings.Repeat("p", 4065) + key, key[:16]},
		{"credential-named key in long JSON", "application/json", `{"pass word":"Z9QX7R18","padding":"` + pad + `"}`, "Z9QX7R18"},
		{"credential-named key in JSON inside long text", "text/plain", `upstream said: {"pass word":"Z9QX7R18T","padding":"` + pad + `"} end`, "Z9QX7R18T"},
		{"body past the scrub bound", "application/json", `{"pass word":"Z9QX7R18C","padding":"` + strings.Repeat(pad, 14) + `"}`, "Z9QX7R18C"},
	}
	for _, ro := range []string{"", "1"} {
		for _, tc := range cases {
			t.Run(tc.name+" read-only="+ro, func(t *testing.T) {
				t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
				base := r18ErrorServer(t, http.StatusUnprocessableEntity, tc.contentType, tc.body)
				msg := r18GetError(t, &config.Config{BaseURL: base, DnsfilterApiKey: key})
				if strings.Contains(msg, tc.leak) {
					t.Fatalf("the error kept %q:\n%.300s", tc.leak, msg)
				}
				if !strings.Contains(msg, "HTTP 422") {
					t.Fatalf("the error lost its status: %.300s", msg)
				}
			})
		}
	}
	// A short JSON error body is left whole for the filters downstream.
	base := r18ErrorServer(t, http.StatusUnprocessableEntity, "application/json", `{"errors":{"new_password":["is too short"]}}`)
	if msg := r18GetError(t, &config.Config{BaseURL: base, DnsfilterApiKey: key}); !strings.Contains(msg, "is too short") {
		t.Fatalf("a short validation error lost its detail: %s", msg)
	}
}

// An HTML page whose markup names a credential is summarized without its
// title (the title extractor strips the label and keeps the value), as an
// error page and as a 2xx document; a title that echoes the configured key,
// raw or as HTML entities, is masked before it is cut to 160 runes.
func TestHTMLTitleNeverCarriesACredential(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	page := `<html><title><secret_key>NEWSECRET123</secret_key></title></html>`
	for _, status := range []int{http.StatusUnprocessableEntity, http.StatusOK} {
		base := r18ErrorServer(t, status, "text/html", page)
		if msg := r18GetError(t, &config.Config{BaseURL: base, DnsfilterApiKey: "r18-key"}); strings.Contains(msg, "NEWSECRET123") || !strings.Contains(msg, "title not shown") {
			t.Fatalf("HTTP %d: an HTML title that names a credential reached the error: %s", status, msg)
		}
	}
	key := "R18TITLEKEY" + strings.Repeat("q", 21)
	var entities strings.Builder
	for _, r := range key {
		fmt.Fprintf(&entities, "&#%d;", r)
	}
	for name, echoed := range map[string]string{"raw": key, "entities": entities.String()} {
		title := strings.Repeat("t", 150) + " " + echoed
		for _, status := range []int{http.StatusConflict, http.StatusOK} {
			base := r18ErrorServer(t, status, "text/html", "<!doctype html><html><head><title>"+title+"</title></head><body>x</body></html>")
			msg := r18GetError(t, &config.Config{BaseURL: base, DnsfilterApiKey: key})
			if strings.Contains(msg, key[:9]) {
				t.Fatalf("%s key, HTTP %d: the 160-rune title cut kept part of the key: %s", name, status, msg)
			}
		}
	}
}

// A credential the response echoes in another spelling is masked too:
// JSON-escaped (a quote, "/" as "\/", HTML-escaped "<"), HTML-escaped and
// lower-cased, in a [headers] value or the API key.
func TestErrorMaskingCoversEchoedForms(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	cases := []struct {
		name, key, header, contentType, body, leak string
	}{
		{"JSON-escaped quote in a header value", "", `proxy"r18secret`, "application/json", `{"echo":"proxy\"r18secret"}`, "r18secret"},
		{"JSON-escaped slash", "r18/slash/key", "", "application/json", `{"echo":"r18\/slash\/key"}`, "slash"},
		{"JSON-escaped HTML character", "r18<angle>key", "", "application/json", `{"echo":"r18\u003cangle\u003ekey"}`, "angle"},
		{"HTML-escaped", "r18&amp<key>", "", "text/plain", `<error>r18&amp;amp&lt;key&gt;</error>`, "amp;amp"},
		{"lower-cased", "R18MixedCaseKey", "", "application/json", `{"echo":"r18mixedcasekey"}`, "mixedcase"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{BaseURL: r18ErrorServer(t, http.StatusBadRequest, tc.contentType, tc.body), DnsfilterApiKey: tc.key}
			if tc.header != "" {
				cfg.DnsfilterApiKey = "r18-plain"
				cfg.Headers = map[string]string{"X-Api-Key": tc.header}
			}
			if msg := r18GetError(t, cfg); strings.Contains(strings.ToLower(msg), tc.leak) {
				t.Fatalf("the echoed credential reached the error: %s", msg)
			}
		})
	}
}
