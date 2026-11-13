// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: end-to-end tests of the novel commands against a fake vCenter
// (TLS, session login, Automation + VI/JSON endpoints) through the real CLI.

package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"vsphere-pp-cli/internal/client"
	"vsphere-pp-cli/internal/cliutil/testenv"
	"vsphere-pp-cli/internal/config"
)

const gib = int64(1) << 30

// slowSnapshots delays the fake vCenter's VI/JSON snapshot reads (timeout
// tests); atomic because handlers may still run when a test resets it.
var slowSnapshots atomic.Int64

func fakeVCenter(t *testing.T, withVI bool) *httptest.Server {
	t.Helper()
	j := func(w http.ResponseWriter, v string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ro@vsphere.local" || p != fakeVCPassword {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
		j(w, `"s-api"`)
	})
	data := map[string]string{
		"/api/vcenter/cluster":                 `[{"cluster":"domain-c8","name":"Prod","ha_enabled":true,"drs_enabled":true}]`,
		"/api/vcenter/host?clusters=domain-c8": `[{"host":"host-10","name":"esx01","connection_state":"CONNECTED","power_state":"POWERED_ON"}]`,
		"/api/vcenter/host": `[{"host":"host-10","name":"esx01","connection_state":"CONNECTED","power_state":"POWERED_ON"},
		                       {"host":"host-20","name":"esx02","connection_state":"CONNECTED","power_state":"POWERED_ON"}]`,
		"/api/vcenter/vm?hosts=host-10": `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON","cpu_count":4,"memory_size_MiB":8192}]`,
		"/api/vcenter/vm?hosts=host-20": `[{"vm":"vm-2","name":"db01","power_state":"POWERED_OFF","cpu_count":2,"memory_size_MiB":4096},
		                                  {"vm":"vm-3","name":"app01","power_state":"POWERED_OFF","cpu_count":1,"memory_size_MiB":2048},
		                                  {"vm":"vm-1","name":"web01","power_state":"POWERED_ON","cpu_count":4,"memory_size_MiB":8192}]`,
		"/api/vcenter/network": `[{"network":"dvportgroup-5","name":"DPG-Prod","type":"DISTRIBUTED_PORTGROUP"}]`,
		"/api/vcenter/vm": `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON","cpu_count":4,"memory_size_MiB":8192},
		                     {"vm":"vm-2","name":"db01","power_state":"POWERED_OFF","cpu_count":2,"memory_size_MiB":4096},
		                     {"vm":"vm-3","name":"app01","power_state":"POWERED_OFF","cpu_count":1,"memory_size_MiB":2048}]`,
		"/api/vcenter/vm?vms=vm-1": `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON","cpu_count":4,"memory_size_MiB":8192}]`,
		"/api/vcenter/vm/vm-1": `{"name":"web01","guest_OS":"WINDOWS_SERVER_2021","power_state":"POWERED_ON","boot":{"type":"EFI"},
		  "identity":{"bios_uuid":"4211-aaaa"},"cpu":{"count":4,"cores_per_socket":2},"memory":{"size_MiB":8192},
		  "scsi_adapters":{"1000":{"label":"SCSI controller 0","type":"PVSCSI","scsi":{"bus":0,"unit":7}}},
		  "disks":{"2000":{"label":"Hard disk 1","type":"SCSI","scsi":{"bus":0,"unit":0},"capacity":107374182400,"backing":{"type":"VMDK_FILE","vmdk_file":"[ds1] web01/web01.vmdk"}}},
		  "nics":{"4000":{"label":"Network adapter 1","type":"VMXNET3","mac_address":"00:50:56:aa:bb:01","backing":{"network_name":"VM Network"}}},
		  "cdroms":{"3000":{"label":"CD/DVD drive 1","type":"SATA","sata":{"bus":0,"unit":0},"state":"CONNECTED","backing":{"type":"ISO_FILE","iso_file":"[ds1] iso/win.iso"}}}}`,
		"/api/vcenter/vm/vm-2": `{"name":"db01","guest_OS":"RHEL_9_64","power_state":"POWERED_OFF","boot":{"type":"BIOS"},"cpu":{"count":2,"cores_per_socket":1},
		  "memory":{"size_MiB":4096},"scsi_adapters":{"1000":{"type":"LSILOGIC"}},
		  "disks":{"2000":{"label":"Hard disk 1","type":"SCSI","capacity":21474836480,"backing":{"vmdk_file":"[ds1] db01/db01.vmdk"}}},
		  "nics":{"4000":{"type":"E1000","mac_address":"00:50:56:aa:bb:02","backing":{"type":"DISTRIBUTED_PORTGROUP","network":"dvportgroup-5"}}}}`,
		"/api/vcenter/vm/vm-3":                             `{"name":"app01","guest_OS":"UBUNTU_64","power_state":"POWERED_OFF","boot":{"type":"BIOS"},"cpu":{"count":1,"cores_per_socket":1},"memory":{"size_MiB":2048}}`,
		"/api/vcenter/vm/vm-1/tools":                       `{"run_state":"RUNNING","version_status":"CURRENT"}`,
		"/api/vcenter/vm/vm-2/tools":                       `{"run_state":"NOT_RUNNING","version_status":"NOT_INSTALLED"}`,
		"/api/vcenter/vm/vm-3/tools":                       `{"run_state":"NOT_RUNNING","version_status":"CURRENT"}`,
		"/api/vcenter/vm/vm-1/guest/identity":              `{"name":"WINDOWS_SERVER_2021","host_name":"web01.corp.local","ip_address":"10.0.0.11","full_name":{"default_message":"Microsoft Windows Server 2022 (64-bit)"}}`,
		"/api/vcenter/vm/vm-1/guest/local-filesystem":      `{"C:\\":{"capacity":107374182400,"free_space":42949672960}}`,
		"/api/vcenter/vm/vm-1/guest/networking/interfaces": `[{"mac_address":"00:50:56:AA:BB:01","ip":{"ip_addresses":[{"ip_address":"10.0.0.11"},{"ip_address":"fe80::1"}]}}]`,
		"/api/vcenter/datastore":                           `[{"datastore":"datastore-11","name":"ds1","type":"VMFS","capacity":2199023255552,"free_space":1099511627776}]`,
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(client.SessionHeader) != "s-api" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		key := r.URL.Path
		if r.URL.RawQuery != "" {
			key += "?" + r.URL.RawQuery
		}
		if body, ok := data[key]; ok {
			j(w, body)
			return
		}
		if strings.Contains(key, "/guest/") {
			w.WriteHeader(http.StatusServiceUnavailable)
			j(w, `{"error_type":"SERVICE_UNAVAILABLE"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		j(w, `{"error_type":"NOT_FOUND"}`)
	})
	if withVI {
		// post-upgrade is dated relative to the wall clock snapshots audit reads,
		// so it stays inside --older-than 30d whenever the test runs. A fixed
		// date here aged past 30 days on 2026-11-13 and flipped the assertion.
		recentSnapshot := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339)
		vi := map[string]string{
			"VirtualMachine/vm-1/snapshot": `{"currentSnapshot":{"value":"snapshot-2"},"rootSnapshotList":[{"snapshot":{"value":"snapshot-1"},"name":"pre-upgrade","createTime":"2025-01-01T00:00:00Z","state":"poweredOn","quiesced":true,
			   "childSnapshotList":[{"snapshot":{"value":"snapshot-2"},"name":"post-upgrade","createTime":"` + recentSnapshot + `","state":"poweredOn"}]}]}`,
			"VirtualMachine/vm-2/snapshot": `{}`,
			"VirtualMachine/vm-3/snapshot": `null`,
			"VirtualMachine/vm-1/summary":  `{"config":{"template":false,"guestFullName":"Microsoft Windows Server 2022 (64-bit)","cpuReservation":500,"memoryReservation":1024},"guest":{"toolsStatus":"toolsOk"},"runtime":{"consolidationNeeded":false}}`,
			"VirtualMachine/vm-2/summary":  `{"config":{"template":false,"guestFullName":"Red Hat Enterprise Linux 9 (64-bit)"},"guest":{"toolsStatus":"toolsNotInstalled"},"runtime":{"consolidationNeeded":true}}`,
			"VirtualMachine/vm-3/summary":  `{"config":{"template":true,"guestFullName":"Ubuntu Linux (64-bit)"},"runtime":{"consolidationNeeded":false}}`,
			"VirtualMachine/vm-1/layoutEx": `{"file":[{"key":1,"type":"diskExtent","size":100},{"key":2,"type":"diskExtent","size":5000},{"key":3,"type":"diskExtent","size":700},{"key":4,"type":"snapshotData","size":9}],
			   "disk":[{"key":2000,"chain":[{"fileKey":[3]},{"fileKey":[2]},{"fileKey":[1]}]}]}`,
			"VirtualMachine/vm-2/layoutEx":           `{"file":[{"key":1,"size":10},{"key":2,"size":20}],"disk":[{"key":2000,"chain":[{"fileKey":[1]},{"fileKey":[2]}]}]}`,
			"VirtualMachine/vm-3/layoutEx":           `{"file":[{"key":1,"size":10}],"disk":[{"key":2000,"chain":[{"fileKey":[1]}]}]}`,
			"VirtualMachine/vm-1/storage":            `{"perDatastoreUsage":[{"datastore":{"value":"datastore-11"},"committed":53687091200,"uncommitted":53687091200}]}`,
			"VirtualMachine/vm-2/storage":            `{"perDatastoreUsage":[{"datastore":{"value":"datastore-11"},"committed":10737418240,"uncommitted":0}]}`,
			"VirtualMachine/vm-3/storage":            `{"perDatastoreUsage":[]}`,
			"VirtualMachine/vm-1/config":             `{"hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds1] web01/web01.vmdk"}}]}}`,
			"VirtualMachine/vm-2/config":             `{"hardware":{"device":[{"_typeName":"VirtualDisk","key":2000,"backing":{"_typeName":"VirtualDiskFlatVer2BackingInfo","fileName":"[ds1] db01/db01.vmdk"}}]}}`,
			"VirtualMachine/vm-3/config":             `{"hardware":{"device":[]}}`,
			"HostSystem/host-10/summary":             `{"hardware":{"vendor":"Dell","model":"R650","numCpuPkgs":2,"numCpuCores":16,"memorySize":274877906944,"numNics":4,"uuid":"h10"},"config":{"product":{"version":"8.0.2","build":"22380479"}}}`,
			"HostSystem/host-20/summary":             `{"hardware":{"vendor":"HPE","model":"DL360","numCpuPkgs":1,"numCpuCores":24,"memorySize":137438953472,"numNics":2,"uuid":"h20"},"config":{"product":{"version":"8.0.2","build":"22380479"}}}`,
			"LicenseManager/LicenseManager/licenses": `[{"licenseKey":"AAAAA-BBBBB-CCCCC-DDDDD-12345","editionKey":"esx.vsphere8.cpuCore","name":"VMware vSphere 8 Enterprise Plus","total":64,"used":56,"costUnit":"cpuCore"}]`,
		}
		mux.HandleFunc("/sdk/vim25/8.0.1.0/ServiceInstance/ServiceInstance/content", func(w http.ResponseWriter, r *http.Request) {
			j(w, `{"about":{"name":"VMware VirtualCenter Server","fullName":"VMware vCenter Server 8.0.2 build-22385739","vendor":"VMware, Inc.","version":"8.0.2","build":"22385739","apiVersion":"8.0.2.0","instanceUuid":"vc-uuid"},
			       "sessionManager":{"value":"SessionManager"},"licenseManager":{"value":"LicenseManager"}}`)
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
			if d := time.Duration(slowSnapshots.Load()); d > 0 && strings.HasSuffix(r.URL.Path, "/snapshot") {
				time.Sleep(d)
			}
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
		})
	}
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runCLI(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	t.Setenv("VSPHERE_BASE_URL", srv.URL)
	t.Setenv("VSPHERE_USERNAME", "ro@vsphere.local")
	t.Setenv("VSPHERE_PASSWORD", fakeVCPassword)
	t.Setenv(client.TLSFingerprintEnv, client.CertFingerprint(srv.Certificate()))
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json", "--rate-limit", "0"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	if err != nil {
		t.Logf("stderr: %s", errOut.String())
	}
	return out.String(), err
}

func decode[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
	return v
}

// fakeVCPassword is long enough to be a real password: the kit holds it from the
// first login, and a two-letter one would withhold ordinary rows.
const fakeVCPassword = "Fake-Vc3nter-Pw91" // gitleaks:allow: synthetic

func TestMigrationBlockersEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	out, err := runCLI(t, srv, "migration", "blockers")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[blockersView](t, out)
	verdicts := map[string]string{}
	reasons := map[string]string{}
	for _, r := range v.VMs {
		verdicts[r.VM] = r.Verdict
		reasons[r.VM] = strings.Join(codes(r.Blockers), ",")
	}
	if verdicts["vm-1"] != "blocked" || !strings.Contains(reasons["vm-1"], "SNAPSHOTS_PRESENT") || !strings.Contains(reasons["vm-1"], "ISO_ATTACHED") || !strings.Contains(reasons["vm-1"], "PVSCSI_CONTROLLER") {
		t.Errorf("vm-1: %s %s", verdicts["vm-1"], reasons["vm-1"])
	}
	if verdicts["vm-2"] != "blocked" || reasons["vm-2"] != "CONSOLIDATION_NEEDED,TOOLS_NOT_INSTALLED" {
		t.Errorf("vm-2: %s %s", verdicts["vm-2"], reasons["vm-2"])
	}
	if verdicts["vm-3"] != "ready" {
		t.Errorf("vm-3 should be ready, got %s %s", verdicts["vm-3"], reasons["vm-3"])
	}
	if v.ScannedVMs != 3 || v.Summary["blocked"] != 2 || v.Summary["ready"] != 1 || len(v.FetchFailures) != 0 {
		t.Errorf("summary %v failures %v", v.Summary, v.FetchFailures)
	}
	// Host/cluster mapping comes from the per-host walk.
	for _, r := range v.VMs {
		if r.VM == "vm-1" && (r.Host != "esx01" || r.Cluster != "Prod") {
			t.Errorf("vm-1 placement %q/%q", r.Host, r.Cluster)
		}
	}
	out, err = runCLI(t, srv, "migration", "blockers", "--severity", "block")
	if err != nil {
		t.Fatal(err)
	}
	if got := decode[blockersView](t, out); len(got.VMs) != 2 {
		t.Errorf("--severity block kept %d VMs, want 2", len(got.VMs))
	}
}

func TestBlockersWithoutVIJSONReportUnknownNotReady(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, false) // vCenter 7.0: no /sdk/vim25 JSON API
	out, err := runCLI(t, srv, "migration", "blockers")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[blockersView](t, out)
	if v.VIJSON.Available || v.Summary["ready"] != 0 {
		t.Fatalf("no VM may be ready without snapshot checks: %+v", v.Summary)
	}
	for _, r := range v.VMs {
		if r.VM == "vm-3" && (r.Verdict != "unknown" || strings.Join(r.UnknownChecks, ",") != "snapshots,consolidation,delta_disks,encryption") {
			t.Errorf("vm-3 = %s %v", r.Verdict, r.UnknownChecks)
		}
	}
	if _, err := runCLI(t, srv, "snapshots", "audit"); err == nil {
		t.Error("snapshots audit must fail without the VI/JSON API")
	}
}

func TestSnapshotsAuditEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	out, err := runCLI(t, srv, "snapshots", "audit")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[snapshotsAuditView](t, out)
	if len(v.Snapshots) != 2 || v.Snapshots[0].Name != "pre-upgrade" || v.Snapshots[1].Depth != 2 || !v.Snapshots[1].IsCurrent {
		t.Fatalf("snapshots %+v", v.Snapshots)
	}
	var vm1, vm2 *vmSnapshotSummary
	for i := range v.VMs {
		switch v.VMs[i].VM {
		case "vm-1":
			vm1 = &v.VMs[i]
		case "vm-2":
			vm2 = &v.VMs[i]
		}
	}
	if vm1 == nil || *vm1.DeltaDiskBytes != 5700 || *vm1.SnapshotFileBytes != 9 || *vm1.SnapshotCount != 2 || *vm1.MaxDepth != 2 {
		t.Errorf("vm-1 summary %+v", vm1)
	}
	if vm2 == nil || vm2.ConsolidationNeeded == nil || !*vm2.ConsolidationNeeded || vm2.DeltaChainNoSnapshots == nil || !*vm2.DeltaChainNoSnapshots {
		t.Errorf("vm-2 should need consolidation: %+v", vm2)
	}
	out, err = runCLI(t, srv, "snapshots", "audit", "--older-than", "30d")
	if err != nil {
		t.Fatal(err)
	}
	if got := decode[snapshotsAuditView](t, out); len(got.Snapshots) != 1 || got.Snapshots[0].Name != "pre-upgrade" {
		t.Errorf("--older-than 30d = %+v", got.Snapshots)
	}
}

func TestLicenceExposureEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	out, err := runCLI(t, srv, "licence", "exposure", "--price-per-core", "10")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[licenceExposureView](t, out)
	if v.Total.LicensableCores == nil || *v.Total.LicensableCores != 56 || *v.Total.Cores != 40 || v.Total.Hosts != 2 || v.Total.HostsCounted != 2 {
		t.Fatalf("totals %+v", v.Total)
	}
	if v.Total.EstimatedCost == nil || *v.Total.EstimatedCost != 560 {
		t.Errorf("estimate %v", v.Total.EstimatedCost)
	}
	for _, h := range v.Hosts {
		if h.Name == "esx01" && (h.LicensableCores != 32 || h.FloorUplift != 16) {
			t.Errorf("esx01 %+v", h)
		}
	}
	if len(v.Licences) != 1 || v.Licences[0].KeySuffix != "*****-*****-*****-*****-12345" {
		t.Errorf("licences %+v", v.Licences)
	}
	if strings.Contains(out, "AAAAA") {
		t.Fatal("full licence key leaked")
	}
}

func TestMigrationCapacityEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	out, err := runCLI(t, srv, "migration", "capacity")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[capacityView](t, out)
	tot := v.Total
	if tot.VMs != 3 || tot.VCPU == nil || *tot.VCPU != 7 || tot.PhysicalCores == nil || *tot.PhysicalCores != 40 || tot.VCPUPerCore == nil || *tot.VCPUPerCore != 0.18 {
		t.Errorf("total %+v", tot)
	}
	if tot.GuestUsedBytes == nil || *tot.GuestUsedBytes != 60*gib || tot.GuestCountedVMs != 1 {
		t.Errorf("guest used %v over %d VMs", tot.GuestUsedBytes, tot.GuestCountedVMs)
	}
	if tot.ProvisionedBytes == nil || *tot.ProvisionedBytes != 110*gib || *tot.CommittedBytes != 60*gib || tot.StorageVMs != 3 {
		t.Errorf("storage %+v", tot)
	}
	// Without VI/JSON the hardware and storage figures are unknown: null, not 0.
	srv7 := fakeVCenter(t, false)
	out, err = runCLI(t, srv7, "migration", "capacity")
	if err != nil {
		t.Fatal(err)
	}
	v7 := decode[capacityView](t, out)
	if v7.Total.PhysicalCores != nil || v7.Total.VCPUPerCore != nil || v7.Total.ProvisionedBytes != nil || v7.Total.VCPU == nil || *v7.Total.VCPU != 7 {
		t.Errorf("no-VI capacity must leave unknown figures null: %+v", v7.Total)
	}
	if len(v.Clusters) != 2 || v.Datastores == nil || v.Datastores.Datastores != 1 || *v.Datastores.CapacityBytes != 2048*gib {
		t.Errorf("clusters %d datastores %+v", len(v.Clusters), v.Datastores)
	}
}

func TestProxmoxMapEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	out, err := runCLI(t, srv, "migration", "proxmox-map", "--vm", "vm-1")
	if err != nil {
		t.Fatal(err)
	}
	v := decode[proxmoxMapView](t, out)
	if len(v.VMs) != 1 {
		t.Fatalf("vms %d failures %+v", len(v.VMs), v.FetchFailures)
	}
	p := v.VMs[0].Proxmox
	if p["bios"] != "ovmf" || p["scsihw"] != "pvscsi" || p["ostype"] != "win11" || p["net0"] != "model=vmxnet3,macaddr=00:50:56:AA:BB:01" {
		t.Errorf("plan %#v", p)
	}
}

func TestRVToolsExportEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	dir := filepath.Join(t.TempDir(), "rv")
	if _, err := runCLI(t, srv, "rvtools", "export", "--out", dir); err != nil {
		t.Fatal(err)
	}
	read := func(name string) [][]string {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		rows, err := csv.NewReader(f).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	info := read("RVTools_tabvInfo.csv")
	if len(info) != 4 || info[0][0] != "VM" || info[0][18] != "VI SDK UUID" {
		t.Fatalf("vInfo %v", info)
	}
	if info[3][0] != "web01" || info[3][2] != "False" || info[3][3] != "web01.corp.local" || info[3][6] != "102400" ||
		info[3][10] != "Microsoft Windows Server 2022 (64-bit)" || info[3][13] != "10.0.0.11" || info[3][15] != "4211-aaaa" {
		t.Errorf("web01 row %v", info[3])
	}
	if info[1][0] != "app01" || info[1][2] != "True" {
		t.Errorf("template column %v", info[1])
	}
	if cpu := read("RVTools_tabvCPU.csv"); cpu[3][3] != "500" {
		t.Errorf("vCPU reservation %v", cpu[3])
	}
	if tools := read("RVTools_tabvTools.csv"); tools[2][1] != "toolsNotInstalled" || tools[3][1] != "toolsOk" {
		t.Errorf("vTools %v", tools)
	}
	if disk := read("RVTools_tabvDisk.csv"); disk[1][1] != "1" || disk[1][2] != "2000" {
		t.Errorf("vDisk sequence %v", disk)
	}
	if host := read("RVTools_tabvHost.csv"); len(host) != 3 || host[0][2] != "# Cores" {
		t.Errorf("vHost %v", host)
	}
	if snap := read("RVTools_tabvSnapshot.csv"); len(snap) != 3 {
		t.Errorf("vSnapshot %v", snap)
	}
	if net := read("RVTools_tabvNetwork.csv"); len(net) != 3 || net[1][1] != "DPG-Prod" || net[2][0] != "web01" || net[2][2] != "10.0.0.11" || net[2][3] != "fe80::1" {
		t.Errorf("vNetwork %v", net)
	}
	// Re-export over an existing looser file: replaced, still 0600 (Windows
	// has no group/other mode bits to loosen).
	if runtime.GOOS != "windows" {
		_ = os.Chmod(filepath.Join(dir, "RVTools_tabvInfo.csv"), 0o644)
		if _, err := runCLI(t, srv, "rvtools", "export", "--out", dir); err != nil {
			t.Fatal(err)
		}
		if st, _ := os.Stat(filepath.Join(dir, "RVTools_tabvInfo.csv")); st.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", st.Mode().Perm())
		}
	}
	out, err := runCLI(t, srv, "rvtools", "export")
	if err != nil {
		t.Fatal(err)
	}
	if v := decode[rvToolsView](t, out); len(v.Tabs["RVTools_tabvInfo.csv"]) != 3 {
		t.Errorf("JSON mode tabs %v", v.Tabs)
	}
}

func TestWalkKeepsHostlessVMsAndFailsOnBrokenHostList(t *testing.T) {
	testenv.Isolate(t)
	plain := `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON"},{"vm":"vm-9","name":"orphan","power_state":"POWERED_OFF"}]`
	hostList := `[{"vm":"vm-1","name":"web01","power_state":"POWERED_ON"}]`
	capHit := false
	brokenHost := false
	mux := http.NewServeMux()
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `"s"`)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/vcenter/cluster":
			_, _ = io.WriteString(w, `[]`)
		case r.URL.Path == "/api/vcenter/host":
			_, _ = io.WriteString(w, `[{"host":"host-1","name":"esx01"}]`)
		case r.URL.Path == "/api/vcenter/vm" && r.URL.Query().Get("hosts") != "":
			if brokenHost {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = io.WriteString(w, hostList)
		case r.URL.Path == "/api/vcenter/vm":
			if capHit {
				w.WriteHeader(http.StatusInternalServerError) // vendor: 500 when more than 4000 VMs match
				_, _ = io.WriteString(w, `{"error_type":"UNABLE_TO_ALLOCATE_RESOURCE","messages":[{"id":"unable_to_allocate_resource"}]}`)
				return
			}
			_, _ = io.WriteString(w, plain)
		}
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	t.Setenv(client.TLSFingerprintEnv, client.CertFingerprint(srv.Certificate()))
	cfgClient := func() *client.Client {
		cfg := &config.Config{BaseURL: srv.URL, VsphereUsername: "u@d", VspherePassword: "p"}
		c := client.New(cfg, 30*time.Second, 0)
		c.NoCache = true
		if err := c.InstallVsphereTransport(); err != nil {
			t.Fatal(err)
		}
		return c
	}
	inv, err := walkInventory(context.Background(), cfgClient(), invScope{}, 2, true)
	if err != nil || len(inv.VMs) != 2 {
		t.Fatalf("hostless VM must be kept: %v %+v", err, inv)
	}
	for _, v := range inv.VMs {
		if v.ID == "vm-1" && v.HostName != "esx01" || v.ID == "vm-9" && v.Host != "" {
			t.Errorf("placement %+v", v)
		}
	}
	capHit = true
	inv, err = walkInventory(context.Background(), cfgClient(), invScope{}, 2, true)
	if err != nil || len(inv.VMs) != 1 || len(inv.Failures) != 1 {
		t.Fatalf("over the 4000 cap: VMs on hosts kept, the gap disclosed: %v %+v", err, inv)
	}
	capHit, brokenHost = false, true
	if _, err := walkInventory(context.Background(), cfgClient(), invScope{}, 2, true); err == nil {
		t.Fatal("a host whose VMs cannot be listed must fail the walk")
	}
}

func TestSnapshotsAuditKeepsVMsNeverReadOnTimeout(t *testing.T) {
	testenv.Isolate(t)
	srv := fakeVCenter(t, true)
	slowSnapshots.Store(int64(1500 * time.Millisecond))
	defer slowSnapshots.Store(0)
	out, _ := runCLI(t, srv, "snapshots", "audit", "--timeout", "1s", "--concurrency", "1")
	v := decode[snapshotsAuditView](t, out)
	if v.UnknownVMs != 3 || len(v.VMs) != 3 {
		t.Fatalf("every VM whose snapshot tree was not read must be listed as unknown: unknown=%d vms=%d", v.UnknownVMs, len(v.VMs))
	}
	for _, vm := range v.VMs {
		if vm.SnapshotCount != nil {
			t.Errorf("%s snapshot_count must be null", vm.VM)
		}
	}
}
