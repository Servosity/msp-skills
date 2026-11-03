// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"watchguard-pp-cli/internal/config"
)

func TestScopedBearerSendsAudienceAndCaches(t *testing.T) {
	var mints int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "AID" || pass != "PW" {
			t.Errorf("basic auth = %q/%q %v", user, pass, ok)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("scope") != "api-access" || r.Form.Get("audience") != "aud-123" {
			t.Errorf("form = %v", r.Form)
		}
		atomic.AddInt32(&mints, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-aud", "expires_in": 3600})
	}))
	defer srv.Close()

	c := &Client{HTTPClient: srv.Client(), Config: &config.Config{ClientID: "AID", ClientSecret: "PW", TokenURL: srv.URL}}
	for i := 0; i < 2; i++ {
		bearer, _, err := c.ScopedBearer(context.Background(), "aud-123")
		if err != nil || bearer != "Bearer tok-aud" {
			t.Fatalf("bearer = %q err = %v", bearer, err)
		}
	}
	if mints != 1 {
		t.Fatalf("token minted %d times, want 1 (cached)", mints)
	}
}

func TestScopedBearerErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()
	c := &Client{HTTPClient: srv.Client(), Config: &config.Config{ClientID: "AID", ClientSecret: "bad", TokenURL: srv.URL}}
	if _, _, err := c.ScopedBearer(context.Background(), "aud-401"); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("want a 401 error, got %v", err)
	}
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	empty := &Client{HTTPClient: srv.Client(), Config: &config.Config{TokenURL: srv.URL}}
	if _, _, err := empty.ScopedBearer(context.Background(), "aud-x"); err == nil {
		t.Fatalf("missing credentials must error")
	}
}

func TestTenantHeadersSkipsOwnAccount(t *testing.T) {
	c := &Client{Config: &config.Config{TemplateVars: map[string]string{"accountId": "ACC-SELF"}}}
	h, err := c.TenantHeaders(context.Background(), "ACC-SELF")
	if err != nil || h != nil {
		t.Fatalf("own account must use default auth, got %v %v", h, err)
	}
}

// tokenAndAPI serves /oauth/token (audience -> tok-<audience>) and records the
// Authorization of API calls; apiStatus decides each API response.
func tokenAndAPI(t *testing.T, apiStatus func(auth string) int) (*httptest.Server, *[]string, *int32) {
	t.Helper()
	var seen []string
	var mints int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = r.ParseForm()
			n := atomic.AddInt32(&mints, 1)
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": fmt.Sprintf("tok-%s-%d", r.Form.Get("audience"), n), "expires_in": 3600})
			return
		}
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Header.Get(scopedAudienceHeader) != "" {
			t.Errorf("internal audience header leaked to the API")
		}
		w.WriteHeader(apiStatus(r.Header.Get("Authorization")))
		_, _ = w.Write([]byte(`{"echo":"` + r.Header.Get("Authorization") + `"}`))
	}))
	return srv, &seen, &mints
}

func TestScopeTransportPinsAudience(t *testing.T) {
	srv, seen, _ := tokenAndAPI(t, func(string) int { return 200 })
	defer srv.Close()
	c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-pin", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	c.HTTPClient.Transport = &scopeTransport{base: http.DefaultTransport, c: c, origin: originOf(srv.URL), audience: "aud-sub"}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/rest/x", nil)
	req.Header.Set("Authorization", "Bearer service-provider-token")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(*seen) != 1 || !strings.HasPrefix((*seen)[0], "Bearer tok-aud-sub-") {
		t.Fatalf("API saw %v, want the audience token, never the Service Provider token", *seen)
	}
}

