// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// Adversarial round 11.

// Codex round 22 / adversarial round 12: restore-points predicts no prune
// outcome. A prediction needs the whole of vzdump (backup before prune,
// per-node storages across migrations, PBS-side prune jobs, storage aliases
// of one datastore), and a wrong "keep" is a false green; protected is the
// fact it reports instead.
func TestRestorePointsPredictsNoPrune(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/prune"] = []map[string]any{{"id": "p1", "store": "main", "keep-last": 1, "schedule": "daily"}}
	var previews atomic.Int64
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/prunebackups") {
			previews.Add(1)
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(pve.Close)
	t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if g := decodeJSON[restorePointsReport](t, out).Guests; len(g) == 0 || g[0].Count == 0 {
		t.Fatalf("100 needs points for this test:\n%s", out)
	}
	if previews.Load() != 0 {
		t.Errorf("restore-points asked for %d prune previews", previews.Load())
	}
	for _, w := range []string{"prune_mark", "pbs-prune"} {
		if strings.Contains(out, w) {
			t.Errorf("output still carries %q:\n%s", w, out)
		}
	}
	if !strings.Contains(out, `"protected"`) {
		t.Errorf("protected is the reported fact:\n%s", out)
	}
}

// clusters check fails when nothing at all is configured.
func TestClustersCheckFailsWithNothingConfigured(t *testing.T) {
	newFixture(t)
	t.Setenv("PROXMOX_BASE_URL", "")
	t.Setenv("PROXMOX_HOST", "")
	t.Setenv("PROXMOX_PBS_HOST", "")
	if _, _, err := runCLI(t, "clusters", "check", "--json"); err == nil {
		t.Error("clusters check passed with nothing configured")
	}
}

// Codex round 20: every pve_host spelling validate accepts builds the PVE API
// base on port 8006 (https://pve1/ once built https://pve1/:8006/api2/json).
func TestPVEHostSpellings(t *testing.T) {
	for in, want := range map[string]string{
		"pve1.example.com":               "https://pve1.example.com:8006/api2/json",
		"https://pve1.example.com/":      "https://pve1.example.com:8006/api2/json",
		"https://pve1.example.com":       "https://pve1.example.com:8006/api2/json",
		"pve1.example.com:9006":          "https://pve1.example.com:9006/api2/json",
		"https://pve1.example.com:9006/": "https://pve1.example.com:9006/api2/json",
		"[2001:db8::1]":                  "https://[2001:db8::1]:8006/api2/json",
		"https://[2001:db8::1]:9006/":    "https://[2001:db8::1]:9006/api2/json",
	} {
		cl := clusterProfile{PVEHost: in}
		if err := cl.validate("acme"); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		c := client.New(&config.Config{}, time.Second, 0)
		if err := applyClusterToClient(c, cl); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if c.BaseURL != want {
			t.Errorf("pve_host %q: base %q, want %q", in, c.BaseURL, want)
		}
	}
}
