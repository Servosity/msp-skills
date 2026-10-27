// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Codex review round 1 (reprint on press 4.34.0): each test below failed on
// the code Codex reviewed and names the class of defect it pins.
package coverpc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingTransport records every request it is asked to send and answers
// none of them: a test using it proves a request was refused before dialling.
type countingTransport struct{ n atomic.Int32 }

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.n.Add(1)
	return nil, os.ErrDeadlineExceeded
}

// Class: credentials sent over plaintext HTTP. Every coverpc request carries
// the password (Login) or the session visa, so a non-loopback http:// origin
// must be refused before anything is sent.
func TestCredentialBearingRequestsRefusePlaintextHTTP(t *testing.T) {
	for _, base := range []string{"http://api.backup.management", "HTTP://api.backup.management:8080", "http://10.0.0.5"} {
		rt := &countingTransport{}
		c := New(base)
		c.HTTP.Transport = rt
		c.Creds = Credentials{Partner: "acme", Username: "api", Password: "secret123456"}
		c.SessionPath = ""
		if _, err := c.Login(context.Background()); err == nil {
			t.Fatalf("%s: Login over plaintext HTTP succeeded", base)
		}
		c.visa = "cached-visa-0123456789"
		if _, err := c.Call(context.Background(), "GetServerInfo", nil); err == nil {
			t.Fatalf("%s: Call over plaintext HTTP succeeded", base)
		}
		if got := rt.n.Load(); got != 0 {
			t.Fatalf("%s: %d request(s) were sent over plaintext HTTP", base, got)
		}
	}
}

func loginServer(t *testing.T, visa string, partnerID int64, logins *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		switch req["method"] {
		case "Login":
			logins.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "jsonrpc", "jsonrpc": "2.0", "visa": visa,
				"result": map[string]any{"result": map[string]any{"Id": 1, "PartnerId": partnerID}},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "jsonrpc", "jsonrpc": "2.0", "visa": visa,
				"result": map[string]any{"result": map[string]any{"sawVisa": req["visa"]}},
			})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Class: a cached session reused for another tenant. The cache must be bound
// to the origin, partner and username it was issued for.
func TestSessionCacheIsBoundToOriginPartnerAndUser(t *testing.T) {
	var logins atomic.Int32
	srv := loginServer(t, "visa-tenant-B", 20, &logins)
	path := filepath.Join(t.TempDir(), "session.json")
	// A fresh cache written for tenant A (partner id 10) under the same login name.
	stale, _ := json.Marshal(map[string]any{"visa": "visa-tenant-A", "saved_at": time.Now().UTC(), "username": "api", "partner_id": 10})
	if err := os.WriteFile(path, stale, 0o600); err != nil {
		t.Fatal(err)
	}
	c := New(srv.URL)
	c.Creds = Credentials{Partner: "B", Username: "api", Password: "test-secret-b"}
	c.SessionPath = path
	root, err := c.RootPartnerID(context.Background())
	if err != nil {
		t.Fatalf("RootPartnerID: %v", err)
	}
	if root != 20 {
		t.Fatalf("partner %d from tenant A's cached session was used for tenant B (want 20)", root)
	}
	if logins.Load() != 1 {
		t.Fatalf("expected a fresh Login for tenant B, got %d logins", logins.Load())
	}

	// The same login at another origin must not reuse this origin's visa.
	var logins2 atomic.Int32
	srv2 := loginServer(t, "visa-origin-2", 30, &logins2)
	c2 := New(srv2.URL)
	c2.Creds = c.Creds
	c2.SessionPath = path
	visa, err := c2.Visa(context.Background())
	if err != nil {
		t.Fatalf("Visa: %v", err)
	}
	if visa != "visa-origin-2" || logins2.Load() != 1 {
		t.Fatalf("visa %q from another origin was reused (logins at origin 2: %d)", visa, logins2.Load())
	}
}

