// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_LooseConfigCannotWeakenTLS: a config file other users can change may
// not alter how the connection carrying the token is verified, including a
// file that sets only skip_tls_verify.
func TestLoad_LooseConfigCannotWeakenTLS(t *testing.T) {
	for _, content := range []string{
		"skip_tls_verify = true\n",
		"skip_tls_verify = true\nca_cert = \"/tmp/attacker-ca.pem\"\ntls_server_name = \"attacker.example\"\n",
	} {
		isolateCredentials(t)
		clearTLSEnv(t)
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
	}
}

// TestLoad_PrivateConfigKeepsSkipTLSVerify: the owner's own private file still
// sets it.
func TestLoad_PrivateConfigKeepsSkipTLSVerify(t *testing.T) {
	isolateCredentials(t)
	clearTLSEnv(t)
	p := filepath.Join(t.TempDir(), configFileName())
	if err := os.WriteFile(p, []byte("skip_tls_verify = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockOwnerOnly(t, p)
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.SkipTLSVerify {
		t.Fatal("private config lost skip_tls_verify")
	}
}

// clearTLSEnv keeps the operator's own TLS environment out of these tests.
func clearTLSEnv(t *testing.T) {
	t.Setenv("EXCHANGE_ONLINE_CA_CERT", "")
	t.Setenv("EXCHANGE_ONLINE_TLS_SERVER_NAME", "")
}
