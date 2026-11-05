// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"appdirect-pp-cli/internal/cliutil"
	"appdirect-pp-cli/internal/config"
)

// bindingMarketplace is an AppDirect marketplace for the token-binding tests:
// POST /oauth2/token mints a fresh token per call for the client named in the
// HTTP Basic credentials (form body grant_type=client_credentials&scope=...),
// and every other request records the Authorization it carried.
type bindingMarketplace struct {
	srv   *httptest.Server
	mu    sync.Mutex
	mints map[string]int // client ID -> mints
	auths []string       // Authorization of each API request
}

func newBindingMarketplace(t *testing.T, tag string) *bindingMarketplace {
	t.Helper()
	m := &bindingMarketplace{mints: map[string]int{}}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/oauth2/token" {
			id, secret, ok := r.BasicAuth()
			raw, _ := io.ReadAll(r.Body)
			form, _ := url.ParseQuery(string(raw))
			if r.Method != http.MethodPost || !ok || secret != "secret-for-"+id || form.Get("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"error":"invalid_client"}`)
				return
			}
			m.mu.Lock()
			m.mints[id]++
			n := m.mints[id]
			m.mu.Unlock()
			fmt.Fprintf(w, `{"access_token":"minted-%s%s-%d","expires_in":3600}`, tag, id, n)
			return
		}
		m.mu.Lock()
		m.auths = append(m.auths, r.Header.Get("Authorization"))
		m.mu.Unlock()
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func (m *bindingMarketplace) state() (map[string]int, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	mints := map[string]int{}
	for k, v := range m.mints {
		mints[k] = v
	}
	return mints, append([]string(nil), m.auths...)
}

// minted-token-bound-to-its-client: the files hold client pair A and the token
// minted for A (what `auth login` with A's pair writes); the environment
// supplies pair B. The API request carries a token freshly minted for B, never
// A's; the credential store still holds A's token, and B's token is written
// to no file; a second request in the same process (a second client, as the
// MCP server builds one per call) reuses B's token without minting again.
func TestEnvironmentPairMintsItsOwnTokenAndKeepsTheStoredOne(t *testing.T) {
	home := isolateCredentialStore(t)
	m := newBindingMarketplace(t, "")
	t.Setenv("APPDIRECT_BASE_URL", m.srv.URL+"/api")
	const idA, idB = "bind-client-a-5e1c", "bind-client-b-8d4f"
	const tokenA = "stored-token-for-client-a-2b9e"
	files, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := files.SaveTokens(idA, "secret-for-"+idA, tokenA, "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	credPath, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(credPath); err != nil || !strings.Contains(string(b), tokenA) || !strings.Contains(string(b), idA) {
		t.Fatalf("the files do not hold pair A and token A; the test did not run (err %v):\n%s", err, b)
	}
	// Control: with no environment pair, A's stored token is A's and is sent
	// as stored, with no mint.
	send := func() {
		t.Helper()
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/billing/v1/subscriptions", nil); err != nil {
			t.Fatal(err)
		}
	}
	send()
	if mints, auths := m.state(); len(mints) != 0 || len(auths) != 1 || auths[0] != "Bearer "+tokenA {
		t.Fatalf("control: A's own stored token must be sent without a mint; mints %v, sent %v", mints, auths)
	}
	t.Setenv("APPDIRECT_CLIENT_ID", idB)
	t.Setenv("APPDIRECT_CLIENT_SECRET", "secret-for-"+idB)
	send()
	send()
	mints, auths := m.state()
	if mints[idB] != 1 || mints[idA] != 0 {
		t.Fatalf("want exactly one mint, for B, across two requests in one process; got %v", mints)
	}
	for _, a := range auths[1:] {
		if a != "Bearer minted-"+idB+"-1" {
			t.Errorf("a request for client B carried %q, want B's minted token (A's stored token authenticates as A)", a)
		}
	}
	b, err := os.ReadFile(credPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), tokenA) || !strings.Contains(string(b), idA) {
		t.Errorf("the credential store lost pair A or token A:\n%s", b)
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, v := range []string{"minted-" + idB, "secret-for-" + idB, idB} {
			if strings.Contains(string(data), v) {
				t.Errorf("%s holds %q: an environment pair's credential or its token was persisted", path, v)
			}
		}
		return nil
	})
}

// A token minted for an environment pair is bound to that pair and the token
// URL it was minted at: the same pair against another marketplace (another
// APPDIRECT_BASE_URL, so another token URL) mints its own token.
func TestProcessTokenIsBoundToItsTokenURL(t *testing.T) {
	isolateCredentialStore(t)
	one, two := newBindingMarketplace(t, "one-"), newBindingMarketplace(t, "two-")
	const id = "bind-client-c-3a7d"
	t.Setenv("APPDIRECT_CLIENT_ID", id)
	t.Setenv("APPDIRECT_CLIENT_SECRET", "secret-for-"+id)
	t.Setenv("APPDIRECT_NO_CONFIG_WRITE", "1")
	for _, m := range []*bindingMarketplace{one, two, one} {
		t.Setenv("APPDIRECT_BASE_URL", m.srv.URL+"/api")
		cfg, err := config.Load("")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(cfg, 5*time.Second, 0).Get(context.Background(), "/billing/v1/subscriptions", nil); err != nil {
			t.Fatal(err)
		}
	}
	for tag, m := range map[string]*bindingMarketplace{"one-": one, "two-": two} {
		mints, auths := m.state()
		if mints[id] != 1 {
			t.Errorf("marketplace %s: want one mint for its own token, got %v", tag, mints)
		}
		for _, a := range auths {
			if a != "Bearer minted-"+tag+id+"-1" {
				t.Errorf("marketplace %s received %q, want the one token minted for it in this process", tag, a)
			}
		}
	}
}
