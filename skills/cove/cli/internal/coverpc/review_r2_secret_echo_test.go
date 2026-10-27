// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 2 (reprint on press 4.34.0), class secret-echo: each
// test below failed before the fix it names.
package coverpc

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func r2Client(t *testing.T, status int, body, password string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	c.Creds = Credentials{Partner: "acme", Username: "api", Password: password}
	c.SessionPath = ""
	return c
}

// Class short-credential funnel: decodings were not composed for a short
// secret, so a password base64-encoded twice was shown.
func TestShortSecretUnderStackedEncodingsIsWithheld(t *testing.T) {
	c := r2Client(t, 200, `{"error":{"code":-32603,"data":2100,"message":"YzJWamNtVjA="}}`, "secret")
	_, err := c.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "YzJWamNtVjA") {
		t.Fatalf("double-base64 short password shown: %v", err)
	}
}

// Class short-credential funnel, beyond the reported input: every stack of
// the encodings the shared funnel decodes reaches a short secret.
func TestShortSecretStackedEncodingsTable(t *testing.T) {
	const secret = "Kp4$w"
	b64 := base64.StdEncoding.EncodeToString
	stacks := map[string]string{
		"base64 of base64":           b64([]byte(b64([]byte(secret)))),
		"hex of base64":              hex.EncodeToString([]byte(b64([]byte(secret)))),
		"percent-encoded base64":     url.QueryEscape(b64([]byte(secret))),
		"base64 of percent-encoded":  b64([]byte(url.QueryEscape(secret))),
		"json-escaped base64":        esc(`~u0053~u0033~u0041~u0030~u004a~u0048~u0063~u003d`),
		"base64 of hex":              b64([]byte(hex.EncodeToString([]byte(secret)))),
		"hex of hex":                 hex.EncodeToString([]byte(hex.EncodeToString([]byte(secret)))),
		"base64url of base64":        base64.RawURLEncoding.EncodeToString([]byte(b64([]byte(secret)))),
		"base64 of base64 of base64": b64([]byte(b64([]byte(b64([]byte(secret)))))),
		"html entity in base64":      b64([]byte("Kp4&#36;w")),
	}
	for name, msg := range stacks {
		t.Run(name, func(t *testing.T) {
			if w := c0().withhold("upstream said "+msg, []string{secret}); !strings.HasPrefix(w, "[text withheld") {
				t.Fatalf("%s of a short secret shown: %q", name, w)
			}
		})
	}
	if w := c0().withhold("upstream said nothing secret", []string{secret}); w != "upstream said nothing secret" {
		t.Fatalf("clean text withheld: %q", w)
	}
}

func c0() *Client { return &Client{} }

// esc writes a backslash for every ~ in a fixture, so escape sequences reach
// the code under test exactly as a server would write them.
func esc(s string) string { return strings.ReplaceAll(s, "~", string(rune(92))) }

// Class success-data funnel: a non-string JSON scalar was not checked, so a
// numeric password came back in a success result.
func TestNumericPasswordInSuccessResultIsRefused(t *testing.T) {
	c := r2Client(t, 200, `{"visa":"visa-0123456789","result":{"result":{"echo":12345678}}}`, "12345678")
	c.visa = "visa-0123456789"
	if res, err := c.Call(context.Background(), "GetServerInfo", nil); err == nil {
		t.Fatalf("numeric echo of the password returned: %s", res)
	}
}

// Class success-data funnel, beyond the reported input: every scalar form
// (exponent, fraction, nested arrays, booleans, duplicate members) is checked.
func TestEveryScalarFormInSuccessResultIsChecked(t *testing.T) {
	for name, tc := range map[string]struct{ password, result string }{
		"exponent":            {"12345678", `{"result":{"n":1.2345678e7}}`},
		"fraction exponent":   {"12345678", `{"result":{"n":123456.78E2}}`},
		"negative":            {"12345678", `{"result":{"n":-12345678}}`},
		"nested array":        {"12345678", `{"result":{"rows":[[1,[2,[12345678]]]]}}`},
		"float rendering":     {"1.2345678e+07", `{"result":{"n":12345678}}`},
		"boolean":             {"true", `{"result":{"ok":true}}`},
		"null":                {"null", `{"result":{"v":null}}`},
		"duplicate member":    {"secret123456", `{"result":{"a":"c2VjcmV0MTIzNDU2","a":"x"}}`},
		"numeric member name": {"12345678", `{"result":{"12345678":0}}`},
	} {
		t.Run(name, func(t *testing.T) {
			c := r2Client(t, 200, `{"visa":"visa-0123456789","result":`+tc.result+`}`, tc.password)
			c.visa = "visa-0123456789"
			if res, err := c.Call(context.Background(), "GetServerInfo", nil); err == nil {
				t.Fatalf("%s echo of the password returned: %s", name, res)
			}
		})
	}
	c := r2Client(t, 200, `{"visa":"visa-0123456789","result":{"result":{"n":1234567,"ok":false,"s":"fine"}}}`, "12345678")
	c.visa = "visa-0123456789"
	if _, err := c.Call(context.Background(), "GetServerInfo", nil); err != nil {
		t.Fatalf("clean result refused: %v", err)
	}
}

