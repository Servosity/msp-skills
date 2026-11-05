// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kaseya-vsa-pp-cli/internal/cliutil"
)

// TestSave_RefusesConfigLoadedFromLooseFile: a config other users can change
// is dropped in memory, but saving it would rewrite the file owner-only and the
// next load would trust its base_url and template_vars. Every saver (auth
// logout, auth set-credentials, token and single-credential saves) must refuse,
// name the chmod fix, and leave the file and the credentials store unchanged.
func TestSave_RefusesConfigLoadedFromLooseFile(t *testing.T) {
	savers := map[string]func(*Config) error{
		"ClearTokens":     func(c *Config) error { return c.ClearTokens() },
		"SaveCredentials": func(c *Config) error { return c.SaveCredentials("new-id", "new-secret") },
		"SaveCredential":  func(c *Config) error { return c.SaveCredential("new-id") },
		"SaveTokens": func(c *Config) error {
			return c.SaveTokens("cid", "csec", "at", "rt", time.Now().Add(time.Hour))
		},
	}
	for name, save := range savers {
		t.Run(name, func(t *testing.T) {
			isolateCredentials(t)
			t.Setenv("KASEYA_VSA_SERVER", "acme.vsax.net")
			t.Setenv("KASEYA_VSA_TOKEN_ID", "env-token-id")
			t.Setenv("KASEYA_VSA_TOKEN_SECRET", "env-token-secret")
			p := filepath.Join(t.TempDir(), configFileName())
			content := []byte("base_url = \"https://evil.vsax.net/api/v3\"\n[template_vars]\nserver = \"evil.vsax.net\"\n")
			if err := os.WriteFile(p, content, 0o600); err != nil {
				t.Fatal(err)
			}
			grantWorld(t, p)
			before, _ := os.Stat(p)
			credsPath, err := cliutil.CredentialsFilePath()
			if err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(p)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			err = save(cfg)
			if err == nil || !strings.Contains(err.Error(), "chmod 600 "+p) {
				t.Fatalf("save of a loose config: err=%v, want a refusal naming chmod 600 %s", err, p)
			}
			after, _ := os.ReadFile(p)
			if !bytes.Equal(after, content) {
				t.Fatalf("loose config rewritten:\n%s", after)
			}
			if st, _ := os.Stat(p); runtime.GOOS != "windows" && st.Mode().Perm() != before.Mode().Perm() {
				t.Fatalf("loose config mode changed %v -> %v", before.Mode().Perm(), st.Mode().Perm())
			}
			if _, err := os.Stat(credsPath); !os.IsNotExist(err) {
				t.Fatalf("credentials store written despite the refusal: %v", err)
			}

			cfg, err = Load(p)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			if cfg.hasCredentialFields() || !cfg.CredentialRouteBlocked() {
				t.Fatal("credential attached to the loose base_url after a save attempt")
			}
		})
	}
}
