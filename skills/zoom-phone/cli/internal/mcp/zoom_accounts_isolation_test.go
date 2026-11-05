// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"zoom-phone-pp-cli/internal/cliutil/testenv"
)

func zoomAcctWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Codex round 6: the MCP server refuses to start (exit 10) when the selected
// account's data directory is a link into another account; a clean selection
// starts.
func TestZoomAcctMCPStartupIsolation(t *testing.T) {
	home := testenv.Isolate(t)
	for _, k := range []string{"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL"} {
		t.Setenv(k, "")
	}
	root := filepath.Join(home, ".config", "zoom-phone-cli", "accounts")
	acme, globex := filepath.Join(root, "acme"), filepath.Join(root, "globex")
	zoomAcctWrite(t, filepath.Join(acme, "config.toml"), "client_id = 'CID_A'\nclient_secret = 'acme-secret'\nphone_account_id = 'ACCT_A'\n")
	zoomAcctWrite(t, filepath.Join(globex, "config.toml"), "client_id = 'CID_B'\nclient_secret = 'globex-secret'\nphone_account_id = 'ACCT_B'\n")
	zoomAcctWrite(t, filepath.Join(globex, "data", "credentials.toml"), "access_token = 'tok-globex'\n")
	t.Setenv("ZOOM_PHONE_HOME", acme)
	t.Setenv("ZOOM_PHONE_CONFIG", filepath.Join(acme, "config.toml"))

	exitCode := -1
	prev := zoomAcctIsolationExit
	zoomAcctIsolationExit = func(code int) { exitCode = code }
	t.Cleanup(func() { zoomAcctIsolationExit = prev })

	if err := os.Symlink(filepath.Join(globex, "data"), filepath.Join(acme, "data")); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if zoomAcctEnforceIsolation(&stderr) || exitCode != 10 || !strings.Contains(stderr.String(), "is a symlink") {
		t.Fatalf("tampered acme: served=true or exit=%d stderr=%s", exitCode, stderr.String())
	}

	// Clean selection: the link removed, the server may start.
	if err := os.Remove(filepath.Join(acme, "data")); err != nil {
		t.Fatal(err)
	}
	exitCode, stderr = -1, bytes.Buffer{}
	if !zoomAcctEnforceIsolation(&stderr) || exitCode != -1 {
		t.Fatalf("clean acme refused: exit=%d stderr=%s", exitCode, stderr.String())
	}
}

// Codex round 7: a hard-linked database from another account also stops the
// MCP server at startup.
func TestZoomAcctMCPStartupRefusesHardLinkedDB(t *testing.T) {
	home := testenv.Isolate(t)
	for _, k := range []string{"ZOOM_PHONE_ACCOUNT_ID", "ZOOM_PHONE_CLIENT_ID", "ZOOM_PHONE_CLIENT_SECRET", "ZOOM_PHONE_BASE_URL", "ZOOM_PHONE_TOKEN_URL"} {
		t.Setenv(k, "")
	}
	root := filepath.Join(home, ".config", "zoom-phone-cli", "accounts")
	acme, globex := filepath.Join(root, "acme"), filepath.Join(root, "globex")
	zoomAcctWrite(t, filepath.Join(acme, "config.toml"), "client_id = 'CID_A'\nclient_secret = 'acme-secret'\nphone_account_id = 'ACCT_A'\n")
	zoomAcctWrite(t, filepath.Join(globex, "config.toml"), "client_id = 'CID_B'\nclient_secret = 'globex-secret'\nphone_account_id = 'ACCT_B'\n")
	zoomAcctWrite(t, filepath.Join(globex, "data", "data-x.db"), "SQLite format 3\x00globex rows")
	zoomAcctWrite(t, filepath.Join(acme, "data", "placeholder"), "")
	if err := os.Link(filepath.Join(globex, "data", "data-x.db"), filepath.Join(acme, "data", "data-x.db")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZOOM_PHONE_HOME", acme)
	t.Setenv("ZOOM_PHONE_CONFIG", filepath.Join(acme, "config.toml"))

	exitCode := -1
	prev := zoomAcctIsolationExit
	zoomAcctIsolationExit = func(code int) { exitCode = code }
	t.Cleanup(func() { zoomAcctIsolationExit = prev })
	var stderr bytes.Buffer
	if zoomAcctEnforceIsolation(&stderr) || exitCode != 10 || !strings.Contains(stderr.String(), "hard links") {
		t.Fatalf("hard-linked db: exit=%d stderr=%s", exitCode, stderr.String())
	}
	if err := os.Remove(filepath.Join(acme, "data", "data-x.db")); err != nil {
		t.Fatal(err)
	}
	exitCode, stderr = -1, bytes.Buffer{}
	if !zoomAcctEnforceIsolation(&stderr) || exitCode != -1 {
		t.Fatalf("clean acme refused: exit=%d stderr=%s", exitCode, stderr.String())
	}
}
