// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: per-cluster sizing totals for the migration target.
// pp:data-source live

package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/cliutil"
)

// capAcc accumulates one cluster (or the total) while walking.
type capAcc struct {
	Cluster, ClusterName                        string
	Hosts, HostsCounted, VMs, PoweredOnVMs      int
	SizedVMs, StorageVMs, GuestCountedVMs       int
	VCPU, VRAMMiB, PhysicalCores, PhysRAMMiB    int64
	ProvisionedBytes, CommittedBytes, GuestUsed int64
}

// capacityRow is the output. A figure that no read could supply is null,
// never 0: physical_* and the ratios need host hardware (VI/JSON),
// provisioned/committed need VI/JSON VM storage, guest_used needs VMware Tools.
// *_counted_* say how many hosts or VMs each figure covers.
type capacityRow struct {
	Cluster          string   `json:"cluster"`
	ClusterName      string   `json:"cluster_name"`
	Hosts            int      `json:"hosts"`
	VMs              int      `json:"vms"`
	PoweredOnVMs     int      `json:"powered_on_vms"`
	SizedVMs         int      `json:"sizing_counted_vms"`
	VCPU             *int64   `json:"vcpu"`
	VRAMMiB          *int64   `json:"vram_MiB"`
	HostsCounted     int      `json:"hosts_hardware_counted"`
	PhysicalCores    *int64   `json:"physical_cores"`
	PhysicalRAMMiB   *int64   `json:"physical_ram_MiB"`
	VCPUPerCore      *float64 `json:"vcpu_per_physical_core"`
	VRAMPerRAM       *float64 `json:"vram_per_physical_ram"`
	StorageVMs       int      `json:"storage_counted_vms"`
	ProvisionedBytes *int64   `json:"provisioned_bytes"`
	CommittedBytes   *int64   `json:"committed_bytes"`
	GuestCountedVMs  int      `json:"guest_counted_vms"`
	GuestUsedBytes   *int64   `json:"guest_used_bytes"`
}

func (a *capAcc) row() capacityRow {
	r := capacityRow{Cluster: a.Cluster, ClusterName: a.ClusterName, Hosts: a.Hosts, VMs: a.VMs, PoweredOnVMs: a.PoweredOnVMs,
		SizedVMs: a.SizedVMs, HostsCounted: a.HostsCounted, StorageVMs: a.StorageVMs, GuestCountedVMs: a.GuestCountedVMs}
	i := func(v int64) *int64 { return &v }
	f := func(v float64) *float64 { return &v }
	// Each sum covers only the items whose read supplied it (see *_counted_*);
	// it is null when none did. Ratios need both sides complete.
	if a.VMs == 0 || a.SizedVMs > 0 {
		r.VCPU, r.VRAMMiB = i(a.VCPU), i(a.VRAMMiB)
	}
	if a.Hosts == 0 || a.HostsCounted > 0 {
		r.PhysicalCores, r.PhysicalRAMMiB = i(a.PhysicalCores), i(a.PhysRAMMiB)
	}
	if a.SizedVMs == a.VMs && a.HostsCounted == a.Hosts && a.Hosts > 0 {
		if a.PhysicalCores > 0 {
			r.VCPUPerCore = f(roundTo(float64(a.VCPU)/float64(a.PhysicalCores), 2))
		}
		if a.PhysRAMMiB > 0 {
			r.VRAMPerRAM = f(roundTo(float64(a.VRAMMiB)/float64(a.PhysRAMMiB), 2))
		}
	}
	if a.VMs == 0 || a.StorageVMs > 0 {
		r.ProvisionedBytes, r.CommittedBytes = i(a.ProvisionedBytes), i(a.CommittedBytes)
	}
	if a.VMs == 0 || a.GuestCountedVMs > 0 {
		r.GuestUsedBytes = i(a.GuestUsed)
	}
	return r
}

// datastoreTotals is vCenter-wide (datastores are shared across clusters), so
// it is reported only for a whole-vCenter run; it is null when skipped or when
// the datastore list could not be read. The sums cover datastores that
// reported both capacity and free space (datastores_counted) and are null
// when none did.
type datastoreTotals struct {
	Datastores    int    `json:"datastores"`
	Counted       int    `json:"datastores_counted"`
	CapacityBytes *int64 `json:"capacity_bytes"`
	FreeBytes     *int64 `json:"free_bytes"`
}

type capacityView struct {
	VIJSON        viStatus      `json:"vi_json"`
	Clusters      []capacityRow `json:"clusters"`
	Total         capacityRow   `json:"total"`
	total         capAcc
	Datastores    *datastoreTotals `json:"datastores"`
	ScannedVMs    int              `json:"scanned_vms"`
	FetchFailures []fetchFailure   `json:"fetch_failures"`
}

func newNovelMigrationCapacityCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	var poweredOnOnly bool
	cmd := &cobra.Command{
		Use:   "capacity",
		Short: "Per-cluster sizing totals: vCPU/vRAM vs physical cores/RAM and provisioned vs committed vs guest-used storage",
		Long: `Use this command for per-cluster totals that size the Proxmox target.
Do NOT use this command for Broadcom licensable-core counts; use 'licence exposure' instead.

Storage has three numbers: provisioned (committed + uncommitted, what thin disks
may grow to), committed (on the datastores today) and guest_used (what the guest
filesystems hold, needs VMware Tools). *_counted_* fields say how many VMs or
hosts each total covers; a failed read is listed in fetch_failures and never
counted as zero. Physical cores and RAM, provisioned and committed storage need
the VI/JSON API (vCenter 8.0 Update 1 or later).`,
		Example: strings.Trim(`
  vsphere-cli migration capacity --agent
  vsphere-cli migration capacity --powered-on-only --cluster domain-c8 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "migration capacity")
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
			vms := inv.VMs
			if poweredOnOnly {
				vms = make([]invVM, 0, len(inv.VMs))
				for _, v := range inv.VMs {
					if v.PowerState == "POWERED_ON" {
						vms = append(vms, v)
					}
				}
			}
			details, failures := gatherDetails(ctx, c, vms, detailWant{Guest: true, Storage: true}, vi, sf.concurrency)
			view := capacityView{VIJSON: vi, Clusters: make([]capacityRow, 0), ScannedVMs: len(vms),
				FetchFailures: nonNilFailures(append(inv.Failures, failures...))}

			summaries := map[string]hostSummary{}
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
				for _, r := range res {
					summaries[r.Source] = r.Value
				}
			}

			rows := map[string]*capAcc{}
			row := func(id, name string) *capAcc {
				if id == "" {
					id, name = "", "(standalone hosts)"
				}
				r := rows[id]
				if r == nil {
					r = &capAcc{Cluster: id, ClusterName: name}
					rows[id] = r
				}
				return r
			}
			view.total.ClusterName = "total"
			for _, h := range inv.Hosts {
				r := row(h.Cluster, h.ClusterName)
				for _, acc := range []*capAcc{r, &view.total} {
					acc.Hosts++
					if s, ok := summaries[h.ID]; ok && s.Hardware != nil && s.Hardware.NumCPUCores > 0 {
						acc.HostsCounted++
						acc.PhysicalCores += s.Hardware.NumCPUCores
						acc.PhysRAMMiB += s.Hardware.MemorySize >> 20
					}
				}
			}
			for _, v := range vms {
				r := row(v.Cluster, v.ClusterName)
				d := details[v.ID]
				for _, acc := range []*capAcc{r, &view.total} {
					acc.VMs++
					if v.PowerState == "POWERED_ON" {
						acc.PoweredOnVMs++
					}
					if v.CPUCount != nil && v.MemoryMiB != nil {
						acc.SizedVMs++
						acc.VCPU += *v.CPUCount
						acc.VRAMMiB += *v.MemoryMiB
					}
					if d == nil {
						continue
					}
					if d.Storage != nil {
						acc.StorageVMs++
						for _, u := range d.Storage.PerDatastoreUsage {
							acc.CommittedBytes += u.Committed
							acc.ProvisionedBytes += u.Committed + u.Uncommitted
						}
					}
					if d.Guest != nil {
						acc.GuestCountedVMs++
						for _, fs := range d.Guest {
							if fs.Capacity > fs.FreeSpace {
								acc.GuestUsed += fs.Capacity - fs.FreeSpace
							}
						}
					}
				}
			}
			for _, k := range sortedKeys(rows) {
				view.Clusters = append(view.Clusters, rows[k].row())
			}
			view.Total = view.total.row()

			if sf.scope.Cluster == "" && sf.scope.Host == "" {
				var ds []struct {
					Capacity  *int64 `json:"capacity"`
					FreeSpace *int64 `json:"free_space"`
				}
				if err := getJSON(ctx, c, "/api/vcenter/datastore", nil, &ds); err != nil {
					view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "datastores", Path: "/api/vcenter/datastore", Error: errText(err)})
				} else {
					t := &datastoreTotals{Datastores: len(ds)}
					var capSum, freeSum int64
					for _, d := range ds {
						if d.Capacity != nil && d.FreeSpace != nil {
							t.Counted++
							capSum += *d.Capacity
							freeSum += *d.FreeSpace
						}
					}
					if t.Counted > 0 || t.Datastores == 0 {
						t.CapacityBytes, t.FreeBytes = &capSum, &freeSum
					}
					view.Datastores = t
				}
			}
			warnFailures(cmd.ErrOrStderr(), "vCenter", view.FetchFailures, len(vms)+len(inv.Hosts))
			if !vi.Available {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: "+vi.Reason+"; physical cores/RAM and provisioned/committed storage are not counted")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			gibOrDash := func(v *int64) any {
				if v == nil {
					return "-"
				}
				return *v >> 30
			}
			ratioOrDash := func(v *float64) any {
				if v == nil {
					return "-"
				}
				return *v
			}
			table := make([]map[string]any, 0, len(view.Clusters)+1)
			for _, r := range append(view.Clusters, view.Total) {
				vcpu, vram := any("-"), any("-")
				if r.VCPU != nil {
					vcpu, vram = *r.VCPU, *r.VRAMMiB>>10
				}
				table = append(table, map[string]any{"cluster": r.ClusterName, "hosts": r.Hosts, "vms": r.VMs, "vcpu": vcpu,
					"vcpu/core": ratioOrDash(r.VCPUPerCore), "vram_GiB": vram, "provisioned_GiB": gibOrDash(r.ProvisionedBytes), "guest_used_GiB": gibOrDash(r.GuestUsedBytes)})
			}
			return printAutoTable(cmd.OutOrStdout(), table)
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().BoolVar(&poweredOnOnly, "powered-on-only", false, "Count only powered-on VMs")
	return cmd
}
