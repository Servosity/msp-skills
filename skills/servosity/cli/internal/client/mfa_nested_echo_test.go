// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "credential-header-values-masked" entry. See docs/reprint-survival.md.

package client

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/config"
)

// TestErrorBodyMasksANestedEncodedMFAHeader pins that a short credential sent
// as a request header (a 6-digit X-Servosity-Mfa code) is withheld when the
// server echoes it inside more than one encoding. The decoded checks skip
// secrets under minEncodedSecretLen, and the masks cover one encoding layer,
// so each echo below survived unmasked before the bounded short-value walk.
func TestErrorBodyMasksANestedEncodedMFAHeader(t *testing.T) {
	const mfa = "123456"
	b64 := base64.StdEncoding.EncodeToString([]byte(mfa))
	layered := func(n int) string {
		v := mfa
		for i := 0; i < n; i++ {
			v = base64.StdEncoding.EncodeToString([]byte(v))
		}
		return v
	}
	fullwidth := []rune{}
	for _, r := range mfa {
		fullwidth = append(fullwidth, r-'0'+0xFF10)
	}
	echoes := map[string]string{
		"status line inside the body": layered(6) + " returned HTTP 403",
		"base64 of full-width digits": base64.StdEncoding.EncodeToString([]byte(string(fullwidth))),
		"base64 six times":            layered(6),
		"base64 ten times":            layered(10),
		"hex of base64x3":             hex.EncodeToString([]byte(layered(3))),
		"percent of percent":          "%2531%2532%2533%2534%2535%2536",
		"base64 of base64":            base64.StdEncoding.EncodeToString([]byte(b64)),
		"hex of base64":               hex.EncodeToString([]byte(b64)),
		"entity of percent":           "&#37;31&#37;32&#37;33&#37;34&#37;35&#37;36",
	}
	for name, echo := range echoes {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"detail":"Invalid MFA code: ` + echo + `"}`))
			}))
			defer srv.Close()

			c := New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}, 10*time.Second, 0)
			c.NoCache = true
			_, err := c.GetWithHeaders(context.Background(), "/backups/17/encryption-key/", nil, map[string]string{"X-Servosity-Mfa": mfa})
			if err == nil {
				t.Fatal("want the 403 as an error")
			}
			if strings.Contains(err.Error(), echo) || strings.Contains(err.Error(), mfa) {
				t.Fatalf("error echoes the MFA code (%s): %v", name, err)
			}
		})
	}
}

// TestShortHeaderWalkLeavesUnrelatedErrorsAlone pins the other direction: with
// an MFA code held, an error body that does not carry it is shown as sent.
func TestShortHeaderWalkLeavesUnrelatedErrorsAlone(t *testing.T) {
	const body = `{"detail":"Backup 4421 is locked by job 98765."}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}, 10*time.Second, 0)
	c.NoCache = true
	_, err := c.GetWithHeaders(context.Background(), "/backups/17/encryption-key/", nil, map[string]string{"X-Servosity-Mfa": "123456"})
	if err == nil {
		t.Fatal("want the 409 as an error")
	}
	if !strings.Contains(err.Error(), "locked by job 98765") {
		t.Fatalf("unrelated error body was withheld: %v", err)
	}
}

// TestErrorBodyMasksAShortHeaderUnderUnpaddedBase64: a 4-character credential
// header value echoed as unpadded base64url four times is withheld too.
func TestErrorBodyMasksAShortHeaderUnderUnpaddedBase64(t *testing.T) {
	const value = "A1B2"
	echo := value
	for i := 0; i < 4; i++ {
		echo = base64.RawURLEncoding.EncodeToString([]byte(echo))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"MFA rejected: ` + echo + `"}`))
	}))
	defer srv.Close()
	c := New(&config.Config{BaseURL: srv.URL, ServosityMspToken: "partner-secret"}, 10*time.Second, 0)
	c.NoCache = true
	_, err := c.GetWithHeaders(context.Background(), "/backups/17/encryption-key/", nil, map[string]string{"X-Servosity-Mfa": value})
	if err == nil {
		t.Fatal("want the 403 as an error")
	}
	if strings.Contains(err.Error(), echo) {
		t.Fatalf("error echoes the short header value: %v", err)
	}
}
