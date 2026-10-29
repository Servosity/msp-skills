// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Class credential-field-not-recognized: a credential name matches in every
// spelling (camelCase, snake_case, kebab-case flags and headers, any case),
// and the public half of a credential pair (an access key ID) counts as a
// credential; a resource identifier does not (apiKeyId keeps matching
// "apikey", a signal the rule already had). A Pax8 client ID is half of the
// client-credentials pair, so it is a credential name.
func TestCredentialNamesMatchInEverySpelling(t *testing.T) {
	for _, name := range []string{
		"clientSecret", "client_secret", "client-secret", "password", "psk", "key", "Key", "secretAccessKey",
		"apiKey", "api_key", "api-key", "x-api-key", "X-API-KEY",
		"accessKeyId", "access_key_id", "access-key-id",
		"clientId", "client_id", "client-id", "ClientID", "CLIENT_ID",
		"private-key", "license-key", "pre-shared-key", "pass-phrase", "passphrase",
	} {
		if !SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"id", "name", "keyId", "creatorKeyId", "client", "clientProfile", "client-device-type",
		"password_complexity", "passwordComplexity", "keyName", "deviceKey", "psk-id-prefix", "originId",
	} {
		if SecretFieldName(name) {
			t.Errorf("SecretFieldName(%q) = true, want false", name)
		}
	}
}

// Class outbound-sink-bypasses-chokepoint (Codex r2): a non-API request whose
// URL (path, query, userinfo, percent-encoded or not) or a header repeats a
// held value is refused before any byte is sent, a clean one is sent, and a
// redirect is never followed.
func TestSendOutboundRefusesHeldValuesAndRedirects(t *testing.T) {
	const tok = "outboundUnitToken-6d2a9f1c4e8b"
	HoldKeys(tok)
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/moved" {
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	refused := []string{
		srv.URL + "/collect?token=" + tok,
		srv.URL + "/collect?token=" + url.QueryEscape("x "+tok),
		srv.URL + "/hooks/" + tok,
		srv.URL + "/hooks/" + url.PathEscape(tok) + "%2F",
		"http://user:" + tok + "@" + u.Host + "/collect",
		"http://" + tok + "@" + u.Host + "/collect",
		srv.URL + "/collect#" + tok,
	}
	body := []byte("{}")
	for _, target := range refused {
		if _, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, target, nil, body, 5*time.Second); !errors.Is(err, ErrOutboundHoldsSecret) || strings.Contains(err.Error(), tok) {
			t.Errorf("%s: want a refusal that names the setting only, got %v", target, err)
		}
	}
	if _, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, srv.URL+"/collect", http.Header{"X-Api-Key": {"Bearer " + tok}}, body, 5*time.Second); !errors.Is(err, ErrOutboundHoldsSecret) {
		t.Errorf("a held header value was not refused: %v", err)
	}
	if hits != 0 {
		t.Fatalf("a refused request reached the server %d time(s)", hits)
	}
	resp, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, srv.URL+"/collect", http.Header{"Content-Type": {"application/json"}}, body, 5*time.Second)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("a clean request failed: %v", err)
	}
	_ = resp.Body.Close()
	if _, err := SendOutbound("TEST_ENDPOINT", http.MethodPost, srv.URL+"/moved", nil, body, 5*time.Second); err == nil || !strings.Contains(err.Error(), "redirects are not followed") {
		t.Fatalf("a redirect was not refused: %v", err)
	}
	if hits != 2 {
		t.Fatalf("want 2 server hits (clean send, redirect answer, no follow), got %d", hits)
	}
}

// Class holder-incomplete (pax8): every credential variable in the
// environment is held, the client ID and secret included, and the
// credentials a *_URL variable carries; a URL, the audience and the scope
// are not credentials.
func TestHoldEnvCredentialsHoldsEveryCredentialVariable(t *testing.T) {
	t.Setenv("PAX8_CLIENT_ID", "envHoldUnitId-4d8b2f6a")
	t.Setenv("PAX8_CLIENT_SECRET", "envHoldUnitSecret-9c3e7a1b")
	t.Setenv("PAX8_MCP_HTTP_TOKEN", "envHoldUnitHttp-2a6c8e4f")
	t.Setenv("PAX8_FUTURE_API_KEY", "envHoldUnitApiKey-7b1d5f3e")
	t.Setenv("PAX8_TOKEN_URL", "https://ro:envHoldUnitUrlPw-5e9a@token.example.com/oauth/token")
	t.Setenv("PAX8_AUDIENCE", "api://envHoldUnitAudience")
	HoldEnvCredentials()
	for _, v := range []string{"envHoldUnitId-4d8b2f6a", "envHoldUnitSecret-9c3e7a1b", "envHoldUnitHttp-2a6c8e4f", "envHoldUnitApiKey-7b1d5f3e", "envHoldUnitUrlPw-5e9a"} {
		if !HeldEchoes("x " + v + " y") {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"https://ro:", "token.example.com/oauth/token", "api://envHoldUnitAudience"} {
		if HeldEchoes(v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}
