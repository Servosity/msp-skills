// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"servosity-msp-pp-cli/internal/cliutil"
	"servosity-msp-pp-cli/internal/cliutil/testenv"
)

// TestAuthHeader_TokenScheme guards the fix for issue #78: the Servosity API
// authenticates the MSP partner token via Django REST Framework's
// TokenAuthentication, which requires the "Token " scheme on the Authorization
// header. A bare token value is rejected with HTTP 403. AuthHeader() must
// prepend the scheme, while tolerating a value that already carries a
// recognized scheme (the historical SERVOSITY_MSP_TOKEN="Token <t>" workaround).
func TestAuthHeader_TokenScheme(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "bare token gets the Token scheme",
			cfg:  Config{ServosityMspToken: "abc123"},
			want: "Token abc123",
		},
		{
			name: "value already carrying Token scheme is not double-prefixed",
			cfg:  Config{ServosityMspToken: "Token abc123"},
			want: "Token abc123",
		},
		{
			name: "lowercase token scheme is normalized to canonical Token",
			cfg:  Config{ServosityMspToken: "token abc123"},
			want: "Token abc123",
		},
		{
			name: "mistaken Bearer scheme is normalized to the DRF Token scheme",
			cfg:  Config{ServosityMspToken: "Bearer abc123"},
			want: "Token abc123",
		},
		{
			name: "empty token yields an empty header",
			cfg:  Config{ServosityMspToken: ""},
			want: "",
		},
		{
			name: "explicit AuthHeaderVal override is returned verbatim",
			cfg:  Config{AuthHeaderVal: "Token override", ServosityMspToken: "abc123"},
			want: "Token override",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.AuthHeader(); got != tc.want {
				t.Fatalf("AuthHeader() = %q, want %q", got, tc.want)
			}
		})
	}
}

// explicitConfigHome stages an explicitly selected config file (the --config /
// SERVOSITY_MSP_CONFIG shape) with a colocated credentials store, inside a
// sandboxed home so the default store is a throwaway too. It returns the config
// path and the sibling credentials path.
func explicitConfigHome(t *testing.T, siblingToken string) (configPath, siblingCredsPath string) {
	t.Helper()
	if restore, err := cliutil.SetHomeOverride(""); err == nil {
		t.Cleanup(restore)
	} else {
		t.Fatalf("reset home override: %v", err)
	}
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("SERVOSITY_MSP_TOKEN", "")

	dir := filepath.Join(home, "explicit")
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
		t.Fatalf("mkdir explicit config home: %v", err)
	}
	configPath = filepath.Join(dir, "config.toml")
	if err := os.WriteFile(configPath, []byte("base_url = \"https://explicit.example\"\n"), 0o600); err != nil {
		t.Fatalf("write explicit config: %v", err)
	}
	siblingCredsPath = filepath.Join(dir, "data", "credentials.toml")
	if siblingToken != "" {
		body := []byte("msp_token = \"" + siblingToken + "\"\n")
		if err := os.WriteFile(siblingCredsPath, body, 0o600); err != nil {
			t.Fatalf("write sibling credentials: %v", err)
		}
	}
	return configPath, siblingCredsPath
}

func siblingTokenOnDisk(t *testing.T, path string) string {
	t.Helper()
	creds, ok, err := cliutil.LoadCredentialsForConfig(filepath.Join(filepath.Dir(filepath.Dir(path)), "config.toml"))
	if err != nil {
		t.Fatalf("load sibling credentials: %v", err)
	}
	if !ok || creds == nil {
		return ""
	}
	return creds.ServosityMspToken
}

// TestClearTokensWithExplicitConfigClearsSiblingStore guards the `auth logout`
// half of the config-path write asymmetry. Load() reads the credentials store
// colocated with an explicitly selected config file, so a logout that only
// removed the default store reported "Logged out. Credentials cleared." while
// the sibling token stayed live and kept authenticating every later call.
func TestClearTokensWithExplicitConfigClearsSiblingStore(t *testing.T) {
	configPath, siblingCredsPath := explicitConfigHome(t, "sibling-secret")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ServosityMspToken != "sibling-secret" {
		t.Fatalf("Load() token = %q, want the sibling store's token", cfg.ServosityMspToken)
	}

	if err := cfg.ClearTokens(); err != nil {
		t.Fatalf("ClearTokens() error = %v", err)
	}

	if got := siblingTokenOnDisk(t, siblingCredsPath); got != "" {
		t.Fatalf("sibling credentials still hold %q after ClearTokens(); logout reported success but the token stays usable", got)
	}
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("reload after ClearTokens() error = %v", err)
	}
	if reloaded.ServosityMspToken != "" {
		t.Fatalf("reload after ClearTokens() token = %q, want empty", reloaded.ServosityMspToken)
	}
	if reloaded.AuthHeader() != "" {
		t.Fatalf("reload after ClearTokens() AuthHeader() = %q, want empty", reloaded.AuthHeader())
	}
}

// TestSaveCredentialWithExplicitConfigWritesSiblingStore guards the `auth
// set-token` half, which is the more dangerous one: a rotation that wrote the
// new token to the default store left the sibling store holding the OLD token,
// and the sibling wins on read, so the CLI silently kept using the credential
// the operator believed they had replaced.
func TestSaveCredentialWithExplicitConfigWritesSiblingStore(t *testing.T) {
	configPath, siblingCredsPath := explicitConfigHome(t, "old-secret")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.SaveCredential("new-secret"); err != nil {
		t.Fatalf("SaveCredential() error = %v", err)
	}

	if got := siblingTokenOnDisk(t, siblingCredsPath); got != "new-secret" {
		t.Fatalf("sibling credentials hold %q after SaveCredential(); want the rotated token", got)
	}
	reloaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("reload after SaveCredential() error = %v", err)
	}
	if reloaded.ServosityMspToken != "new-secret" {
		t.Fatalf("reload after SaveCredential() token = %q, want %q", reloaded.ServosityMspToken, "new-secret")
	}
}

// TestCredentialsPathFollowsExplicitConfig pins the path `auth set-token`
// prints back. Reporting the default store while writing the sibling (or the
// reverse) sends the operator to the wrong file when they audit or revoke.
func TestCredentialsPathFollowsExplicitConfig(t *testing.T) {
	configPath, siblingCredsPath := explicitConfigHome(t, "")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got, err := cfg.CredentialsPath()
	if err != nil {
		t.Fatalf("CredentialsPath() error = %v", err)
	}
	wantDir, err := filepath.EvalSymlinks(filepath.Dir(siblingCredsPath))
	if err != nil {
		t.Fatalf("resolve sibling dir: %v", err)
	}
	gotDir, err := filepath.EvalSymlinks(filepath.Dir(got))
	if err != nil {
		t.Fatalf("resolve reported dir: %v", err)
	}
	if gotDir != wantDir || filepath.Base(got) != "credentials.toml" {
		t.Fatalf("CredentialsPath() = %q, want the store colocated with %q", got, configPath)
	}

	defaultCfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	defaultPath, err := defaultCfg.CredentialsPath()
	if err != nil {
		t.Fatalf("default CredentialsPath() error = %v", err)
	}
	wantDefault, err := cliutil.CredentialsFilePath()
	if err != nil {
		t.Fatalf("cliutil.CredentialsFilePath() error = %v", err)
	}
	if defaultPath != wantDefault {
		t.Fatalf("default CredentialsPath() = %q, want %q", defaultPath, wantDefault)
	}
}
