// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// domotz handfix device-list-synced (spec x-pp-sync-walker on GET
// /agent/{agent_id}/device): the store-backed novels read the device list a
// real `sync` writes. On 4.24 and stock 4.34.2 no sync wrote Collector
// devices to the table the novels read (4.24 filled it with application and
// variable rows), so after a real sync every device novel answered from the
// wrong rows or from none. This test syncs from a fake Domotz API with the
// documented shapes (bare JSON arrays, page_number paging) and runs each
// device novel on the result.

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"domotz-pp-cli/internal/cliutil/testenv"
)

// fakeDomotz serves the documented Domotz list shapes for agents and their
// devices (bare JSON arrays, /agent paged by page_number; every other path
// answers []), records the paths it was asked, and lets a test change the
// account between syncs.
type fakeDomotz struct {
	mu        sync.Mutex
	devices   map[string]string // agent id -> its device list (JSON array)
	fail      map[string]bool   // agent id -> its device list answers 500
	failPaths map[string]bool   // exact path (under /public-api/v1) -> 500
	bodies    map[string]string // exact path (under /public-api/v1) -> its JSON body
	agents    map[string]string // agent id -> its GET /agent record (default: id, display_name, status ONLINE)
	seen      []string
}

func newFakeDomotz(t *testing.T, devices map[string]string) *fakeDomotz {
	f := &fakeDomotz{devices: devices, fail: map[string]bool{}, failPaths: map[string]bool{}, bodies: map[string]string{}, agents: map[string]string{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := strings.TrimPrefix(r.URL.Path, "/public-api/v1")
		f.mu.Lock()
		defer f.mu.Unlock()
		f.seen = append(f.seen, p)
		switch {
		case f.bodies[p] != "":
			fmt.Fprint(w, f.bodies[p])
		case f.failPaths[p]:
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"message":"internal error"}`)
		case p == "/agent":
			// page_size defaults to 10 and page_number to 0 (spec.json GET /agent).
			size, number := 10, 0
			if v, err := strconv.Atoi(r.URL.Query().Get("page_size")); err == nil && v > 0 {
				size = v
			}
			if v, err := strconv.Atoi(r.URL.Query().Get("page_number")); err == nil && v >= 0 {
				number = v
			}
			var ids []string
			for id := range f.devices {
				ids = append(ids, id)
			}
			sort.Slice(ids, func(i, j int) bool { a, _ := strconv.Atoi(ids[i]); b, _ := strconv.Atoi(ids[j]); return a < b })
			agents := []string{}
			for i := number * size; i < len(ids) && i < (number+1)*size; i++ {
				if rec := f.agents[ids[i]]; rec != "" {
					agents = append(agents, rec)
					continue
				}
				agents = append(agents, `{"id":`+ids[i]+`,"display_name":"Site `+ids[i]+`","status":{"value":"ONLINE"}}`)
			}
			fmt.Fprint(w, "["+strings.Join(agents, ",")+"]")
		case strings.HasPrefix(p, "/agent/") && strings.HasSuffix(p, "/device") && strings.Count(p, "/") == 3:
			id := strings.Split(p, "/")[2]
			if _, ok := f.devices[id]; !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"message":"agent not found"}`)
				return
			}
			if f.fail[id] {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"message":"internal error"}`)
				return
			}
			fmt.Fprint(w, f.devices[id])
		default:
			fmt.Fprint(w, `[]`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DOMOTZ_BASE_URL", srv.URL+"/public-api/v1")
	t.Setenv("DOMOTZ_API_KEY", "syncNovelsUnitKey-6b2d9f41")
	return f
}

func (f *fakeDomotz) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.seen...)
}

func (f *fakeDomotz) change(fn func(devices map[string]string, fail map[string]bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f.devices, f.fail)
}

func runDomotz(t *testing.T, args ...string) string {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s%s", args, err, out.String(), errb.String())
	}
	return out.String()
}

func TestNovelsReadTheDevicesSyncWrites(t *testing.T) {
	testenv.Isolate(t)
	// NAS-Acme is first seen relative to the wall clock `fleet new --since 30d`
	// reads, so it stays new whenever the test runs. A fixed 2026-10-27 aged out
	// of that window on 2026-11-26.
	nasFirstSeen := time.Now().UTC().Add(-17 * 24 * time.Hour).Format(time.RFC3339)
	newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"NAS-Acme","protocol":"IP","status":"DOWN","importance":"VITAL","authentication_status":"REQUIRED","type":{"id":5,"label":"Storage"},"user_data":{"vendor":"Synology","model":"DS920"},"first_seen_on":"` + nasFirstSeen + `"},` +
			`{"id":2,"display_name":"AP-Acme","protocol":"IP","status":"ONLINE","importance":"FLOATING","type":{"id":6,"label":"Access Point"},"first_seen_on":"2025-01-01T10:00:00Z"}]`,
		"200": `[{"id":3,"display_name":"FW-Beta","protocol":"IP","status":"ONLINE","importance":"VITAL","type":{"id":7,"label":"Firewall"},"first_seen_on":"2025-01-01T10:00:00Z"}]`,
	})
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")

	for _, c := range []struct {
		args []string
		want []string
	}{
		{[]string{"fleet", "inventory"}, []string{"NAS-Acme", "AP-Acme", "FW-Beta", "Synology"}},
		{[]string{"fleet", "offline"}, []string{"NAS-Acme"}},
		{[]string{"fleet", "unmonitored"}, []string{"NAS-Acme", "REQUIRED"}},
		{[]string{"fleet", "breakdown", "--by", "type"}, []string{"Storage", "Firewall", "Access Point"}},
		{[]string{"fleet", "health"}, []string{`"devices_offline": 1`}},
		{[]string{"fleet", "stale", "--max-age", "0"}, []string{`"devices": 2`, `"devices": 1`}},
		{[]string{"fleet", "new", "--since", "30d"}, []string{"NAS-Acme"}},
		{[]string{"drift", "100", "--baseline"}, []string{"(2 devices)"}},
	} {
		out := runDomotz(t, append(c.args, "--db", db, "--json", "--no-learn")...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("%v after a real sync: missing %q in\n%s", c.args, w, out)
			}
		}
	}
	if out := runDomotz(t, "fleet", "offline", "--db", db, "--json", "--no-learn"); strings.Contains(out, "AP-Acme") || strings.Contains(out, "FW-Beta") {
		t.Errorf("fleet offline lists an ONLINE device:\n%s", out)
	}
}

