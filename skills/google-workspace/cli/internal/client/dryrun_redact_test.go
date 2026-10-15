// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google-workspace-pp-cli/internal/config"
)

func TestRedactDryRunBody(t *testing.T) {
	v, ok := redactDryRunBody([]byte(`{"primaryEmail":"jane@example.com","password":"Hunter2!","name":{"givenName":"Jane"},"keys":[{"refreshToken":"r1","id":7}],"suspended":false}`))
	if !ok {
		t.Fatal("JSON body must parse")
	}
	out, _ := json.Marshal(v)
	s := string(out)
	for _, leaked := range []string{"Hunter2!", `"r1"`} {
		if strings.Contains(s, leaked) {
			t.Fatalf("secret leaked into the dry-run preview: %s", s)
		}
	}
	for _, kept := range []string{"jane@example.com", "Jane", `"id":7`, `"suspended":false`} {
		if !strings.Contains(s, kept) {
			t.Fatalf("non-secret field lost: %s", s)
		}
	}
	v, _ = redactDryRunBody([]byte(`{"changePasswordAtNextLogin":true,"pageToken":"p2","customerAuthToken":"xfer-secret"}`))
	out, _ = json.Marshal(v)
	s = string(out)
	if !strings.Contains(s, `"changePasswordAtNextLogin":true`) || !strings.Contains(s, `"pageToken":"p2"`) || strings.Contains(s, "xfer-secret") {
		t.Fatalf("booleans and paging cursors stay visible, transfer tokens do not: %s", s)
	}
	if _, ok := redactDryRunBody([]byte("not json")); ok {
		t.Fatal("a non-JSON body is not printed")
	}
}

func TestRedactSecretQueryParams(t *testing.T) {
	in := `POST https://reseller.googleapis.com/apps/reseller/v1/customers/C0x/subscriptions?customerAuthToken=xfer-SECRET&pageToken=p2&customer=my_customer returned HTTP 400`
	out := redactSecretQueryParams(in)
	if strings.Contains(out, "xfer-SECRET") || !strings.Contains(out, "customerAuthToken=[redacted]") {
		t.Fatalf("secret query parameter not masked: %s", out)
	}
	if !strings.Contains(out, "pageToken=p2") || !strings.Contains(out, "customer=my_customer") {
		t.Fatalf("non-secret parameters must stay: %s", out)
	}
	for _, tc := range []string{
		`?token='quoted-SECRET'`,
		`{"address":"https://x/?token=\"esc-SECRET\""}`,
		`https://x/cb?a=1\u0026token=json-SECRET`,
		`https://x/cb?a=1&amp;token=html-SECRET`,
		`https://user:pass-SECRET@hooks.example/cb`,
		`https://bob@corp.example:pw-SECRET@127.0.0.1:9/cb`,
		`https://user:p@ss-SECRET@hooks.example/cb`,
		`https://user:pa'ss-SECRET@hooks.example/cb`,
		`https://x/invoke?sp=%2Ftriggers&sig=sas-SECRET`,
		`https://x/api/hook?code=azfn-SECRET`,
		`https://hooks.example/cb?token=';HOOK-SECRET'`,
		`https://b.s3.example/o?X-Amz-Signature=s3-SECRET&X-Amz-Date=1`,
		`https://hooks.example/cb?token=O'Hara-SECRET`,
		`https://hooks.example/cb?key=api-SECRET`,
		`Post "https://hooks.example/cb?a=1&token=go-q-SECRET": dial tcp`,
		`https://hooks.example/cb?token="unterminated-SECRET`,
		`webhook:https://hooks.example/cb?name=O'Hara&token=O'Brien-SECRET`,
		`'https://hooks.example/cb?token='lead-SECRET`,
		`{"description":"https://hooks.example/cb?note=it's&token='q-SECRET'"}`,
		`https://hooks.example/cb?token=';HOOK-SECRET'`,
		`https://x/?token="a b SPACED-SECRET"`,
		`{"d":"https://x/?token=\"a b ESC-SECRET\" more","e":"f"}`,
		`https://x/?token=";PUNCT-SECRET"`,
		`https://x/?token=" WS-SECRET"`,
		`https://x/?token=",DANGLING-SECRET`,
		`https://x/?token="}BRACE-SECRET"`,
		`https://x/?token="]BRACKET-SECRET"`,
		`https://x/?x-api-key=HYPHEN-SECRET`,
	} {
		if got := RedactURLSecrets(tc); strings.Contains(got, "SECRET") {
			t.Errorf("secret survived: %s -> %s", tc, got)
		}
	}
	for _, keep := range []string{
		`{"a":"https://x/?token=","b":"c"}`,
		`["https://x/?token=",1]`,
		`["https://x/?token=",{"k":"v"}]`,
		`{"msg":"see https://x/?token=","b":"c"}`,
		`{"msg":"see https://x/?token="}`,
		`[{"msg":"see https://x/?token="},{"k":"v"}]`,
		`https://host:443?login_hint=bob@example.com`,
		`https://x/?pageToken=p2&customer=my_customer`,
	} {
		if got := RedactURLSecrets(keep); got != keep {
			t.Errorf("non-secret text changed: %s -> %s", keep, got)
		}
	}
	if got := string(redactChannelToken([]byte(`{"kind":"api#channel","id":"c1","address":"https://user:pw-SECRET@hooks.example/cb"}`))); strings.Contains(got, "SECRET") {
		t.Errorf("a watch response must mask a userinfo password in its address: %s", got)
	}
	c := New(&config.Config{BaseURL: "https://example.invalid"}, time.Second, 0)
	if got := c.maskCredentialText(in); strings.Contains(got, "xfer-SECRET") {
		t.Fatalf("maskCredentialText must mask secret query parameters: %s", got)
	}
	v, _ := redactDryRunBody([]byte(`{"password":12345,"customSchemas":{"Secrets":{"doorCode":"1"}}}`))
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "12345") || strings.Contains(string(b), "doorCode") {
		t.Fatalf("non-string secret-named values must be redacted too: %s", b)
	}
}

func TestWatchResponseHidesChannelToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"kind":"api#channel","id":"chan-1","resourceId":"r1","token":"hook-verify-SECRET","address":"https://hooks.example/notify?a=1&token=addr-SECRET","expiration":"1"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL}, time.Second, 0)
	c.NoCache = true
	data, _, err := c.Post(context.Background(), "/admin/directory/v1/users/watch", map[string]any{"id": "chan-1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hook-verify-SECRET") || strings.Contains(string(data), "addr-SECRET") || !strings.Contains(string(data), `"id":"chan-1"`) {
		t.Fatalf("watch response must keep the channel but hide its token: %s", data)
	}
	other := []byte(`{"kind":"admin#directory#user","token":"not-a-channel"}`)
	if string(redactChannelToken(other)) != string(other) {
		t.Fatal("only api#channel responses are rewritten")
	}
}

// Hand-fix (handfixes.json dry-run-redacts-secrets): a URL that fails to
// parse must not come back verbatim in the request error.
func TestRequestBuildErrorMasksURLSecrets(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := New(&config.Config{BaseURL: "https://user:SECRET@host.example/%zz"}, time.Second, 0)
	_, err := c.Get(context.Background(), "/x?token=SECRET", nil)
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("request error leaked a secret: %v", err)
	}
}

// The scanner's quote lookup is bounded: a megabyte of parameters stays fast.
func TestRedactSecretQueryParamsIsLinear(t *testing.T) {
	in := strings.Repeat("&token=a", 125000)
	start := time.Now()
	if out := RedactURLSecrets(in); strings.Contains(out, "=a&") {
		t.Fatal("values must be masked")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("1 MB took %s", d)
	}
}

// Hand-fix (handfixes.json watch-response-hides-channel-token): an echoed
// webhook address keeps only its scheme and host.
func TestWatchResponseMasksAddressPath(t *testing.T) {
	out := string(redactChannelToken([]byte(`{"kind":"api#channel","id":"c1","address":"https://hooks.slack.example/services/T0/B0/PATH-SECRET"}`)))
	if strings.Contains(out, "SECRET") || !strings.Contains(out, "https://hooks.slack.example/[redacted]") {
		t.Fatalf("address path must be masked: %s", out)
	}
}

// Masking a quoted value never breaks the JSON around it.
func TestRedactKeepsJSONValid(t *testing.T) {
	for _, in := range []string{
		`{"a":"https://x/?token='abc-SECRET","b":"it's"}`,
		`{"d":"https://x/?token=\"a b SECRET\" more","e":"f"}`,
		`["https://x/?token=",1]`,
	} {
		out := RedactURLSecrets(in)
		if !json.Valid([]byte(out)) || strings.Contains(out, "SECRET") {
			t.Errorf("%s -> %s", in, out)
		}
	}
}
