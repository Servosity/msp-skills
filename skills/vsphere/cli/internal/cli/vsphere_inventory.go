// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (not generated): the shared live inventory walk behind the novel
// commands (migration, snapshots, licence, rvtools). It reads only the GET
// endpoints declared in spec.yaml plus two VI/JSON reads: the LicenseManager
// licences and VirtualMachine config (the encryption keys, migration blockers).
//
// VMs are listed per host (clusters -> hosts -> VMs). That maps every VM to its
// host and cluster, and keeps each /api/vcenter/vm call far below the 4000-VM
// result cap that a single unfiltered list hits on large estates.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"vsphere-pp-cli/internal/client"
	"vsphere-pp-cli/internal/cliutil"
)

// viRoot is the VI/JSON API root the spec uses: release 8.0.1.0, served by
// every vCenter 8.0U1 or later. Kept as a path (not a bare dotted literal) so
// it is never mistaken for a hostname.
const viRoot = "/sdk/vim25/8.0.1.0"

// fetchFailure is one read that failed. Failed reads never become zero-valued
// rows in totals; they are listed so the caller can see the gap.
type fetchFailure struct {
	Item  string `json:"item"`
	Path  string `json:"path"`
	Error string `json:"error"`
}

type invScope struct {
	Cluster string // cluster id (domain-c8)
	Host    string // host id (host-10)
	Folder  string // VM folder id (group-v3)
	VM      string // single VM id (vm-42)
}

type invHost struct {
	ID              string `json:"host"`
	Name            string `json:"name"`
	ConnectionState string `json:"connection_state"`
	PowerState      string `json:"power_state"`
	Cluster         string `json:"cluster,omitempty"`
	ClusterName     string `json:"cluster_name,omitempty"`
}

