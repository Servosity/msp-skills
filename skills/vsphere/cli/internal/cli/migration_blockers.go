// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: per-VM migration go/no-go with reason codes.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type blockerRow struct {
	VM            string    `json:"vm"`
	Name          string    `json:"name"`
	PowerState    string    `json:"power_state"`
	Host          string    `json:"host_name,omitempty"`
	Cluster       string    `json:"cluster_name,omitempty"`
	Verdict       string    `json:"verdict"`
	Blockers      []blocker `json:"blockers"`
	UnknownChecks []string  `json:"unknown_checks"`
}

type blockersView struct {
	VIJSON        viStatus       `json:"vi_json"`
	Summary       map[string]int `json:"summary"`
	VMs           []blockerRow   `json:"vms"`
	ScannedVMs    int            `json:"scanned_vms"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

var severityRank = map[string]int{sevInfo: 1, sevWarn: 2, sevBlock: 3}

func newNovelMigrationBlockersCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	var severity string
	cmd := &cobra.Command{
		Use:   "blockers",
		Short: "Per-VM migration verdict (blocked, review, ready, unknown) with reason codes",
		Long: `Use this command for a per-VM go/no-go list of what stops a VM from migrating.
Do NOT use this command for snapshot ages, sizes or cleanup review; use 'snapshots audit' instead.

Verdicts: blocked (a block-severity reason), review (warnings), ready, or unknown
(a check could not run, e.g. snapshots on vCenter 7.0 without the VI/JSON API,
and nothing blocking was found). A check that cannot run is never a pass.

Reason codes: block SNAPSHOTS_PRESENT, VSAN_STORAGE (the Proxmox importer cannot
read a vSAN-backed disk), ENCRYPTED_DISK (the VM or a disk is encrypted with a
vCenter key provider), CONSOLIDATION_NEEDED (vCenter's
runtime.consolidationNeeded flag); warn DELTA_CHAIN_WITHOUT_SNAPSHOTS (a linked
clone or leftover deltas), ISO_ATTACHED, PVSCSI_CONTROLLER, NVME_DISK,
TOOLS_NOT_INSTALLED, TOOLS_NOT_RUNNING; info EFI_FIRMWARE, VMXNET_NIC.
A disk whose datastore cannot be resolved to one type leaves vsan_storage
unknown. Snapshot, delta-chain, consolidation and encryption checks need
vCenter 8.0 Update 1 or later (VI/JSON API); on older vCenters they are
reported as unknown.`,
		Example: strings.Trim(`
  vsphere-cli migration blockers --agent
  vsphere-cli migration blockers --cluster domain-c8 --severity block --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "migration blockers")
			}
			if severity != "" && severityRank[severity] == 0 {
				return usageErr(fmt.Errorf("--severity must be block, warn or info"))
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
			details, failures := gatherDetails(ctx, c, inv.VMs, detailWant{Info: true, Tools: true, Snapshots: true, Layout: true, Summary: true, Config: true}, vi, sf.concurrency)
			storage, storageErr := datastoreTypes(ctx, c)
			view := blockersView{VIJSON: vi, Summary: map[string]int{"blocked": 0, "review": 0, "ready": 0, "unknown": 0},
				VMs: make([]blockerRow, 0), ScannedVMs: len(inv.VMs), FetchFailures: nonNilFailures(append(inv.Failures, failures...))}
			if storageErr != nil {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "datastores", Path: "/api/vcenter/datastore", Error: errText(storageErr)})
			}
			for _, vm := range inv.VMs {
				d := details[vm.ID]
				if d == nil {
					d = &vmDetail{}
				}
				found, unknown, verdict := evaluateBlockers(blockerInput{DatastoreTypes: storage, Info: d.Info, Tools: d.Tools, Snapshots: d.Snapshots, Layout: d.Layout, Summary: d.Summary, Config: d.Config})
				view.Summary[verdict]++
				if severity != "" && !hasSeverity(found, severity) {
					continue
				}
				view.VMs = append(view.VMs, blockerRow{VM: vm.ID, Name: vm.Name, PowerState: vm.PowerState, Host: vm.HostName,
					Cluster: vm.ClusterName, Verdict: verdict, Blockers: found, UnknownChecks: unknown})
			}
			warnFailures(cmd.ErrOrStderr(), "vCenter", view.FetchFailures, len(inv.VMs))
			if !vi.Available {
				fmt.Fprintln(cmd.ErrOrStderr(), "note: "+vi.Reason+"; snapshot, delta-disk and encryption checks are reported as unknown")
			}
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.VMs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No VMs matched.")
				return nil
			}
			rows := make([]map[string]any, 0, len(view.VMs))
			for _, r := range view.VMs {
				codes := make([]string, 0, len(r.Blockers))
				for _, b := range r.Blockers {
					codes = append(codes, b.Code)
				}
				rows = append(rows, map[string]any{"vm": r.VM, "name": r.Name, "verdict": r.Verdict, "reasons": strings.Join(codes, ","), "unknown": strings.Join(r.UnknownChecks, ",")})
			}
			if err := printAutoTable(cmd.OutOrStdout(), rows); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nblocked %d  review %d  ready %d  unknown %d\n", view.Summary["blocked"], view.Summary["review"], view.Summary["ready"], view.Summary["unknown"])
			return nil
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().StringVar(&sf.scope.Folder, "folder", "", "Only VMs in this VM folder (id like group-v3, from 'folder')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().StringVar(&severity, "severity", "", "Only list VMs with at least one reason of this severity or worse: block, warn, info")
	return cmd
}

func hasSeverity(found []blocker, min string) bool {
	for _, b := range found {
		if severityRank[b.Severity] >= severityRank[min] {
			return true
		}
	}
	return false
}
