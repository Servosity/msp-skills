// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/config"
)

// Adversarial round 7.

// A storage restricted to other nodes lists as enabled=0 on this one: it is
// read on its own node, not disabled. One disabled in the config is.
func TestNodeRestrictedStorageIsNotDisabled(t *testing.T) {
	for name, c := range map[string]struct {
		cfg          map[string]any
		wantDisabled bool
	}{
		"restricted to pve1": {map[string]any{"storage": "usb", "type": "dir", "content": "backup", "nodes": "pve1"}, false},
		"disabled":           {map[string]any{"storage": "usb", "type": "dir", "content": "backup", "disable": 1}, true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.pveRoutes["/nodes"] = []map[string]any{{"node": "pve1", "status": "online"}, {"node": "pve2", "status": "online"}}
			st := f.pveRoutes["/nodes/pve1/storage"].([]map[string]any)
			usb := map[string]any{"storage": "usb", "type": "dir", "shared": 0, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 1, "avail": 999}
			if c.wantDisabled {
				usb = map[string]any{"storage": "usb", "type": "dir", "shared": 0, "active": 0, "enabled": 0, "content": "backup"}
			}
			f.pveRoutes["/nodes/pve1/storage"] = append(append([]map[string]any{}, st...), usb)
			f.pveRoutes["/nodes/pve1/storage/usb/content"] = []any{}
			f.pveRoutes["/nodes/pve2/storage"] = []map[string]any{{"storage": "pbs1", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup"},
				{"storage": "usb", "type": "dir", "shared": 0, "active": 0, "enabled": 0, "content": "backup"}}
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
					port, _ := strconv.Atoi(hp[1])
					list := []map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "namespace": f.pbsNS, "fingerprint": f.pbsFP}}
					if r.URL.Query().Get("type") == "" {
						list = append(list, c.cfg)
					}
					writeData(w, list)
					return
				}
				f.pve.Config.Handler.ServeHTTP(w, r)
			}))
			t.Cleanup(pve.Close)
			t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
			t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
			rep, _ := postureByVMID(t)
			disabled := false
			for _, ff := range rep.FetchFailures {
				disabled = disabled || (strings.HasPrefix(ff.Scope, "storage usb") && strings.Contains(ff.Error, "disabled"))
			}
			if disabled != c.wantDisabled {
				t.Errorf("usb reported disabled = %v, want %v: %+v", disabled, c.wantDisabled, rep.FetchFailures)
			}
		})
	}
}

// An unreadable entry that names no PBS may still write to any PBS; its
// declared pbs_namespace scopes it, so the recommended setup stays fresh.
func TestIdentitylessDownPeerMayWriteAnyPBS(t *testing.T) {
	for name, c := range map[string]struct {
		decl      string
		wantFresh bool
	}{"beta declares beta": {`pbs_namespace = "beta"`, true}, "beta declares nothing": {"", false}} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			setAcmeTokens(t)
			writeClustersFile(t, acmeClusters(f, f.pbs.URL, "")+`
[clusters.beta]
pve_base_url = "https://127.0.0.1:9/api2/json"
pve_token_env = "ACME_PVE"
pve_fingerprint = "zz"
`+c.decl+"\n")
			out, _, err := runCLI(t, "--cluster", "acme", "posture", "--json")
			if err != nil {
				t.Fatal(err)
			}
			fresh := false
			for _, r := range decodeJSON[postureReport](t, out).Rows {
				fresh = fresh || (r.VMID == 100 && r.Verdict == "fresh")
			}
			if fresh != c.wantFresh {
				t.Errorf("acme/100 fresh = %v, want %v:\n%s", fresh, c.wantFresh, out)
			}
			// A PBS task is PBS-wide: beta may write somewhere on that PBS either way.
			out, _, _ = runCLI(t, "--cluster", "acme", "failures", "--type", "verify", "--json")
			for _, r := range decodeJSON[failuresReport](t, out).Rows {
				if r.Tenant != "" || !strings.Contains(r.Attribution, "beta (could not be read)") {
					t.Errorf("PBS task credited to %q (attribution %q) while beta is unread", r.Tenant, r.Attribution)
				}
			}
		})
	}
}

// The PBS placeholder origin, or any loopback spelling of the PBS endpoint,
// is never a PVE base.
func TestReservedAndAliasedPVEBaseRefused(t *testing.T) {
	f := newFixture(t)
	port := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")[1]
	for base, want := range map[string]string{
		"https://localhost:8007/api2/json":         "reserves for routing PBS",
		"https://127.0.0.1:8007/api2/json":         "reserves for routing PBS",
		"https://localhost:" + port + "/api2/json": "points at the configured Proxmox Backup Server",
		"https://[::1]:" + port + "/api2/json":     "points at the configured Proxmox Backup Server",
	} {
		t.Setenv("PROXMOX_BASE_URL", base)
		before := f.pbsHits.Load()
		if _, stderr, err := runCLI(t, "cluster", "resources", "--json", "--no-cache"); err == nil || !strings.Contains(stderr+err.Error(), want) {
			t.Errorf("PVE base %s: want refusal %q, got %v", base, want, err)
		}
		if f.pbsHits.Load() != before {
			t.Errorf("PVE base %s reached PBS", base)
		}
	}
}