// Class response-visa discovery: an escaped member name hid the visa.
func TestEscapedVisaMemberNameIsHeld(t *testing.T) {
	c := r2Client(t, 500, esc(`{"~u0076isa":"new-visa-1234567890","detail":"failed"}`), "secret123456")
	_, err := c.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "new-visa-1234567890") {
		t.Fatalf("returned visa under an escaped key shown: %v", err)
	}
}

// Class response-visa discovery, beyond the reported input: the member is
// found by its decoded name at any depth, in arrays, under a duplicate name,
// in a JSON document carried in a string, and in a body that does not parse.
func TestResponseVisaDiscoveryByDecodedName(t *testing.T) {
	for name, body := range map[string]string{
		"upper case":          `{"VISA":"new-visa-1234567890"}`,
		"escaped upper":       esc(`{"~u0056~u0049SA":"new-visa-1234567890"}`),
		"nested":              `{"a":{"b":[{"c":{"visa":"new-visa-1234567890"}}]}}`,
		"duplicate name":      esc(`{"~u0076isa":"new-visa-1234567890","visa":"other"}`),
		"json in a string":    `{"detail":"{\"visa\":\"new-visa-1234567890\"}"}`,
		"truncated":           esc(`{"~u0076isa":"new-visa-1234567890","detail":"fai`),
		"html wrapped":        esc(`<pre>{&quot;~u0076isa&quot;:&quot;new-visa-1234567890&quot;}</pre>`),
		"single quoted":       `{'visa': 'new-visa-1234567890'}`,
		"form encoded":        `error=1&visa=new-visa-1234567890&x=2`,
		"numeric visa":        `{"visa":1234567890123456}`,
		"object-valued visa":  `{"visa":{"token":"new-visa-1234567890"}}`,
		"trailing garbage":    esc(`{"~u0076isa":"new-visa-1234567890"} trailing`),
		"second JSON value":   esc(`{} {"~u0076isa":"new-visa-1234567890"}`),
		"escaped quote value": esc(`{"~u0076isa":"new~u002dvisa-1234567890"}`),
	} {
		t.Run(name, func(t *testing.T) {
			want := "new-visa-1234567890"
			if name == "numeric visa" {
				want = "1234567890123456"
			}
			got := responseVisas([]byte(body))
			found := false
			for _, v := range got {
				if v == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("visa %q not discovered in %s: %q", want, body, got)
			}
		})
	}
	if got := responseVisas([]byte(`{"visas":"x","provisa":"y","detail":"visa expired"}`)); len(got) != 0 {
		t.Fatalf("non-visa members discovered: %q", got)
	}
}

// Class success-data funnel, error side: the JSON decoder's message quotes a
// number it could not store, so a numeric password reached the error text.
func TestDecoderErrorTextIsFunnelled(t *testing.T) {
	c := r2Client(t, 200, `{"error":{"code":-32603,"data":12345678.5,"message":"m"}}`, "12345678")
	_, err := c.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "12345678") {
		t.Fatalf("decoder error repeats the password: %v", err)
	}
}

// Class success-data funnel, error side: the JSON-RPC error numbers are server
// text too. A repeating number is left out of the error and the doctor's
// verdict, while Data still classifies the error.
func TestRPCErrorNumbersThatRepeatACredentialAreWithheld(t *testing.T) {
	c := r2Client(t, 200, `{"error":{"code":12345678,"data":1701,"message":"m"}}`, "12345678")
	_, err := c.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "12345678") {
		t.Fatalf("error number repeats the password: %v", err)
	}
	if !IsVisaError(err) {
		t.Fatalf("withheld numbers lost the visa classification: %v", err)
	}
	c = r2Client(t, 200, `{"error":{"code":-32603,"data":12345678,"message":"m"}}`, "12345678")
	_, err = c.Login(context.Background())
	var rpcErr *RPCError
	if !errors.As(err, &rpcErr) || strings.Contains(rpcErr.DataText(), "12345678") || strings.Contains(err.Error(), "12345678") {
		t.Fatalf("data number repeats the password: %v", err)
	}
	c = r2Client(t, 200, `{"error":{"code":-32603,"data":2100,"message":"m"}}`, "secret123456")
	_, err = c.Login(context.Background())
	if !errors.As(err, &rpcErr) || rpcErr.DataText() != "2100" || !strings.Contains(err.Error(), "-32603 (data 2100)") {
		t.Fatalf("clean error numbers withheld: %v", err)
	}
}

