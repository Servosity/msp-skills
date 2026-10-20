// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/secrets"
	"github.com/pelletier/go-toml/v2"
)

// TestCredentialEnvIsRegisteredWhenRead (handfixes.json:
// secret-registry-masks-every-boundary): a credential environment variable
// is a secret from the moment EnvOverride reads it; other variables are not.
func TestCredentialEnvIsRegisteredWhenRead(t *testing.T) {
	const httpToken = "mcp-http-token-read-6G8H0J"
	const region = "region-like-value-not-secret"
	t.Setenv("ESET_PROTECT_MCP_HTTP_TOKEN", httpToken)
	t.Setenv("ESET_PROTECT_REGION", region)
	if got := cliutil.EnvOverride("ESET_PROTECT_MCP_HTTP_TOKEN"); got != httpToken {
		t.Fatalf("EnvOverride = %q", got)
	}
	_ = cliutil.EnvOverride("ESET_PROTECT_REGION")
	if got := secrets.Mask("caller sent " + httpToken); strings.Contains(got, "6G8H0J") {
		t.Fatalf("the MCP HTTP token was not registered when read: %q", got)
	}
	if got := secrets.Mask("region " + region); got != "region "+region {
		t.Fatalf("a non-credential variable was registered: %q", got)
	}
}

// TestLoadedCredentialsAreRegistered: every value of the credentials file is
// a secret from the moment it is read.
func TestLoadedCredentialsAreRegistered(t *testing.T) {
	resetCredentialEnv(t)
	creds := &cliutil.Credentials{AccessToken: "cached-access-2K4M6N", RefreshToken: "cached-refresh-8P0R2T", EsetProtectPassword: "cached-password-4V6X8Z"}
	data, err := toml.Marshal(creds)
	if err != nil {
		t.Fatal(err)
	}
	path, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := cliutil.LoadCredentials(); err != nil || !ok {
		t.Fatalf("LoadCredentials: ok=%v err=%v", ok, err)
	}
	got := secrets.Mask("cached-access-2K4M6N cached-refresh-8P0R2T cached-password-4V6X8Z")
	for _, tail := range []string{"2K4M6N", "8P0R2T", "4V6X8Z"} {
		if strings.Contains(got, tail) {
			t.Fatalf("a cached credential was not registered when loaded: %q", got)
		}
	}
}
