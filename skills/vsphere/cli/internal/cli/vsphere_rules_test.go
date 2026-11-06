// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: tests for the pure rules in vsphere_rules.go.

package cli

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func mustJSON[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return v
}

func TestFlattenSnapshotsDepthAgeCurrent(t *testing.T) {
	info := mustJSON[snapshotInfo](t, `{
	  "currentSnapshot": {"type":"VirtualMachineSnapshot","value":"snapshot-3"},
	  "rootSnapshotList": [{
	    "snapshot":{"type":"VirtualMachineSnapshot","value":"snapshot-1"},"name":"pre-patch","id":1,
	    "createTime":"2026-10-01T00:00:00Z","state":"poweredOn","quiesced":true,
	    "childSnapshotList":[{"snapshot":{"value":"snapshot-3"},"name":"child","id":3,"createTime":"2026-10-14T12:00:00Z","state":"poweredOff"}]
	  }]}`)
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	rows := flattenSnapshots(invVM{ID: "vm-1", Name: "web01"}, info, now)
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	if rows[0].Depth != 1 || rows[1].Depth != 2 {
		t.Fatalf("depths %d,%d", rows[0].Depth, rows[1].Depth)
	}
	if rows[0].AgeDays != 14 || rows[1].AgeDays != 0.5 {
		t.Fatalf("ages %v,%v", rows[0].AgeDays, rows[1].AgeDays)
	}
	if rows[0].IsCurrent || !rows[1].IsCurrent || !rows[0].Quiesced {
		t.Fatalf("current/quiesced wrong: %+v", rows)
	}
	if got := flattenSnapshots(invVM{}, snapshotInfo{}, now); got == nil || len(got) != 0 {
		t.Fatalf("no snapshots must be an empty (non-nil) slice, got %#v", got)
	}
}

func TestSummarizeLayoutCountsOnlyDeltaLinks(t *testing.T) {
	l := mustJSON[layoutEx](t, `{
	  "file":[{"key":1,"type":"diskDescriptor","size":1000},{"key":2,"type":"diskExtent","size":40000},
	          {"key":3,"type":"diskDescriptor","size":500},{"key":4,"type":"diskExtent","size":7000},
	          {"key":5,"type":"snapshotData","size":30},{"key":6,"type":"snapshotMemory","size":900},
	          {"key":7,"type":"diskExtent","size":250}],
	  "disk":[{"key":2000,"chain":[{"fileKey":[3,4]},{"fileKey":[1,2]}]}],
	  "snapshot":[{"key":{"value":"snapshot-1"},"disk":[{"key":2000,"chain":[{"fileKey":[7]},{"fileKey":[1,2]}]}]},
	              {"key":{"value":"snapshot-2"},"disk":[{"key":2000,"chain":[{"fileKey":[3,4]},{"fileKey":[1,2]}]}]}]}`)
	got := summarizeLayout(l)
	// Chains run attached-disk first, base last: 3+4 (current branch) and 7
	// (another branch, retained by snapshot-1) are deltas, each counted once.
	want := diskChains{DeltaBytes: 7750, SnapshotFileBytes: 930, MaxChainLength: 2}
	if got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestLicensableCoresAppliesPerCPUFloor(t *testing.T) {
	cases := []struct {
		sockets, cores, want int64
		ok                   bool
	}{
		{2, 16, 32, true}, // 2 x 8 cores -> 2 x 16 (KB 313548 example host A)
		{2, 48, 48, true}, // 2 x 24 cores, above the floor
		{1, 10, 16, true}, // a 10-core CPU still counts as 16
		{2, 34, 34, true}, // 17 per socket
		{2, 33, 34, true}, // uneven split rounds the per-socket count up
		{0, 16, 0, false}, // unknown hardware is not zero
		{2, 0, 0, false},
	}
	for _, c := range cases {
		got, ok := licensableCores(c.sockets, c.cores, 16)
		if got != c.want || ok != c.ok {
			t.Errorf("licensableCores(%d,%d) = %d,%v want %d,%v", c.sockets, c.cores, got, ok, c.want, c.ok)
		}
	}
	if got, _ := licensableCores(2, 16, 0); got != 16 {
		t.Errorf("floor 0 should not inflate: %d", got)
	}
}

func TestMaskLicenceKey(t *testing.T) {
	if got := maskLicenceKey("AAAAA-BBBBB-CCCCC-DDDDD-EEEEE"); got != "*****-*****-*****-*****-EEEEE" {
		t.Fatalf("got %q", got)
	}
	if got := maskLicenceKey("00000-00000-00000-00000-00000"); strings.Count(got, "*") != 20 {
		t.Fatalf("evaluation key not masked: %q", got)
	}
	if got := maskLicenceKey("ABCDEFGHIJ"); got != "*****FGHIJ" {
		t.Fatalf("no-dash key: %q", got)
	}
	if maskLicenceKey("") != "" {
		t.Fatal("empty")
	}
}

func codes(bs []blocker) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.Code)
	}
	return out
}

