// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil/testenv"
	"zoom-phone-pp-cli/internal/config"
)

// Codex round 10 (c): the refresh-token save (apply-handfixes
// "internal/client/client.go#refreshlock") holds the per-profile lock and
// passes the complete-snapshot stale check like the mint.
func TestZoomRefreshSaveRefusesReplacedProfile(t *testing.T) {
	home := testenv.Isolate(t)
	for _, k := range []string{"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
	acct := filepath.Join(home, "accounts", "acme")
	t.Setenv("ZOOM_PHONE_HOME", acct)
	path := filepath.Join(acct, "config.toml")
	write := func(body string) {
		if err := os.MkdirAll(acct, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"tok-refreshed","refresh_token":"refresh-2","expires_in":3600}`)
	}))
	defer srv.Close()
	tenantA := "client_id = 'CID_A'\nclient_secret = 'secret-A'\nphone_account_id = 'ACCT_A'\nrefresh_token = 'refresh-1'\ntoken_url = '" + srv.URL + "/oauth/token'\n"

	// Normal refresh saves.
	write(tenantA)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	if err := c.refreshAccessToken(context.Background()); err != nil {
		t.Fatalf("normal refresh: %v", err)
	}
	if reloaded, _ := config.Load(path); reloaded.AccessToken != "tok-refreshed" {
		t.Fatalf("normal refresh not saved: %q", reloaded.AccessToken)
	}

	// Profile replaced between load and the refresh save: refused, B intact.
	_ = os.RemoveAll(filepath.Join(acct, "data"))
	write(tenantA)
	cfg, err = config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	tenantB := "client_id = 'CID_B'\nclient_secret = 'secret-B'\nphone_account_id = 'ACCT_B'\n"
	write(tenantB)
	c = New(cfg, 5*time.Second, 0)
	if err := c.refreshAccessToken(context.Background()); err == nil || !strings.Contains(err.Error(), "saved profile changed") {
		t.Fatalf("refresh over a replaced profile: err=%v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != tenantB {
		t.Fatalf("replaced profile overwritten:\n%s", data)
	}
	if _, err := os.Stat(filepath.Join(acct, "data", "credentials.toml")); !os.IsNotExist(err) {
		t.Fatalf("refreshed token cached for the replaced profile: %v", err)
	}
}
