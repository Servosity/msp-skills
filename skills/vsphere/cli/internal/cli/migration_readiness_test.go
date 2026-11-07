// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"vsphere-pp-cli/internal/client"
	"vsphere-pp-cli/internal/cliutil/testenv"
)

// readinessVM is vm-1021 with every check passing except what a case sets.
type readinessVM struct {
	disks      string // the Automation API "disks" object
	datastores string // GET /api/vcenter/datastore
	config     string // VI/JSON VirtualMachine config; "" fails the read
}

const plainConfig = `{"_typeName":"VirtualMachineConfigInfo","hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds01] a/a.vmdk"}}]}}`

func readinessVCenter(t *testing.T, vm readinessVM) *httptest.Server {
	t.Helper()
	j := func(w http.ResponseWriter, v string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, v)
	}
	api := map[string]string{
		"/api/vcenter/cluster":          `[]`,
		"/api/vcenter/host":             `[{"host":"host-10","name":"esx01","connection_state":"CONNECTED","power_state":"POWERED_ON"}]`,
		"/api/vcenter/vm?hosts=host-10": `[{"vm":"vm-1021","name":"app01","power_state":"POWERED_OFF","cpu_count":1,"memory_size_MiB":2048}]`,
		"/api/vcenter/vm":               `[{"vm":"vm-1021","name":"app01","power_state":"POWERED_OFF","cpu_count":1,"memory_size_MiB":2048}]`,
		"/api/vcenter/vm/vm-1021":       `{"name":"app01","guest_OS":"UBUNTU_64","power_state":"POWERED_OFF","boot":{"type":"BIOS"},"cpu":{"count":1,"cores_per_socket":1},"memory":{"size_MiB":2048},"disks":` + vm.disks + `}`,
		"/api/vcenter/vm/vm-1021/tools": `{"run_state":"NOT_RUNNING","version_status":"CURRENT"}`,
		"/api/vcenter/datastore":        vm.datastores,
	}
	vi := map[string]string{
		"VirtualMachine/vm-1021/snapshot": `null`,
		"VirtualMachine/vm-1021/summary":  `{"config":{"template":false},"runtime":{"consolidationNeeded":false}}`,
		"VirtualMachine/vm-1021/layoutEx": `{"file":[{"key":1,"size":10}],"disk":[{"key":2000,"chain":[{"fileKey":[1]}]}]}`,
	}
	if vm.config != "" {
		vi["VirtualMachine/vm-1021/config"] = vm.config
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if _, p, ok := r.BasicAuth(); !ok || p != fakeVCPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		j(w, `"s-api"`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(client.SessionHeader) != "s-api" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		if body, ok := api[key]; ok {
			j(w, body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		j(w, `{"error_type":"NOT_FOUND"}`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/ServiceInstance/ServiceInstance/content", func(w http.ResponseWriter, r *http.Request) {
		j(w, `{"about":{"name":"VMware VirtualCenter Server","version":"8.0.2","apiVersion":"8.0.2.0","instanceUuid":"vc-uuid"},"sessionManager":{"value":"SessionManager"}}`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/SessionManager/SessionManager/Login", func(w http.ResponseWriter, r *http.Request) {
		var b map[string]string
		_ = json.NewDecoder(r.Body).Decode(&b)
		if b["password"] != fakeVCPassword {
			w.WriteHeader(http.StatusInternalServerError)
			j(w, `{"_typeName":"InvalidLogin"}`)
			return
		}
		w.Header().Set(client.SessionHeader, "s-vi")
		j(w, `{}`)
	})
	mux.HandleFunc("/sdk/vim25/8.0.1.0/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(client.SessionHeader) != "s-vi" {
			w.WriteHeader(http.StatusInternalServerError)
			j(w, `{"_typeName":"NotAuthenticated"}`)
			return
		}
		if body, ok := vi[strings.TrimPrefix(r.URL.Path, "/sdk/vim25/8.0.1.0/")]; ok {
			j(w, body)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		j(w, `{"_typeName":"ManagedObjectNotFound"}`)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func blockersFor(t *testing.T, vm readinessVM) blockerRow {
	t.Helper()
	testenv.Isolate(t)
	out, err := runCLI(t, readinessVCenter(t, vm), "migration", "blockers")
	if err != nil {
		t.Fatalf("migration blockers: %v\n%s", err, out)
	}
	v := decode[blockersView](t, out)
	if len(v.VMs) != 1 {
		t.Fatalf("want one VM, got %+v", v.VMs)
	}
	return v.VMs[0]
}

func vmdkDisk(file string) string {
	return `{"2000":{"label":"Hard disk 1","type":"SCSI","capacity":1073741824,"backing":{"type":"VMDK_FILE","vmdk_file":` + jsonString(file) + `}}}`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// Class migration-readiness-wrong-result: a VM is ready only when every check
// ran and passed for every disk.
func TestMigrationReadinessNeverReadsAnUnresolvedCheckAsAPass(t *testing.T) {
	notReady := func(t *testing.T, row blockerRow, check string) {
		t.Helper()
		if row.Verdict == "ready" || !slices.Contains(row.UnknownChecks, check) && !hasCode(row.Blockers, strings.ToUpper(check)) {
			t.Errorf("verdict %s, blockers %v, unknown %v: want %s unknown or blocking", row.Verdict, codes(row.Blockers), row.UnknownChecks, check)
		}
	}

	t.Run("control: one VMFS datastore, no keys", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), datastores: `[{"datastore":"datastore-11","name":"ds01","type":"VMFS"}]`, config: plainConfig})
		if row.Verdict != "ready" || len(row.UnknownChecks) != 0 {
			t.Fatalf("the control failed: %s %v %v", row.Verdict, codes(row.Blockers), row.UnknownChecks)
		}
	})

	// (a) A datastore name is unique only inside its datacenter.
	for _, order := range []string{
		`[{"datastore":"datastore-11","name":"ds01","type":"VSAN"},{"datastore":"datastore-12","name":"ds01","type":"VMFS"}]`,
		`[{"datastore":"datastore-12","name":"ds01","type":"VMFS"},{"datastore":"datastore-11","name":"ds01","type":"VSAN"}]`,
	} {
		t.Run("a repeated name with two types", func(t *testing.T) {
			row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), datastores: order, config: plainConfig})
			notReady(t, row, "vsan_storage")
		})
	}
	t.Run("a repeated name with one type is read", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), config: plainConfig,
			datastores: `[{"datastore":"datastore-11","name":"ds01","type":"VSAN"},{"datastore":"datastore-12","name":"ds01","type":"VSAN"}]`})
		if row.Verdict != "blocked" || !hasCode(row.Blockers, "VSAN_STORAGE") {
			t.Errorf("verdict %s %v %v", row.Verdict, codes(row.Blockers), row.UnknownChecks)
		}
	})

	// (b) A name runs up to the "] " separator.
	t.Run("a name holding ]", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds]x] a/a.vmdk"), config: plainConfig,
			datastores: `[{"datastore":"datastore-11","name":"ds","type":"VMFS"},{"datastore":"datastore-12","name":"ds]x","type":"VSAN"}]`})
		if row.Verdict != "blocked" || !hasCode(row.Blockers, "VSAN_STORAGE") {
			t.Errorf("verdict %s %v %v", row.Verdict, codes(row.Blockers), row.UnknownChecks)
		}
	})
	t.Run("a path that names two datastores", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds] x] a/a.vmdk"), config: plainConfig,
			datastores: `[{"datastore":"datastore-11","name":"ds","type":"VMFS"},{"datastore":"datastore-12","name":"ds] x","type":"VSAN"}]`})
		notReady(t, row, "vsan_storage")
	})

	// (c) A disk whose datastore cannot be read is never a pass.
	allVSAN := `[{"datastore":"datastore-11","name":"vsanDatastore","type":"VSAN"},{"datastore":"datastore-12","name":"ds01","type":"VSAN"}]`
	for name, disks := range map[string]string{
		"no vmdk_file":   `{"2000":{"label":"Hard disk 1","type":"SCSI","backing":{"type":"VMDK_FILE"}}}`,
		"empty":          vmdkDisk(""),
		"a host path":    vmdkDisk("/vmfs/volumes/vsan:52a1/7f1c/app.vmdk"),
		"a bare name":    vmdkDisk("app.vmdk"),
		"an empty name":  vmdkDisk("[] a/a.vmdk"),
		"no close":       vmdkDisk("[vsanDatastore a/a.vmdk"),
		"a host device":  `{"2000":{"label":"Hard disk 1","type":"SCSI","backing":{"type":"HOST_DEVICE"}}}`,
		"no type at all": `{"2000":{"label":"Hard disk 1","type":"SCSI","backing":{"vmdk_file":"[ds01] a/a.vmdk"}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			row := blockersFor(t, readinessVM{disks: disks, datastores: allVSAN, config: plainConfig})
			notReady(t, row, "vsan_storage")
		})
	}
	t.Run("a datastore row with no type", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), datastores: `[{"datastore":"datastore-11","name":"ds01"}]`, config: plainConfig})
		notReady(t, row, "vsan_storage")
	})

	// (d) An encrypted VM or disk cannot be imported without vCenter's key provider.
	key := `{"_typeName":"CryptoKeyId","keyId":"5a1c-key","providerId":{"_typeName":"KeyProviderId","id":"kms01"}}`
	for name, config := range map[string]string{
		"VM and disk keys": `{"keyId":` + key + `,"hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds01] a/a.vmdk","keyId":` + key + `}}]}}`,
		"a disk key":       `{"hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds01] a/a.vmdk","keyId":` + key + `}}]}}`,
		"a VM key":         `{"keyId":` + key + `,"hardware":{"device":[]}}`,
		"a parent disk key": `{"hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds01] a/a-000001.vmdk",` +
			`"parent":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds01] a/a.vmdk","keyId":` + key + `}}}]}}`,
	} {
		t.Run("encrypted: "+name, func(t *testing.T) {
			row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), datastores: `[{"datastore":"datastore-11","name":"ds01","type":"VMFS"}]`, config: config})
			if row.Verdict != "blocked" || !hasCode(row.Blockers, "ENCRYPTED_DISK") {
				t.Errorf("verdict %s %v %v, want blocked ENCRYPTED_DISK", row.Verdict, codes(row.Blockers), row.UnknownChecks)
			}
		})
	}
	t.Run("the config read fails", func(t *testing.T) {
		row := blockersFor(t, readinessVM{disks: vmdkDisk("[ds01] a/a.vmdk"), datastores: `[{"datastore":"datastore-11","name":"ds01","type":"VMFS"}]`})
		notReady(t, row, "encryption")
	})
}

