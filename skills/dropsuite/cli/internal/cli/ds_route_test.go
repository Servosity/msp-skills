// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"dropsuite-pp-cli/internal/cliutil"
	"dropsuite-pp-cli/internal/cliutil/testenv"
)

// Codex review round 1 class read-call-reaches-a-mint-route: a request that
// is not issued as a mutation never reaches a route that mints a portal
// sign-in link, whatever spelling its path takes (an id argument that names
// the route, a format suffix, another case, dot segments, an encoded slash or
// percent sign, a redirect); a mutation call (GetMutating) still does, and
// ordinary reads are untouched.
func TestReadsNeverReachASignInLinkRoute(t *testing.T) {
	var mu sync.Mutex
	var reached []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(strings.ToLower(r.URL.Path+" "+r.URL.RawPath), "generate_") {
			mu.Lock()
			reached = append(reached, r.Method+" "+r.URL.RequestURI())
			mu.Unlock()
			_, _ = w.Write([]byte(`{"redirect_url":"https://portal.example.com/sso/x"}`))
			return
		}
		switch r.URL.Path {
		case "/api/users":
			_, _ = w.Write([]byte(`[{"id":"26561","email":"owner@example.com","authentication_token":"` + pinUnitOrgToken + `"}]`))
		case "/api/users/26561":
			_, _ = w.Write([]byte(`{"id":"26561"}`))
		case "/api/users/26561/moved":
			http.Redirect(w, r, "/api/users/generate_redirect_link", http.StatusFound)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("DROPSUITE_BASE_URL", srv.URL+"/api")
	t.Setenv("DROPSUITE_ACCESS_TOKEN", pinUnitAccess)
	t.Setenv("DROPSUITE_RESELLER_TOKEN", pinUnitReseller)
	t.Setenv(defaultOrgEnv, "26561")
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var paths []string
	for _, id := range []string{"generate_redirect_link", "generate_support_sso_link", "generate_redirect_link.json",
		"GENERATE_REDIRECT_LINK", "Generate_Support_SSO_Link", "generate_redirect_link;x", "..", ".",
		"x/../generate_redirect_link", "%67enerate_redirect_link", `x\..\generate_redirect_link`} {
		esc := cliutil.EscapePathParam(id)
		paths = append(paths, "/users/"+esc, "/users/"+esc+"/tenants", "/sharepoints/domains/"+esc+"/sites")
	}
	paths = append(paths, "/users/generate_redirect_link", "/users//generate_redirect_link", "/users/./generate_redirect_link",
		"/users/generate_redirect_link/", "/users/26561/../generate_redirect_link", "/users/26561/moved")
	for _, p := range paths {
		if _, err := c.Get(ctx, p, nil); err == nil {
			t.Errorf("GET %s: want a refusal, got none", p)
		}
	}
	mu.Lock()
	if len(reached) != 0 {
		t.Fatalf("reads reached a sign-in link route: %v", reached)
	}
	mu.Unlock()
	// Ordinary reads still go out.
	for _, p := range []string{"/users", "/users/26561", "/users/26561/tenants", "/accounts", "/sharepoints/domains/d1/sites", "/status"} {
		if _, err := c.Get(ctx, p, map[string]string{"page": "1"}); err != nil {
			t.Errorf("GET %s: %v", p, err)
		}
	}
	// A mutation call still mints, once per call.
	for _, p := range []string{"/users/generate_redirect_link", "/users/generate_support_sso_link"} {
		if _, err := c.GetMutating(ctx, p, nil); err != nil {
			t.Fatalf("GetMutating %s: %v", p, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reached) != 2 {
		t.Fatalf("the two mutation calls reached %d time(s): %v", len(reached), reached)
	}
}

// A request the transport refuses (route guard, origin pin) is refused again
// on any replay, so the client returns at once instead of retrying it with
// backoff, and the error still names the refusal.
func TestRefusedRequestIsNotRetried(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://elsewhere.example.com/api/users", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("DROPSUITE_BASE_URL", srv.URL+"/api")
	t.Setenv("DROPSUITE_ACCESS_TOKEN", pinUnitAccess)
	t.Setenv("DROPSUITE_RESELLER_TOKEN", pinUnitReseller)
	flags := rootFlags{}
	c, err := flags.newClient()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/users/generate_redirect_link", "/users/%2E%2E", "/status"} {
		start := time.Now()
		_, err := c.Get(context.Background(), p, nil)
		if took := time.Since(start); took > 900*time.Millisecond {
			t.Errorf("GET %s: refused after %v; a refusal must not be retried", p, took)
		}
		if !errors.Is(err, cliutil.ErrRequestRefused) {
			t.Errorf("GET %s: want the refusal, got %v", p, err)
		}
	}
}
