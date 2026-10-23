// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package coverpc

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Hand-owned (handfixes.json: redirect-strips-every-credential). Every
// JSON-RPC call is a POST whose body carries the password (Login) or the
// session visa; a 307 to another host:port used to replay that body there.

func TestJSONRPCRefusesARedirectToAnotherOrigin(t *testing.T) {
	var elsewhere atomic.Int32
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		elsewhere.Add(1)
	}))
	defer second.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, second.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer first.Close()
	c := New(first.URL)
	c.SessionPath = ""
	c.Creds = Credentials{Partner: "partner", Username: "user", Password: "password-credential"}
	if _, err := c.Login(context.Background()); err == nil || !strings.Contains(err.Error(), "another origin") {
		t.Fatalf("Login error = %v, want the cross-origin redirect refused", err)
	}
	if n := elsewhere.Load(); n != 0 {
		t.Fatalf("the second host received %d request(s) carrying the password", n)
	}
}

func TestJSONRPCFollowsASameOriginRedirect(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == rpcPath {
			http.Redirect(w, r, srv.URL+"/jsonapi-moved", http.StatusTemporaryRedirect)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "password-credential") {
			http.Error(w, "the body did not survive a same-origin redirect", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"jsonrpc","visa":"visa-1","result":{"result":{"PartnerId":7}}}`)
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.SessionPath = ""
	c.Creds = Credentials{Partner: "partner", Username: "user", Password: "password-credential"}
	if _, err := c.Login(context.Background()); err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
}

func TestJSONRPCRedirectOriginComparesLikeAURL(t *testing.T) {
	c := New("https://api.backup.management")
	for _, tc := range []struct {
		from, to string
		allowed  bool
	}{
		{"https://API.BACKUP.MANAGEMENT/jsonapi", "https://api.backup.management/jsonapi-canonical", true},
		{"https://api.backup.management/jsonapi", "https://api.backup.management:443/jsonapi", true},
		{"https://api.backup.management/jsonapi", "https://api.backup.management:8443/jsonapi", false},
		{"https://api.backup.management/jsonapi", "http://api.backup.management/jsonapi", false},
		{"https://api.backup.management/jsonapi", "https://elsewhere.example.test/jsonapi", false},
	} {
		first, _ := http.NewRequest(http.MethodPost, tc.from, nil)
		next, _ := http.NewRequest(http.MethodPost, tc.to, nil)
		err := c.HTTP.CheckRedirect(next, []*http.Request{first})
		if (err == nil) != tc.allowed {
			t.Errorf("%s -> %s: err=%v, want allowed=%v", tc.from, tc.to, err, tc.allowed)
		}
	}
}
