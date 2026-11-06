// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"vsphere-pp-cli/internal/cliutil/testenv"
)

// saveTestHome isolates the user directories, points VSPHERE_HOME at a fresh
// home (Codex's relocated home) and clears every credential and routing
// variable, so each case starts from a clean configuration.
func saveTestHome(t *testing.T) string {
	t.Helper()
	testenv.Isolate(t)
	home := t.TempDir()
	t.Setenv("VSPHERE_HOME", home)
	for _, name := range []string{"VSPHERE_BASE_URL", "VSPHERE_USERNAME", "VSPHERE_PASSWORD", "VSPHERE_CA_CERT", "VSPHERE_TLS_SERVER_NAME"} {
		t.Setenv(name, "")
	}
	return home
}

func readIfExists(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

// Class file-sink-bypasses-chokepoint (Codex r1): every byte the
// configuration writes to a file other than the 0600 credentials store passes
// one fail-closed check. A persisted URL loses its userinfo and its
// credential-named query values, credential-named headers are dropped, and
// when the marshalled bytes would still repeat any held value (or carry a URL
// credential) the save is refused and the config file is not written.
func TestSaveKeepsCredentialsOutOfTheConfigFile(t *testing.T) {
	const login = "LoginPw-A8s4"
	t.Run("codex: base_url userinfo password", func(t *testing.T) {
		saveTestHome(t)
		t.Setenv("VSPHERE_BASE_URL", "https://ro:UrlOnlyPw-Q7n9@vc.example")
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveCredentials("readonly@vsphere.local", login); err != nil {
			t.Fatalf("SaveCredentials: %v", err)
		}
		got, ok := readIfExists(t, cfg.Path)
		if !ok {
			t.Fatalf("config file %s was not written", cfg.Path)
		}
		for _, secret := range []string{"UrlOnlyPw-Q7n9", login, "ro:"} {
			if strings.Contains(got, secret) {
				t.Errorf("config.toml carries %q:\n%s", secret, got)
			}
		}
		if !strings.Contains(got, "https://vc.example") {
			t.Errorf("config.toml lost the host:\n%s", got)
		}
	})

	t.Run("percent-encoded userinfo and credential-named query values", func(t *testing.T) {
		saveTestHome(t)
		t.Setenv("VSPHERE_BASE_URL", "https://ro:Url%2FPw-Q7n9@vc.example/?api_token=QueryTok-Z4k8&site=lab&Session-Id=SessQ-7n9x")
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveCredentials("readonly@vsphere.local", login); err != nil {
			t.Fatalf("SaveCredentials: %v", err)
		}
		got, _ := readIfExists(t, cfg.Path)
		for _, secret := range []string{"Url%2FPw-Q7n9", "Url/Pw-Q7n9", "QueryTok-Z4k8", "SessQ-7n9x", login} {
			if strings.Contains(got, secret) {
				t.Errorf("config.toml carries %q:\n%s", secret, got)
			}
		}
		if !strings.Contains(got, "site=lab") {
			t.Errorf("config.toml lost a non-credential query parameter:\n%s", got)
		}
	})

	t.Run("credential-named headers are dropped", func(t *testing.T) {
		home := saveTestHome(t)
		cfgPath := filepath.Join(home, "config", "config.toml")
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
			t.Fatal(err)
		}
		basic := base64.StdEncoding.EncodeToString([]byte("ro:HeaderPw-Q7n9"))
		body := "base_url = \"https://vc.example\"\n[headers]\nAuthorization = \"Basic " + basic + "\"\nvmware-api-session-id = \"SessHdr-Q7n9xx\"\nCookie = \"vmware_soap_session=CookieQ-7n9x\"\n"
		if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.SaveCredentials("readonly@vsphere.local", login); err != nil {
			t.Fatalf("SaveCredentials: %v", err)
		}
		got, _ := readIfExists(t, cfg.Path)
		for _, secret := range []string{basic, "SessHdr-Q7n9xx", "CookieQ-7n9x", login} {
			if strings.Contains(got, secret) {
				t.Errorf("config.toml carries %q:\n%s", secret, got)
			}
		}
	})

	t.Run("logout writes no URL password either", func(t *testing.T) {
		saveTestHome(t)
		t.Setenv("VSPHERE_BASE_URL", "https://ro:LogoutUrlPw-Q7n9@vc.example")
		cfg, err := Load("")
		if err != nil {
			t.Fatal(err)
		}
		if err := cfg.ClearTokens(); err != nil {
			t.Fatalf("ClearTokens: %v", err)
		}
		if got, _ := readIfExists(t, cfg.Path); strings.Contains(got, "LogoutUrlPw-Q7n9") {
			t.Fatalf("config.toml carries the URL password:\n%s", got)
		}
	})
}

// The legacy config is rewritten when its credentials move: that rewrite is a
// non-store file writer too, so it keeps no URL password, no credential field
// and no credential-named header.
func TestLegacyScrubKeepsCredentialsOutOfTheLegacyFile(t *testing.T) {
	home := testenv.Isolate(t)
	for _, name := range []string{"VSPHERE_BASE_URL", "VSPHERE_USERNAME", "VSPHERE_PASSWORD", "VSPHERE_CA_CERT", "VSPHERE_TLS_SERVER_NAME"} {
		t.Setenv(name, "")
	}
	// The primary config lives in a relocated config dir, so Load falls back
	// to the legacy path and the save scrubs it.
	t.Setenv("VSPHERE_CONFIG_DIR", filepath.Join(home, "relocated-config"))
	legacy, err := LegacyConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(legacy, home) {
		t.Fatalf("legacy path %s is outside the sandbox", legacy)
	}
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	body := "base_url = \"https://ro:LegacyUrlPw-Q7n9@vc.example/?token=LegacyTok-Q7n9\"\nusername = \"ro@vsphere.local\"\npassword = \"LegacyPw-Q7n9x\"\n[headers]\nAuthorization = \"Bearer LegacyHdr-Q7n9\"\n"
	if err := os.WriteFile(legacy, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Path == legacy {
		t.Skip("this layout reads the legacy path as the primary config")
	}
	if err := cfg.SaveCredentials("ro@vsphere.local", "NewLoginPw-A8s4"); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	for _, path := range []string{legacy, cfg.Path} {
		got, _ := readIfExists(t, path)
		for _, secret := range []string{"LegacyUrlPw-Q7n9", "LegacyTok-Q7n9", "LegacyPw-Q7n9x", "LegacyHdr-Q7n9", "NewLoginPw-A8s4"} {
			if strings.Contains(got, secret) {
				t.Errorf("%s carries %q:\n%s", path, secret, got)
			}
		}
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(cfg.Path); err == nil && info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v, want 0600", cfg.Path, info.Mode().Perm())
		}
	}
}
