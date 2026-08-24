// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"servosity-msp-pp-cli/internal/config"
	"servosity-msp-pp-cli/internal/platform"
)

func TestNewPurgesSensitiveResponseCacheOnce(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SERVOSITY_MSP_CACHE_DIR", cacheDir)
	t.Setenv("XDG_CACHE_HOME", "")
	httpCacheDir := filepath.Join(cacheDir, "http")
	sensitiveDir := filepath.Join(httpCacheDir, "resources", "current-user")
	if err := os.MkdirAll(sensitiveDir, 0o700); err != nil {
		t.Fatalf("create sensitive cache namespace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sensitiveDir, "response.json"), []byte(`{"token":"fixture-secret"}`), 0o600); err != nil {
		t.Fatalf("seed sensitive cache: %v", err)
	}
	safeFile := filepath.Join(httpCacheDir, "resources", "issues", "response.json")
	if err := os.MkdirAll(filepath.Dir(safeFile), 0o700); err != nil {
		t.Fatalf("create safe cache namespace: %v", err)
	}
	if err := os.WriteFile(safeFile, []byte(`{"id":1}`), 0o600); err != nil {
		t.Fatalf("seed safe cache: %v", err)
	}

	New(&config.Config{}, time.Second, 0)

	if _, err := os.Stat(sensitiveDir); !os.IsNotExist(err) {
		t.Fatalf("sensitive cache namespace still exists: %v", err)
	}
	if _, err := os.Stat(safeFile); err != nil {
		t.Fatalf("unrelated cache was removed: %v", err)
	}
	legitimateFile := filepath.Join(sensitiveDir, "ordinary-current-user.json")
	if err := os.MkdirAll(sensitiveDir, 0o700); err != nil {
		t.Fatalf("recreate current-user namespace: %v", err)
	}
	if err := os.WriteFile(legitimateFile, []byte(`{"id":"user"}`), 0o600); err != nil {
		t.Fatalf("seed post-upgrade cache: %v", err)
	}
	New(&config.Config{}, time.Second, 0)
	if _, err := os.Stat(legitimateFile); err != nil {
		t.Fatalf("one-time cleanup removed post-upgrade cache: %v", err)
	}
}

func TestBindPlatformSessionPurgesSensitiveResponseCacheOnce(t *testing.T) {
	cacheDir := t.TempDir()
	sensitiveDir := filepath.Join(cacheDir, "resources", "current-user")
	if err := os.MkdirAll(sensitiveDir, 0o700); err != nil {
		t.Fatalf("create profile cache namespace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sensitiveDir, "response.json"), []byte(`{"token":"fixture-secret"}`), 0o600); err != nil {
		t.Fatalf("seed profile cache: %v", err)
	}
	c := &Client{}
	session := &platform.Session{Paths: platform.Paths{CacheDir: cacheDir}, GateOutcome: platform.GateVerified}
	if err := c.BindPlatformSession(session); err != nil {
		t.Fatalf("bind profile: %v", err)
	}
	if _, err := os.Stat(sensitiveDir); !os.IsNotExist(err) {
		t.Fatalf("profile cache namespace still exists: %v", err)
	}
	legitimateFile := filepath.Join(sensitiveDir, "ordinary-current-user.json")
	if err := os.MkdirAll(sensitiveDir, 0o700); err != nil {
		t.Fatalf("recreate profile namespace: %v", err)
	}
	if err := os.WriteFile(legitimateFile, []byte(`{"id":"user"}`), 0o600); err != nil {
		t.Fatalf("seed post-upgrade profile cache: %v", err)
	}
	if err := c.BindPlatformSession(session); err != nil {
		t.Fatalf("rebind profile: %v", err)
	}
	if _, err := os.Stat(legitimateFile); err != nil {
		t.Fatalf("one-time profile cleanup removed post-upgrade cache: %v", err)
	}
}
