// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class tests for the 4.34.2 reprint's credential fix (orchestrator group A):
// credential-output and credential-persistence at the client, the one place
// every vendor answer enters the program.

package client

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/config"
)

func ingressClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(&config.Config{BaseURL: srv.URL, NetsapiensApiKey: "DifferentApiKey55443322"}, 5*time.Second, 0)
	c.HTTPClient = srv.Client()
	c.cacheDir = t.TempDir()
	return c
}

// Every form the secrets below could be shown in.
func shownForms(secrets ...string) []string {
	var out []string
	for _, s := range secrets {
		out = append(out, s, base64.StdEncoding.EncodeToString([]byte(s)))
	}
	return out
}

func assertNoneShown(t *testing.T, where, text string, secrets ...string) {
	t.Helper()
	for _, f := range shownForms(secrets...) {
		if strings.Contains(text, f) {
			t.Fatalf("%s shows a vendor credential (%q):\n%s", where, f, text)
		}
	}
}

func dirText(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			data, _ := os.ReadFile(p)
			b.Write(data)
		}
		return nil
	})
	return b.String()
}

// A credential field in a documented 2xx answer (Domain.email-send-smtp-password,
// spec.json GET /domains) never reaches the caller or the response cache.
func TestClassResponseCredentialsNeverLeaveTheClient(t *testing.T) {
	const secret = "SmtpSecret55443322"
	c := ingressClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"domain":"acme.example","email-send-smtp-password":"`+secret+`"}]`)
	})
	data, err := c.Get(context.Background(), "/domains", nil)
	if err != nil {
		t.Fatal(err)
	}
	assertNoneShown(t, "the answer", string(data), secret)
	if !strings.Contains(string(data), `"domain":"acme.example"`) {
		t.Fatalf("redaction must keep every other field: %s", data)
	}
	assertNoneShown(t, "the response cache", dirText(t, c.cacheDir), secret)
}

// A credential the vendor names inside error text (here a key=value pair in
// the message) never reaches the error.
func TestClassErrorBodyCredentialPairsAreWithheld(t *testing.T) {
	const secret = "SipSecret55443322"
	c := ingressClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":400,"message":"device-sip-registration-password=`+secret+`"}`)
	})
	c.NoCache = true
	_, err := c.Get(context.Background(), "/domains/acme.example/devices", nil)
	if err == nil {
		t.Fatal("a 400 must be an error")
	}
	assertNoneShown(t, "the error", err.Error(), secret)
}

// The redaction placeholder is never written back as a credential: a body
// carrying it in a credential field is refused before the wire.
func TestClassRedactedPlaceholderIsNeverSent(t *testing.T) {
	var hits atomic.Int32
	c := ingressClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})
	_, _, err := c.Put(context.Background(), "/domains/acme.example/users/100", map[string]any{"user": "100", "voicemail-login-pin": "<redacted>"})
	if err == nil || hits.Load() != 0 {
		t.Fatalf("a body that would set a credential to the redaction placeholder was sent (err=%v, requests=%d)", err, hits.Load())
	}
}