func TestEvaluateBlockers(t *testing.T) {
	info := mustJSON[vmInfo](t, `{"name":"db01","power_state":"POWERED_ON","boot":{"type":"EFI"},
	  "scsi_adapters":{"1000":{"label":"SCSI controller 0","type":"PVSCSI"}},
	  "nics":{"4000":{"label":"Network adapter 1","type":"VMXNET3"}},
	  "cdroms":{"3000":{"label":"CD/DVD drive 1","state":"CONNECTED","backing":{"type":"ISO_FILE","iso_file":"[ds1] iso/win.iso"}}}}`)
	snaps := mustJSON[snapshotInfo](t, `{"rootSnapshotList":[{"snapshot":{"value":"snapshot-1"},"name":"s","createTime":"2026-10-01T00:00:00Z"}]}`)
	tools := toolsInfo{RunState: "NOT_RUNNING", VersionStatus: "CURRENT"}
	notNeeded := mustJSON[vmSummary](t, `{"runtime":{"consolidationNeeded":false}}`)
	needed := mustJSON[vmSummary](t, `{"runtime":{"consolidationNeeded":true}}`)
	plain := &vmCryptoConfig{} // VI/JSON config read, no encryption key
	found, unknown, verdict := evaluateBlockers(blockerInput{Info: &info, Tools: &tools, Snapshots: &snaps, Layout: &layoutEx{}, Summary: &notNeeded, Config: plain})
	want := []string{"SNAPSHOTS_PRESENT", "ISO_ATTACHED", "EFI_FIRMWARE", "PVSCSI_CONTROLLER", "VMXNET_NIC", "TOOLS_NOT_RUNNING"}
	if !reflect.DeepEqual(codes(found), want) || len(unknown) != 0 || verdict != "blocked" {
		t.Fatalf("got %v unknown=%v verdict=%s", codes(found), unknown, verdict)
	}

	clean := mustJSON[vmInfo](t, `{"power_state":"POWERED_OFF","boot":{"type":"BIOS"}}`)
	current := &toolsInfo{VersionStatus: "CURRENT"}
	deep := mustJSON[layoutEx](t, `{"disk":[{"key":2000,"chain":[{"fileKey":[1]},{"fileKey":[2]}]}]}`)
	// A deep chain with no snapshots is only a warning (linked clones have one)...
	found, _, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: current, Snapshots: &snapshotInfo{}, Layout: &deep, Summary: &notNeeded, Config: plain})
	if !reflect.DeepEqual(codes(found), []string{"DELTA_CHAIN_WITHOUT_SNAPSHOTS"}) || verdict != "review" {
		t.Fatalf("deep chain: %v %s", codes(found), verdict)
	}
	// ...vCenter's consolidationNeeded flag is what blocks.
	found, _, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: current, Snapshots: &snapshotInfo{}, Layout: &deep, Summary: &needed, Config: plain})
	if !reflect.DeepEqual(codes(found), []string{"CONSOLIDATION_NEEDED"}) || verdict != "blocked" {
		t.Fatalf("consolidation: %v %s", codes(found), verdict)
	}

	// Ready only when every check ran and found nothing above info.
	_, _, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: current, Snapshots: &snapshotInfo{}, Layout: &layoutEx{}, Summary: &notNeeded, Config: plain})
	if verdict != "ready" {
		t.Fatalf("clean VM verdict %s", verdict)
	}
	// A summary without the consolidation flag is unknown, not a pass.
	_, unknown, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: current, Snapshots: &snapshotInfo{}, Layout: &layoutEx{}, Summary: &vmSummary{}, Config: plain})
	if verdict != "unknown" || !reflect.DeepEqual(unknown, []string{"consolidation"}) {
		t.Fatalf("missing flag: %v %s", unknown, verdict)
	}

	// vCenter 7.0: no VI/JSON, so those checks are unknown, never a pass.
	found, unknown, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: current})
	if len(found) != 0 || !reflect.DeepEqual(unknown, []string{"snapshots", "consolidation", "delta_disks", "encryption"}) || verdict != "unknown" {
		t.Fatalf("no VI: %v %v %s", codes(found), unknown, verdict)
	}
	_, _, verdict = evaluateBlockers(blockerInput{Info: &info, Tools: &tools})
	if verdict != "unknown" {
		t.Fatalf("warn-only with unknown checks should be unknown, got %s", verdict)
	}
	// Tools stopped and no version status reported: installation is unknown.
	_, unknown, verdict = evaluateBlockers(blockerInput{Info: &clean, Tools: &toolsInfo{RunState: "NOT_RUNNING"}, Snapshots: &snapshotInfo{}, Layout: &layoutEx{}, Summary: &notNeeded, Config: plain})
	if verdict != "unknown" || !reflect.DeepEqual(unknown, []string{"vmware_tools"}) {
		t.Fatalf("tools without version status: %v %s", unknown, verdict)
	}
}