// Secret-named fields (third-party storage plugins keep passwords in
// storage.cfg) never reach the output, the cache or the local store.
func TestStorageSecretsRedacted(t *testing.T) {
	f := newFixture(t)
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			writeData(w, []map[string]any{{"storage": "truenas", "type": "zfs", "freenas_password": "SENTINEL-SECRET-0201", "truenas_secret": "SENTINEL-SECRET-0202", "api-key": "SENTINEL-SECRET-0203", "keep-last": 3}})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(pve.Close)
	t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
	out, _, err := runCLI(t, "storage", "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "SENTINEL-SECRET") || !strings.Contains(out, "truenas") {
		t.Errorf("storage list leaks a secret or lost the row:\n%s", out)
	}
	_, _, _ = runCLI(t, "sync", "--resources", "storage")
	home, _ := os.UserHomeDir()
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "SENTINEL-SECRET") {
				t.Errorf("secret persisted in %s", p)
			}
		}
		return nil
	})
}

// Credential-class [headers] entries are dropped, not sent to every service.
func TestConfiguredCredentialHeadersDropped(t *testing.T) {
	c := &client.Client{Config: &config.Config{Headers: map[string]string{"Cookie": "PVEAuthCookie=x", "CSRFPreventionToken": "y", "X-Api-Key": "z", "X-Trace": "1"}}}
	dropConfiguredAuthorization(c)
	if len(c.Config.Headers) != 1 || c.Config.Headers["X-Trace"] != "1" {
		t.Fatalf("headers after drop: %v", c.Config.Headers)
	}
}

// A newer backup listed without a readable manifest leaves the latest run
// unproven: the guest is not green.
func TestNewerManifestlessSnapshotIsNotGreen(t *testing.T) {
	f := newFixture(t)
	snaps := append([]any{}, prodSnapshots(f)...)
	snaps = append(snaps, map[string]any{"backup-type": "vm", "backup-id": "100", "backup-time": ago(f.now, 0.5), "owner": "root@pam"})
	f.pbsRoutes["/admin/datastore/main/snapshots"] = snaps
	_, rows := postureByVMID(t)
	if r := rows[100]; r.Severity == "green" {
		t.Errorf("acme/100 green with a newer manifest-less snapshot: %s %q", r.Verdict, r.Reason)
	}
}

// A deeper node holding only non-propagating grants hides nothing below it:
// its descendants keep the inherited privileges.
func TestNonPropagatingOnlyNodeIsTransparent(t *testing.T) {
	f := newFixture(t)
	setAcmeTokens(t)
	f.pbsNS = "acme"
	f.pbsRoutes["/admin/datastore/main/namespace"] = []map[string]any{{"ns": ""}, {"ns": "acme"}, {"ns": "acme/sub"}}
	f.pbsRoutes["/admin/datastore/main/snapshots#acme"] = prodSnapshots(f)
	f.pbsRoutes["/access/permissions"] = map[string]any{
		"/":                        map[string]any{"Sys.Audit": true, "Datastore.Audit": true},
		"/datastore/main/acme/sub": map[string]any{"Datastore.Audit": false},
	}
	writeClustersFile(t, acmeClusters(f, f.pbs.URL, `pbs_namespace = "acme"`))
	rep, _ := postureByVMID(t)
	if e := failureByScope(rep.FetchFailures)["pbs"]; strings.Contains(e, "propagate") {
		t.Errorf("a node with only non-propagating grants hides nothing: %q", e)
	}
}

// Each clusters.toml entry gets its own local store: colliding VMIDs never
// overwrite another client's rows, and a fallback never answers with them.
func TestLocalStoreIsPerCluster(t *testing.T) {
	newFixture(t)
	defer func() { selectedCluster = ""; configureDefaultDBScope("") }()
	paths := map[string]string{}
	for _, name := range []string{"acme", "beta", ""} {
		selectedCluster = name
		configureDefaultDBScope("")
		paths[name] = defaultDBPath("proxmox-pp-cli")
	}
	if paths["acme"] == paths["beta"] || paths["acme"] == paths[""] || paths["beta"] == paths[""] {
		t.Errorf("stores are shared across clusters: %v", paths)
	}
}
