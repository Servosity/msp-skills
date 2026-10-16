// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"proxmox-pp-cli/internal/client"
)

// Codex round 11: unread evidence is reported, never silently skipped.

// A storage backing up to a PBS other than the configured one: its tasks
// were not checked.
func TestFailuresNamesStorageOnAnotherPBS(t *testing.T) {
	f := newFixture(t)
	hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
	pve := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/api2/json/storage") {
			port, _ := strconv.Atoi(hp[1])
			writeData(w, []map[string]any{
				{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main"},
				{"storage": "offsite", "type": "pbs", "server": "pbs2.example.net", "datastore": "dr"},
			})
			return
		}
		f.pve.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(pve.Close)
	t.Setenv("PROXMOX_BASE_URL", pve.URL+"/api2/json")
	t.Setenv("PROXMOX_TLS_FINGERPRINT", client.CertFingerprint(pve.Certificate()))
	out, _, err := runCLI(t, "failures", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[failuresReport](t, out).FetchFailures)["pbs tasks"]; !strings.Contains(e, "offsite -> pbs2.example.net") || strings.Contains(e, "pbs1 ->") {
		t.Errorf("want only the offsite storage named: %q", e)
	}
}

// No PBS host and a storage list the token cannot fully see: a hidden
// storage may back up to a PBS whose tasks were not checked.
func TestFailuresHiddenStorageListWithoutPBSHost(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PROXMOX_PBS_HOST", "")
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":        map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "VM.Backup": 1},
		"/storage": map[string]any{"Datastore.Audit": 0, "Datastore.AllocateSpace": 0},
	}
	out, _, err := runCLI(t, "failures", "--json")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ff := range decodeJSON[failuresReport](t, out).FetchFailures {
		found = found || strings.Contains(ff.Error, "a storage hidden from this token may back up")
	}
	if !found {
		t.Errorf("want the hidden storage list named:\n%s", out)
	}
}

// A job writing to a dumpdir (a path, not a storage) has unknown capacity.
func TestCapacityNamesDumpdirJob(t *testing.T) {
	f := newFixture(t)
	jobs := f.pveRoutes["/cluster/backup"].([]map[string]any)
	delete(jobs[0], "storage")
	jobs[0]["dumpdir"] = "/mnt/backup"
	out, _, err := runCLI(t, "capacity", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if e := failureByScope(decodeJSON[capacityReport](t, out).FetchFailures)["job "+jobs[0]["id"].(string)]; !strings.Contains(e, "dumpdir /mnt/backup") {
		t.Errorf("want the dumpdir job's capacity reported unknown:\n%s", out)
	}
}

// A name query whose guest list the token cannot fully see may miss a second
// guest with the same name.
func TestRestorePointsNameQueryWithHiddenGuests(t *testing.T) {
	f := newFixture(t)
	f.pveRoutes["/access/permissions"] = map[string]any{
		"/":        map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1},
		"/vms/105": map[string]any{"VM.Console": 1},
	}
	out, _, _ := runCLI(t, "restore-points", "files", "--json")
	if e := failureByScope(decodeJSON[restorePointsReport](t, out).FetchFailures)["guests"]; !strings.Contains(e, `another guest named "files"`) {
		t.Errorf("want the possibly hidden same-name guest named:\n%s", out)
	}
}
