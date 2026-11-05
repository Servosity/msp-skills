// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// An explicit login with environment credentials for tenant B over a saved
// tenant A profile saves B as one consistent identity (app and account).
func TestZoomPersistAccountIDKeepsIdentityConsistent(t *testing.T) {
	dir := t.TempDir()
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "ZOOM_PHONE_HOME"} {
		t.Setenv(k, dir)
	}
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("phone_account_id = 'acct-A'\nclient_id = 'cid-A'\nclient_secret = 'sec-A'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "acct-B")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ZoomFileAccountID() != "acct-A" || cfg.ZoomPhoneAccountId != "acct-B" {
		t.Fatalf("file=%q effective=%q", cfg.ZoomFileAccountID(), cfg.ZoomPhoneAccountId)
	}
	cfg.ZoomPersistAccountID()
	if err := cfg.SaveTokens("cid-B", "sec-B", "tok-B", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZOOM_PHONE_ACCOUNT_ID", "")
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ZoomPhoneAccountId != "acct-B" || reloaded.ClientID != "cid-B" {
		t.Fatalf("saved identity mixed tenants: account=%q app=%q", reloaded.ZoomPhoneAccountId, reloaded.ClientID)
	}
}