func TestProxmoxOSType(t *testing.T) {
	cases := map[string][2]any{
		"WINDOWS_9_SERVER_64": {"win10", true}, "WINDOWS_SERVER_2021": {"win11", true},
		"WINDOWS_7_SERVER_64": {"w2k8", true}, "WIN_NET_STANDARD": {"w2k3", true},
		"RHEL_9_64": {"l26", false}, "UBUNTU_64": {"l26", false}, "SOLARIS_11_64": {"solaris", false},
		"FREEBSD_14_64": {"other", false}, "DARWIN_23_64": {"other", false}, "OTHER_64": {"other", false},
		"DOS": {"other", false}, "SOMETHING_NEW_64": {"l26", false},
	}
	for g, want := range cases {
		os, win := proxmoxOSType(g)
		if os != want[0] || win != want[1] {
			t.Errorf("%s -> %s,%v want %v,%v", g, os, win, want[0], want[1])
		}
	}
}

func TestPlanProxmox(t *testing.T) {
	info := mustJSON[vmInfo](t, `{"name":"SQL 01 (prod)","guest_OS":"WINDOWS_SERVER_2021","power_state":"POWERED_ON",
	  "identity":{"bios_uuid":"4211aaaa-bbbb-cccc-dddd-eeeeffff0000"},
	  "boot":{"type":"EFI"},"cpu":{"count":8,"cores_per_socket":4},"memory":{"size_MiB":16384},
	  "scsi_adapters":{"1000":{"type":"PVSCSI","scsi":{"bus":1,"unit":7}},"1001":{"type":"LSILOGICSAS","scsi":{"bus":0,"unit":7}}},
	  "disks":{"2001":{"label":"Hard disk 3","type":"SCSI","scsi":{"bus":0,"unit":10},"capacity":2147483648,"backing":{"vmdk_file":"[ds1] sql01/sql01_2.vmdk"}},
	           "2000":{"label":"Hard disk 1","type":"SCSI","scsi":{"bus":0,"unit":2},"capacity":42949672960,"backing":{"type":"VMDK_FILE","vmdk_file":"[ds1] sql01/sql01.vmdk"}},
	           "16000":{"label":"Hard disk 2","type":"NVME","nvme":{"bus":0,"unit":0},"capacity":1073741824,"backing":{"vmdk_file":"[ds1] sql01/sql01_1.vmdk"}}},
	  "nics":{"4000":{"type":"VMXNET3","mac_address":"00:50:56:aa:bb:cc","backing":{"network_name":"VM Network"}}},
	  "cdroms":{"3000":{"type":"SATA","sata":{"bus":0,"unit":0},"backing":{"type":"ISO_FILE","iso_file":"[ds1] iso/x.iso"}}}}`)
	p := planProxmox("vm-7", info).Proxmox
	want := map[string]any{
		"bios": "ovmf", "ostype": "win11", "cores": int64(4), "sockets": int64(2), "memory": int64(16384),
		"scsihw": "pvscsi", "net0": "model=vmxnet3,macaddr=00:50:56:AA:BB:CC", "name": "SQL-01-prod",
		"smbios1": "uuid=aaaa1142-bbbb-cccc-dddd-eeeeffff0000", "sata0": "none,media=cdrom",
		"boot": "order=sata0;scsi0;scsi1;scsi2",
		// scsihw follows the controller of the attached disks (bus 0, LSI SAS),
		// not the unused PVSCSI adapter; OVMF has no LSI driver, so Windows gets pvscsi.
	}
	for k, v := range want {
		if !reflect.DeepEqual(p[k], v) {
			t.Errorf("%s = %#v want %#v", k, p[k], v)
		}
	}
	// Importer order is lexical on .vmx keys: scsi0:10 before scsi0:2; NVMe last.
	if d, _ := p["scsi0"].(map[string]any); d["source_vmdk"] != "[ds1] sql01/sql01_2.vmdk" {
		t.Errorf("scsi0 = %#v", p["scsi0"])
	}
	if d, _ := p["scsi1"].(map[string]any); d["size"] != "40G" || d["source_vmdk"] != "[ds1] sql01/sql01.vmdk" {
		t.Errorf("scsi1 = %#v", p["scsi1"])
	}
	if d, _ := p["scsi2"].(map[string]any); d["source_vmdk"] != "[ds1] sql01/sql01_1.vmdk" {
		t.Errorf("scsi2 (nvme) = %#v", p["scsi2"])
	}
	if _, ok := p["efidisk0"]; !ok {
		t.Error("EFI VM needs efidisk0")
	}

	// Linux BIOS VM with no LSI controller defaults to virtio-scsi-single.
	lin := mustJSON[vmInfo](t, `{"name":"web","guest_OS":"UBUNTU_64","power_state":"POWERED_OFF","boot":{"type":"BIOS"},"cpu":{"count":3,"cores_per_socket":2}}`)
	lp := planProxmox("vm-8", lin)
	if lp.Proxmox["scsihw"] != "virtio-scsi-single" || lp.Proxmox["bios"] != "seabios" {
		t.Errorf("linux plan %#v", lp.Proxmox)
	}
	if lp.Proxmox["cores"] != int64(3) || lp.Proxmox["sockets"] != int64(1) {
		t.Errorf("odd cpu split should collapse to one socket: %#v", lp.Proxmox)
	}
	if len(lp.Warnings) != 0 {
		t.Errorf("powered-off clean VM has warnings %v", lp.Warnings)
	}
	// Windows BIOS VM on LSI keeps lsi; no virtio default for non-Linux.
	win := mustJSON[vmInfo](t, `{"guest_OS":"WINDOWS_9_SERVER_64","boot":{"type":"BIOS"},"scsi_adapters":{"1000":{"type":"LSILOGIC"}},
	  "disks":{"2000":{"type":"SCSI","scsi":{"bus":0,"unit":0},"backing":{"vmdk_file":"[ds] w.vmdk"}}}}`)
	if got := planProxmox("vm-9", win).Proxmox["scsihw"]; got != "lsi" {
		t.Errorf("windows bios lsi -> %v", got)
	}
}

