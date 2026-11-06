// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written novel command: map each VM onto Proxmox VE qm parameters.
// pp:data-source live

package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type proxmoxMapView struct {
	VMs           []proxmoxPlan  `json:"vms"`
	ScannedVMs    int            `json:"scanned_vms"`
	FetchFailures []fetchFailure `json:"fetch_failures"`
}

func newNovelMigrationProxmoxMapCmd(flags *rootFlags) *cobra.Command {
	var sf scopeFlags
	cmd := &cobra.Command{
		Use:   "proxmox-map",
		Short: "Each VM's sizing, firmware, controllers, NICs and guest OS mapped to Proxmox VE qm settings",
		Long: `Use this command for each VM's source sizing and the Proxmox VE settings it maps to.
Do NOT use this command to find what stops a VM from migrating; use 'migration blockers' instead. Do NOT use it for cluster totals; use 'migration capacity' instead.

The proxmox object uses qm parameter names (bios, efidisk0, ostype, cores,
sockets, memory, scsihw, netN, scsiN/sataN/ideN, boot, smbios1, serialN). The
rules follow Proxmox VE's own VMware import: OVMF plus a new efidisk0 for UEFI
VMs, LSI Logic -> lsi and PVSCSI -> pvscsi, Linux defaulting to
virtio-scsi-single, NIC models and MAC addresses kept, NVMe disks attached as
SCSI, CD-ROMs left empty. Disk entries name the source VMDK and size; they are
hints for an import, not a ready-to-run qm command.`,
		Example: strings.Trim(`
  vsphere-cli migration proxmox-map --vm vm-1021 --agent
  vsphere-cli migration proxmox-map --cluster domain-c8 --agent`, "\n"),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "migration proxmox-map")
			}
			if sf.scope.VM != "" && (sf.scope.Cluster != "" || sf.scope.Host != "" || sf.scope.Folder != "") {
				return usageErr(fmt.Errorf("--vm cannot be combined with --cluster, --host or --folder"))
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
			details, failures := gatherDetails(ctx, c, inv.VMs, detailWant{Info: true}, viStatus{}, sf.concurrency)
			view := proxmoxMapView{VMs: make([]proxmoxPlan, 0, len(inv.VMs)), ScannedVMs: len(inv.VMs),
				FetchFailures: nonNilFailures(append(inv.Failures, failures...))}
			names, nerr := networkNames(ctx, c)
			if nerr != nil {
				view.FetchFailures = append(view.FetchFailures, fetchFailure{Item: "networks", Path: "/api/vcenter/network", Error: errText(nerr)})
			}
			for _, vm := range inv.VMs {
				if d := details[vm.ID]; d != nil && d.Info != nil {
					fillNetworkNames(d.Info, names)
					plan := planProxmox(vm.ID, *d.Info)
					if vm.HostName != "" {
						plan.Source["host_name"], plan.Source["cluster_name"] = vm.HostName, vm.ClusterName
					}
					view.VMs = append(view.VMs, plan)
				}
			}
			warnFailures(cmd.ErrOrStderr(), "VM detail", view.FetchFailures, len(inv.VMs))
			if !wantsHumanTable(cmd.OutOrStdout(), flags) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			if len(view.VMs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No VMs matched.")
				return nil
			}
			rows := make([]map[string]any, 0, len(view.VMs))
			for _, p := range view.VMs {
				rows = append(rows, map[string]any{"vm": p.VM, "name": p.Name, "bios": p.Proxmox["bios"], "ostype": p.Proxmox["ostype"],
					"cores": p.Proxmox["cores"], "sockets": p.Proxmox["sockets"], "memory": p.Proxmox["memory"], "scsihw": p.Proxmox["scsihw"], "warnings": len(p.Warnings)})
			}
			return printAutoTable(cmd.OutOrStdout(), rows)
		},
	}
	cmd.Flags().StringVar(&sf.scope.Cluster, "cluster", "", "Only this cluster (id like domain-c8, from 'cluster list')")
	cmd.Flags().StringVar(&sf.scope.Host, "host", "", "Only this ESXi host (id like host-10, from 'host')")
	cmd.Flags().StringVar(&sf.scope.Folder, "folder", "", "Only VMs in this VM folder (id like group-v3, from 'folder')")
	cmd.Flags().IntVar(&sf.concurrency, "concurrency", 8, "Parallel vCenter reads")
	cmd.Flags().StringVar(&sf.scope.VM, "vm", "", "Only this VM (id like vm-1021, from 'vm list')")
	return cmd
}