type invVM struct {
	ID          string `json:"vm"`
	Name        string `json:"name"`
	PowerState  string `json:"power_state"`
	CPUCount    *int64 `json:"cpu_count"`       // vCenter omits it when the VM config is unavailable
	MemoryMiB   *int64 `json:"memory_size_MiB"` // ditto
	Host        string `json:"host,omitempty"`
	HostName    string `json:"host_name,omitempty"`
	Cluster     string `json:"cluster,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
}

type inventory struct {
	Clusters []clusterSummary
	Hosts    []invHost
	VMs      []invVM
	Failures []fetchFailure
}

type clusterSummary struct {
	ID         string `json:"cluster"`
	Name       string `json:"name"`
	HAEnabled  bool   `json:"ha_enabled"`
	DRSEnabled bool   `json:"drs_enabled"`
}

// getJSON reads one endpoint, bypassing the response cache so every run sees
// the vCenter as it is now.
func getJSON(ctx context.Context, c *client.Client, path string, params map[string]string, out any) error {
	data, err := c.GetNoCache(ctx, path, params)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decoding %s: %w", path, err)
	}
	return nil
}

func withQuery(path, key, value string) string {
	if value == "" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
}

func esc(id string) string { return url.PathEscape(id) }

// walkInventory lists clusters, hosts and VMs inside scope.
func walkInventory(ctx context.Context, c *client.Client, scope invScope, concurrency int, withVMs bool) (*inventory, error) {
	inv := &inventory{}
	if scope.VM != "" {
		var vms []invVM
		if err := getJSON(ctx, c, withQuery("/api/vcenter/vm", "vms", scope.VM), nil, &vms); err != nil {
			return nil, err
		}
		if len(vms) == 0 {
			return nil, notFoundErr(fmt.Errorf("VM %q not found (use an id like vm-42 from 'vm list')", scope.VM))
		}
		inv.VMs = vms
		return inv, nil
	}

	clusterPath := withQuery("/api/vcenter/cluster", "clusters", scope.Cluster)
	if err := getJSON(ctx, c, clusterPath, nil, &inv.Clusters); err != nil {
		return nil, err
	}
	if scope.Cluster != "" && len(inv.Clusters) == 0 {
		return nil, notFoundErr(fmt.Errorf("cluster %q not found (use an id like domain-c8 from 'cluster list')", scope.Cluster))
	}

	hostCluster := map[string]clusterSummary{}
	type clusterHosts struct {
		cluster clusterSummary
		hosts   []invHost
	}
	res, errs := cliutil.FanoutRun(ctx, inv.Clusters, func(cl clusterSummary) string { return cl.ID },
		func(ctx context.Context, cl clusterSummary) (clusterHosts, error) {
			var hosts []invHost
			err := getJSON(ctx, c, withQuery("/api/vcenter/host", "clusters", cl.ID), nil, &hosts)
			return clusterHosts{cluster: cl, hosts: hosts}, err
		}, cliutil.WithConcurrency(concurrency))
	// Cluster membership is what every per-cluster figure hangs on: a cluster
	// whose host list cannot be read would otherwise show its hosts as
	// standalone, so it fails the whole walk instead.
	if len(errs) > 0 {
		return nil, fmt.Errorf("listing hosts of cluster %s: %w", errs[0].Source, errs[0].Err)
	}
	for _, r := range res {
		for _, h := range r.Value.hosts {
			hostCluster[h.ID] = r.Value.cluster
		}
	}

	var hosts []invHost
	switch {
	case scope.Cluster != "":
		for _, r := range res {
			hosts = append(hosts, r.Value.hosts...)
		}
	default:
		if err := getJSON(ctx, c, withQuery("/api/vcenter/host", "hosts", scope.Host), nil, &hosts); err != nil {
			return nil, err
		}
		if scope.Host != "" && len(hosts) == 0 {
			return nil, notFoundErr(fmt.Errorf("host %q not found (use an id like host-10 from 'host')", scope.Host))
		}
	}
	for i := range hosts {
		if cl, ok := hostCluster[hosts[i].ID]; ok {
			hosts[i].Cluster, hosts[i].ClusterName = cl.ID, cl.Name
		}
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Name < hosts[j].Name })
	inv.Hosts = hosts
	if !withVMs {
		return inv, nil
	}

	vmRes, vmErrs := cliutil.FanoutRun(ctx, hosts, func(h invHost) string { return h.ID },
		func(ctx context.Context, h invHost) ([]invVM, error) {
			path := withQuery(withQuery("/api/vcenter/vm", "hosts", h.ID), "folders", scope.Folder)
			var vms []invVM
			if err := getJSON(ctx, c, path, nil, &vms); err != nil {
				return nil, err
			}
			for i := range vms {
				vms[i].Host, vms[i].HostName = h.ID, h.Name
				vms[i].Cluster, vms[i].ClusterName = h.Cluster, h.ClusterName
			}
			return vms, nil
		}, cliutil.WithConcurrency(concurrency))
	// The VM lists are every total's denominator: a host whose VMs cannot be
	// listed would silently shrink the estate, so it fails the walk.
	if len(vmErrs) > 0 {
		return nil, fmt.Errorf("listing VMs on host %s: %w", vmErrs[0].Source, vmErrs[0].Err)
	}
	// A VM that vMotions during the walk can be listed under two hosts;
	// keep it once (first host seen) so no total counts it twice.
	seen := map[string]bool{}
	for _, r := range vmRes {
		for _, v := range r.Value {
			if !seen[v.ID] {
				seen[v.ID] = true
				inv.VMs = append(inv.VMs, v)
			}
		}
	}
	// A VM can have no host (vCenter allows runtime.host to be unset, e.g. an
	// orphaned or inaccessible VM). For a whole-vCenter walk, one unfiltered
	// list catches those; they are kept with no placement.
	if scope.Cluster == "" && scope.Host == "" {
		var all []invVM
		err := getJSON(ctx, c, withQuery("/api/vcenter/vm", "folders", scope.Folder), nil, &all)
		switch {
		case err != nil && strings.Contains(strings.ToLower(err.Error()), "unable_to_allocate_resource"):
			inv.Failures = append(inv.Failures, fetchFailure{Item: "vcenter", Path: "/api/vcenter/vm",
				Error: "more than 4000 VMs: VMs without a host could not be listed (VMs on hosts are complete)"})
		case err != nil:
			return nil, fmt.Errorf("listing VMs: %w", err)
		}
		for _, v := range all {
			if !seen[v.ID] {
				seen[v.ID] = true
				inv.VMs = append(inv.VMs, v)
			}
		}
	}
	sort.Slice(inv.VMs, func(i, j int) bool { return inv.VMs[i].Name < inv.VMs[j].Name })
	return inv, nil
}

// perVM runs fn for every VM with bounded concurrency. Results keep VM order;
// failures are recorded instead of producing a value.
func perVM[T any](ctx context.Context, vms []invVM, concurrency int, fn func(context.Context, invVM) (T, error)) (map[string]T, []fetchFailure) {
	res, errs := cliutil.FanoutRun(ctx, vms, func(v invVM) string { return v.ID }, fn, cliutil.WithConcurrency(concurrency))
	out := make(map[string]T, len(res))
	for _, r := range res {
		out[r.Source] = r.Value
	}
	failures := make([]fetchFailure, 0, len(errs))
	for _, e := range errs {
		failures = append(failures, fetchFailure{Item: e.Source, Error: errText(e.Err)})
	}
	return out, failures
}

// viStatus says whether the VI/JSON API answers on this vCenter, plus the
// vCenter identity it reports (ServiceInstance content "about").
type viStatus struct {
	Available      bool   `json:"available"`
	Version        string `json:"vcenter_version,omitempty"`
	Build          string `json:"vcenter_build,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Name           string `json:"-"`
	FullName       string `json:"-"`
	APIVersion     string `json:"-"`
	Vendor         string `json:"-"`
	InstanceUUID   string `json:"-"`
	LicenseManager string `json:"-"`
}

