// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: RVTools-format CSV tabs from both vSphere JSON APIs.
// pp:data-source live
//
// File names and columns follow awslabs/export-for-vcenter, which writes the
// RVTools CSV layout that AWS Transform and other assessment tools read
// (src/exporters/csv_exporter.py get_csv_headers and its collectors).
// vSnapshot is an addition in RVTools column style.

package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/cliutil"
	"vsphere-pp-cli/internal/credkit"
)

type rvTab struct {
	File    string
	Columns []string
	Rows    [][]string
}

type rvToolsView struct {
	OutDir        string                         `json:"out_dir,omitempty"`
	Files         []string                       `json:"files,omitempty"`
	Tabs          map[string][]map[string]string `json:"tabs,omitempty"`
	VMs           int                            `json:"vms"`
	Hosts         int                            `json:"hosts"`
	VIJSON        viStatus                       `json:"vi_json"`
	FetchFailures []fetchFailure                 `json:"fetch_failures"`
}

var rvPower = map[string]string{"POWERED_ON": "poweredOn", "POWERED_OFF": "poweredOff", "SUSPENDED": "suspended"}

// rvToolsStatus fills the vTools "Tools" column. awslabs reads
// vm.guest.toolsStatus; the VI/JSON VM summary carries the same value. Without
// it (vCenter < 8.0U1) the /api tools state is mapped only where the meaning is
// unambiguous; anything else stays blank rather than claiming toolsOk.
func rvToolsStatus(sum *vmSummary, t *toolsInfo) string {
	if sum != nil && sum.Guest != nil && sum.Guest.ToolsStatus != "" {
		return sum.Guest.ToolsStatus
	}
	if t == nil {
		return ""
	}
	switch {
	case t.VersionStatus == "NOT_INSTALLED":
		return "toolsNotInstalled"
	case t.RunState == "NOT_RUNNING":
		return "toolsNotRunning"
	case t.VersionStatus == "TOO_OLD_UNSUPPORTED" || t.VersionStatus == "SUPPORTED_OLD":
		return "toolsOld"
	case (t.RunState == "RUNNING" || t.RunState == "EXECUTING_SCRIPTS") && (t.VersionStatus == "CURRENT" || t.VersionStatus == "UNMANAGED" || t.VersionStatus == "SUPPORTED_NEW"):
		return "toolsOk"
	}
	return ""
}

