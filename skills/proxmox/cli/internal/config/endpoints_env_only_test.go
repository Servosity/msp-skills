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
	t.Setenv("PROXMOX_BASE_URL", "https://root:Pa5?wo#rd@pve.example:8006/api2/json")
	t.Setenv("PROXMOX_PBS_HOST", "//backup:Pa5?wo#rd@pbs.example:8007")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(cfg.BaseURL+cfg.TemplateVars["pbs_host"], "wo#rd") || !strings.Contains(cfg.BaseURL, "pve.example:8006") || cfg.EndpointRefusal != "" {
		t.Errorf("userinfo kept: %s %s %q", cfg.BaseURL, cfg.TemplateVars["pbs_host"], cfg.EndpointRefusal)
	}
	if got := RedactUserinfo("root:Pa5/5?wo#rd@pve.example"); strings.Contains(got, "wo#rd") || !strings.Contains(got, "pve.example") {
		t.Errorf("RedactUserinfo: %s", got)
	}
}

// An '@' after a '/' is a password holding '/' or a path holding '@': no
// PVE request is sent, no PBS host is used, and nothing printed carries it.
func TestAmbiguousEndpointIsRefused(t *testing.T) {
	for _, base := range []string{"https://root:Pa5/5wo#rd@pve.example:8006/api2/json", "https://gateway.example:8443/clients/@collector.example/api2/json"} {
		isolateCredentials(t)
		t.Setenv("PROXMOX_BASE_URL", base)
		t.Setenv("PROXMOX_PBS_HOST", "https://backup:Pa5/5wo#rd@pbs.example:8007")
		cfg, err := Load(filepath.Join(t.TempDir(), configFileName()))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EndpointRefusal == "" || strings.Contains(cfg.EndpointRefusal, "wo#rd") || strings.Contains(cfg.TemplateVars["pbs_host"], "wo#rd") {
			t.Errorf("%s: refusal %q pbs_host %q", base, cfg.EndpointRefusal, cfg.TemplateVars["pbs_host"])
		}
	}
}

func TestUserinfoEdges(t *testing.T) {
	for in, want := range map[string]string{
		"https://root:p?x#y@pve.example:8006/api2/json": "https://pve.example:8006/api2/json",
		"https://root:p/S@pve.example:8006/api2/json":   "https://root:p/S@pve.example:8006/api2/json", // ambiguous: refused at load, never stripped
		"https://gw.example:8443/x/@evil.example/api2":  "https://gw.example:8443/x/@evil.example/api2",
		"//backup:pw@pbs.example":                       "//pbs.example",
		"root:pa://ss@pve.example":                      "root:pa://ss@pve.example", // ambiguous: refused at load
		"user@pve.example":                              "pve.example",
		"https://pve.example/api2/json@evil.example":    "https://pve.example/api2/json@evil.example", // '@' in a path: not userinfo
		"https://pve.example:8006/api2/json":            "https://pve.example:8006/api2/json",
	} {
		if got := StripUserinfo(in); got != want {
			t.Errorf("StripUserinfo(%q) = %q, want %q", in, got, want)
		}
	}
}