func TestScopeTransportRefreshesThenFailsClosed(t *testing.T) {
	srv, seen, mints := tokenAndAPI(t, func(string) int { return 401 })
	defer srv.Close()
	c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-401", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	c.HTTPClient.Transport = &scopeTransport{base: http.DefaultTransport, c: c, origin: originOf(srv.URL)}
	bearer, _, err := c.ScopedBearer(context.Background(), "aud-x")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/rest/x", nil)
	req.Header.Set("Authorization", bearer)
	req.Header.Set(scopedAudienceHeader, "aud-x")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 so the generated handler does not re-mint an unscoped token", resp.StatusCode)
	}
	if len(*seen) != 2 || (*seen)[0] == (*seen)[1] || atomic.LoadInt32(mints) != 2 {
		t.Fatalf("want one retry with a fresh scoped token; API saw %v, mints %d", *seen, *mints)
	}
}

func TestScrubScoped(t *testing.T) {
	srv, _, _ := tokenAndAPI(t, func(string) int { return 200 })
	defer srv.Close()
	c := &Client{HTTPClient: srv.Client(), Config: &config.Config{ClientID: "AID-scrub", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	bearer, _, err := c.ScopedBearer(context.Background(), "aud-scrub")
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimPrefix(bearer, "Bearer ")
	got := ScrubScoped(fmt.Errorf("HTTP 500: {\"echo\":%q}", raw))
	if strings.Contains(got.Error(), raw) || !strings.Contains(got.Error(), "****") {
		t.Fatalf("token not scrubbed: %v", got)
	}
}

func TestScopeTransportNeverSendsBearerOffOrigin(t *testing.T) {
	var leaked []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = append(leaked, r.Header.Get("Authorization"), r.Header.Get(scopedAudienceHeader))
	}))
	defer other.Close()
	srv, _, _ := tokenAndAPI(t, func(string) int { return 200 })
	defer srv.Close()
	c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-origin", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	c.HTTPClient.Transport = &scopeTransport{base: http.DefaultTransport, c: c, origin: originOf(srv.URL), audience: "aud-o"}
	bearer, _, err := c.ScopedBearer(context.Background(), "aud-o")
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, other.URL+"/elsewhere", nil)
	req.Header.Set("Authorization", bearer)
	req.Header.Set(scopedAudienceHeader, "aud-o")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(leaked) != 2 || leaked[0] != "" || leaked[1] != "" {
		t.Fatalf("off-origin request carried scoped credentials: %q", leaked)
	}
}

func TestScopeTransportScrubsErrorBodies(t *testing.T) {
	srv, _, _ := tokenAndAPI(t, func(string) int { return 500 })
	defer srv.Close()
	c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-echo", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	c.HTTPClient.Transport = &scopeTransport{base: http.DefaultTransport, c: c, origin: originOf(srv.URL), audience: "aud-echo"}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/rest/x", nil)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(body), "tok-aud-echo") || !strings.Contains(string(body), "****") {
		t.Fatalf("echoed scoped token not scrubbed: %s", body)
	}
}

func TestScopeTransportOffOriginBypassesClosed(t *testing.T) {
	var got []string
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.Path+"="+r.Header.Get("Authorization"))
	}))
	defer other.Close()
	srv, _, _ := tokenAndAPI(t, func(string) int { return 200 })
	defer srv.Close()
	c := &Client{HTTPClient: &http.Client{}, Config: &config.Config{ClientID: "AID-bypass", ClientSecret: "PW", TokenURL: srv.URL + "/oauth/token"}}
	c.HTTPClient.Transport = &scopeTransport{base: http.DefaultTransport, c: c, origin: originOf(srv.URL), audience: "aud-bp"}
	bearer, _, err := c.ScopedBearer(context.Background(), "aud-bp")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/x/oauth/token", "/plain"} { // token-looking path; scoped bearer without the marker header
		req, _ := http.NewRequest(http.MethodGet, other.URL+path, nil)
		req.Header.Set("Authorization", bearer)
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if len(got) != 2 || got[0] != "/x/oauth/token=" || got[1] != "/plain=" {
		t.Fatalf("off-origin requests carried a scoped bearer: %q", got)
	}
}