func TestPlanProxmoxSCSIHWFromAttachedDiskController(t *testing.T) {
	// An unused PVSCSI adapter (bus 1) must not decide scsihw for disks on
	// the LSI Logic adapter (bus 0); a Linux BIOS VM keeps lsi.
	info := mustJSON[vmInfo](t, `{"guest_OS":"RHEL_9_64","boot":{"type":"BIOS"},
	  "scsi_adapters":{"1000":{"type":"PVSCSI","scsi":{"bus":1,"unit":7}},"1001":{"type":"LSILOGIC","scsi":{"bus":0,"unit":7}}},
	  "disks":{"2000":{"type":"SCSI","scsi":{"bus":0,"unit":0},"capacity":1073741824,"backing":{"vmdk_file":"[ds] a.vmdk"}},
	           "2001":{"type":"SCSI","capacity":null,"scsi":{"bus":0,"unit":1},"backing":{"vmdk_file":"[ds] b.vmdk"}}}}`)
	p := planProxmox("vm-1", info).Proxmox
	if p["scsihw"] != "lsi" {
		t.Errorf("scsihw = %v, want lsi", p["scsihw"])
	}
	if d, _ := p["scsi1"].(map[string]any); d["size"] != nil {
		t.Errorf("unknown capacity must stay unknown: %#v", d)
	}
}

