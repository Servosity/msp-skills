// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// --- class: credential sent to another host (token grant redirects) -------

// redirectingTokenEndpoint answers the pinned token URL with a redirect to
// another host and records anything that reaches a host other than it.
type redirectingTokenEndpoint struct {
	status  int
	mu      sync.Mutex
	offHost []string
}

func (rt *redirectingTokenEndpoint) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "oauth2.googleapis.com" {
		return &http.Response{StatusCode: rt.status, Header: http.Header{"Location": {"https://attacker.example/collect"}},
			Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	}
	var body []byte
	if r.Body != nil {
		body, _ = io.ReadAll(r.Body)
	}
	rt.mu.Lock()
	rt.offHost = append(rt.offHost, r.Method+" "+r.URL.Host+" carries-assertion="+strconv.FormatBool(bytes.Contains(body, []byte("assertion="))))
	rt.mu.Unlock()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"access_token":"minted-elsewhere","token_type":"Bearer","expires_in":3600}`)), Request: r}, nil
}

func writeTestServiceAccountKey(t *testing.T) string {
	t.Helper()
	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(pk)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := json.Marshal(map[string]string{
		"type":           "service_account",
		"client_email":   "sa@example.iam.gserviceaccount.com",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})),
		"private_key_id": "kid-1",
		"token_uri":      "https://oauth2.googleapis.com/token",
	})
	p := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(p, key, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The signed service-account assertion goes to the pinned token URL and
// nowhere else: a 307/308 from the token endpoint would replay the POST body
// (the assertion) to the Location host.
func TestTenantTokenGrantNeverFollowsRedirect(t *testing.T) {
	keyPath := writeTestServiceAccountKey(t)
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect, http.StatusFound} {
		rt := &redirectingTokenEndpoint{status: status}
		orig := http.DefaultTransport
		http.DefaultTransport = rt
		tn := gwsTenant{Name: "redirect-" + http.StatusText(status), Domain: "example.com", Admin: "admin@example.com", KeyFile: keyPath}
		_, err := mintTenantToken(context.Background(), tn, []string{"https://www.googleapis.com/auth/admin.directory.user.readonly"})
		http.DefaultTransport = orig
		rt.mu.Lock()
		off := append([]string(nil), rt.offHost...)
		rt.mu.Unlock()
		if len(off) > 0 {
			t.Errorf("HTTP %d from the token endpoint sent a request to another host: %q", status, off)
		}
		if err == nil {
			t.Errorf("HTTP %d from the token endpoint must fail the mint, not yield a token", status)
		}
	}
}
