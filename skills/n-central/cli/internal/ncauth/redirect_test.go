// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package ncauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Hand-owned (handfixes.json: redirect-strips-every-credential). The handshake
// POSTs the User-API JWT in its Authorization header (and the refresh token in
// its body); a 307 to another host:port used to replay both there.

func TestHandshakeRefusesARedirectToAnotherOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	_, err := postBearer(context.Background(), first.URL+"/api/auth/refresh", "jwt-credential", map[string]string{"refreshToken": "refresh-credential"})
	if err == nil || !strings.Contains(err.Error(), "another origin") {
		t.Fatalf("postBearer error = %v, want the cross-origin redirect refused", err)
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d request(s) carrying the JWT and refresh token", n)
	}
}

func TestHandshakeFollowsASameOriginRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/auth/authenticate" {
			http.Redirect(w, r, srv.URL+"/api/v2/auth/authenticate", http.StatusTemporaryRedirect)
			return
		}
		if r.Header.Get("Authorization") != "Bearer jwt-credential" {
			http.Error(w, "missing JWT after a same-origin redirect", http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"tokens":{"access":{"token":"access-token","type":"Bearer","expirySeconds":3600}}}`)
	}))
	defer srv.Close()
	resp, err := postBearer(context.Background(), srv.URL+"/api/auth/authenticate", "jwt-credential", nil)
	if err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	if resp.Tokens.Access.Token != "access-token" {
		t.Fatalf("access token = %q", resp.Tokens.Access.Token)
	}
}

func TestHandshakeRedirectOriginComparesLikeAURL(t *testing.T) {
	check := httpClient().CheckRedirect
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"https://NCENTRAL.example.test/api/auth/authenticate", "https://ncentral.example.test/api/v2/auth/authenticate", true},
		{"https://ncentral.example.test/api/auth/authenticate", "https://ncentral.example.test:443/api/auth/authenticate", true},
		{"https://ncentral.example.test/api/auth/authenticate", "https://ncentral.example.test:8443/api/auth/authenticate", false},
		{"https://ncentral.example.test/api/auth/authenticate", "http://ncentral.example.test/api/auth/authenticate", false},
		{"https://ncentral.example.test/api/auth/authenticate", "https://elsewhere.example.test/api/auth/authenticate", false},
	} {
		first, _ := http.NewRequest(http.MethodPost, tc.from, nil)
		next, _ := http.NewRequest(http.MethodPost, tc.to, nil)
		err := check(next, []*http.Request{first})
		if (err == nil) != tc.allowed {
			t.Errorf("%s -> %s: err=%v, want allowed=%v", tc.from, tc.to, err, tc.allowed)
		}
	}
}