func TestSmbiosUUIDMatchesImporter(t *testing.T) {
	// pve ESXiPlugin smbios1_uuid: "$4$3$2$1-$6$5-$8$7-$9-$10" over the 16 bytes.
	if got := smbiosUUID("42 11 aa bb cc dd ee ff-01 23 45 67 89 ab cd ef"); got != "bbaa1142-ddcc-ffee-0123-456789abcdef" {
		t.Fatalf("got %q", got)
	}
	if got := smbiosUUID("4211aabb-ccdd-eeff-0123-456789abcdef"); got != "bbaa1142-ddcc-ffee-0123-456789abcdef" {
		t.Fatalf("dashed form: %q", got)
	}
	if smbiosUUID("not-a-uuid") != "" {
		t.Fatal("garbage must give no smbios1")
	}
}

func TestSpreadsheetSafeNeutralizesFormulas(t *testing.T) {
	for in, want := range map[string]string{
		"=HYPERLINK(\"x\")": "'=HYPERLINK(\"x\")", "+1": "'+1", "-2": "'-2", "@SUM(A1)": "'@SUM(A1)",
		"\tx": "'\tx", "web01": "web01", "": "", "10.0.0.1": "10.0.0.1",
	} {
		if got := spreadsheetSafe(in); got != want {
			t.Errorf("spreadsheetSafe(%q) = %q want %q", in, got, want)
		}
	}
}

func TestPlanProxmoxUnaddressedDevicesLeaveOrderUnset(t *testing.T) {
	info := mustJSON[vmInfo](t, `{"guest_OS":"UBUNTU_64","boot":{"type":"BIOS"},
	  "scsi_adapters":{"1000":{"type":"PVSCSI","scsi":{"bus":0,"unit":7}}},
	  "disks":{"2000":{"label":"Hard disk 1","type":"SCSI","backing":{"vmdk_file":"[ds] a.vmdk"}}}}`)
	plan := planProxmox("vm-1", info)
	if _, ok := plan.Proxmox["boot"]; ok {
		t.Errorf("boot order must stay unset: %#v", plan.Proxmox)
	}
	if _, ok := plan.Proxmox["scsihw"]; ok {
		t.Errorf("scsihw must stay unset: %#v", plan.Proxmox)
	}
	found := false
	for _, w := range plan.Warnings {
		found = found || strings.HasPrefix(w, "device-address-unknown")
	}
	if !found || plan.Proxmox["scsi0"] == nil {
		t.Errorf("disk kept, gap disclosed: %#v %v", plan.Proxmox, plan.Warnings)
	}
}

// The Proxmox importer does not read a vSAN-backed disk
// (https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE), and a check that cannot
// run is never a pass.
func TestVSANDiskBlocksTheMigration(t *testing.T) {
	info := mustJSON[vmInfo](t, `{"boot":{"type":"BIOS"},"disks":{"2000":{"label":"Hard disk 1","backing":{"type":"VMDK_FILE","vmdk_file":"[vsanDatastore] 7f1c/app.vmdk"}}}}`)
	tools := &toolsInfo{VersionStatus: "CURRENT"}
	base := blockerInput{Info: &info, Tools: tools, Snapshots: &snapshotInfo{}, Layout: &layoutEx{}, Summary: &vmSummary{}}
	vsan := base
	vsan.DatastoreTypes = map[string]string{"vsanDatastore": "VSAN"}
	found, _, verdict := evaluateBlockers(vsan)
	if verdict != "blocked" || !slices.Contains(codes(found), "VSAN_STORAGE") {
		t.Fatalf("vSAN disk: got %v verdict %s", codes(found), verdict)
	}
	vmfs := base
	vmfs.DatastoreTypes = map[string]string{"vsanDatastore": "VMFS"}
	found, unknown, _ := evaluateBlockers(vmfs)
	if slices.Contains(codes(found), "VSAN_STORAGE") || slices.Contains(unknown, "vsan_storage") {
		t.Fatalf("VMFS disk: got %v unknown %v", codes(found), unknown)
	}
	for name, types := range map[string]map[string]string{"no list": nil, "not in the list": {"other": "VMFS"}} {
		missing := base
		missing.DatastoreTypes = types
		if _, unknown, _ := evaluateBlockers(missing); !slices.Contains(unknown, "vsan_storage") {
			t.Fatalf("%s: unknown %v, want vsan_storage", name, unknown)
		}
	}
}