// Class: server-controlled SUCCESS data shown without the funnel. A result
// that repeats a credential (password, sent visa, or returned visa) is refused.
func TestSuccessResultThatRepeatsACredentialIsRefused(t *testing.T) {
	for name, echo := range map[string]string{
		"password":        "secret123456",
		"sent visa":       "visa-sent-0123456789",
		"returned visa":   "visa-new-0123456789",
		"base64 password": base64.StdEncoding.EncodeToString([]byte("secret123456")),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "jsonrpc", "jsonrpc": "2.0", "visa": "visa-new-0123456789",
					"result": map[string]any{"result": map[string]any{"Version": echo}},
				})
			}))
			t.Cleanup(srv.Close)
			c := New(srv.URL)
			c.Creds = Credentials{Partner: "acme", Username: "api", Password: "secret123456"}
			c.SessionPath = ""
			c.visa = "visa-sent-0123456789"
			result, err := c.Call(context.Background(), "GetServerInfo", nil)
			if err == nil {
				t.Fatalf("a result repeating the %s was returned: %s", name, result)
			}
			if strings.Contains(err.Error(), echo) {
				t.Fatalf("error repeats the %s: %v", name, err)
			}
		})
	}
}

// Class: a visa the server just returned is a credential before any text of
// that response is shown.
func TestResponseVisaIsHeldBeforeErrorTextIsShown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":"jsonrpc","jsonrpc":"2.0","visa":"new-visa-1234567890","error":{"code":-32603,"data":13501,"message":"new-visa-1234567890"}}`))
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	c.Creds = Credentials{Partner: "acme", Username: "api", Password: "secret123456"}
	c.SessionPath = ""
	_, err := c.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "new-visa-1234567890") {
		t.Fatalf("returned visa shown in the error: %v", err)
	}
	// Same for a non-200 answer whose body carries the returned visa.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"visa":"other-visa-654321","detail":"session other-visa-654321 rejected"}`))
	}))
	t.Cleanup(srv2.Close)
	c2 := New(srv2.URL)
	c2.Creds = c.Creds
	c2.SessionPath = ""
	_, err = c2.Login(context.Background())
	if err == nil || strings.Contains(err.Error(), "other-visa-654321") {
		t.Fatalf("returned visa shown in the HTTP error: %v", err)
	}
}

// Class: credentials shorter than the shared funnel's decoded-view minimum
// bypassed even literal matching.
func TestShortCredentialIsWithheldAsWrittenAndEncoded(t *testing.T) {
	// "hunter2" is shorter than the shared funnel's 8-character minimum. The
	// escaped case carries a literal backslash-u escape the server never decoded.
	for name, msg := range map[string]string{
		"literal": "hunter2",
		"base64":  "token aHVudGVyMg== refused",
		"hex":     "token 68756e74657232 refused",
		"escaped": `token \u0068unter2 refused`,
	} {
		t.Run(name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"id": "jsonrpc", "jsonrpc": "2.0", "error": map[string]any{"code": -32603, "data": 2100, "message": msg}})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
			t.Cleanup(srv.Close)
			c := New(srv.URL)
			c.Creds = Credentials{Partner: "acme", Username: "api", Password: "hunter2"}
			c.SessionPath = ""
			_, err := c.Login(context.Background())
			if err == nil {
				t.Fatal("expected a Login error")
			}
			shown := err.Error()
			for _, form := range []string{"hunter2", "aHVudGVyMg", "68756e74657232", `\u0068unter2`} {
				if strings.Contains(shown, form) {
					t.Fatalf("short password shown (%s): %v", form, shown)
				}
			}
		})
	}
}

// Class: one shared parser for Cove column values. Multi-valued columns keep
// every value and JSON numbers decode losslessly as decimal integers.
func TestRowsAndSettingsDecodeEveryValueLosslessly(t *testing.T) {
	raw := json.RawMessage(`{"result":[{"AccountId":9007199254740993,"PartnerId":7,"Settings":[{"I78":["D1","D2"]},{"I14":2000000000},{"I15":2e9},{"D9F09":1792800000}]}]}`)
	rows, err := Rows(raw)
	if err != nil {
		t.Fatalf("Rows: %v", err)
	}
	if id, ok := RowInt(rows[0], "AccountId"); !ok || id != 9007199254740993 {
		t.Fatalf("AccountId = %d ok=%v, want 9007199254740993", id, ok)
	}
	s := FlattenSettings(rows[0]["Settings"])
	if s["I78"] != "D1,D2" {
		t.Fatalf("I78 = %q, want every value (D1,D2)", s["I78"])
	}
	for _, code := range []string{"I14", "I15"} {
		if n, ok := SettingInt(s, code); !ok || n != 2000000000 {
			t.Fatalf("%s = %q -> %d ok=%v, want 2000000000", code, s[code], n, ok)
		}
	}
	if n, ok := SettingInt(s, "D9F09"); !ok || n != 1792800000 {
		t.Fatalf("D9F09 = %q -> %d ok=%v", s["D9F09"], n, ok)
	}
}
