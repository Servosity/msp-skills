// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): pure rules behind the novel commands. No I/O,
// so every rule is covered by vsphere_rules_test.go.

package cli

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// --- snapshots ---

type snapshotRow struct {
	VM          string  `json:"vm"`
	VMName      string  `json:"vm_name"`
	Snapshot    string  `json:"snapshot"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	CreateTime  string  `json:"create_time"`
	AgeDays     float64 `json:"age_days"`
	Depth       int     `json:"depth"`
	Quiesced    bool    `json:"quiesced"`
	IsCurrent   bool    `json:"is_current"`
	PowerState  string  `json:"power_state_at_snapshot"`
}

// flattenSnapshots walks a VM's snapshot tree depth-first. Depth 1 is a root.
func flattenSnapshots(vm invVM, info snapshotInfo, now time.Time) []snapshotRow {
	current := ""
	if info.CurrentSnapshot != nil {
		current = info.CurrentSnapshot.Value
	}
	rows := make([]snapshotRow, 0)
	var walk func(nodes []snapshotTree, depth int)
	walk = func(nodes []snapshotTree, depth int) {
		for _, n := range nodes {
			row := snapshotRow{
				VM: vm.ID, VMName: vm.Name, Snapshot: n.Snapshot.Value, Name: n.Name,
				Description: n.Description, CreateTime: n.CreateTime, Depth: depth,
				Quiesced: n.Quiesced, IsCurrent: current != "" && n.Snapshot.Value == current, PowerState: n.State,
				AgeDays: -1,
			}
			if t, err := time.Parse(time.RFC3339, n.CreateTime); err == nil {
				row.AgeDays = roundTo(now.Sub(t).Hours()/24, 1)
			}
			rows = append(rows, row)
			walk(n.Children, depth+1)
		}
	}
	walk(info.RootSnapshots, 1)
	return rows
}

// diskChains summarizes a VM's file layout. A chain runs from the disk as
// attached (first) back to its base disk (last): vSphere documents the first
// diskFile entry as the attached disk, and govmomi builds the chain by walking
// each backing to its parent. Delta bytes are the files of every link except
// the base, across the current chains and every snapshot's retained chains
// (other branches of the tree), each file counted once; snapshot file bytes
// are the snapshot state and memory files. MaxChainLength is the deepest
// current chain.
type diskChains struct {
	DeltaBytes        int64 `json:"delta_disk_bytes"`
	SnapshotFileBytes int64 `json:"snapshot_file_bytes"`
	MaxChainLength    int   `json:"max_chain_length"`
}

func summarizeLayout(l layoutEx) diskChains {
	size := map[int64]int64{}
	var out diskChains
	for _, f := range l.File {
		size[f.Key] = f.Size
		if f.Type == "snapshotData" || f.Type == "snapshotMemory" {
			out.SnapshotFileBytes += f.Size
		}
	}
	chains := append([]layoutDisk{}, l.Disk...)
	for _, d := range l.Disk {
		if len(d.Chain) > out.MaxChainLength {
			out.MaxChainLength = len(d.Chain)
		}
	}
	for _, sl := range l.Snapshot {
		chains = append(chains, sl.Disk...)
	}
	base, delta := map[int64]bool{}, map[int64]bool{}
	for _, d := range chains {
		last := len(d.Chain) - 1
		for i, link := range d.Chain {
			for _, k := range link.FileKey {
				if i == last {
					base[k] = true
				} else {
					delta[k] = true
				}
			}
		}
	}
	for k := range delta {
		if !base[k] {
			out.DeltaBytes += size[k]
		}
	}
	return out
}

// --- licensing (Broadcom KB 313548: license at least 16 physical cores per CPU) ---

// licensableCores applies the per-CPU core floor. cores is the host's total
// physical core count across sockets.
func licensableCores(sockets, cores, minPerCPU int64) (int64, bool) {
	if sockets <= 0 || cores <= 0 {
		return 0, false
	}
	perCPU := (cores + sockets - 1) / sockets
	if perCPU < minPerCPU {
		perCPU = minPerCPU
	}
	return sockets * perCPU, true
}

// maskLicenceKey keeps only the last group of a licence key.
func maskLicenceKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "-")
	last := parts[len(parts)-1]
	if len(parts) == 1 {
		if len(last) <= 5 {
			return strings.Repeat("*", len(last))
		}
		return strings.Repeat("*", len(last)-5) + last[len(last)-5:]
	}
	for i := 0; i < len(parts)-1; i++ {
		parts[i] = strings.Repeat("*", len(parts[i]))
	}
	return strings.Join(parts, "-")
}

// --- migration blockers ---

const (
	sevBlock = "block"
	sevWarn  = "warn"
	sevInfo  = "info"
)

type blocker struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

// blockerInput is everything the rules look at for one VM. A nil pointer
// means that read failed or is unavailable, so its checks report unknown.
type blockerInput struct {
	Info      *vmInfo
	Tools     *toolsInfo
	Snapshots *snapshotInfo
	Layout    *layoutEx
	Summary   *vmSummary // VI/JSON VM summary: runtime.consolidationNeeded
	// Config is VI/JSON VirtualMachine.config (the encryption keys); nil when it
	// could not be read.
	Config *vmCryptoConfig
	// DatastoreTypes is vCenter's datastore list, name to type; nil when it could
	// not be read. A name listed with two types maps to "" (see datastoreTypes).
	DatastoreTypes map[string]string
}

// datastoreName reads the datastore out of a vmdk path: "[name] folder/disk.vmdk".
// A name may hold "]" (ManagedEntity.name escapes only %, / and \), so it runs
// up to the "] " separator. A path with no separator, an empty name, or a
// second separator (a folder that holds "] ") cannot be read: ok is false.
func datastoreName(vmdk string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(vmdk), "[")
	if !ok || strings.Count(rest, "] ") != 1 {
		return "", false
	}
	name, _, _ := strings.Cut(rest, "] ")
	return name, name != ""
}

// evaluateBlockers returns the reason codes, the checks that could not run,
// and the verdict: blocked, review, ready, or unknown (a check could not run
// and nothing blocking was found). A missing check never counts as a pass.
func evaluateBlockers(in blockerInput) ([]blocker, []string, string) {
	found := make([]blocker, 0)
	unknown := make([]string, 0)
	snapCount := -1
	if in.Snapshots == nil {
		unknown = append(unknown, "snapshots")
	} else {
		snapCount = len(flattenSnapshots(invVM{}, *in.Snapshots, time.Now()))
		if snapCount > 0 {
			found = append(found, blocker{"SNAPSHOTS_PRESENT", sevBlock, fmt.Sprintf("%d snapshot(s); delete or consolidate them before export", snapCount)})
		}
	}
	// vCenter's own flag (VirtualMachine runtime.consolidationNeeded) is the
	// authority on leftover delta disks. A deep chain with no snapshots is
	// only an observation: linked clones legitimately have one.
	if in.Summary == nil || in.Summary.Runtime.ConsolidationNeeded == nil {
		unknown = append(unknown, "consolidation")
	} else if *in.Summary.Runtime.ConsolidationNeeded {
		found = append(found, blocker{"CONSOLIDATION_NEEDED", sevBlock, "vCenter reports this VM needs disk consolidation (leftover snapshot delta disks); run Consolidate before export"})
	}
	if in.Layout == nil {
		unknown = append(unknown, "delta_disks")
	} else if snapCount == 0 && (in.Summary == nil || in.Summary.Runtime.ConsolidationNeeded == nil || !*in.Summary.Runtime.ConsolidationNeeded) {
		if ch := summarizeLayout(*in.Layout); ch.MaxChainLength > 1 {
			found = append(found, blocker{"DELTA_CHAIN_WITHOUT_SNAPSHOTS", sevWarn, fmt.Sprintf("a disk chain is %d files deep with no snapshot (a linked clone, or leftover deltas); the import flattens it, so check the source and the space it needs", ch.MaxChainLength)})
		}
	}
	if in.Info == nil {
		unknown = append(unknown, "hardware")
	} else {
		info := in.Info
		// The Proxmox importer does not read a vSAN-backed disk
		// (https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE). Every disk has
		// to resolve to one datastore type: a disk that is not a VMDK file, a
		// path whose datastore cannot be read, a name not in the list, and a
		// name listed with two types leave the check unknown, never a pass.
		storageUnknown := false
		for _, k := range sortedKeys(info.Disks) {
			d := info.Disks[k]
			name, readable := datastoreName(d.Backing.VmdkFile)
			kind := ""
			if readable {
				kind = in.DatastoreTypes[name]
			}
			switch {
			case kind == "VSAN":
				found = append(found, blocker{"VSAN_STORAGE", sevBlock, labelOr(d.Label, k) + " is on vSAN datastore " + name + "; the Proxmox importer cannot read vSAN-backed disks, so move it to VMFS or NFS first"})
			case kind == "" || d.Backing.Type != "VMDK_FILE":
				storageUnknown = true
			}
		}
		if storageUnknown {
			unknown = append(unknown, "vsan_storage")
		}
		for _, k := range sortedKeys(info.Cdroms) {
			cd := info.Cdroms[k]
			connected := cd.State == "CONNECTED" || cd.StartConnected
			if connected && (cd.Backing.Type == "ISO_FILE" || cd.Backing.Type == "HOST_DEVICE") {
				found = append(found, blocker{"ISO_ATTACHED", sevWarn, fmt.Sprintf("%s connected to %s %s; the Proxmox importer does not bring CD-ROM media", labelOr(cd.Label, k), strings.ToLower(cd.Backing.Type), cd.Backing.IsoFile+cd.Backing.HostDevice)})
			}
		}
		if info.Boot.Type == "EFI" {
			found = append(found, blocker{"EFI_FIRMWARE", sevInfo, "UEFI firmware: Proxmox needs bios=ovmf plus an efidisk0; EFI variables (boot entries, Secure Boot keys) do not carry over"})
		}
		for _, k := range sortedKeys(info.ScsiAdapters) {
			if a := info.ScsiAdapters[k]; a.Type == "PVSCSI" {
				found = append(found, blocker{"PVSCSI_CONTROLLER", sevWarn, labelOr(a.Label, k) + " is VMware Paravirtual; keep scsihw=pvscsi for first boot, install VirtIO drivers before switching to virtio-scsi"})
				break
			}
		}
		for _, k := range sortedKeys(info.Disks) {
			if d := info.Disks[k]; d.Type == "NVME" {
				found = append(found, blocker{"NVME_DISK", sevWarn, labelOr(d.Label, k) + " is on an NVMe controller; the Proxmox importer attaches it as SCSI instead"})
				break
			}
		}
		for _, k := range sortedKeys(info.Nics) {
			if n := info.Nics[k]; n.Type == "VMXNET3" || n.Type == "VMXNET2" || n.Type == "VMXNET" {
				found = append(found, blocker{"VMXNET_NIC", sevInfo, labelOr(n.Label, k) + " is " + n.Type + "; the guest sees a new NIC after import, so re-check static IP settings"})
				break
			}
		}
	}
	// The Proxmox importer cannot import an encrypted disk
	// (https://pve.proxmox.com/wiki/Migrate_to_Proxmox_VE: "Encrypted VM disks,
	// for example via a Storage Policy, cannot be imported."); without the
	// vCenter key provider the disk cannot be read. vSphere records the key as
	// a CryptoKeyId on VirtualMachineConfigInfo.keyId (the VM home) and on a
	// virtual disk's backing.keyId. VirtualMachineRuntimeInfo.cryptoState is
	// set only for an encrypted VM, so it blocks even when config is unread.
	encryptedBySummary := in.Summary != nil && in.Summary.Runtime.CryptoState != ""
	switch {
	case in.Config != nil && in.Config.encrypted() || encryptedBySummary:
		found = append(found, blocker{"ENCRYPTED_DISK", sevBlock, "the VM or a disk is encrypted with a vCenter key provider (config.keyId or a disk backing keyId); the Proxmox importer cannot read encrypted disks, so decrypt the VM first"})
	case in.Config == nil:
		unknown = append(unknown, "encryption")
	}
	toolsRunning := in.Tools != nil && (in.Tools.RunState == "RUNNING" || in.Tools.RunState == "EXECUTING_SCRIPTS")
	if in.Tools == nil || (in.Tools.VersionStatus == "" && !toolsRunning) {
		// No version status and not running: vCenter has not said whether
		// Tools is installed, so the check could not run.
		unknown = append(unknown, "vmware_tools")
	} else if in.Tools.VersionStatus == "NOT_INSTALLED" {
		found = append(found, blocker{"TOOLS_NOT_INSTALLED", sevWarn, "VMware Tools not installed: no guest IP/OS detail, no graceful shutdown before cutover"})
	} else if in.Info != nil && in.Info.PowerState == "POWERED_ON" && in.Tools.RunState == "NOT_RUNNING" {
		found = append(found, blocker{"TOOLS_NOT_RUNNING", sevWarn, "VMware Tools is installed but not running in the powered-on guest"})
	}
	return found, unknown, verdictFor(found, unknown)
}

func verdictFor(found []blocker, unknown []string) string {
	worst := ""
	for _, b := range found {
		if b.Severity == sevBlock {
			return "blocked"
		}
		if b.Severity == sevWarn {
			worst = sevWarn
		}
	}
	if len(unknown) > 0 {
		return "unknown"
	}
	if worst == sevWarn {
		return "review"
	}
	return "ready"
}

// --- Proxmox VE mapping ---
//
// Rules follow Proxmox VE's own VMware import (the ESXi import storage in
// pve-storage, ESXiPlugin.pm): bios seabios/ovmf + efidisk0, cores per socket
// and sockets from the VMware CPU layout, memory in MiB, the guest OS mapped to
// a Proxmox ostype (unknown types fall back to l26), NIC models kept,
// LSI Logic controllers -> lsi and PVSCSI -> pvscsi, Linux defaulting to
// virtio-scsi-single, OVMF never paired with lsi, NVMe disks attached as SCSI,
// CD-ROMs left empty, and the BIOS UUID carried as smbios1.

var proxmoxOSTypes = map[string]string{
	"DOS": "other", "WIN_31": "other", "WIN_95": "other", "WIN_98": "other", "WIN_ME": "other",
	"WIN_NT": "wxp", "WIN_2000_PRO": "w2k", "WIN_2000_SERV": "w2k", "WIN_2000_ADV_SERV": "w2k",
	"WIN_XP_HOME": "wxp", "WIN_XP_PRO": "wxp", "WIN_XP_PRO_64": "wxp",
	"WIN_NET_WEB": "w2k3", "WIN_NET_STANDARD": "w2k3", "WIN_NET_ENTERPRISE": "w2k3", "WIN_NET_DATACENTER": "w2k3",
	"WIN_NET_BUSINESS": "w2k3", "WIN_NET_STANDARD_64": "w2k3", "WIN_NET_ENTERPRISE_64": "w2k3", "WIN_NET_DATACENTER_64": "w2k3",
	"WIN_LONGHORN": "w2k8", "WIN_LONGHORN_64": "w2k8", "WIN_VISTA": "wvista", "WIN_VISTA_64": "wvista",
	"WINDOWS_7": "win7", "WINDOWS_7_64": "win7", "WINDOWS_7_SERVER_64": "w2k8",
	"WINDOWS_8": "win8", "WINDOWS_8_64": "win8", "WINDOWS_8_SERVER_64": "win8",
	"WINDOWS_9": "win10", "WINDOWS_9_64": "win10", "WINDOWS_9_SERVER_64": "win10", "WINDOWS_SERVER_2019": "win10",
	"WINDOWS_HYPERV":      "win10",
	"WINDOWS_SERVER_2021": "win11", "WINDOWS_SERVER_2025": "win11", "WINDOWS_11_64": "win11", "WINDOWS_12_64": "win11",
}

var otherOSPrefixes = []string{"FREEBSD", "SOLARIS", "DARWIN", "OS2", "ECOMSTATION", "NETWARE", "OPENSERVER", "UNIXWARE", "VMKERNEL", "CRXPOD"}

// proxmoxOSType maps a vSphere guest_OS value to (ostype, isWindows).
func proxmoxOSType(guest string) (string, bool) {
	if t, ok := proxmoxOSTypes[guest]; ok {
		return t, t != "other"
	}
	if guest == "OTHER" || guest == "OTHER_64" {
		return "other", false
	}
	for _, p := range otherOSPrefixes {
		if strings.HasPrefix(guest, p) {
			if p == "SOLARIS" {
				return "solaris", false
			}
			return "other", false
		}
	}
	return "l26", false
}

var proxmoxNICModels = map[string]string{
	"E1000": "e1000", "E1000E": "e1000e", "VMXNET3": "vmxnet3", "PCNET32": "pcnet",
	"VMXNET": "vmxnet3", "VMXNET2": "vmxnet3",
}

type proxmoxPlan struct {
	VM       string         `json:"vm"`
	Name     string         `json:"name"`
	Source   map[string]any `json:"source"`
	Proxmox  map[string]any `json:"proxmox"`
	Warnings []string       `json:"warnings"`
}

var dnsUnsafe = regexp.MustCompile(`[^a-zA-Z0-9\-.]`)

func proxmoxName(name string) string {
	n := strings.Join(strings.Fields(name), "-")
	n = dnsUnsafe.ReplaceAllString(n, "")
	return strings.Trim(n, ".-")
}

// planProxmox maps one VM's /api detail onto Proxmox VE qm parameters.
func planProxmox(id string, info vmInfo) proxmoxPlan {
	p := map[string]any{}
	warnings := make([]string, 0)
	src := map[string]any{
		"guest_os": info.GuestOS, "firmware": info.Boot.Type, "cpu_count": info.CPU.Count,
		"cores_per_socket": info.CPU.CoresPerSocket, "memory_MiB": info.Memory.SizeMiB,
		"hardware_version": info.Hardware.Version, "power_state": info.PowerState,
	}

	cores := info.CPU.CoresPerSocket
	if cores < 1 {
		cores = 1
	}
	sockets := int64(1)
	if info.CPU.Count > 0 {
		sockets = info.CPU.Count / cores
		if sockets < 1 || info.CPU.Count%cores != 0 {
			sockets, cores = 1, info.CPU.Count
		}
	}
	p["cores"], p["sockets"] = cores, sockets
	if info.Memory.SizeMiB > 0 {
		p["memory"] = info.Memory.SizeMiB
	}
	if name := proxmoxName(info.Name); name != "" {
		p["name"] = name
	}

	efi := info.Boot.Type == "EFI"
	if efi {
		p["bios"] = "ovmf"
		p["efidisk0"] = "1 (new, empty EFI vars disk)"
		warnings = append(warnings, "efi-state-lost: EFI variables are not migrated; check the boot entry after first start")
	} else {
		p["bios"] = "seabios"
	}

	ostype, windows := proxmoxOSType(info.GuestOS)
	p["ostype"] = ostype

	for i, k := range sortedKeys(info.Nics) {
		n := info.Nics[k]
		model, ok := proxmoxNICModels[n.Type]
		if !ok {
			model = "vmxnet3"
		}
		if n.Type == "VMXNET" || n.Type == "VMXNET2" {
			warnings = append(warnings, fmt.Sprintf("net%d: %s has no Proxmox equivalent, mapped to vmxnet3", i, n.Type))
		}
		val := "model=" + model
		if n.MacAddress != "" {
			val += ",macaddr=" + strings.ToUpper(n.MacAddress)
		}
		p[fmt.Sprintf("net%d", i)] = val
		src[fmt.Sprintf("nic%d", i)] = map[string]any{"type": n.Type, "network": n.Backing.NetworkName, "network_id": n.Backing.Network, "mac": n.MacAddress}
	}

	// Devices are walked the way the importer walks the .vmx: disk and CD-ROM
	// entries in lexical order of their device keys (ide0:0, sata0:0,
	// scsi0:0, scsi0:10, scsi0:2, ...), NVMe disks deferred to the end and
	// attached as SCSI. Each bus numbers its slots in that order, the boot
	// order follows it, and scsihw comes from the controller of the first
	// SCSI device met (LSI Logic -> lsi, PVSCSI -> pvscsi).
	type dev struct {
		vmxKey, key, bus, label, vmdk, iso string
		ctrl                               int64
		capacity                           *int64
		cdrom                              bool
	}
	var devs, nvme []dev
	var unaddressed []string
	for _, k := range sortedKeys(info.Disks) {
		d := info.Disks[k]
		bus, ctrl, unit, known := addressOf(d.busAddress, d.Type)
		x := dev{vmxKey: fmt.Sprintf("%s%d:%d", bus, ctrl, unit), key: k, bus: bus, label: d.Label, vmdk: d.Backing.VmdkFile, capacity: d.Capacity, ctrl: ctrl}
		if !known {
			unaddressed = append(unaddressed, labelOr(d.Label, k))
			x.vmxKey = "~" + k // after every addressed device; its real position is unknown
		}
		if bus == "nvme" {
			nvme = append(nvme, x)
			continue
		}
		devs = append(devs, x)
	}
	for _, k := range sortedKeys(info.Cdroms) {
		cd := info.Cdroms[k]
		bus, ctrl, unit, known := addressOf(cd.busAddress, cd.Type)
		if bus != "sata" {
			bus = "ide"
		}
		x := dev{vmxKey: fmt.Sprintf("%s%d:%d", bus, ctrl, unit), key: k, bus: bus, label: cd.Label, cdrom: true, ctrl: ctrl}
		if !known {
			unaddressed = append(unaddressed, labelOr(cd.Label, k))
			x.vmxKey = "~" + k
		}
		if cd.Backing.Type == "ISO_FILE" {
			x.iso = cd.Backing.IsoFile
		}
		devs = append(devs, x)
	}
	byKey := func(list []dev) {
		sort.SliceStable(list, func(i, j int) bool { return list[i].vmxKey < list[j].vmxKey })
	}
	byKey(devs)
	byKey(nvme)

	adapterType := map[int64]string{}
	for _, k := range sortedKeys(info.ScsiAdapters) {
		if a := info.ScsiAdapters[k]; a.SCSI != nil {
			adapterType[a.SCSI.Bus] = a.Type
		} else if len(info.ScsiAdapters) == 1 {
			adapterType[0] = a.Type
		}
	}
	scsihw := ""
	setSCSIHW := func(d dev) {
		if d.bus != "scsi" {
			return
		}
		want := ""
		switch adapterType[d.ctrl] {
		case "LSILOGIC", "LSILOGICSAS":
			want = "lsi"
		case "PVSCSI":
			want = "pvscsi"
		}
		switch {
		case want == "":
		case scsihw == "":
			scsihw = want
		case scsihw != want:
			warnings = append(warnings, "multiple-scsi-types: Proxmox has one scsihw per VM; kept "+scsihw)
		}
	}
	counts := map[string]int{}
	var boot []string
	place := func(bus string) string {
		slot := fmt.Sprintf("%s%d", bus, counts[bus])
		counts[bus]++
		boot = append(boot, slot)
		return slot
	}
	for _, d := range devs {
		setSCSIHW(d)
		slot := place(d.bus)
		if d.cdrom {
			p[slot] = "none,media=cdrom"
			if d.iso != "" {
				warnings = append(warnings, "cdrom-image-ignored: "+slot+" was "+d.iso)
			}
			continue
		}
		p[slot] = diskValue(d.vmdk, d.capacity)
	}
	for _, d := range nvme {
		slot := place("scsi")
		p[slot] = diskValue(d.vmdk, d.capacity)
		warnings = append(warnings, "nvme-unsupported: "+labelOr(d.label, d.key)+" attached as "+slot)
	}
	if len(unaddressed) > 0 {
		// Without every device's bus address the slot order, boot order and
		// controller cannot be derived the way the importer would; leave them
		// unresolved rather than guess.
		warnings = append(warnings, "device-address-unknown: vCenter reported no bus address for "+strings.Join(unaddressed, ", ")+"; slot numbering may differ and boot order and scsihw are left unset")
		return proxmoxPlan{VM: id, Name: info.Name, Source: src, Proxmox: finishPlan(p, info, &warnings), Warnings: warnings}
	}
	if scsihw == "" && ostype == "l26" {
		scsihw = "virtio-scsi-single"
	}
	if efi && (scsihw == "" || scsihw == "lsi") {
		if windows {
			scsihw = "pvscsi"
		} else {
			scsihw = "virtio-scsi-single"
		}
		warnings = append(warnings, "ovmf-with-lsi-unsupported: OVMF has no LSI driver, scsihw set to "+scsihw)
	}
	if scsihw != "" {
		p["scsihw"] = scsihw
	}
	if len(boot) > 0 {
		p["boot"] = "order=" + strings.Join(boot, ";")
	}
	return proxmoxPlan{VM: id, Name: info.Name, Source: src, Proxmox: finishPlan(p, info, &warnings), Warnings: warnings}
}

// finishPlan adds the settings that do not depend on device order.
func finishPlan(p map[string]any, info vmInfo, warnings *[]string) map[string]any {
	if u := smbiosUUID(info.Identity.BiosUUID); u != "" {
		p["smbios1"] = "uuid=" + u
	}
	for i := range sortedKeys(info.SerialPorts) {
		p[fmt.Sprintf("serial%d", i)] = "socket"
		*warnings = append(*warnings, fmt.Sprintf("serial-port-socket-only: serial%d becomes a socket", i))
	}
	if info.PowerState != "" && info.PowerState != "POWERED_OFF" {
		*warnings = append(*warnings, "guest-is-running: shut the VM down before the final import")
	}
	return p
}

// smbiosUUID converts a VMware BIOS UUID the way the Proxmox importer does
// (smbios1_uuid): the first three fields are byte-swapped, because SMBIOS
// stores them little-endian, so the guest keeps seeing the same UUID.
func smbiosUUID(raw string) string {
	var hexDigits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			hexDigits.WriteRune(r)
		}
	}
	h := hexDigits.String()
	if len(h) != 32 {
		return ""
	}
	return h[6:8] + h[4:6] + h[2:4] + h[0:2] + "-" + h[10:12] + h[8:10] + "-" + h[14:16] + h[12:14] + "-" + h[16:20] + "-" + h[20:32]
}

// addressOf maps a vSphere device address to (bus, controller, unit).
func addressOf(a busAddress, typ string) (bus string, ctrl, unit int64, known bool) {
	switch {
	case a.SCSI != nil:
		return "scsi", a.SCSI.Bus, a.SCSI.Unit, true
	case a.SATA != nil:
		return "sata", a.SATA.Bus, a.SATA.Unit, true
	case a.NVMe != nil:
		return "nvme", a.NVMe.Bus, a.NVMe.Unit, true
	case a.IDE != nil:
		ctrl, unit := int64(1), int64(1)
		if a.IDE.Primary {
			ctrl = 0
		}
		if a.IDE.Master {
			unit = 0
		}
		return "ide", ctrl, unit, true
	}
	// The address is optional in the vendor schema: the bus type is still
	// known from the device type, its position is not.
	switch strings.ToLower(typ) {
	case "sata", "ide", "nvme":
		return strings.ToLower(typ), 0, 0, false
	}
	return "scsi", 0, 0, false
}

func diskValue(vmdk string, capacity *int64) map[string]any {
	v := map[string]any{"source_vmdk": vmdk}
	if capacity != nil && *capacity > 0 {
		v["size"] = fmt.Sprintf("%dG", (*capacity+(1<<30)-1)>>30)
		v["size_bytes"] = *capacity
	} else {
		v["size"] = nil // vCenter did not report it (inaccessible disk)
	}
	return v
}

// --- small helpers ---

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func labelOr(label, key string) string {
	if label != "" {
		return label
	}
	return key
}

func roundTo(v float64, places int) float64 {
	p := 1.0
	for i := 0; i < places; i++ {
		p *= 10
	}
	if v < 0 {
		return -float64(int64(-v*p+0.5)) / p
	}
	return float64(int64(v*p+0.5)) / p
}

func nowUTC() time.Time { return time.Now().UTC() }
