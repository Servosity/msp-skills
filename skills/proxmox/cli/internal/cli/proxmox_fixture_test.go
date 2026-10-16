// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: a fake PVE + PBS pair for the backup-assurance command tests.
// Every guest exercises one verdict path (see fixtureGuests).

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"proxmox-pp-cli/internal/client"
	"proxmox-pp-cli/internal/cliutil/testenv"
)

type fixture struct {
	now        time.Time
	pve, pbs   *httptest.Server
	pbsNS      string           // namespace the pbs1 storage writes to
	pbsFP      string           // fingerprint the pbs1 storage pins (optional)
	pbsPrune   string           // prune-backups of the pbs1 storage (optional)
	storageCfg []map[string]any // more /storage config entries (optional)
	// The route maps the fixture servers answer from; a test may change an
	// entry before it runs a command.
	pveRoutes, pbsRoutes map[string]any
	pveHits              atomic.Int64
	pbsHits              atomic.Int64
	writeCalls           atomic.Int64
}

// oldVerify marks a snapshot as verified 40 days ago (verify job outdated-after is 30).
func oldVerify(s map[string]any, now time.Time) map[string]any {
	st := now.Add(-40 * 24 * time.Hour).Unix()
	s["verification"] = map[string]any{"state": "ok", "upid": fmt.Sprintf("UPID:pbs:000003E8:00000064:00000001:%08X:verificationjob:main\\x3av1:root@pam:", st)}
	return s
}

func ago(now time.Time, h float64) int64 {
	return now.Add(-time.Duration(h * float64(time.Hour))).Unix()
}