// Class endpoint binding: a binding must keep every routing-relevant part of
// the request URL, and a binding in the older form must never match.
func TestEndpointBindingKeepsEveryRoutingPart(t *testing.T) {
	distinct := [][2]string{
		{"https://[fe80::1%25lan0]:8443", "https://[fe80::1%25LAN0]:8443"},
		{"https://[fe80::1%25lan0]", "https://[fe80::1%25LAN0]"},
		{"https://h.example/a/", "https://h.example/a"},
		{"https://h.example/a%2Fb", "https://h.example/a/b"},
		{"https://h.example:8443", "https://h.example"},
		{"http://127.0.0.1", "https://127.0.0.1"},
		{"https://exämple.test", "https://EXÄMPLE.test"},
	}
	for _, p := range distinct {
		if a, b := normalizeEndpoint(p[0]), normalizeEndpoint(p[1]); a == "" || a == b {
			t.Errorf("%s and %s share the binding %q", p[0], p[1], a)
		}
	}
	same := [][2]string{
		{"https://API.Example.com:443", "https://api.example.com"},
		{"https://[FE80::1%25lan0]", "https://[fe80::1%25lan0]"},
		{"https://user:pw-0123456789@h.example", "https://h.example"},
	}
	for _, p := range same {
		if a, b := normalizeEndpoint(p[0]), normalizeEndpoint(p[1]); a == "" || a != b {
			t.Errorf("%s and %s bound differently: %q vs %q", p[0], p[1], a, b)
		}
	}
	if b := normalizeEndpoint("https://user:pw-0123456789@h.example"); strings.Contains(b, "pw-0123456789") || strings.Contains(b, "user") {
		t.Errorf("binding carries userinfo: %q", b)
	}
	// A query or fragment would swallow the JSON-RPC path: no binding, and the
	// client refuses to dial it.
	for _, base := range []string{"https://h.example/?tenant=a", "https://h.example?", "https://h.example/#x", "https://h.example#"} {
		if b := normalizeEndpoint(base); b != "" {
			t.Errorf("%s has the binding %q", base, b)
		}
		if err := refusePlaintextOrigin(base); err == nil {
			t.Errorf("%s accepted as a base URL", base)
		}
	}
	if EndpointBindingIsCurrent("https://h.example") || !EndpointBindingIsCurrent(normalizeEndpoint("https://h.example")) {
		t.Error("an older-form binding reads as current, or the current one does not")
	}
}

// Class endpoint binding: a session cached under the older binding form is
// refused (a fresh login), never reused.
func TestOlderFormSessionBindingIsNotReused(t *testing.T) {
	var logins atomic.Int32
	srv := loginServer(t, "visa-fresh-0123456789", 20, &logins)
	path := filepath.Join(t.TempDir(), "session.json")
	old := strings.TrimRight(srv.URL, "/") // the form older versions wrote
	stale, _ := json.Marshal(map[string]any{"visa": "visa-old-binding-0123", "saved_at": time.Now().UTC(), "username": "api", "partner": "acme", "endpoint": old})
	if err := os.WriteFile(path, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(srv.URL)
	c.Creds = Credentials{Partner: "acme", Username: "api", Password: "secret123456"}
	c.SessionPath = path
	visa, err := c.Visa(context.Background())
	if err != nil {
		t.Fatalf("Visa: %v", err)
	}
	if visa != "visa-fresh-0123456789" || logins.Load() != 1 {
		t.Fatalf("older-form binding reused: visa %q, %d logins", visa, logins.Load())
	}
	// The session it saved carries the current binding and is reused.
	c2 := New(srv.URL)
	c2.Creds = c.Creds
	c2.SessionPath = path
	if v, err := c2.Visa(context.Background()); err != nil || v != "visa-fresh-0123456789" || logins.Load() != 1 {
		t.Fatalf("current binding not reused: %q %v (%d logins)", v, err, logins.Load())
	}
}
