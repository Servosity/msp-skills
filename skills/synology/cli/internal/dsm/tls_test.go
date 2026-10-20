// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package dsm

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func tlsServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(srv.Certificate().Raw)
	return srv, hex.EncodeToString(sum[:])
}

func clientWith(t *testing.T, pin string, skip bool) *http.Client {
	t.Helper()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if err := ApplyTLS(cfg, pin, skip); err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
}

func TestPinnedFingerprintAccepted(t *testing.T) {
	srv, fp := tlsServer(t)
	// Colon-separated upper-case form normalizes to the same digest.
	var b strings.Builder
	for i := 0; i < len(fp); i += 2 {
		if i > 0 {
			b.WriteString(":")
		}
		b.WriteString(strings.ToUpper(fp[i : i+2]))
	}
	resp, err := clientWith(t, b.String(), false).Get(srv.URL)
	if err != nil {
		t.Fatalf("pinned request failed: %v", err)
	}
	resp.Body.Close()
}

func TestPinnedFingerprintMismatchRejected(t *testing.T) {
	srv, _ := tlsServer(t)
	wrong := strings.Repeat("ab", 32)
	if _, err := clientWith(t, wrong, false).Get(srv.URL); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want fingerprint mismatch", err)
	}
}

func TestDefaultVerificationRejectsSelfSigned(t *testing.T) {
	srv, _ := tlsServer(t)
	if _, err := clientWith(t, "", false).Get(srv.URL); err == nil {
		t.Fatal("self-signed certificate accepted without pin or skip")
	}
}

func TestMalformedPinIsAnError(t *testing.T) {
	for _, pin := range []string{"not-hex", "sha256/", ":", " : "} {
		if err := ApplyTLS(&tls.Config{}, pin, true); err == nil {
			t.Fatalf("pin %q accepted", pin)
		}
	}
}

func TestFetchFingerprint(t *testing.T) {
	srv, fp := tlsServer(t)
	got, err := FetchFingerprint(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != fp {
		t.Fatalf("fingerprint = %s want %s", got, fp)
	}
	if _, err := FetchFingerprint(context.Background(), "http://nas:5000"); err == nil {
		t.Fatal("expected an error for an http:// URL")
	}
}

func TestRefuseAllTLSBlocksTrustedCerts(t *testing.T) {
	srv, _ := tlsServer(t)
	cfg := RefuseAllTLS(errors.New("bad pin"))
	cfg.RootCAs = nil
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}
	if _, err := c.Get(srv.URL); err == nil || !strings.Contains(err.Error(), "bad pin") {
		t.Fatalf("err = %v, want TLS refused", err)
	}
}
