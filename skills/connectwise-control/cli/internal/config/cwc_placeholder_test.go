// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): handfixes.json no-credential-to-placeholder-host.

package config

import (
	"os"
	"strings"
	"testing"

	"connectwise-control-pp-cli/internal/cliutil"
)

// Fixture credentials: random, and sharing no 8-character piece with the
// connector's program text.
const (
	placeholderCfgUser = "OX9CDjlmCkCC5eb74y"
	placeholderCfgPass = "VEdUe4ciJhEknKlR7u"
)

const placeholderHostForTest = "your-instance.screenconnect.com"

func readFileOrEmpty(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

// auth set-credentials (SaveCredentials) with no base URL configured, or
// with the placeholder given in any spelling, writes the placeholder neither as
// config.toml's base_url nor as the server the credential was saved for. The
// next Load still sees the shipped placeholder, so doctor keeps reporting
// "not configured" and nothing is sent.
func TestSaveCredentialsNeverPersistsThePlaceholder(t *testing.T) {
	for _, env := range []string{"", "https://your-instance.screenconnect.com", "HTTPS://YOUR-INSTANCE.ScreenConnect.com.:443/"} {
		t.Run("env="+env, func(t *testing.T) {
			isolateCredentials(t)
			t.Setenv("CONNECTWISE_CONTROL_BASE_URL", env)
			cfg, err := Load("")
			if err != nil {
				t.Fatal(err)
			}
			if err := cfg.SaveCredentials(placeholderCfgUser, placeholderCfgPass); err != nil {
				t.Fatalf("SaveCredentials: %v", err)
			}
			credsPath, err := cliutil.CredentialsFilePath()
			if err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{"config.toml": readFileOrEmpty(t, cfg.Path), "credentials.toml": readFileOrEmpty(t, credsPath)} {
				if strings.Contains(strings.ToLower(body), placeholderHostForTest) {
					t.Errorf("%s records the shipped placeholder:\n%s", name, body)
				}
			}
			if !strings.Contains(readFileOrEmpty(t, credsPath), placeholderCfgPass) {
				t.Fatalf("the credential was not saved at all (%s)", credsPath)
			}
			t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "")
			again, err := Load("")
			if err != nil {
				t.Fatal(err)
			}
			if again.BaseURL != "https://your-instance.screenconnect.com" {
				t.Errorf("after the save, Load's base URL = %q; want the built-in placeholder, so doctor still reports not configured", again.BaseURL)
			}
		})
	}
}

// A real instance is still persisted: the fix drops only the placeholder.
func TestSaveCredentialsKeepsARealBaseURL(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "https://cwc-fixture-tenant.screenconnect.com")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredentials(placeholderCfgUser, placeholderCfgPass); err != nil {
		t.Fatal(err)
	}
	if body := readFileOrEmpty(t, cfg.Path); !strings.Contains(body, "https://cwc-fixture-tenant.screenconnect.com") {
		t.Fatalf("config.toml lost the configured base URL:\n%s", body)
	}
}

// The built-in base URL is a placeholder, so it is no origin a stored
// credential was issued for: an explicit --config whose server is the
// placeholder does not pick up global credentials that recorded no server.
func TestBuiltinPlaceholderIsNoTrustedCredentialOrigin(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("CONNECTWISE_CONTROL_BASE_URL", "")
	if origin, ok := usableBuiltinOrigin(); ok {
		t.Fatalf("usableBuiltinOrigin() = %q, true; the shipped placeholder is no usable origin", origin)
	}
	if origin, ok := (&Config{}).trustedGlobalCredentialOrigin(); ok {
		t.Fatalf("trustedGlobalCredentialOrigin() = %q, true with no default config; want no trusted origin", origin)
	}
	c := &Config{BaseURL: "https://your-instance.screenconnect.com"}
	if c.allowsGlobalCredential(&cliutil.Credentials{ConnectwiseControlUsername: placeholderCfgUser, ConnectwiseControlPassword: placeholderCfgPass}) {
		t.Fatal("global credentials with no recorded server were allowed to the shipped placeholder")
	}
	if c.allowsGlobalCredential(&cliutil.Credentials{BaseURL: "https://YOUR-INSTANCE.screenconnect.com", ConnectwiseControlUsername: placeholderCfgUser, ConnectwiseControlPassword: placeholderCfgPass}) {
		t.Fatal("global credentials recorded for the placeholder were allowed to the placeholder")
	}
}