func probeVI(ctx context.Context, c *client.Client) viStatus {
	var content struct {
		About struct {
			Name         string `json:"name"`
			FullName     string `json:"fullName"`
			Vendor       string `json:"vendor"`
			Version      string `json:"version"`
			Build        string `json:"build"`
			APIVersion   string `json:"apiVersion"`
			InstanceUUID string `json:"instanceUuid"`
		} `json:"about"`
		LicenseManager *moRef `json:"licenseManager"`
	}
	if err := getJSON(ctx, c, viRoot+"/ServiceInstance/ServiceInstance/content", nil, &content); err != nil {
		if isNotFound(err) {
			return viStatus{Reason: "this vCenter has no VI/JSON API at " + viRoot + " (it needs vCenter 8.0 Update 1 or later)"}
		}
		return viStatus{Reason: "VI/JSON API at " + viRoot + " did not answer: " + errText(err)}
	}
	a := content.About
	st := viStatus{Available: true, Version: a.Version, Build: a.Build, Name: a.Name, FullName: a.FullName,
		APIVersion: a.APIVersion, Vendor: a.Vendor, InstanceUUID: a.InstanceUUID, LicenseManager: "LicenseManager"}
	if content.LicenseManager != nil && content.LicenseManager.Value != "" {
		st.LicenseManager = content.LicenseManager.Value
	}
	return st
}

// detailWant selects the per-VM reads gatherDetails makes.
type detailWant struct {
	Info, Tools, Identity, Guest, GuestNet bool // Automation API
	Snapshots, Layout, Storage, Summary    bool // VI/JSON (skipped when unavailable)
	Config                                 bool // VI/JSON VirtualMachine config
}

// vmDetail holds one VM's reads. A nil field means the read was not wanted,
// is unavailable (VI/JSON on vCenter < 8.0U1, guest data without Tools), or
// failed; failures are listed in Failures.
type vmDetail struct {
	Info      *vmInfo
	Tools     *toolsInfo
	Identity  *guestIdentity
	Guest     map[string]guestFilesystem
	GuestNet  []guestInterface
	Snapshots *snapshotInfo
	Layout    *layoutEx
	Storage   *storageInfo
	Summary   *vmSummary
	Config    *vmCryptoConfig
	Failures  []fetchFailure
}

type guestIdentity struct {
	Name     string `json:"name"`
	Family   string `json:"family"`
	HostName string `json:"host_name"`
	IP       string `json:"ip_address"`
	FullName struct {
		DefaultMessage string `json:"default_message"`
	} `json:"full_name"`
}

type guestInterface struct {
	Nic        string `json:"nic"`
	MacAddress string `json:"mac_address"`
	IP         struct {
		IPAddresses []struct {
			IPAddress string `json:"ip_address"`
		} `json:"ip_addresses"`
	} `json:"ip"`
}

