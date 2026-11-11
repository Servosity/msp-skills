// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Nerdio fakes for the internal/cli tests. Every MSP runs its own NMM
// instance (NERDIO_BASE_URL, the API under /rest-api/v1), and the token comes
// from the MSP's Entra ID tenant on another host (NERDIO_TOKEN_URL,
// <tenant host>/<TENANT_ID>/oauth2/v2.0/token: POST, a form body
// grant_type=client_credentials&scope=<NERDIO_OAUTH_SCOPE> with the client
// pair as client_id and client_secret, answered {"access_token","expires_in"}).

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// nerdioTestScope is the NMM application's scope the tests configure: the
// bare '<app-id>/.default' (no api:// prefix).
const nerdioTestScope = "6f1e2d3c-4b5a-4968-8776-5a4b3c2d1e0f/.default"

// nerdioTestTokenPath is the Entra token path the fakes answer.
const nerdioTestTokenPath = "/tenant-7c1e9a3f/oauth2/v2.0/token"

// NERDIO_OAUTH_SCOPE is required (nerdio handfix oauth-scope-required): with
// it unset nothing is posted, so every fleet test of auth login's mint
// (auth_dry_run_test.go, auth_token_redirect_test.go) would pass or fail
// without reaching its token endpoint. The test process configures it the way
// an operator does; a test of the unset case clears it with t.Setenv.
func init() {
	_ = os.Setenv("NERDIO_OAUTH_SCOPE", nerdioTestScope)
}

// fakeInstance is an NMM instance (srv, answered by api) and its operator's
// Entra tenant (entra: the token POST at tokenURL, answered with access for
// the one pair and scope it knows; anything else is refused and counted).
type fakeInstance struct {
	srv, entra   *httptest.Server
	tokenURL     string
	id, secret   string
	access       string
	mints, bad   atomic.Int32
	other        atomic.Int32 // token host requests that were not the token POST path
	lastMintForm atomic.Value
}

func newFakeInstance(t *testing.T, id, secret, access string, api http.HandlerFunc) *fakeInstance {
	t.Helper()
	m := &fakeInstance{id: id, secret: secret, access: access}
	m.srv = httptest.NewServer(api)
	t.Cleanup(m.srv.Close)
	m.entra = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != nerdioTestTokenPath {
			m.other.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		m.lastMintForm.Store(r.Header.Get("Content-Type") + " " + string(raw))
		form, _ := url.ParseQuery(string(raw))
		gotID, gotSecret := form.Get("client_id"), form.Get("client_secret")
		if r.Method != http.MethodPost || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") ||
			r.Header.Get("Authorization") != "" || !strings.HasPrefix(string(raw), "grant_type=") || gotID != id || gotSecret != secret || form.Get("grant_type") != "client_credentials" ||
			form.Get("scope") != nerdioTestScope {
			m.bad.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		m.mints.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"`+access+`","expires_in":3600,"token_type":"Bearer"}`)
	}))
	t.Cleanup(m.entra.Close)
	m.tokenURL = m.entra.URL + nerdioTestTokenPath
	return m
}

// useAs points the process at this instance with its client pair in the
// environment.
func (m *fakeInstance) useAs(t *testing.T) {
	t.Helper()
	t.Setenv("NERDIO_BASE_URL", m.srv.URL)
	t.Setenv("NERDIO_TOKEN_URL", m.tokenURL)
	t.Setenv("NERDIO_OAUTH_SCOPE", nerdioTestScope)
	t.Setenv("NERDIO_CLIENT_ID", m.id)
	t.Setenv("NERDIO_CLIENT_SECRET", m.secret)
}

// networkFence replaces http.DefaultTransport, which every client this
// program builds clones (the API client, auth login's token client), for one
// test: it dials loopback only (the fakes) and records every dial, so a
// regression that aims a request at a real host (the shipped placeholder NMM
// instance, the real Entra ID with the placeholder tenant) is refused before
// any connection or name lookup. The tests below that could reach a real host
// if the rule under test broke use it; seen records loopback dials too, so a
// test can prove its clients really dial through the fence.
type networkFence struct {
	mu      sync.Mutex
	dialed  []string
	escaped []string
}

func fenceNetwork(t *testing.T) *networkFence {
	t.Helper()
	orig, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport is %T, not *http.Transport: the fence cannot be installed", http.DefaultTransport)
	}
	f := &networkFence{}
	tr := orig.Clone()
	tr.Proxy = nil
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		f.mu.Lock()
		f.dialed = append(f.dialed, addr)
		host, _, err := net.SplitHostPort(addr)
		ip := net.ParseIP(host)
		loopback := err == nil && ip != nil && ip.IsLoopback()
		if !loopback {
			f.escaped = append(f.escaped, addr)
		}
		f.mu.Unlock()
		if !loopback {
			return nil, fmt.Errorf("network fence: %s is not a loopback fake", addr)
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = orig })
	return f
}

// result reports every dial and the ones that were not loopback.
func (f *networkFence) result() (dialed, escaped []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.dialed...), append([]string(nil), f.escaped...)
}
