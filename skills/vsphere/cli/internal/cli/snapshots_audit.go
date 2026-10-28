// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: every snapshot in the vCenter as a flat row.
// pp:data-source live

package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"vsphere-pp-cli/internal/cliutil"
)

// vmSnapshotSummary uses each VI/JSON read on its own: a field is null when
// the read it needs failed, never a guessed zero or false.
type vmSnapshotSummary struct {
	VM                    string   `json:"vm"`
	VMName                string   `json:"vm_name"`
	SnapshotCount         *int     `json:"snapshot_count"`  // null: snapshot tree unavailable
	MaxDepth              *int     `json:"max_depth"`       // ditto
	OldestAgeDays         *float64 `json:"oldest_age_days"` // null also when there are no snapshots
	DeltaDiskBytes        *int64   `json:"delta_disk_bytes"`
	SnapshotFileBytes     *int64   `json:"snapshot_file_bytes"`
	ConsolidationNeeded   *bool    `json:"consolidation_needed"`          // vCenter runtime.consolidationNeeded
	DeltaChainNoSnapshots *bool    `json:"delta_chain_without_snapshots"` // needs tree + layout; linked clone or leftover deltas
}

type snapshotsAuditView struct {
	VIJSON        viStatus            `json:"vi_json"`
	Snapshots     []snapshotRow       `json:"snapshots"`
	VMs           []vmSnapshotSummary `json:"vms"`
	ScannedVMs    int                 `json:"scanned_vms"`
	UnknownVMs    int                 `json:"unknown_vms"` // VMs whose snapshot tree could not be read (listed in vms with snapshot_count null)
	FetchFailures []fetchFailure      `json:"fetch_failures"`
}

func newNovelSnapshotsAuditCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	var olderThan string
	var minSize int64
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Every snapshot with age and chain depth, per-VM delta-disk totals, and VMs that need consolidation",
		Long: `Use this command for every snapshot's age, chain depth and delta size across the vCenter.
Do NOT use this command to decide whether a VM is ready to migrate; use 'migration blockers' instead.

Snapshot rows come from each VM's VI/JSON snapshot tree; delta_disk_bytes and
snapshot_file_bytes are per-VM totals in vms[] from the VM file layout (every
disk-chain link after the base disk across all snapshot branches, plus snapshot
state and memory files); snapshot rows carry age, depth and quiesced.
consolidation_needed is vCenter's own flag; delta_chain_without_snapshots marks
a VM with no snapshots whose disk chain is deeper than one file (a linked clone,
or leftover deltas). Needs vCenter 8.0 Update 1 or later (VI/JSON API) and
fails on older vCenters.`,
		Example: strings.Trim(`
  vsphere-cli snapshots audit --older-than 7d --agent
  vsphere-cli snapshots audit --cluster domain-c8 --min-size 10737418240 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "snapshots audit")
			}
			var minAge time.Duration
			if olderThan != "" {
				d, err := cliutil.ParseDurationLoose(olderThan)
				if err != nil || d < 0 {
					return usageErr(fmt.Errorf("--older-than %q: want a duration like 7d, 36h or 2w", olderThan))
				}
				minAge = d
			}
			if minSize < 0 {
				return usageErr(fmt.Errorf("--min-size must be zero or more bytes"))
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
			vi := probeVI(ctx, c)
			if !vi.Available {
				return classifyAPIError(cmd.OutOrStdout(), fmt.Errorf("snapshots audit needs the VI/JSON API: %s", vi.Reason), flags)
			}
			inv, err := walkInventory(ctx, c, sf.scope, sf.concurrency, true)
			if err != nil {
				return classifyAPIError(cmd.OutOrStdout(), err, flags)
			}
			details, failures := gatherDetails(ctx, c, inv.VMs, detailWant{Snapshots: true, Layout: true, Summary: true}, vi, sf.concurrency)
			now := time.Now().UTC()
			view := snapshotsAuditView{VIJSON: vi, Snapshots: make([]snapshotRow, 0), VMs: make([]vmSnapshotSummary, 0),
				ScannedVMs: len(inv.VMs), FetchFailures: nonNilFailures(append(inv.Failures, failures...))}
			for _, vm := range inv.VMs {
				d := details[vm.ID]
				if d == nil {
					d = &vmDetail{} // never read (timeout/cancel): reported as unknown, not dropped
				}
				sum := vmSnapshotSummary{VM: vm.ID, VMName: vm.Name}
				var rows []snapshotRow
				if d.Snapshots != nil {
					rows = flattenSnapshots(vm, *d.Snapshots, now)
					count, depth := len(rows), 0
					var oldest float64 = -1
					for _, r := range rows {
						if r.Depth > depth {
							depth = r.Depth
						}
						if r.AgeDays > oldest {
							oldest = r.AgeDays
						}
					}
					sum.SnapshotCount, sum.MaxDepth = &count, &depth
					if oldest >= 0 {
						sum.OldestAgeDays = &oldest
					}
				}
				var chain *diskChains
				if d.Layout != nil {
					ch := summarizeLayout(*d.Layout)
					chain = &ch
					sum.DeltaDiskBytes, sum.SnapshotFileBytes = &ch.DeltaBytes, &ch.SnapshotFileBytes
				}
				if chain != nil && sum.SnapshotCount != nil {
					v := *sum.SnapshotCount == 0 && chain.MaxChainLength > 1
					sum.DeltaChainNoSnapshots = &v
				}
				if d.Summary != nil {
					sum.ConsolidationNeeded = d.Summary.Runtime.ConsolidationNeeded
				}
				flagged := (sum.DeltaChainNoSnapshots != nil && *sum.DeltaChainNoSnapshots) ||
					(sum.ConsolidationNeeded != nil && *sum.ConsolidationNeeded)
				if sum.SnapshotCount == nil {
					// Unknown is not "no snapshots": keep the VM so the gap
					// shows, whatever the filters say.
					view.UnknownVMs++
					view.VMs = append(view.VMs, sum)
					continue
				}
				if len(rows) == 0 && !flagged {
					continue
				}
				// An unknown size cannot be ruled out by --min-size.
				if minSize > 0 && sum.DeltaDiskBytes != nil && *sum.DeltaDiskBytes+*sum.SnapshotFileBytes < minSize {
					continue
				}
				kept := 0
				for _, r := range rows {
					if minAge > 0 {
						created, err := time.Parse(time.RFC3339, r.CreateTime)
						if err != nil || now.Sub(created) < minAge {
							continue // exact cutoff, not the rounded age_days
						}
					}
					view.Snapshots = append(view.Snapshots, r)
					kept++
				}
				if kept > 0 || (minAge == 0 && flagged) {
					view.VMs = append(view.VMs, sum)
				}
			}
			sort.SliceStable(view.Snapshots, func(i, j int) bool { return view.Snapshots[i].AgeDays > view.Snapshots[j].AgeDays })
			warnFailures(cmd.ErrOrStderr(), "VM snapshot", view.FetchFailures, len(inv.VMs))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if view.UnknownVMs > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "snapshot data unavailable for %d of %d VMs (see --json fetch_failures)\n", view.UnknownVMs, len(inv.VMs))
			}
			if len(view.Snapshots) == 0 && len(view.VMs) == view.UnknownVMs {
				if view.UnknownVMs == 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "No matching snapshots across %d VMs.\n", len(inv.VMs))
				}
				return nil
			}
			rows := make([]map[string]any, 0, len(view.Snapshots))
			for _, r := range view.Snapshots {
				rows = append(rows, map[string]any{"vm": r.VMName, "snapshot": r.Name, "created": r.CreateTime, "age_days": r.AgeDays, "depth": r.Depth, "current": r.IsCurrent})
			}
			if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
				return err
			}
			for _, v := range view.VMs {
				if v.ConsolidationNeeded != nil && *v.ConsolidationNeeded {
					fmt.Fprintf(cmd.OutOrStdout(), "consolidation needed: %s (%s)\n", v.VMName, v.VM)
				} else if v.DeltaChainNoSnapshots != nil && *v.DeltaChainNoSnapshots {
					fmt.Fprintf(cmd.OutOrStdout(), "delta chain without snapshots: %s (%s)\n", v.VMName, v.VM)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().StringVar(&sf.scope.Folder, "folder", "", "Only VMs in this VM folder (id like group-v3, from 'folder')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "Only snapshots at least this old (e.g. 7d, 36h, 2w)")
	cmd.Flags().Int64Var(&minSize, "min-size", 0, "Only VMs whose snapshot delta + state files total at least this many bytes")
	return cmd
}