func gatherDetails(ctx context.Context, c *client.Client, vms []invVM, want detailWant, vi viStatus, concurrency int) (map[string]*vmDetail, []fetchFailure) {
	details, errs := perVM(ctx, vms, concurrency, func(ctx context.Context, vm invVM) (*vmDetail, error) {
		want := want // per-VM copy: guest reads are switched off per VM below
		d := &vmDetail{}
		base := "/api/vcenter/vm/" + esc(vm.ID)
		// read decodes path into out. It reports false on failure and when
		// vCenter answers JSON null (a VI/JSON property that is unset).
		read := func(path string, out any, guestRead bool) bool {
			data, err := c.GetNoCache(ctx, path, nil)
			if err != nil {
				// Guest reads need VMware Tools running; 503/404 there is
				// "no guest data", not a failure of this command.
				if !(guestRead && (isUnavailable(err) || isNotFound(err))) {
					d.Failures = append(d.Failures, fetchFailure{Item: vm.ID, Path: path, Error: errText(err)})
				}
				return false
			}
			// An empty body is an unset property too (the Pulse VI/JSON
			// client treats it the same way as JSON null).
			if body := strings.TrimSpace(string(data)); body == "null" || body == "" {
				return false
			}
			if err := json.Unmarshal(data, out); err != nil {
				d.Failures = append(d.Failures, fetchFailure{Item: vm.ID, Path: path, Error: "decoding: " + err.Error()})
				return false
			}
			return true
		}
		if want.Info {
			var v vmInfo
			if read(base, &v, false) {
				d.Info = &v
			}
		}
		// Guest data exists only while VMware Tools runs in a powered-on
		// guest; vCenter answers 503 otherwise, which the client would retry
		// with backoff. Read tools state first and skip guest reads that
		// cannot succeed.
		wantGuest := want.Identity || want.Guest || want.GuestNet
		if want.Tools || wantGuest {
			var v toolsInfo
			if read(base+"/tools", &v, false) {
				d.Tools = &v
			}
		}
		if vm.PowerState != "POWERED_ON" || d.Tools == nil || d.Tools.RunState == "NOT_RUNNING" {
			want.Identity, want.Guest, want.GuestNet = false, false, false
		}
		if want.Identity {
			var v guestIdentity
			if read(base+"/guest/identity", &v, true) {
				d.Identity = &v
			}
		}
		if want.Guest {
			var v map[string]guestFilesystem
			if read(base+"/guest/local-filesystem", &v, true) {
				d.Guest = v
			}
		}
		if want.GuestNet {
			var v []guestInterface
			if read(base+"/guest/networking/interfaces", &v, true) {
				d.GuestNet = v
			}
		}
		if vi.Available {
			vbase := viRoot + "/VirtualMachine/" + esc(vm.ID)
			if want.Snapshots {
				// The snapshot property is null when the VM has no
				// snapshots ("valid if snapshots have been created"), so
				// null is an empty tree, not an unknown.
				var v snapshotInfo
				before := len(d.Failures)
				if read(vbase+"/snapshot", &v, false) || len(d.Failures) == before {
					d.Snapshots = &v
				}
			}
			if want.Layout {
				var v layoutEx
				if read(vbase+"/layoutEx", &v, false) {
					d.Layout = &v
				}
			}
			if want.Storage {
				var v storageInfo
				if read(vbase+"/storage", &v, false) {
					d.Storage = &v
				}
			}
			if want.Summary {
				var v vmSummary
				if read(vbase+"/summary", &v, false) {
					d.Summary = &v
				}
			}
			if want.Config {
				var v vmCryptoConfig
				if read(vbase+"/config", &v, false) {
					d.Config = &v
				}
			}
		}
		return d, nil
	})
	failures := append([]fetchFailure{}, errs...)
	for _, vm := range vms {
		if d := details[vm.ID]; d != nil {
			failures = append(failures, d.Failures...)
		}
	}
	return details, failures
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if i := strings.Index(msg, "\nhint:"); i > 0 {
		msg = msg[:i]
	}
	if len(msg) > 400 {
		msg = msg[:400]
	}
	return msg
}

func warnFailures(w io.Writer, what string, failures []fetchFailure, total int) {
	if len(failures) == 0 {
		return
	}
	items := map[string]bool{}
	for _, f := range failures {
		items[f.Item] = true
	}
	fmt.Fprintf(w, "warning: %d %s read(s) failed across %d item(s) of %d; affected figures exclude them (see fetch_failures)\n", len(failures), what, len(items), total)
}

// --- typed views of the vendor responses the novel commands read ---