func firstOf(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func mib(b int64) string { return strconv.FormatInt(b>>20, 10) }

func i64(v int64) string { return strconv.FormatInt(v, 10) }

func optI64(v *int64) string {
	if v == nil {
		return ""
	}
	return i64(*v)
}

func newNovelRvtoolsExportCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	var out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write RVTools-format CSV tabs (vInfo, vCPU, vMemory, vDisk, vPartition, vNetwork, vTools, vHost, vSource, vSnapshot)",
		Long: `Builds the RVTools CSV export layout from macOS or Linux, without a Windows jump box.

File names and columns follow awslabs/export-for-vcenter (the RVTools subset AWS
Transform reads), plus a vSnapshot tab. With --out the tabs are written as
RVTools_tab<name>.csv files (mode 0600); without --out they print as JSON.
Columns this CLI cannot read (switch names, partition disk keys) stay empty.
DNS name, IP addresses and guest filesystems need VMware Tools in the guest.
vHost, vSource, vSnapshot and the Template, configured-OS, reservation and Tools
columns need the VI/JSON API (vCenter 8.0 Update 1 or later); on older vCenters
they stay empty.`,
		Example: strings.Trim(`
  vsphere-cli rvtools export --out ./acme-rvtools
  vsphere-cli rvtools export --cluster domain-c8 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "rvtools export")
			}
			if err := sf.validate(); err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := vcenterClient(cmd, flags)
			if err != nil {
				return err
			}
			inv, err := walkInventory(ctx, c, sf.scope, sf.concurrency, true)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			vi := probeVI(ctx, c)
			details, failures := gatherDetails(ctx, c, inv.VMs,
				detailWant{Info: true, Tools: true, Identity: true, Guest: true, GuestNet: true, Snapshots: true, Summary: true}, vi, sf.concurrency)
			view := rvToolsView{VMs: len(inv.VMs), Hosts: len(inv.Hosts), VIJSON: vi, FetchFailures: nonNilFailures(append(inv.Failures, failures...))}

			tab := func(name string, cols ...string) *rvTab {
				return &rvTab{File: "RVTools_tab" + name + ".csv", Columns: cols}
			}
			vInfo := tab("vInfo", "VM", "Powerstate", "Template", "DNS Name", "CPUs", "Memory", "Total disk capacity MiB",
				"NICs", "Disks", "Host", "OS according to the configuration file", "OS according to the VMware Tools",
				"VI SDK API Version", "Primary IP Address", "VM ID", "VM UUID", "VI SDK Server type", "VI SDK Server", "VI SDK UUID")
			vCPU := tab("vCPU", "VM", "CPUs", "Sockets", "Reservation")
			vMemory := tab("vMemory", "VM", "Size MiB", "Reservation")
			vDisk := tab("vDisk", "VM", "Disk", "Disk Key", "Disk Path", "Capacity MiB")
			vPartition := tab("vPartition", "VM", "Disk Key", "Disk", "Capacity MiB", "Free MiB")
			vNetwork := tab("vNetwork", "VM", "Network", "IPv4 Address", "IPv6 Address", "Switch", "Mac Address")
			vTools := tab("vTools", "VM", "Tools")
			vHost := tab("vHost", "Host", "# CPU", "# Cores", "# Memory", "# NICs", "Vendor", "Model", "Object ID", "UUID", "VI SDK UUID")
			vSource := tab("vSource", "Name", "API version", "Vendor", "VI SDK UUID")
			vSnapshot := tab("vSnapshot", "VM", "Name", "Description", "Date / time", "Quiesced", "State")

			names, nerr := networkNames(ctx, c)
			if nerr != nil {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "networks", Path: "/api/vcenter/network", Error: errText(nerr)})
			}
			for _, vm := range inv.VMs {
				d := details[vm.ID]
				if d == nil {
					d = &vmDetail{}
				}
				nics, disks, diskTotal, guestOS, sockets, uuid := "", "", "", "", "", ""
				if info := d.Info; info != nil {
					fillNetworkNames(info, names)
					var total int64
					measured := true
					for n, k := range sortedKeys(info.Disks) {
						dk := info.Disks[k]
						capMiB := ""
						if dk.Capacity != nil {
							total += *dk.Capacity
							capMiB = mib(*dk.Capacity)
						} else {
							measured = false // inaccessible disk: size unknown
						}
						// awslabs numbers disks 1..n per VM and keeps the device key apart.
						vDisk.Rows = append(vDisk.Rows, []string{vm.Name, strconv.Itoa(n + 1), k, dk.Backing.VmdkFile, capMiB})
					}
					nics, disks = strconv.Itoa(len(info.Nics)), strconv.Itoa(len(info.Disks))
					if measured {
						diskTotal = mib(total)
					}
					uuid = info.Identity.BiosUUID
					if cps := info.CPU.CoresPerSocket; cps > 0 && info.CPU.Count%cps == 0 {
						sockets = i64(info.CPU.Count / cps)
					}
					ipsByMAC := map[string][2][]string{}
					for _, g := range d.GuestNet {
						v4, v6 := []string{}, []string{}
						for _, a := range g.IP.IPAddresses {
							if strings.Contains(a.IPAddress, ":") {
								v6 = append(v6, a.IPAddress)
							} else {
								v4 = append(v4, a.IPAddress)
							}
						}
						ipsByMAC[strings.ToLower(g.MacAddress)] = [2][]string{v4, v6}
					}
					for _, k := range sortedKeys(info.Nics) {
						n := info.Nics[k]
						ips := ipsByMAC[strings.ToLower(n.MacAddress)]
						// awslabs emits the first address of each family per NIC.
						vNetwork.Rows = append(vNetwork.Rows, []string{vm.Name, n.Backing.NetworkName, firstOf(ips[0]), firstOf(ips[1]), "", n.MacAddress})
					}
				}
				template, cpuRes, memRes := "", "", ""
				if sum := d.Summary; sum != nil {
					// awslabs: config.guestFullName and config.template; the
					// VI/JSON VM summary config carries the same fields.
					guestOS = sum.Config.GuestFullName
					template = "False"
					if sum.Config.Template {
						template = "True"
					}
					cpuRes, memRes = optI64(sum.Config.CPUReservation), optI64(sum.Config.MemoryReservation)
				}
				dnsName, toolsOS, ip := "", "", ""
				if id := d.Identity; id != nil {
					dnsName, toolsOS, ip = id.HostName, id.FullName.DefaultMessage, id.IP
				}
				vInfo.Rows = append(vInfo.Rows, []string{vm.Name, rvPower[vm.PowerState], template, dnsName, optI64(vm.CPUCount), optI64(vm.MemoryMiB),
					diskTotal, nics, disks, vm.HostName, guestOS, toolsOS, vi.APIVersion, ip, vm.ID, uuid, vi.Name, vi.FullName, vi.InstanceUUID})
				vCPU.Rows = append(vCPU.Rows, []string{vm.Name, optI64(vm.CPUCount), sockets, cpuRes})
				vMemory.Rows = append(vMemory.Rows, []string{vm.Name, optI64(vm.MemoryMiB), memRes})
				vTools.Rows = append(vTools.Rows, []string{vm.Name, rvToolsStatus(d.Summary, d.Tools)})
				for _, mount := range sortedKeys(d.Guest) {
					fs := d.Guest[mount]
					vPartition.Rows = append(vPartition.Rows, []string{vm.Name, "", mount, mib(fs.Capacity), mib(fs.FreeSpace)})
				}
				if d.Snapshots != nil {
					for _, sn := range flattenSnapshots(vm, *d.Snapshots, nowUTC()) {
						vSnapshot.Rows = append(vSnapshot.Rows, []string{vm.Name, sn.Name, sn.Description, sn.CreateTime, strconv.FormatBool(sn.Quiesced), sn.PowerState})
					}
				}
			}
			if vi.Available {
				res, errs := cliutil.FanoutRun(ctx, inv.Hosts, func(h invHost) string { return h.ID },
					func(ctx context.Context, h invHost) (hostSummary, error) {
						var s hostSummary
						err := getJSON(ctx, c, viRoot+"/HostSystem/"+esc(h.ID)+"/summary", nil, &s)
						return s, err
					}, cliutil.WithConcurrency(sf.concurrency))
				for _, e := range errs {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: e.Source, Path: viRoot + "/HostSystem/" + e.Source + "/summary", Error: errText(e.Err)})
				}
				hostName := map[string]string{}
				for _, h := range inv.Hosts {
					hostName[h.ID] = h.Name
				}
				for _, r := range res {
					hw := r.Value.Hardware
					if hw == nil {
						view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: r.Source, Path: viRoot + "/HostSystem/" + r.Source + "/summary", Error: "host summary has no hardware section (host disconnected?)"})
						vHost.Rows = append(vHost.Rows, []string{hostName[r.Source], "", "", "", "", "", "", r.Source, "", vi.InstanceUUID})
						continue
					}
					vHost.Rows = append(vHost.Rows, []string{hostName[r.Source], i64(hw.NumCPUPkgs), i64(hw.NumCPUCores), mib(hw.MemorySize), i64(hw.NumNics),
						hw.Vendor, hw.Model, r.Source, hw.UUID, vi.InstanceUUID})
				}
				vSource.Rows = append(vSource.Rows, []string{vi.Name, vi.APIVersion, vi.Vendor, vi.InstanceUUID})
			}
			tabs := []*rvTab{vInfo, vCPU, vMemory, vDisk, vPartition, vNetwork, vTools, vHost, vSource, vSnapshot}
			warnFailures(cmd.ErrOrStderr(), "vCenter", view.FetchFailures, len(inv.VMs))

			if out == "" {
				view.Tabs = map[string][]map[string]string{}
				for _, t := range tabs {
					rows := make([]map[string]string, 0, len(t.Rows))
					for _, r := range t.Rows {
						m := make(map[string]string, len(t.Columns))
						for i, col := range t.Columns {
							m[col] = r[i]
						}
						rows = append(rows, m)
					}
					view.Tabs[t.File] = rows
				}
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if err := os.MkdirAll(out, 0o700); err != nil {
				return fmt.Errorf("creating --out %s: %w", out, err)
			}
			var incomplete []string
			for _, t := range tabs {
				path := filepath.Join(out, t.File)
				if err := writeCSV(path, t); err != nil {
					return err
				}
				if recordWithheld(path) {
					incomplete = append(incomplete, t.File)
				}
				view.Files = append(view.Files, path)
			}
			view.OutDir = out
			finished := func(err error) error {
				if err == nil && len(incomplete) > 0 {
					return fmt.Errorf("%s incomplete: a row repeated a credential this program holds and was withheld", strings.Join(incomplete, ", "))
				}
				return err
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return finished(printJSONFiltered(cmd.OutOrStdout(), view, flags))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %d RVTools tabs for %d VMs and %d hosts to %s\n", len(view.Files), view.VMs, view.Hosts, out)
			return finished(nil)
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().StringVar(&sf.scope.Folder, "folder", "", "Only VMs in this VM folder (id like group-v3, from 'folder')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().StringVar(&out, "out", "", "Directory for the RVTools_tab*.csv files (default: print the tabs as JSON)")
	return cmd
}

// spreadsheetSafe stops a vCenter-supplied value (VM name, snapshot
// description) that starts with =, +, -, @, tab or CR from running as a
// formula when the CSV is opened in a spreadsheet: it is prefixed with an
// apostrophe. Only the CSV files are changed; JSON output keeps raw values.
func spreadsheetSafe(cell string) string {
	if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
		return "'" + cell
	}
	return cell
}

// writeCSV writes t to path through a private temp file in the same
// directory, then renames it over path (so an existing file never keeps
// looser permissions or a half-written state).
func writeCSV(path string, t *rvTab) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".rvtools-*.csv")
	if err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	// Hand-fix (handfixes.json credkit-wiring): one row, one unit of a stream, through
	// the credential kit, so a held value split across rows is seen and a large tab
	// never spends the kit's budget as one unit.
	sink := credkit.NewWriter(tmp, credkit.SinkFile)
	writeRow := func(row []string) error {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write(row)
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}
		_, err := sink.Write(buf.Bytes())
		return err
	}
	if err := writeRow(t.Columns); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	for _, row := range t.Rows {
		safe := make([]string, len(row))
		for i, cell := range row {
			safe[i] = spreadsheetSafe(cell)
		}
		if err := writeRow(safe); err != nil {
			_ = tmp.Close()
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
