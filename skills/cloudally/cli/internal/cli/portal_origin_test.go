// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloudally-pp-cli/internal/client"
	"cloudally-pp-cli/internal/config"
	"cloudally-pp-cli/internal/portal"
)

func TestResolvePortalURL(t *testing.T) {
	const stub = "http://127.0.0.1:18080"
	cases := []struct {
		name    string
		flag    string
		env     string
		api     string
		want    string
		refused bool
	}{
		{name: "production api keeps production portal", flag: portal.DefaultBaseURL, api: "https://api.cloudally.com", want: portal.DefaultBaseURL},
		{name: "production api spelled differently", flag: portal.DefaultBaseURL, api: "https://API.cloudally.com:443/v2/", want: portal.DefaultBaseURL},
		{name: "unset base url is production", flag: "", api: "", want: portal.DefaultBaseURL},
		{name: "explicit flag wins", flag: "https://portal.example.test", env: "https://env.example.test", api: stub, want: "https://portal.example.test"},
		{name: "env names the portal", flag: portal.DefaultBaseURL, env: "https://env.example.test", api: stub, want: "https://env.example.test"},
		{name: "env names production on purpose", flag: portal.DefaultBaseURL, env: portal.DefaultBaseURL, api: stub, want: portal.DefaultBaseURL},
		{name: "stub api refused", flag: portal.DefaultBaseURL, api: stub, refused: true},
		{name: "default flag with trailing slash still counts as unset", flag: portal.DefaultBaseURL + "/", api: stub, refused: true},
		{name: "plain http production host refused", flag: portal.DefaultBaseURL, api: "http://api.cloudally.com", refused: true},
		{name: "lookalike host refused", flag: portal.DefaultBaseURL, api: "https://api.cloudally.com.example.test", refused: true},
		{name: "other port refused", flag: portal.DefaultBaseURL, api: "https://api.cloudally.com:8443", refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(cloudallyPortalURLEnv, tc.env)
			got, err := resolvePortalURL(tc.flag, tc.api)
			if tc.refused {
				if err == nil {
					t.Fatalf("want a refusal, got %q", got)
				}
				if cliErrCode(t, err) != 2 || !strings.Contains(err.Error(), cloudallyPortalURLEnv) {
					t.Fatalf("refusal should be a usage error naming %s: %v", cloudallyPortalURLEnv, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("resolvePortalURL(%q, %q) = %q, %v; want %q", tc.flag, tc.api, got, err, tc.want)
			}
		})
	}
}

// With CLOUDALLY_BASE_URL on a stub, the handoff token must not be sent to
// the production portal. The refusal comes before the token is minted, and
// CLOUDALLY_PORTAL_URL routes the session to the matching portal.
func TestPortalHandoffStaysWithANonDefaultAPI(t *testing.T) {
	isolateAuthState(t)
	t.Setenv(cloudallyPortalURLEnv, "")

	var mints atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/partner-access-token") {
			mints.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"single-use-handoff"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()

	var opened []string
	errStop := errors.New("recorder: no network")
	orig := openPortal
	openPortal = func(_ context.Context, cfg portal.Config, _ string) (*portal.Session, error) {
		opened = append(opened, cfg.BaseURL)
		return nil, errStop
	}
	t.Cleanup(func() { openPortal = orig })

	c := client.New(&config.Config{BaseURL: api.URL, AccessToken: "partner-token", ClientID: "cid", ClientSecret: "csecret"}, 5*time.Second, 0)
	scope := restoreScope{account: "5f1c2a"}

	_, err := openPortalSession(context.Background(), c, scope, portal.DefaultBaseURL)
	if err == nil || errors.Is(err, errStop) {
		t.Fatalf("want a refusal before any portal contact, got %v (opened %q)", err, opened)
	}
	if len(opened) != 0 {
		t.Fatalf("a session was opened at %q for a stub API", opened)
	}
	if n := mints.Load(); n != 0 {
		t.Fatalf("a handoff token was minted (%d) although the run was refused", n)
	}

	const stubPortal = "http://127.0.0.1:18081"
	t.Setenv(cloudallyPortalURLEnv, stubPortal)
	if _, err := openPortalSession(context.Background(), c, scope, portal.DefaultBaseURL); !errors.Is(err, errStop) {
		t.Fatalf("want the recorder to be reached, got %v", err)
	}
	if len(opened) != 1 || opened[0] != stubPortal {
		t.Fatalf("session opened at %q, want %s", opened, stubPortal)
	}
	if n := mints.Load(); n != 1 {
		t.Fatalf("mints = %d, want 1", n)
	}
}

// Round two, item 4: the handoff token travels in the portal URL, so a
// portal override must be https unless it is a loopback test host, and the
// check runs before any token is minted.
func TestPortalURLMustBeHTTPS(t *testing.T) {
	const api = "https://api.cloudally.com"
	cases := []struct {
		flag, env string
		ok        bool
	}{
		{env: "http://10.255.255.1", ok: false},
		{env: "http://portal.example.test", ok: false},
		{flag: "http://10.0.0.5:8080", ok: false},
		{env: "ftp://portal.example.test", ok: false},
		{env: "portal.example.test", ok: false},
		{env: "https://portal.example.test", ok: true},
		{env: "http://127.0.0.1:18080", ok: true},
		{env: "http://localhost:18080", ok: true},
		{flag: "http://[::1]:18080", ok: true},
	}
	for _, tc := range cases {
		t.Run(tc.flag+tc.env, func(t *testing.T) {
			t.Setenv(cloudallyPortalURLEnv, tc.env)
			flag := tc.flag
			if flag == "" {
				flag = portal.DefaultBaseURL
			}
			got, err := resolvePortalURL(flag, api)
			if tc.ok {
				if err != nil {
					t.Fatalf("want accepted, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want refused, got %q", got)
			}
			if cliErrCode(t, err) != 2 {
				t.Fatalf("want a usage error, got %v", err)
			}
		})
	}
}

func TestPlainHTTPPortalIsRefusedBeforeMinting(t *testing.T) {
	isolateAuthState(t)
	var mints atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/partner-access-token") {
			mints.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"accessToken":"single-use-handoff"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer api.Close()
	var opened []string
	orig := openPortal
	openPortal = func(_ context.Context, cfg portal.Config, _ string) (*portal.Session, error) {
		opened = append(opened, cfg.BaseURL)
		return nil, errors.New("recorder: no network")
	}
	t.Cleanup(func() { openPortal = orig })

	t.Setenv(cloudallyPortalURLEnv, "http://10.255.255.1")
	c := client.New(&config.Config{BaseURL: api.URL, AccessToken: "partner-token", ClientID: "cid", ClientSecret: "csecret"}, 5*time.Second, 0)
	if _, err := openPortalSession(context.Background(), c, restoreScope{account: "5f1c2a"}, portal.DefaultBaseURL); err == nil {
		t.Fatal("a plain-http portal was accepted")
	}
	if n := mints.Load(); n != 0 || len(opened) != 0 {
		t.Fatalf("handoff tokens minted = %d, sessions opened at %q; want none", n, opened)
	}
}