type moRef struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type vmInfo struct {
	Name       string `json:"name"`
	GuestOS    string `json:"guest_OS"`
	PowerState string `json:"power_state"`
	Hardware   struct {
		Version string `json:"version"`
	} `json:"hardware"`
	Identity struct {
		BiosUUID     string `json:"bios_uuid"`
		InstanceUUID string `json:"instance_uuid"`
	} `json:"identity"`
	Boot struct {
		Type string `json:"type"`
	} `json:"boot"`
	CPU struct {
		Count          int64 `json:"count"`
		CoresPerSocket int64 `json:"cores_per_socket"`
	} `json:"cpu"`
	Memory struct {
		SizeMiB int64 `json:"size_MiB"`
	} `json:"memory"`
	Disks map[string]struct {
		busAddress
		Label    string `json:"label"`
		Type     string `json:"type"`
		Capacity *int64 `json:"capacity"` // vCenter omits it for an inaccessible disk
		Backing  struct {
			Type     string `json:"type"`
			VmdkFile string `json:"vmdk_file"`
		} `json:"backing"`
	} `json:"disks"`
	Nics map[string]struct {
		Label      string `json:"label"`
		Type       string `json:"type"`
		MacAddress string `json:"mac_address"`
		State      string `json:"state"`
		Backing    struct {
			Type        string `json:"type"`
			Network     string `json:"network"`
			NetworkName string `json:"network_name"`
		} `json:"backing"`
	} `json:"nics"`
	Cdroms map[string]struct {
		busAddress
		Label          string `json:"label"`
		Type           string `json:"type"`
		State          string `json:"state"`
		StartConnected bool   `json:"start_connected"`
		Backing        struct {
			Type       string `json:"type"`
			IsoFile    string `json:"iso_file"`
			HostDevice string `json:"host_device"`
		} `json:"backing"`
	} `json:"cdroms"`
	ScsiAdapters map[string]struct {
		Label string   `json:"label"`
		Type  string   `json:"type"`
		SCSI  *busUnit `json:"scsi"` // the adapter's SCSI bus number
	} `json:"scsi_adapters"`
	SerialPorts map[string]struct {
		Label string `json:"label"`
	} `json:"serial_ports"`
}

// busAddress is where a disk or CD-ROM sits (one of these is set, by bus type).
type busAddress struct {
	IDE *struct {
		Primary bool `json:"primary"`
		Master  bool `json:"master"`
	} `json:"ide"`
	SCSI *busUnit `json:"scsi"`
	SATA *busUnit `json:"sata"`
	NVMe *busUnit `json:"nvme"`
}

type busUnit struct {
	Bus  int64 `json:"bus"`
	Unit int64 `json:"unit"`
}

type toolsInfo struct {
	RunState      string `json:"run_state"`
	VersionStatus string `json:"version_status"`
	Version       string `json:"version"`
}

