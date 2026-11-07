// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The client secret goes only to the token endpoint the operator configured:
// a redirect from it (a 307/308 replays the POST body, secret included, to
// wherever Location points) is refused, never followed.
func TestMintRefusesEveryRedirect(t *testing.T) {
	for _, code := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		var leaked atomic.Bool
		other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), "s3cret") || strings.Contains(r.URL.RawQuery, "s3cret") {
				leaked.Store(true)
			}
			_, _ = io.WriteString(w, `{"access_token":"stolen","expires_in":60}`)
		}))
		auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, other.URL+"/oauth/token", code)
		}))
		tok, err := mintBlumiraToken(context.Background(), auth.URL+"/oauth/token", "id", "s3cret", "public-api", 5*time.Second)
		auth.Close()
		other.Close()
		if err == nil {
			t.Errorf("HTTP %d: mint followed the redirect and returned %+v", code, tok)
		}
		if leaked.Load() {
			t.Errorf("HTTP %d: the client secret reached the redirect target", code)
		}
	}
}

// A token endpoint over plaintext http on a non-loopback host never receives
// the client secret (the press's GuardTokenRequest refuses it before dialing).
func TestMintRefusesPlaintextRemoteTokenURL(t *testing.T) {
	_, err := mintBlumiraToken(context.Background(), "http://192.0.2.1/oauth/token", "id", "s3cret", "public-api", 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "plaintext http") {
		t.Fatalf("plaintext remote token URL was not refused before sending: %v", err)
	}
}