func hasCode(found []blocker, code string) bool {
	return slices.Contains(codes(found), code)
}

// The summary's cryptoState is set only for an encrypted VM, so it blocks even
// when config could not be read; a keyId that is JSON null is no key.
func TestEncryptionSignals(t *testing.T) {
	info := mustJSON[vmInfo](t, `{"boot":{"type":"BIOS"}}`)
	tools := &toolsInfo{VersionStatus: "CURRENT"}
	base := blockerInput{Info: &info, Tools: tools, Snapshots: &snapshotInfo{}, Layout: &layoutEx{},
		Summary: mustJSONPtr[vmSummary](t, `{"runtime":{"consolidationNeeded":false}}`)}
	locked := base
	locked.Summary = mustJSONPtr[vmSummary](t, `{"runtime":{"consolidationNeeded":false,"cryptoState":"locked"}}`)
	if found, _, verdict := evaluateBlockers(locked); verdict != "blocked" || !hasCode(found, "ENCRYPTED_DISK") {
		t.Errorf("cryptoState locked, config unread: %v %s", codes(found), verdict)
	}
	null := base
	null.Config = mustJSONPtr[vmCryptoConfig](t, `{"keyId":null,"hardware":{"device":[{"backing":{"keyId":null}}]}}`)
	if found, unknown, verdict := evaluateBlockers(null); verdict != "ready" {
		t.Errorf("null keys: %v %v %s", codes(found), unknown, verdict)
	}
}

func mustJSONPtr[T any](t *testing.T, s string) *T {
	t.Helper()
	v := mustJSON[T](t, s)
	return &v
}
