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

func TestHostEnvBeatsSavedBaseURL(t *testing.T) {
	isolateCredentials(t)
	p := filepath.Join(t.TempDir(), configFileName())
	if err := os.WriteFile(p, []byte("base_url = \"https://client-a.example:8006/api2/json\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockOwnerOnly(t, p)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "client-b.example")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.BaseURL, "{host}") || cfg.TemplateVars["host"] != "client-b.example" {
		t.Errorf("PROXMOX_HOST=client-b.example but requests go to %q (host %q)", cfg.BaseURL, cfg.TemplateVars["host"])
	}
}
