// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ninjaone-pp-cli/internal/config"
)

// Class token-errors-funnelled-at-source: every error the client-credentials
// token exchange returns passes the secret funnel before it leaves the client,
// so no printer downstream (a sync event, a warning line, an MCP result) can
// carry the client secret, and a refused mint returns instead of deadlocking
// on the token-state lock the mint already holds.

func tokenErrorClient(t *testing.T, srv *httptest.Server, secret string) *Client {
	t.Helper()
	home := t.TempDir()
	for _, k := range []string{"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "NINJAONE_HOME"} {
		t.Setenv(k, home)
	}
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	cfg := &config.Config{
		Path:         filepath.Join(home, "config.toml"),
		BaseURL:      srv.URL,
		TokenURL:     srv.URL + "/ws/oauth/token",
		ClientID:     "token-error-client-id",
		ClientSecret: secret,
	}
	return New(cfg, 5*time.Second, 0)
}

// callWithin runs one GET and fails the test (instead of hanging the binary)
// when it does not return in time.
func callWithin(t *testing.T, c *Client, d time.Duration) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		_, _, err := c.do(context.Background(), http.MethodGet, "/v2/devices", nil, nil, nil)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("a GET whose token mint was refused did not return within %s (deadlock on the token-state lock)", d)
		return nil
	}
}

func TestRefusedTokenMintReturnsAnError(t *testing.T) {
	for _, status := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"Invalid client credentials"}`)
		}))
		c := tokenErrorClient(t, srv, "refused-mint-secret-7Qx2Lk9")
		err := callWithin(t, c, 10*time.Second)
		srv.Close()
		if err == nil {
			t.Fatalf("HTTP %d from the token endpoint produced no error", status)
		}
		if !strings.Contains(err.Error(), "invalid_client") {
			t.Fatalf("HTTP %d: the error lost the server's answer: %v", status, err)
		}
	}
}

func TestTokenMintTransportErrorsNeverCarryTheClientSecret(t *testing.T) {
	secret := "mint-test-secret-000"
	for name, location := range map[string]string{
		"host":   "https://" + secret + ".example.invalid/token",
		"query":  "https://other.example.invalid/token?s=" + base64.StdEncoding.EncodeToString([]byte(secret)),
		"pathed": "https://other.example.invalid/" + secret,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			}))
			defer srv.Close()
			c := tokenErrorClient(t, srv, secret)
			err := callWithin(t, c, 10*time.Second)
			if err == nil {
				t.Fatal("a token mint redirected off its origin produced no error")
			}
			text := err.Error()
			if strings.Contains(text, secret) || strings.Contains(text, base64.StdEncoding.EncodeToString([]byte(secret))) {
				t.Fatalf("the client's error carries the client secret: %q", text)
			}
		})
	}
}

func TestTokenMintTransportErrorsNeverCarryAShortSecret(t *testing.T) {
	secret := "s3cr3t"
	for name, location := range map[string]string{
		"host":   "https://" + secret + ".example.invalid/token",
		"upper":  "https://" + strings.ToUpper(secret) + ".example.invalid/token",
		"query":  "https://other.example.invalid/token?s=" + base64.StdEncoding.EncodeToString([]byte(secret)),
		"pathed": "https://other.example.invalid/x/" + secret,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, location, http.StatusTemporaryRedirect)
			}))
			defer srv.Close()
			c := tokenErrorClient(t, srv, secret)
			err := callWithin(t, c, 10*time.Second)
			if err == nil {
				t.Fatal("a token mint redirected off its origin produced no error")
			}
			text := strings.ToLower(err.Error())
			if strings.Contains(text, secret) || strings.Contains(err.Error(), base64.StdEncoding.EncodeToString([]byte(secret))) {
				t.Fatalf("the client's error carries the short client secret: %q", err.Error())
			}
		})
	}
}
