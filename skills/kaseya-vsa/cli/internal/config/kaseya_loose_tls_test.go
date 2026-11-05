// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestLoad_LooseConfigCannotWeakenTLS: a config file other users can change may
// not alter how the connection carrying the token is verified. Every TLS knob
// the file can set is written at once, with the operator's normal environment.
func TestLoad_LooseConfigCannotWeakenTLS(t *testing.T) {
	for _, content := range []string{
		"skip_tls_verify = true\n",
		"skip_tls_verify = true\nca_cert = \"/tmp/attacker-ca.pem\"\ntls_server_name = \"attacker.example\"\n",
	} {
		isolateCredentials(t)
		t.Setenv("KASEYA_VSA_SERVER", "acme.vsax.net")
		t.Setenv("KASEYA_VSA_TOKEN_ID", "env-token-id")
		t.Setenv("KASEYA_VSA_TOKEN_SECRET", "env-token-secret")
		p := filepath.Join(t.TempDir(), configFileName())
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		grantWorld(t, p)

		cfg, err := Load(p)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.SkipTLSVerify || cfg.CACert != "" || cfg.TLSServerName != "" {
			t.Fatalf("loose config %q changed TLS verification: skip=%v ca=%q name=%q", content, cfg.SkipTLSVerify, cfg.CACert, cfg.TLSServerName)
		}
		if cfg.KaseyaVsaTokenId != "env-token-id" {
			t.Fatalf("env credential not applied: %q", cfg.KaseyaVsaTokenId)
		}
	}
}

// TestLoad_LooseConfigCannotChooseServer: the {server} placeholder picks the
// host the token goes to, so a loose file may not set it either.
func TestLoad_LooseConfigCannotChooseServer(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("KASEYA_VSA_SERVER", "")
	t.Setenv("KASEYA_VSA_TOKEN_ID", "env-token-id")
	t.Setenv("KASEYA_VSA_TOKEN_SECRET", "env-token-secret")
	p := filepath.Join(t.TempDir(), configFileName())
	if err := os.WriteFile(p, []byte("[template_vars]\nserver = \"attacker.example\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	grantWorld(t, p)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	base := strings.ReplaceAll(cfg.BaseURL, "{server}", cfg.TemplateVars["server"])
	if strings.Contains(base, "attacker.example") || cfg.hasCredentialFields() || !cfg.CredentialRouteBlocked() {
		t.Fatalf("loose server route not blocked: base=%s creds=%v", base, cfg.hasCredentialFields())
	}

	// The operator naming the server lifts the block and wins over the file.
	t.Setenv("KASEYA_VSA_SERVER", "acme.vsax.net")
	cfg, err = Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.TemplateVars["server"] != "acme.vsax.net" || !cfg.hasCredentialFields() || cfg.CredentialRouteBlocked() {
		t.Fatalf("env server: server=%q creds=%v", cfg.TemplateVars["server"], cfg.hasCredentialFields())
	}
}

// TestLoad_EnvServerDoesNotLiftLooseBaseURL: KASEYA_VSA_SERVER lifts only the
// {server} block, never a loose base_url that names another host outright.
func TestLoad_EnvServerDoesNotLiftLooseBaseURL(t *testing.T) {
	isolateCredentials(t)
	t.Setenv("KASEYA_VSA_SERVER", "acme.vsax.net")
	t.Setenv("KASEYA_VSA_TOKEN_ID", "env-token-id")
	t.Setenv("KASEYA_VSA_TOKEN_SECRET", "env-token-secret")
	p := filepath.Join(t.TempDir(), configFileName())
	writePermsConfig(t, p, "https://attacker.example/collect", "", "")
	grantWorld(t, p)

	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.hasCredentialFields() || !cfg.CredentialRouteBlocked() {
		t.Fatal("env credential attached to a loose base_url once KASEYA_VSA_SERVER was set")
	}
}

// TestConfigFieldsReviewedForLooseFiles fails when Config gains a persisted
// field, so whoever adds it decides whether a loose file may set it (a TLS,
// proxy or routing field must be dropped in acceptOpenedConfig).
func TestConfigFieldsReviewedForLooseFiles(t *testing.T) {
	reviewed := map[string]bool{
		"base_url": true, "skip_tls_verify": true, "ca_cert": true, "tls_server_name": true,
		"auth_header": true, "headers": true, "access_token": true, "refresh_token": true,
		"token_expiry": true, "client_id": true, "client_secret": true, "vsa_token_id": true,
		"vsa_token_secret": true, "template_vars": true,
	}
	ty := reflect.TypeOf(Config{})
	for i := 0; i < ty.NumField(); i++ {
		tag := strings.Split(ty.Field(i).Tag.Get("toml"), ",")[0]
		if tag == "" || tag == "-" || reviewed[tag] {
			continue
		}
		t.Errorf("Config field %s (toml %q) is not reviewed for loose config files", ty.Field(i).Name, tag)
	}
}
