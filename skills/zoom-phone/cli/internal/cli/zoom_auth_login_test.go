// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthLoginMintUsesZoomAccountCredentialsGrant(t *testing.T) {
	var grant, account string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		grant, account = r.PostForm.Get("grant_type"), r.PostForm.Get("account_id")
		_, _ = w.Write([]byte(`{"access_token":"tok","expires_in":3599}`))
	}))
	defer srv.Close()
	tok, err := mintClientCredentialsToken(srv.Client(), srv.URL, "cid", "csecret", "acct-123")
	if err != nil {
		t.Fatal(err)
	}
	if grant != "account_credentials" || account != "acct-123" || tok.AccessToken != "tok" {
		t.Fatalf("grant=%q account=%q token=%q", grant, account, tok.AccessToken)
	}
}

// Codex review r1 (class mint-error-disclosure). A token endpoint that echoes
// the request (a proxy, a misconfigured or hostile endpoint) can return the
// client secret in any encoding; masking known encodings is a denylist. The
// error shows only the HTTP status and a recognized OAuth error code.
func TestAuthLoginMintErrorWithholdsEndpointText(t *testing.T) {
	secret := "s3cret-value-123"
	echoes := zoomTestSecretEncodings("cid", secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"reason":"echo ` + strings.Join(echoes, " ") + `","error":"invalid_client"}`))
	}))
	defer srv.Close()
	_, err := mintClientCredentialsToken(srv.Client(), srv.URL, "cid", secret, "acct-123")
	if err == nil {
		t.Fatal("HTTP 400 from the token endpoint accepted")
	}
	for _, e := range echoes {
		if strings.Contains(err.Error(), e) {
			t.Fatalf("mint error repeats endpoint text %q: %v", e, err)
		}
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("mint error lost the status or the OAuth error code: %v", err)
	}
	// An unrecognized error code is not repeated either.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"` + echoes[1] + `"}`))
	}))
	defer srv2.Close()
	if _, err := mintClientCredentialsToken(srv2.Client(), srv2.URL, "cid", secret, "acct-123"); err == nil || strings.Contains(err.Error(), echoes[1]) {
		t.Fatalf("unrecognized error code repeated: %v", err)
	}
}

// zoomTestSecretEncodings returns the secret in encodings a reflecting
// endpoint could use.
func zoomTestSecretEncodings(id, secret string) []string {
	return []string{
		secret,
		base64.StdEncoding.EncodeToString([]byte(secret)),
		base64.RawURLEncoding.EncodeToString([]byte(secret)),
		hex.EncodeToString([]byte(secret)),
		base64.StdEncoding.EncodeToString([]byte(id + ":" + secret)),
		strings.ToUpper(secret),
		reverseString(secret),
	}
}

func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