// fixtureGuests: 100 fresh, 101 stale, 102 only in a disabled job, 103 in no job
// (tagged veeam), 104 newest point failed verify, 105 covered but never backed
// up, 106 template, 107 newest PBS point unverified, 108 last vzdump attempt failed.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{now: time.Now()}
	n := f.now
	guests := []map[string]any{
		{"id": "qemu/100", "vmid": 100, "name": "web", "node": "pve1", "type": "qemu", "status": "running"},
		{"id": "qemu/101", "vmid": 101, "name": "db", "node": "pve1", "type": "qemu", "status": "running"},
		{"id": "lxc/102", "vmid": 102, "name": "mail", "node": "pve1", "type": "lxc", "status": "running"},
		{"id": "qemu/103", "vmid": 103, "name": "dev", "node": "pve1", "type": "qemu", "status": "stopped", "tags": "veeam;lab"},
		{"id": "qemu/104", "vmid": 104, "name": "files", "node": "pve1", "type": "qemu", "status": "running"},
		{"id": "qemu/105", "vmid": 105, "name": "app", "node": "pve1", "type": "qemu", "status": "running"},
		{"id": "qemu/106", "vmid": 106, "name": "tmpl", "node": "pve1", "type": "qemu", "status": "stopped", "template": 1},
		{"id": "lxc/107", "vmid": 107, "name": "cache", "node": "pve1", "type": "lxc", "status": "running"},
		{"id": "qemu/108", "vmid": 108, "name": "erp", "node": "pve1", "type": "qemu", "status": "running"},
	}
	vol := func(id string, inc int, reason string) map[string]any {
		return map[string]any{"id": id, "name": id, "included": inc, "reason": reason}
	}
	pbsArchive := func(vmid int, sub string, h float64, state string) map[string]any {
		e := map[string]any{"volid": "pbs1:backup/" + sub, "vmid": vmid, "ctime": ago(n, h), "size": 1 << 30, "subtype": sub, "format": "pbs-vm"}
		if sub == "lxc" {
			e["format"] = "pbs-ct"
		}
		if state != "" {
			e["verification"] = map[string]any{"state": state, "upid": "UPID:pbs:verify"}
		}
		return e
	}
	pveRoutes := map[string]any{
		"/cluster/resources": guests,
		"/cluster/backup": []map[string]any{
			{"id": "job1", "enabled": 1, "schedule": "21:00", "storage": "pbs1", "all": 1, "exclude": "103"},
			{"id": "job2", "enabled": 0, "schedule": "sat 02:00", "storage": "pbs1"},
		},
		"/cluster/backup/job1/included_volumes": map[string]any{"children": []map[string]any{
			{"id": 100, "name": "web", "type": "qemu", "children": []any{vol("scsi0", 1, "default")}},
			{"id": 101, "name": "db", "type": "qemu", "children": []any{vol("scsi0", 1, "default")}},
			{"id": 104, "name": "files", "type": "qemu", "children": []any{vol("scsi0", 1, "default"), vol("scsi1", 0, "backup=no")}},
			{"id": 105, "name": "app", "type": "qemu", "children": []any{vol("scsi0", 1, "default")}},
			{"id": 107, "name": "cache", "type": "lxc", "children": []any{vol("rootfs", 1, "default")}},
			{"id": 108, "name": "erp", "type": "qemu", "children": []any{vol("scsi0", 1, "default")}},
		}},
		"/cluster/backup/job2/included_volumes": map[string]any{"children": []map[string]any{
			{"id": 102, "name": "mail", "type": "lxc", "children": []any{vol("rootfs", 1, "default")}},
		}},
		// Upstream blind spot: 102 (only in disabled job2) is NOT listed here.
		"/cluster/backup-info/not-backed-up": []map[string]any{{"vmid": 103, "name": "dev", "type": "qemu"}},
		"/nodes":                             []map[string]any{{"node": "pve1", "status": "online"}},
		"/nodes/pve1/storage": []map[string]any{
			{"storage": "pbs1", "type": "pbs", "shared": 1, "active": 1, "enabled": 1, "content": "backup", "total": 1000, "used": 900, "avail": 100, "used_fraction": 0.9},
			{"storage": "local", "type": "dir", "shared": 0, "active": 1, "enabled": 1, "content": "backup,iso", "total": 1000, "used": 100, "avail": 900, "used_fraction": 0.1},
		},
		"/nodes/pve1/storage/pbs1/content": []any{
			pbsArchive(100, "qemu", 2, "ok"), pbsArchive(100, "qemu", 26, "ok"),
			pbsArchive(101, "qemu", 50, "ok"),
			pbsArchive(104, "qemu", 3, "failed"), pbsArchive(104, "qemu", 27, "ok"),
			pbsArchive(107, "lxc", 5, ""),
			pbsArchive(108, "qemu", 28, "ok"), pbsArchive(108, "qemu", 4, "ok"),
		},
		"/nodes/pve1/storage/local/content": []any{},
		"/nodes/pve1/vzdump/defaults":       map[string]any{}, // no /etc/vzdump.conf overrides
		"/nodes/pve1/storage/pbs1/prunebackups": []map[string]any{
			{"volid": "pbs1:backup/qemu", "vmid": 100, "ctime": ago(n, 2), "mark": "keep", "type": "qemu"},
			{"volid": "pbs1:backup/qemu", "vmid": 100, "ctime": ago(n, 26), "mark": "remove", "type": "qemu"},
		},
		"/nodes/pve1/storage/local/prunebackups": []any{},
		"/nodes/pve1/tasks": []map[string]any{
			{"upid": "UPID:pve1:1:1:1:vzdump::root@pam:", "node": "pve1", "type": "vzdump", "id": "", "status": "job errors", "starttime": ago(n, 1), "endtime": ago(n, 0.5), "user": "root@pam"},
			{"upid": "UPID:pve1:2:2:2:vzdump:105:root@pam:", "node": "pve1", "type": "vzdump", "id": "105", "status": "OK", "starttime": ago(n, 30), "endtime": ago(n, 29.9), "user": "root@pam"},
		},
		"/nodes/pve1/tasks/UPID:pve1:1:1:1:vzdump::root@pam:/log": []map[string]any{
			{"n": 1, "t": "INFO: starting new backup job: vzdump 100 104 108 --mode snapshot --storage pbs1"},
			{"n": 2, "t": "INFO: Starting Backup of VM 100 (qemu)"},
			{"n": 3, "t": "INFO: Finished Backup of VM 100 (00:00:42)"},
			{"n": 4, "t": "INFO: Starting Backup of VM 108 (qemu)"},
			{"n": 5, "t": "ERROR: Backup of VM 108 failed - VM 108 qmp command 'backup' failed - got timeout"},
			{"n": 6, "t": "INFO: Starting Backup of VM 104 (qemu)"},
			{"n": 7, "t": "INFO: Backup job finished with errors"},
		},
		"/nodes/pve1/tasks/UPID:pve1:2:2:2:vzdump:105:root@pam:/log": []map[string]any{
			{"n": 1, "t": "INFO: Starting Backup of VM 105 (qemu)"},
			{"n": 2, "t": "INFO: Finished Backup of VM 105 (00:00:01)"},
		},
		"/cluster/jobs/schedule-analyze": "SCHEDULE", // computed in handler
		// The documented PVE token: PVEAuditor plus the two privileges PVE
		// requires before it lists backup volumes (Datastore.AllocateSpace,
		// VM.Backup). backup_visibility_behavior_test.go covers audit-only.
		"/access/permissions": map[string]any{"/": map[string]any{"Sys.Audit": 1, "VM.Audit": 1, "Datastore.Audit": 1, "Datastore.AllocateSpace": 1, "VM.Backup": 1}},
		"/storage":            "STORAGE", // computed in handler (needs the PBS server address)
		"/version":            map[string]any{"version": "8.2.4", "release": "8.2"},
	}
	snap := func(typ, id string, h float64, state string, size int64) map[string]any {
		s := map[string]any{"backup-type": typ, "backup-id": id, "backup-time": ago(n, h), "size": size, "files": []string{"index.json.blob"}, "owner": "root@pam"}
		if state != "" {
			s["verification"] = map[string]any{"state": state, "upid": "UPID:pbs:verify"}
		}
		return s
	}
	pbsRoutes := map[string]any{
		"/admin/datastore":                []map[string]any{{"store": "main", "backend-type": "filesystem", "mount-status": "nonremovable"}},
		"/config/remote/dr":               map[string]any{"name": "dr", "host": "dr.example.net", "port": 8007, "auth-id": "sync@pbs"},
		"/admin/datastore/main/namespace": []map[string]any{{"ns": ""}},
		"/admin/datastore/main/snapshots": []any{
			snap("vm", "100", 2, "ok", 1<<30), snap("vm", "100", 26, "ok", 1<<30), // duplicates of the PVE listing
			snap("vm", "100", 50, "failed", 1<<30), oldVerify(snap("vm", "101", 50, "ok", 1<<30), n),
			snap("vm", "104", 3, "failed", 1<<30), snap("vm", "104", 27, "ok", 1<<30),
			snap("ct", "107", 5, "", 1<<29), snap("vm", "108", 4, "ok", 1<<30), snap("vm", "108", 28, "ok", 1<<30),
			snap("host", "nas", 10, "", 1<<20),
		},
		"/admin/sync":              "SYNC", // computed in handler: honours sync-direction like PBS
		"/version":                 map[string]any{"version": "3.2", "release": "3"},
		"/access/permissions":      map[string]any{"/": map[string]any{"Sys.Audit": true, "Datastore.Audit": true}, "/datastore": map[string]any{"Datastore.Audit": true}},
		"/admin/verify":            []map[string]any{{"id": "v1", "store": "main", "ns": "", "max-depth": 7, "outdated-after": 30, "last-run-state": "OK", "last-run-endtime": ago(n, 8)}},
		"/admin/datastore/main/gc": map[string]any{"store": "main", "last-run-state": "TASK ERROR: not enough space", "still-bad": 3, "removed-bad": 1, "pending-bytes": 5 << 30},
		"/admin/gc":                []map[string]any{{"store": "main", "last-run-state": "TASK ERROR: not enough space", "last-run-endtime": ago(n, 20), "pending-bytes": 5 << 30, "still-bad": 3}},
		"/admin/prune":             []map[string]any{},
		"/config/datastore/main":   map[string]any{"name": "main", "path": "/mnt/datastore/main"},
		"/status/datastore-usage":  []map[string]any{{"store": "main", "total": 1000 << 30, "used": 980 << 30, "avail": 20 << 30, "estimated-full-date": n.Add(72 * time.Hour).Unix(), "mount-status": "nonremovable"}},
		"/nodes/localhost/tasks": []map[string]any{
			{"upid": "UPID:pbs:gc", "node": "localhost", "worker_type": "garbage_collection", "worker_id": "main", "status": "TASK ERROR: not enough space", "starttime": ago(n, 20), "endtime": ago(n, 19.9), "user": "root@pam"},
			{"upid": "UPID:pbs:verify", "node": "localhost", "worker_type": "verificationjob", "worker_id": "main:v1", "status": "verification failed - 1 snapshot(s) failed", "starttime": ago(n, 8), "endtime": ago(n, 7.5), "user": "root@pam"},
		},
		"/nodes/localhost/tasks/UPID:pbs:sync/log": []map[string]any{
			{"n": 1, "t": "2026-10-13T07:00:00+02:00: Syncing datastore 'main', root namespace into datastore 'dr-main', root namespace"},
			{"n": 2, "t": "2026-10-13T07:01:02+02:00: [vm/100]: " + time.Unix(ago(n, 26), 0).UTC().Format(time.RFC3339) + ": sync done"},
		},
		"/nodes/localhost/tasks/UPID:pbs:gc/log":     []map[string]any{{"n": 1, "t": "starting garbage collection on store main"}, {"n": 2, "t": "TASK ERROR: not enough space"}},
		"/nodes/localhost/tasks/UPID:pbs:verify/log": []map[string]any{{"n": 1, "t": "verify main:vm/104/..."}, {"n": 2, "t": "verification failed - 1 snapshot(s) failed"}},
	}
	f.pveRoutes, f.pbsRoutes = pveRoutes, pbsRoutes
	f.pve = httptest.NewTLSServer(f.handler(t, pveRoutes, "PVEAPIToken=", true))
	f.pbs = httptest.NewTLSServer(f.handler(t, pbsRoutes, "PBSAPIToken=", false))
	t.Cleanup(func() { f.pve.Close(); f.pbs.Close() })

	testenv.Isolate(t)
	t.Setenv("PROXMOX_BASE_URL", f.pve.URL+"/api2/json")
	t.Setenv("PROXMOX_API_TOKEN", "auditor@pve!cli=00000000-0000-0000-0000-000000000000")
	t.Setenv("PROXMOX_PBS_HOST", f.pbs.URL)
	t.Setenv("PROXMOX_PBS_API_TOKEN", "auditor@pbs!cli:11111111-1111-1111-1111-111111111111")
	// Self-signed fixture certificates, trusted the Proxmox way: by pinned fingerprint.
	t.Setenv("PROXMOX_TLS_FINGERPRINT", f.pveFingerprint())
	t.Setenv("PROXMOX_PBS_TLS_FINGERPRINT", f.pbsFingerprint())
	t.Setenv("PROXMOX_CLUSTERS_FILE", filepath.Join(t.TempDir(), "clusters.toml"))
	t.Setenv("PROXMOX_CLUSTER", "")
	selectedCluster = ""
	return f
}

