// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/credkit"
)

func TestRefreshAccessTokenRefusesPlaintextRemoteTokenURL(t *testing.T) {
	_, _, _, err := refreshAccessToken(context.Background(), "http://tokens.example.com/oauth2/v1/tokens/bearer", "id", "client-secret-value", "refresh-token-value")
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("plaintext token URL accepted: %v", err)
	}
	if strings.Contains(err.Error(), "client-secret-value") || strings.Contains(err.Error(), "refresh-token-value") {
		t.Fatalf("refusal echoes a credential: %v", err)
	}
}

func TestRefreshAccessTokenDoesNotFollowRedirects(t *testing.T) {
	var leaked int64
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "refresh-token-value") || r.Header.Get("Authorization") != "" {
			atomic.AddInt64(&leaked, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"stolen","expires_in":3600}`)
	}))
	defer elsewhere.Close()
	token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer token.Close()
	_, _, _, err := refreshAccessToken(context.Background(), token.URL, "id", "client-secret-value", "refresh-token-value")
	if err == nil {
		t.Fatal("a redirected token response was accepted")
	}
	if atomic.LoadInt64(&leaked) != 0 {
		t.Fatal("the refresh token followed a redirect to another server")
	}
}

func TestAuthRefreshIgnoresUnresolvedBundlePlaceholders(t *testing.T) {
	t.Setenv("QUICKBOOKS_CLIENT_ID", "${user_config.quickbooks_client_id}")
	t.Setenv("QUICKBOOKS_CLIENT_SECRET", "${user_config.quickbooks_client_secret}")
	t.Setenv("QUICKBOOKS_REFRESH_TOKEN", "${user_config.quickbooks_refresh_token}")
	t.Setenv("QUICKBOOKS_CONFIG", t.TempDir()+"/config.toml")
	root := newRootCmd(&rootFlags{})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"auth", "refresh"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "auth refresh needs") {
		t.Fatalf("unresolved placeholders were used as credentials: %v", err)
	}
}

// TestRefreshTokenErrorShowsOnlyOAuthErrorCode pins the class "the token
// endpoint's error body reaches output through a detector narrower than the
// encodings a server can echo": whatever the endpoint wrote, the error shows
// only an RFC 6749 section 5.2 error code, so an echo of the client secret or
// refresh token in any encoding (base32 is one a plain search does not
// decode) never reaches the terminal or an MCP result.
func TestRefreshTokenErrorShowsOnlyOAuthErrorCode(t *testing.T) {
	const secret = "test-client-secret-000000000000000000000" // 40 chars, Intuit client-secret length
	const refresh = "AB11798912345678901234567890abcdefghijklmnopqrstuvwxyz0123456789"
	echoes := []string{
		base32.StdEncoding.EncodeToString([]byte(secret)),
		base32.HexEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(refresh)),
		reverseString(secret),
		strings.ToUpper(hex.EncodeToString([]byte(secret))),
	}
	for _, echo := range echoes {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"bad credential `+echo+`"}`)
		}))
		_, _, _, err := refreshAccessToken(context.Background(), srv.URL, "client-id-value", secret, refresh)
		srv.Close()
		if err == nil {
			t.Fatal("a 400 token response was accepted")
		}
		if strings.Contains(err.Error(), echo) {
			t.Errorf("token error repeats the server's echo %q: %v", echo, err)
		}
		if !strings.Contains(err.Error(), "invalid_client") {
			t.Errorf("token error lost the OAuth error code: %v", err)
		}
	}
	// A body that is not an OAuth error object shows nothing of itself.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "denied for "+base32.StdEncoding.EncodeToString([]byte(secret)))
	}))
	defer srv.Close()
	_, _, _, err := refreshAccessToken(context.Background(), srv.URL, "client-id-value", secret, refresh)
	if err == nil || strings.Contains(err.Error(), "denied") || !strings.Contains(err.Error(), "401") {
		t.Fatalf("non-OAuth token error body must be withheld whole, status kept: %v", err)
	}
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

// A token answer that fails to decode part way (a well-formed access_token,
// then an expires_in of the wrong type) still holds the token it decoded.
func TestRefreshAccessTokenHoldsATokenFromAnAnswerThatFailsToDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"part-decoded-Tk9","refresh_token":"part-decoded-Rf9","expires_in":"soon"}`)
	}))
	defer srv.Close()
	got := recordHolds(t)
	if _, _, _, err := refreshAccessToken(context.Background(), srv.URL, "id-part-decoded", "cs-part-decoded", "rt-part-decoded"); err == nil {
		t.Fatal("an answer with a string expires_in decoded")
	}
	held := got()
	for _, v := range []string{"part-decoded-Tk9", "part-decoded-Rf9"} {
		if !strings.Contains(held, v) {
			t.Errorf("%s was decoded and not held", v)
		}
	}
}

// Codex next-batch review 2, round 2: a duplicate access_token overwrote the
// first, which was never held, and the overflowing expires_in's decode error
// quoted that first token's digits on stderr. The class: a credential answer
// holds every value as it is read, and its decode error quotes nothing.
func TestRefreshAccessTokenHoldsAnOverwrittenTokenAndQuotesNothing(t *testing.T) {
	const first = "123456789012345678901"
	for answer, held := range map[string][]string{
		`{"access_token":"` + first + `","access_token":"replacement-token","expires_in":` + first + `}`:  {first},
		`{"access_token":` + first + `,"refresh_token":"rt-overwritten-Qz7"}`:                             {first, "rt-overwritten-Qz7"},
		`{"refresh_token":"rt-syntax-Wq3","refresh_token":"rt-later-Ln6","access_token":"at-Ax4"` + first: {"rt-syntax-Wq3"},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, answer)
		}))
		got := recordHolds(t)
		_, _, _, err := refreshAccessToken(context.Background(), srv.URL, "id-overwritten", "cs-overwritten", "rt-overwritten")
		srv.Close()
		if err == nil {
			t.Fatalf("answer %s decoded", answer)
		}
		if strings.Contains(err.Error(), first) || strings.Contains(err.Error(), "Qz7") || strings.Contains(err.Error(), "Wq3") {
			t.Errorf("the decode error quotes the answer: %v", err)
		}
		for _, v := range held {
			if !strings.Contains(got(), v) {
				t.Errorf("%s was read from %s and not held", v, answer)
			}
		}
	}
}

// recordHolds replaces the no-op hold seam with a recorder for one test.
func recordHolds(t *testing.T) func() string {
	t.Helper()
	old := cliutil.HoldCredential
	var held []string
	cliutil.HoldCredential = func(_ credkit.Source, values ...string) { held = append(held, values...) }
	t.Cleanup(func() { cliutil.HoldCredential = old })
	return func() string { return strings.Join(held, "\n") }
}
