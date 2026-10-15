// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// Adversarial round 11.

// A PBS prune job prunes a PBS storage on its own schedule: PVE's preview
// does not model it, so no "keep" is promised.
func TestPBSPruneJobOverridesPVEPruneMarks(t *testing.T) {
	f := newFixture(t)
	f.pbsRoutes["/admin/prune"] = []map[string]any{{"id": "p1", "store": "main", "ns": "", "keep-last": 1, "schedule": "daily"}}
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatal(err)
	}
	marked := 0
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		for _, rp := range g.RestorePoints {
			if strings.HasPrefix(rp.Source, "pve-storage:pbs1") {
				marked++
				if rp.PruneMark != "pbs-prune" {
					t.Errorf("point %s on a PBS-pruned datastore marked %q", rp.TimeRFC3339, rp.PruneMark)
				}
			}
		}
	}
	if marked == 0 {
		t.Fatalf("no pve-storage:pbs1 points:\n%s", out)
	}
}

// Points no enabled job writes are not scheduled to be pruned: no preview,
// no failure.
func TestUncoveredGuestPointsAreUnmarkedQuietly(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/nodes/pve1/storage/pbs1/content"] = append(f.pveRoutes["/nodes/pve1/storage/pbs1/content"].([]any), map[string]any{"volid": "pbs1:backup/vm-103", "vmid": 103, "ctime": ago(f.now, 200), "size": 1 << 30, "subtype": "qemu", "format": "pbs-vm", "verification": map[string]any{"state": "ok", "upid": "UPID:pbs:verify"}})
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// PVE::Storage::prune_backups dies without retention to apply.
		if strings.HasSuffix(r.URL.Path, "/prunebackups") && r.URL.Query().Get("prune-backups") == "" {
			http.Error(w, `{"errors":"no prune-backups options configured for storage 'pbs1'"}`, http.StatusInternalServerError)
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(pve.Close)
	t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
	out, _, _ := runCLI(t, "restore-points", "103", "--json")
	if n := decodeJSON[restorePointsReport](t, out).Guests; len(n) == 0 || n[0].Count == 0 {
		t.Fatalf("103 needs a point for this test:\n%s", out)
	}
	for _, ff := range decodeJSON[restorePointsReport](t, out).FetchFailures {
		if strings.HasPrefix(ff.Scope, "prune preview") {
			t.Errorf("spurious prune preview failure: %+v", ff)
		}
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

// Codex round 20: a root-namespace PBS prune job prunes every namespace
// below it (recursive by default).
func TestRootPBSPruneJobCoversNamedNamespaces(t *testing.T) {
	f := newFixture(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/prune"] = []map[string]any{{"id": "p1", "store": "main", "keep-last": 1, "schedule": "daily"}}
	out, _, err := runCLI(t, "restore-points", "100", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range decodeJSON[restorePointsReport](t, out).Guests {
		for _, rp := range g.RestorePoints {
			if strings.HasPrefix(rp.Source, "pve-storage:pbs1") && rp.PruneMark != "pbs-prune" {
				t.Errorf("point %s in acme under a root prune job marked %q", rp.TimeRFC3339, rp.PruneMark)
			}
		}
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
