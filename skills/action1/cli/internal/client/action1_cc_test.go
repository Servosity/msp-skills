// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"action1-pp-cli/internal/config"
)

// action1 handfix client-credentials-mint: the env-supplied client pair is
// never written to any file (config.toml or the credential store), and nor is
// the token minted for it (minted-token-bound-to-its-client: stored beside the
// files' pair, it would read as that client's token on the next run). The
// token is reused by every client this process builds for the pair.
func TestEnvClientPairIsNeverPersisted(t *testing.T) {
	isolateCredentialStore(t)
	const id, secret = "env-client-id-7f3a9c21", "env-client-secret-Qx8mW2pL"
	var mints int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mints++
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"minted-access-5d1e","refresh_token":"minted-refresh-8b2c","expires_in":3600}`)
	}))
	defer srv.Close()
	t.Setenv("ACTION1_BASE_URL", srv.URL)
	t.Setenv("ACTION1_CLIENT_ID", id)
	t.Setenv("ACTION1_CLIENT_SECRET", secret)
	cfgPath := filepath.Join(os.Getenv("ACTION1_HOME"), "config.toml")
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if h, err := New(cfg, 5*time.Second, 0).authHeader(context.Background()); err != nil || h != "Bearer minted-access-5d1e" {
		t.Fatalf("authHeader = %q, %v; want the minted bearer", h, err)
	}
	var files int
	_ = filepath.Walk(os.Getenv("ACTION1_HOME"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		files++
		b, _ := os.ReadFile(p)
		for _, v := range []string{id, secret, "minted-access-5d1e", "minted-refresh-8b2c"} {
			if strings.Contains(string(b), v) {
				t.Errorf("%s holds the env-supplied client credential or its token %q", p, v)
			}
		}
		return nil
	})
	_ = files
	cfg2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if h, err := New(cfg2, 5*time.Second, 0).authHeader(context.Background()); err != nil || h != "Bearer minted-access-5d1e" || mints != 1 {
		t.Fatalf("a second client for the pair: authHeader = %q, %v after %d mints; want the process's token, one mint", h, err, mints)
	}
}

// The token request goes to <base URL>/oauth2/token on the API origin, with
// the base URL's own path (a region base URL mints from its own region).
func TestMintPostsJSONToTheBaseURLTokenPath(t *testing.T) {
	isolateCredentialStore(t)
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.Path + " " + r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-region","expires_in":3600}`)
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL + "/api/3.0/", ClientID: "cid", ClientSecret: "csecret", Path: filepath.Join(t.TempDir(), "config.toml")}
	if _, err := New(cfg, 5*time.Second, 0).authHeader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got != "POST /api/3.0/oauth2/token application/json" {
		t.Fatalf("token request = %q", got)
	}
}

// A rejected mint returns promptly with an error that names the variables and
// never repeats the posted secret, even when the endpoint echoes it.
func TestRejectedMintReturnsWithoutTheSecret(t *testing.T) {
	isolateCredentialStore(t)
	const secret = "rejected-test-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(append(append([]byte(`{"error":"invalid_client","error_description":`), body...), '}'))
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL, ClientID: "cid", ClientSecret: secret, Path: filepath.Join(t.TempDir(), "config.toml")}
	done := make(chan error, 1)
	go func() {
		_, err := New(cfg, 5*time.Second, 0).authHeader(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a 401 mint must fail")
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("mint error repeats the secret: %v", err)
		}
		if !strings.Contains(err.Error(), "ACTION1_CLIENT_SECRET") || !strings.Contains(err.Error(), "invalid_client") {
			t.Fatalf("mint error should name the variables and the RFC 6749 code: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a rejected mint hung")
	}
}

// --dry-run never mints: the preview must not contact the token endpoint.
func TestDryRunNeverMints(t *testing.T) {
	isolateCredentialStore(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = io.WriteString(w, `{"access_token":"never"}`)
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "csecret", Path: filepath.Join(t.TempDir(), "config.toml")}
	c := New(cfg, 5*time.Second, 0)
	c.DryRun = true
	if _, err := c.authHeader(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("--dry-run contacted the token endpoint %d time(s)", n)
	}
}

// Concurrent requests (sync fans out across organizations) mint one token.
func TestConcurrentRequestsMintOnce(t *testing.T) {
	isolateCredentialStore(t)
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"tok-once","expires_in":3600}`)
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL, ClientID: "cid", ClientSecret: "csecret", Path: filepath.Join(t.TempDir(), "config.toml")}
	c := New(cfg, 5*time.Second, 0)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h, err := c.authHeader(context.Background()); err != nil || h != "Bearer tok-once" {
				t.Errorf("authHeader = %q, %v", h, err)
			}
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("%d mints for 8 concurrent requests; want 1", n)
	}
}
