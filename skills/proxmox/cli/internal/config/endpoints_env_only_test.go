// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: an endpoint from one run's environment is never saved, and
// PROXMOX_HOST beats a base_url in config.toml (a token must reach its own host).

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvEndpointIsNeverSaved(t *testing.T) {
	isolateCredentials(t)
	p := filepath.Join(t.TempDir(), configFileName())
	t.Setenv("PROXMOX_BASE_URL", "https://client-a.example:8006/api2/json")
	t.Setenv("PROXMOX_HOST", "client-a.example")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveCredential("a@pve!t=00000000-0000-0000-0000-00000000000a"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "client-a.example") {
		t.Errorf("an env endpoint was saved:\n%s", b)
	}
}

func TestHostEnvMustAgreeWithSavedBaseURL(t *testing.T) {
	for _, tc := range []struct {
		hostEnv string
		refuse  bool
	}{
		{"client-b.example", true},
		{"client-a.example", true},      // port 8006, not the saved 443
		{"client-a.example:8443", true}, // another port on the same host
		{"client-a.example:443", false},
		{"https://CLIENT-A.example.:443", false},
	} {
		isolateCredentials(t)
		p := filepath.Join(t.TempDir(), configFileName())
		const saved = "https://client-a.example:443/api2/json" // a reverse proxy on 443
		if err := os.WriteFile(p, []byte("base_url = \""+saved+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		lockOwnerOnly(t, p)
		t.Setenv("PROXMOX_BASE_URL", "")
		t.Setenv("PROXMOX_HOST", tc.hostEnv)
		cfg, err := Load(p)
		if err != nil {
			t.Fatal(err)
		}
		if refused := cfg.EndpointRefusal != ""; refused != tc.refuse || cfg.BaseURL != saved {
			t.Errorf("PROXMOX_HOST=%s with base_url %s: refused %v (%q), want %v", tc.hostEnv, saved, refused, cfg.EndpointRefusal, tc.refuse)
		}
	}
}

// A user:password@ part of an endpoint is dropped at load: it is never sent
// or printed.
func TestEndpointUserinfoIsDropped(t *testing.T) {
	isolateCredentials(t)
	p := filepath.Join(t.TempDir(), configFileName())
	t.Setenv("PROXMOX_BASE_URL", "https://root:Pa5/5?wo#rd@pve.example:8006/api2/json")
	t.Setenv("PROXMOX_PBS_HOST", "//backup:Pa5/5?wo#rd@pbs.example:8007")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.BaseURL+cfg.TemplateVars["pbs_host"], "wo#rd") || !strings.Contains(cfg.BaseURL, "pve.example:8006") {
		t.Errorf("userinfo kept: %s %s", cfg.BaseURL, cfg.TemplateVars["pbs_host"])
	}
	if got := RedactUserinfo("root:Pa5/5?wo#rd@pve.example"); strings.Contains(got, "wo#rd") || !strings.Contains(got, "pve.example") {
		t.Errorf("RedactUserinfo: %s", got)
	}
}