// domotz handfix per-device-sync-opt-in: a plain sync sends no request per
// device (GET /agent/{agent_id}/device/{device_id}/application or /variable);
// those dependents run only when named.
func TestPlainSyncSendsNoPerDeviceRequests(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{
		"100": `[{"id":1,"display_name":"NAS-Acme","status":"ONLINE"},{"id":2,"display_name":"AP-Acme","status":"ONLINE"}]`,
	})
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")
	perDevice := 0
	for _, p := range fake.paths() {
		if strings.HasPrefix(p, "/agent/100/device/1/") || strings.HasPrefix(p, "/agent/100/device/2/") {
			perDevice++
		}
	}
	if perDevice != 0 {
		t.Errorf("a plain sync sent %d per-device requests: %v", perDevice, fake.paths())
	}
	before := len(fake.paths())
	runDomotz(t, "sync", "--db", db, "--no-learn", "--resources", "device_application")
	named := 0
	for _, p := range fake.paths()[before:] {
		if p == "/agent/100/device/1/application" || p == "/agent/100/device/2/application" {
			named++
		}
	}
	if named != 2 {
		t.Errorf("sync --resources device_application sent %d per-device requests, want 2: %v", named, fake.paths())
	}
}

// domotz handfix fleet-alerts-documented-fields: GET /alert-profile returns
// AlertProfile records (id, name, is_enabled, description, tag, events), but
// fleet alerts printed alert_profile_id and device_id, columns only the
// binding endpoints carry, so after a real sync every profile showed
// alert_profile_id null and device_id null ("bound to no device").
func TestFleetAlertsAfterARealSync(t *testing.T) {
	testenv.Isolate(t)
	fake := newFakeDomotz(t, map[string]string{"100": `[]`})
	fake.bodies["/alert-profile"] = `[{"id":210,"name":"Down Alert","is_enabled":true,"description":"device down","tag":"critical","events":["device_status_down"]},` +
		`{"id":211,"name":"Quiet Rule","is_enabled":false,"description":"muted","tag":"info","events":[]}]`
	db := filepath.Join(t.TempDir(), "data.db")
	runDomotz(t, "sync", "--db", db, "--no-learn")
	out := runDomotz(t, "fleet", "alerts", "--db", db, "--json", "--no-learn")
	for _, w := range []string{`"alert_profile_id": 210`, `"alert_profile_id": 211`, `"name": "Down Alert"`, `"enabled": 1`} {
		if !strings.Contains(out, w) {
			t.Errorf("fleet alerts after a real sync: missing %s in\n%s", w, out)
		}
	}
	if strings.Contains(out, "device_id") {
		t.Errorf("fleet alerts prints a device_id GET /alert-profile never returns:\n%s", out)
	}
}