func (f *fixture) pveFingerprint() string { return client.CertFingerprint(f.pve.Certificate()) }
func (f *fixture) pbsFingerprint() string { return client.CertFingerprint(f.pbs.Certificate()) }

func (f *fixture) handler(t *testing.T, routes map[string]any, authPrefix string, pve bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			f.writeCalls.Add(1)
			http.Error(w, "read-only fixture", http.StatusMethodNotAllowed)
			return
		}
		if pve {
			f.pveHits.Add(1)
		} else {
			f.pbsHits.Add(1)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), authPrefix) {
			http.Error(w, `{"data":null,"message":"authentication failure"}`, http.StatusUnauthorized)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api2/json")
		q := r.URL.Query()
		switch {
		case pve && path == "/cluster/jobs/schedule-analyze":
			// Like PVE: iterations is capped at 100; events start at starttime.
			iters := 10
			if v := q.Get("iterations"); v != "" {
				iters, _ = strconv.Atoi(v)
			}
			if iters > 100 {
				http.Error(w, `{"data":null,"errors":{"iterations":"value must have a maximum value of 100"}}`, http.StatusBadRequest)
				return
			}
			from, _ := strconv.ParseInt(q.Get("starttime"), 10, 64)
			first := f.now.Add(-180 * time.Hour).Unix() // daily runs at 7.5d, 6.5d, ... ago: never on a window edge
			out := []map[string]any{}
			for ts := first; len(out) < iters; ts += 86400 {
				if ts >= from {
					out = append(out, map[string]any{"timestamp": ts, "utc": time.Unix(ts, 0).UTC().Format(time.RFC3339)})
				}
			}
			writeData(w, out)
			return
		case pve && path == "/nodes/pve1/vzdump/defaults" && q.Get("storage") == "pbs1" && f.pbsPrune != "":
			// The defaults API overlays the asked-for storage's own retention
			// on /etc/vzdump.conf (PVE::API2::VZDump defaults).
			d := map[string]any{}
			if base, ok := routes[path].(map[string]any); ok {
				for k, v := range base {
					d[k] = v
				}
			}
			d["prune-backups"] = f.pbsPrune
			writeData(w, d)
			return
		case pve && path == "/storage":
			hp := strings.Split(strings.TrimPrefix(f.pbs.URL, "https://"), ":")
			port, _ := strconv.Atoi(hp[1])
			writeData(w, append([]map[string]any{{"storage": "pbs1", "type": "pbs", "server": hp[0], "port": port, "datastore": "main", "namespace": f.pbsNS, "fingerprint": f.pbsFP, "prune-backups": f.pbsPrune}}, f.storageCfg...))
			return
		case !pve && path == "/admin/sync":
			if q.Get("sync-direction") != "all" && q.Get("sync-direction") != "push" {
				writeExtra(w, []any{}, "digest", "d1") // default lists pull jobs only; there are none
				return
			}
			writeExtra(w, []map[string]any{{"id": "offsite", "store": "main", "ns": "", "remote": "dr", "remote-store": "dr-main", "sync-direction": "push", "last-run-state": "OK", "last-run-endtime": ago(f.now, 6), "last-run-upid": "UPID:pbs:sync"}}, "digest", "d1")
			return
		}
		// Like Proxmox, /access/permissions?path=X answers for exactly that path
		// when the fixture lists it; otherwise the full dump stands in.
		if want := q.Get("path"); path == "/access/permissions" && want != "" {
			if m, ok := routes[path].(map[string]any); ok {
				if entry, ok := m[want]; ok {
					writeData(w, map[string]any{want: entry})
					return
				}
			}
		}
		// Like PVE::Storage::storage_info: a content filter keeps only enabled
		// storages carrying that content type.
		if want := q.Get("content"); pve && want != "" && strings.HasPrefix(path, "/nodes/") && strings.HasSuffix(path, "/storage") {
			var out []map[string]any
			list, _ := routes[path].([]map[string]any)
			for _, st := range list {
				if fmt.Sprint(st["enabled"]) == "0" || !slices.Contains(strings.Split(fmt.Sprint(st["content"]), ","), want) {
					continue
				}
				out = append(out, st)
			}
			writeData(w, out)
			return
		}
		// A route keyed "<path>#<ns>" answers that namespace only, like PBS.
		if ns := q.Get("ns"); ns != "" {
			if data, ok := routes[path+"#"+ns]; ok {
				writeData(w, data)
				return
			}
		}
		data, ok := routes[path]
		if !ok && pve && strings.HasPrefix(path, "/nodes/") && strings.Contains(path, "/qemu/") && strings.HasSuffix(path, "/config") {
			data, ok = map[string]any{}, true // a VM config with no creation record (meta ctime)
		}
		if !ok {
			t.Logf("fixture 404: %s %s", r.Method, r.URL.String())
			http.Error(w, `{"data":null}`, http.StatusNotFound)
			return
		}
		// Honour the filters the commands rely on.
		if vmid := r.URL.Query().Get("vmid"); vmid != "" {
			data = filterByField(data, "vmid", vmid)
		}
		if id := r.URL.Query().Get("backup-id"); id != "" {
			data = filterByField(data, "backup-id", id)
		}
		// Real servers put sidecar keys next to data: total on task lists and
		// logs (both products), digest on PBS job lists and config reads.
		switch {
		case strings.Contains(path, "/tasks"):
			writeExtra(w, data, "total", 99)
		case !pve && (strings.HasPrefix(path, "/admin/verify") || strings.HasPrefix(path, "/admin/prune") || strings.HasPrefix(path, "/config/")):
			writeExtra(w, data, "digest", "d0")
		default:
			writeData(w, data)
		}
	}
}

func writeExtra(w http.ResponseWriter, data any, key string, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, key: value})
}

func filterByField(data any, field, want string) any {
	raw, _ := json.Marshal(data)
	var rows []map[string]any
	if json.Unmarshal(raw, &rows) != nil {
		return data
	}
	out := []map[string]any{}
	for _, row := range rows {
		b, _ := json.Marshal(row[field])
		if strings.Trim(string(b), `"`) == want {
			out = append(out, row)
		}
	}
	return out
}

func writeData(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
}

// runCLI executes the root command and returns stdout, stderr and the error.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs(append([]string{"--rate-limit", "0"}, args...))
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

func decodeJSON[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
	return v
}

func writeClustersFile(t *testing.T, body string) {
	t.Helper()
	if err := os.WriteFile(os.Getenv("PROXMOX_CLUSTERS_FILE"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
