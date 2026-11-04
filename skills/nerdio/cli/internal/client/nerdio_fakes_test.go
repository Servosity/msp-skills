// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

// Nerdio fakes for the client tests. The NMM Partner API and its token
// endpoint live on two hosts: every MSP runs its own NMM instance
// (NERDIO_BASE_URL, API paths under /rest-api/v1), and the access token comes
// from the MSP's Entra ID tenant (NERDIO_TOKEN_URL,
// https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token): POST, a
// form body grant_type=client_credentials&scope=<NERDIO_OAUTH_SCOPE>, the
// client authenticated with HTTP Basic, answered
// {"access_token":"...","expires_in":3600}.

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// nerdioTestScope is the NMM application's scope the tests configure: the
// bare '<app-id>/.default' (no api:// prefix).
const nerdioTestScope = "6f1e2d3c-4b5a-4968-8776-5a4b3c2d1e0f/.default"

// NERDIO_OAUTH_SCOPE is required (nerdio handfix oauth-scope-required): with
// it unset nothing is posted, so every generated and fleet test of the mint
// (token_race_test.go, dry_run_mint_test.go, token_redirect_test.go) would
// pass or fail without ever reaching its token endpoint. The test process
// configures it the way an operator does; a test of the unset case clears it
// with t.Setenv.
func init() {
	_ = os.Setenv("NERDIO_OAUTH_SCOPE", nerdioTestScope)
}

// entraFake is an Entra ID tenant's token endpoint. It mints a token per call
// for a client whose secret is "secret-for-<client id>", named
// minted-<tag><client id>-<n>, and records every request it received.
type entraFake struct {
	srv      *httptest.Server
	tokenURL string
	tag      string
	mu       sync.Mutex
	mints    map[string]int // client ID -> accepted mints
	requests int            // every request, accepted or not
	scopes   []string       // the scope of each token request
	refused  []string       // why each refused request was refused
}

func newEntraFake(t *testing.T, tag string) *entraFake {
	t.Helper()
	f := &entraFake{tag: tag, mints: map[string]int{}}
	const path = "/tenant-7c1e9a3f/oauth2/v2.0/token"
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		form, ferr := url.ParseQuery(string(raw))
		id, secret, basic := r.BasicAuth()
		why := ""
		switch {
		case r.Method != http.MethodPost || r.URL.Path != path:
			why = r.Method + " " + r.URL.Path
		case !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") || ferr != nil:
			why = "not a form body"
		case !basic:
			why = "no HTTP Basic client authentication"
		case form.Has("client_id") || form.Has("client_secret"):
			why = "the client pair in the body"
		case form.Get("grant_type") != "client_credentials":
			why = "grant_type " + form.Get("grant_type")
		case secret != "secret-for-"+id:
			why = "wrong client secret"
		}
		f.mu.Lock()
		f.requests++
		f.scopes = append(f.scopes, form.Get("scope"))
		if why != "" {
			f.refused = append(f.refused, why)
		} else {
			f.mints[id]++
		}
		n := f.mints[id]
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if why != "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
			return
		}
		fmt.Fprintf(w, `{"access_token":"minted-%s%s-%d","expires_in":3600,"token_type":"Bearer"}`, tag, id, n)
	}))
	t.Cleanup(f.srv.Close)
	f.tokenURL = f.srv.URL + path
	return f
}

func (f *entraFake) state() (mints map[string]int, requests int, scopes, refused []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mints = map[string]int{}
	for k, v := range f.mints {
		mints[k] = v
	}
	return mints, f.requests, append([]string(nil), f.scopes...), append([]string(nil), f.refused...)
}

// nmmFake is an NMM instance: every request records the Authorization it
// carried and is answered with an empty account list.
type nmmFake struct {
	srv   *httptest.Server
	mu    sync.Mutex
	auths []string
}

func newNMMFake(t *testing.T) *nmmFake {
	t.Helper()
	m := &nmmFake{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.auths = append(m.auths, r.Header.Get("Authorization"))
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *nmmFake) sent() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.auths...)
}