type snapshotTree struct {
	Snapshot    moRef          `json:"snapshot"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	ID          int64          `json:"id"`
	CreateTime  string         `json:"createTime"`
	State       string         `json:"state"`
	Quiesced    bool           `json:"quiesced"`
	Children    []snapshotTree `json:"childSnapshotList"`
}

type snapshotInfo struct {
	CurrentSnapshot *moRef         `json:"currentSnapshot"`
	RootSnapshots   []snapshotTree `json:"rootSnapshotList"`
}

type layoutEx struct {
	File []struct {
		Key  int64  `json:"key"`
		Name string `json:"name"`
		Type string `json:"type"`
		Size int64  `json:"size"`
	} `json:"file"`
	Disk     []layoutDisk `json:"disk"`
	Snapshot []struct {
		Key  moRef        `json:"key"`
		Disk []layoutDisk `json:"disk"`
	} `json:"snapshot"`
}

type layoutDisk struct {
	Key   int64 `json:"key"`
	Chain []struct {
		FileKey []int64 `json:"fileKey"`
	} `json:"chain"`
}

// vmSummary is the part of VI/JSON VirtualMachine.summary the commands read.
type vmSummary struct {
	Config struct {
		Template          bool   `json:"template"`
		GuestFullName     string `json:"guestFullName"`
		CPUReservation    *int64 `json:"cpuReservation"`
		MemoryReservation *int64 `json:"memoryReservation"`
	} `json:"config"`
	Guest *struct {
		ToolsStatus string `json:"toolsStatus"`
	} `json:"guest"`
	Runtime struct {
		ConsolidationNeeded *bool `json:"consolidationNeeded"`
		// CryptoState (vim.vm.RuntimeInfo, vSphere 6.7+) is set only for an
		// encrypted VM: "unlocked" or "locked".
		CryptoState string `json:"cryptoState"`
	} `json:"runtime"`
}

// vmCryptoConfig is the part of VI/JSON VirtualMachine.config the encryption
// check reads: the VM's keyId and every device backing's keyId, the parent
// chain included. Each is a vim.encryption.CryptoKeyId, set only when that
// part is encrypted (VirtualMachineConfigInfo.keyId,
// VirtualDiskFlatVer2BackingInfo.keyId and the other disk backings).
type vmCryptoConfig struct {
	KeyID    *json.RawMessage `json:"keyId"`
	Hardware struct {
		Device []struct {
			Backing *cryptoBacking `json:"backing"`
		} `json:"device"`
	} `json:"hardware"`
}

type cryptoBacking struct {
	KeyID  *json.RawMessage `json:"keyId"`
	Parent *cryptoBacking   `json:"parent"`
}

// encrypted reports any keyId that is present and not JSON null.
func (c *vmCryptoConfig) encrypted() bool {
	if c == nil {
		return false
	}
	if keySet(c.KeyID) {
		return true
	}
	for _, dev := range c.Hardware.Device {
		for b := dev.Backing; b != nil; b = b.Parent {
			if keySet(b.KeyID) {
				return true
			}
		}
	}
	return false
}

func keySet(raw *json.RawMessage) bool {
	return raw != nil && strings.TrimSpace(string(*raw)) != "null"
}

type storageInfo struct {
	PerDatastoreUsage []struct {
		Datastore   moRef `json:"datastore"`
		Committed   int64 `json:"committed"`
		Uncommitted int64 `json:"uncommitted"`
		Unshared    int64 `json:"unshared"`
	} `json:"perDatastoreUsage"`
}

type hostSummary struct {
	Hardware *struct {
		Vendor        string `json:"vendor"`
		Model         string `json:"model"`
		CPUModel      string `json:"cpuModel"`
		CPUMhz        int64  `json:"cpuMhz"`
		NumCPUPkgs    int64  `json:"numCpuPkgs"`
		NumCPUCores   int64  `json:"numCpuCores"`
		NumCPUThreads int64  `json:"numCpuThreads"`
		MemorySize    int64  `json:"memorySize"`
		NumNics       int64  `json:"numNics"`
		UUID          string `json:"uuid"`
	} `json:"hardware"`
	Config struct {
		Name    string `json:"name"`
		Product struct {
			FullName string `json:"fullName"`
			Version  string `json:"version"`
			Build    string `json:"build"`
		} `json:"product"`
	} `json:"config"`
}

type guestFilesystem struct {
	Capacity   int64  `json:"capacity"`
	FreeSpace  int64  `json:"free_space"`
	Filesystem string `json:"filesystem"`
}

// isNotFound reports a 404 from vCenter (missing object or API).
func isNotFound(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

// isUnavailable reports vCenter's 503 "service unavailable", which the guest
// endpoints return while VMware Tools is not running.
func isUnavailable(err error) bool {
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 503
}

// networkNames maps network ids to names (/api/vcenter/network). Distributed
// portgroup NIC backings carry only the id, so names come from here.
func networkNames(ctx context.Context, c *client.Client) (map[string]string, error) {
	var nets []struct {
		Network string `json:"network"`
		Name    string `json:"name"`
	}
	if err := getJSON(ctx, c, "/api/vcenter/network", nil, &nets); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(nets))
	for _, n := range nets {
		out[n.Network] = n.Name
	}
	return out, nil
}

// fillNetworkNames sets each NIC's network name from its network id when the
// backing did not carry one.
func fillNetworkNames(info *vmInfo, names map[string]string) {
	for k, n := range info.Nics {
		if n.Backing.NetworkName == "" && names[n.Backing.Network] != "" {
			n.Backing.NetworkName = names[n.Backing.Network]
			info.Nics[k] = n
		}
	}
}

// datastoreTypes reads vCenter's datastore list as name to type. A name is
// unique only inside its datacenter (vim.Datastore), and a disk path names a
// datastore only by name, so a name listed with two types maps to "": which
// one the disk is on cannot be told, and the check reads it as unknown.
func datastoreTypes(ctx context.Context, c *client.Client) (map[string]string, error) {
	var list []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := getJSON(ctx, c, "/api/vcenter/datastore", nil, &list); err != nil {
		return nil, err
	}
	types := make(map[string]string, len(list))
	for _, d := range list {
		if prev, seen := types[d.Name]; seen && prev != d.Type {
			types[d.Name] = ""
			continue
		}
		types[d.Name] = d.Type
	}
	return types, nil
}
